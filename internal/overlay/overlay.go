// Package overlay is the terminal UI a user composes a draft prompt in.
package overlay

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"
	"sync"
	"time"

	"trans/internal/frame"
	"trans/internal/history"
	"trans/internal/promptflow"
	"trans/internal/vimarea"

	"github.com/charmbracelet/bubbles/key"
	"github.com/charmbracelet/bubbles/spinner"
	"github.com/charmbracelet/bubbles/textarea"
	tea "github.com/charmbracelet/bubbletea"
)

// Prompter is the draft's way out: translated for reading, delivered for the
// agent, or both at once when nothing has been previewed.
type Prompter interface {
	Submit(ctx context.Context, draft string, how promptflow.Delivery) (string, error)
	Translate(ctx context.Context, draft string) (string, error)
	Deliver(ctx context.Context, text string, how promptflow.Delivery) error
	Usage(ctx context.Context) (promptflow.Usage, bool, error)
}

type Options struct {
	Service  string
	Language string
	// WithoutService says there is nothing to translate with. The popup is then a
	// draft box: no second panel, no keys for a translation that cannot happen,
	// and the draft reaches the agent as it was written.
	WithoutService bool
	// Trouble is a service that was asked for and could not be built, said out
	// loud as soon as the popup opens.
	Trouble error
	// Review says the prompt is only typed into the agent's input, not sent.
	Review bool
	Vim    bool
	// Live translates the draft while it is written instead of only on send.
	Live bool
	// Confirm shows the English and waits for a second key before delivering.
	Confirm  bool
	Debounce time.Duration
	// MaxDraft is how long a prompt may get before the box says something. This
	// is a place for prompts, not for pasted files.
	MaxDraft int
	// Pulse fills and empties a circle beside "live" while a translation runs.
	Pulse bool
	// Logo signs the empty draft box with the plugin's braille mark.
	Logo bool
	// NoticeLinger is how long a message stays before it goes by itself.
	NoticeLinger time.Duration
	// Drafts keeps an unfinished prompt between sessions. Without one the draft
	// simply goes when the popup closes.
	Drafts Drafts
	// History is the record of prompts already delivered, which ctrl+g opens.
	// Without one there is no record and no key for it.
	History History
	// SendKey is how the key that hands a prompt over is named in the footer. A
	// terminal that keeps the chord for itself — Windows Terminal opens it full
	// screen — has the panel name the other key that sends.
	SendKey string
	// Cursor, when given, is told where the caret sits in every frame that is
	// drawn. A terminal that draws its own cursor there shows an input method's
	// pre-edit text where the writing is instead of at the foot of the panel;
	// only a console the console host draws can be moved that way.
	Cursor *CursorPlace
	// Read turns the panel around: the draft holds text to read — an agent's
	// reply, an error, anything already in English — and the second pane shows
	// it in the author's own language, which Language then names. Nothing is
	// delivered into the window; the key that sends elsewhere copies the result
	// to the clipboard.
	Read bool
	// Prefill opens with text already in the draft: what was on the clipboard,
	// or a selection captured from the window. It arrived rather than being
	// written, but unlike a draft from an earlier session it is exactly what
	// the panel was opened for, so live translation takes it up at once.
	Prefill string
	// PrefillTrouble is why there is nothing to start with when something was
	// asked for and did not come — a capture that found nothing, a chord that
	// would not press — said out loud as soon as the popup opens, like Trouble.
	PrefillTrouble error
	// Theme names the palette the panel is drawn with, so this window follows
	// the theme chosen for the program's other windows. An empty name is the
	// default palette, and a name nobody has heard of is no reason for the
	// popup to refuse to open.
	Theme string
	// DraftRows is how many rows the draft box asks for. Zero takes this
	// package's default, and either way the number is kept inside the ends
	// config.Load refuses, so a caller that never goes near config cannot ask
	// for a box no terminal can show.
	DraftRows int
	// PanelWidth is how wide the popup asks for, on the same terms as
	// DraftRows: zero takes the default, and the ends are config's own, so
	// what the settings window writes back is what the popup is sized to.
	PanelWidth int
}

// CursorPlace is a cell of the frame just drawn, counted the way a terminal
// counts: rows and columns from the top left, one-based. Lines is how many rows
// that frame had, which is where the terminal's cursor was left. The model
// writes it and whatever draws the frame reads it, which are not the same
// goroutine, so it looks after itself.
type CursorPlace struct {
	mu      sync.Mutex
	row     int
	column  int
	lines   int
	visible bool
}

