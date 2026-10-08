package overlay

import (
	"fmt"
	"strconv"
	"strings"

	"trans/internal/frame"
	"trans/internal/promptflow"
	"trans/internal/vimarea"

	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
)

func (m Model) View() string {
	if m.historyOpen {
		m.reportCursor(0, false)
		return m.historyView()
	}
	if m.reading {
		m.reportCursor(0, false)
		return m.readingView()
	}

	first, visible, total := m.draftScroll()
	draft := frame.Labelled(&m.styles.mark, &m.styles.accent, &m.styles.badge,
		m.palette.Box(true, m.width).Render(m.draftBody()),
		frame.HowFarThrough(first, visible, total), true)
	line := lipgloss.Width(draft)

	parts := []string{m.header(line), draft}
	if m.showsEnglish() {
		parts = append(parts, m.englishPane())
	}
	parts = append(parts, m.footer(line))

	// The window the popup runs in is frame enough; a second one inside it only
	// takes room from the draft.
	drawn := strings.Join(parts, "\n")
	m.reportCursor(strings.Count(drawn, "\n")+1, true)
	return drawn
}

// The box's own furniture, in cells: the header above it, its top border, and
// the border and padding on its left. What is left is where the writing is.
const (
	firstDraftRow    = headerRows + 1
	firstDraftColumn = 1 + boxPadding/2
)

// reportCursor tells a terminal where the caret sits, so that an input method's
// pre-edit text appears where the writing is. Rows and columns are counted the
// way a terminal counts them, from one.
func (m Model) reportCursor(lines int, drafting bool) {
	if m.options.Cursor == nil {
		return
	}
	if !drafting {
		m.options.Cursor.place(0, 0, 0, false)
		return
	}
	m.options.Cursor.place(
		firstDraftRow+m.cursorRow()-m.draftTop+1,
		firstDraftColumn+m.draft.CharOffset()+1,
		lines,
		true,
	)
}

// While reading, the translation has every row the draft and its box were using.
func (m Model) readingView() string {
	rows := m.readingRows()
	total := m.readingTotal()

	text, style := m.english()
	if m.preview == "" && m.previewError == nil {
		text = "nothing translated yet"
	}
	shown := style.Render(frame.RowsFrom(text, m.contentWidth(), m.readingFrom, rows))

	box := frame.Labelled(&m.styles.mark, &m.styles.accent, &m.styles.badge,
		m.palette.Box(true, m.width).Height(rows).Render(
			m.scrolled(shown, m.readingFrom, rows, total)),
		frame.HowFarThrough(m.readingFrom, rows, total), true)
	line := lipgloss.Width(box)

	return strings.Join([]string{m.header(line), box, m.readingFooter(line - 1)}, "\n")
}

// The mark is only ever drawn over space nobody is using, so it goes as soon as
// there is a draft to read.
func (m Model) draftBody() string {
	body := m.draft.View()
	if m.options.Logo && m.draft.Value() == "" {
		body = m.sign(body)
	}
	first, visible, total := m.draftScroll()
	return m.scrolled(body, first, visible, total)
}

// Beside the draft there is room to glance at the translation, not to read it, so
// the panel shows where it starts and marks that it goes on. Tab is what reads it.
// The text is fitted before it is styled: escape sequences in the middle of it
// would decide where the lines break.
func (m Model) englishPane() string {
	text, style := m.english()

	rows := englishRows - 2
	shown := frame.RowsFrom(text, m.contentWidth(), 0, rows)
	if m.translationIsCut() {
		shown = frame.CutTo(shown, m.contentWidth()-2) + " …"
	}
	return m.palette.Box(false, m.width).Height(rows).Render(style.Render(shown))
}

func (m Model) translationIsCut() bool {
	return m.preview != "" && frame.RowsOf(m.preview, m.contentWidth()) > englishRows-2
}

// english is the translation as it stands, and how it should read: dimmed while it
// belongs to an older draft, in the danger colour when the service said no.
func (m Model) english() (string, lipgloss.Style) {
	switch {
	case m.previewError != nil:
		return "✗ " + m.previewError.Error(), m.styles.danger
	case m.preview == "":
		return "…", m.styles.placeholder
	case !m.previewIsCurrent():
		return m.preview, m.styles.placeholder
	default:
		return m.preview, m.styles.text
	}
}

// The window carries this program's name in its title, so the heading is only
// the badge saying what will happen. It starts one column in, under that title,
// and is cut rather than allowed to wrap onto the draft.
func (m Model) header(line int) string {
	return lipgloss.NewStyle().MaxWidth(line - 1).Render(" " + m.badge())
}

