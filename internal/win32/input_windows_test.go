//go:build windows

package win32

import (
	"context"
	"testing"
	"time"
	"unsafe"
)

// The size of the structure SendInput takes is part of its contract: Windows
// reads it as written, and one that is a byte short makes every event land in
// the wrong place. A keyboard event is the smallest thing this package sends,
// and the structure is sized for a mouse event, which is the largest member of
// the union on every architecture Windows runs on.
func TestTheInputStructureIsTheSizeWindowsReads(t *testing.T) {
	t.Parallel()

	size := unsafe.Sizeof(input{})
	if size%unsafe.Alignof(uintptr(0)) != 0 {
		t.Errorf("the input structure is %d bytes, which is not aligned for a handle", size)
	}
	if size < unsafe.Sizeof(mouseInput{})+4 {
		t.Errorf("the input structure is %d bytes, too small to hold a mouse event", size)
	}
}

// WaitForForeground gives up on a window that cannot come forward, and does it
// within its bound rather than looping. A handle that is not a window at all is
// the one thing a session without a desktop can still be asked about.
func TestWaitForForegroundGivesUpOnAWindowThatIsNotThere(t *testing.T) {
	t.Parallel()

	started := time.Now()
	if WaitForForeground(context.Background(), ^uintptr(0)) {
		t.Fatal("WaitForForeground answered that a window that is not there is in front")
	}
	if waited := time.Since(started); waited > 5*time.Second {
		t.Errorf("WaitForForeground waited %s, want a bounded wait", waited)
	}
}

// A cancelled context is not a wait: the panel cancels the work a chord started
// when the author presses escape, and the return key that would otherwise
// follow must not be sent.
func TestWaitForForegroundStopsOnACancelledContext(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	started := time.Now()
	if WaitForForeground(ctx, ^uintptr(0)) {
		t.Fatal("WaitForForeground answered that a window it never saw is in front")
	}
	if waited := time.Since(started); waited > time.Second {
		t.Errorf("WaitForForeground waited %s after cancellation, want an immediate answer", waited)
	}
}