// Where is the caret of the frame drawn last, and how many rows that frame had.
func (c *CursorPlace) Where() (row, column, lines int, visible bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.row, c.column, c.lines, c.visible
}

func (c *CursorPlace) place(row, column, lines int, visible bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.row, c.column, c.lines, c.visible = row, column, lines, visible
}

// Drafts is the pane's own unfinished prompt.
type Drafts interface {
	Load() (string, error)
	Save(text string) error
	Clear() error
}

// History is the record of prompts that already reached the agent. It is
// written on delivery and read back by ctrl+g, which is how a prompt sent last
// week can be sent again instead of being remembered and retyped.
type History interface {
	Record(source, translation string, how promptflow.Delivery) error
	Entries() ([]history.Entry, error)
	Forget(entry *history.Entry) error
}

// errNoService is what the keys for translating say when there is nothing to
// translate with.
var errNoService = errors.New("no translation service configured")

type stage int

const (
	composing stage = iota
	translating
	// confirming is the finished translation waiting to be let go.
	confirming
)

const (
	// draftHeight is the rows the draft box opens with — the same number
	// config.DefaultDraftRows holds, so the popup and the settings window
	// agree on one default rather than on two that could drift apart.
	draftHeight = 6
	// A prompt is rarely one line, so the draft keeps this many rows and scrolls.
	minDraftRows = 4
	// maxDraftRows is the other end config.Load refuses to write past. It is
	// repeated here rather than imported so that this package keeps drawing
	// for itself; the clamp below only catches a caller that skips config.
	maxDraftRows = 16
	// pastedAtOnce is how much text arriving in one keystroke counts as pasted
	// rather than written. Nobody types this much between two updates.
	pastedAtOnce = 200

	defaultDebounce     = 600 * time.Millisecond
	defaultMaxDraft     = 2_000
	defaultNoticeLinger = 5 * time.Second

	// The panel keeps some of a popup for its own frame: three columns and two
	// rows of the size asked for are frame rather than content.
	popupChromeColumns = 3
	popupChromeRows    = 2

	// PopupBorder is what a caller must add to a wanted height.
	PopupBorder = popupChromeRows

	englishRows = 5 // the pane holding the translation
	// draftFrame is the border around a box, boxPadding the column either side of
	// its content. Lipgloss counts a width as content plus padding, so what is
	// drawn inside a box is boxPadding narrower than the width it is given.
	draftFrame = 2
	boxPadding = 2
	headerRows = 1
	footerRows = 1

	// PopupWidth is what the panel asks for when it opens: wide enough for a
	// sentence of the translation to be read as it stands, and the dialog then
	// fills the popup exactly, leaving no unused space. It is the same number
	// config.DefaultPanelWidth holds, for the reason draftHeight gives.
	PopupWidth = 110
	// minPanelWidth and maxPanelWidth are the ends config.Load and the settings
	// window already refuse to step past. They are kept here as a second line
	// for a caller that sizes the popup without reading config: a panel too
	// narrow to read is no use, and one wider than any terminal is lost off
	// the edge of all of them.
	minPanelWidth = 60
	maxPanelWidth = 180
)

// PopupHeight leaves room for the translation whether live translation starts on
// or off: a popup cannot be resized once open, and ctrl+l must never translate
// into a pane with nowhere to show the result. With live off the draft has the
// room instead. The rows the draft asks for are counted here as well, so the
// height a caller reserves is the height the panel then fills.
func PopupHeight(draftRows int) int {
	return headerRows + footerRows + draftFrame + clampDraftRows(draftRows) + englishRows + PopupBorder
}

// clampDraftRows keeps a wanted row count between the ends config.Load refuses
// to write past, with zero meaning this package's default.
func clampDraftRows(rows int) int {
	if rows == 0 {
		return draftHeight
	}
	return min(max(rows, minDraftRows), maxDraftRows)
}

// clampPanelWidth does the same for the width the popup asks for, so a panel
// sized by a caller that never reads config still lands on a window a terminal
// can show.
func clampPanelWidth(width int) int {
	if width == 0 {
		return PopupWidth
	}
	return min(max(width, minPanelWidth), maxPanelWidth)
}