// The badge is joined from rendered pieces: the pulse carries its own colour,
// which a single Render around everything would cut short.
func (m Model) badge() string {
	// Read mode says so first: the rest of the heading — which language, which
	// key — reads differently there, and this is the word that says why.
	way := []string{}
	if m.options.Read {
		way = []string{m.styles.badge.Render("read"), m.styles.badge.Render("·")}
	}

	if !m.translating() {
		return strings.Join(append(way,
			m.styles.badge.Render("no translation · "+m.whatHappens())),
			m.styles.badge.Render(" "))
	}

	pieces := way
	// A glyph and the word follow the language, always the same width, so
	// switching live translation on or off does not shift everything after it
	// along the line.
	pieces = append(pieces,
		m.styles.badge.Render(m.options.Service+" → "+m.options.Language),
		m.styles.badge.Render("·"),
		m.liveState(),
		m.styles.badge.Render("· "+m.whatHappens()),
	)
	if m.resumed {
		pieces = append(pieces, m.styles.badge.Render("· resumed draft"))
	}
	if m.spentKnown {
		// Services bill translation by the character, so that is what the number
		// counts, and it is the month's allowance it counts against.
		pieces = append(pieces,
			m.styles.badge.Render("· "+spentAsWords(m.spent)+" chars this month"))
	}
	return strings.Join(pieces, m.styles.badge.Render(" "))
}

// A circle that breathes while translating, or a struck-through word when live
// translation is off. Both are the same width, so nothing after them moves.
func (m Model) liveState() string {
	if m.options.Live {
		glyph := m.styles.bright.Render("●")
		if m.options.Pulse {
			glyph = m.pulseGlyph()
		}
		return glyph + m.styles.badge.Render(" live")
	}
	return m.styles.badge.Render("✘") + m.styles.off.Render(" live")
}

// outcome is what the key that hands the text over does, named twice: in the
// words the heading has room for, and in the one word the footer has room for.
// Read mode copies instead of delivering, and where there is no other way to
// choose — there choosing stops — the second name stays empty.
type outcome struct {
	words string
	word  string
	other string
}

func (m Model) sendKeyDoes() outcome {
	switch {
	case m.options.Read:
		return outcome{words: "copies to clipboard", word: "copy"}
	case m.delivery == promptflow.Typing:
		return outcome{words: "fills the input", word: "fill", other: "send"}
	default:
		return outcome{words: "sends to agent", word: "send", other: "fill"}
	}
}

// The heading has room to say what will happen in words; the footer, which has
// to hold every key, names the same thing in one. Both readings are padded to
// one width so switching between them moves nothing.
func (m Model) whatHappens() string {
	const widest = len("fills the input")
	return fmt.Sprintf("%-*s", widest, m.sendKeyDoes().words)
}

func (m Model) destination() string { return m.sendKeyDoes().word }

func (m Model) otherDestination() string { return m.sendKeyDoes().other }

// escapeSays is what the footer promises escape does while a message is up. It
// takes the message away in a panel that keeps its draft; read mode keeps
// nothing for next time, so escape is the way out there and the footer says so.
func (m Model) escapeSays(action string) string {
	if m.options.Read {
		action = "close"
	}
	return m.styles.key.Render("esc") + m.styles.hint.Render(" "+action)
}

// The mode sits at the end of the line, next to the key that changes it.
func (m Model) footer(line int) string {
	mode := ""
	if m.draft.Modal() {
		mode = m.styles.mode.Render(m.draft.Mode().String())
	}

	// One column in on both sides, like the heading.
	inner := line - 1

	switch {
	case m.failure != nil:
		return frame.Spread(" "+m.styles.danger.Render("✗ "+m.failure.Error()),
			m.styles.hint.Render("ctrl+c close"), inner)
	case m.notice != nil:
		return frame.Spread(" "+m.styles.danger.Render("✗ "+m.notice.Error()),
			m.escapeSays("dismiss"), inner)
	case m.hint != "":
		// A read-mode hint says how to leave in its own words, so the side of
		// the line stays empty rather than repeating them.
		side := m.escapeSays("dismiss")
		if m.options.Read {
			side = ""
		}
		return frame.Spread(" "+m.styles.badge.Render(m.hint), side, inner)
	case m.draftIsTooLong():
		return frame.Spread(
			" "+m.styles.danger.Render(fmt.Sprintf("⚠ %d characters", len([]rune(m.draft.Value()))))+
				m.styles.hint.Render(" — this box is for prompts you write, not files you paste"),
			m.styles.hint.Render("ctrl+u discard"), inner)
	case m.stage == confirming:
		return frame.Spread(
			" "+m.styles.key.Render("ctrl+d")+m.styles.hint.Render(" send this")+
				m.styles.hint.Render(" · ")+m.styles.key.Render("esc")+
				m.styles.hint.Render(" keep writing"),
			m.styles.badge.Render("read it first"), inner)
	case m.stage == translating:
		return frame.Spread(" "+m.spinner.View()+m.styles.accent.Render(" translating …"), mode, inner)
	default:
		return frame.Spread(" "+m.keyHints(roomBeside(mode, inner)), mode, inner)
	}
}

