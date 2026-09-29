package selection_test

import (
	"bytes"
	"context"
	"errors"
	"io"
	"strings"
	"sync"
	"testing"
	"time"

	"trans/internal/frame"
	"trans/internal/selection"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/exp/teatest"
)

const (
	selected = "Bitte behebe den fehlschlagenden Test"
	english  = "Please fix the failing test"
)

type stubTranslator struct {
	english string

	mu    sync.Mutex
	calls int
}

func (s *stubTranslator) Translate(context.Context, string) (string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.calls++
	return s.english, nil
}

func (s *stubTranslator) count() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.calls
}

// flaky fails the way a service does when it is unreachable, and answers the
// second time — the state ctrl+t exists for.
type flakyTranslator struct {
	english string

	mu    sync.Mutex
	calls int
}

func (f *flakyTranslator) Translate(context.Context, string) (string, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls++
	if f.calls == 1 {
		return "", errors.New("deepl unreachable")
	}
	return f.english, nil
}

// gated holds the answer until the test lets it go, so the window can be read
// while the translation is on its way.
type gatedTranslator struct {
	english string
	started chan struct{}
	release chan struct{}
	once    sync.Once
}

func (g *gatedTranslator) Translate(ctx context.Context, _ string) (string, error) {
	g.once.Do(func() { close(g.started) })
	select {
	case <-g.release:
		return g.english, nil
	case <-ctx.Done():
		return "", ctx.Err()
	}
}

func newSelection(t *testing.T, translator selection.Translator, options selection.Options) *teatest.TestModel {
	t.Helper()
	if options.Service == "" {
		options.Service = "deepl"
	}
	if options.Language == "" {
		options.Language = "EN-US"
	}
	return teatest.NewTestModel(t,
		selection.New(context.Background(), translator, options),
		teatest.WithInitialTermSize(87, 20))
}

func closeSelection(t *testing.T, model *teatest.TestModel) {
	t.Helper()
	model.Send(tea.KeyMsg{Type: tea.KeyCtrlC})
	model.WaitFinished(t, teatest.WithFinalTimeout(2*time.Second))
}

// waitFor reads the window until every wanted word has been on screen,
// answering with everything that was read — which is what a test then checks
// what is not there against.
func waitFor(t *testing.T, model *teatest.TestModel, wanted ...string) []byte {
	t.Helper()
	var shown bytes.Buffer
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		chunk, err := io.ReadAll(model.Output())
		if err != nil {
			t.Fatalf("reading output: %v", err)
		}
		shown.Write(chunk)
		if containsAll(shown.Bytes(), wanted...) {
			return shown.Bytes()
		}
		time.Sleep(50 * time.Millisecond)
	}
	t.Fatalf("the window never showed %q; it said:\n%s", wanted, shown.String())
	return nil
}

func containsAll(shown []byte, wanted ...string) bool {
	for _, word := range wanted {
		if !bytes.Contains(shown, []byte(word)) {
			return false
		}
	}
	return true
}

// The window exists for this: what was selected beside what it means, with the
// service that did it in the header — and it asks on the way in, so nobody
// waits for a press that will not come.
func TestTheSelectionAndItsTranslationAreShownTogether(t *testing.T) {
	t.Parallel()
	translator := &stubTranslator{english: english}

	model := newSelection(t, translator, selection.Options{Source: selected})
	waitFor(t, model, selected, english, "deepl", "EN-US")

	if translator.count() == 0 {
		t.Error("the window was opened without the translation being asked for")
	}
	closeSelection(t, model)
}

func TestEscapeClosesTheWindow(t *testing.T) {
	t.Parallel()

	model := newSelection(t, &stubTranslator{english: english}, selection.Options{Source: selected})
	waitFor(t, model, english)

	model.Send(tea.KeyMsg{Type: tea.KeyEsc})
	model.WaitFinished(t, teatest.WithFinalTimeout(2*time.Second))
}

func TestCtrlCAlsoClosesTheWindow(t *testing.T) {
	t.Parallel()

	model := newSelection(t, &stubTranslator{english: english}, selection.Options{Source: selected})
	waitFor(t, model, selected)

	closeSelection(t, model)
}

