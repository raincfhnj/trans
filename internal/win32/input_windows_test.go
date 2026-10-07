//go:build windows

package win32

import (
	"context"
	"testing"
	"time"
	"unsafe"
)

// The size of the structure SendInput takes is part of its contract: Windows
// reads it as written, and one that is wrong makes every event land in the
// wrong place or none of them arrive at all.
//
// The numbers are Windows's own rather than this package's: INPUT is a tag and
// then the largest of the three event structures laid over each other — a
// mouse event — which is 40 bytes in all on 64-bit Windows and 28 on 32-bit.
// SendInput answers ERROR_INVALID_PARAMETER for any other size and takes none
// of the events, which is a paste that never happens, and a test that only
// asked whether the structure was big enough passed while it was 72.
//
// The offsets are pinned for the reason the sizes do not cover on their own: a
// keyboard event that is wide enough but begins a word too late still reaches
// Windows, and what lands in the window is then whatever the fields after that
// word happen to hold. KEYBDINPUT's first field is the virtual key — the
// structure carries no tag of its own — and everything else is read from there.
func TestTheInputStructureIsTheSizeWindowsReads(t *testing.T) {
	t.Parallel()

	pointer := unsafe.Sizeof(uintptr(0))
	wantInput, wantKey, wantMouse, wantExtra := uintptr(28), uintptr(16), uintptr(24), uintptr(12)
	if pointer == 8 {
		wantInput, wantKey, wantMouse, wantExtra = 40, 24, 32, 16
	}

	sizes := []struct {
		what string
		got  uintptr
		want uintptr
	}{
		{"INPUT", unsafe.Sizeof(input{}), wantInput},
		{"the union inside INPUT", unsafe.Sizeof(inputUnion{}), wantMouse},
		{"KEYBDINPUT", unsafe.Sizeof(keyInput{}), wantKey},
		{"MOUSEINPUT", unsafe.Sizeof(mouseInput{}), wantMouse},
	}
	for _, size := range sizes {
		if size.got != size.want {
			t.Errorf("%s is %d bytes, Windows reads %d", size.what, size.got, size.want)
		}
	}

	offsets := []struct {
		what string
		got  uintptr
		want uintptr
	}{
		{"the union inside INPUT", unsafe.Offsetof(input{}.union), pointer},
		{"KEYBDINPUT's virtual key", unsafe.Offsetof(keyInput{}.key), 0},
		{"KEYBDINPUT's scan code", unsafe.Offsetof(keyInput{}.scan), 2},
		{"KEYBDINPUT's flags", unsafe.Offsetof(keyInput{}.flags), 4},
		{"KEYBDINPUT's time", unsafe.Offsetof(keyInput{}.Time), 8},
		{"KEYBDINPUT's extra information", unsafe.Offsetof(keyInput{}.Extra), wantExtra},
	}
	for _, offset := range offsets {
		if offset.got != offset.want {
			t.Errorf("%s begins at byte %d, Windows reads it at byte %d", offset.what, offset.got, offset.want)
		}
	}
}

// The sizes say what Windows expects; this is Windows taking it. The events
// release a key Windows has none of — virtual key 0 — so the check costs no
// keystroke in whatever holds the keyboard, while still proving the one thing
// the sizes cannot: SendInput reads the structure this package builds instead
// of answering ERROR_INVALID_PARAMETER and taking none of the events, which is
// a paste that never happens and no error anybody can act on.
//
// A session with no interactive desktop cannot inject anything at all, and
// that is the one thing this skips — the capability the test needs rather than
// a failure of the code, which is how the clipboard tests skip too.
func TestWindowsTakesWhatSendInputIsGiven(t *testing.T) {
	if Foreground().Handle == 0 {
		t.Skip("no window in front: this session has no interactive desktop")
	}

	// The shape a paste goes out in — the six events ctrl+shift+v is made of,
	// all at once — with the keys taken out of them: virtual key 0 is no key
	// at all, so nothing is pressed wherever the keyboard happens to be and
	// this check costs no keystroke in anything running beside it.
	events := []keyInput{keyUp(0), keyUp(0), keyUp(0), keyUp(0), keyUp(0), keyUp(0)}
	seen, err := sendKeys(events)
	if err != nil {
		t.Fatalf("sendKeys: %v", err)
	}
	if seen != len(events) {
		t.Errorf("windows took %d of %d key events", seen, len(events))
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
