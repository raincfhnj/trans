// Package frame is the drawing furniture the popup windows share: the palette
// slots the terminal paints, the boxes the writing goes in, and the bar that
// tells where a long text sits. It is styles and functions rather than a model,
// so each window keeps its own state around it.
package frame

import (
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
)

// The terminal paints its palette from the active theme but does not expose
// the theme itself, so the popups only name palette slots and let whatever
// theme is running decide how they look.
var (
	accent = lipgloss.Color("5")
	danger = lipgloss.Color("1")
	// The frame is a line, not something to read, so grey suits it.
	grey   = lipgloss.Color("8")
	bright = lipgloss.Color("13")
)

// The box keeps its padding, and a column beside every line is reserved for
// the scroll bar whether or not there is anything to scroll, so text never
// rewraps when a bar appears.
const (
	BoxPadding   = 2
	ScrollColumn = 2

	// A dashed hairline for the track, so it does not read as a second border,
	// and a heavier line in the accent colour for the thumb.
	ScrollTrack = "╎"
	ScrollThumb = "┃"
)

// Foregrounds only: a cell left alone keeps the background the terminal painted.
type Styles struct {
	Text        lipgloss.Style
	Placeholder lipgloss.Style
	Badge       lipgloss.Style
	Hint        lipgloss.Style
	Key         lipgloss.Style
	Accent      lipgloss.Style
	Danger      lipgloss.Style
	Faded       lipgloss.Style
	Bright      lipgloss.Style
	// Off is for a setting that is not in force: struck through and otherwise
	// in the same colour as the rest, so it reads as switched off, not as
	// faded out.
	Off  lipgloss.Style
	Mark lipgloss.Style

	ActiveBox lipgloss.Style
	IdleBox   lipgloss.Style
}

func NewStyles() Styles {
	// Anything meant to be read keeps the terminal's own foreground: grey on a
	// dark theme, or on a light one, is what makes a popup unreadable. The
	// difference between a key and its label is weight, not brightness.
	return Styles{
		Text:        lipgloss.NewStyle(),
		Placeholder: lipgloss.NewStyle().Faint(true),
		Badge:       lipgloss.NewStyle(),
		Hint:        lipgloss.NewStyle(),
		Key:         lipgloss.NewStyle().Foreground(accent).Bold(true),
		Accent:      lipgloss.NewStyle().Foreground(accent),
		Danger:      lipgloss.NewStyle().Foreground(danger).Bold(true),
		Faded:       lipgloss.NewStyle().Faint(true),
		Bright:      lipgloss.NewStyle().Foreground(bright).Bold(true),
		Off:         lipgloss.NewStyle().Strikethrough(true),
		Mark:        lipgloss.NewStyle().Foreground(grey),

		// The accented border marks the box being written in or read; the other
		// one recedes into the frame's grey.
		ActiveBox: lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(accent).
			Padding(0, 1),
		IdleBox: lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(grey).
			Padding(0, 1),
	}
}

// Box is a box the width of the popup, accented while it is the one being
// written in or read.
func (s Styles) Box(active bool, width int) lipgloss.Style {
	if active {
		return s.ActiveBox.Width(width)
	}
	return s.IdleBox.Width(width)
}

// Labelled writes a word into the bottom border, near the right corner, where
// a box has room for it and nothing else is drawn. The border is one colour,
// so the line is rebuilt from its characters rather than picked apart.
func (s Styles) Labelled(box, label string, active bool) string {
	if label == "" {
		return box
	}

	lines := strings.Split(box, "\n")
	bottom := ansi.Strip(lines[len(lines)-1])
	runes := []rune(bottom)

	const margin = 2
	room := len(runes) - margin*2
	if lipgloss.Width(label) > room {
		return box
	}

	border := s.Mark
	if active {
		border = s.Accent
	}
	at := len(runes) - margin - lipgloss.Width(label)
	lines[len(lines)-1] = border.Render(string(runes[:at])) +
		s.Badge.Render(label) +
		border.Render(string(runes[at+lipgloss.Width(label):]))
	return strings.Join(lines, "\n")
}
