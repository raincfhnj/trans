package wintarget

import (
	"context"
	"errors"
	"fmt"
	"time"

	"trans/internal/win32"
	"trans/internal/winlog"
)

// The waiting a delivery does where it has no signal to wait for. What it waits
// on elsewhere is a real signal — the window in front, the input queue
// emptying — and these two are the small, named sleeps that stand in for one.
// They are not guarantees and are not written as any: each says what it waits
// for and why nothing reports it.
const (
	// pauseBeforeReturn is the gap between the paste and the return key. A pane
	// that is still chewing on the pasted text can read the return as part of
	// it, and no call reports that a pane has finished with a paste — a
	// terminal reads the paste itself and says nothing about it. The delivery
	// waits the target's input queue empty before this, which is the nearest
	// thing to a signal there is, and this is the moment left over: it also
	// covers the case of a queue that could not be asked about at all.
	pauseBeforeReturn = 150 * time.Millisecond
	// settleBeforeRestore is the pause between the paste and putting the
	// clipboard back. The pane reads the prompt out of the clipboard after the
	// chord, and nothing says when it has taken the text rather than merely
	// seen it: the clipboard being read leaves no trace at all. This is the
	// last moment of that handover.
	settleBeforeRestore = 250 * time.Millisecond
)

// The errors a delivery answers with. They are values rather than strings
// because the panel shows the same words for the same trouble every time, and
// because a test can then ask which one it got instead of reading it.
var (
	errNoWindow   = errors.New("no window to deliver the prompt to")
	errNoText     = errors.New("nothing to deliver")
	errLostFocus  = errors.New("the target window lost focus before the prompt could be pasted")
	errPastedLate = errors.New("the target window lost focus after the prompt was pasted: the text is in the window but was not submitted")
)

// seams is everything a delivery touches outside this program: the clipboard
// it borrows, the window it brings forward, the keys it presses, and the
// waiting in between. A test stands in for all of it, which is how the policy
// below is checked without a desktop, and the Windows side lives in
// target_windows.go.
//
// The two waits that are signals answer whether the thing waited for happened,
// and a false answer means the bound was reached rather than that the wait
// failed: what to do about it is the policy's decision. Every one of them takes
// the context, because the panel cancels the delivery it started when the
// author presses escape.
type seams struct {
	snapshot   func() (win32.ClipboardSnapshot, error)
	restore    func(win32.ClipboardSnapshot) error
	release    func(win32.ClipboardSnapshot)
	activate   func(handle uintptr) error
	foreground func(handle uintptr) bool
	waitFocus  func(ctx context.Context, handle uintptr) bool
	waitIdle   func(ctx context.Context, handle uintptr) bool
	write      func(text string) error
	pause      func(ctx context.Context, howLong time.Duration) error
	paste      func(ctx context.Context, chord string) (sent int, err error)
	submit     func(ctx context.Context)
}

// deliverWith hands the prompt over along the seams given. The order is the
// whole of the delivery, so it is written out here rather than in the Windows
// file: put what is on the clipboard to one side, bring the target forward,
// wait for it to be in front, put the prompt on the clipboard, paste it, and —
// when the caller wants the prompt sent — wait for the pane to take the paste
// before pressing return.
//
// The seams are taken by pointer because they are a table of function values
// rather than a thing: they are read and never written, and there is no reason
// to copy the table for every delivery.
//
// What was on the clipboard is put back on every path from the moment the
// prompt goes on it, including a path that ends in an error: the clipboard
// belongs to whoever filled it and this only borrows it.
func deliverWith(ctx context.Context, seam *seams, handle uintptr, pasteKeys, text string, submit bool) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if handle == 0 {
		return errNoWindow
	}
	if text == "" {
		return errNoText
	}

	before, err := seam.snapshot()
	if err != nil {
		// The clipboard holds something that cannot be put back, and the
		// prompt would overwrite it. The author keeps the screenshot and loses
		// the prompt, which is the trade this makes on purpose: what is on the
		// clipboard is not this program's to throw away.
		note("the prompt was not delivered: " + err.Error())
		return err
	}
	defer seam.release(before)

	// Whatever happens from here on, the clipboard goes back as it was found.
	// The snapshot is the only copy of it, and a path that failed before the
	// prompt went on the clipboard puts back what was never taken away — which
	// costs one write and saves a second path to get right.
	promptOnClipboard := true
	pasted := false
	defer func() {
		if !promptOnClipboard || !before.Holds() {
			return
		}
		// Nothing reports that a pane is done with the clipboard: reading it
		// leaves no trace. The wait is a moment, not a guarantee.
		if pasted {
			_ = seam.pause(ctx, settleBeforeRestore)
		}
		if restoreErr := seam.restore(before); restoreErr != nil {
			note("the clipboard could not be put back: " + restoreErr.Error())
		}
	}()

	if err := seam.activate(handle); err != nil {
		return err
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	// Focus is not held for anyone: between the activation and the keystrokes
	// below a click, a notification, or a dialog the target opened can take it.
	// A key sent to whatever took it puts the prompt in the wrong window while
	// this reports success, so the window in front is waited for as a signal
	// and then asked about again immediately before each keystroke.
	if !seam.waitFocus(ctx, handle) {
		// A wait the panel cancelled is the panel's own answer and not a
		// window that would not come forward; saying "lost focus" for it would
		// report something that never happened.
		if err := ctx.Err(); err != nil {
			return err
		}
		return errLostFocus
	}
	if !seam.foreground(handle) {
		return errLostFocus
	}

	if err := seam.write(text); err != nil {
		return err
	}

	sent, err := seam.paste(ctx, pasteKeys)
	if err != nil {
		return pasteFailed(err)
	}
	if sent == 0 {
		return pasteFailed(errors.New("windows took none of the key events"))
	}
	pasted = true

	if !submit {
		return nil
	}
	// The paste has happened, so the text is in the window; what is left is the
	// return that hands it over. The pane's input queue emptying is the nearest
	// thing to a signal that it has read the paste, and the named pause below
	// is what covers the rest.
	if !seam.waitIdle(ctx, handle) {
		if err := seam.pause(ctx, pauseBeforeReturn); err != nil {
			return err
		}
	}
	if !seam.foreground(handle) {
		// The paste already happened, so the text is in the window; what did
		// not happen is the return that hands it over. Saying only that the
		// window lost focus would leave the text where it is unexplained.
		return errPastedLate
	}
	seam.submit(ctx)
	return nil
}

// pasteFailed says why a prompt that is on the clipboard was not pasted, and
// what that means for the author: the text is waiting on the clipboard and
// nothing has taken it, so it can be pasted by hand.
//
// UIPI is the reason this cannot be told apart from the outside. Windows
// silently drops input aimed at a window of a higher integrity level — an
// elevated console is the ordinary case — and reports nothing to the program
// that sent it, so what the author experiences is a prompt that quietly never
// arrives.
func pasteFailed(err error) error {
	return fmt.Errorf("the prompt is on the clipboard but could not be pasted: %w", err)
}

// note writes down what a delivery did about a clipboard it could not take
// safely, or keys Windows refused. There is nobody to tell at the moment it
// happens — the panel is told by the error the caller answers — so this is for
// whoever reads the log afterwards.
func note(what string) {
	winlog.Note("trans-window", "%s", what)
}