type Model struct {
	// ctx spans the whole overlay session; Bubble Tea commands are plain
	// closures, so there is nowhere else to carry it.
	ctx      context.Context
	prompter Prompter
	options  Options
	styles   styles
	// palette is the slots the panel is drawn from, chosen once for the whole
	// session so every frame of it is painted the same way.
	palette frame.Palette
	// contentCap is how wide the popup asked for, less the box frame: the
	// widest the writing may get, whichever pane the popup ends up in.
	contentCap int
	draft      vimarea.Model
	spinner    spinner.Model
	stage      stage
	// confirmWait is the number of the preview request the send key is waiting
	// on while Options.Confirm translates the draft before it may go out. It is
	// 0 whenever no confirmation is owed, so a cancelled preview reply can tell
	// whether the translating stage is waiting for it — and must then let go —
	// or whether the stage belongs to an ordinary send already on its way, which
	// the reply must not touch.
	confirmWait int
	// failure is a broken way out, so it stays on screen.
	failure error
	// loadFailure is a kept draft that could not be read, which the author has to
	// hear about: writing here and closing would write over it.
	loadFailure error
	// notice is something that did not work, hint something worth knowing. Both go
	// by themselves; escape is quicker.
	notice  error
	hint    string
	notices int
	width   int

	// preview holds the last translation and the draft it belongs to, so a
	// prompt the author has read is delivered as it stands.
	preview      string
	previewOf    string
	previewError error
	// revision counts draft changes; a reply for an older one is discarded.
	revision  int
	requested int
	// cancelPreview stops the translation started for an older draft. It is a
	// closure, so every copy of the model cancels the same request.
	cancelPreview context.CancelFunc
	// delivery can be changed while writing: sending or only typing is easier to
	// choose once the English is there to read.
	delivery promptflow.Delivery
	// reading gives the whole popup to the translation. Three rows beside the draft
	// are enough to glance at, not to read.
	reading bool
	// readingFrom is the first row of the translation on screen while reading.
	readingFrom int
	// historyOpen puts the record of delivered prompts on screen instead of the
	// draft, with the entry the arrows are on ready to be taken back.
	historyOpen bool
	historyList []history.Entry
	historyAt   int
	historyFrom int
	// draftTop is the first row of the draft on screen, kept here because the text
	// area does not say where its own view sits.
	draftTop int
	// resumed says the draft came from an earlier session, heldBackLive that this
	// is why live translation is off.
	resumed      bool
	heldBackLive bool
	// delivered says the agent has the prompt, so there is nothing left to keep.
	delivered       bool
	spent           promptflow.Usage
	spentKnown      bool
	beat            int
	pulsing         bool
	translationDone bool
	pane            tea.WindowSizeMsg
}

func New(ctx context.Context, prompter Prompter, options Options) Model {
	// The two layout settings are asked for in whole rows and columns and are
	// brought inside their ends here, once, so the model never has to wonder
	// whether a caller went past them.
	options.DraftRows = clampDraftRows(options.DraftRows)
	options.PanelWidth = clampPanelWidth(options.PanelWidth)
	palette := frame.PaletteFor(options.Theme)
	look := newStyles(palette)

	// Read mode keeps nothing: the text came from somewhere else, a reply to
	// read is not a prompt being written, and copying twice costs nothing — so
	// there is no store to save to and nothing to confirm before the copy.
	if options.Read {
		options.Drafts = nil
		options.Confirm = false
	}

	placeholder := "Write your prompt in your own language …"
	if options.Read {
		placeholder = "Paste or write what you want to read …"
	}
	draft := vimarea.New(
		vimarea.WithVim(options.Vim),
		vimarea.WithPlaceholder(placeholder),
		vimarea.WithStyles(look.text, look.placeholder, look.cursorFor(options.Cursor)),
	)
	draft.SetHeight(options.DraftRows)

	working := spinner.New()
	working.Spinner = spinner.Dot
	working.Style = look.accent

	if options.Debounce <= 0 {
		options.Debounce = defaultDebounce
	}
	if options.MaxDraft <= 0 {
		options.MaxDraft = defaultMaxDraft
	}
	if options.NoticeLinger <= 0 {
		options.NoticeLinger = defaultNoticeLinger
	}

	// What the popup asked for, less its own frame: the width the writing is
	// capped at, worked out before the model so that every resize — the one
	// here and the ones after a draft came back — caps against the same number.
	contentCap := options.PanelWidth - draftFrame

	model := Model{
		ctx:        ctx,
		prompter:   prompter,
		options:    options,
		styles:     look,
		palette:    palette,
		contentCap: contentCap,
		draft:      draft,
		spinner:    working,
		delivery:   deliveryFor(options.Review),
	}
	if options.WithoutService {
		model.options.Live = false
	}
	model.resize(model.contentCap)
	if options.Drafts != nil {
		kept, err := options.Drafts.Load()
		model.loadFailure = err
		if kept != "" {
			model.draft.Resume(kept)
			model.resumed = true
			// Whatever came back is paid for by the character if it is translated,
			// so it is not, until ctrl+l says so.
			model.heldBackLive = options.Live
			model.options.Live = false
			model.resize(model.contentCap)
		}
	}
	// Text that arrived rather than being written still does not mark the box:
	// it is what the panel was opened for, not yesterday's thinking.
	if options.Prefill != "" {
		model.draft.Resume(options.Prefill)
	}
	return model
}

