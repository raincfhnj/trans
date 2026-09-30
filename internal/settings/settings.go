package settings

import (
	"fmt"
	"slices"
	"strings"

	"trans/internal/config"
	"trans/internal/frame"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
)

// The window is a row per setting with the header and footer around them, so
// its height follows the settings it offers — 16 of them, plus four.
const (
	PopupWidth = 100
	chromeRows = 4
)

// PopupHeight is the height of the window for these settings.
func PopupHeight(options Options) int {
	return len(buildRows(options.Settings, options)) + chromeRows
}

type Options struct {
	// Settings is what the window opens on, and its ConfigFile is where a save
	// writes.
	Settings config.Settings
	// Getenv answers the environment as the process sees it, for marking the
	// rows a save cannot change while a variable is set there. Without one
	// nothing is marked.
	Getenv func(string) string
	// Services are the translation services the service row steps through, in
	// the order the registry knows them.
	Services []string
	// Chord says whether a key combination can be pressed at all; a save that
	// would store one it refuses is refused instead. Without one nothing is
	// checked here and the panel refuses it when it opens.
	Chord func(string) error
}

type Model struct {
	settings config.Settings
	options  Options
	styles   frame.Styles
	// rows is shared by every copy of the model on purpose: the window holds
	// them, and bubbletea's copies are all the same window.
	rows   []*row
	cursor int
	// top is the first row of the list on screen, moved to keep the cursor.
	top int
	// input is the writing for the row being edited; editing is its index, or
	// -1 while the list itself has the keys.
	input   textinput.Model
	editing int
	// leaving has seen an escape on changed settings: the next one closes.
	leaving bool
	notice  error
	hint    string
	pane    tea.WindowSizeMsg
}

func New(options Options) Model {
	look := frame.NewStyles()

	writing := textinput.New()
	writing.Prompt = ""
	writing.CharLimit = 0

	model := Model{
		settings: options.Settings,
		options:  options,
		styles:   look,
		rows:     buildRows(options.Settings, options),
		input:    writing,
		editing:  -1,
		pane: tea.WindowSizeMsg{
			Width:  PopupWidth,
			Height: PopupHeight(options),
		},
	}
	model.input.Width = max(model.contentWidth()-model.labelRoom()-2, 8)
	return model
}

func (m Model) Init() tea.Cmd {
	return nil
}

type saveResultMsg struct {
	err     error
	hint    string
	written bool
}

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.pane = msg
		m.input.Width = max(m.contentWidth()-m.labelRoom()-2, 8)
		return m, nil

	case saveResultMsg:
		m.notice, m.hint = msg.err, msg.hint
		m.leaving = false
		if msg.written {
			// From here on this is what the file holds; a further save writes
			// only what changes after it.
			for _, row := range m.rows {
				row.original = row.value
			}
		}
		return m, nil

	case tea.KeyMsg:
		if m.editing >= 0 {
			return m.editingKey(msg)
		}
		return m.listKey(msg)
	}
	return m, nil
}

func (m Model) listKey(key tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch {
	// Ctrl+C closes whenever it is pressed, changes or no changes: it is the
	// key every other window here answers to as well.
	case key.Type == tea.KeyCtrlC:
		return m, tea.Quit

	case key.Type == tea.KeyEsc:
		// The way out is in steps: first what a key said is taken away, then
		// dropping the changes is said out loud, and only a second escape
		// after that closes.
		switch {
		case m.leaving:
			return m, tea.Quit
		case m.notice != nil:
			m.notice = nil
			return m, nil
		case m.hint != "":
			m.hint = ""
			return m, nil
		case m.changed():
			m.leaving = true
			m.hint = "unsaved — s saves them, esc again leaves them behind"
			return m, nil
		default:
			return m, tea.Quit
		}

	// Saving is one key from the list, and it speaks for itself.
	case key.String() == "s", key.Type == tea.KeyCtrlS:
		m.notice, m.hint = nil, ""
		m.leaving = false
		return m, m.saving()

	case key.Type == tea.KeyUp, key.String() == "k":
		return m.moved(-1), nil
	case key.Type == tea.KeyDown, key.String() == "j":
		return m.moved(1), nil
	case key.Type == tea.KeyHome, key.String() == "g":
		m.cursor, m.top = 0, 0
		return m.settled(), nil
	case key.Type == tea.KeyEnd, key.String() == "G":
		m.cursor = len(m.rows) - 1
		return m.settled().followed(), nil

	case key.Type == tea.KeyEnter:
		return m.entered()
	case key.Type == tea.KeyRight:
		return m.stepped(1), nil
	case key.Type == tea.KeyLeft:
		return m.stepped(-1), nil
	}
	// Every other key is back to the list: the message it carried goes with it.
	return m.settled(), nil
}

// settled takes away the message on screen: any key but escape, save and
// close is the author back in the list rather than answering it.
func (m Model) settled() Model {
	m.notice, m.hint = nil, ""
	m.leaving = false
	return m
}

