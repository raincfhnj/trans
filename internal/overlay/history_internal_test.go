package overlay

import (
	"context"
	"testing"

	"trans/internal/history"
	"trans/internal/promptflow"

	tea "github.com/charmbracelet/bubbletea"
)

// silentTranslator is a translator these tests never ask anything of: every key
// pressed here is answered from the record.
type silentTranslator struct{}

func (silentTranslator) Translate(context.Context, string) (string, error) { return "EN", nil }

// silentTarget is a target nothing is delivered to, for the same reason.
type silentTarget struct{}

func (silentTarget) Insert(context.Context, string) error { return nil }

// recordWith is a record holding the given prompts, newest first, which is what
// the popup is given when the record is opened.
type recordWith struct{ entries []history.Entry }

func (r *recordWith) Record(_, _ string, _ promptflow.Delivery) error { return nil }
func (r *recordWith) Entries() ([]history.Entry, error)               { return r.entries, nil }
func (r *recordWith) Forget(*history.Entry) error                     { return nil }

func recordOf(sources ...string) *recordWith {
	records := &recordWith{}
	for _, source := range sources {
		records.entries = append(records.entries, history.Entry{Source: source, Translation: "EN(" + source + ")"})
	}
	return records
}

// takingAPromptBackOut changes the record of a panel directly, so the keys can
// be pressed without a terminal: what is asserted is the model's own state, and
// a frame is not part of it.
func takingAPromptBackOut(t *testing.T, record History, presses ...tea.KeyMsg) Model {
	t.Helper()

	var model tea.Model = New(context.Background(),
		promptflow.New(silentTranslator{}, silentTarget{}, silentTarget{}),
		Options{Service: "deepl", Language: "EN-US", Vim: true, History: record})
	for _, press := range presses {
		model, _ = model.Update(press)
	}
	overlay, ok := model.(Model)
	if !ok {
		t.Fatalf("the panel is a %T, want the overlay's own model", model)
	}
	return overlay
}

func keyPress(keys ...string) []tea.KeyMsg {
	presses := make([]tea.KeyMsg, 0, len(keys))
	for _, key := range keys {
		switch key {
		case "ctrl+g":
			presses = append(presses, tea.KeyMsg{Type: tea.KeyCtrlG})
		case "enter":
			presses = append(presses, tea.KeyMsg{Type: tea.KeyEnter})
		case "esc":
			presses = append(presses, tea.KeyMsg{Type: tea.KeyEsc})
		case "delete":
			presses = append(presses, tea.KeyMsg{Type: tea.KeyDelete})
		default:
			presses = append(presses, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(key)})
		}
	}
	return presses
}

// Enter puts the prompt that is selected back into the box, exactly as it was
// written, and leaves the record: the box is what the keys go to again.
func TestEnterPutsTheSelectedPromptBackIntoTheBox(t *testing.T) {
	t.Parallel()

	model := takingAPromptBackOut(t, recordOf("Alter Satz aus dem Protokoll"),
		keyPress("ctrl+g", "enter")...)

	if model.historyOpen {
		t.Error("the record is still open, want the box back")
	}
	if got := model.draft.Value(); got != "Alter Satz aus dem Protokoll" {
		t.Errorf("the box holds %q, want the prompt that was chosen", got)
	}
	// Nothing is spent on a prompt that was already paid for: the translation
	// came back with the record, so the panel has it to show already.
	if model.preview != "EN(Alter Satz aus dem Protokoll)" {
		t.Errorf("the panel shows %q, want the translation the record kept", model.preview)
	}
}

// Escape closes the record and leaves the box as it was: looking at what was
// sent before is not a reason to lose what is being written now.
func TestEscapeClosesTheRecordWithoutTouchingTheBox(t *testing.T) {
	t.Parallel()

	model := takingAPromptBackOut(t, recordOf("Ein Prompt aus dem Protokoll"),
		keyPress("Bitte", "ctrl+g", "esc")...)

	if model.historyOpen {
		t.Error("the record is still open after escape")
	}
	if got := model.draft.Value(); got != "Bitte" {
		t.Errorf("the box holds %q, want the draft left as it was", got)
	}
}

// Deleting takes one prompt out of the record, and the last one out closes the
// view: an empty record is nothing to look at.
func TestDeletingEmptiesTheRecordAndClosesIt(t *testing.T) {
	t.Parallel()

	record := recordOf("Nur ein Test")
	model := takingAPromptBackOut(t, record, keyPress("ctrl+g", "delete")...)
	if model.historyOpen {
		t.Error("the record stayed open with nothing left in it")
	}

	// A record with more than one prompt stays open on the next one.
	record = recordOf("Nur ein Test", "Ein Prompt aus dem Protokoll")
	model = takingAPromptBackOut(t, record, keyPress("ctrl+g", "delete")...)
	if !model.historyOpen {
		t.Error("the record closed with a prompt still in it")
	}
	if holds := len(model.historyList); holds != 1 {
		t.Errorf("the record shows %d prompts, want the one that was not deleted", holds)
	}
}