type (
	// translated is the text the agent received, kept so the prompt can be
	// written down together with what was sent instead of written in twice.
	promptSentMsg   struct{ translated string }
	blankDraftMsg   struct{}
	submitFailedMsg struct{ err error }
	hintMsg         struct{ what string }
	// shown names the notice this timer belongs to, so a newer one stays up.
	noticeExpiredMsg struct{ shown int }

	previewDueMsg   struct{ revision int }
	previewReadyMsg struct {
		request int
		// of is the draft this translation was made from. Without it the
		// translation could be paired with a draft it never belonged to.
		of   string
		text string
		err  error
	}

	usageMsg struct {
		spent    promptflow.Usage
		reported bool
	}
)

func (m Model) Init() tea.Cmd {
	// The allowance is asked for after the box is already on screen, so nothing
	// waits for the network.
	started := []tea.Cmd{textarea.Blink, m.askUsage()}
	if m.options.Trouble != nil {
		started = append(started, refuse(m.options.Trouble))
	}
	if m.loadFailure != nil {
		started = append(started, refuse(m.loadFailure))
	}
	if m.heldBackLive {
		started = append(started, hint("resumed draft, so live is off — ctrl+l translates it"))
	}
	if m.options.PrefillTrouble != nil {
		started = append(started, refuse(m.options.PrefillTrouble))
	}
	// Prefilled text is there to be read, so live translation takes it up at
	// once rather than waiting for an edit. A wall of it is the exception this
	// box has always made: too long to translate is too long however it came.
	if m.options.Prefill != "" && m.options.Live && !m.draftIsTooLong() {
		started = append(started, m.schedulePreview())
	}
	return tea.Batch(started...)
}

func (m Model) askUsage() tea.Cmd {
	return func() tea.Msg {
		spent, reported, err := m.prompter.Usage(m.ctx)
		if err != nil {
			// A service that will not say is simply not shown.
			return usageMsg{}
		}
		return usageMsg{spent: spent, reported: reported}
	}
}

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.pane = msg
		m.resize(msg.Width - draftFrame)
		return m, nil

	case tea.KeyMsg:
		return m.handleKey(msg)

	case usageMsg:
		m.spent, m.spentKnown = msg.spent, msg.reported && msg.spent.Limit > 0
		return m, nil

	case pulseMsg:
		if !m.pulsing || msg.beat <= m.beat {
			return m, nil
		}
		beating := m.keepBreathing(msg.beat)
		return m, beating

	case promptSentMsg:
		// Read mode copies instead of delivering. The draft and the result stay
		// where they are — one copy may go to more than one place — and the way
		// out is what is said.
		if m.options.Read {
			m.stage = composing
			return m, hint("copied to clipboard · esc closes")
		}
		m.delivered = true
		// The record is written before the box empties, because the box is what
		// the prompt was written in.
		m.recordDelivery(msg.translated)
		m.forgetDraft()
		// Neither way of delivering closes the panel: a sent prompt and one that
		// was only typed into the agent's input both leave the box empty and
		// waiting for the next one, and closing is what esc is for.
		return m.emptied()

	case blankDraftMsg:
		return m.raiseNotice(promptflow.ErrBlankDraft)

	case submitFailedMsg:
		return m.raiseNotice(msg.err)

	case hintMsg:
		m.hint = msg.what
		ticking := m.startNoticeClock()
		return m, ticking

	case noticeExpiredMsg:
		if msg.shown == m.notices {
			m.notice, m.hint = nil, ""
		}
		return m, nil

	case previewDueMsg:
		// Starting a translation while sending would spend a second call and
		// leave the finished preview describing the wrong draft.
		if msg.revision != m.revision || m.stage == translating {
			return m, nil
		}
		breathing, cmd := m.startPreview()
		return breathing, tea.Batch(cmd, breathing.beginPulse())

	case previewReadyMsg:
		if errors.Is(msg.err, context.Canceled) {
			// A cancelled translation is normally passed over — except the one
			// the send key is waiting for. That translation will never come, so
			// the wait is abandoned and the panel goes back in front of the
			// draft, where the send key works again. Any other cancelled reply
			// belongs to a preview the stage never waited for and changes
			// nothing: an ordinary send on its way must keep the stage.
			if m.confirmWait != 0 && msg.request == m.confirmWait {
				m.abandonConfirm()
			}
			return m, nil
		}
		if msg.request != m.requested {
			return m, nil
		}
		if m.confirmWait != 0 && msg.request == m.confirmWait {
			m.confirmWait = 0
		}
		m.previewOf, m.preview, m.previewError = msg.of, msg.text, msg.err
		m.endPulse()
		m.refit()

		// A translation asked for in order to send it waits for the go-ahead.
		if m.stage == translating && m.options.Confirm {
			if msg.err != nil {
				return m.raiseNotice(msg.err)
			}
			m.stage = confirming
			m.refit()
		}
		return m, nil

	case spinner.TickMsg:
		if m.stage != translating {
			return m, nil
		}
		var cmd tea.Cmd
		m.spinner, cmd = m.spinner.Update(msg)
		return m, cmd
	}

	var cmd tea.Cmd
	m.draft, cmd = m.draft.Update(msg)
	m.followCursor()
	return m, cmd
}

