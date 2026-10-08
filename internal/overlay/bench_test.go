package overlay

import (
	"context"
	"testing"

	"trans/internal/promptflow"

	tea "github.com/charmbracelet/bubbletea"
)

// stillPrompter is the panel's way out with nothing behind it: the benchmarks
// below draw the panel, and nothing they do reaches for a translation.
type stillPrompter struct{}

func (stillPrompter) Submit(context.Context, string, promptflow.Delivery) (string, error) {
	return "", nil
}

func (stillPrompter) Translate(context.Context, string) (string, error) { return "", nil }

func (stillPrompter) Deliver(context.Context, string, promptflow.Delivery) error { return nil }

func (stillPrompter) Usage(context.Context) (promptflow.Usage, bool, error) {
	return promptflow.Usage{}, false, nil
}

const (
	benchDraft   = "Bitte behebe den fehlschlagenden Test im Formular und schick ihn zurück."
	benchEnglish = "Please fix the failing test in the form and send it back."
)

// filledPanel is the panel as it looks while it is open with work in it: the
// draft written, its translation beside it, the usage in the header. That is
// the frame that costs something — an empty box is drawn once and then sits.
func filledPanel() Model {
	model := New(context.Background(), stillPrompter{},
		Options{Service: "deepl", Language: "EN-US", Live: true, Vim: true})
	sized, _ := model.Update(tea.WindowSizeMsg{Width: 87, Height: 22})
	model = sized.(Model)
	model.draft.Resume(benchDraft)
	model.preview, model.previewOf = benchEnglish, benchDraft
	model.spent, model.spentKnown = promptflow.Usage{Used: 999_900, Limit: 1_000_000}, true
	return model
}

// The panel draws itself on every frame — once per key, and sixty times a
// second while the pulse runs — so what one frame costs is what the popup
// pays for as long as it is open. This measures that frame alone: the model
// is brought to a full pane first and then only asked what it looks like.
func BenchmarkView(b *testing.B) {
	model := filledPanel()
	b.ReportAllocs()
	b.ResetTimer()
	for range b.N {
		_ = model.View()
	}
}

// The other half of what a keystroke costs: the message going in and the
// model coming back, with a backspace behind it so the draft does not grow
// for as long as the benchmark runs. The commands the update returns are not
// run — timers are the Bubble Tea program's work, not the model's.
func BenchmarkTyping(b *testing.B) {
	model := filledPanel()
	letter := tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("ö")}
	erase := tea.KeyMsg{Type: tea.KeyBackspace}
	b.ReportAllocs()
	b.ResetTimer()
	for range b.N {
		written, _ := model.Update(letter)
		erased, _ := written.(Model).Update(erase)
		model = erased.(Model)
	}
}
