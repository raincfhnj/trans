package overlay_test

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"

	"trans/internal/history"
	"trans/internal/overlay"
	"trans/internal/promptflow"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/exp/teatest"
	"github.com/muesli/termenv"
)

// fakeHistory is the record of sent prompts, newest entry first like the real
// store answers, with the errors the test wants to happen on demand.
type fakeHistory struct {
	mu        sync.Mutex
	entries   []history.Entry
	forgetErr error
}

func (f *fakeHistory) Record(source, translation string, how promptflow.Delivery) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.entries = append([]history.Entry{{
		At:          time.Now(),
		Source:      source,
		Translation: translation,
		Delivery:    how.String(),
	}}, f.entries...)
	return nil
}

func (f *fakeHistory) Entries() ([]history.Entry, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]history.Entry(nil), f.entries...), nil
}

func (f *fakeHistory) Forget(wanted *history.Entry) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.forgetErr != nil {
		return f.forgetErr
	}
	left := make([]history.Entry, 0, len(f.entries))
	for _, entry := range f.entries {
		if entry.Source != wanted.Source || !entry.At.Equal(wanted.At) {
			left = append(left, entry)
		}
	}
	f.entries = left
	return nil
}

func (f *fakeHistory) kept() []history.Entry {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]history.Entry(nil), f.entries...)
}

// seededHistory is a record with prompts already in it, newest first.
func seededHistory(prompts ...string) *fakeHistory {
	record := &fakeHistory{}
	for _, prompt := range prompts {
		record.entries = append(record.entries, history.Entry{
			At:     time.Now(),
			Source: prompt,
		})
	}
	return record
}

func newHistoryOverlay(t *testing.T, record overlay.History) *teatest.TestModel {
	t.Helper()
	return newOverlayWith(t, stubTranslator{english: english}, &recordingTarget{},
		overlay.Options{
			Service:  "deepl",
			Language: "EN-US",
			Vim:      true,
			History:  record,
		})
}

// The footer only has a key for a record that exists.
func newOverlayWithoutHistory(t *testing.T) *teatest.TestModel {
	t.Helper()
	return newOverlayWith(t, stubTranslator{english: english}, &recordingTarget{},
		overlay.Options{Service: "deepl", Language: "EN-US", Vim: true})
}

// The tail of what was drawn last: one frame is far longer than this, so what
// is in it belongs to the frame the test just waited for.
func recent(out []byte) []byte {
	return out[max(0, len(out)-700):]
}

func TestAPromptThatWasSentIsWrittenToTheRecord(t *testing.T) {
	t.Parallel()
	record := &fakeHistory{}
	overlayUnderTest := newHistoryOverlay(t, record)

	overlayUnderTest.Type(draft)
	overlayUnderTest.Send(tea.KeyMsg{Type: tea.KeyCtrlD})
	waitForTheNextPrompt(t, overlayUnderTest)

	kept := record.kept()
	if len(kept) != 1 {
		t.Fatalf("the record holds %d prompts, want the one that was just sent", len(kept))
	}
	if kept[0].Source != draft {
		t.Errorf("the record holds %q, want the prompt as it was written", kept[0].Source)
	}
	if kept[0].Translation != english {
		t.Errorf("the translation recorded is %q, want what the agent received", kept[0].Translation)
	}
	if kept[0].Delivery != "sending" {
		t.Errorf("the delivery recorded is %q, want \"sending\"", kept[0].Delivery)
	}

	closeTheOverlay(t, overlayUnderTest)
}

func TestCtrlGOpensTheRecordOfPromptsThatWereSent(t *testing.T) {
	t.Parallel()
	record := seededHistory("zweiter prompt", "erster prompt")
	overlayUnderTest := newHistoryOverlay(t, record)

	overlayUnderTest.Send(tea.KeyMsg{Type: tea.KeyCtrlG})
	// The newest prompt is the one worth seeing first.
	teatest.WaitFor(t, overlayUnderTest.Output(), func(out []byte) bool {
		newest := bytes.Index(out, []byte("zweiter prompt"))
		older := bytes.Index(out, []byte("erster prompt"))
		return newest >= 0 && older >= 0 && newest < older &&
			bytes.Contains(out, []byte("use it"))
	}, teatest.WithDuration(2*time.Second))

	closeTheOverlay(t, overlayUnderTest)
}

func TestTheRecordSaysSoWhenNothingHasGoneOutYet(t *testing.T) {
	t.Parallel()
	overlayUnderTest := newHistoryOverlay(t, &fakeHistory{})

	overlayUnderTest.Send(tea.KeyMsg{Type: tea.KeyCtrlG})
	teatest.WaitFor(t, overlayUnderTest.Output(), func(out []byte) bool {
		return bytes.Contains(recent(out), []byte("nothing sent yet"))
	}, teatest.WithDuration(2*time.Second))

	closeTheOverlay(t, overlayUnderTest)
}