// emptied is what is left after a prompt was sent: nothing in the box, nothing
// translated, and the same panel waiting for the next one.
func (m Model) emptied() (tea.Model, tea.Cmd) {
	m.delivered = false
	m.stage = composing
	m.confirmWait = 0
	m.stopPreview()
	m.draft.Clear()
	m.draftTop = 0
	m.resumed = false
	m.preview, m.previewOf, m.previewError = "", "", nil
	m.refit()

	notice := "sent — the box is empty, ready for the next one"
	if m.delivery == promptflow.Typing {
		notice = "typed into the agent's input — press enter there; the box is ready for the next one"
	}
	return m, hint(notice)
}

// raiseNotice puts a message up and starts the clock that takes it down again.
func (m Model) raiseNotice(why error) (tea.Model, tea.Cmd) {
	m.stage = composing
	m.confirmWait = 0
	m.notice = why
	ticking := m.startNoticeClock()
	return m, ticking
}

func hint(what string) tea.Cmd {
	return func() tea.Msg { return hintMsg{what: what} }
}

func refuse(why error) tea.Cmd {
	return func() tea.Msg { return submitFailedMsg{err: why} }
}

func (m *Model) startNoticeClock() tea.Cmd {
	m.notices++
	shown := m.notices
	return tea.Tick(m.options.NoticeLinger, func(time.Time) tea.Msg {
		return noticeExpiredMsg{shown: shown}
	})
}

// refit gives the boxes their rows again after something changed which of them is
// on screen: a translation arriving, one failing, a confirmation opening.
func (m *Model) refit() {
	if m.pane.Width > 0 {
		m.resize(m.pane.Width - draftFrame)
	}
}

func (m *Model) resize(contentWidth int) {
	// Never wider than the pane, and never wider than the popup asked for. A
	// width clamped up past either wraps every line the popup draws, and an
	// inline renderer then stacks frame on frame.
	m.width = min(max(contentWidth, 1), m.contentCap)
	// The text area wraps at the width the box shows, or the box wraps again what
	// the area already wrapped and words drop onto lines nobody asked for.
	m.draft.SetWidth(m.contentWidth())
	m.draft.SetHeight(m.draftRows())
	m.followCursor()
}

func (m Model) translating() bool { return !m.options.WithoutService }

// sendKey is what the footer calls the key that sends, which is not the same
// key everywhere.
func (m Model) sendKey() string {
	if m.options.SendKey != "" {
		return m.options.SendKey
	}
	return "alt+enter"
}

func deliveryFor(review bool) promptflow.Delivery {
	if review {
		return promptflow.Typing
	}
	return promptflow.Sending
}

// switchDelivery is ctrl+r: whether the prompt is sent or only typed is a
// decision worth making once the English can be read, not when the popup opens.
func (m Model) switchDelivery() Model {
	if m.delivery == promptflow.Sending {
		m.delivery = promptflow.Typing
	} else {
		m.delivery = promptflow.Sending
	}
	return m
}

// switchLive is ctrl+l: translating while writing costs characters, so it can be
// turned on for a sentence that needs watching and off again.
func (m Model) switchLive() (Model, tea.Cmd) {
	m.options.Live = !m.options.Live
	m.resize(m.pane.Width - draftFrame)

	if !m.options.Live {
		m.stopPreview()
		m.pulsing = false
		return m, nil
	}

	m.revision++
	return m, m.schedulePreview()
}

