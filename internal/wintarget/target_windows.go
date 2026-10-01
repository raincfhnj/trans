//go:build windows

package wintarget

import (
	"context"
	"fmt"

	"trans/internal/win32"
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

// deliver is the Windows delivery: the policy in policy.go, over the seams
// Windows answers.
func deliver(ctx context.Context, handle uintptr, pasteKeys, text string, submit bool) error {
	seam := systemSeams()
	return deliverWith(ctx, &seam, handle, pasteKeys, text, submit)
}

// systemSeams is the delivery against the real Windows. It is the only part of
// the path that Win32 knows about, which is what leaves the delivery itself
// checkable anywhere.
//
// The paste chord is not one of the seams: it is what the caller asked for, and
// it is passed to the delivery the way the prompt is.
func systemSeams() seams {
	return seams{
		snapshot:   win32.SnapshotClipboard,
		restore:    win32.RestoreClipboard,
		release:    win32.ReleaseClipboard,
		activate:   win32.Activate,
		foreground: win32.StillForeground,
		waitFocus:  win32.WaitForForeground,
		waitIdle:   win32.WaitForInputIdle,
		write:      win32.SetClipboardText,
		pause:      win32.Pause,
		paste: func(ctx context.Context, chord string) (int, error) {
			if err := ctx.Err(); err != nil {
				return 0, err
			}
			// Injected keystrokes are silent when they fail: Windows drops
			// input aimed at a window of a higher integrity level without a
			// word, and what the author sees is a prompt that never arrives.
			// What can be checked is checked, and what happened is written
			// down.
			if err := win32.Paste(chord); err != nil {
				note(fmt.Sprintf("the paste chord %s was refused: %s", chord, err))
				return 0, err
			}
			// Nothing was typed here, so there is no count of events to pass
			// back beyond the chord having gone out whole: win32.Paste answers
			// an error when Windows took fewer events than it was given.
			return 1, nil
		},
		submit: func(ctx context.Context) {
			if err := ctx.Err(); err != nil {
				// The prompt is in the window; the author sends it themselves.
				note("the return key was not pressed: the delivery was cancelled")
				return
			}
			win32.Enter()
		},
	}
}
