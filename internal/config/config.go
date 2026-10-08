// Package config resolves the plugin's settings from the environment and
// from the .env file in the config directory. It stays neutral about which
// translation service is used.
package config

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"

	"trans/internal/translation"
)

// The settings the settings window offers are named here, so the window and
// the reading of them below can never spell one differently. The rest are
// private: they are resolved at a point the window never reaches.
const (
	targetVar    = "TRANS_TARGET"
	ProviderVar  = "TRANS_PROVIDER"
	ApiKeyVar    = "TRANS_API_KEY"
	LanguageVar  = "TRANS_LANGUAGE"
	EndpointVar  = "TRANS_ENDPOINT"
	ModelVar     = "TRANS_MODEL"
	PasteVar     = "TRANS_PASTE_KEYS"
	CommandVar   = "TRANS_COMMAND"
	SubmitVar    = "TRANS_SUBMIT"
	VimVar       = "TRANS_VIM"
	LiveVar      = "TRANS_LIVE"
	KeepDraftVar = "TRANS_KEEP_DRAFT"
	ConfirmVar   = "TRANS_CONFIRM"
	maxDraftVar  = "TRANS_MAX_DRAFT"
	pulseVar     = "TRANS_PULSE"
	logoVar      = "TRANS_LOGO"
	trayVar      = "TRANS_TRAY"
	HotkeyVar    = "TRANS_HOTKEY"
	// SelectHotkeyVar opens the translation of a selection; ConfigHotkeyVar the
	// settings window. SelectCopyVar is the chord that copies a selection for
	// terminals that only copy on a key.
	SelectHotkeyVar = "TRANS_SELECT_HOTKEY"
	ConfigHotkeyVar = "TRANS_CONFIG_HOTKEY"
	SelectCopyVar   = "TRANS_SELECT_COPY"
	// KeysVar decides how the provider key is kept on disk: "dpapi" wraps it
	// with Windows Data Protection (the default), "plain" leaves the .env
	// line alone and skips the migration. The settings window shows it.
	KeysVar = "TRANS_KEYS"
	// ThemeVar names the colour scheme the panel draws itself in; an empty
	// value leaves the terminal's own palette in charge.
	ThemeVar = "TRANS_THEME"
	// PanelHostVar says where the panel opens: the terminal in front
	// ("terminal", the default), where it runs as a TUI beside the pane it
	// delivers into, or a window of its own over that pane ("popup").
	PanelHostVar = "TRANS_PANEL_HOST"
	// DraftRowsVar is how many rows the draft box of the panel asks its
	// terminal for, and PanelWidthVar how many columns the popup asks for.
	// Both ends of what may be asked for are named below, so the settings
	// window writes back the same numbers this reading refuses.
	DraftRowsVar  = "TRANS_DRAFT_ROWS"
	PanelWidthVar = "TRANS_PANEL_WIDTH"
	configDirVar  = "TRANS_CONFIG_DIR"
	stateDirVar   = "TRANS_STATE_DIR"

	historyVar      = "TRANS_HISTORY"
	historyLimitVar = "TRANS_HISTORY_LIMIT"

	defaultHistoryLimit = 500

	// The two ends are what the settings window will accept to write as
	// well: a number outside them is refused here rather than quietly
	// brought back in, so the two never disagree about what is a whole
	// number of rows or columns worth asking the terminal for.
	DraftRowsLow     = 4
	DraftRowsHigh    = 16
	DefaultDraftRows = 6
	// The panel popup is too narrow to read a line under the low end and
	// wider than any terminal under the high end; both ends are refused
	// here and by the settings window alike.
	PanelWidthLow     = 60
	PanelWidthHigh    = 180
	DefaultPanelWidth = 110

	// HostPopup and HostTerminal are the two answers PanelHostVar takes: the
	// panel beside the pane the author is at, or in a window of its own.
	HostPopup    = "popup"
	HostTerminal = "terminal"

	defaultLanguage = "EN-US"
	dotenvName      = ".env"

	// Registered once when the daemon starts, so each window has its own chord
	// and turning one off is a matter of setting it to "off".
	defaultHotkey       = "ctrl+alt+t"
	defaultSelectHotkey = "ctrl+alt+s"
	defaultConfigHotkey = "ctrl+alt+c"
)

// The other half of the panel — reading English that is already there instead
// of writing a prompt — has settings of its own, kept in one block so the two
// halves can grow without writing in each other's lines.
const (
	// The language text read in read mode comes back in, whatever language the
	// prompts themselves are written in.
	readLanguageVar = "TRANS_READ_LANGUAGE"
	// The chord that copies a selection out of the target window before the
	// panel covers it. Parsed the way a paste chord is, and only pressed when
	// a capture was actually asked for.
	captureKeysVar = "TRANS_CAPTURE_KEYS"

	defaultReadLanguage = "ZH"
	defaultCaptureKeys  = "ctrl+shift+c"
)