func (m Model) draftRows() int {
	if m.pane.Height <= 0 {
		return m.options.DraftRows
	}

	rows := m.pane.Height - headerRows - footerRows - draftFrame
	if m.showsEnglish() {
		rows -= englishRows
	}
	return max(rows, 1)
}

// A pane too short for both boxes drops the translation rather than overflow.
func (m Model) showsEnglish() bool {
	if !m.translating() {
		return false
	}
	// With live translation off there is nothing to show until one is asked for.
	if !m.options.Live && m.stage != confirming && m.preview == "" && m.previewError == nil {
		return false
	}
	if m.pane.Height <= 0 {
		return true
	}
	return m.pane.Height-headerRows-footerRows-draftFrame-englishRows >= minDraftRows
}

// handleKey is the dispatcher: where the panel is decides which handler a key
// belongs to, and only what is left over is a question about the key itself.
//
// The order below is the stacking order of the popup, and it is load-bearing:
// the record and the full-screen reading are drawn in front of the draft, so
// they are asked for a key first. A letter must never be typed into a box the
// author cannot see.
func (m Model) handleKey(pressed tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch {
	// ctrl+c is not the panel's to give away: it closes whether a draft is
	// half-written, a translation is on its way, or a message is up.
	case key.Matches(pressed, keys.Quit):
		return m, tea.Quit

	// The record of delivered prompts answers to its own keys while it is open,
	// and ctrl+g opens it from wherever the panel is — which is why that key is
	// asked for here rather than inside the record's own handler.
	case m.historyOpen || key.Matches(pressed, keys.History):
		return m.historyKey(pressed)

	// Tab turns the translation around from either side of it, so it is asked
	// for before the reading pane gets the chance to claim it.
	case key.Matches(pressed, keys.Flip):
		return m.flipReading(), nil

	case m.reading:
		return m.readKey(pressed)
	}

	// Escape is asked for next because what it means depends on where the panel
	// is; a vim edit mode that claims it passes it on to the draft from there.
	if key.Matches(pressed, keys.Escape) {
		return m.handleEscape(pressed)
	}

	// These are the panel's own keys, and they mean the same thing in every
	// stage, so no one mode handler owns them.
	switch {
	case key.Matches(pressed, keys.Delivery):
		return m.switchDelivery(), nil

	// With nothing to translate with, the two keys that ask for a translation
	// say so instead of doing nothing at all. The guard is asked first because
	// the cases below it would otherwise take both keys.
	case !m.translating() && key.Matches(pressed, keys.Live, keys.Translate):
		return m.raiseNotice(errNoService)

	case key.Matches(pressed, keys.Live):
		return m.switchLive()

	case key.Matches(pressed, keys.Translate):
		return m.translateNow()

	case key.Matches(pressed, keys.Clear):
		return m.clearDraft()
	}

	// What is left is the send key, which the stage has a say in, or a key the
	// draft box takes.
	if m.stage == confirming {
		return m.handleConfirmKey(pressed)
	}
	return m.handleComposingKey(pressed)
}

// handleEscape is what escape means where the panel is. Closing keeps the draft,
// so it is always the way out; what comes before that are the exceptions, in the
// order they matter, and only a vim edit mode ever gets the key instead.
func (m Model) handleEscape(pressed tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch {
	// A read-mode panel goes at once, whatever is on screen: the text came from
	// somewhere else and nothing here is kept for next time, so escape is the
	// way out even while a message is up. Leaving an edit mode is the one
	// exception — a vim author's fingers expect escape out of insert before
	// anything else.
	case m.options.Read && (!m.draft.Modal() || m.draft.Mode() == vimarea.Normal):
		return m.close()

	// Otherwise escape takes the message away first. It must not also close the
	// popup.
	case m.notice != nil || m.hint != "":
		m.notice, m.hint = nil, ""
		return m, nil

	// A finished translation waiting to be let go is turned down rather than
	// closed: the draft it was made from is still there to carry on with.
	case m.stage == confirming:
		m.stage = composing
		m.confirmWait = 0
		m.refit()
		return m, nil

	case !m.draft.Modal():
		return m.close()

	// Escape is the whole way out: insert mode first, then the popup. Nothing is
	// lost by it, since closing keeps the draft.
	case m.draft.Mode() == vimarea.Normal:
		return m.close()
	}

	// An edit mode claimed it, so leaving insert or visual is the draft box's
	// business: the key goes on to the box like any other.
	return m.handleTypingKey(pressed)
}

