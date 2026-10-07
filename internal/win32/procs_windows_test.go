//go:build windows

package win32

import (
	"testing"
	"unsafe"
)

// The size of the structure GetGUIThreadInfo fills in is part of its contract:
// Windows is told the size before it writes, and a size it does not recognise
// — too small or too large — makes the call fail with ERROR_INVALID_PARAMETER,
// which reads here as "the queue could not be asked about" and quietly turns
// the delivery's one real wait into its fallback pause. Pinning the size turns
// a wrong field into a failing test instead.
//
// The numbers are Windows's own: the size and the flags, six window handles,
// and the rectangle the caret is drawn in. The shape this pins was written the
// other way round once — five handles and two rectangles for the caret, 88
// bytes where Windows reads 72 — and the test then said 88 was right, which is
// the failure this is written against.
func TestTheInputStateStructureIsTheSizeWindowsWrites(t *testing.T) {
	t.Parallel()

	pointer := unsafe.Sizeof(uintptr(0))
	want := uintptr(2*4) + 6*pointer + 4*4

	if size := unsafe.Sizeof(guiThreadInfo{}); size != want {
		t.Errorf("the input state structure is %d bytes, Windows reads %d", size, want)
	}
	if offset := unsafe.Offsetof(guiThreadInfo{}.Flags); offset != 4 {
		t.Errorf("the flags begin at byte %d, Windows reads them at byte 4", offset)
	}
	if offset := unsafe.Offsetof(guiThreadInfo{}.Active); offset != 8 {
		t.Errorf("the window handles begin at byte %d, Windows reads them at byte 8", offset)
	}
}

// Windows is asked with the structure rather than only measured against it: a
// size the call refuses is the one thing the layout above cannot show, since
// what Windows makes of the bytes is its own. This is the call itself, made
// with the structure this package builds, over the thread of the window in
// front.
//
// A session with no interactive desktop has no window in front to ask about,
// and that is the one thing this skips — the capability the test needs rather
// than a failure of the code, which is how the clipboard tests skip too.
func TestWindowsAnswersWhatTheInputStateStructureAsks(t *testing.T) {
	handle := Foreground().Handle
	if handle == 0 {
		t.Skip("no window in front: this session has no interactive desktop")
	}

	if _, known := inputPending(handle); !known {
		t.Error("GetGUIThreadInfo would not answer about the window in front: " +
			"the structure this package asks with is not the one Windows reads")
	}
}