// roomBeside is what a line has left for the hints once the mode has its place.
func roomBeside(mode string, inner int) int {
	room := inner - 1
	if mode != "" {
		room -= ansi.StringWidth(mode) + 1
	}
	return room
}

// Every key fits on the line as long as each is named in one word, so there is
// nothing to go looking for. The vim bindings are the exception, and they are in
// the readme rather than the footer.
func (m Model) keyHints(room int) string {
	shown := [][2]string{{m.sendKey(), m.destination()}}

	if m.translating() {
		// A translation that did not arrive is worth another try before anything
		// else, and reading it is only worth naming while some is out of sight.
		if m.previewError != nil {
			shown = append(shown, [2]string{"ctrl+t", "try again"})
		}
		if m.translationIsCut() {
			shown = append(shown, [2]string{"tab", "read it"})
		}
	}

	// Choosing between sending and filling is a question where something is
	// delivered. Read mode copies — the front of the footer already says so —
	// and has no second way to switch to.
	if other := m.otherDestination(); other != "" {
		shown = append(shown, [2]string{"ctrl+r", "→ " + other})
	}
	if m.translating() {
		shown = append(shown, [2]string{"ctrl+l", "live"})
	}

	// The record of prompts already sent is only a key while there is one.
	if m.options.History != nil {
		shown = append(shown, [2]string{"ctrl+g", "history"})
	}

	// An arrow reads as "takes you to", which a bare mode name does not.
	switch {
	case m.draft.Modal() && m.draft.Mode() == vimarea.Normal:
		shown = append(shown, [2]string{"i", "→ insert"}, [2]string{"esc", "close"})
	case m.draft.Modal():
		shown = append(shown, [2]string{"ctrl+u", "clear"}, [2]string{"esc", "→ normal"})
	default:
		shown = append(shown, [2]string{"ctrl+u", "clear"}, [2]string{"esc", "close"})
	}

	// A pane too narrow for every key shows the ones it has room for. Half a key
	// name is worth less than none.
	//
	// The room is measured on the plain words: the styles colour a hint
	// without padding it, so what a hint draws is as wide as what it says.
	// Measuring before drawing also means a hint the line has no room for is
	// never drawn at all, and the line itself is written once into a builder
	// rather than copied anew for every hint appended to it.
	separator := m.styles.hint.Render(" · ")
	separatorWidth := ansi.StringWidth(separator)
	var line strings.Builder
	width := 0
	for _, hint := range shown {
		needed := ansi.StringWidth(hint[0]) + 1 + ansi.StringWidth(hint[1])
		if width > 0 {
			needed += separatorWidth
		}
		if width+needed > room {
			break
		}
		if width > 0 {
			line.WriteString(separator)
		}
		line.WriteString(m.styles.key.Render(hint[0]))
		line.WriteString(m.styles.hint.Render(" " + hint[1]))
		width += needed
	}
	return line.String()
}

// The keys are few enough here to name them all, and the last one is the way out.
func (m Model) readingFooter(inner int) string {
	hints := []string{
		m.styles.key.Render("↑↓") + m.styles.hint.Render(" read"),
		m.styles.key.Render("tab") + m.styles.hint.Render(" → write"),
	}
	if m.previewError != nil {
		hints = append(hints, m.styles.key.Render("ctrl+t")+m.styles.hint.Render(" try again"))
	}
	hints = append(hints,
		m.styles.key.Render(m.sendKey())+m.styles.hint.Render(" "+m.destination()),
		m.styles.key.Render("esc")+m.styles.hint.Render(" back"))
	return frame.Spread(" "+strings.Join(hints, m.styles.hint.Render(" · ")), "", inner)
}

// Short enough for a header: 12.3k/1M.
func spentAsWords(spent promptflow.Usage) string {
	return compactCount(spent.Used) + "/" + compactCount(spent.Limit)
}

func compactCount(count int64) string {
	switch {
	case count >= 1_000_000:
		return trimZero(float64(count)/1_000_000) + "M"
	case count >= 1_000:
		return trimZero(float64(count)/1_000) + "k"
	default:
		return strconv.FormatInt(count, 10)
	}
}

func trimZero(value float64) string {
	rendered := strconv.FormatFloat(value, 'f', 1, 64)
	return strings.TrimSuffix(rendered, ".0")
}
