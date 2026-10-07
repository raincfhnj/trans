//go:build windows

package win32

import (
	"syscall"
	"unsafe"
)

// The Win32 calls that do not belong to the window, the console or the
// keyboard alone: the clipboard as a whole, the memory it hands out, and the
// input queue. They are kept together so that every lazy procedure this package
// imports can be seen at once.
var (
	procEnumClipboardFormats      = user32.NewProc("EnumClipboardFormats")
	procGetClipboardSequenceNumbr = user32.NewProc("GetClipboardSequenceNumber")
	procCopyImage                 = user32.NewProc("CopyImage")
	procDeleteObject              = gdi32.NewProc("DeleteObject")
	procSendInput                 = user32.NewProc("SendInput")
	procGetWindowThreadProcessID  = user32.NewProc("GetWindowThreadProcessId")
	procGetGUIThreadInfo          = user32.NewProc("GetGUIThreadInfo")
	// The two that hand a message on: a loop that waits for its own messages
	// is still the loop every window on that thread depends on, and anything
	// it keeps to itself never reaches them.
	procTranslateMessage = user32.NewProc("TranslateMessage")
	procDispatchMessageW = user32.NewProc("DispatchMessageW")

	procGlobalFree = kernel32.NewProc("GlobalFree")
)

// gdi32 is where the two calls that touch a picture live: copying one off the
// clipboard and deleting the copy this program made.
var gdi32 = syscall.NewLazyDLL("gdi32.dll")

// input is what SendInput takes: one input event, tagged with the kind it is
// and carrying that kind's own structure.
//
// The union is where Go and Windows part company. Windows's INPUT is a tag and
// then *one* of three event structures, laid over each other in the same place
// and sized by the largest of them — a mouse event: 32 bytes on 64-bit Windows
// and 24 on 32-bit, which is what makes INPUT 40 and 28. Go has no union, and
// a field for each of the three would be three structures end to end — 72
// bytes — which SendInput answers with ERROR_INVALID_PARAMETER and takes none
// of the events: a paste that never happens. So the room the largest member
// takes is written out beside the keyboard event, which is the only kind this
// package sends, and what is left of it is padding. Every size and offset here
// is pinned by a test, because Windows says only that the size was wrong and
// not what it wanted.
type input struct {
	kind  uint32
	union inputUnion
}

// inputUnion is that room: the keyboard event at the start of it, where the
// union begins, and the rest left as it is. The tag that says which kind of
// event this is is not part of it — Windows reads that from input.kind — and
// the rest of the room is for the mouse event this package never sends:
// SendInput measures the size of INPUT before it has read any of it, so the
// union has to be the largest member's width whatever actually goes out.
type inputUnion struct {
	keyboard keyInput
	_        [inputUnionBytes - unsafe.Sizeof(keyInput{})]byte
}

// inputUnionBytes is the largest structure Windows's union holds, which is the
// size of INPUT less its tag.
const inputUnionBytes = unsafe.Sizeof(mouseInput{})

// mouseInput is Windows's MOUSEINPUT. It is here to give the union its width
// and to be measured against the field offsets Windows reads; this package
// sends no mouse event.
type mouseInput struct {
	X         int32
	Y         int32
	MouseData uint32
	Flags     uint32
	Time      uint32
	Extra     uintptr
}

// keyInput is Windows's KEYBDINPUT. The first field is the virtual key: the
// structure carries no tag of its own, and everything Windows reads lines up
// from there — the scan code, the flags, the time, and what the caller
// attaches — so a word in front of the key would move every one of them.
type keyInput struct {
	key   uint16
	scan  uint16
	flags uint32
	Time  uint32
	Extra uintptr
}

// guiThreadInfo is what GetGUIThreadInfo fills in about a thread's input state.
// The shape is Windows's own — GUITHREADINFO in winuser.h: the size and the
// flags, six window handles, and the rectangle the caret is drawn in. The
// window with the caret is a handle like the rest, not a rectangle of its own,
// and a structure written the other way round is one Windows refuses.
//
// Every field is kept even though only the flags are read, because Windows
// writes the whole structure: a field left out would move the ones after it.
// The size is what Windows is told before it writes, and it is measured rather
// than assumed for the same reason as the input structure — a size Windows
// does not recognise makes the call fail with ERROR_INVALID_PARAMETER, which
// reads here as "the queue could not be asked about" and quietly turns the
// delivery's one real wait into its fallback pause. Both sizes and offsets are
// pinned by a test, which is what the 88-byte shape this replaces never had.
type guiThreadInfo struct {
	Size      uint32
	Flags     uint32
	Active    uintptr
	Focus     uintptr
	Capture   uintptr
	MenuOwner uintptr
	MoveSize  uintptr
	Caret     uintptr
	CaretRect rect
}

const (
	// inputKeyboard tags an event as a key press, and keyEventKeyUp as one that
	// releases a key rather than pressing it.
	inputKeyboard = 1
	keyEventKeyUp = 0x0002

	// guiInInput is the flag GetGUIThreadInfo sets while a thread's input queue
	// still holds something it has not dispatched.
	guiInInput = 0x0001
)
