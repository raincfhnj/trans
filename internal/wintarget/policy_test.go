package wintarget

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"trans/internal/win32"
)

// fakeSnapshot is what a fake clipboard was asked to put back: whether it held
// anything, and how often it was restored. The real one is memory on the
// clipboard, which a test has no business touching.
type fakeSnapshot struct {
	holds      bool
	restored   int
	restoreErr error
}

func (fake *fakeSnapshot) Restore() error {
	fake.restored++
	return fake.restoreErr
}

func (fake *fakeSnapshot) Holds() bool { return fake.holds }

// probe is a delivery over seams that record what they were asked to do, so
// that the order of a delivery can be checked and not only its result. The
// clipboard here is the snapshot alone: what a delivery does with it — take it,
// write the prompt, put it back — is what these tests are about.
type probe struct {
	calls []string

	clipboard    *fakeSnapshot
	snapshotErr  error
	activateErr  error
	comesForward bool
	losesFocus   bool
	writeErr     error
	pasteErr     error
	// pauseErr is what the named wait answers with, which is how a wait that
	// cannot happen — a cancelled context under it — is written down.
	pauseErr  error
	pasteSent int
	// pasted is the chord the paste seam was handed, which is what the setting
	// TRANS_PASTE_KEYS decides.
	pasted string
	idle   bool
	// cancelDuringFocus makes the context done inside the focus wait, which is
	// where the panel's escape arrives in practice.
	cancelDuringFocus context.CancelFunc
	// cancelDuringPaste does the same at the moment the chord goes out, which
	// is the one cancellation that reaches the paste seam itself.
	cancelDuringPaste context.CancelFunc
}

func newProbe() *probe {
	return &probe{
		clipboard:    &fakeSnapshot{holds: true},
		comesForward: true,
		pasteSent:    2,
		idle:         true,
	}
}

func (p *probe) seams() seams {
	return seams{
		snapshot: func() (win32.ClipboardSnapshot, error) {
			p.calls = append(p.calls, "snapshot")
			if p.snapshotErr != nil {
				return nil, p.snapshotErr
			}
			return p.clipboard, nil
		},
		restore: func(win32.ClipboardSnapshot) error {
			p.calls = append(p.calls, "restore")
			return p.clipboard.Restore()
		},
		release: func(win32.ClipboardSnapshot) {
			p.calls = append(p.calls, "release")
		},
		activate: func(uintptr) error {
			p.calls = append(p.calls, "activate")
			return p.activateErr
		},
		foreground: func(uintptr) bool {
			p.calls = append(p.calls, "foreground")
			return !p.losesFocus
		},
		waitFocus: func(context.Context, uintptr) bool {
			p.calls = append(p.calls, "wait-focus")
			if p.cancelDuringFocus != nil {
				p.cancelDuringFocus()
			}
			return p.comesForward
		},
		waitIdle: func(context.Context, uintptr) bool {
			p.calls = append(p.calls, "wait-idle")
			return p.idle
		},
		write: func(string) error {
			p.calls = append(p.calls, "write")
			return p.writeErr
		},
		pause: func(context.Context, time.Duration) error {
			p.calls = append(p.calls, "pause")
			return p.pauseErr
		},
		paste: func(ctx context.Context, chord string) (int, error) {
			p.calls = append(p.calls, "paste")
			p.pasted = chord
			// The Windows seam asks the context first, and this one does too:
			// a delivery cancelled while the chord was going out has to be
			// answered the way the real one answers it, or the test is of a
			// cancellation that cannot happen.
			if p.cancelDuringPaste != nil {
				p.cancelDuringPaste()
			}
			if err := ctx.Err(); err != nil {
				return 0, err
			}
			if p.pasteErr != nil {
				return 0, p.pasteErr
			}
			return p.pasteSent, nil
		},
		submit: func(context.Context) {
			p.calls = append(p.calls, "submit")
		},
	}
}

