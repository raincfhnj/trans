// Package selection is the popup that puts a selection and its translation on
// the screen together — the window the daemon opens when the selection's chord
// is pressed. Nothing is delivered from here: the text the author selected
// stays where it is, and this window only reads it.
package selection

import (
	"context"
	"strings"

	"trans/internal/frame"

	"github.com/charmbracelet/bubbles/spinner"
	tea "github.com/charmbracelet/bubbletea"
)

// The window is a fixed size so it lands where it was meant to, and the two
// boxes share whatever height they are given: the selection takes the rows it
// needs up to sourceRows, the translation the rest.
const (
	PopupWidth = 100

	sourceRows         = 4
	translationRows    = 8
	headerRows         = 1
	footerRows         = 1
	popupBorder        = 2
	minSelectedRows    = 1
	minTranslationRows = 1
)

// PopupHeight is the height of the window the selection opens in.
func PopupHeight() int {
	return popupBorder + headerRows + footerRows +
		(sourceRows + popupBorder) + (translationRows + popupBorder)
}

// Translator answers the selection in the target language.
type Translator interface {
	Translate(context.Context, string) (string, error)
}

type Options struct {
	// Service is the name of the chosen translation service and Language the
	// language it translates into, for the header.
	Service  string
	Language string
	// Source is the text that was selected — the reason this window exists.
	Source string
	// SelectCopy is the chord the pane was asked to copy with; an empty Source
	// reads differently depending on whether one was sent.
	SelectCopy string
	// Trouble is what stopped the service from being usable at all. The window
	// opens anyway: the selection was worth the chord even if it cannot be
	// translated yet.
	Trouble error
	// WithoutService says there is nothing to translate with.
	WithoutService bool
	// Theme is the name frame.PaletteFor knows, the same one the panel was
	// given so this window is drawn the way the rest of the program is; empty
	// draws with the default.
	Theme string
}

type Model struct {
	ctx        context.Context
	translator Translator
	options    Options
	styles     frame.Styles
	// palette is the slots the boxes are drawn from, kept beside the styles
	// because every draw builds the border styles out of it.
	palette frame.Palette
	spinner spinner.Model
	// translating is on from the first frame, so the wait is said while it lasts.
	translating bool
	// translated is the answer on screen; failure the last one that did not come.
	translated string
	failure    error
	// first is the first row of the translation on screen.
	first int
	pane  tea.WindowSizeMsg
}

func New(ctx context.Context, translator Translator, options Options) Model {
	palette := frame.PaletteFor(options.Theme)
	look := palette.Styles()

	working := spinner.New()
	working.Spinner = spinner.Dot
	working.Style = look.Accent

	model := Model{
		ctx:        ctx,
		translator: translator,
		options:    options,
		styles:     look,
		palette:    palette,
		spinner:    working,
		pane: tea.WindowSizeMsg{
			Width:  PopupWidth,
			Height: PopupHeight(),
		},
	}
	model.translating = model.translatable()
	return model
}

// translatable says whether the service can be asked at all: not without one,
// not when the service refused to start, and not when there is nothing to ask
// with.
func (m Model) translatable() bool {
	return !m.options.WithoutService &&
		m.options.Trouble == nil &&
		strings.TrimSpace(m.options.Source) != ""
}

type translatedMsg struct {
	text string
	err  error
}

func (m Model) Init() tea.Cmd {
	if !m.translating {
		return nil
	}
	return tea.Batch(m.spinner.Tick, m.ask())
}

func (m Model) ask() tea.Cmd {
	source := m.options.Source
	return func() tea.Msg {
		translated, err := m.translator.Translate(m.ctx, source)
		return translatedMsg{text: translated, err: err}
	}
}

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.pane = msg
		return m, nil

	case spinner.TickMsg:
		if !m.translating {
			return m, nil
		}
		var cmd tea.Cmd
		m.spinner, cmd = m.spinner.Update(msg)
		return m, cmd

	case translatedMsg:
		m.translating = false
		m.failure = msg.err
		if msg.err == nil {
			m.translated = msg.text
			m.first = 0
		}
		return m, nil

	case tea.KeyMsg:
		return m.handleKey(msg)
	}
	return m, nil
}

func (m Model) handleKey(key tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch {
	// Escape closes without ceremony: there is no writing here to keep.
	case key.Type == tea.KeyCtrlC, key.Type == tea.KeyEsc:
		return m, tea.Quit

	// A translation that did not arrive is worth another try, and the key
	// stays free for it: the selection cannot change behind the window.
	case key.Type == tea.KeyCtrlT && m.failure != nil:
		m.failure = nil
		m.translating = true
		return m, tea.Batch(m.spinner.Tick, m.ask())

	case key.Type == tea.KeyUp:
		return m.moved(-1), nil
	case key.Type == tea.KeyDown:
		return m.moved(1), nil
	case key.Type == tea.KeyPgUp:
		return m.moved(-m.translationVisible()), nil
	case key.Type == tea.KeyPgDown, key.Type == tea.KeySpace:
		return m.moved(m.translationVisible()), nil
	case key.Type == tea.KeyHome:
		m.first = 0
		return m, nil
	case key.Type == tea.KeyEnd:
		m.first = m.translationTotal() - m.translationVisible()
		return m, nil

	case key.Type == tea.KeyRunes:
		switch string(key.Runes) {
		case "k":
			return m.moved(-1), nil
		case "j":
			return m.moved(1), nil
		case "g":
			m.first = 0
			return m, nil
		case "G":
			m.first = m.translationTotal() - m.translationVisible()
			return m, nil
		case " ":
			return m.moved(m.translationVisible()), nil
		}
	}
	return m, nil
}

// moved is the view walked by rows, as far as there is text to scroll — one
// row for a step, the windowful for a page.
func (m Model) moved(rows int) Model {
	m.first = min(max(m.first+rows, 0), max(m.translationTotal()-m.translationVisible(), 0))
	return m
}