// stepped is an arrow on the row under the cursor: the value a choice steps
// through, the side a flag is thrown to, and nothing at all for a row that is
// written rather than chosen.
func (m Model) stepped(direction int) Model {
	m = m.settled()

	row := m.rows[m.cursor]
	switch row.kind {
	case choice:
		step := slices.Index(row.choices, row.chosen())
		row.value = ""
		if index := (step + direction + len(row.choices)) % len(row.choices); row.choices[index] != "auto" {
			row.value = row.choices[index]
		}
	case flag:
		if direction > 0 {
			row.value = "1"
		} else {
			row.value = "0"
		}
	}
	return m
}

// chosen is the choice a choice row is on: auto is its first, under the name
// the list shows it by.
func (row *row) chosen() string {
	if row.value == "" {
		return "auto"
	}
	if slices.Contains(row.choices, row.value) {
		return row.value
	}
	// A service from the file that the registry has never heard of is shown as
	// it stands and left where an arrow would start again.
	return ""
}

// entered is enter on the row under the cursor: a row that is written opens
// for writing, a choice takes its next value, a flag is thrown.
func (m Model) entered() (tea.Model, tea.Cmd) {
	m = m.settled()
	row := m.rows[m.cursor]

	switch row.kind {
	case choice:
		return m.stepped(1), nil
	case flag:
		row.value = flip(row.value)
		return m, nil
	default:
		m.editing = m.cursor
		m.input.SetValue(row.value)
		m.input.CursorEnd()
		return m, m.input.Focus()
	}
}

func flip(value string) string {
	if value == "1" {
		return "0"
	}
	return "1"
}

func (m Model) editingKey(key tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch {
	case key.Type == tea.KeyCtrlC:
		return m, tea.Quit

	case key.Type == tea.KeyEnter:
		next := m.committed()
		return next, nil

	case key.Type == tea.KeyEsc:
		m.editing = -1
		m.input.Blur()
		return m, nil

	case key.Type == tea.KeyCtrlS:
		// The value under the cursor is the one being saved, so the writing is
		// put away first and then the save runs.
		next := m.committed()
		return next, next.saving()
	}

	var cmd tea.Cmd
	m.input, cmd = m.input.Update(key)
	return m, cmd
}

// committed keeps what was written: trimmed, because that is what reading it
// back would answer with anyway.
func (m Model) committed() Model {
	row := m.rows[m.editing]
	row.value = strings.TrimSpace(m.input.Value())
	m.editing = -1
	m.input.Blur()
	m.notice, m.hint = nil, ""
	m.leaving = false
	return m
}

// moved walks the list and brings the view along, which is what a list a pane
// may be too short for has to do.
func (m Model) moved(rows int) Model {
	m = m.settled()
	m.cursor += rows
	return m.followed()
}

func (m Model) followed() Model {
	visible := m.visibleRows()
	m.cursor = min(max(m.cursor, 0), len(m.rows)-1)
	m.top = min(m.top, m.cursor)
	m.top = max(m.top, m.cursor-visible+1)
	m.top = min(max(m.top, 0), max(len(m.rows)-visible, 0))
	return m
}

func (m Model) changed() bool {
	for _, row := range m.rows {
		if row.changed() {
			return true
		}
	}
	return false
}

// saving writes the changed rows into the .env file, leaving every line it
// does not name as it stands. It refuses first and writes after: a chord that
// cannot be pressed leaves the file alone.
func (m Model) saving() tea.Cmd {
	updates := map[string]string{}
	changed := []*row{}
	restart := false
	for _, row := range m.rows {
		if !row.changed() {
			continue
		}
		updates[row.variable] = row.value
		changed = append(changed, row)
		// The daemon waits on its chords from the start, so a window opened by
		// one cannot change it for the daemon already running.
		if row.kind == hotkey {
			restart = true
		}
	}
	if len(changed) == 0 {
		return func() tea.Msg { return saveResultMsg{hint: "nothing has changed"} }
	}
	for _, row := range changed {
		if err := m.chordFits(row); err != nil {
			return func() tea.Msg { return saveResultMsg{err: err} }
		}
	}

	file := m.settings.ConfigFile
	return func() tea.Msg {
		if err := config.Save(file, updates); err != nil {
			return saveResultMsg{err: err}
		}
		hint := "settings written"
		if restart {
			hint += " — restart trans-windowd for the hotkeys"
		}
		return saveResultMsg{hint: hint, written: true}
	}
}

// chordFits is the row's value as a key combination a person can press, or why
// it is not one. Off switches a hotkey off rather than naming a key.
func (m Model) chordFits(row *row) error {
	if row.kind != chord && row.kind != hotkey || m.options.Chord == nil {
		return nil
	}
	value := strings.TrimSpace(row.value)
	if value == "" || (row.kind == hotkey && strings.EqualFold(value, "off")) {
		return nil
	}
	if err := m.options.Chord(value); err != nil {
		return fmt.Errorf("%s (%s): %w", row.label, row.variable, err)
	}
	return nil
}

// visibleRows is what fits in the window, which is smaller than the list when
// the pane it opens over is.
func (m Model) visibleRows() int {
	return max(m.pane.Height-chromeRows, 1)
}

func (m Model) contentWidth() int {
	return frame.ContentWidth(m.pane.Width)
}

// labelRoom is the column the labels take, wide enough for the longest of them.
func (m Model) labelRoom() int {
	longest := 0
	for _, row := range m.rows {
		longest = max(longest, len(row.label))
	}
	return longest + 2
}