func (p *probe) called(step string) bool {
	for _, call := range p.calls {
		if call == step {
			return true
		}
	}
	return false
}

// expectCalls pins the whole order of a delivery, which is the part of it that
// the fixed sleeps used to hide: a paste before the window is in front, or a
// return before the paste, is what the order is here to catch.
func expectCalls(t *testing.T, probe *probe, want ...string) {
	t.Helper()
	if len(probe.calls) != len(want) {
		t.Fatalf("the delivery called %v, want %v", probe.calls, want)
	}
	for index := range want {
		if probe.calls[index] != want[index] {
			t.Fatalf("the delivery called %v, want %v", probe.calls, want)
		}
	}
}

// seamPointer hands the delivery the table of seams a probe records into, which
// is the same table the Windows side builds. The delivery takes it by pointer
// because it is a table of functions rather than a thing.
func seamPointer(p *probe) *seams {
	table := p.seams()
	return &table
}

// A send pastes the prompt and then presses return, in that order.
func TestASendPastesAndThenPressesReturn(t *testing.T) {
	t.Parallel()

	probe := newProbe()
	if err := deliverWith(context.Background(), seamPointer(probe), 0x1234, "ctrl+v", "a prompt", true); err != nil {
		t.Fatalf("deliverWith: %v", err)
	}
	expectCalls(t, probe,
		"snapshot", "activate", "wait-focus", "foreground", "write", "paste",
		"wait-idle", "foreground", "submit", "pause", "restore", "release")
}

// The chord that pastes is the one the author's terminal takes, and it is
// handed on as it was given: a delivery that pasted with a chord of its own
// choosing would type into panes that never agreed to it.
func TestThePasteChordIsTheOneTheDeliveryWasGiven(t *testing.T) {
	t.Parallel()

	probe := newProbe()
	if err := deliverWith(context.Background(), seamPointer(probe), 0x1234,
		"ctrl+shift+v", "a prompt", true); err != nil {
		t.Fatalf("deliverWith: %v", err)
	}
	if probe.pasted != "ctrl+shift+v" {
		t.Errorf("pasted with %q, want the chord the delivery was given", probe.pasted)
	}
}

// Typing pastes the prompt and leaves the last keystroke to the author: the
// return key is what the review action is for.
func TestTypingPastesWithoutPressingReturn(t *testing.T) {
	t.Parallel()

	probe := newProbe()
	if err := deliverWith(context.Background(), seamPointer(probe), 0x1234, "ctrl+v", "a prompt", false); err != nil {
		t.Fatalf("deliverWith: %v", err)
	}
	expectCalls(t, probe,
		"snapshot", "activate", "wait-focus", "foreground", "write", "paste",
		"pause", "restore", "release")
}

// A target that never comes forward is an error and nothing is pasted: a key
// sent now would land in whatever has the keyboard.
func TestATargetThatNeverComesForwardIsNotPastedInto(t *testing.T) {
	t.Parallel()

	probe := newProbe()
	probe.comesForward = false

	err := deliverWith(context.Background(), seamPointer(probe), 0x1234, "ctrl+v", "a prompt", true)
	if !errors.Is(err, errLostFocus) {
		t.Fatalf("deliverWith answered %v, want the lost-focus error", err)
	}
	if probe.called("write") || probe.called("paste") || probe.called("submit") {
		t.Errorf("the delivery did %v, want nothing written or pasted", probe.calls)
	}
}

// The window in front can be taken between the wait and the write, so it is
// asked about again; a delivery that finds it gone does not write the prompt.
func TestAWindowLostBeforeThePasteIsNotWrittenTo(t *testing.T) {
	t.Parallel()

	probe := newProbe()
	probe.losesFocus = true

	err := deliverWith(context.Background(), seamPointer(probe), 0x1234, "ctrl+v", "a prompt", true)
	if !errors.Is(err, errLostFocus) {
		t.Fatalf("deliverWith answered %v, want the lost-focus error", err)
	}
	if probe.called("write") || probe.called("paste") {
		t.Errorf("the delivery did %v, want nothing written or pasted", probe.calls)
	}
}