type Settings struct {
	Target string
	// Provider names the translation service; empty means the default.
	Provider   string
	Options    translation.Options
	ConfigFile string
	// StateDir is where an unfinished prompt is kept between sessions.
	StateDir  string
	Submit    bool
	Vim       bool
	Live      bool
	KeepDraft bool
	Confirm   bool
	Pulse     bool
	Logo      bool
	// Tray is whether the daemon puts an icon in the notification area at all.
	// It is read here rather than by the daemon alone so that every setting of
	// this program is read in one place.
	Tray     bool
	MaxDraft int
	// PasteKeys is the chord a terminal takes for a paste, for the panel on
	// Windows. What is left empty is worked out from the terminal in front.
	PasteKeys string
	// Hotkey is the key combination that opens the panel (Windows daemon).
	Hotkey string
	// SelectHotkey translates the selection in the pane in front; ConfigHotkey
	// opens this window's bigger sibling, the settings window (Windows daemon).
	SelectHotkey string
	ConfigHotkey string
	// SelectCopy is the chord sent to the pane to copy a selection before it is
	// read from the clipboard. Empty reads the clipboard as it stands, which is
	// what a terminal that copies on selection has already filled.
	SelectCopy string
	// ReadLanguage is the language text read in read mode is translated into.
	// It stands beside Options.TargetLanguage rather than in it: a normal panel
	// translates into the one, a read panel into the other.
	ReadLanguage string
	// CaptureKeys is the chord pressed in the target window to copy what is
	// selected there, for `open --read --capture`.
	CaptureKeys string
	// Keys is TRANS_KEYS: how the provider key is stored at rest — "dpapi"
	// (the default) or "plain". Read like any other setting, so the
	// environment and the .env file both have their say.
	Keys string

	// History is whether the prompts already delivered are written down, so
	// the panel can offer them again instead of them being retyped.
	History bool
	// HistoryLimit is how many delivered prompts are kept before the oldest
	// ones are dropped.
	HistoryLimit int
	// Theme is the name of the colour scheme; empty follows the terminal's
	// default palette. A name this package does not know is left alone for
	// the frame package to fall back from, so config stays neutral about
	// how the panel looks.
	Theme string
	// PanelHost is where the panel opens: "terminal" for the terminal in
	// front, which runs the panel as a TUI beside the pane it delivers into,
	// or "popup" for a window of its own over the pane it belongs on.
	PanelHost string
	// DraftRows is the number of rows the draft box of the panel asks for.
	DraftRows int
	// PanelWidth is the number of columns the panel popup asks for.
	PanelWidth int
}

// The environment wins over the .env file, so a one-off invocation can
// override stored settings. Credentials are passed along unchecked: only the
// service knows what it needs.
func Load(getenv func(string) string) (Settings, error) {
	// Without the directory there is nothing of ours to read.
	// Falling back to a relative path would let a .env in whatever directory
	// the process started in decide the settings.
	configFile := ""
	stored := map[string]string{}
	if configDir := getenv(configDirVar); configDir != "" {
		configFile = filepath.Join(configDir, dotenvName)
		stored = readDotenv(configFile)
	}

	lookup := func(key string) string {
		if value := strings.TrimSpace(getenv(key)); value != "" {
			return value
		}
		return stored[key]
	}

	provider := lookup(ProviderVar)

	// A setting that cannot be read is not quietly taken as off: a typo in
	// TRANS_SUBMIT would otherwise send every prompt to the agent.
	given := &reading{lookup: lookup}
	settings := Settings{
		Target:     lookup(targetVar),
		Provider:   provider,
		ConfigFile: configFile,
		StateDir:   lookup(stateDirVar),
		Submit:     given.flag(SubmitVar, true),
		Vim:        given.flag(VimVar, false),
		Live:       given.flag(LiveVar, true),
		KeepDraft:  given.flag(KeepDraftVar, true),
		Confirm:    given.flag(ConfirmVar, false),
		Pulse:      given.flag(pulseVar, true),
		Logo:       given.flag(logoVar, true),
		Tray:       given.flag(trayVar, true),
		MaxDraft:   given.number(maxDraftVar),
		PasteKeys:  lookup(PasteVar),
		Keys:       orDefault(lookup(KeysVar), keysDPAPI),
		Hotkey:     orDefault(lookup(HotkeyVar), defaultHotkey),
		// Each window keeps its own chord, so a panel that answers one press
		// does not have to give up the selection's.
		SelectHotkey: orDefault(lookup(SelectHotkeyVar), defaultSelectHotkey),
		ConfigHotkey: orDefault(lookup(ConfigHotkeyVar), defaultConfigHotkey),
		SelectCopy:   lookup(SelectCopyVar),

		History:      given.flag(historyVar, true),
		HistoryLimit: given.count(historyLimitVar, defaultHistoryLimit),

		// A theme is taken as written, name and all: this package does not
		// know which names exist, so an unknown one is left for the frame
		// package to fall back from.
		Theme:      lookup(ThemeVar),
		PanelHost:  given.oneOf(PanelHostVar, HostTerminal, HostPopup, HostTerminal),
		DraftRows:  given.between(DraftRowsVar, DraftRowsLow, DraftRowsHigh, DefaultDraftRows),
		PanelWidth: given.between(PanelWidthVar, PanelWidthLow, PanelWidthHigh, DefaultPanelWidth),

		Options: translation.Options{
			APIKey:         orDefault(lookup(ScopedKeyVar(provider)), lookup(ApiKeyVar)),
			TargetLanguage: orDefault(lookup(LanguageVar), defaultLanguage),
			Endpoint:       lookup(EndpointVar),
			Model:          lookup(ModelVar),
			Command:        lookup(CommandVar),
		},
		ReadLanguage: orDefault(lookup(readLanguageVar), defaultReadLanguage),
		CaptureKeys:  orDefault(lookup(captureKeysVar), defaultCaptureKeys),
	}

	if given.err != nil {
		return Settings{}, given.err
	}
	return settings, nil
}

