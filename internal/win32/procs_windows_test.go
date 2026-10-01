//go:build windows

package win32

import (
	"testing"
	"unsafe"
)

// The size of the structure GetGUIThreadInfo fills in is part of its contract:
// Windows is told the size before it writes, and a size that is too small makes
// the call fail — which reads here as "the queue could not be asked about" and
// quietly turns the delivery's one real wait into its fallback pause. Pinning
// the size turns a wrong field into a failing test instead.
func TestTheInputStateStructureIsTheSizeWindowsWrites(t *testing.T) {
	t.Parallel()

	size := unsafe.Sizeof(guiThreadInfo{})
	// Two uint32 fields, one pointer for each of the five window fields, two
	// rectangles of four int32, and the caret's place as two int32, with the
	// padding a pointer-sized field needs in front of the caret's.
	want := 2*4 + 5*unsafe.Sizeof(uintptr(0)) + 2*4*4 + 2*4
	if size != want {
		t.Errorf("the input state structure is %d bytes, want %d: the fields and Windows must agree", size, want)
	}
}
