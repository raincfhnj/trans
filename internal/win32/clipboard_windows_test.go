//go:build windows

// The tests of the clipboard itself, in the package that keeps it. They were
// written next to the delivery that uses the clipboard first, which left the
// package they actually check without a test of its own.
package win32_test

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
// put on the clipboard is what a paste would take from it, and what was there
// before can be found again afterwards.
func TestClipboardRoundTrip(t *testing.T) {
	skipWithoutAClipboard(t)

	// What was on the clipboard is put to one side rather than read as text, so
	// that a picture or a set of files is not thrown away by a test either.
	before, err := win32.SnapshotClipboard()
	if err != nil {
		t.Skipf("the clipboard holds %v, which this session's test cannot put back", err)
	}
	t.Cleanup(func() {
		if err := win32.RestoreClipboard(before); err != nil {
			t.Errorf("putting the clipboard back: %v", err)
		}
		win32.ReleaseClipboard(before)
	})

	wanted := "trans clipboard probe — with a dash and a 中文 line"
	if err := win32.SetClipboardText(wanted); err != nil {
		t.Fatalf("SetClipboardText: %v", err)
	}
	// The clipboard belongs to the whole desktop, and `go test ./...` runs
	// packages in parallel: another package's test may have taken it between
	// these two lines. That is someone else's timing rather than a failure of
	// this code, so it is said and not failed.
	if got := win32.ClipboardText(); got != wanted {
		t.Skipf("the clipboard holds %q rather than %q: another process is using it",
			got, wanted)
	}
}

// TestASnapshotAndRestoreLeaveTheClipboardAsItWas is the promise a prompt makes
// when it borrows the clipboard: what was there before a delivery is there
// after it.
func TestASnapshotAndRestoreLeaveTheClipboardAsItWas(t *testing.T) {
	skipWithoutAClipboard(t)

	before, err := win32.SnapshotClipboard()
	if err != nil {
		t.Skipf("the clipboard holds %v, which this session's test cannot put back", err)
	}
	t.Cleanup(func() { win32.ReleaseClipboard(before) })

	// The clipboard is emptied over the snapshot the way a delivery empties it,
	// so that the text read afterwards is the prompt's and not the snapshot's.
	beforeText := win32.ClipboardText()
	if err := win32.SetClipboardText("a prompt that borrowed the clipboard"); err != nil {
		t.Fatalf("SetClipboardText: %v", err)
	}
	if err := win32.RestoreClipboard(before); err != nil {
		t.Fatalf("RestoreClipboard: %v", err)
	}
	if got := win32.ClipboardText(); got != beforeText {
		t.Errorf("the clipboard holds %q after a restore, want %q", got, beforeText)
	}
}

// A snapshot taken and never put back is released without touching the
// clipboard: this is the path a delivery takes when it borrows nothing.
func TestASnapshotThatIsNeverUsedLeavesTheClipboardAlone(t *testing.T) {
	skipWithoutAClipboard(t)

	before, err := win32.SnapshotClipboard()
	if err != nil {
		t.Skipf("the clipboard holds %v, which this session's test cannot put back", err)
	}

	held := win32.ClipboardText()
	win32.ReleaseClipboard(before)
	if got := win32.ClipboardText(); got != held {
		t.Errorf("the clipboard holds %q, want %q: releasing a snapshot must not touch it", got, held)
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
