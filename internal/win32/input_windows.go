//go:build windows

package win32

import (
	"errors"
	"fmt"
	"syscall"
	"time"
	"unicode/utf16"
	"unsafe"
)

// ClipboardText reads what is on the clipboard, so a prompt that goes through
// the clipboard does not take with it whatever was copied before. It is text
// alone: everything else the clipboard can hold is the business of
// SnapshotClipboard, which copies it so that it can be put back.
func ClipboardText() string {
	if !openClipboard() {
		return ""
	}
	defer closeClipboard()

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
	// The address is the clipboard's own memory and is valid until the
	// CloseClipboard above, which is why the reading happens here rather than
	// from anything the handle is kept in.
	return syscall.UTF16ToString(locked[uint16](pointer, size/2))
}

// SetClipboardText puts text on the clipboard. A prompt is delivered by pasting
// it, because typing a long one key by key would take longer than the agent
// needs to answer.
//
// This empties the clipboard, so it is for a caller that has already put what
// was there to one side with SnapshotClipboard and will put it back; what it
// cannot restore is lost otherwise.
func SetClipboardText(text string) error {
	if !openClipboard() {
		return errors.New("the clipboard is held by something else")
	}
	defer closeClipboard()

	call(procEmptyClipboard)

	units := append(utf16.Encode([]rune(text)), 0)
	size := uintptr(len(units) * 2)
	handle := call(procGlobalAlloc, gmemMoveable, size)
	if handle == 0 {
		return lastError("GlobalAlloc")
	}
	pointer := call(procGlobalLock, handle)
	if pointer == 0 {
		call(procGlobalFree, handle)
		return lastError("GlobalLock")
	}
	copy(locked[uint16](pointer, len(units)), units)
	call(procGlobalUnlock, handle)

	// The clipboard owns the memory from here on: what SetClipboardData accepts
	// is never freed by this program. What it refuses stays this program's and
	// is freed here.
	if call(procSetClipboardData, cfUnicodeText, handle) == 0 {
		call(procGlobalFree, handle)
		return lastError("SetClipboardData")
	}
	return nil
}

// openClipboard tries for a moment: another process holding the clipboard for a
// few milliseconds is normal and no reason to lose a prompt.
//
// The owner it names matters. Microsoft's contract for OpenClipboard is that a
// window opened with a NULL owner makes the EmptyClipboard that follows set the
// clipboard's owner to NULL, "which causes SetClipboardData to fail" — so a
// window of this process is named whenever there is one. NULL is kept for the
// case of a process with no window at all, where reading the clipboard still
// works and writing it is what would fail.
func openClipboard() bool {
	return openClipboardOwned(true)
}

// openClipboardOwner is the handle the clipboard is opened under, asked for
// each time rather than remembered: the panel's window can be closed and opened
// again while this program runs.
func openClipboardOwner() uintptr {
	if own := call(procGetConsoleWindow); own != 0 {
		return own
	}
	return HostWindow(PanelTitle)
}

// closeClipboard gives the clipboard back to whoever wants it next.
func closeClipboard() {
	call(procCloseClipboard)
}

func openClipboardOwned(allowOwner bool) bool {
	owner := uintptr(0)
	if allowOwner {
		owner = openClipboardOwner()
	}
	for attempt := 0; attempt < 10; attempt++ {
		if call(procOpenClipboard, owner) != 0 {
			return true
		}
		time.Sleep(20 * time.Millisecond)
	}
	// A window handle of this process that the clipboard refuses is worth
	// trying again without: the NULL owner is documented to make a later write
	// fail, and a clipboard that cannot be written at all is worse than one
	// opened the hard way.
	if owner != 0 {
		return openClipboardOwned(false)
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

// Chord presses a combination written the way a person writes one, and answers
// how many key events Windows took and whether it took them all: input aimed at
// a window of a higher integrity level is dropped without a word, which is
// checked for rather than assumed.
func Chord(spec string) error {
	modifiers, key, err := Keys(spec)
	if err != nil {
		return err
	}

	held := heldKeys(modifiers)
	events := make([]keyInput, 0, len(held)*2+2)
	for _, modifier := range held {
		events = append(events, keyDown(modifier))
	}
	events = append(events, keyDown(uintptr(key)), keyUp(uintptr(key)))
	for index := len(held) - 1; index >= 0; index-- {
		events = append(events, keyUp(held[index]))
	}

	seen, err := sendKeys(events)
	if err != nil {
		return err
	}
	if seen != len(events) {
		return fmt.Errorf("%s went only partly through: %d of %d key events were taken", spec, seen, len(events))
	}
	return nil
}

// heldKeys is the set of keys held down for a combination, in the order they
// are pressed (and the reverse of that for the release). It answers for win as
// for the others: a chord written win+v is the Windows key held while v is
// pressed, which is the paste chord on its way to the clipboard history — it
// must not degrade into a bare v.
func heldKeys(modifiers uint32) []uintptr {
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
	if modifiers&modWin != 0 {
		held = append(held, vkLWin)
	}
	return held
}

// Enter presses return, which is what hands the prompt over in a pane that is
// waiting for one. It is the one injection whose result is not reported back:
// the caller has already decided the prompt is pasted, and a return that did
// not go through leaves the text in the window, which is what the delivery then
// says. The log has the detail.
func Enter() {
	seen, err := sendKeys([]keyInput{keyDown(vkReturn), keyUp(vkReturn)})
	if err != nil {
		note("return could not be sent: " + err.Error())
		return
	}
	if seen != 2 {
		// UIPI drops input aimed at a window of a higher integrity level
		// without telling anyone. What the author sees is a prompt that sits in
		// the window and is never submitted; nothing here can do better than
		// write it down.
		note("return was only partly accepted: 2 key events were sent, " +
			"and Windows took a different number")
	}
}

// keyDown and keyUp are the two events one key press is made of. The scan code
// is left for Windows to fill in, which is what it does for keybd_event's
// callers too; the virtual key alone is enough for a terminal.
func keyDown(key uintptr) keyInput {
	return keyInput{kind: inputKeyboard, key: uint16(key)}
}

func keyUp(key uintptr) keyInput {
	return keyInput{kind: inputKeyboard, key: uint16(key), flags: keyEventKeyUp}
}

// sendKeys hands Windows a run of key events at once and answers how many of
// them it took. It waits the same short moment between the events that the
// older keybd_event path did, because a pane reads a paste as it arrives and
// two events with no gap at all can be read as one.
func sendKeys(events []keyInput) (int, error) {
	inputs := make([]input, 0, len(events))
	for _, event := range events {
		inputs = append(inputs, input{kind: event.kind, union: inputUnion{keyboard: event}})
	}
	if len(inputs) == 0 {
		return 0, nil
	}

	size := unsafe.Sizeof(input{})
	result, _, _ := procSendInput.Call(
		uintptr(len(inputs)),
		uintptr(unsafe.Pointer(&inputs[0])),
		size,
	)
	seen := int(result)
	if seen == 0 {
		return 0, lastError("SendInput")
	}

	// Windows has the events now; the pause is the pane's moment to read them
	// in the order they were sent.
	time.Sleep(keyGap * time.Duration(len(inputs)))
	return seen, nil
}

// keyGap is the moment left between two injected key events. It is not a
// signal: nothing reports that a pane has read a key, and a paste that arrives
// in one unbroken run can be read as a single keystroke by the program drawing
// the console.
const keyGap = 15 * time.Millisecond
