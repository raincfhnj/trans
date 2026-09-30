package selection

import (
	"strings"

	"trans/internal/frame"

	"github.com/charmbracelet/lipgloss"
)

func (m Model) View() string {
	selectedRows, translationRows := m.rows()
	content := m.contentWidth()

	selected := m.styles.Labelled(
		m.styles.Box(false, m.pane.Width).Height(selectedRows).
			Render(m.sourceBody(content, selectedRows)),
		"selection", false)

	text, style := m.translation()
	total := frame.RowsOf(text, content)
	body := style.Render(frame.RowsFrom(text, content, m.first, translationRows))
	translation := m.styles.Labelled(
		m.styles.Box(true, m.pane.Width).Height(translationRows).Render(
			m.styles.Scrolled(content, body, m.first, translationRows, total)),
		frame.HowFarThrough(m.first, translationRows, total), true)

	line := lipgloss.Width(translation)
	return strings.Join([]string{
		m.header(line),
		selected,
		translation,
		m.footer(line),
	}, "\n")
}

// rows is how the two boxes share the window: the selection takes what it
// needs up to its cap, and the translation gets the rest — it is the text being
// read here, while the selection is the one the author already knows.
func (m Model) rows() (selected, translation int) {
	available := max(m.pane.Height-popupBorder-headerRows-footerRows,
		minSelectedRows+minTranslationRows+popupBorder*2)

	selected = min(frame.RowsOf(m.selectedText(), m.contentWidth()), sourceRows,
		available-popupBorder*2-minTranslationRows)
	selected = max(selected, minSelectedRows)
	translation = max(available-selected-popupBorder*2, minTranslationRows)
	return selected, translation
}

// selectedText is the selection, or what stands in its place when the clipboard
// held nothing to translate.
func (m Model) selectedText() string {
	if strings.TrimSpace(m.options.Source) == "" {
		return m.nothingSelected()
	}
	return m.options.Source
}

func (m Model) nothingSelected() string {
	if strings.TrimSpace(m.options.SelectCopy) != "" {
		return "nothing was selected in the pane — the chord put nothing back"
	}
	return "nothing was selected — the clipboard was empty; " +
		"set TRANS_SELECT_COPY=ctrl+shift+c to copy from the pane"
}

// sourceBody is the selection itself, cut where it runs past its rows, and the
// line saying why there is none when nothing came.
func (m Model) sourceBody(content, rows int) string {
	if strings.TrimSpace(m.options.Source) == "" {
		return m.styles.Placeholder.Render(m.nothingSelected())
	}
	shown := frame.RowsFrom(m.options.Source, content, 0, rows)
	if frame.RowsOf(m.options.Source, content) > rows {
		shown = frame.CutTo(shown, content-2) + " …"
	}
	return m.styles.Text.Render(shown)
}

// translation is what goes in the big box: the answer, or why there is not one.
func (m Model) translation() (string, lipgloss.Style) {
	switch {
	case m.options.Trouble != nil:
		return "✗ " + m.options.Trouble.Error(), m.styles.Danger
	case m.options.WithoutService:
		return "no translation service is configured — the settings window sets one",
			m.styles.Placeholder
	case strings.TrimSpace(m.options.Source) == "":
		return "nothing to translate", m.styles.Placeholder
	case m.failure != nil:
		return "✗ " + m.failure.Error(), m.styles.Danger
	case m.translating && m.translated != "":
		// The last answer is still on screen: readable, but not the answer to
		// the request now running.
		return m.translated, m.styles.Faded
	case m.translating:
		return "…", m.styles.Placeholder
	default:
		return m.translated, m.styles.Text
	}
}

// The header says which service this is from and into which language; without
// a service it says so instead.
func (m Model) header(line int) string {
	service := m.options.Service
	if service == "" || m.options.WithoutService {
		service = "no translation"
	}
	badge := m.styles.Accent.Render(service)
	if language := m.options.Language; language != "" {
		badge += m.styles.Faded.Render(" → " + language)
	}
	return frame.CutTo(" "+badge, line-1)
}

func (m Model) footer(line int) string {
	if m.translating {
		return frame.Spread(
			" "+m.spinner.View()+m.styles.Accent.Render(" translating …"), "", line)
	}
	return frame.Spread(" "+m.keyHints(line-1), "", line)
}

// keyHints names the keys there are, dropping the ones without room: half a
// key name is worth less than none.
func (m Model) keyHints(room int) string {
	shown := [][2]string{}
	if m.failure != nil {
		shown = append(shown, [2]string{"ctrl+t", "try again"})
	}
	if m.translationTotal() > m.translationVisible() {
		shown = append(shown, [2]string{"↑↓", "read"})
	}
	shown = append(shown, [2]string{"esc", "close"})

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

func (m Model) contentWidth() int {
	return frame.ContentWidth(m.pane.Width)
}

func (m Model) translationVisible() int {
	_, rows := m.rows()
	return rows
}

func (m Model) translationTotal() int {
	text, _ := m.translation()
	return frame.RowsOf(text, m.contentWidth())
}
