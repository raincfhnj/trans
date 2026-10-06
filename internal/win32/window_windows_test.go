//go:build windows

package win32

import "testing"

// The pane a popup opens over is a window a person works in. Whatever holds
// the keyboard is not always one — the tray's own hidden window can, after its
// menu — and a panel opened over that ends up over a box in the corner of the
// screen, with the finished prompt pasted into it.
func TestFrontPaneAnswersAWindowAPanelCanSitOn(t *testing.T) {
	window := FrontPane()
	if window.Handle == 0 {
		t.Skip("nothing in front is a pane to open over")
	}
	if !opensOver(window.Handle) {
		t.Errorf("FrontPane answered %#x %q, which is not a pane to open over",
			window.Handle, window.Title)
	}
}

// A handle that is not a window at all is none of the windows a person could
// point at, so no panel belongs over it.
func TestNoHandleAtAllIsNotAPane(t *testing.T) {
	t.Parallel()

	if opensOver(0) {
		t.Error("no handle at all was taken for a pane")
	}
}

// The listing of windows a person could point at and the pane a popup opens
// over are one rule: a window one of them names is a window the other would
// open over.
func TestTheListingAndThePaneAgree(t *testing.T) {
	for _, window := range Windows() {
		if !opensOver(window.Handle) {
			t.Errorf("Windows listed %#x %q, which is not a pane to open over",
				window.Handle, window.Title)
		}
	}
}
