package overlay_test

import (
	"context"
	"strings"
	"sync"
	"testing"

	"trans/internal/history"
	"trans/internal/overlay"
	"trans/internal/promptflow"

	tea "github.com/charmbracelet/bubbletea"
)

// turnCounter is a History that says when a prompt was recorded. The panel
// writes to the record from its own goroutine while it is still working — before
// the box empties — so a send that must not happen says so twice here: the
// record hears nothing, and the panel never leaves the stage a send would have
// left behind. A test that waits on this waits for the panel rather than for a
// frame to be drawn.
type turnCounter struct {
	mu      sync.Mutex
	entries []history.Entry
}

func (c *turnCounter) Record(source, translation string, _ promptflow.Delivery) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.entries = append(c.entries, history.Entry{Source: source, Translation: translation})
	return nil
}

func (c *turnCounter) Entries() ([]history.Entry, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	return append([]history.Entry(nil), c.entries...), nil
}

func (c *turnCounter) Forget(entry *history.Entry) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	left := make([]history.Entry, 0, len(c.entries))
	for _, kept := range c.entries {
		if kept.Source != entry.Source {
			left = append(left, kept)
		}
	}
	c.entries = left
	return nil
}

func (c *turnCounter) recorded() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return len(c.entries)
}

// sendTo runs messages through the model the way the runtime would, one at a
// time — keystrokes, the window size, whatever the panel is given.
func sendTo(model tea.Model, messages ...any) tea.Model {
	for _, message := range messages {
		var next tea.Model
		switch typed := message.(type) {
		case tea.KeyMsg:
			next, _ = model.Update(typed)
		default:
			next, _ = model.Update(message)
		}
		model = next
	}
	return model
}

// pressKeys runs keystrokes against a model the way the runtime would.
func pressKeys(model tea.Model, presses ...tea.KeyMsg) tea.Model {
	messages := make([]any, 0, len(presses))
	for _, press := range presses {
		messages = append(messages, press)
	}
	return sendTo(model, messages...)
}

// panelStartingAt builds a panel the way the popup starts one, without a
// terminal: the same model, the same messages, and nothing drawn, so a test
// reads what the panel decided rather than asking when a frame appeared.
func panelStartingAt(t *testing.T, target promptflow.Target, options overlay.Options) tea.Model {
	t.Helper()
	if options.History == nil {
		options.History = &turnCounter{}
	}
	options.Service = "deepl"
	options.Language = "EN-US"

	var model tea.Model = overlay.New(context.Background(),
		promptflow.New(stubTranslator{english: english}, target, target), options)
	return sendTo(model, tea.WindowSizeMsg{Width: 87, Height: 20})
}

// A terminal writes an alt chord as an escape and then the key. Two presses stay
// two messages, so leaving insert mode and pressing enter is not a send: the
// panel composes on, the record hears nothing, and the target gets nothing.
func TestEscapeThenEnterIsNotASend(t *testing.T) {
	t.Parallel()

	counter := &turnCounter{}
	target := &recordingTarget{}
	model := panelStartingAt(t, target, overlay.Options{History: counter, Vim: true})

	model = pressKeys(model, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(draft)})
	model = pressKeys(model, tea.KeyMsg{Type: tea.KeyEsc}, tea.KeyMsg{Type: tea.KeyEnter})

	if !overlay.IsComposing(model.(overlay.Model)) {
		t.Error("the panel is not composing after escape and enter, want it idle with the draft")
	}
	if counter.recorded() != 0 {
		t.Errorf("the record holds %d prompts, want none: escape and enter is not a send",
			counter.recorded())
	}
	if sent := target.sent(); len(sent) != 0 {
		t.Errorf("target received %v, want nothing sent", sent)
	}
}

// Without live translation nothing is asked of the service until the send key is
// pressed: the panel composes with the draft written, and the service has not
// heard of it. The frame the panel draws would be a second claim on top of it
// and would say only how fast the machine redraws.
func TestWithoutLiveModeNothingIsTranslatedUntilYouSend(t *testing.T) {
	t.Parallel()

	translator := &recordingTranslator{english: english}
	var model tea.Model = overlay.New(context.Background(),
		promptflow.New(translator, &recordingTarget{}, &recordingTarget{}),
		overlay.Options{Service: "deepl", Language: "EN-US"})
	model = sendTo(model, tea.WindowSizeMsg{Width: 87, Height: 20})
	model = pressKeys(model, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(draft)})

	if !overlay.IsComposing(model.(overlay.Model)) {
		t.Error("the panel is not composing, want it waiting for the send key")
	}
	if seen := translator.seenDraft; seen != "" {
		t.Errorf("the translator was called with %q, want no call before sending", seen)
	}
	if !strings.Contains(overlay.DraftHolds(model.(overlay.Model)), draft) {
		t.Errorf("the box holds %q, want the draft written into it",
			overlay.DraftHolds(model.(overlay.Model)))
	}
}
