//go:build windows

package wintarget

import (
	"context"
	"errors"
	"time"

	"trans/internal/win32"
)

const (
	// A pane needs a moment between taking the keyboard and being pasted into;
	// a key press that arrives while the window is still coming forward lands
	// in whatever had the focus before.
	settleDelay = 250 * time.Millisecond
	// Return after the paste: a terminal that is still chewing on the paste may
	// read the return as part of it, and a prompt that is only typed in must not
	// be sent at all.
	enterDelay = 200 * time.Millisecond
	// The clipboard has to hold the prompt until the pane has taken it. Only
	// then is what was there before put back.
	restoreDelay = 250 * time.Millisecond
)

// Sending hands the prompt over: it is pasted in and return is pressed, so the
// agent starts working.
type Sending struct {
	handle    uintptr
	pasteKeys string
}

// NewSending hands the prompt over: the pane is pasted into and return is
// pressed, so the agent starts working. The chord is the one that terminal takes
// for a paste, which is not the same everywhere.
func NewSending(handle uintptr, pasteKeys string) Sending {
	return Sending{handle: handle, pasteKeys: pasteKeys}
}

func (s Sending) Insert(ctx context.Context, text string) error {
	return deliver(ctx, s.handle, s.pasteKeys, text, true)
}

// Typing pastes the prompt and leaves the last keystroke to the author, which is
// what the review action of the plugin does.
type Typing struct {
	handle    uintptr
	pasteKeys string
}

// NewTyping pastes the prompt and leaves the last keystroke to the author, which
// is what the review action of the plugin does.
func NewTyping(handle uintptr, pasteKeys string) Typing {
	return Typing{handle: handle, pasteKeys: pasteKeys}
}

func (t Typing) Insert(ctx context.Context, text string) error {
	return deliver(ctx, t.handle, t.pasteKeys, text, false)
}

func deliver(ctx context.Context, handle uintptr, pasteKeys, text string, submit bool) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if handle == 0 {
		return errors.New("no window to deliver the prompt to")
	}
	if text == "" {
		return errors.New("nothing to deliver")
	}

	// What was on the clipboard belongs to whoever put it there; the prompt only
	// borrows it for the moment the pane needs to take it.
	before := win32.ClipboardText()

	// From the SetClipboardText below on, the clipboard no longer holds what it
	// held a moment ago — it is emptied before the prompt is put on it — so every
	// path from there puts the old text back. Once the prompt has been pasted the
	// pane may still be taking it, so that path waits restoreDelay first; a path
	// that failed before the paste restores at once.
	promptOnClipboard := false
	pasted := false
	defer func() {
		if !promptOnClipboard {
			return
		}
		if pasted {
			time.Sleep(restoreDelay)
		}
		if before != "" {
			_ = win32.SetClipboardText(before)
		}
	}()

	if err := win32.Activate(handle); err != nil {
		return err
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	promptOnClipboard = true
	if err := win32.SetClipboardText(text); err != nil {
		return err
	}

	// Focus is not held for anyone: between the activation above and each
	// keystroke below a click, a notification, or a dialog the target opened can
	// take it. Keys sent to whatever took the focus would put the prompt in the
	// wrong window while this reports success, so the window in front is asked
	// again immediately before each of the two keystrokes.
	time.Sleep(settleDelay)
	if !win32.StillForeground(handle) {
		return errors.New("the target window lost focus before the prompt could be pasted")
	}
	if err := win32.Paste(pasteKeys); err != nil {
		return err
	}
	pasted = true
	if submit {
		time.Sleep(enterDelay)
		if !win32.StillForeground(handle) {
			// The paste already happened, so the text is in the window; what did
			// not happen is the return that hands it over. Saying only that the
			// window lost focus would leave the text where it is unexplained.
			return errors.New("the target window lost focus after the prompt was pasted: the text is in the window but was not submitted")
		}
		win32.Enter()
	}

	return nil
}
