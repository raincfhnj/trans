package frame

import (
	"slices"

	"github.com/charmbracelet/lipgloss"
)

// The terminal paints its palette from the active theme but does not expose
// the theme itself, so a palette here names slots rather than colours: the
// four names point at 0-15 of whatever palette the terminal is running, and
// the terminal decides how they look. A theme of this program's own is
// therefore a way of pointing at the terminal's colours rather than of
// overriding them — a popup that carried colours of its own would be the one
// thing on screen not following the theme the author chose.
type Palette struct {
	Accent lipgloss.Color
	Danger lipgloss.Color
	// Grey is the furniture: a frame is a line, not something to read.
	Grey   lipgloss.Color
	Bright lipgloss.Color
}

// DefaultPalette is the magenta the panel has always drawn with — the theme
// with no name, and what an unset or unknown TRANS_THEME draws with.
var DefaultPalette = Palette{
	Accent: lipgloss.Color("5"),
	Danger: lipgloss.Color("1"),
	Grey:   lipgloss.Color("8"),
	Bright: lipgloss.Color("13"),
}

// themes are the built-in palettes under their names. Danger and grey are
// the same slots in all of them: red means danger and grey means furniture
// wherever the accent comes from, so a theme changes what is lit up rather
// than what words mean. Every slot is 0-15 so that the terminal's own theme
// still decides the shades — see the test that keeps the popups on it.
var themes = map[string]Palette{
	"ocean": {
		Accent: lipgloss.Color("4"), Danger: DefaultPalette.Danger,
		Grey: DefaultPalette.Grey, Bright: lipgloss.Color("12"),
	},
	"forest": {
		Accent: lipgloss.Color("2"), Danger: DefaultPalette.Danger,
		Grey: DefaultPalette.Grey, Bright: lipgloss.Color("10"),
	},
	"amber": {
		Accent: lipgloss.Color("3"), Danger: DefaultPalette.Danger,
		Grey: DefaultPalette.Grey, Bright: lipgloss.Color("11"),
	},
	"mono": {
		Accent: lipgloss.Color("7"), Danger: DefaultPalette.Danger,
		Grey: DefaultPalette.Grey, Bright: lipgloss.Color("15"),
	},
}

// themeNames is the order the settings window steps through the themes, kept
// apart from the map because a map has no order to step in.
var themeNames = []string{"ocean", "forest", "amber", "mono"}

// ThemeNames is every built-in theme in stepping order. The palette with no
// name is not among them: it is the position the theme row starts on and
// returns to, called "auto" the way the service that picks itself is.
func ThemeNames() []string { return slices.Clone(themeNames) }

// PaletteFor is the palette a theme names. A name this program has never
// heard of — a typo in the .env — draws with the default rather than the
// panel refusing to open over a colour it cannot find.
func PaletteFor(name string) Palette {
	if palette, known := themes[name]; known {
		return palette
	}
	return DefaultPalette
}

// Styles is the palette made into the styles the windows draw with. Anything
// meant to be read keeps the terminal's own foreground: grey on a dark theme,
// or on a light one, is what makes a popup unreadable. The difference between
// a key and its label is weight, not brightness.
func (p Palette) Styles() Styles {
	return Styles{
		Text:        lipgloss.NewStyle(),
		Placeholder: lipgloss.NewStyle().Faint(true),
		Badge:       lipgloss.NewStyle(),
		Hint:        lipgloss.NewStyle(),
		Key:         lipgloss.NewStyle().Foreground(p.Accent).Bold(true),
		Accent:      lipgloss.NewStyle().Foreground(p.Accent),
		Danger:      lipgloss.NewStyle().Foreground(p.Danger).Bold(true),
		Faded:       lipgloss.NewStyle().Faint(true),
		Bright:      lipgloss.NewStyle().Foreground(p.Bright).Bold(true),
		Off:         lipgloss.NewStyle().Strikethrough(true),
		Mark:        lipgloss.NewStyle().Foreground(p.Grey),

		// The accented border marks the box being written in or read; the other
		// one recedes into the frame's grey.
	}
}

// ActiveBox is the border of the box being written in or read.
func (p Palette) ActiveBox() lipgloss.Style {
	return lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(p.Accent).
		Padding(0, 1)
}

// IdleBox is the border of the box that is not being read.
func (p Palette) IdleBox() lipgloss.Style {
	return lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(p.Grey).
		Padding(0, 1)
}

// Box is a box the width of the popup, accented while it is the one being
// written in or read. The border styles are built here rather than carried in
// Styles: Styles is copied on every call that takes it, and two more lipgloss
// styles in it are two more copies a redraw would pay for. The palette they
// are built from is four colour names — a fraction of what Styles carries —
// so a window keeping one beside its styles pays for it once, not per call.
func (p Palette) Box(active bool, width int) lipgloss.Style {
	if active {
		return p.ActiveBox().Width(width)
	}
	return p.IdleBox().Width(width)
}
