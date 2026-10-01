package overlay_test

import (
	"bytes"
	"context"
	"testing"
	"time"

	"trans/internal/overlay"
	"trans/internal/promptflow"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/exp/teatest"
)

// markingTranslator answers with English that says which draft it came from, so a
// test can tell one translation from another.
type markingTranslator struct{}

func (markingTranslator) Translate(_ context.Context, draft string) (string, error) {
	return "EN(" + draft + ")", nil
}

// Reading the English before it goes is only worth anything if what goes is what
// was read. Writing after the confirmation therefore takes the question back.
func TestWritingAfterAConfirmationNeverDeliversTheOlderEnglish(t *testing.T) {
	t.Parallel()
	target := &recordingTarget{}

	overlayUnderTest := confirmingOverlay(t, markingTranslator{}, target)
	overlayUnderTest.Type("erste Fassung")
	overlayUnderTest.Send(tea.KeyMsg{Type: tea.KeyCtrlD})

	teatest.WaitFor(t, overlayUnderTest.Output(), func(out []byte) bool {
		return bytes.Contains(out, []byte("EN(erste Fassung)"))
	}, teatest.WithDuration(frameTimeout))

	// The draft moves on while the English of the older one is on screen.
	overlayUnderTest.Type(" und mehr")
	overlayUnderTest.Send(tea.KeyMsg{Type: tea.KeyCtrlD})

	teatest.WaitFor(t, overlayUnderTest.Output(), func(out []byte) bool {
		return bytes.Contains(out, []byte("EN(erste Fassung und mehr)"))
	}, teatest.WithDuration(frameTimeout))

	overlayUnderTest.Send(tea.KeyMsg{Type: tea.KeyCtrlD})
	waitForTheNextPrompt(t, overlayUnderTest)

	if len(target.sent()) != 1 || target.sent()[0] != "EN(erste Fassung und mehr)" {
		t.Errorf("target received %v, want only the English of the draft as it stood", target.sent())
	}

	closeTheOverlay(t, overlayUnderTest)
}

// Throwing the draft away while its English waits leaves nothing to send, and
// saying so beats handing the agent an empty prompt.
func TestDiscardingADraftDuringConfirmationSendsNothing(t *testing.T) {
	t.Parallel()
	target := &recordingTarget{}

	overlayUnderTest := confirmingOverlay(t, markingTranslator{}, target)
	overlayUnderTest.Type("wieder weg")
	overlayUnderTest.Send(tea.KeyMsg{Type: tea.KeyCtrlD})

	teatest.WaitFor(t, overlayUnderTest.Output(), func(out []byte) bool {
		return bytes.Contains(out, []byte("EN(wieder weg)"))
	}, teatest.WithDuration(frameTimeout))

	overlayUnderTest.Send(tea.KeyMsg{Type: tea.KeyCtrlU})
	overlayUnderTest.Send(tea.KeyMsg{Type: tea.KeyCtrlD})

	teatest.WaitFor(t, overlayUnderTest.Output(), func(out []byte) bool {
		return bytes.Contains(out, []byte("the draft is empty"))
	}, teatest.WithDuration(frameTimeout))

	if len(target.sent()) != 0 {
		t.Errorf("target received %v, want nothing after the draft was thrown away", target.sent())
	}

	overlayUnderTest.Send(tea.KeyMsg{Type: tea.KeyCtrlC})
	overlayUnderTest.WaitFinished(t, teatest.WithFinalTimeout(frameTimeout))
}

// A translation is only ever delivered for the draft it was made from, whatever
// order the model is driven in.
func TestAStaleConfirmationIsTranslatedAgainRatherThanDelivered(t *testing.T) {
	t.Parallel()
	target := &recordingTarget{}
	flow := promptflow.New(markingTranslator{}, target, target)

	var model tea.Model = overlay.New(context.Background(), flow, overlay.Options{
		Service: "deepl", Language: "EN-US", Confirm: true,
	})
	model, _ = model.Update(tea.WindowSizeMsg{Width: 87, Height: 17})
	model, _ = model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("eine Fassung")})

	// Confirmation reached with a translation that belongs to an older draft.
	var next tea.Model = overlay.ConfirmationOf(
		model.(overlay.Model), "eine andere Fassung", "EN(eine andere Fassung)")

	next, cmd := next.Update(tea.KeyMsg{Type: tea.KeyCtrlD})
	drive(next, cmd)

	for _, delivered := range target.sent() {
		if delivered == "EN(eine andere Fassung)" {
			t.Errorf("target received %q, the English of a draft that is not there", delivered)
		}
	}
}

