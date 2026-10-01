package overlay_test

import (
	"bytes"
	"errors"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/exp/teatest"

	"trans/internal/overlay"
)

// What a read-mode draft holds is written in English and comes back in the
// author's own language.
const readBack = "修复失败的测试"

// Read mode says what it is and which way it translates: the heading carries
// the word and the language the result comes back in, and the footer names the
// key as the copy it now is — with no ctrl+r, because there is no other way
// to choose.
func TestReadModeSaysReadAndNamesTheKeyAsCopying(t *testing.T) {
	t.Parallel()

	model := plainModel(t, overlay.Options{
		Service: "deepl", Language: "ZH", Read: true, SendKey: "ctrl+d",
	})
	shown := plain(model.View())

	for _, wanted := range []string{"read", "→ ZH", "copies to clipboard"} {
		if !strings.Contains(shown, wanted) {
			t.Errorf("the heading does not say %q:\n%s", wanted, shown)
		}
	}
	for _, absent := range []string{"sends to agent", "fills the input"} {
		if strings.Contains(shown, absent) {
			t.Errorf("read mode promises to deliver with %q:\n%s", absent, shown)
		}
	}

	footer := lastLine(shown)
	if !strings.Contains(footer, "ctrl+d copy") {
		t.Errorf("the footer does not name the key as a copy: %q", footer)
	}
	if strings.Contains(footer, "ctrl+r") {
		t.Errorf("the footer offers a delivery to switch to: %q", footer)
	}

	// The full-screen reading of the translation says the same thing in the
	// same place.
	reading, _ := model.Update(tea.KeyMsg{Type: tea.KeyTab})
	if footer := lastLine(plain(reading.View())); !strings.Contains(footer, "ctrl+d copy") {
		t.Errorf("while reading, the footer does not name the key as a copy: %q", footer)
	}
}

// A copy is a copy however often it is asked for: the panel stays open with the
// draft still there, escape is the way out even while the hint is up, and the
// result reached the target once — the clipboard in the panel, a stand-in here.
func TestReadModeCopiesTheResultAndClosesOnEscapeWithTheHintStillUp(t *testing.T) {
	t.Parallel()

	target := &recordingTarget{}
	overlayUnderTest := newOverlayWith(t, stubTranslator{english: readBack}, target,
		overlay.Options{
			Service: "deepl", Language: "ZH", Read: true, SendKey: "ctrl+d",
			Live: true, Debounce: 10 * time.Millisecond,
		})

	overlayUnderTest.Type(draft)
	teatest.WaitFor(t, overlayUnderTest.Output(), func(out []byte) bool {
		return bytes.Contains(out, []byte(readBack))
	}, teatest.WithDuration(frameTimeout))

	overlayUnderTest.Send(tea.KeyMsg{Type: tea.KeyCtrlD})
	teatest.WaitFor(t, overlayUnderTest.Output(), func(out []byte) bool {
		return bytes.Contains(out, []byte("copied to clipboard · esc closes"))
	}, teatest.WithDuration(frameTimeout))

	if len(target.sent()) != 1 || target.sent()[0] != readBack {
		t.Errorf("target received %v, want the result copied exactly once", target.sent())
	}

	// The panel is still there, with the draft still in it — copying does not
	// clear anything the way sending does.
	overlayUnderTest.Type("!")
	teatest.WaitFor(t, overlayUnderTest.Output(), func(out []byte) bool {
		return bytes.Contains(out, []byte("Test!"))
	}, teatest.WithDuration(frameTimeout))

	// Escape takes the whole panel away, hint or no hint: a read-mode panel
	// keeps nothing that escape could be needed to leave behind.
	overlayUnderTest.Send(tea.KeyMsg{Type: tea.KeyEsc})
	overlayUnderTest.WaitFinished(t, teatest.WithFinalTimeout(frameTimeout))
}

