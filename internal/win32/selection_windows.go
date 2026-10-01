//go:build windows

package win32

import (
	"context"
	"time"
	"unsafe"
)

// RestoreClipboard puts a snapshot back the way it found the clipboard. It is
// written as a plain function as well as a method so that it can be handed to
// the delivery and capture seams, which take functions rather than types.
func RestoreClipboard(snapshot ClipboardSnapshot) error {
	if snapshot == nil {
		return nil
	}
	return snapshot.Restore()
}

// ReleaseClipboard frees what a snapshot still holds, after which it is of no
// further use: a snapshot that was put back still holds the picture when a
// restore failed part way, and one that was never used holds everything.
func ReleaseClipboard(snapshot ClipboardSnapshot) {
	if snapshot == nil {
		return
	}
	if releaser, ok := snapshot.(interface{ Release() }); ok {
		releaser.Release()
	}
}

// clipboardSequence is the number Windows moves every time the contents of the
// clipboard change. A capture waits for it to move instead of sleeping and
// hoping: it is the only signal the clipboard offers, and it says that
// something wrote — not who, which is why the token is still what decides
// whether a copy happened.
func clipboardSequence() uint32 {
	return uint32(call(procGetClipboardSequenceNumbr))
}

// SelectedText answers the text the author has selected in the pane in front —
// what the selection's chord sends to the panel for translating.
//
// With no chord the clipboard is read as it stands: a terminal that copies on
// selection has already put the selection there. With a chord the clipboard is
// put to one side, marked with a token, and copied into by the pane — a token
// still on the clipboard afterwards is what says nothing was selected, where a
// plain before-and-after could not tell a copy that did not happen from one
// that brought back the very text that was on the clipboard.
//
// The capture itself is in selection.go, written over ports rather than over
// Win32, so that the paths it takes can be checked on a machine that is not
// Windows. This is the Windows end of it: the ports, and the window the chord
// was pressed in.
func SelectedText(chord string) (string, error) {
	return captureSelection(context.Background(), systemSelectionPorts(), foregroundTarget(), chord)
}

// systemSelectionPorts is a capture against the real Windows: the reading and
// writing of the clipboard, the window in front, the keys, the sequence number
// that says the pane has copied, and the two small waits that cover the moments
// nothing reports.
func systemSelectionPorts() selectionPorts {
	return selectionPorts{
		read:     ClipboardText,
		write:    SetClipboardText,
		snapshot: SnapshotClipboard,
		restore:  RestoreClipboard,
		release:  ReleaseClipboard,
		front:    Activate,
		copyKeys: func(ctx context.Context, chord string) error {
			if err := ctx.Err(); err != nil {
				return err
			}
			return Chord(chord)
		},
		waitCopy:   waitForClipboardChange,
		waitSettle: Pause,
	}
}

// foregroundTarget is the window the copy is meant for: whatever is in front
// when the capture starts, because that is where the chord was pressed. A
// capture has no handle of its own to work from.
func foregroundTarget() uintptr {
	return Foreground().Handle
}

// The bounds on the waits a delivery does, and the steps it polls in. Each is a
// bound and not a guarantee: a window that is going to take the keyboard does so
// within a few milliseconds, and one that has not by the end of the second has
// been refused by Windows.
const (
	foregroundPoll = 25 * time.Millisecond
	foregroundWait = time.Second

	// The target's input queue is asked about between a paste and the return
	// key: a queue still holding the paste can read the return as part of it.
	inputQueuePoll = 20 * time.Millisecond
	inputQueueWait = time.Second

	// The clipboard is polled for the write a pane makes when it copies the
	// selection: a pane takes the copy a few milliseconds after the chord, and
	// one that has not written by the end of this is not going to.
	clipboardPoll = 10 * time.Millisecond
	clipboardWait = time.Second
)

// WaitForForeground waits, bounded, for the window to be the one in front, and
// answers whether it is. It stops early when the context is done, which is what
// the panel asks for when the author presses escape.
func WaitForForeground(ctx context.Context, handle uintptr) bool {
	deadline := time.Now().Add(foregroundWait)
	for {
		if StillForeground(handle) {
			return true
		}
		if time.Now().After(deadline) {
			return false
		}
		if err := Pause(ctx, foregroundPoll); err != nil {
			return false
		}
	}
}

// waitForClipboardChange waits, bounded, for the clipboard to change, which is
// what a pane does when it copies the selection. It answers whether the change
// came. There is no way to ask which window wrote the clipboard, so a change is
// taken as the pane's — the chord went nowhere else — and a capture that sees
// none reads the clipboard anyway, where the token says what happened.
//
// It belongs to this package: the wait is part of how a capture is driven, and
// the delivery that also waits for a pane to settle waits on its input queue
// rather than on the clipboard.
func waitForClipboardChange(ctx context.Context) bool {
	started := clipboardSequence()
	deadline := time.Now().Add(clipboardWait)
	for time.Now().Before(deadline) {
		if err := Pause(ctx, clipboardPoll); err != nil {
			return false
		}
		if clipboardSequence() != started {
			return true
		}
	}
	return false
}

// WaitForInputIdle waits, bounded, for the target's input queue to be empty,
// which is nearer to "the paste has been read" than a sleep is. It answers
// false when the queue could not be asked about at all — it may belong to a
// program this process cannot look into — and the caller then falls back to its
// own short pause rather than treating that as a failure.
//
// What it waits for is a moment and not a guarantee: an empty queue says the
// messages have been dispatched, and a terminal can still be drawing the paste
// when that happens.
func WaitForInputIdle(ctx context.Context, handle uintptr) bool {
	deadline := time.Now().Add(inputQueueWait)
	for time.Now().Before(deadline) {
		busy, known := inputPending(handle)
		if !known {
			return false
		}
		if !busy {
			return true
		}
		if err := Pause(ctx, inputQueuePoll); err != nil {
			return false
		}
	}
	return false
}

// inputPending says whether the thread of a window still holds input it has not
// dispatched, and whether that could be asked at all. GetGUIThreadInfo answers
// nothing for the thread of a program this process may not reach.
func inputPending(handle uintptr) (busy, known bool) {
	thread := uint32(call(procGetWindowThreadProcessID, handle, 0))
	if thread == 0 {
		return false, false
	}
	state := guiThreadInfo{Size: uint32(unsafe.Sizeof(guiThreadInfo{}))}
	if call(procGetGUIThreadInfo, uintptr(thread), uintptr(unsafe.Pointer(&state))) == 0 {
		return false, false
	}
	return state.Flags&guiInInput != 0, true
}
