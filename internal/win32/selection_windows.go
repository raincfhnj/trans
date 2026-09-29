//go:build windows

package win32

import (
	"fmt"
	"os"
	"strings"
	"time"
)

// How long the pane is given to write the clipboard after the chord is out.
const copySettle = 200 * time.Millisecond

// SelectedText answers the text the author has selected in the pane in front —
// what the selection's chord sends to the panel for translating.
//
// With no chord the clipboard is read as it stands: a terminal that copies on
// selection has already put the selection there. With a chord the clipboard is
// saved, marked, and copied into by the pane — the mark standing afterwards is
// what says nothing was selected, where a plain before-and-after could not tell
// a copy that did not happen from one that brought back the very text that was
// on the clipboard.
func SelectedText(chord string) (string, error) {
	if strings.TrimSpace(chord) == "" {
		return ClipboardText(), nil
	}

	before := ClipboardText()
	mark := fmt.Sprintf("trans-selection-%d-%d", os.Getpid(), time.Now().UnixNano())
	if err := SetClipboardText(mark); err != nil {
		return "", fmt.Errorf("marking the clipboard for the selection: %w", err)
	}
	if err := Chord(chord); err != nil {
		// The chord never went out, so neither may the mark it was sent with.
		_ = SetClipboardText(before)
		return "", fmt.Errorf("pressing %s to copy the selection: %w", chord, err)
	}
	// The pane writes the clipboard once the keys are out of its way.
	time.Sleep(copySettle)

	selection, putBack := captured(mark, before, ClipboardText())
	if putBack {
		if err := SetClipboardText(before); err != nil {
			return "", fmt.Errorf("putting the clipboard back: %w", err)
		}
	}
	return selection, nil
}

// captured takes what the clipboard holds after the copy chord: the mark still
// standing means the pane had nothing selected. It answers the text and
// whether the clipboard goes back as it was — always when the mark is still
// there, so the mark never stays behind, and otherwise only when there was
// something to put back: what reads the clipboard reads text alone, so an
// empty reading may well be an image, which writing text over would lose.
func captured(mark, before, after string) (selection string, putBack bool) {
	if after == mark || after == "" {
		return "", true
	}
	return after, before != ""
}