// A read-mode draft is text that arrived, not a prompt being written; it is
// never kept for a later session, and the panel quits without asking a store.
func TestReadModeKeepsNothingForTheNextSession(t *testing.T) {
	t.Parallel()

	drafts := &fakeDrafts{kept: "a draft from yesterday"}
	model, _ := composer(t, overlay.Options{
		Service: "deepl", Language: "ZH", Read: true, Drafts: drafts,
		Debounce: time.Millisecond,
	}, stubTranslator{english: readBack})

	if strings.Contains(plain(model.View()), "a draft from yesterday") {
		t.Error("read mode resumed a draft nobody was writing")
	}

	model, cmd := model.Update(tea.KeyMsg{Type: tea.KeyEsc})
	if cmd == nil {
		t.Fatal("escape in read mode asks no question, it closes")
	}
	if msg := cmd(); msg != (tea.QuitMsg{}) {
		t.Errorf("escape returned %v, want the panel to quit", msg)
	}

	if len(drafts.saved) != 0 || drafts.cleared != 0 {
		t.Errorf("read mode touched the draft store: saved %v, cleared %d",
			drafts.saved, drafts.cleared)
	}
	if err := model.(overlay.Model).KeepUnfinished(); err != nil {
		t.Errorf("keeping the finished session returned %v, want nothing kept", err)
	}
	if len(drafts.saved) != 0 {
		t.Errorf("the store was given %v after closing, want nothing kept", drafts.saved)
	}
}

// Text that arrived rather than being written is translated at once: the panel
// was opened to read it, not to watch an empty pane.
func TestReadModeTranslatesWhatItOpenedWithAtOnce(t *testing.T) {
	t.Parallel()

	overlayUnderTest := newOverlayWith(t, stubTranslator{english: readBack}, &recordingTarget{},
		overlay.Options{
			Service: "deepl", Language: "ZH", Read: true, SendKey: "ctrl+d",
			Live: true, Debounce: 10 * time.Millisecond, Prefill: draft,
		})

	teatest.WaitFor(t, overlayUnderTest.Output(), func(out []byte) bool {
		return bytes.Contains(out, []byte(draft)) && bytes.Contains(out, []byte(readBack))
	}, teatest.WithDuration(frameTimeout))

	overlayUnderTest.Send(tea.KeyMsg{Type: tea.KeyEsc})
	overlayUnderTest.WaitFinished(t, teatest.WithFinalTimeout(frameTimeout))
}

// A capture that found nothing — no selection, no clipboard — is the reason
// there is nothing to open with, and the panel says it as soon as it appears,
// the way it says any other trouble.
func TestWhenThereIsNothingToReadThePanelSaysWhyAtOnce(t *testing.T) {
	t.Parallel()

	trouble := errors.New("nothing captured — no text was selected, and the clipboard is empty")
	model := plainModel(t, overlay.Options{
		Service: "deepl", Language: "ZH", Read: true, PrefillTrouble: trouble,
	})

	model = driveOnce(model, model.Init())
	shown := plain(model.View())
	if !strings.Contains(shown, "nothing captured") {
		t.Errorf("the panel does not say why it opened empty:\n%s", shown)
	}
	if footer := lastLine(shown); !strings.Contains(footer, "esc close") {
		t.Errorf("the footer does not offer escape as the way out: %q", footer)
	}
}

// Nothing waits for an answer before the copy in read mode — asking would be a
// confirmation for a delivery that never happens, so the first press of the key
// is the copy itself, even with confirmation turned on for the panel normally.
func TestReadModeAsksForNothingBeforeItCopies(t *testing.T) {
	t.Parallel()

	model, target := composer(t, overlay.Options{
		Service: "deepl", Language: "ZH", Read: true, Confirm: true,
		Debounce: time.Millisecond, NoticeLinger: time.Millisecond,
	}, stubTranslator{english: readBack})

	model, cmd := model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("Bitte behebe den Test")})
	model = drive(model, cmd)
	model, cmd = model.Update(tea.KeyMsg{Type: tea.KeyCtrlD})
	model = drive(model, cmd)

	if len(target.sent()) != 1 || target.sent()[0] != readBack {
		t.Errorf("target received %v, want the result copied on the first key", target.sent())
	}
	// A confirmation would have shown the English first, waiting to be agreed
	// to. Read mode copies and the draft box is all there is.
	if strings.Contains(plain(model.View()), readBack) {
		t.Errorf("read mode waits for an answer before copying:\n%s", plain(model.View()))
	}
}
