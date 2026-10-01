//go:build windows

package win32

import "syscall"

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

	procGlobalFree = kernel32.NewProc("GlobalFree")
)

// gdi32 is where the two calls that touch a picture live: copying one off the
// clipboard and deleting the copy this program made.
var gdi32 = syscall.NewLazyDLL("gdi32.dll")

// input is what SendInput takes: one input event, tagged with the kind it is
// and carrying that kind's own structure.
//
// The union is sized for a mouse event on purpose. A mouse event is the largest
// member of the union on every architecture Windows runs on, so this holds a
// keyboard event with room to spare and keeps the structure the size Windows
// expects; a test pins that size.
type input struct {
	kind  uint32
	union inputUnion
}

type inputUnion struct {
	mouse    mouseInput
	keyboard keyInput
	hardware hardwareInput
}

type mouseInput struct {
	X         int32
	Y         int32
	MouseData uint32
	Flags     uint32
	Time      uint32
	Extra     uintptr
}

type keyInput struct {
	kind  uint32
	key   uint16
	scan  uint16
	flags uint32
	Time  uint32
	Extra uintptr
}

type hardwareInput struct {
	Message uint32
	ParamL  uint16
	ParamH  uint16
}

// guiThreadInfo is what GetGUIThreadInfo fills in about a thread's input state.
// Only the fields this package reads are named, and every other one is kept
// because Windows writes the whole structure: a field left out would move the
// ones after it, and the size is what Windows is told before it writes.
type guiThreadInfo struct {
	Size          uint32
	Flags         uint32
	Active        uintptr
	Focus         uintptr
	Capture       uintptr
	MenuOwner     uintptr
	MoveSize      uintptr
	CaretBlink    rect
	CaretRect     rect
	CaretPosition point
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
