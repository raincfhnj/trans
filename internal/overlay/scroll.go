package overlay

import (
	"strings"

	"trans/internal/frame"
)

// scrolled draws the bar beside text that does not fit, telling where the view
// sits in it. The drawing itself is the frame's, shared with the two smaller
// windows; what is the overlay's own is which palette slots it is painted with.
func (m Model) scrolled(body string, first, visible, total int) string {
	return frame.Scrolled(m.barStyles(), m.contentWidth(), body, first, visible, total)
}

// barStyles is the two slots the bar is painted from, gathered in the shape the
// frame draws with. It is built per call rather than kept: the rest of the
// overlay's palette has names of its own — a cursor, a mode — that the frame
// has no slot for, and the panel's own palette is kept whole for the boxes.
func (m Model) barStyles() *frame.Styles {
	return &frame.Styles{Mark: m.styles.mark, Accent: m.styles.accent}
}

func (m Model) draftScroll() (first, visible, total int) {
	return m.draftTop, m.draftRows(), frame.RowsOf(m.draft.Value(), m.contentWidth())
}

// followCursor moves the view as little as it takes to keep the cursor on screen,
// which is what the text area does with the viewport it keeps to itself.
func (m *Model) followCursor() {
	rows, total := m.draftRows(), frame.RowsOf(m.draft.Value(), m.contentWidth())
	cursor := m.cursorRow()

	m.draftTop = min(m.draftTop, cursor)
	m.draftTop = max(m.draftTop, cursor-rows+1)
	m.draftTop = min(max(m.draftTop, 0), max(total-rows, 0))
}

func (m Model) cursorRow() int {
	width := m.contentWidth()
	row := m.draft.RowOffset()
	for index, line := range strings.Split(m.draft.Value(), "\n") {
		if index >= m.draft.Row() {
			break
		}
		row += frame.RowsOf(line, width)
	}
	return row
}

// contentWidth is what a box leaves for text beside its padding and the bar.
func (m Model) contentWidth() int {
	return frame.ContentWidth(m.width)
}
