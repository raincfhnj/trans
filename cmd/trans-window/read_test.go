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

// fakeWindows is the capture's touches of the world outside this process,
// played by a machine of our own: the clipboard, the window coming forward, the
// chord, and the waiting in between. Pressing the chord copies the selection
// onto the clipboard, the way a real copy would.
type fakeWindows struct {
	clipboard string
	selection string
	events    []string
	frontErr  error
	pressErr  error
	// snapshotErr is a clipboard that cannot be kept to one side at all —
	// something on it that cannot be copied back — and restoreErr one that will
	// not take what was kept back.
	snapshotErr error
	restoreErr  error
}

func (w *fakeWindows) ports() capturePorts {
	return capturePorts{
		read: func() string {
			w.events = append(w.events, "read")
			return w.clipboard
		},
		snapshot: func() (win32.ClipboardSnapshot, error) {
			w.events = append(w.events, "snapshot")
			if w.snapshotErr != nil {
				return nil, w.snapshotErr
			}
			return &fakeSnapshot{windows: w, held: w.clipboard}, nil
		},
		restore: func(snapshot win32.ClipboardSnapshot) error {
			w.events = append(w.events, "restore")
			return snapshot.Restore()
		},
		release: func(win32.ClipboardSnapshot) {
			w.events = append(w.events, "release")
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

// fakeSnapshot is the clipboard kept to one side: what it held when it was
// taken, and whether the clipboard will take it back.
type fakeSnapshot struct {
	windows *fakeWindows
	held    string
}

func (s *fakeSnapshot) Restore() error {
	if s.windows.restoreErr != nil {
		return s.windows.restoreErr
	}
	s.windows.clipboard = s.held
	return nil
}

func (s *fakeSnapshot) Holds() bool { return s.held != "" }

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

	want := []string{"read", "snapshot", "front", "wait", "press", "wait", "read", "restore", "release"}
	if !equal(windows.events, want) {
		t.Errorf("the capture happened as %v, want %v", windows.events, want)
	}
}

// A clipboard whose content cannot be copied back — a screenshot, a set of
// files — is not written over at all. The capture refuses before anything is
// pressed: a read that did not happen costs a read, and the author's screenshot
// is gone for good.
func TestAClipboardThatCannotBeKeptIsNotWrittenOver(t *testing.T) {
	t.Parallel()

	windows := &fakeWindows{
		clipboard:   "what a picture reads as",
		selection:   "the line that is selected",
		snapshotErr: errors.New("the clipboard holds a picture that cannot be copied back"),
	}

	got, err := windows.ports().selection(0x1234, "ctrl+shift+c")
	if err == nil {
		t.Fatalf("the capture went ahead over a clipboard it could not keep, opening with %q", got)
	}
	if windows.clipboard != "what a picture reads as" {
		t.Errorf("the clipboard holds %q, want it untouched", windows.clipboard)
	}
	if !equal(windows.events, []string{"read", "snapshot"}) {
		t.Errorf("the capture reached as far as %v, want it stopped at the snapshot",
			windows.events)
	}
}

// A clipboard that will not take its own content back is not worth failing a
// capture over: the selection was read out of it, and the author still has a
// draft to work with.
func TestAClipboardThatWillNotTakeItsContentBackStillOpensTheDraft(t *testing.T) {
	t.Parallel()

	windows := &fakeWindows{
		clipboard:  "a note someone kept",
		selection:  "the line that is selected",
		restoreErr: errors.New("the clipboard is held by something else"),
	}

	got, err := windows.ports().selection(0x1234, "ctrl+shift+c")
	if err != nil {
		t.Fatalf("a clipboard that would not take its content back failed the capture: %v", err)
	}
	if got != windows.selection {
		t.Errorf("the draft opens with %q, want the selection %q", got, windows.selection)
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
		// wantEvents is how far the capture got, and it ends with the clipboard
		// put back even though the capture failed: a chord that reports a
		// failure may still have copied into it. The last read is the fallback
		// the draft opens with.
		wantEvents []string
	}{
		{
			name:       "the window would not come forward",
			which:      func(w *fakeWindows) { w.frontErr = errors.New("the window is gone") },
			wantErr:    "the window is gone",
			wantEvents: []string{"read", "snapshot", "front", "restore", "release", "read"},
		},
		{
			name:       "the keys would not press",
			which:      func(w *fakeWindows) { w.pressErr = errors.New("the chord failed") },
			wantErr:    "the chord failed",
			wantEvents: []string{"read", "snapshot", "front", "wait", "press", "restore", "release", "read"},
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
			if !equal(windows.events, test.wantEvents) {
				t.Errorf("the capture happened as %v, want %v", windows.events, test.wantEvents)
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
// so it goes where pastes go: the clipboard. What was on it is kept whole and
// put back, and a clipboard this test cannot put back is one it leaves alone —
// the clipboard is the whole machine's, and a test has no more right to empty
// someone's screenshot than the panel does.
//
// It is also the one test in the suite that drives the real clipboard, so it
// can meet another package's test doing the same: `go test ./...` runs packages
// in parallel, and the clipboard is one per desktop. A run that finds its own
// text gone says so and passes rather than failing on someone else's timing.
func TestTheResultIsCopiedWhereAPasteTakesIt(t *testing.T) {
	before, err := win32.SnapshotClipboard()
	if err != nil {
		t.Skipf("the clipboard holds something this test cannot put back: %v", err)
	}
	t.Cleanup(func() {
		if err := win32.RestoreClipboard(before); err != nil {
			t.Errorf("putting the clipboard back: %v", err)
		}
		win32.ReleaseClipboard(before)
	})

	const result = "修复失败的测试"
	if err := (clipboardTarget{}).Insert(context.Background(), result); err != nil {
		t.Fatalf("copying the result: %v", err)
	}
	if got := win32.ClipboardText(); got != result {
		t.Skipf("the clipboard holds %q rather than the result: another process is using it", got)
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
