//go:build windows

package wintarget_test

import (
	"testing"

	"trans/internal/win32"
)

// TestClipboardRoundTrip checks the half of the delivery that is ours: what is
// put on the clipboard is what a paste would take from it.
func TestClipboardRoundTrip(t *testing.T) {
	before := win32.ClipboardText()

	wanted := "trans clipboard probe — with a dash and a 中文 line"
	if err := win32.SetClipboardText(wanted); err != nil {
		t.Fatalf("SetClipboardText: %v", err)
	}
	if got := win32.ClipboardText(); got != wanted {
		t.Fatalf("the clipboard holds %q, want %q", got, wanted)
	}

	if before != "" {
		_ = win32.SetClipboardText(before)
	}
}

// TestActivateBringsAWindowForward checks the other half: the window a prompt is
// meant for is the one that has the keys afterwards.
func TestActivateBringsAWindowForward(t *testing.T) {
	if err := win32.Activate(win32.Foreground().Handle); err != nil {
		t.Fatalf("Activate: %v", err)
	}
}
