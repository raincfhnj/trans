package settings

import (
	"strings"

	"trans/internal/frame"

	"github.com/charmbracelet/lipgloss"
)

func (m Model) View() string {
	visible := m.visibleRows()
	listed := make([]string, 0, visible)
	for offset := 0; offset < visible; offset++ {
		index := m.top + offset
		if index >= len(m.rows) {
			break
		}
		listed = append(listed, m.rowLine(m.rows[index], index))
	}

	box := frame.Box(true, m.pane.Width).Height(visible).
		Render(strings.Join(listed, "\n"))
	box = frame.Labelled(&m.styles.Mark, &m.styles.Badge, &m.styles.Badge, box,
		frame.HowFarThrough(m.top, visible, len(m.rows)), true)
	line := lipgloss.Width(box)

	return strings.Join([]string{m.header(line), box, m.footer(line)}, "\n")
}

// rowLine is one setting in the list: its label, the value it holds, and —
// right at the end — the mark that says the environment would answer with
// something else, whatever the file is made to say.
func (m Model) rowLine(row *row, index int) string {
	mark := "  "
	if index == m.cursor {
		mark = m.styles.Accent.Render("▸") + " "
	}
	label := m.styles.Badge.Render(padRight(row.label, m.labelRoom()))
	left := mark + label + m.valueView(row, index)

	right := ""
	if row.env {
		right = m.styles.Faded.Render("env")
	}
	return frame.Spread(left, right, m.contentWidth())
}

// valueView is the value as it reads: the setting itself, struck through when
// it is off, a mask for a key, and the accent when it differs from what the
// file holds — the ones the save would write.
func (m Model) valueView(row *row, index int) string {
	if index == m.editing {
		return m.styles.Text.Render(m.input.View())
	}

	switch row.kind {
	case secret:
		if row.value == "" {
			return m.styles.Placeholder.Render(row.shown())
		}
		mask := "••••••••"
		if row.changed() {
			return m.styles.Accent.Render(mask)
		}
		return m.styles.Text.Render(mask)
	case flag:
		if row.value == "1" {
			return m.styles.Text.Render("on")
		}
		return m.styles.Off.Render("off")
	default:
		if row.value == "" {
			return m.styles.Placeholder.Render(row.shown())
		}
		if row.changed() {
			return m.styles.Accent.Render(row.shown())
		}
		return m.styles.Text.Render(row.shown())
	}
}

func padRight(text string, width int) string {
	if lipgloss.Width(text) >= width {
		return text
	}
	return text + strings.Repeat(" ", width-lipgloss.Width(text))
}

// The header says where the settings are written, which is the file a save
// puts them in. A path too long for the line keeps its end: the directory it
// ends in and the file itself are the parts worth reading.
func (m Model) header(line int) string {
	if m.settings.ConfigFile == "" {
		return frame.CutTo(" "+m.styles.Accent.Render("settings"), line-1)
	}
	path := m.settings.ConfigFile
	// The leading space and a margin beside it: a header that just about fits
	// is a header cut one character short of the file it is about.
	room := line - 5 - lipgloss.Width(" settings · ")
	if lipgloss.Width(path) > room {
		if room < 2 {
			path = ""
		} else {
			path = "…" + tailRunes(path, room-1)
		}
	}
	badge := m.styles.Accent.Render("settings")
	if path != "" {
		badge += m.styles.Faded.Render(" · " + path)
	}
	return frame.CutTo(" "+badge, line-1)
}

// tailRunes is the end of the text, counted in cells as a terminal counts them.
func tailRunes(text string, width int) string {
	runes := []rune(text)
	if len(runes) <= width {
		return text
	}
	return string(runes[len(runes)-width:])
}

func (m Model) footer(line int) string {
	switch {
	case m.editing >= 0:
		return frame.Spread(" "+m.hints(line-1,
			[2]string{"enter", "keep"},
			[2]string{"esc", "cancel"},
			[2]string{"ctrl+s", "save"},
		), "", line)

	case m.notice != nil:
		return frame.Spread(" "+m.styles.Danger.Render("✗ "+m.notice.Error()),
			m.styles.Key.Render("esc")+m.styles.Hint.Render(" dismiss"), line)

	case m.hint != "":
		right := m.styles.Key.Render("esc") + m.styles.Hint.Render(" dismiss")
		if m.leaving {
			right = m.styles.Key.Render("esc") + m.styles.Hint.Render(" close")
		}
		return frame.Spread(" "+m.styles.Badge.Render(m.hint), right, line)
	}

	shown := [][2]string{
		{"↑↓", "move"},
		m.rowKey(),
		{"s", "save"},
		{"esc", "close"},
	}
	return frame.Spread(" "+m.hints(line-1, shown...), "", line)
}

// rowKey is what the row under the cursor takes: arrows for a row with values
// to step through, enter for one that is written.
func (m Model) rowKey() [2]string {
	switch m.rows[m.cursor].kind {
	case choice, flag:
		return [2]string{"←→", "change"}
	default:
		return [2]string{"enter", "edit"}
	}
}

// hints is the keys on one line, each with the word for what it does, dropping
// the ones without room: half a key name is worth less than none.
func (m Model) hints(room int, shown ...[2]string) string {
	separator := m.styles.Hint.Render(" · ")
	line, width := "", 0
	for _, hint := range shown {
		drawn := m.styles.Key.Render(hint[0]) + m.styles.Hint.Render(" "+hint[1])
		needed := lipgloss.Width(drawn)
		if line != "" {
			needed += lipgloss.Width(separator)
		}
		if width+needed > room {
			break
		}
		if line != "" {
			line += separator
		}
		line += drawn
		width += needed
	}
	return line
}