// Focus lost after the paste is its own answer: the text is in the window and
// only the return is missing, and a message that said nothing about the text
// would leave the author looking for it.
func TestFocusLostAfterThePasteSaysWhereTheTextIs(t *testing.T) {
	t.Parallel()

	probe := newProbe()
	// The focus is asked about twice: once before the paste — where it is still
	// there — and once before the return, where it has gone.
	lostAfterPaste := false
	base := seamPointer(probe)
	base.foreground = func(uintptr) bool {
		probe.calls = append(probe.calls, "foreground")
		if probe.called("paste") {
			lostAfterPaste = true
		}
		return !lostAfterPaste
	}

	err := deliverWith(context.Background(), base, 0x1234, "ctrl+v", "a prompt", true)
	if !errors.Is(err, errPastedLate) {
		t.Fatalf("deliverWith answered %v, want the error that says the text was not submitted", err)
	}
	if !strings.Contains(err.Error(), "the text is in the window but was not submitted") {
		t.Errorf("the error reads %q, want it to say where the text is", err)
	}
	if !probe.called("paste") || probe.called("submit") {
		t.Errorf("the delivery did %v, want the paste and no return", probe.calls)
	}
	if probe.clipboard.restored == 0 {
		t.Error("the clipboard was not put back after a paste that could not be submitted")
	}
}

// A clipboard holding something that cannot be put back is not written over:
// the delivery refuses and says so, and the chord is never pressed.
func TestAClipboardThatCannotBeSnapshottedRefusesTheDelivery(t *testing.T) {
	t.Parallel()

	probe := newProbe()
	probe.snapshotErr = errors.New("the clipboard holds a picture that cannot be copied, so it was not overwritten")

	err := deliverWith(context.Background(), seamPointer(probe), 0x1234, "ctrl+v", "a prompt", true)
	if err == nil {
		t.Fatal("deliverWith answered no error for a clipboard it could not put back")
	}
	if !strings.Contains(err.Error(), "cannot be copied") {
		t.Errorf("the error reads %q, want it to say what happened to the clipboard", err)
	}
	if len(probe.calls) != 1 || probe.calls[0] != "snapshot" {
		t.Errorf("the delivery did %v, want nothing after the snapshot was refused", probe.calls)
	}
}

// Everything that happens once the prompt is on the clipboard puts the
// clipboard back, whichever path it takes: a screenshot must not be lost to a
// delivery that then failed.
func TestTheClipboardGoesBackOnEveryPathOnceItWasTouched(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name    string
		prepare func(*probe)
		submit  bool
	}{
		{name: "sent", prepare: func(*probe) {}, submit: true},
		{name: "typed only", prepare: func(*probe) {}, submit: false},
		{name: "the write failed", prepare: func(p *probe) { p.writeErr = errors.New("the clipboard is held by something else") }},
		{name: "the paste was refused", prepare: func(p *probe) { p.pasteErr = errors.New("no such key") }},
		{
			name: "the target was taken by something else",
			prepare: func(p *probe) {
				p.idle = false
				p.losesFocus = true
			},
			submit: true,
		},
	}

	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			probe := newProbe()
			test.prepare(probe)

			_ = deliverWith(context.Background(), seamPointer(probe), 0x1234, "ctrl+v", "a prompt", test.submit)

			if probe.clipboard.restored != 1 {
				t.Errorf("the clipboard was put back %d times, want once on every path", probe.clipboard.restored)
			}
			if !probe.called("release") {
				t.Error("the snapshot was not released")
			}
		})
	}
}

