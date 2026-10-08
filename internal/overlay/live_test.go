package overlay_test

import (
	"bytes"
	"strings"
	"testing"
	"time"

	"trans/internal/overlay"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/exp/teatest"
)

// Ordinary writing is not a paste, whatever the pace.
func TestWritingKeepsLiveTranslationOn(t *testing.T) {
	t.Parallel()
	translator := &countingTranslator{english: english}

	overlayUnderTest := newOverlayWith(t, translator, &recordingTarget{},
		overlay.Options{
			Service: "deepl", Language: "EN-US", Live: true,
			Debounce: 10 * time.Millisecond,
		})

	for word := range strings.FieldsSeq("Bitte behebe den fehlschlagenden Test im Formular") {
		overlayUnderTest.Send(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(word + " ")})
	}

	teatest.WaitFor(t, overlayUnderTest.Output(), func(out []byte) bool {
		return bytes.Contains(out, []byte(english))
	}, teatest.WithDuration(frameTimeout))

	overlayUnderTest.Send(tea.KeyMsg{Type: tea.KeyCtrlC})
	overlayUnderTest.WaitFinished(t, teatest.WithFinalTimeout(frameTimeout))
}

// Pasting a wall of text is one keystroke and a whole draft to pay for, so live
// translation steps aside and waits to be asked. The frame that says so is the
// signal the panel has decided; by the time it does, nothing was asked of the
// service.
func TestABigPasteIsNotTranslatedUntilItIsAskedFor(t *testing.T) {
	t.Parallel()
	translator := &countingTranslator{english: english}

	overlayUnderTest := newOverlayWith(t, translator, &recordingTarget{},
		overlay.Options{
			Service: "deepl", Language: "EN-US", Live: true,
			Debounce: 10 * time.Millisecond,
		})

	pasted := strings.Repeat("Fehlermeldung aus dem Log. ", 20)
	overlayUnderTest.Send(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(pasted), Paste: true})

	teatest.WaitFor(t, overlayUnderTest.Output(), func(out []byte) bool {
		return bytes.Contains(out, []byte("pasted"))
	}, teatest.WithDuration(frameTimeout))

	if calls := translator.count(); calls != 0 {
		t.Errorf("the service was asked %d times for text that was pasted, not written", calls)
	}

	// Asking for it is one key, and then it is translated.
	overlayUnderTest.Send(tea.KeyMsg{Type: tea.KeyCtrlL})
	teatest.WaitFor(t, overlayUnderTest.Output(), func(out []byte) bool {
		return bytes.Contains(out, []byte(english))
	}, teatest.WithDuration(frameTimeout))

	overlayUnderTest.Send(tea.KeyMsg{Type: tea.KeyCtrlC})
	overlayUnderTest.WaitFinished(t, teatest.WithFinalTimeout(frameTimeout))
}