// handleComposingKey is the keys while the draft box is what is in front, with
// nothing waiting to be agreed to: the send key starts a send.
func (m Model) handleComposingKey(pressed tea.KeyMsg) (tea.Model, tea.Cmd) {
	if key.Matches(pressed, keys.Send) {
		// A send already on its way answers for the send key inside startSubmit
		// — a second one would put the same prompt in front of the agent twice
		// — so the translating stage needs nothing said about it here.
		return m.startSubmit()
	}
	return m.handleTypingKey(pressed)
}

// handleConfirmKey is the keys while a finished translation waits for the
// go-ahead: the send key is that go-ahead, and anything else is writing, which
// takes the question back (see handleTypingKey).
func (m Model) handleConfirmKey(pressed tea.KeyMsg) (tea.Model, tea.Cmd) {
	if key.Matches(pressed, keys.Send) {
		return m.deliverPreview()
	}
	return m.handleTypingKey(pressed)
}

// handleTypingKey is the key no panel has claimed: the draft box takes it, and
// what the box then says decides what follows — a confirmation is taken back, a
// resumed draft becomes this session's, a pasted wall turns live translation
// off, and a shorter edit asks for a translation again.
func (m Model) handleTypingKey(pressed tea.KeyMsg) (tea.Model, tea.Cmd) {
	// Writing anything is a fresh start for the panel's messages: a way out
	// that did not work, or a notice, is about the attempt before this one.
	m.failure, m.notice, m.hint = nil, nil, ""
	before := m.draft.Value()

	var cmd tea.Cmd
	m.draft, cmd = m.draft.Update(pressed)
	m.followCursor()

	if m.draft.Value() != before && m.stage == confirming {
		m.stage = composing
		m.refit()
	}
	if m.draft.Value() != before {
		// Once it has been edited it is this session's draft, not an old one.
		m.resumed = false
	}

	// A wall of text arriving at once was pasted, not written, and paying to
	// translate it is a decision, not a side effect of pasting.
	if added := grown(before, m.draft.Value()); m.options.Live && added >= pastedAtOnce {
		m.options.Live = false
		m.resize(m.pane.Width - draftFrame)
		return m, tea.Batch(cmd, hint(fmt.Sprintf(
			"%d characters pasted, so live is off — ctrl+l translates it", added)))
	}

	// Translating a pasted wall of text again after every pause would spend the
	// allowance on something this box is not for.
	if after := m.draft.Value(); m.options.Live && after != before && !m.draftIsTooLong() {
		m.revision++
		m.stopPreview()
		m.previewError = nil
		return m, tea.Batch(cmd, m.schedulePreview())
	}
	return m, cmd
}

// translateNow is ctrl+t: translate what is there now, which is how a
// translation that did not arrive is asked for again, and how the English is
// read at all when live translation is off. A confirmation that asked for a
// translation waits for this one now instead of the one it started.
func (m Model) translateNow() (tea.Model, tea.Cmd) {
	m.previewError = nil
	m.refit()
	m.revision++
	translating, cmd := m.startPreview()
	if translating.confirmWait != 0 {
		translating.confirmWait = translating.requested
	}
	return translating, tea.Batch(cmd, translating.beginPulse())
}

// clearDraft is ctrl+u: the whole draft goes, as ctrl+u clears a line in a
// shell. The text area would otherwise use it to delete back to the line start.
// What the panel had translated for that draft goes with it, so nothing that
// belonged to a draft which is gone can still be delivered.
func (m Model) clearDraft() (tea.Model, tea.Cmd) {
	m.stage = composing
	m.confirmWait = 0
	m.draft.Clear()
	m.draftTop = 0
	m.forgetDraft()
	m.resumed = false
	m.preview, m.previewOf, m.previewError = "", "", nil
	return m, nil
}

// grown is how much longer the draft got, counted in characters rather than bytes.
func grown(before, after string) int {
	return len([]rune(after)) - len([]rune(before))
}

func (m Model) draftIsTooLong() bool {
	return len([]rune(m.draft.Value())) > m.options.MaxDraft
}

func (m Model) schedulePreview() tea.Cmd {
	revision := m.revision
	return tea.Tick(m.options.Debounce, func(time.Time) tea.Msg {
		return previewDueMsg{revision: revision}
	})
}

func (m Model) startPreview() (Model, tea.Cmd) {
	draft := m.draft.Value()
	m.requested++
	request := m.requested

	m.stopPreview()
	previewCtx, cancel := context.WithCancel(m.ctx)
	m.cancelPreview = cancel

	return m, func() tea.Msg {
		translated, err := m.prompter.Translate(previewCtx, draft)
		if errors.Is(err, promptflow.ErrBlankDraft) {
			return previewReadyMsg{request: request, of: draft}
		}
		return previewReadyMsg{request: request, of: draft, text: translated, err: err}
	}
}

