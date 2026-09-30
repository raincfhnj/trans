//go:build windows

package main

import (
	"context"
	"errors"
	"strings"
	"time"

	"trans/internal/win32"
)

// The capture touches the world outside this process in five places: the
// clipboard twice, the window that has to be in front, the keys pressed into
// it, and the waiting in between. A test stands in for Windows with its own.
type capturePorts struct {
	read  func() string
	hold  func(string) error
	front func(uintptr) error
	press func(string) error
	wait  func(time.Duration)
}

var systemPorts = capturePorts{
	read:  win32.ClipboardText,
	hold:  win32.SetClipboardText,
	front: win32.Activate,
	press: win32.Chord,
	wait:  time.Sleep,
}

const (
	// The window needs a moment to come forward before a key press is worth
	// sending, and the selection needs a moment to reach the clipboard after
	// the chord — a press that arrives while the window is still coming forward
	// lands in whatever had the focus before.
	frontSettle = 250 * time.Millisecond
	copySettle  = 250 * time.Millisecond
)

// errNothingCaptured is the capture and the clipboard both coming up empty:
// there is nothing to open the panel with, which is worth saying rather than
// leaving a box that looks broken.
var errNothingCaptured = errors.New(
	"nothing captured — no text was selected, and the clipboard is empty")

// selection copies what is selected in the target window. The clipboard is put
// back as it was: it belongs to whoever filled it, and the capture borrows it
// for only the moment it takes to read the selection. When the chord brought
// nothing — nothing was selected — whatever was on the clipboard is what comes
// back, because that is now the only text there is to read.
func (ports capturePorts) selection(target uintptr, keys string) (string, error) {
	before := ports.read()

	if err := ports.front(target); err != nil {
		return "", err
	}
	ports.wait(frontSettle)
	if err := ports.press(keys); err != nil {
		return "", err
	}
	ports.wait(copySettle)

	captured := ports.read()
	if before != "" {
		_ = ports.hold(before)
	}
	if strings.TrimSpace(captured) == "" {
		return before, nil
	}
	return captured, nil
}

// readSource is what the draft opens with in read mode. Plain --read takes what
// is already on the clipboard; --capture goes and gets the selection out of the
// window first, because the panel is about to cover it. A capture that could
// not happen falls back to the clipboard and says why; one that found nothing
// at all — no selection, no clipboard — leaves an empty draft and says that.
func readSource(ports capturePorts, target uintptr, capture bool, keys string) (string, error) {
	if !capture {
		return ports.read(), nil
	}

	// The chord is checked before anything is moved: a setting that names no
	// key must not bring a window forward for nothing.
	if _, _, err := win32.Keys(keys); err != nil {
		return ports.read(), err
	}

	captured, err := ports.selection(target, keys)
	if err != nil {
		return ports.read(), err
	}
	if strings.TrimSpace(captured) == "" {
		return "", errNothingCaptured
	}
	return captured, nil
}

// copying is where a translation being read goes. Nothing reaches the window
// in read mode: the author pastes the result wherever it belongs, so this is
// the target the flow hands it to.
type copying struct{}

func (copying) Insert(_ context.Context, text string) error {
	return win32.SetClipboardText(text)
}
