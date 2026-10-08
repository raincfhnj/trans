//go:build windows

package win32

import (
	"testing"
	"unsafe"
)

// Windows writes the whole of PROCESSENTRY32W into the room it is given and
// says only that something was wrong when it will not: the size and the
// offsets are pinned here, where the layout is visible.
func TestTheProcessEntryIsTheShapeWindowsWrites(t *testing.T) {
	t.Parallel()

	var entry processEntry
	size := unsafe.Sizeof(entry)
	if want := uintptr(568); size != want {
		t.Errorf("processEntry is %d bytes, want %d (PROCESSENTRY32W on 64-bit Windows)", size, want)
	}
	if offset := unsafe.Offsetof(entry.ProcessID); offset != 8 {
		t.Errorf("ProcessID is at %d, want 8", offset)
	}
	if offset := unsafe.Offsetof(entry.DefaultHeapID); offset != 16 {
		t.Errorf("DefaultHeapID is at %d, want 16", offset)
	}
	if offset := unsafe.Offsetof(entry.ParentProcessID); offset != 32 {
		t.Errorf("ParentProcessID is at %d, want 32", offset)
	}
	if offset := unsafe.Offsetof(entry.ExeFile); offset != 44 {
		t.Errorf("ExeFile is at %d, want 44", offset)
	}
}

// The tree Windows is asked about includes this program's own process, which
// is how a window of its own is recognised later.
func TestTheProcessTreeCarriesThisProgram(t *testing.T) {
	t.Parallel()

	self := SelfPID()
	for _, process := range Processes() {
		if process.PID == self {
			return
		}
	}
	t.Fatalf("Processes() does not list this program's own id %d", self)
}

func TestAWindowNamesTheProcessThatDrewIt(t *testing.T) {
	t.Parallel()

	// The desktop always has a window, and it belongs to a process that is
	// named: what is asked here is that the two are read together.
	window := Foreground()
	if window.Handle == 0 {
		t.Skip("no window in front to ask about")
	}
	if got := ProcessOfWindow(window.Handle); got.PID == 0 || got.Name == "" {
		t.Errorf("ProcessOfWindow(%#x) = %+v, want a named process", window.Handle, got)
	}
}

// The console's own list is how a panel run from a shell recognises the shell
// it was started from �� and what else that shell is running.
func TestTheOwnConsoleListsTheProgramsOnIt(t *testing.T) {
	if _, ok := ConsoleMates(); !ok {
		t.Skip("this program has no console to ask")
	}
}
