package overlay_test

import (
	"context"
	"strings"
	"testing"

	"trans/internal/frame"
	"trans/internal/overlay"
	"trans/internal/promptflow"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/muesli/termenv"
)

// A theme of this program's own only points at the terminal's colours, so that
// the popup is the one window that follows the theme the author chose rather
// than the one that argues with it. Every theme the settings window can name —
// and the position it starts on, the palette with no name — is opened here in
// a profile where a colour carried from anywhere else would be written out in
// full and could be read off the frame.
//
// The profile is global and the frames below are drawn one after another, so
// this test runs on its own the way the rest of the palette tests do.
func TestEveryThemeDrawsWithTheTerminalPaletteOnly(t *testing.T) {
	previous := lipgloss.ColorProfile()
	lipgloss.SetColorProfile(termenv.TrueColor)
	defer lipgloss.SetColorProfile(previous)

	for _, theme := range append([]string{""}, frame.ThemeNames()...) {
		flow := promptflow.New(stubTranslator{english: english}, &recordingTarget{}, &recordingTarget{})
		var model tea.Model = overlay.New(context.Background(), flow, overlay.Options{
			Service: "deepl", Language: "EN-US", Vim: true, Live: true,
			History: seededHistory(draft),
			Theme:   theme,
		})
		model, _ = model.Update(tea.WindowSizeMsg{Width: 87, Height: 22})
		model, _ = model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(draft)})
		model, _ = model.Update(overlay.PreviewShown(draft, english))
		assertPaletteOnly(t, theme, "writing", model.View())

		// The same box given to the translation alone, where the accent and
		// the frame's grey meet on one border.
		model, _ = model.Update(tea.KeyMsg{Type: tea.KeyTab})
		assertPaletteOnly(t, theme, "reading", model.View())
		model, _ = model.Update(tea.KeyMsg{Type: tea.KeyTab})

		// The record draws its box from the palette too, and one entry is on
		// screen to be drawn.
		model, _ = model.Update(tea.KeyMsg{Type: tea.KeyCtrlG})
		assertPaletteOnly(t, theme, "history", model.View())
		model, _ = model.Update(tea.KeyMsg{Type: tea.KeyEsc})

		// A translation that did not come is the only frame drawn in the
		// danger slot.
		model, _ = model.Update(overlay.PreviewFailed("the service said no"))
		assertPaletteOnly(t, theme, "a failed translation", model.View())
	}
}

// assertPaletteOnly reads the escape sequences of a frame and fails on a colour
// written out as a number, which is what a style carrying anything but the
// terminal's own slots would leave behind.
func assertPaletteOnly(t *testing.T, theme, where, rendered string) {
	t.Helper()
	for _, sequence := range sgrSequences(rendered) {
		for _, fixed := range []string{"38;5;", "48;5;", "38;2;", "48;2;"} {
			if strings.Contains(sequence, fixed) {
				t.Errorf("theme %q draws %s with the fixed colour %q in \x1b[%sm",
					theme, where, fixed, sequence)
			}
		}
	}
}
