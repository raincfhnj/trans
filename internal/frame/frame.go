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
// The two box styles are named rather than kept whole: a Styles carries every
// style at once, and a method that takes it by value would copy the lot on each
// call — which the linter counts, and which the calls in a redraw do often.
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
}

// Labelled writes a word into the bottom border, near the right corner, where
// a box has room for it and nothing else is drawn. The border is one colour,
// so the line is rebuilt from its characters rather than picked apart.
//
// Three styles come in because the windows light their boxes differently: the
// border of the box being read is not always the style that carries the word,
// and a window whose badge sits on an accented border has to be able to say so.
// A caller that wants the same style for both passes it twice.
func Labelled(border, activeBorder, badge *lipgloss.Style, box, label string, active bool) string {
	if label == "" {
		return box
	}

	// Asked once: the label is measured three times over below, and each ask
	// walks the string's escape sequences again.
	labelWidth := ansi.StringWidth(label)

	lines := strings.Split(box, "\n")
	bottom := ansi.Strip(lines[len(lines)-1])
	runes := []rune(bottom)

	const margin = 2
	room := len(runes) - margin*2
	if labelWidth > room {
		return box
	}

	if active {
		border = activeBorder
	}
	at := len(runes) - margin - labelWidth
	lines[len(lines)-1] = border.Render(string(runes[:at])) +
		badge.Render(label) +
		border.Render(string(runes[at+labelWidth:]))
	return strings.Join(lines, "\n")
}