// drive runs commands the way the runtime would, feeding every message back into
// the model until nothing is left to do. A command that is still waiting after
// cmdPatience is taken for one that is waiting on a timer — a debounce, a
// message that lingers — and is left to answer later: the runtime would deliver
// its message in its own time, and a test that slept for it would be measuring
// the clock rather than the panel. That is where the two eleven-second tests in
// this package used to go.
func drive(model tea.Model, cmd tea.Cmd) tea.Model {
	pending := []tea.Cmd{cmd}
	for rounds := 0; len(pending) > 0 && rounds < 12; rounds++ {
		next := pending[0]
		pending = pending[1:]
		if next == nil {
			continue
		}

		msg, answered := within(next, cmdPatience)
		if !answered {
			continue
		}
		switch msg := msg.(type) {
		case nil:
		case tea.BatchMsg:
			pending = append(pending, msg...)
		default:
			var following tea.Cmd
			model, following = model.Update(msg)
			pending = append(pending, following)
		}
	}
	return model
}

// cmdPatience is how long a command is given to answer before it is taken for
// one that is waiting on a timer. Work that is done returns at once; a tick
// sleeps, and nothing in a test needs the message it would deliver.
const cmdPatience = 50 * time.Millisecond

// within runs one command and says whether it answered in time. The command
// itself is left running: it may deliver its message into the buffered channel
// and be collected, which is what a tick does when its moment finally comes.
func within(cmd tea.Cmd, patience time.Duration) (tea.Msg, bool) {
	answered := make(chan tea.Msg, 1)
	go func() { answered <- cmd() }()

	select {
	case msg := <-answered:
		return msg, true
	case <-time.After(patience):
		return nil, false
	}
}

// driveOnce runs the commands a model asked for, without following what those
// produce: enough to deliver a message, and it never runs into a timer.
func driveOnce(model tea.Model, cmd tea.Cmd) tea.Model {
	if cmd == nil {
		return model
	}
	switch msg := cmd().(type) {
	case nil:
	case tea.BatchMsg:
		for _, each := range msg {
			model = driveOnce(model, each)
		}
	default:
		model, _ = model.Update(msg)
	}
	return model
}

// ctxAwareTranslator stops the moment its request is cancelled, the way a real
// service does, so a test can let a translation be abandoned on the way.
type ctxAwareTranslator struct{ english string }

func (c ctxAwareTranslator) Translate(ctx context.Context, _ string) (string, error) {
	if err := ctx.Err(); err != nil {
		return "", err
	}
	return c.english, nil
}

// Writing on while the confirmation translation runs cancels it — live
// translation lets go of the request it started for the older draft. The
// cancelled reply must not leave the panel translating forever with the send
// key dead; the panel falls back to the draft and sending works again.
func TestACancelledConfirmationTranslationLeavesTheSendKeyWorking(t *testing.T) {
	t.Parallel()
	target := &recordingTarget{}

	var model tea.Model = overlay.New(context.Background(),
		promptflow.New(ctxAwareTranslator{english: english}, target, target),
		overlay.Options{
			Service: "deepl", Language: "EN-US",
			Confirm: true, Live: true, Debounce: time.Millisecond,
		})
	model, _ = model.Update(tea.WindowSizeMsg{Width: 87, Height: 17})
	model, _ = model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("erste Fassung")})

	// The send key asks for the translation the confirmation waits for.
	model, waiting := model.Update(tea.KeyMsg{Type: tea.KeyCtrlD})
	if !overlay.IsTranslating(model.(overlay.Model)) {
		t.Fatal("the confirmation translation never started")
	}

	// Writing on while it runs cancels it.
	model, _ = model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("!")})

	// The cancelled reply comes back and must not strand the stage.
	model = driveOnce(model, waiting)
	if overlay.IsTranslating(model.(overlay.Model)) {
		t.Fatal("the panel still waits for a translation that was cancelled")
	}
	if overlay.IsConfirming(model.(overlay.Model)) {
		t.Fatal("the panel confirms a translation that never arrived")
	}

	// The send key works again: a fresh confirmation arrives, then goes out.
	model, sending := model.Update(tea.KeyMsg{Type: tea.KeyCtrlD})
	model = driveOnce(model, sending)
	if !overlay.IsConfirming(model.(overlay.Model)) {
		t.Fatal("the send key did not start a fresh confirmation")
	}

	model, delivering := model.Update(tea.KeyMsg{Type: tea.KeyCtrlD})
	driveOnce(model, delivering)
	if len(target.sent()) != 1 || target.sent()[0] != english {
		t.Errorf("target received %v, want the draft sent once after the cancelled confirmation",
			target.sent())
	}
}

