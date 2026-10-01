package win32

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"
)

// fakeClipboard is a clipboard a test can drive: what it holds, what it was
// asked to write, and how often it was put back. It stands in for the Windows
// clipboard, which needs a desktop session, so that every path a capture takes
// over it is checked on any machine.
type fakeClipboard struct {
	holds      string
	written    []string
	restored   int
	refuseRead bool
	// snapshots counts how many times the clipboard was put to one side, and
	// refuseLater refuses only from the second one on. That second snapshot is
	// the token the capture writes, and it is the case where the clipboard can
	// be read but not written over.
	snapshots   int
	refuseLater bool
	writeErr    error
}

func (fake *fakeClipboard) read() string { return fake.holds }

func (fake *fakeClipboard) write(text string) error {
	if fake.writeErr != nil {
		return fake.writeErr
	}
	fake.written = append(fake.written, text)
	fake.holds = text
	return nil
}

func (fake *fakeClipboard) snapshot() (ClipboardSnapshot, error) {
	fake.snapshots++
	if fake.refuseRead || (fake.refuseLater && fake.snapshots > 1) {
		return nil, errors.New("the clipboard holds a picture that cannot be copied, so it was not overwritten")
	}
	return &fakeSnapshot{holds: fake.holds != ""}, nil
}

func (fake *fakeClipboard) restore(ClipboardSnapshot) error {
	fake.restored++
	fake.holds = ""
	return nil
}

func (fake *fakeClipboard) release(ClipboardSnapshot) {}

// fakeSnapshot is what one snapshot of the fake clipboard was: whether there
// was anything on it, which is what decides whether a token is written.
type fakeSnapshot struct{ holds bool }

func (snapshot *fakeSnapshot) Restore() error { return nil }
func (snapshot *fakeSnapshot) Holds() bool    { return snapshot.holds }
func (snapshot *fakeSnapshot) Release()       {}

// captureProbe is a capture over the fake clipboard and a record of every call
// it made, so that the order of them can be checked and not only their effect.
// The pane either writes the clipboard when the chord is pressed — which is
// what moves the sequence number — or does not.
type captureProbe struct {
	fake     *fakeClipboard
	calls    []string
	copies   bool
	chordErr error
	pressed  string
	// fronted is the handle the capture brought forward, which is the window
	// the chord was pressed in.
	fronted uintptr
}

func (probe *captureProbe) ports() selectionPorts {
	return selectionPorts{
		read:     probe.fake.read,
		write:    probe.fake.write,
		snapshot: probe.fake.snapshot,
		restore:  probe.fake.restore,
		release:  probe.fake.release,
		front: func(handle uintptr) error {
			probe.calls = append(probe.calls, "front")
			probe.fronted = handle
			return nil
		},
		copyKeys: func(_ context.Context, chord string) error {
			probe.calls = append(probe.calls, "chord")
			probe.pressed = chord
			if probe.chordErr != nil {
				return probe.chordErr
			}
			if probe.copies {
				probe.fake.holds = "what the pane copied"
			}
			return nil
		},
		waitCopy: func(context.Context) bool {
			probe.calls = append(probe.calls, "wait-copy")
			return probe.copies
		},
		waitSettle: func(context.Context, time.Duration) error { return nil },
	}
}

// A capture with a chord puts a token on the clipboard, brings the pane
// forward, presses the chord, waits for the clipboard to move, and reads back
// what the pane copied into it.
func TestACaptureReadsWhatThePaneCopied(t *testing.T) {
	t.Parallel()

	fake := &fakeClipboard{holds: "what was there before"}
	probe := &captureProbe{fake: fake, copies: true}

	selection, err := captureSelection(context.Background(), probe.ports(), 0x1234, "ctrl+shift+c")
	if err != nil {
		t.Fatalf("captureSelection: %v", err)
	}
	if selection != "what the pane copied" {
		t.Errorf("captured %q, want the text the pane copied", selection)
	}
	if len(fake.written) != 1 || !strings.HasPrefix(fake.written[0], "trans-selection-") {
		t.Errorf("the clipboard was written %q, want the capture's own token", fake.written)
	}
	if fake.restored == 0 {
		t.Error("the clipboard was never put back: the token would stay on it")
	}
	if want := []string{"front", "chord", "wait-copy"}; !equalCalls(probe.calls, want) {
		t.Errorf("the capture called %v, want %v", probe.calls, want)
	}
}

