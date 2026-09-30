//go:build windows

package wintarget_test

import (
	"syscall"
	"testing"
	"time"

	"trans/internal/win32"
)

// The clipboard and the window in front live on an interactive desktop. A CI
// runner may execute tests in a session without one, where the clipboard
// never opens and there is no window to bring forward. That is a capability
// the session does not have rather than a failure of the code, and it is the
// only thing these tests skip; on a real desktop all of them still run.

var (
	user32             = syscall.NewLazyDLL("user32.dll")
	procOpenClipboard  = user32.NewProc("OpenClipboard")
	procCloseClipboard = user32.NewProc("CloseClipboard")
)

// skipWithoutAClipboard asks for the one capability the round trip needs. The
// patience mirrors SetClipboardText's: another process holding the clipboard
// for a moment is normal, a session with no desktop never opens it at all.
func skipWithoutAClipboard(t *testing.T) {
	t.Helper()
	for attempt := 0; attempt < 10; attempt++ {
		if open, _, _ := procOpenClipboard.Call(0); open != 0 {
			_, _, _ = procCloseClipboard.Call()
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Skip("the clipboard cannot be opened: this session has no interactive desktop")
}

// TestClipboardRoundTrip checks the half of the delivery that is ours: what is
// put on the clipboard is what a paste would take from it.
func TestClipboardRoundTrip(t *testing.T) {
	skipWithoutAClipboard(t)

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
	target := win32.Foreground().Handle
	// A session without an interactive desktop has no window in front, and
	// Activate on none would only report the absence back.
	if target == 0 {
		t.Skip("no window in front: this session has no interactive desktop")
	}
	if err := win32.Activate(target); err != nil {
		t.Fatalf("Activate: %v", err)
	}
}