// ctrl+l turning live off while the confirmation translation runs cancels it
// the same way writing on does, and the panel has to come back the same way.
func TestTurningLiveOffDuringAConfirmationLeavesTheSendKeyWorking(t *testing.T) {
	t.Parallel()
	target := &recordingTarget{}

	var model tea.Model = overlay.New(context.Background(),
		promptflow.New(ctxAwareTranslator{english: english}, target, target),
		overlay.Options{
			Service: "deepl", Language: "EN-US",
			Confirm: true, Live: true, Debounce: time.Millisecond,
		})
	model, _ = model.Update(tea.WindowSizeMsg{Width: 87, Height: 17})
	model, _ = model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("erste Fassung")})

	model, waiting := model.Update(tea.KeyMsg{Type: tea.KeyCtrlD})

	// ctrl+l takes live translation out from under the running request.
	model, _ = model.Update(tea.KeyMsg{Type: tea.KeyCtrlL})

	model = driveOnce(model, waiting)
	if overlay.IsTranslating(model.(overlay.Model)) {
		t.Fatal("the panel still waits for the translation ctrl+l cancelled")
	}

	model, sending := model.Update(tea.KeyMsg{Type: tea.KeyCtrlD})
	model = driveOnce(model, sending)
	if !overlay.IsConfirming(model.(overlay.Model)) {
		t.Fatal("the send key did not start a fresh confirmation")
	}

	model, delivering := model.Update(tea.KeyMsg{Type: tea.KeyCtrlD})
	driveOnce(model, delivering)
	if len(target.sent()) != 1 {
		t.Errorf("target received %v, want the draft sent once", target.sent())
	}
}

// The translating stage can belong to an ordinary send rather than to a
// confirmation. A cancelled preview the send never waited for must then be
// passed over without handing the send key back while the send is still out —
// or the same prompt could be sent twice.
func TestACancelledPreviewLeavesAnOrdinarySendOnItsWay(t *testing.T) {
	t.Parallel()
	target := &recordingTarget{}

	var model tea.Model = overlay.New(context.Background(),
		promptflow.New(ctxAwareTranslator{english: english}, target, target),
		overlay.Options{
			Service: "deepl", Language: "EN-US",
			Live: true, Debounce: time.Millisecond,
		})
	model, _ = model.Update(tea.WindowSizeMsg{Width: 87, Height: 17})
	model, _ = model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(draft)})

	// A preview is asked for and left unanswered for now.
	model, previewing := model.Update(tea.KeyMsg{Type: tea.KeyCtrlT})

	// Sending stops that preview and puts the stage on the send itself.
	model, sending := model.Update(tea.KeyMsg{Type: tea.KeyCtrlD})
	if !overlay.IsTranslating(model.(overlay.Model)) {
		t.Fatal("the send never started")
	}

	// The abandoned preview answers with its cancellation; the send keeps the stage.
	model = driveOnce(model, previewing)
	if !overlay.IsTranslating(model.(overlay.Model)) {
		t.Fatal("a cancelled preview took the stage from the send already on its way")
	}

	driveOnce(model, sending)
	if len(target.sent()) != 1 || target.sent()[0] != english {
		t.Errorf("target received %v, want the one send to go through untouched", target.sent())
	}
}