// The handle a capture is given is the window the chord was pressed in, and it
// is that window the pane is brought forward in: a capture that brought
// anything else forward would copy out of whatever happened to be in front.
func TestACaptureBringsForwardTheWindowItWasGiven(t *testing.T) {
	t.Parallel()

	fake := &fakeClipboard{holds: "what was there before"}
	probe := &captureProbe{fake: fake, copies: true}

	if _, err := captureSelection(context.Background(), probe.ports(), 0xfeed, "ctrl+shift+c"); err != nil {
		t.Fatalf("captureSelection: %v", err)
	}
	if probe.fronted != 0xfeed {
		t.Errorf("brought forward %#x, want the window the capture was given", probe.fronted)
	}
}

// A pane that copies back the very text the clipboard held is the case the
// token exists for: a plain before-and-after could not tell that from a copy
// which did not happen at all.
func TestASelectionJustLikeTheClipboardIsStillASelection(t *testing.T) {
	t.Parallel()

	// The clipboard holds what the pane will copy, and the pane copies it: the
	// mark was written over, so what comes back is a selection even though it
	// matches what was there before.
	fake := &fakeClipboard{holds: "the same text"}
	probe := &captureProbe{fake: fake, copies: true}
	ports := probe.ports()
	ports.read = func() string { return "the same text" }

	selection, err := captureSelection(context.Background(), ports, 0x1234, "ctrl+shift+c")
	if err != nil {
		t.Fatalf("captureSelection: %v", err)
	}
	if selection != "the same text" {
		t.Errorf("captured %q, want the text the pane copied", selection)
	}
}

// The token still standing afterwards is what says nothing was selected, and a
// token is never a selection to translate.
func TestAMarkStillStandingMeansNothingWasSelected(t *testing.T) {
	t.Parallel()

	fake := &fakeClipboard{holds: "what was there before"}
	probe := &captureProbe{fake: fake}
	ports := probe.ports()
	// Nothing selected: the pane never wrote, so the clipboard still holds the
	// token the capture put there.
	ports.read = func() string { return fake.written[len(fake.written)-1] }

	selection, err := captureSelection(context.Background(), ports, 0x1234, "ctrl+shift+c")
	if err != nil {
		t.Fatalf("captureSelection: %v", err)
	}
	if selection != "" {
		t.Errorf("captured %q, want nothing when the token came back", selection)
	}
	if fake.restored == 0 {
		t.Error("the token was left on the clipboard")
	}
}

// The clipboard goes back as it was when the chord never went out: the token is
// the capture's, and it must not be left behind.
func TestTheClipboardGoesBackWhenTheChordFails(t *testing.T) {
	t.Parallel()

	fake := &fakeClipboard{holds: "what was there before"}
	probe := &captureProbe{fake: fake, chordErr: errors.New("no such key")}

	_, err := captureSelection(context.Background(), probe.ports(), 0x1234, "ctrl+nonsense")
	if err == nil {
		t.Fatal("captureSelection answered no error for a chord that could not be pressed")
	}
	if fake.restored == 0 {
		t.Error("the token was left on the clipboard")
	}
	if !strings.Contains(err.Error(), "pressing ctrl+nonsense to copy the selection") {
		t.Errorf("the error reads %q, want it to say which chord was pressed", err)
	}
}

// A clipboard that holds something which cannot be put back is never written
// over: the panel loses the selection rather than the author losing a picture,
// and it says so.
func TestAClipboardThatCannotBeSnapshottedIsLeftAlone(t *testing.T) {
	t.Parallel()

	fake := &fakeClipboard{holds: "a picture", refuseRead: true}
	probe := &captureProbe{fake: fake}

	selection, err := captureSelection(context.Background(), probe.ports(), 0x1234, "ctrl+shift+c")
	if err == nil {
		t.Fatal("captureSelection answered no error for a clipboard it cannot put back")
	}
	if !strings.Contains(err.Error(), "cannot be copied") {
		t.Errorf("the error reads %q, want it to say the clipboard could not be put back", err)
	}
	if selection != "" {
		t.Errorf("captured %q, want no selection from a refused capture", selection)
	}
	if len(fake.written) != 0 {
		t.Errorf("the clipboard was written %q, want it left alone", fake.written)
	}
	if probe.pressed != "" {
		t.Errorf("the chord %q was pressed, want nothing pressed for a refused capture", probe.pressed)
	}
}