// reading takes the settings apart, keeping the first value it could not make
// sense of so that Load can refuse it.
type reading struct {
	lookup func(string) string
	err    error
}

func (r *reading) flag(variable string, whenUnset bool) bool {
	value := strings.TrimSpace(r.lookup(variable))
	switch strings.ToLower(value) {
	case "":
		return whenUnset
	case "1", "true", "yes", "on":
		return true
	case "0", "false", "no", "off":
		return false
	default:
		r.refuse(variable, value, "neither on (1, true, yes, on) nor off (0, false, no, off)")
		return whenUnset
	}
}

// oneOf reads a setting that takes one of a few named answers. A name outside
// them is refused rather than quietly taken as the default: a typo in
// TRANS_PANEL_HOST would otherwise open the panel somewhere nobody asked for.
func (r *reading) oneOf(variable, fallback string, allowed ...string) string {
	value := strings.ToLower(strings.TrimSpace(r.lookup(variable)))
	if value == "" {
		return fallback
	}
	if slices.Contains(allowed, value) {
		return value
	}
	r.refuse(variable, value, "neither "+strings.Join(allowed, " nor "))
	return fallback
}

// A zero means no number was given and the overlay picks its own.
func (r *reading) number(variable string) int {
	value := strings.TrimSpace(r.lookup(variable))
	if value == "" {
		return 0
	}

	number, err := strconv.Atoi(value)
	if err != nil || number < 0 {
		r.refuse(variable, value, "not a whole number of characters")
		return 0
	}
	return number
}

// A count that cannot be read is not quietly taken as the default: a typo in
// TRANS_HISTORY_LIMIT would otherwise silently change how much is kept.
// Unlike a character count, zero here means nobody wants any.
func (r *reading) count(variable string, whenUnset int) int {
	value := strings.TrimSpace(r.lookup(variable))
	if value == "" {
		return whenUnset
	}

	number, err := strconv.Atoi(value)
	if err != nil || number < 1 {
		r.refuse(variable, value, "not a positive whole number")
		return whenUnset
	}
	return number
}

// A number outside the ends it may sit between is refused rather than
// quietly brought back in: the window would have refused to write it too.
func (r *reading) between(variable string, low, high, whenUnset int) int {
	value := strings.TrimSpace(r.lookup(variable))
	if value == "" {
		return whenUnset
	}

	number, err := strconv.Atoi(value)
	if err != nil || number < low || number > high {
		r.refuse(variable, value, fmt.Sprintf("a whole number between %d and %d", low, high))
		return whenUnset
	}
	return number
}

func (r *reading) refuse(variable, value, why string) {
	if r.err == nil {
		r.err = fmt.Errorf("%s is %q, which is %s", variable, value, why)
	}
}

// ScopedKeyVar is the variable a key for one service is read from; the
// unscoped TRANS_API_KEY stands in when no service is named. Scoping the key
// by service lets several services be configured side by side.
func ScopedKeyVar(provider string) string {
	if provider == "" {
		return ""
	}
	scope := strings.ToUpper(strings.NewReplacer("-", "_", ".", "_").Replace(provider))
	return "TRANS_" + scope + "_API_KEY"
}

func orDefault(value, fallback string) string {
	if value == "" {
		return fallback
	}
	return value
}

// readDotenv parses KEY=VALUE lines, ignoring comments and blanks. A missing
// file is normal: the plugin works from the environment alone.
func readDotenv(path string) map[string]string {
	values := map[string]string{}

	file, err := os.Open(path)
	if err != nil {
		return values
	}
	defer file.Close()

	lines := bufio.NewScanner(file)
	for lines.Scan() {
		line := strings.TrimSpace(lines.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		key, value, found := strings.Cut(line, "=")
		if !found {
			continue
		}
		values[strings.TrimSpace(key)] = unquoted(value)
	}
	if err := lines.Err(); err != nil {
		fmt.Fprintf(os.Stderr, "trans: warning: reading %s: %v\n", path, err)
	}
	return values
}

// unquoted takes the quotes off a value only when they wrap it whole, the way
// Save puts them on. Stripping every quote at either end would eat the last
// quote of a command like sed 's/  */ /g' and hand back something else.
func unquoted(value string) string {
	stripped := strings.TrimSpace(value)
	if len(stripped) >= 2 {
		first, last := stripped[0], stripped[len(stripped)-1]
		if (first == '"' || first == '\'') && last == first {
			return stripped[1 : len(stripped)-1]
		}
	}
	return stripped
}
