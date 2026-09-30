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

	"trans/internal/history"
	"trans/internal/promptflow"
	"trans/internal/vimarea"

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
	minContentWidth = 32
	maxContentWidth = 108
	draftHeight     = 6
	// A prompt is rarely one line, so the draft keeps this many rows and scrolls.
	minDraftRows = 4
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

	dialogRows  = 10 // heading, draft box and footer
	englishRows = 5  // the pane holding the translation
	// draftFrame is the border around a box, boxPadding the column either side of
	// its content. Lipgloss counts a width as content plus padding, so what is
	// drawn inside a box is boxPadding narrower than the width it is given.
	draftFrame = 2
	boxPadding = 2
	headerRows = 1
	footerRows = 1

	// PopupWidth is what the panel asks for when it opens: wide enough for a
	// sentence of the translation to be read as it stands, and the dialog then
	// fills the popup exactly, leaving no unused space.
	PopupWidth = 110
)

// PopupHeight leaves room for the translation whether live translation starts on
// or off: a popup cannot be resized once open, and ctrl+l must never translate
// into a pane with nowhere to show the result. With live off the draft has the
// room instead.
func PopupHeight() int {
	return dialogRows + englishRows + PopupBorder
}

type Model struct {
	// ctx spans the whole overlay session; Bubble Tea commands are plain
	// closures, so there is nowhere else to carry it.
	ctx      context.Context
	prompter Prompter
	options  Options
	styles   styles
	draft    vimarea.Model
	spinner  spinner.Model
	stage    stage
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
	look := newStyles()

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
	draft.SetHeight(draftHeight)

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

	model := Model{
		ctx:      ctx,
		prompter: prompter,
		options:  options,
		styles:   look,
		draft:    draft,
		spinner:  working,
		delivery: deliveryFor(options.Review),
	}
	if options.WithoutService {
		model.options.Live = false
	}
	model.resize(maxContentWidth)
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
			model.resize(maxContentWidth)
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
		started, cmd := m.startPreview()
		breathing := started.(Model)
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
	// Never wider than the pane. A width clamped up past it wraps every line the
	// popup draws, and an inline renderer then stacks frame on frame.
	m.width = min(max(contentWidth, 1), maxContentWidth)
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
		return draftHeight
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

func (m Model) handleKey(key tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch {
	case key.Type == tea.KeyCtrlC:
		return m, tea.Quit

	// The record of delivered prompts answers to its own keys while it is open,
	// and ctrl+g opens it from wherever the panel is.
	case m.historyOpen || key.Type == tea.KeyCtrlG:
		return m.historyKey(key)

	// Tab flips between writing and reading the translation.
	case key.Type == tea.KeyTab:
		return m.flipReading(), nil

	case m.reading:
		return m.readKey(key)

	// Escape closes a read-mode panel whatever is on screen: the text came from
	// somewhere else, nothing here is kept for next time, and the notice says
	// so. Leaving an edit mode is the one exception — a vim author's fingers
	// expect escape out of insert before anything else.
	case m.options.Read && key.Type == tea.KeyEsc &&
		(!m.draft.Modal() || m.draft.Mode() == vimarea.Normal):
		return m.close()

	// Escape takes the message away. It must not also close the popup.
	case key.Type == tea.KeyEsc && (m.notice != nil || m.hint != ""):
		m.notice, m.hint = nil, ""
		return m, nil

	case key.Type == tea.KeyEsc && m.stage == confirming:
		m.stage = composing
		m.confirmWait = 0
		m.refit()
		return m, nil

	case key.Type == tea.KeyEsc && !m.draft.Modal():
		return m.close()

	// Escape is the whole way out: insert mode first, then the popup. Nothing is
	// lost by it, since closing keeps the draft.
	case key.Type == tea.KeyEsc && m.draft.Mode() == vimarea.Normal:
		return m.close()

	case key.Type == tea.KeyCtrlR:
		return m.switchDelivery(), nil

	case (key.Type == tea.KeyCtrlL || key.Type == tea.KeyCtrlT) &&
		!m.translating():
		return m.raiseNotice(errNoService)

	case key.Type == tea.KeyCtrlL:
		return m.switchLive()

	// Translate what is there now: after a translation that did not arrive, and as
	// the way to read the English at all when live translation is off.
	case key.Type == tea.KeyCtrlT:
		m.previewError = nil
		m.refit()
		m.revision++
		started, cmd := m.startPreview()
		translating := started.(Model)
		// A confirmation that asked for this translation waits for this one now.
		if translating.confirmWait != 0 {
			translating.confirmWait = translating.requested
		}
		return translating, tea.Batch(cmd, translating.beginPulse())

	// ctrl+u clears the whole draft, as it clears a line in a shell. The text
	// area would otherwise use it to delete back to the line start.
	case key.Type == tea.KeyCtrlU:
		m.stage = composing
		m.confirmWait = 0
		m.draft.Clear()
		m.draftTop = 0
		m.forgetDraft()
		m.resumed = false
		m.preview, m.previewOf, m.previewError = "", "", nil
		return m, nil

	// Sending is deliberate: a bare enter is a new line, alt+enter sends. The
	// terminal hands each press over on its own, so two presses of escape and
	// enter stay two messages and cannot become a send.
	case key.Type == tea.KeyCtrlD, key.Type == tea.KeyEnter && key.Alt:
		switch m.stage {
		case translating:
			return m, nil
		case confirming:
			return m.deliverPreview()
		default:
			return m.startSubmit()
		}
	}

	m.failure, m.notice, m.hint = nil, nil, ""
	before := m.draft.Value()

	var cmd tea.Cmd
	m.draft, cmd = m.draft.Update(key)
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

func (m Model) startPreview() (tea.Model, tea.Cmd) {
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
	model, cmd := m.startPreview()
	confirming := model.(Model)
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
