//go:build windows

package win32

import "testing"

const mark = "trans-selection-1234-5678"

// The mark is what tells a copy that did not happen from one that did: a plain
// before-and-after cannot, when the selection is exactly what was on the
// clipboard already.
func TestAMarkStillStandingMeansNothingWasSelected(t *testing.T) {
	t.Parallel()

	selection, putBack := captured(mark, "the same text", mark)
	if selection != "" {
		t.Errorf("captured %q, want no selection when the mark came back", selection)
	}
	if !putBack {
		t.Error("putBack is false, want the mark taken off the clipboard")
	}
}

func TestTheSelectionIsReadAndTheClipboardGoesBackAsItWas(t *testing.T) {
	t.Parallel()

	selection, putBack := captured(mark, "what was copied before", "what is selected now")
	if selection != "what is selected now" {
		t.Errorf("captured %q, want the text the pane copied", selection)
	}
	if !putBack {
		t.Error("putBack is false, want the earlier clipboard restored")
	}
}

// Selection and clipboard alike is exactly the case the mark exists for: the
// text came from the pane, so it is a selection even though it matches.
func TestASelectionJustLikeTheClipboardIsStillASelection(t *testing.T) {
	t.Parallel()

	selection, _ := captured(mark, "the same text", "the same text")
	if selection != "the same text" {
		t.Errorf("captured %q, want the text the pane copied", selection)
	}
}

// An empty reading of the clipboard may be no text at all — an image, a file —
// and writing text over it to "restore" nothing would only lose it.
func TestAnEmptyClipboardIsNotWrittenOverAfterACopy(t *testing.T) {
	t.Parallel()

	selection, putBack := captured(mark, "", "what is selected now")
	if selection != "what is selected now" {
		t.Errorf("captured %q, want the text the pane copied", selection)
	}
	if putBack {
		t.Error("putBack is true, want nothing written over an empty clipboard")
	}
}
