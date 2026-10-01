package overlay_test

import (
	"bytes"
	"context"
	"strings"
	"sync"
	"testing"
	"time"

	"trans/internal/overlay"
	"trans/internal/promptflow"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/exp/teatest"
	"github.com/muesli/termenv"
)

func numberedEnglish(sentences int) string {
	var written strings.Builder
	for number := 1; number <= sentences; number++ {
		written.WriteString("Sentence number ")
		written.WriteString(strings.Repeat("x", 40))
		written.WriteString(" ")
		written.WriteString(marker(number))
		written.WriteString(". ")
	}
	return written.String()
}

func marker(number int) string {
	return "END" + strings.Repeat("0", 3-len(itoa(number))) + itoa(number)
}

func itoa(number int) string {
	if number < 10 {
		return string(rune('0' + number))
	}
	return string(rune('0'+number/10)) + string(rune('0'+number%10))
}

func reader(t *testing.T) tea.Model {
	t.Helper()
	previous := lipgloss.ColorProfile()
	lipgloss.SetColorProfile(termenv.ANSI)
	t.Cleanup(func() { lipgloss.SetColorProfile(previous) })

	target := &recordingTarget{}
	english := numberedEnglish(20)
	flow := promptflow.New(stubTranslator{english: english}, target, target,
		promptflow.WithPreviewTranslator(stubTranslator{english: english}))
	var model tea.Model = overlay.New(context.Background(), flow, overlay.Options{
		Service: "deepl", Language: "EN-US", Live: true, Debounce: time.Millisecond,
	})
	model, _ = model.Update(tea.WindowSizeMsg{Width: 87, Height: 15})
	model, _ = model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("Bitte behebe den Test.")})
	model, _ = model.Update(overlay.PreviewShown("Bitte behebe den Test.", english))
	return model
}

func TestTabTurnsTheTranslationIntoSomethingReadable(t *testing.T) {
	model := reader(t)

	writing := model.View()
	if !strings.Contains(writing, marker(1)) {
		t.Errorf("while writing, the translation does not start at its beginning:\n%s", writing)
	}

	model, _ = model.Update(tea.KeyMsg{Type: tea.KeyTab})
	reading := model.View()

	if !strings.Contains(reading, marker(1)) {
		t.Errorf("reading does not start at the beginning:\n%s", reading)
	}
	if strings.Contains(reading, "Bitte behebe den Test.") {
		t.Error("the draft is still on screen while reading, so there is no room to read")
	}
	if rows := strings.Count(reading, "\n"); rows != strings.Count(writing, "\n") {
		t.Errorf("reading is %d rows and writing %d, want the popup unchanged", rows,
			strings.Count(writing, "\n"))
	}
}

func TestReadingScrollsWithTheArrowsAndBack(t *testing.T) {
	model := reader(t)
	model, _ = model.Update(tea.KeyMsg{Type: tea.KeyTab})

	first := model.View()
	for range 3 {
		model, _ = model.Update(tea.KeyMsg{Type: tea.KeyDown})
	}
	scrolled := model.View()
	if scrolled == first {
		t.Error("the arrows do not scroll the translation")
	}

	for range 10 {
		model, _ = model.Update(tea.KeyMsg{Type: tea.KeyUp})
	}
	if back := model.View(); back != first {
		t.Error("scrolling back up does not return to the beginning")
	}

	model, _ = model.Update(tea.KeyMsg{Type: tea.KeyPgDown})
	if paged := model.View(); paged == first {
		t.Error("page down does not move")
	}
}

func TestReadingReachesTheEndAndStops(t *testing.T) {
	model := reader(t)
	model, _ = model.Update(tea.KeyMsg{Type: tea.KeyTab})

	for range 200 {
		model, _ = model.Update(tea.KeyMsg{Type: tea.KeyDown})
	}
	atEnd := model.View()
	if !strings.Contains(atEnd, marker(20)) {
		t.Errorf("scrolling down does not reach the end:\n%s", atEnd)
	}

	model, _ = model.Update(tea.KeyMsg{Type: tea.KeyDown})
	if past := model.View(); past != atEnd {
		t.Error("the translation scrolls past its own end")
	}
}

// Reading is not a place to get stuck: writing a letter goes back to the draft and
// types it, and so does escape without typing anything.
func TestWritingAnythingGoesBackToTheDraft(t *testing.T) {
	model := reader(t)
	model, _ = model.Update(tea.KeyMsg{Type: tea.KeyTab})
	model, _ = model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("X")})

	back := model.View()
	if !strings.Contains(back, "Bitte behebe den Test.X") {
		t.Errorf("the letter did not reach the draft:\n%s", back)
	}

	model, _ = model.Update(tea.KeyMsg{Type: tea.KeyTab})
	model, _ = model.Update(tea.KeyMsg{Type: tea.KeyEsc})
	if !strings.Contains(model.View(), "Bitte behebe den Test.X") {
		t.Error("escape did not come back from reading")
	}
}