// A cancel is not a delivery: what is on the clipboard goes back and nothing is
// pasted or pressed, which is what pressing escape in the panel asks for.
func TestACancelledDeliveryStopsAndPutsTheClipboardBack(t *testing.T) {
	t.Parallel()

	probe := newProbe()
	ctx, cancel := context.WithCancel(context.Background())
	probe.cancelDuringFocus = cancel
	// The escape stands in for the wait being cut short: the context is done,
	// so the wait answers that the window never came forward.
	probe.comesForward = false

	err := deliverWith(ctx, seamPointer(probe), 0x1234, "ctrl+v", "a prompt", true)
	if err == nil {
		t.Fatal("deliverWith answered no error for a cancelled delivery")
	}
	if probe.called("write") || probe.called("paste") || probe.called("submit") {
		// The focus wait is what was cancelled, so the write is never reached.
		t.Errorf("the delivery did %v, want a cancelled delivery to stop there", probe.calls)
	}
	if probe.clipboard.restored == 0 {
		t.Error("the clipboard was not put back after a cancelled delivery")
	}
}

// A delivery with no window and one with nothing to say are refused before the
// clipboard is touched at all.
func TestADeliveryWithNothingToDoTouchesNothing(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name   string
		handle uintptr
		text   string
		want   error
	}{
		{name: "no window", handle: 0, text: "a prompt", want: errNoWindow},
		{name: "nothing to deliver", handle: 0x1234, text: "", want: errNoText},
	}

	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			probe := newProbe()
			err := deliverWith(context.Background(), seamPointer(probe), test.handle, "ctrl+v", test.text, true)
			if !errors.Is(err, test.want) {
				t.Fatalf("deliverWith answered %v, want %v", err, test.want)
			}
			if len(probe.calls) != 0 {
				t.Errorf("the delivery did %v, want nothing at all", probe.calls)
			}
		})
	}
}

// A delivery that was already cancelled touches nothing: the panel closed
// before any of this began, and the clipboard is not this program's to borrow.
func TestADeliveryThatWasAlreadyCancelledTouchesNothing(t *testing.T) {
	t.Parallel()

	probe := newProbe()
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	err := deliverWith(ctx, seamPointer(probe), 0x1234, "ctrl+v", "a prompt", true)

	if !errors.Is(err, context.Canceled) {
		t.Errorf("the delivery answered %v, want the cancellation", err)
	}
	if len(probe.calls) != 0 {
		t.Errorf("a cancelled delivery did %v, want it to touch nothing", probe.calls)
	}
}

// A window that cannot be brought forward stops the delivery before anything is
// written: a key sent now would land in whatever has the keyboard.
func TestAnActivationThatFailsStopsBeforeAnythingIsWritten(t *testing.T) {
	t.Parallel()

	probe := newProbe()
	refused := errors.New("the window could not be brought forward")
	probe.activateErr = refused

	err := deliverWith(context.Background(), seamPointer(probe), 0x1234, "ctrl+v", "a prompt", true)

	if !errors.Is(err, refused) {
		t.Errorf("the delivery answered %v, want the failure to activate", err)
	}
	if probe.called("write") || probe.called("paste") || probe.called("submit") {
		t.Errorf("the delivery did %v after failing to bring the window forward", probe.calls)
	}
}

// A wait the panel cancelled is the panel's own answer, not a window that would
// not come forward: saying "lost focus" would report something that never
// happened.
func TestACancellationDuringTheFocusWaitIsNotALostFocus(t *testing.T) {
	t.Parallel()

	probe := newProbe()
	probe.comesForward = false
	ctx, cancel := context.WithCancel(context.Background())
	probe.cancelDuringFocus = cancel

	err := deliverWith(ctx, seamPointer(probe), 0x1234, "ctrl+v", "a prompt", true)

	if !errors.Is(err, context.Canceled) {
		t.Errorf("the delivery answered %v, want the cancellation", err)
	}
	if errors.Is(err, errLostFocus) {
		t.Error("a cancelled wait was reported as a window that lost focus")
	}
	if probe.called("write") {
		t.Errorf("the delivery did %v after the panel cancelled it", probe.calls)
	}
}

