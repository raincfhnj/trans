package overlay

import (
	"trans/internal/frame"

	"github.com/charmbracelet/lipgloss"
)

// Foregrounds only: a cell left alone keeps the background the terminal painted.
type styles struct {
	text        lipgloss.Style
	placeholder lipgloss.Style
	cursor      lipgloss.Style
	badge       lipgloss.Style
	hint        lipgloss.Style
	key         lipgloss.Style
	accent      lipgloss.Style
	danger      lipgloss.Style
	faded       lipgloss.Style
	bright      lipgloss.Style
	mode        lipgloss.Style
	// off is for a setting that is not in force: struck through and otherwise in
	// the same colour as the rest, so it reads as switched off, not as faded out.
	off lipgloss.Style
	// mark is the braille signature: the colour of the frame, since it is drawn
	// furniture rather than something to read. Faint is too dim for braille dots.
	mark lipgloss.Style
}

// cursorFor is the cell the caret is drawn in. A terminal that is told where the
// caret is draws its own cursor there, and a second one painted under it would
// only be in the way.
func (s styles) cursorFor(placed *CursorPlace) lipgloss.Style {
	if placed != nil {
		return lipgloss.NewStyle()
	}
	return s.cursor
}

// newStyles builds the panel's styles from a palette, which only names slots of
// the terminal's own theme. Anything meant to be read keeps the terminal's own
// foreground: grey on a dark theme, or on a light one, is what makes a popup
// unreadable. The difference between a key and its label is weight, not
// brightness.
func newStyles(p frame.Palette) styles {
	return styles{
		text:        lipgloss.NewStyle(),
		placeholder: lipgloss.NewStyle().Faint(true),
		cursor:      lipgloss.NewStyle().Foreground(p.Accent),
		badge:       lipgloss.NewStyle(),
		hint:        lipgloss.NewStyle(),
		key:         lipgloss.NewStyle().Foreground(p.Accent).Bold(true),
		accent:      lipgloss.NewStyle().Foreground(p.Accent),
		danger:      lipgloss.NewStyle().Foreground(p.Danger).Bold(true),
		faded:       lipgloss.NewStyle().Faint(true),
		bright:      lipgloss.NewStyle().Foreground(p.Bright).Bold(true),
		mode:        lipgloss.NewStyle().Foreground(p.Accent).Bold(true),
		off:         lipgloss.NewStyle().Strikethrough(true),
		mark:        lipgloss.NewStyle().Foreground(p.Grey),
	}
}
