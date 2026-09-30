package overlay

import (
	"errors"
	"fmt"
	"os"
	"strings"

	"trans/internal/history"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

var (
	// errNoHistory is what ctrl+g says when nothing is written down: the record
	// can be turned off, and then there is no key to open.
	errNoHistory = errors.New("no record of sent prompts is kept — TRANS_HISTORY=1 writes them down")
	// errNothingSent is the record opened before anything has gone out.
	errNothingSent = errors.New("nothing sent yet — the record starts with the first prompt")
)

// recordDelivery writes the prompt down as it reached the agent. It happens
// while the box still holds the prompt, because the box is what was written.
func (m Model) recordDelivery(translated string) {
	if m.options.History == nil {
		return
	}
	if err := m.options.History.Record(m.draft.Value(), translated, m.delivery); err != nil {
		// The prompt is already delivered, so this cannot stop anything; the
		// log keeps what a plugin writes here, like a draft that could not be.
		fmt.Fprintln(os.Stderr, "trans: the sent prompt could not be recorded:", err)
	}
}

// historyKey is what the keys do while the record has the popup. Nothing here
// reaches the draft: a stray letter must not be typed into a box the author
// cannot see.
func (m Model) historyKey(key tea.KeyMsg) (tea.Model, tea.Cmd) {
	if !m.historyOpen {
		// The only key that gets here while the record is closed is ctrl+g.
		return m.openHistory()
	}

	rows := m.historyRows()
	switch {
	case key.Type == tea.KeyEsc, key.Type == tea.KeyCtrlG:
		return m.leaveHistory(), nil

	case key.Type == tea.KeyUp, key.String() == "k":
		return m.historyTo(m.historyAt - 1), nil
	case key.Type == tea.KeyDown, key.String() == "j":
		return m.historyTo(m.historyAt + 1), nil
	case key.Type == tea.KeyPgUp:
		return m.historyTo(m.historyAt - rows), nil
	case key.Type == tea.KeyPgDown, key.Type == tea.KeySpace:
		return m.historyTo(m.historyAt + rows), nil
	case key.Type == tea.KeyHome, key.String() == "g":
		return m.historyTo(0), nil
	case key.Type == tea.KeyEnd, key.String() == "G":
		return m.historyTo(len(m.historyList) - 1), nil

	case key.Type == tea.KeyEnter:
		return m.useHistoryEntry()
	case key.Type == tea.KeyDelete, key.Type == tea.KeyCtrlU:
		return m.dropHistoryEntry()
	}
	return m, nil
}

// openHistory is the record of delivered prompts on screen, newest first. The
// record is read once when the view opens, so it holds a snapshot: what was
// delivered while the view is open is for the next time.
func (m Model) openHistory() (tea.Model, tea.Cmd) {
	if m.options.History == nil {
		return m.raiseNotice(errNoHistory)
	}
	entries, err := m.options.History.Entries()
	if err != nil {
		return m.raiseNotice(err)
	}
	if len(entries) == 0 {
		return m, hint(errNothingSent.Error())
	}

	// The record takes the popup over from wherever it was: reading the
	// translation, a confirmation, a message — the draft is behind all of them.
	m.reading = false
	return m.withHistory(entries, 0), nil
}

// withHistory puts a fresh list on screen with one of its entries selected.
func (m Model) withHistory(list []history.Entry, at int) Model {
	m.historyOpen = true
	m.historyList = list
	m.historyAt, m.historyFrom = 0, 0
	return m.historyTo(at)
}

func (m Model) leaveHistory() Model {
	m.historyOpen = false
	m.historyList = nil
	m.historyAt, m.historyFrom = 0, 0
	return m
}

// historyRows is what the record gets of the popup: the same rows the
// translation has while reading, since the box holding it is the same box.
func (m Model) historyRows() int {
	return m.readingRows()
}

// historyTo moves the selection, keeping it on screen: the record scrolls the
// least it has to, the way the text area follows its own cursor.
func (m Model) historyTo(at int) Model {
	m.historyAt = min(max(at, 0), max(len(m.historyList)-1, 0))

	rows := m.historyRows()
	if m.historyAt < m.historyFrom {
		m.historyFrom = m.historyAt
	}
	if m.historyAt >= m.historyFrom+rows {
		m.historyFrom = m.historyAt - rows + 1
	}
	return m
}

// useHistoryEntry puts the prompt back in the box exactly as it was written
// the time it went out, with the translation that went with it beside it.
// Nothing is spent on it: the English was paid for when the prompt was first
// delivered, and writing it down is what the record is for.
func (m Model) useHistoryEntry() (tea.Model, tea.Cmd) {
	if len(m.historyList) == 0 {
		return m.leaveHistory(), nil
	}
	entry := m.historyList[m.historyAt]

	loaded := m.leaveHistory()
	loaded.reading = false
	loaded.stage = composing
	loaded.notice, loaded.hint, loaded.failure = nil, "", nil
	loaded.stopPreview()
	loaded.draft.Clear()
	loaded.draft.Resume(entry.Source)
	loaded.draftTop = 0
	loaded.resumed = false
	loaded.preview, loaded.previewOf, loaded.previewError =
		entry.Translation, entry.Source, nil
	loaded.refit()
	return loaded, nil
}

// dropHistoryEntry takes one prompt out of the record: the sentence that was
// only ever a test is not worth keeping. The last entry gone takes the view
// with it — an empty record is nothing to look at.
func (m Model) dropHistoryEntry() (tea.Model, tea.Cmd) {
	if len(m.historyList) == 0 {
		return m.leaveHistory(), nil
	}

	removed := m.historyList[m.historyAt]
	if err := m.options.History.Forget(&removed); err != nil {
		return m.leaveHistory().raiseNotice(err)
	}

	left := make([]history.Entry, 0, len(m.historyList)-1)
	left = append(left, m.historyList[:m.historyAt]...)
	left = append(left, m.historyList[m.historyAt+1:]...)
	if len(left) == 0 {
		return m.leaveHistory(), nil
	}
	return m.withHistory(left, m.historyAt), nil
}

// While the record is open it has every row the draft and its box were using:
// one prompt to a line, the newest at the top, the time beside what was
// written. The rows are the ones the translation gets while reading — the
// popup is the same popup.
func (m Model) historyView() string {
	rows := m.historyRows()
	total := len(m.historyList)
	width := m.contentWidth()

	shown := make([]string, 0, rows)
	for index := m.historyFrom; index < min(m.historyFrom+rows, total); index++ {
		shown = append(shown, m.historyLine(m.historyList[index], index == m.historyAt, width))
	}

	box := m.labelled(
		m.box(true).Height(rows).Render(
			m.scrolled(strings.Join(shown, "\n"), m.historyFrom, rows, total)),
		howFarThrough(m.historyFrom, rows, total), true)
	line := lipgloss.Width(box)
	return strings.Join([]string{m.header(line), box, m.historyFooter(line - 1)}, "\n")
}

// One prompt on one line: when it went out, and the first line of what was
// written, which is what is worth recognizing. The rest of a longer prompt is
// in the box the line opens into.
func (m Model) historyLine(entry history.Entry, selected bool, width int) string {
	when := entry.At.Format("01-02 15:04")
	first, _, _ := strings.Cut(entry.Source, "\n")
	first = strings.ReplaceAll(first, "\t", " ")

	// Two spaces between the time and the prompt, one of which the selection
	// marker takes: a selected line must not be wider than the others.
	body := cutTo(first, max(width-len(when)-3, 1))
	if selected {
		return m.styles.accent.Render("❯ ") +
			m.styles.faded.Render(when) + "  " +
			m.styles.accent.Render(body)
	}
	return "  " + m.styles.faded.Render(when) + "  " +
		m.styles.text.Render(body)
}

// The keys are few enough to name them all, and the way out is last.
func (m Model) historyFooter(inner int) string {
	hints := []string{
		m.styles.key.Render("↑↓") + m.styles.hint.Render(" move"),
		m.styles.key.Render("enter") + m.styles.hint.Render(" use it"),
		m.styles.key.Render("delete") + m.styles.hint.Render(" drop it"),
		m.styles.key.Render("esc") + m.styles.hint.Render(" back"),
	}
	return spread(" "+strings.Join(hints, m.styles.hint.Render(" · ")), "", inner)
}
