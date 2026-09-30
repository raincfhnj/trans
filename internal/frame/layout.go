package frame

import (
	"strconv"
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
)

// ContentWidth is what a box leaves for text beside its padding and the bar.
func ContentWidth(width int) int {
	return max(width-BoxPadding-ScrollColumn, 1)
}

// Scrolled draws the bar beside text that does not fit, telling where the
// view sits in it. It reads the two colours off the palette rather than
// carrying Styles by value: a copy of the whole palette per row would be the
// expensive way to colour a bar.
func Scrolled(palette *Styles, contentWidth int, body string, first, visible, total int) string {
	lines := strings.Split(body, "\n")
	if total <= visible || len(lines) == 0 {
		return body
	}

	// The thumb is as long a part of the bar as the box is of the text, and
	// sits where the view sits — at the bottom once the last row is on screen.
	height := max(len(lines)*visible/total, 1)
	room := len(lines) - height
	scrollable := total - visible

	top := 0
	if room > 0 && scrollable > 0 {
		top = min((first*room+scrollable/2)/scrollable, room)
	}

	for index, row := range lines {
		// The thumb is read, the track only tells it where it can go.
		mark := palette.Mark.Render(ScrollTrack)
		if index >= top && index < top+height {
			mark = palette.Accent.Render(ScrollThumb)
		}
		// Padded first: a bar is a column, not something that follows the text.
		lines[index] = row + strings.Repeat(" ",
			max(contentWidth-lipgloss.Width(row), 0)) + " " + mark
	}
	return strings.Join(lines, "\n")
}

// Spread pins left to the start of the line and right to its end.
func Spread(left, right string, line int) string {
	// Nothing here may outgrow the line: a wrapped footer would push the box up
	// and the popup out of the pane.
	if right == "" {
		return CutTo(left, line)
	}

	gap := line - lipgloss.Width(left) - lipgloss.Width(right)
	if gap < 1 {
		// Too narrow for both, and the keys on the left are worth more.
		return CutTo(left, line)
	}
	return left + strings.Repeat(" ", gap) + right
}

// CutTo is the text as it fits, cut where it would otherwise wrap.
func CutTo(text string, width int) string {
	return lipgloss.NewStyle().MaxWidth(width).Render(text)
}

// RowsFrom wraps text and keeps the rows from one onwards.
func RowsFrom(text string, width, from, rows int) string {
	if width < 1 || rows < 1 {
		return text
	}

	lines := strings.Split(Wrapped(text, width), "\n")
	from = min(max(from, 0), max(len(lines)-1, 0))
	return strings.Join(lines[from:min(from+rows, len(lines))], "\n")
}

// RowsOf counts the rows text takes at that width, breaking a word too long to
// fit the way the text area does.
func RowsOf(text string, width int) int {
	if text == "" {
		return 1
	}
	if width < 1 {
		return len(strings.Split(text, "\n"))
	}
	return len(strings.Split(Wrapped(text, width), "\n"))
}

// Wrapped is the text broken for a box of that width.
func Wrapped(text string, width int) string {
	return ansi.Wrap(text, width, "")
}

// HowFarThrough is the share of the text that has been seen, for the border of
// whichever box is being read.
func HowFarThrough(first, visible, total int) string {
	if total <= visible {
		return ""
	}
	return strconv.Itoa(min((first+visible)*100/total, 100)) + "%"
}