// translateForConfirmation reuses the preview machinery, so the finished text
// arrives as a reply and the confirmation shows it.
func (m Model) translateForConfirmation() (tea.Model, tea.Cmd) {
	confirming, cmd := m.startPreview()
	confirming.stage = translating
	// The stage waits for exactly this request; while confirmWait says so, a
	// cancelled reply for it lets go of the wait instead of stranding it.
	confirming.confirmWait = confirming.requested
	return confirming, tea.Batch(m.spinner.Tick, cmd)
}

// abandonConfirm lets go of a confirmation translation that was cancelled on
// its way. Nothing is on its way out, so the panel returns to the draft: the
// send key is at work again and any preview already scheduled can run.
func (m *Model) abandonConfirm() {
	m.confirmWait = 0
	m.stage = composing
	m.refit()
}

func (m Model) deliverPreview() (tea.Model, tea.Cmd) {
	// The translation on screen is what the author agreed to send. If the draft
	// has moved on since, there is nothing to agree to yet.
	if !m.previewIsCurrent() {
		return m.startSubmit()
	}

	prompt := m.preview
	m.stage = translating

	return m, tea.Batch(m.spinner.Tick, func() tea.Msg {
		if err := m.prompter.Deliver(m.ctx, prompt, m.delivery); err != nil {
			return submitFailedMsg{err: err}
		}
		return promptSentMsg{translated: prompt}
	})
}

func (m *Model) stopPreview() {
	if m.cancelPreview != nil {
		m.cancelPreview()
		m.cancelPreview = nil
	}
}

func (m Model) startSubmit() (tea.Model, tea.Cmd) {
	// A send already on its way answers for the send key; starting a second one
	// here would put the same prompt in front of the agent twice.
	if m.stage == translating {
		return m, nil
	}

	draft := m.draft.Value()
	m.stage = translating
	m.confirmWait = 0
	m.notice = nil
	m.stopPreview()

	// Nothing to confirm about an empty draft.
	if m.options.Confirm && strings.TrimSpace(draft) == "" {
		return m.raiseNotice(promptflow.ErrBlankDraft)
	}

	// Read once, sent as it stands: a translation already on screen is what the
	// author means, whether it was asked for or arrived while writing.
	if m.options.Confirm && m.previewIsCurrent() {
		m.stage = confirming
		return m, nil
	}
	if m.options.Confirm {
		return m.translateForConfirmation()
	}

	// A preview the author has just read needs no second translation.
	if m.previewIsCurrent() {
		preview := m.preview
		return m, tea.Batch(m.spinner.Tick, func() tea.Msg {
			if err := m.prompter.Deliver(m.ctx, preview, m.delivery); err != nil {
				return submitFailedMsg{err: err}
			}
			return promptSentMsg{translated: preview}
		})
	}

	return m, tea.Batch(m.spinner.Tick, func() tea.Msg {
		translated, err := m.prompter.Submit(m.ctx, draft, m.delivery)
		switch {
		case errors.Is(err, promptflow.ErrBlankDraft):
			return blankDraftMsg{}
		case err != nil:
			return submitFailedMsg{err: err}
		default:
			return promptSentMsg{translated: translated}
		}
	})
}

// close keeps the draft for next time. Failing to keep it is worth saying,
// because closing anyway would throw the writing away; ctrl+c is the way out.
func (m Model) close() (tea.Model, tea.Cmd) {
	if m.options.Drafts == nil {
		return m, tea.Quit
	}
	if err := m.options.Drafts.Save(m.draft.Value()); err != nil {
		m.failure = err
		return m, nil
	}
	return m, tea.Quit
}

// KeepUnfinished stores the draft after the program ended, whichever way it
// ended: a closed popup and ctrl+c both skip the close key.
func (m Model) KeepUnfinished() error {
	if m.options.Drafts == nil || m.delivered {
		return nil
	}
	return m.options.Drafts.Save(m.draft.Value())
}

func (m Model) forgetDraft() {
	if m.options.Drafts == nil {
		return
	}
	if err := m.options.Drafts.Clear(); err != nil {
		// The prompt is already delivered, so this cannot stop anything; the
		// winlog package keeps what this program writes down in its log.
		fmt.Fprintln(os.Stderr, "trans: the sent draft could not be forgotten:", err)
	}
}

func (m Model) previewIsCurrent() bool {
	return m.preview != "" && m.previewError == nil && m.previewOf == m.draft.Value()
}