// A wait that refuses stops the delivery rather than pressing return into a
// window that was never settled.
func TestAPauseThatRefusesStopsTheDelivery(t *testing.T) {
	t.Parallel()

	probe := newProbe()
	probe.idle = false // so the delivery waits for the pane to finish reading
	refused := errors.New("the wait was cancelled")
	probe.pauseErr = refused

	err := deliverWith(context.Background(), seamPointer(probe), 0x1234, "ctrl+v", "a prompt", true)

	if !errors.Is(err, refused) {
		t.Errorf("the delivery answered %v, want the refused wait", err)
	}
	if probe.called("submit") {
		t.Errorf("the return was pressed after a refused wait: %v", probe.calls)
	}
}

// An empty clipboard has nothing to put back, so the delivery does not open the
// clipboard a second time to put nothing on it.
func TestAnEmptyClipboardIsNotRestored(t *testing.T) {
	t.Parallel()

	probe := newProbe()
	probe.clipboard.holds = false

	if err := deliverWith(context.Background(), seamPointer(probe), 0x1234, "ctrl+v", "a prompt", true); err != nil {
		t.Fatalf("deliverWith: %v", err)
	}
	expectCalls(t, probe,
		"snapshot", "activate", "wait-focus", "foreground", "write", "paste",
		"wait-idle", "foreground", "submit", "release")
}

// A failed restore is not a failed delivery: what the author asked for happened,
// and the trouble with their clipboard is written down rather than thrown back
// at them as if the prompt had not gone.
func TestAFailedRestoreDoesNotFailTheDelivery(t *testing.T) {
	t.Parallel()

	probe := newProbe()
	probe.clipboard.restoreErr = errors.New("the clipboard is held by something else")

	if err := deliverWith(context.Background(), seamPointer(probe), 0x1234, "ctrl+v", "a prompt", true); err != nil {
		t.Fatalf("deliverWith answered %v, want the delivery to be reported as done", err)
	}
	if !probe.called("submit") {
		t.Error("the return key was not pressed")
	}
}

// Keys Windows took none of are said plainly: the prompt is on the clipboard
// and the author can paste it themselves.
func TestAPasteWindowsTookNoneOfIsReported(t *testing.T) {
	t.Parallel()

	probe := newProbe()
	probe.pasteSent = 0

	err := deliverWith(context.Background(), seamPointer(probe), 0x1234, "ctrl+v", "a prompt", true)
	if err == nil {
		t.Fatal("deliverWith answered no error when Windows took none of the key events")
	}
	if !strings.Contains(err.Error(), "the prompt is on the clipboard") {
		t.Errorf("the error reads %q, want it to say the prompt is on the clipboard", err)
	}
	if probe.called("submit") {
		t.Error("the return key was pressed after a paste that did not happen")
	}
}

// A delivery the panel cancelled as the chord went out is not a paste Windows
// refused. The clipboard is put back below, so the words "the prompt is on the
// clipboard" would be untrue as well as unhelpful, and an author who pressed
// escape wants that read back as what they did.
func TestACancellationAtThePasteSaysItWasCancelled(t *testing.T) {
	t.Parallel()

	probe := newProbe()
	ctx, cancel := context.WithCancel(context.Background())
	probe.cancelDuringPaste = cancel

	err := deliverWith(ctx, seamPointer(probe), 0x1234, "ctrl+v", "a prompt", true)

	if !errors.Is(err, context.Canceled) {
		t.Errorf("the delivery answered %v, want the cancellation", err)
	}
	if strings.Contains(err.Error(), "the prompt is on the clipboard") {
		t.Errorf("a cancelled delivery said %q, want it not to claim the prompt is on the clipboard", err)
	}
	if probe.clipboard.restored == 0 {
		t.Error("the clipboard was not put back after a cancelled delivery")
	}
	if probe.called("submit") {
		t.Errorf("the return was pressed after a cancelled paste: %v", probe.calls)
	}
}