func TestTheRecordKeySaysWhenNothingIsKept(t *testing.T) {
	t.Parallel()
	overlayUnderTest := newOverlayWithoutHistory(t)

	overlayUnderTest.Send(tea.KeyMsg{Type: tea.KeyCtrlG})
	teatest.WaitFor(t, overlayUnderTest.Output(), func(out []byte) bool {
		return bytes.Contains(recent(out), []byte("TRANS_HISTORY"))
	}, teatest.WithDuration(2*time.Second))

	closeTheOverlay(t, overlayUnderTest)
}

func TestEnterTakesAPromptBackOutOfTheRecord(t *testing.T) {
	t.Parallel()
	record := seededHistory("Alter Satz aus dem Protokoll")
	overlayUnderTest := newHistoryOverlay(t, record)

	overlayUnderTest.Send(tea.KeyMsg{Type: tea.KeyCtrlG})
	teatest.WaitFor(t, overlayUnderTest.Output(), func(out []byte) bool {
		return bytes.Contains(out, []byte("Alter Satz"))
	}, teatest.WithDuration(2*time.Second))

	overlayUnderTest.Send(tea.KeyMsg{Type: tea.KeyEnter})
	teatest.WaitFor(t, overlayUnderTest.Output(), func(out []byte) bool {
		// The prompt is in the box instead of the record: the footer is the
		// draft's own again, and the placeholder is covered by the writing.
		return bytes.Contains(recent(out), []byte("Alter Satz")) &&
			!bytes.Contains(recent(out), []byte("Write your prompt")) &&
			!bytes.Contains(recent(out), []byte("use it"))
	}, teatest.WithDuration(2*time.Second))

	// The prompt is in the box, and writing goes to the box again.
	overlayUnderTest.Type("!")
	teatest.WaitFor(t, overlayUnderTest.Output(), func(out []byte) bool {
		return bytes.Contains(recent(out), []byte("!Alter Satz"))
	}, teatest.WithDuration(2*time.Second))

	closeTheOverlay(t, overlayUnderTest)
}

func TestEscapeLeavesTheRecordWithoutTouchingTheDraft(t *testing.T) {
	t.Parallel()
	record := seededHistory("Ein Prompt aus dem Protokoll")
	overlayUnderTest := newHistoryOverlay(t, record)

	overlayUnderTest.Send(tea.KeyMsg{Type: tea.KeyCtrlG})
	teatest.WaitFor(t, overlayUnderTest.Output(), func(out []byte) bool {
		return bytes.Contains(out, []byte("Ein Prompt"))
	}, teatest.WithDuration(2*time.Second))

	overlayUnderTest.Send(tea.KeyMsg{Type: tea.KeyEsc})
	overlayUnderTest.Type("hallo")
	teatest.WaitFor(t, overlayUnderTest.Output(), func(out []byte) bool {
		return bytes.Contains(recent(out), []byte("hallo")) &&
			!bytes.Contains(recent(out), []byte("Ein Prompt"))
	}, teatest.WithDuration(2*time.Second))

	closeTheOverlay(t, overlayUnderTest)
}

func TestDeleteDropsAPromptOutOfTheRecord(t *testing.T) {
	t.Parallel()
	record := seededHistory("Nur ein Test", "Ein Prompt aus dem Protokoll")
	overlayUnderTest := newHistoryOverlay(t, record)

	overlayUnderTest.Send(tea.KeyMsg{Type: tea.KeyCtrlG})
	teatest.WaitFor(t, overlayUnderTest.Output(), func(out []byte) bool {
		return bytes.Contains(out, []byte("Nur ein Test"))
	}, teatest.WithDuration(2*time.Second))

	overlayUnderTest.Send(tea.KeyMsg{Type: tea.KeyDelete})
	teatest.WaitFor(t, overlayUnderTest.Output(), func(out []byte) bool {
		return bytes.Contains(recent(out), []byte("Ein Prompt aus dem Protokoll")) &&
			!bytes.Contains(recent(out), []byte("Nur ein Test"))
	}, teatest.WithDuration(2*time.Second))

	kept := record.kept()
	if len(kept) != 1 || kept[0].Source != "Ein Prompt aus dem Protokoll" {
		t.Errorf("the record holds %d prompts, want the one that was not deleted", len(kept))
	}

	// One prompt left is still a record worth looking at; dropping it closes
	// the view, because an empty record is nothing to look at.
	overlayUnderTest.Send(tea.KeyMsg{Type: tea.KeyDelete})
	teatest.WaitFor(t, overlayUnderTest.Output(), func(out []byte) bool {
		return !bytes.Contains(recent(out), []byte("use it")) &&
			bytes.Contains(recent(out), []byte("Write your prompt"))
	}, teatest.WithDuration(2*time.Second))

	if len(record.kept()) != 0 {
		t.Errorf("the record still holds prompts, want it emptied")
	}

	closeTheOverlay(t, overlayUnderTest)
}