func TestTheReadingFooterSaysHowToGetAroundAndOut(t *testing.T) {
	model := reader(t)
	model, _ = model.Update(tea.KeyMsg{Type: tea.KeyTab})

	footer := lastLine(model.View())
	for _, key := range []string{"tab", "esc"} {
		if !strings.Contains(footer, key) {
			t.Errorf("the reading footer does not mention %s: %q", key, footer)
		}
	}
	if lipgloss.Width(footer) > 87 {
		t.Errorf("the reading footer is %d columns: %q", lipgloss.Width(footer), footer)
	}
}

// slowPrompter holds a send on its way until the test lets it go, and counts
// every call that reaches it.
type slowPrompter struct {
	mu       sync.Mutex
	submits  int
	delivers int
	started  chan struct{}
	release  chan struct{}
	once     sync.Once
}

func (s *slowPrompter) Submit(_ context.Context, _ string, _ promptflow.Delivery) (string, error) {
	s.once.Do(func() { close(s.started) })
	s.mu.Lock()
	s.submits++
	s.mu.Unlock()
	<-s.release
	return english, nil
}

func (s *slowPrompter) Translate(context.Context, string) (string, error) {
	return english, nil
}

func (s *slowPrompter) Deliver(_ context.Context, _ string, _ promptflow.Delivery) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.delivers++
	return nil
}

func (s *slowPrompter) Usage(context.Context) (promptflow.Usage, bool, error) {
	return promptflow.Usage{}, false, nil
}

func (s *slowPrompter) counts() (submits, delivers int) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.submits, s.delivers
}

// Tab to reading is free while a send is on its way, but the send key is not:
// pressing it from reading must not start a second Submit or Deliver of the
// same draft while the first one is still out.
func TestSendingFromReadingWhileASendIsInFlightSendsNothingTwice(t *testing.T) {
	t.Parallel()
	prompter := &slowPrompter{
		started: make(chan struct{}),
		release: make(chan struct{}),
	}
	var releaseOnce sync.Once
	release := func() { releaseOnce.Do(func() { close(prompter.release) }) }
	t.Cleanup(release)

	overlayUnderTest := teatest.NewTestModel(t,
		overlay.New(context.Background(), prompter, overlay.Options{
			Service: "deepl", Language: "EN-US",
		}),
		teatest.WithInitialTermSize(87, 17))

	overlayUnderTest.Type(draft)
	overlayUnderTest.Send(tea.KeyMsg{Type: tea.KeyCtrlD})

	select {
	case <-prompter.started:
	case <-time.After(2 * time.Second):
		t.Fatal("the send never started")
	}

	// Reading is reachable while the send runs — the send key is not.
	overlayUnderTest.Send(tea.KeyMsg{Type: tea.KeyTab})
	overlayUnderTest.Send(tea.KeyMsg{Type: tea.KeyCtrlD})
	time.Sleep(300 * time.Millisecond)

	if submits, delivers := prompter.counts(); submits != 1 || delivers != 0 {
		t.Errorf("the send key from reading started %d submits and %d deliveries while one was in flight, want none",
			submits, delivers)
	}

	// The one send on its way finishes, and the panel is ready again.
	release()
	waitForTheNextPrompt(t, overlayUnderTest)

	if submits, delivers := prompter.counts(); submits != 1 || delivers != 0 {
		t.Errorf("the prompt reached the target as %d submits and %d deliveries, want it sent once",
			submits, delivers)
	}

	closeTheOverlay(t, overlayUnderTest)
}

// The English waits for its go-ahead while confirming; the send key from reading
// must give it rather than fall silent.
func TestSendingFromReadingWhileConfirmingDeliversTheEnglish(t *testing.T) {
	t.Parallel()
	target := &recordingTarget{}

	overlayUnderTest := confirmingOverlay(t, markingTranslator{}, target)
	overlayUnderTest.Type("erste Fassung")
	overlayUnderTest.Send(tea.KeyMsg{Type: tea.KeyCtrlD})

	teatest.WaitFor(t, overlayUnderTest.Output(), func(out []byte) bool {
		return bytes.Contains(out, []byte("EN(erste Fassung)"))
	}, teatest.WithDuration(frameTimeout))

	overlayUnderTest.Send(tea.KeyMsg{Type: tea.KeyTab})
	overlayUnderTest.Send(tea.KeyMsg{Type: tea.KeyCtrlD})
	waitForTheNextPrompt(t, overlayUnderTest)

	if len(target.sent()) != 1 || target.sent()[0] != "EN(erste Fassung)" {
		t.Errorf("target received %v, want the confirmed English delivered once", target.sent())
	}

	closeTheOverlay(t, overlayUnderTest)
}
