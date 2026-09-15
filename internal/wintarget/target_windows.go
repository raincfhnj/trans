//go:build windows

package wintarget

import (
	"context"
	"fmt"
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
		return fmt.Errorf("no window to deliver the prompt to")
	}
	if text == "" {
		return fmt.Errorf("nothing to deliver")
	}

	// What was on the clipboard belongs to whoever put it there; the prompt only
	// borrows it for the moment the pane needs to take it.
	before := win32.ClipboardText()
	if err := win32.Activate(handle); err != nil {
		return err
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	if err := win32.SetClipboardText(text); err != nil {
		return err
	}

	time.Sleep(settleDelay)
	if err := win32.Paste(pasteKeys); err != nil {
		return err
	}
	if submit {
		time.Sleep(enterDelay)
		win32.Enter()
	}

	time.Sleep(restoreDelay)
	if before != "" {
		_ = win32.SetClipboardText(before)
	}
	return nil
}