// An empty clipboard has no token to write and nothing to put back, so whatever
// is read after the chord is what the pane copied.
func TestAnEmptyClipboardIsReadWithoutAToken(t *testing.T) {
	t.Parallel()

	fake := &fakeClipboard{}
	probe := &captureProbe{fake: fake, copies: true}
	ports := probe.ports()
	ports.read = func() string { return "what the pane copied" }

	selection, err := captureSelection(context.Background(), ports, 0x1234, "ctrl+shift+c")
	if err != nil {
		t.Fatalf("captureSelection: %v", err)
	}
	if selection != "what the pane copied" {
		t.Errorf("captured %q, want the text the pane copied", selection)
	}
	if len(fake.written) != 0 {
		t.Errorf("the clipboard was written %q, want no token on an empty clipboard", fake.written)
	}
}

// A capture with no chord reads the clipboard as it stands: a terminal that
// copies on selection has already put the selection there, and nothing about
// the clipboard is touched.
func TestACaptureWithoutAChordReadsTheClipboardAsItStands(t *testing.T) {
	t.Parallel()

	fake := &fakeClipboard{holds: "already on the clipboard"}
	probe := &captureProbe{fake: fake}

	selection, err := captureSelection(context.Background(), probe.ports(), 0x1234, "")
	if err != nil {
		t.Fatalf("captureSelection: %v", err)
	}
	if selection != "already on the clipboard" {
		t.Errorf("captured %q, want what the clipboard held", selection)
	}
	if len(fake.written) != 0 || probe.pressed != "" {
		t.Error("a capture without a chord wrote the clipboard or pressed a key")
	}
}

// A clipboard that can be read but not written over — a format it holds on
// demand, which GetClipboardData answers nothing for — is refused at the token
// rather than at the snapshot, and nothing is pressed into the pane for it.
func TestAClipboardThatCannotBeMarkedIsRefused(t *testing.T) {
	t.Parallel()

	fake := &fakeClipboard{holds: "what was there before", refuseLater: true}
	probe := &captureProbe{fake: fake}

	_, err := captureSelection(context.Background(), probe.ports(), 0x1234, "ctrl+shift+c")
	if err == nil {
		t.Fatal("captureSelection answered no error for a clipboard it could not mark")
	}
	if !strings.Contains(err.Error(), "cannot be marked for the selection") {
		t.Errorf("the error reads %q, want it to say the clipboard could not be marked", err)
	}
	if len(fake.written) != 0 || probe.pressed != "" {
		t.Errorf("the capture wrote %q and pressed %q, want nothing done for a clipboard it cannot mark",
			fake.written, probe.pressed)
	}
	if fake.restored != 1 {
		t.Errorf("the clipboard was put back %d times, want the one it was put to one side with", fake.restored)
	}
}

// A token that cannot be written is answered the same way as a snapshot that
// could not be taken, and the chord is never pressed: a mark that is not there
// would read as a copy that brought the pane's own token back.
func TestATokenThatCannotBeWrittenStopsTheCapture(t *testing.T) {
	t.Parallel()

	fake := &fakeClipboard{holds: "what was there before", writeErr: errors.New("the clipboard is held by something else")}
	probe := &captureProbe{fake: fake}

	_, err := captureSelection(context.Background(), probe.ports(), 0x1234, "ctrl+shift+c")
	if err == nil {
		t.Fatal("captureSelection answered no error for a token it could not write")
	}
	if !strings.Contains(err.Error(), "marking the clipboard for the selection") {
		t.Errorf("the error reads %q, want it to say the clipboard could not be marked", err)
	}
	if probe.pressed != "" {
		t.Errorf("the chord %q was pressed, want nothing pressed when the token is not on the clipboard", probe.pressed)
	}
	if fake.restored == 0 {
		t.Error("the clipboard was not put back")
	}
}

// equalCalls compares two recorded call sequences, which is how the order of a
// capture's steps is pinned rather than only its result.
func equalCalls(got, want []string) bool {
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
