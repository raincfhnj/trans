//go:build windows

package win32

import (
	"errors"
	"syscall"
	"time"
	"unicode/utf16"
	"unsafe"
)

// ClipboardText reads what is on the clipboard, so a prompt that goes through
// the clipboard does not take with it whatever was copied before.
func ClipboardText() string {
	if !openClipboard() {
		return ""
	}
	defer call(procCloseClipboard)

	handle := call(procGetClipboardData, cfUnicodeText)
	if handle == 0 {
		return ""
	}
	pointer := call(procGlobalLock, handle)
	if pointer == 0 {
		return ""
	}
	defer call(procGlobalUnlock, handle)

	size := int(call(procGlobalSize, handle))
	if size <= 0 {
		return ""
	}
	return syscall.UTF16ToString(cellsAt(pointer, size/2))
}

// SetClipboardText puts text on the clipboard. A prompt is delivered by pasting
// it, because typing a long one key by key would take longer than the agent
// needs to answer.
func SetClipboardText(text string) error {
	if !openClipboard() {
		return errors.New("the clipboard is held by something else")
	}
	defer call(procCloseClipboard)

	call(procEmptyClipboard)

	units := append(utf16.Encode([]rune(text)), 0)
	size := uintptr(len(units) * 2)
	handle := call(procGlobalAlloc, gmemMoveable, size)
	if handle == 0 {
		return lastError("GlobalAlloc")
	}
	pointer := call(procGlobalLock, handle)
	if pointer == 0 {
		return lastError("GlobalLock")
	}
	copy(cellsAt(pointer, len(units)), units)
	call(procGlobalUnlock, handle)

	// The clipboard owns the memory from here on; it is not freed here.
	if call(procSetClipboardData, cfUnicodeText, handle) == 0 {
		return lastError("SetClipboardData")
	}
	return nil
}

// openClipboard tries for a moment: another process holding the clipboard for a
// few milliseconds is normal and no reason to lose a prompt.
func openClipboard() bool {
	for attempt := 0; attempt < 10; attempt++ {
		if call(procOpenClipboard, 0) != 0 {
			return true
		}
		time.Sleep(20 * time.Millisecond)
	}
	return false
}

// Paste presses the chord a terminal takes for pasting. Which chord that is
// differs: the console host pastes on ctrl+v, a terminal program of its own on
// ctrl+shift+v — so it is said rather than assumed.
func Paste(keys string) error {
	return Chord(keys)
}

// DefaultPasteChord is the chord the program drawing this console takes for a
// paste. The console's own window is the one that pastes on ctrl+v; anything
// else drawing the console is a terminal program.
func DefaultPasteChord() string {
	own := call(procGetConsoleWindow)
	if own != 0 && own == HostWindow(PanelTitle) {
		return "ctrl+v"
	}
	return "ctrl+shift+v"
}

// Chord presses a combination written the way a person writes one.
func Chord(spec string) error {
	modifiers, key, err := Keys(spec)
	if err != nil {
		return err
	}

	held := []uintptr{}
	if modifiers&modControl != 0 {
		held = append(held, vkControl)
	}
	if modifiers&modShift != 0 {
		held = append(held, vkShift)
	}
	if modifiers&modAlt != 0 {
		held = append(held, vkMenu)
	}
	for _, modifier := range held {
		keyDown(modifier)
	}
	press(uintptr(key))
	for index := len(held) - 1; index >= 0; index-- {
		keyUp(held[index])
	}
	return nil
}

// Enter presses return, which is what hands the prompt over in a pane that is
// waiting for one.
func Enter() {
	press(vkReturn)
}

func press(key uintptr) {
	keyDown(key)
	keyUp(key)
}

func keyDown(key uintptr) {
	call(procKeybdEvent, key, 0, 0, 0)
	time.Sleep(15 * time.Millisecond)
}

func keyUp(key uintptr) {
	call(procKeybdEvent, key, 0, keyEventKeyUp, 0)
	time.Sleep(15 * time.Millisecond)
}

// cellsAt is the memory GlobalLock handed back, seen as the characters it holds.
// The address is Windows's, not the collector's: nothing here can keep it alive
// and nothing needs to.
func cellsAt(address uintptr, units int) []uint16 {
	return unsafe.Slice((*uint16)(unsafe.Pointer(address)), units) //nolint:gosec,govet // the address belongs to the clipboard
}
