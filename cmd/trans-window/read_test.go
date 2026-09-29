//go:build windows

package main

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"trans/internal/win32"
)

// fakeWindows is the capture's five touches of the world outside this process,
// played by a machine of our own: the clipboard, the window coming forward, the
// chord, and the waiting in between. Pressing the chord copies the selection
// onto the clipboard, the way a real copy would.
type fakeWindows struct {
	clipboard string
	selection string
	events    []string
	frontErr  error
	pressErr  error
}

func (w *fakeWindows) ports() capturePorts {
	return capturePorts{
		read: func() string {
			w.events = append(w.events, "read")
			return w.clipboard
		},
		hold: func(text string) error {
			w.events = append(w.events, "hold")
			w.clipboard = text
			return nil
		},
		front: func(uintptr) error {
			w.events = append(w.events, "front")
			return w.frontErr
		},
		press: func(string) error {
			w.events = append(w.events, "press")
			if w.pressErr == nil {
				w.clipboard = w.selection
			}
			return w.pressErr
		},
		wait: func(time.Duration) {
			w.events = append(w.events, "wait")
		},
	}
}

// The capture moves in one order, and the clipboard ends up as it was: the
// selection comes back to the draft, everything else goes back to the person
// whose clipboard it is.
func TestTheSelectionIsCopiedOutAndTheClipboardIsPutBackAsItWas(t *testing.T) {
	t.Parallel()

	windows := &fakeWindows{
		clipboard: "a note someone kept",
		selection: "the line that is selected in the window",
	}

	got, err := windows.ports().selection(0x1234, "ctrl+shift+c")
	if err != nil {
		t.Fatalf("the selection returned an error: %v", err)
	}
	if got != windows.selection {
		t.Errorf("the draft opens with %q, want the selection %q", got, windows.selection)
	}
	if windows.clipboard != "a note someone kept" {
		t.Errorf("the clipboard holds %q after the capture, want it as it was",
			windows.clipboard)
	}

	want := []string{"read", "front", "wait", "press", "wait", "read", "hold"}
	if !equal(windows.events, want) {
		t.Errorf("the capture happened as %v, want %v", windows.events, want)
	}
}

// A chord pressed over nothing selects nothing. What was on the clipboard is
// then the only text there is to read, so it is what the draft opens with — the
// capture never throws away what it failed to improve on.
func TestNothingSelectedFallsBackToWhatWasOnTheClipboard(t *testing.T) {
	t.Parallel()

	windows := &fakeWindows{clipboard: "kept from before", selection: ""}

	got, err := windows.ports().selection(0x1234, "ctrl+shift+c")
	if err != nil {
		t.Fatalf("the selection returned an error: %v", err)
	}
	if got != "kept from before" {
		t.Errorf("the draft opens with %q, want the clipboard as it was", got)
	}
	if windows.clipboard != "kept from before" {
		t.Errorf("the clipboard holds %q after the capture, want it as it was",
			windows.clipboard)
	}
}

// Capture with nothing anywhere — no selection, no clipboard — leaves an empty
// draft and a reason, which is what the panel says when it opens.
func TestCaptureWithNothingToReadSaysSoAndLeavesTheDraftEmpty(t *testing.T) {
	t.Parallel()

	windows := &fakeWindows{}

	got, err := readSource(windows.ports(), 0x1234, true, "ctrl+shift+c")
	if !errors.Is(err, errNothingCaptured) {
		t.Errorf("readSource returned (%q, %v), want errNothingCaptured", got, err)
	}
	if got != "" {
		t.Errorf("the draft opens with %q, want nothing to open with", got)
	}
}

// A capture that could not happen — the window would not come forward, the keys
// would not press — falls back to the clipboard and carries the reason, which
// the panel then says out loud.
func TestACaptureThatCouldNotHappenFallsBackToTheClipboard(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		which   func(*fakeWindows)
		wantErr string
	}{
		{
			name:    "the window would not come forward",
			which:   func(w *fakeWindows) { w.frontErr = errors.New("the window is gone") },
			wantErr: "the window is gone",
		},
		{
			name:    "the keys would not press",
			which:   func(w *fakeWindows) { w.pressErr = errors.New("the chord failed") },
			wantErr: "the chord failed",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			windows := &fakeWindows{clipboard: "still here", selection: "selected"}
			test.which(windows)

			got, err := readSource(windows.ports(), 0x1234, true, "ctrl+shift+c")
			if err == nil || !strings.Contains(err.Error(), test.wantErr) {
				t.Errorf("readSource returned (%q, %v), want the reason %q", got, err, test.wantErr)
			}
			if got != "still here" {
				t.Errorf("the draft opens with %q, want the clipboard as the fallback", got)
			}
			if windows.clipboard != "still here" {
				t.Errorf("the clipboard holds %q, want it never touched", windows.clipboard)
			}
		})
	}
}

// A chord that names no key is caught before anything is moved: a setting nobody
// checked must not bring a window forward just to discover the mistake there.
func TestTheChordIsCheckedBeforeTheWindowIsMoved(t *testing.T) {
	t.Parallel()

	windows := &fakeWindows{clipboard: "kept", selection: "selected"}

	got, err := readSource(windows.ports(), 0x1234, true, "definitely-not-a-key")
	if err == nil {
		t.Fatal("an impossible chord was accepted")
	}
	if got != "kept" {
		t.Errorf("the draft opens with %q, want the clipboard", got)
	}
	if !equal(windows.events, []string{"read"}) {
		t.Errorf("the capture reached as far as %v, want the clipboard read alone",
			windows.events)
	}
}

// Plain --read is exactly what it sounds like: the clipboard as it stands,
// with nothing pressed into anything.
func TestWithoutCaptureTheClipboardIsOpenedWithAsItIs(t *testing.T) {
	t.Parallel()

	windows := &fakeWindows{clipboard: "already on the clipboard"}

	got, err := readSource(windows.ports(), 0x1234, false, "ctrl+shift+c")
	if err != nil {
		t.Fatalf("reading the clipboard returned an error: %v", err)
	}
	if got != "already on the clipboard" {
		t.Errorf("the draft opens with %q, want the clipboard", got)
	}
	if !equal(windows.events, []string{"read"}) {
		t.Errorf("plain --read touched %v, want the clipboard read alone", windows.events)
	}
}

// The copy at the end of read mode is a paste away from wherever it belongs,
// so it goes where pastes go: the clipboard.
func TestTheResultIsCopiedWhereAPasteTakesIt(t *testing.T) {
	t.Parallel()

	previous := win32.ClipboardText()
	t.Cleanup(func() { _ = win32.SetClipboardText(previous) })

	const result = "修复失败的测试"
	if err := (copying{}).Insert(context.Background(), result); err != nil {
		t.Fatalf("copying the result: %v", err)
	}
	if win32.ClipboardText() != result {
		t.Errorf("the clipboard holds %q, want the result", win32.ClipboardText())
	}
}

func equal(got, want []string) bool {
	if len(got) != len(want) {
		return false
	}
	for index := range got {
		if got[index] != want[index] {
			return false
		}
	}
	return true
}