// A service takes its time, and a window that says nothing while waiting looks
// broken — so the wait is said until the answer arrives.
func TestWhileTheTranslationIsOnItsWayItSaysSo(t *testing.T) {
	t.Parallel()
	translator := &gatedTranslator{
		english: english,
		started: make(chan struct{}),
		release: make(chan struct{}),
	}

	model := newSelection(t, translator, selection.Options{Source: selected})
	select {
	case <-translator.started:
	case <-time.After(2 * time.Second):
		t.Fatal("the translation never started")
	}
	waitFor(t, model, "translating")

	close(translator.release)
	waitFor(t, model, english)
	closeSelection(t, model)
}

// A translation that did not come is worth saying out loud and worth another
// try: the chord is pressed once more without reopening the window.
func TestAFailedTranslationIsSaidOutLoudAndCanBeTriedAgain(t *testing.T) {
	t.Parallel()

	model := newSelection(t, &flakyTranslator{english: english}, selection.Options{Source: selected})
	waitFor(t, model, "deepl unreachable", "ctrl+t")

	model.Send(tea.KeyMsg{Type: tea.KeyCtrlT})
	waitFor(t, model, english)
	closeSelection(t, model)
}

// Without a service there is nothing to ask, but the selection was free to
// read: it stays on screen with what to do about the rest.
func TestWithoutAServiceTheSelectionIsStillShown(t *testing.T) {
	t.Parallel()
	translatorThatMustNotRun := &stubTranslator{english: english}

	model := newSelection(t, translatorThatMustNotRun,
		selection.Options{Source: selected, WithoutService: true})
	waitFor(t, model, selected, "no translation service")
	time.Sleep(200 * time.Millisecond)

	if calls := translatorThatMustNotRun.count(); calls != 0 {
		t.Errorf("the translator was called %d times, want none without a service", calls)
	}
	closeSelection(t, model)
}

// Nothing on the clipboard is not an error, but it needs saying — and which
// side of the chord the window was opened from changes what the advice is.
func TestASelectionThatNeverCameSaysWhyAndIsNotTranslated(t *testing.T) {
	t.Parallel()

	t.Run("with a copy chord the pane did not answer", func(t *testing.T) {
		translatorThatMustNotRun := &stubTranslator{english: english}

		model := newSelection(t, translatorThatMustNotRun,
			selection.Options{Source: "", SelectCopy: "ctrl+shift+c"})
		waitFor(t, model, "nothing was selected", "put nothing back", "nothing to translate")
		time.Sleep(200 * time.Millisecond)

		if calls := translatorThatMustNotRun.count(); calls != 0 {
			t.Errorf("the translator was called %d times, want none without a selection", calls)
		}
		closeSelection(t, model)
	})

	t.Run("without one the clipboard advice is given instead", func(t *testing.T) {
		translatorThatMustNotRun := &stubTranslator{english: english}

		model := newSelection(t, translatorThatMustNotRun, selection.Options{Source: ""})
		waitFor(t, model, "nothing was selected", "TRANS_SELECT_COPY")
		closeSelection(t, model)
	})
}

// The translation is the text being read here, so a long one gets a bar down
// its side rather than being cut off without a word about it.
func TestATranslationLongerThanTheWindowCanBeReadByScrolling(t *testing.T) {
	t.Parallel()
	long := strings.Repeat("Das ist ein sehr langer Satz, weitergeschrieben. ", 30)

	model := newSelection(t, &stubTranslator{english: long}, selection.Options{Source: selected})
	waitFor(t, model, "sehr langer Satz", frame.ScrollThumb, "read")

	model.Send(tea.KeyMsg{Type: tea.KeyDown})
	model.Send(tea.KeyMsg{Type: tea.KeyDown})
	closeSelection(t, model)
}

func TestAShortTranslationNeedsNoBar(t *testing.T) {
	t.Parallel()

	model := newSelection(t, &stubTranslator{english: english}, selection.Options{Source: selected})
	shown := waitFor(t, model, english)

	if bytes.Contains(shown, []byte(frame.ScrollThumb)) {
		t.Error("a translation that fits has a scroll bar drawn beside it")
	}
	if bytes.Contains(shown, []byte("↑↓")) {
		t.Error("a translation that fits offers the keys to read it")
	}
	closeSelection(t, model)
}
