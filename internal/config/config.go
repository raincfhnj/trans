// Package config resolves the plugin's settings from the environment and
// from the .env file in the config directory. It stays neutral about which
// translation service is used.
package config

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"trans/internal/translation"
)

const (
	targetVar    = "TRANS_TARGET"
	providerVar  = "TRANS_PROVIDER"
	apiKeyVar    = "TRANS_API_KEY"
	languageVar  = "TRANS_LANGUAGE"
	endpointVar  = "TRANS_ENDPOINT"
	modelVar     = "TRANS_MODEL"
	pasteVar     = "TRANS_PASTE_KEYS"
	commandVar   = "TRANS_COMMAND"
	submitVar    = "TRANS_SUBMIT"
	vimVar       = "TRANS_VIM"
	liveVar      = "TRANS_LIVE"
	keepDraftVar = "TRANS_KEEP_DRAFT"
	confirmVar   = "TRANS_CONFIRM"
	maxDraftVar  = "TRANS_MAX_DRAFT"
	pulseVar     = "TRANS_PULSE"
	logoVar      = "TRANS_LOGO"
	hotkeyVar    = "TRANS_HOTKEY"
	configDirVar = "TRANS_CONFIG_DIR"
	stateDirVar  = "TRANS_STATE_DIR"

	defaultLanguage = "EN-US"
	dotenvName      = ".env"
)

type Settings struct {
	Target string
	// Provider names the translation service; empty means the default.
	Provider   string
	Options    translation.Options
	ConfigFile string
	// StateDir is where an unfinished prompt is kept between sessions.
	StateDir string
	Submit   bool
	Vim      bool
	Live     bool
	KeepDraft bool
	Confirm   bool
	Pulse     bool
	Logo      bool
	MaxDraft  int
	// PasteKeys is the chord a terminal takes for a paste, for the panel on
	// Windows. What is left empty is worked out from the terminal in front.
	PasteKeys string
	// Hotkey is the key combination that opens the panel (Windows daemon).
	Hotkey string
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

	provider := lookup(providerVar)

	// A setting that cannot be read is not quietly taken as off: a typo in
	// TRANS_SUBMIT would otherwise send every prompt to the agent.
	given := &reading{lookup: lookup}
	settings := Settings{
		Target:     lookup(targetVar),
		Provider:   provider,
		ConfigFile: configFile,
		StateDir:   lookup(stateDirVar),
		Submit:     given.flag(submitVar, true),
		Vim:        given.flag(vimVar, false),
		Live:       given.flag(liveVar, true),
		KeepDraft:  given.flag(keepDraftVar, true),
		Confirm:    given.flag(confirmVar, false),
		Pulse:      given.flag(pulseVar, true),
		Logo:       given.flag(logoVar, true),
		MaxDraft:   given.number(maxDraftVar),
		PasteKeys:  lookup(pasteVar),
		Hotkey:     lookup(hotkeyVar),
		Options: translation.Options{
			APIKey:         orDefault(lookup(scopedKeyVar(provider)), lookup(apiKeyVar)),
			TargetLanguage: orDefault(lookup(languageVar), defaultLanguage),
			Endpoint:       lookup(endpointVar),
			Model:          lookup(modelVar),
			Command:        lookup(commandVar),
		},
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

func (r *reading) refuse(variable, value, why string) {
	if r.err == nil {
		r.err = fmt.Errorf("%s is %q, which is %s", variable, value, why)
	}
}

// Scoping the key by service lets several services be configured side by side.
func scopedKeyVar(provider string) string {
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
		values[strings.TrimSpace(key)] = strings.Trim(strings.TrimSpace(value), `"'`)
	}
	return values
}