func TestADeadRecordSaysSoInsteadOfOpening(t *testing.T) {
	t.Parallel()
	record := seededHistory("Ein Prompt aus dem Protokoll")
	record.forgetErr = errors.New("the record is stuck")
	overlayUnderTest := newHistoryOverlay(t, record)

	overlayUnderTest.Send(tea.KeyMsg{Type: tea.KeyCtrlG})
	overlayUnderTest.Send(tea.KeyMsg{Type: tea.KeyDelete})
	teatest.WaitFor(t, overlayUnderTest.Output(), func(out []byte) bool {
		return bytes.Contains(recent(out), []byte("the record is stuck"))
	}, teatest.WithDuration(2*time.Second))

	if len(record.kept()) != 1 {
		t.Errorf("the record holds %d prompts, want the entry left in place", len(record.kept()))
	}

	closeTheOverlay(t, overlayUnderTest)
}

func TestTheRecordScrollsOneLineAtATimeAndBack(t *testing.T) {
	model := recordModel(t, seededHistory(numberedPrompts(20)...))
	model, _ = model.Update(tea.KeyMsg{Type: tea.KeyCtrlG})
	top := model.View()
	if !strings.Contains(top, "prompt 19") {
		t.Fatalf("the record does not show its newest prompt at the top:\n%s", top)
	}

	// Eleven rows fit; the twelfth prompt only comes by scrolling, one line at
	// a time, and the top one then leaves the view.
	for range 16 {
		model, _ = model.Update(tea.KeyMsg{Type: tea.KeyDown})
	}
	scrolled := model.View()
	if !strings.Contains(scrolled, "prompt 03") {
		t.Errorf("the arrows do not scroll the record:\n%s", scrolled)
	}
	if strings.Contains(scrolled, "prompt 19") {
		t.Errorf("the record scrolls with the top prompt still on screen:\n%s", scrolled)
	}

	// The letters do the same, for the hand that is already in the vim keys.
	for range 16 {
		model, _ = model.Update(tea.KeyMsg{Type: tea.KeyUp})
	}
	if back := model.View(); back != top {
		t.Errorf("scrolling back up does not return to the beginning:\n%s", back)
	}
	moved, _ := model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("j")})
	if same := moved.View(); same == top {
		t.Error("the j key does not move the selection")
	}
	model, _ = moved.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("k")})
	if back := model.View(); back != top {
		t.Error("the k key does not move the selection back")
	}
}

func TestTheRecordJumpsWithItsPageAndEndKeys(t *testing.T) {
	model := recordModel(t, seededHistory(numberedPrompts(20)...))
	model, _ = model.Update(tea.KeyMsg{Type: tea.KeyCtrlG})
	top := model.View()

	model, _ = model.Update(tea.KeyMsg{Type: tea.KeyEnd})
	atEnd := model.View()
	if !strings.Contains(atEnd, "prompt 00") {
		t.Errorf("the end key does not reach the oldest prompt:\n%s", atEnd)
	}

	model, _ = model.Update(tea.KeyMsg{Type: tea.KeyHome})
	if back := model.View(); back != top {
		t.Error("the home key does not return to the newest prompt")
	}

	// A page is as many lines as fit: the bottom of one is the top of the next.
	model, _ = model.Update(tea.KeyMsg{Type: tea.KeyPgDown})
	paged := model.View()
	if !strings.Contains(paged, "prompt 08") {
		t.Errorf("page down does not move a page:\n%s", paged)
	}
	if strings.Contains(paged, "prompt 19") {
		t.Errorf("page down keeps the first page on screen:\n%s", paged)
	}

	model, _ = model.Update(tea.KeyMsg{Type: tea.KeyPgUp})
	if back := model.View(); back != top {
		t.Error("page up does not come back to the top")
	}
}

// numberedPrompts is twenty prompts, newest first like the record answers.
func numberedPrompts(count int) []string {
	prompts := make([]string, 0, count)
	for index := count - 1; index >= 0; index-- {
		prompts = append(prompts, fmt.Sprintf("prompt %02d", index))
	}
	return prompts
}

// recordModel is the panel with a record behind ctrl+g, sized like the popup
// it is drawn into and ready for keys to be sent straight to it.
func recordModel(t *testing.T, record overlay.History) tea.Model {
	t.Helper()
	previous := lipgloss.ColorProfile()
	lipgloss.SetColorProfile(termenv.ANSI)
	t.Cleanup(func() { lipgloss.SetColorProfile(previous) })

	flow := promptflow.New(stubTranslator{english: english},
		&recordingTarget{}, &recordingTarget{})
	var model tea.Model = overlay.New(context.Background(), flow, overlay.Options{
		Service: "deepl", Language: "EN-US", History: record,
	})
	model, _ = model.Update(tea.WindowSizeMsg{Width: 80, Height: 20})
	return model
}
