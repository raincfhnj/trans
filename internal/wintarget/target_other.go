//go:build !windows

// The other half of the delivery: everywhere but Windows there is no window to
// paste into, so the two targets exist and refuse. They are here rather than
// left out so that the package has one shape on every platform — a change to
// what a target is would otherwise compile on Windows and go unnoticed by the
// Linux build and its lint pass.
package wintarget

import (
	"context"
	"errors"
)

// errWindowsOnly is what a delivery off Windows answers: the panel is a Windows
// program, and this package speaks to windows.
var errWindowsOnly = errors.New("delivering a prompt is for Windows only")

// Sending hands the prompt over on Windows: it is pasted in and return is
// pressed, so the agent starts working.
type Sending struct {
	handle    uintptr
	pasteKeys string
}

// NewSending is the target that sends. It is built the same way everywhere; what
// differs is whether it can do anything.
func NewSending(handle uintptr, pasteKeys string) Sending {
	return Sending{handle: handle, pasteKeys: pasteKeys}
}

func (s Sending) Insert(context.Context, string) error { return errWindowsOnly }

// Typing pastes the prompt and leaves the last keystroke to the author.
type Typing struct {
	handle    uintptr
	pasteKeys string
}

// NewTyping is the target that types. It is built the same way everywhere; what
// differs is whether it can do anything.
func NewTyping(handle uintptr, pasteKeys string) Typing {
	return Typing{handle: handle, pasteKeys: pasteKeys}
}

func (t Typing) Insert(context.Context, string) error { return errWindowsOnly }
