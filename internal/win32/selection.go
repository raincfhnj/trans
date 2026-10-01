package win32

import (
	"context"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	"trans/internal/winlog"
)

// The waiting a capture does where it has no signal to wait for. The wait for
// the pane to write the clipboard is a signal and lives in
// WaitForClipboardChange; these two are the small, named sleeps that stand in
// for one, and they say what they are for.
const (
	// pasteChordSettle is the pause between bringing the pane forward and
	// pressing the copy chord. No call says that a window has settled: a window
	// that has just taken the keyboard can still be catching up, and a chord
	// pressed then is the one case that lands nowhere. Short enough not to be
	// noticed, long enough for the window manager to be done.
	pasteChordSettle = 40 * time.Millisecond
	// copyReadSettle is the pause between the pane writing the clipboard and
	// this reading it. GetClipboardSequenceNumber moves when the clipboard is
	// claimed, which is the moment before the write is finished; no call
	// reports the second moment.
	copyReadSettle = 20 * time.Millisecond
)

// selectionPorts is everything a capture touches outside this package: the
// clipboard, which is read and put back, the window that has to be in front,
// the keys pressed into it, the signal that the pane has copied, and the two
// waits that cover the moments nothing reports. A test stands in for Windows
// with its own.
type selectionPorts struct {
	read       func() string
	write      func(string) error
	snapshot   func() (ClipboardSnapshot, error)
	restore    func(ClipboardSnapshot) error
	release    func(ClipboardSnapshot)
	front      func(uintptr) error
	copyKeys   func(ctx context.Context, chord string) error
	waitCopy   func(ctx context.Context) bool
	waitSettle func(ctx context.Context, howLong time.Duration) error
}

// captureSelection runs one capture over the ports given, for the window given:
// that is where the chord was pressed, and a capture has no handle of its own to
// work from.
//
// With no chord the clipboard is read as it stands. With a chord the clipboard
// is put to one side, marked with a token, and copied into by the pane — a token
// still on the clipboard afterwards is what says nothing was selected, where a
// plain before-and-after could not tell a copy that did not happen from one that
// brought back the very text that was on the clipboard.
func captureSelection(ctx context.Context, ports selectionPorts, target uintptr, chord string) (string, error) {
	if strings.TrimSpace(chord) == "" {
		return ports.read(), nil
	}

	before, err := ports.snapshot()
	if err != nil {
		// The clipboard holds something that cannot be put back, so it is not
		// marked either: refusing the capture loses the selection, and marking
		// it would lose whatever the author had there.
		note("the selection was not read: " + err.Error())
		return "", err
	}
	defer ports.release(before)
	// From here on the clipboard goes back as it was whatever happens: it is
	// the author's, and this only borrows it. The mark, which is written after
	// this one is taken, is put back before it. A restore that fails has
	// nothing better to do than be written down — there is no second copy of
	// what was on the clipboard to try again with.
	defer func() { _ = ports.restore(before) }()

	guard, err := ports.snapshot()
	if err != nil {
		return "", refusingToMark(err)
	}
	defer ports.release(guard)
	defer func() { _ = ports.restore(guard) }()

	mark := ""
	if guard.Holds() {
		// The token goes on the clipboard before the chord, so that a copy that
		// does not happen leaves it there to be recognised. A pane that writes
		// the clipboard at all writes over it.
		mark = selectionMark()
		if err := ports.write(mark); err != nil {
			return "", fmt.Errorf("marking the clipboard for the selection: %w", err)
		}
	}

	if err := ports.front(target); err != nil {
		return "", err
	}
	if err := ports.waitSettle(ctx, pasteChordSettle); err != nil {
		return "", err
	}
	if err := ports.copyKeys(ctx, chord); err != nil {
		// The chord never went out, so the token it was sent with must not stay
		// behind either; the restore above takes it off.
		return "", fmt.Errorf("pressing %s to copy the selection: %w", chord, err)
	}

	// A pane writes the clipboard some time after the chord, and no call says
	// when: the sequence number moving is the closest thing to one, and it is
	// what is waited for rather than a flat sleep. A wait that finds no change
	// reads the clipboard anyway — the token is what says what happened.
	if !ports.waitCopy(ctx) {
		note("the pane did not copy " + chord + ": nothing was selected")
	}
	if err := ports.waitSettle(ctx, copyReadSettle); err != nil {
		return "", err
	}

	selection := ports.read()
	// Nothing was selected when the token is still there or the clipboard came
	// back empty: an empty reading is not text, and a token is this program's
	// own. Either way the restore above takes the token off the clipboard
	// rather than letting it pass as a selection.
	if guard.Holds() && (selection == "" || selection == mark) {
		return "", nil
	}
	return selection, nil
}

// refusingToMark says why a selection could not be read when the clipboard
// could be put to one side but not marked. It says what happened in the
// snapshot's own terms, because it is the same thing: the clipboard holds
// something this program cannot safely write over.
func refusingToMark(err error) error {
	return fmt.Errorf("the clipboard cannot be marked for the selection: %w", err)
}

// selectionMark is the token a capture puts on the clipboard so that it can
// tell a copy that happened from one that did not. It names this process and
// the moment, so a pane that copies back the very text the clipboard held
// cannot be mistaken for a copy of the token, and it is written in a shape
// nobody types.
func selectionMark() string {
	return "trans-selection-" + strconv.Itoa(os.Getpid()) + "-" +
		strconv.FormatInt(time.Now().UnixNano(), 10)
}

// Pause is the waiting that has no signal to wait for: a short, named sleep in
// place of a real answer from the system. It is written without a build tag,
// and it is exported because the delivery policy spends its two remaining
// delays through it — a policy that is checked on a machine with no Windows has
// to be handed this the same way it is handed the clipboard.
//
// It returns early when the context is done, because the panel cancels the work
// it started when the author presses escape and nothing that follows a
// cancelled delivery is wanted.
func Pause(ctx context.Context, howLong time.Duration) error {
	timer := time.NewTimer(howLong)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}

// note writes down what a capture or a delivery did about a clipboard it could
// not take safely, a chord that brought nothing back, or keys Windows refused.
// There is nobody to tell at the moment it happens — the caller is told by the
// error or by the empty answer — so this is for whoever reads the log later.
func note(what string) {
	winlog.Note("trans-window", "%s", what)
}
