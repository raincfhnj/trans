package config_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"trans/internal/config"
)

func envFrom(pairs map[string]string) func(string) string {
	return func(key string) string { return pairs[key] }
}

func configDirContaining(t *testing.T, dotenv string) string {
	t.Helper()
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, ".env"), []byte(dotenv), 0o600); err != nil {
		t.Fatalf("writing .env: %v", err)
	}
	return dir
}

func TestLoadTakesTheTargetAndCredentialsFromTheEnvironment(t *testing.T) {
	t.Parallel()

	settings, err := config.Load(envFrom(map[string]string{
		"TRANS_TARGET":  "w1:p3",
		"TRANS_API_KEY": "key-123",
	}))
	if err != nil {
		t.Fatalf("Load returned unexpected error: %v", err)
	}
	if settings.Target != "w1:p3" {
		t.Errorf("Target is %q, want w1:p3", settings.Target)
	}
	if settings.Options.APIKey != "key-123" {
		t.Errorf("APIKey is %q, want key-123", settings.Options.APIKey)
	}
	if settings.Provider != "" {
		t.Errorf("Provider is %q, want it left to the composition root", settings.Provider)
	}
	if !settings.Submit {
		t.Error("Submit is false, want submitting by default")
	}
	if settings.Options.TargetLanguage != "EN-US" {
		t.Errorf("TargetLanguage is %q, want EN-US by default", settings.Options.TargetLanguage)
	}
}

func TestLoadHonoursTheChosenServiceAndItsOptions(t *testing.T) {
	t.Parallel()

	settings, err := config.Load(envFrom(map[string]string{
		"TRANS_TARGET":   "w1:p3",
		"TRANS_PROVIDER": "dry-run",
		"TRANS_LANGUAGE": "EN-GB",
		"TRANS_ENDPOINT": "https://translate.example/v2",
		"TRANS_SUBMIT":   "0",
	}))
	if err != nil {
		t.Fatalf("Load returned unexpected error: %v", err)
	}
	if settings.Provider != "dry-run" {
		t.Errorf("Provider is %q, want dry-run", settings.Provider)
	}
	if settings.Options.TargetLanguage != "EN-GB" {
		t.Errorf("TargetLanguage is %q, want EN-GB", settings.Options.TargetLanguage)
	}
	if settings.Options.Endpoint != "https://translate.example/v2" {
		t.Errorf("Endpoint is %q, want the configured one", settings.Options.Endpoint)
	}
	if settings.Submit {
		t.Error("Submit is true, want it disabled by TRANS_SUBMIT=0")
	}
}

// A command line is a setting like any other, and it keeps its quoting: it is a
// shell that runs it, not the plugin.
func TestLoadKeepsTheTranslationCommandAsItWasWritten(t *testing.T) {
	t.Parallel()
	written := `translateLocally -m de-en-base | sed 's/  */ /g'`

	settings, err := config.Load(envFrom(map[string]string{
		"TRANS_TARGET":  "w1:p3",
		"TRANS_COMMAND": written,
	}))
	if err != nil {
		t.Fatalf("Load returned unexpected error: %v", err)
	}
	if settings.Options.Command != written {
		t.Errorf("Command is %q, want it as it was written", settings.Options.Command)
	}
}

func TestLoadReadsSettingsFromTheDotEnvInTheConfigDirectory(t *testing.T) {
	t.Parallel()
	configDir := configDirContaining(t, "# credentials\nTRANS_API_KEY=key-from-file\nTRANS_LANGUAGE=\"EN-GB\"\n")

	settings, err := config.Load(envFrom(map[string]string{
		"TRANS_TARGET":     "w1:p3",
		"TRANS_CONFIG_DIR": configDir,
	}))
	if err != nil {
		t.Fatalf("Load returned unexpected error: %v", err)
	}
	if settings.Options.APIKey != "key-from-file" {
		t.Errorf("APIKey is %q, want the value from the .env file", settings.Options.APIKey)
	}
	if settings.Options.TargetLanguage != "EN-GB" {
		t.Errorf("TargetLanguage is %q, want the unquoted value from the .env file", settings.Options.TargetLanguage)
	}
	if settings.ConfigFile != filepath.Join(configDir, ".env") {
		t.Errorf("ConfigFile is %q, want the .env path so callers can point users at it", settings.ConfigFile)
	}
}

func TestAServiceSpecificKeyWinsOverTheGenericOne(t *testing.T) {
	t.Parallel()
	configDir := configDirContaining(t, "TRANS_API_KEY=generic-key\nTRANS_ACME_API_KEY=acme-key\n")

	settings, err := config.Load(envFrom(map[string]string{
		"TRANS_TARGET":     "w1:p3",
		"TRANS_PROVIDER":   "acme",
		"TRANS_CONFIG_DIR": configDir,
	}))
	if err != nil {
		t.Fatalf("Load returned unexpected error: %v", err)
	}
	if settings.Options.APIKey != "acme-key" {
		t.Errorf("APIKey is %q, want the key scoped to the chosen service", settings.Options.APIKey)
	}
}

func TestTheEnvironmentWinsOverTheDotEnvFile(t *testing.T) {
	t.Parallel()
	configDir := configDirContaining(t, "TRANS_API_KEY=key-from-file\n")

	settings, err := config.Load(envFrom(map[string]string{
		"TRANS_TARGET":     "w1:p3",
		"TRANS_CONFIG_DIR": configDir,
		"TRANS_API_KEY":    "key-from-environment",
	}))
	if err != nil {
		t.Fatalf("Load returned unexpected error: %v", err)
	}
	if settings.Options.APIKey != "key-from-environment" {
		t.Errorf("APIKey is %q, want the environment to win", settings.Options.APIKey)
	}
}

func TestLoadLeavesMissingCredentialsToTheService(t *testing.T) {
	t.Parallel()

	settings, err := config.Load(envFrom(map[string]string{"TRANS_TARGET": "w1:p3"}))
	if err != nil {
		t.Fatalf("Load returned unexpected error: %v", err)
	}
	if settings.Options.APIKey != "" {
		t.Errorf("APIKey is %q, want it empty", settings.Options.APIKey)
	}
}

func TestVimBindingsAreOffUnlessAskedFor(t *testing.T) {
	t.Parallel()

	settings, err := config.Load(envFrom(map[string]string{"TRANS_TARGET": "w1:p3"}))
	if err != nil {
		t.Fatalf("Load returned unexpected error: %v", err)
	}
	if settings.Vim {
		t.Error("Vim is true, want plain editing by default")
	}

	settings, err = config.Load(envFrom(map[string]string{
		"TRANS_TARGET": "w1:p3",
		"TRANS_VIM":    "1",
	}))
	if err != nil {
		t.Fatalf("Load returned unexpected error: %v", err)
	}
	if !settings.Vim {
		t.Error("Vim is false, want it enabled by TRANS_VIM=1")
	}
}

func TestLivePreviewIsOnUnlessTurnedOff(t *testing.T) {
	t.Parallel()

	settings, err := config.Load(envFrom(map[string]string{"TRANS_TARGET": "w1:p3"}))
	if err != nil {
		t.Fatalf("Load returned unexpected error: %v", err)
	}
	if !settings.Live {
		t.Error("Live is false, want translating while writing by default")
	}

	settings, err = config.Load(envFrom(map[string]string{
		"TRANS_TARGET": "w1:p3",
		"TRANS_LIVE":   "0",
	}))
	if err != nil {
		t.Fatalf("Load returned unexpected error: %v", err)
	}
	if settings.Live {
		t.Error("Live is true, want it turned off by TRANS_LIVE=0")
	}
}

func TestKeepingDraftsIsOnUnlessTurnedOff(t *testing.T) {
	t.Parallel()

	settings, err := config.Load(envFrom(map[string]string{"TRANS_TARGET": "w1:p3"}))
	if err != nil {
		t.Fatalf("Load returned unexpected error: %v", err)
	}
	if !settings.KeepDraft {
		t.Error("KeepDraft is false, want an unfinished prompt kept by default")
	}

	settings, err = config.Load(envFrom(map[string]string{
		"TRANS_TARGET":     "w1:p3",
		"TRANS_KEEP_DRAFT": "0",
	}))
	if err != nil {
		t.Fatalf("Load returned unexpected error: %v", err)
	}
	if settings.KeepDraft {
		t.Error("KeepDraft is true, want it turned off by TRANS_KEEP_DRAFT=0")
	}
}

func TestTheStateDirectoryIsPassedAlongForKeepingDrafts(t *testing.T) {
	t.Parallel()

	settings, err := config.Load(envFrom(map[string]string{
		"TRANS_TARGET":    "w1:p3",
		"TRANS_STATE_DIR": "/tmp/state",
	}))
	if err != nil {
		t.Fatalf("Load returned unexpected error: %v", err)
	}
	if settings.StateDir != "/tmp/state" {
		t.Errorf("StateDir is %q, want the configured directory", settings.StateDir)
	}
}

func TestConfirmingBeforeSendingIsOffUnlessAskedFor(t *testing.T) {
	t.Parallel()

	settings, err := config.Load(envFrom(map[string]string{"TRANS_TARGET": "w1:p3"}))
	if err != nil {
		t.Fatalf("Load returned unexpected error: %v", err)
	}
	if settings.Confirm {
		t.Error("Confirm is true, want sending to stay one key by default")
	}

	settings, err = config.Load(envFrom(map[string]string{
		"TRANS_TARGET":  "w1:p3",
		"TRANS_CONFIRM": "1",
	}))
	if err != nil {
		t.Fatalf("Load returned unexpected error: %v", err)
	}
	if !settings.Confirm {
		t.Error("Confirm is false, want it enabled by TRANS_CONFIRM=1")
	}
}

// Without a config directory, there is no configuration to read; reading .env
// from wherever the process happens to run would let a file in a repository
// decide which service gets used.
func TestWithoutAConfigDirectoryNoDotEnvIsRead(t *testing.T) {
	directory := t.TempDir()
	if err := os.WriteFile(filepath.Join(directory, ".env"),
		[]byte("TRANS_API_KEY=planted\n"), 0o600); err != nil {
		t.Fatalf("writing the planted file: %v", err)
	}
	t.Chdir(directory)

	settings, err := config.Load(envFrom(map[string]string{"TRANS_TARGET": "w1:p3"}))
	if err != nil {
		t.Fatalf("Load returned unexpected error: %v", err)
	}

	if settings.ConfigFile != "" {
		t.Errorf("ConfigFile is %q, want none without a config directory", settings.ConfigFile)
	}
	if settings.Options.APIKey != "" {
		t.Errorf("APIKey is %q, want the planted file ignored", settings.Options.APIKey)
	}
}

func TestThePulseIsOnUnlessTurnedOff(t *testing.T) {
	t.Parallel()

	settings, err := config.Load(envFrom(map[string]string{"TRANS_TARGET": "w1:p3"}))
	if err != nil {
		t.Fatalf("Load returned unexpected error: %v", err)
	}
	if !settings.Pulse {
		t.Error("Pulse is false, want the live indicator to breathe by default")
	}

	settings, err = config.Load(envFrom(map[string]string{
		"TRANS_TARGET": "w1:p3",
		"TRANS_PULSE":  "0",
	}))
	if err != nil {
		t.Fatalf("Load returned unexpected error: %v", err)
	}
	if settings.Pulse {
		t.Error("Pulse is true, want it turned off by TRANS_PULSE=0")
	}
}

// A setting nobody can read is a setting nobody can trust: TRANS_SUBMIT=flase
// would otherwise send every prompt straight to the agent.
func TestAValueThatIsNeitherOnNorOffIsRefused(t *testing.T) {
	t.Parallel()

	for _, variable := range []string{
		"TRANS_SUBMIT",
		"TRANS_VIM",
		"TRANS_LIVE",
		"TRANS_CONFIRM",
		"TRANS_KEEP_DRAFT",
		"TRANS_PULSE",
		"TRANS_HISTORY",
		"TRANS_TRAY",
	} {
		environment := map[string]string{
			"TRANS_TARGET": "w1:p1",
			variable:       "flase",
		}

		_, err := config.Load(envFrom(environment))
		if err == nil {
			t.Errorf("%s=flase was accepted, want it refused", variable)
			continue
		}
		if !strings.Contains(err.Error(), variable) {
			t.Errorf("%s=flase failed with %v, want the variable named", variable, err)
		}
	}
}

func TestADraftLimitThatIsNotANumberIsRefused(t *testing.T) {
	t.Parallel()

	_, err := config.Load(envFrom(map[string]string{
		"TRANS_TARGET":    "w1:p1",
		"TRANS_MAX_DRAFT": "zweitausend",
	}))
	if err == nil || !strings.Contains(err.Error(), "TRANS_MAX_DRAFT") {
		t.Errorf("Load returned %v, want the unreadable draft limit refused by name", err)
	}
}

// The tray is on unless it is turned off, and turning it off is a setting like
// any other: it is read here rather than by the daemon looking at the
// environment a second time.
func TestTheTrayIsOnUnlessTurnedOff(t *testing.T) {
	t.Parallel()

	settings, err := config.Load(envFrom(map[string]string{"TRANS_TARGET": "w1:p3"}))
	if err != nil {
		t.Fatalf("Load returned unexpected error: %v", err)
	}
	if !settings.Tray {
		t.Error("Tray is false, want the icon on by default")
	}

	off, err := config.Load(envFrom(map[string]string{
		"TRANS_TARGET": "w1:p3",
		"TRANS_TRAY":   "0",
	}))
	if err != nil {
		t.Fatalf("Load returned unexpected error: %v", err)
	}
	if off.Tray {
		t.Error("Tray is true, want TRANS_TRAY=0 to leave the icon out")
	}
}

// The panel opens beside the pane the author is at by default, and in a
// window of its own when the settings say so. A name outside those two is
// refused: TRANS_PANEL_HOST=termnal would otherwise open the panel somewhere
// nobody asked for.
func TestThePanelOpensInTheTerminalUnlessTheSettingsSayPopup(t *testing.T) {
	t.Parallel()

	settings, err := config.Load(envFrom(map[string]string{"TRANS_TARGET": "w1:p3"}))
	if err != nil {
		t.Fatalf("Load returned unexpected error: %v", err)
	}
	if settings.PanelHost != config.HostTerminal {
		t.Errorf("PanelHost is %q, want %q by default", settings.PanelHost, config.HostTerminal)
	}

	popup, err := config.Load(envFrom(map[string]string{
		"TRANS_TARGET":     "w1:p3",
		"TRANS_PANEL_HOST": "popup",
	}))
	if err != nil {
		t.Fatalf("Load returned unexpected error: %v", err)
	}
	if popup.PanelHost != config.HostPopup {
		t.Errorf("PanelHost is %q, want %q for TRANS_PANEL_HOST=popup",
			popup.PanelHost, config.HostPopup)
	}

	_, err = config.Load(envFrom(map[string]string{
		"TRANS_TARGET":     "w1:p3",
		"TRANS_PANEL_HOST": "termnal",
	}))
	if err == nil || !strings.Contains(err.Error(), "TRANS_PANEL_HOST") {
		t.Errorf("Load returned %v, want the unreadable host refused by name", err)
	}
}

// The three windows the daemon opens are pressed apart by their own chords, and
// each of them has one so a fresh installation answers a press at all.
func TestEveryWindowHasItsOwnChord(t *testing.T) {
	t.Parallel()

	settings, err := config.Load(envFrom(map[string]string{"TRANS_TARGET": "w1:p3"}))
	if err != nil {
		t.Fatalf("Load returned unexpected error: %v", err)
	}
	if settings.Hotkey != "ctrl+alt+t" {
		t.Errorf("Hotkey is %q, want the panel's chord on by default", settings.Hotkey)
	}
	if settings.SelectHotkey != "ctrl+alt+s" {
		t.Errorf("SelectHotkey is %q, want a chord of its own", settings.SelectHotkey)
	}
	if settings.ConfigHotkey != "ctrl+alt+c" {
		t.Errorf("ConfigHotkey is %q, want a chord of its own", settings.ConfigHotkey)
	}
	if settings.SelectCopy != "" {
		t.Errorf("SelectCopy is %q, want the clipboard read as it stands by default", settings.SelectCopy)
	}
}

func TestTheChordsCanBeSetFromTheEnvironmentOrTheDotEnvFile(t *testing.T) {
	t.Parallel()
	configDir := configDirContaining(t, "TRANS_SELECT_HOTKEY=ctrl+shift+y\n")

	settings, err := config.Load(envFrom(map[string]string{
		"TRANS_TARGET":        "w1:p3",
		"TRANS_CONFIG_DIR":    configDir,
		"TRANS_CONFIG_HOTKEY": "ctrl+alt+k",
	}))
	if err != nil {
		t.Fatalf("Load returned unexpected error: %v", err)
	}
	if settings.SelectHotkey != "ctrl+shift+y" {
		t.Errorf("SelectHotkey is %q, want the chord from the .env file", settings.SelectHotkey)
	}
	if settings.ConfigHotkey != "ctrl+alt+k" {
		t.Errorf("ConfigHotkey is %q, want the chord from the environment", settings.ConfigHotkey)
	}
}

// A chord that conflicts with one the author already uses is better turned off
// than fought over: "off" travels through the file like any other value.
func TestAChordCanBeTurnedOff(t *testing.T) {
	t.Parallel()
	configDir := configDirContaining(t, "TRANS_SELECT_HOTKEY=off\n")

	settings, err := config.Load(envFrom(map[string]string{
		"TRANS_TARGET":     "w1:p3",
		"TRANS_CONFIG_DIR": configDir,
		"TRANS_HOTKEY":     "OFF",
	}))
	if err != nil {
		t.Fatalf("Load returned unexpected error: %v", err)
	}
	if settings.SelectHotkey != "off" {
		t.Errorf("SelectHotkey is %q, want off left standing for the daemon to read", settings.SelectHotkey)
	}
	if settings.Hotkey != "OFF" {
		t.Errorf("Hotkey is %q, want the setting kept as it was written", settings.Hotkey)
	}
}

// The record of prompts already delivered is written unless it is turned off,
// and how much of it is kept is a number the setting has to make sense of.
func TestTheRecordOfDeliveredPromptsIsKeptUnlessTurnedOff(t *testing.T) {
	t.Parallel()

	settings, err := config.Load(envFrom(map[string]string{"TRANS_TARGET": "w1:p1"}))
	if err != nil {
		t.Fatalf("Load returned unexpected error: %v", err)
	}
	if !settings.History {
		t.Error("History is false, want the record kept by default")
	}
	if settings.HistoryLimit != 500 {
		t.Errorf("HistoryLimit is %d, want 500 by default", settings.HistoryLimit)
	}

	off, err := config.Load(envFrom(map[string]string{
		"TRANS_TARGET":        "w1:p1",
		"TRANS_HISTORY":       "0",
		"TRANS_HISTORY_LIMIT": "7",
	}))
	if err != nil {
		t.Fatalf("Load returned unexpected error: %v", err)
	}
	if off.History {
		t.Error("History is true, want it turned off by TRANS_HISTORY=0")
	}
	if off.HistoryLimit != 7 {
		t.Errorf("HistoryLimit is %d, want 7 from TRANS_HISTORY_LIMIT", off.HistoryLimit)
	}
}

// Read mode comes back in the author's own language, which is a decision of its
// own: what the prompts are written into says nothing about what a reply read
// afterwards should come back as.
func TestTheReadLanguageIsChineseWhateverThePromptsAreTranslatedInto(t *testing.T) {
	t.Parallel()

	settings, err := config.Load(envFrom(map[string]string{
		"TRANS_TARGET":        "w1:p3",
		"TRANS_LANGUAGE":      "EN-GB",
		"TRANS_READ_LANGUAGE": "DE",
	}))
	if err != nil {
		t.Fatalf("Load returned unexpected error: %v", err)
	}
	if settings.ReadLanguage != "DE" {
		t.Errorf("ReadLanguage is %q, want the configured one", settings.ReadLanguage)
	}
	if settings.Options.TargetLanguage != "EN-GB" {
		t.Errorf("TargetLanguage is %q, want EN-GB — reading has a language of its own",
			settings.Options.TargetLanguage)
	}

	settings, err = config.Load(envFrom(map[string]string{"TRANS_TARGET": "w1:p3"}))
	if err != nil {
		t.Fatalf("Load returned unexpected error: %v", err)
	}
	if settings.ReadLanguage != "ZH" {
		t.Errorf("ReadLanguage is %q, want ZH by default", settings.ReadLanguage)
	}
}

// Copying a selection out of the window the panel is about to cover takes a
// chord, and which chord that is differs between terminals the way a paste
// chord does.
func TestTheChordThatCopiesASelectionIsItsOwnSetting(t *testing.T) {
	t.Parallel()

	settings, err := config.Load(envFrom(map[string]string{"TRANS_TARGET": "w1:p3"}))
	if err != nil {
		t.Fatalf("Load returned unexpected error: %v", err)
	}
	if settings.CaptureKeys != "ctrl+shift+c" {
		t.Errorf("CaptureKeys is %q, want ctrl+shift+c by default", settings.CaptureKeys)
	}
	if settings.ReadLanguage != "ZH" {
		t.Errorf("ReadLanguage is %q, want ZH — the two settings are separate",
			settings.ReadLanguage)
	}

	settings, err = config.Load(envFrom(map[string]string{
		"TRANS_TARGET":       "w1:p3",
		"TRANS_CAPTURE_KEYS": "ctrl+alt+c",
	}))
	if err != nil {
		t.Fatalf("Load returned unexpected error: %v", err)
	}
	if settings.CaptureKeys != "ctrl+alt+c" {
		t.Errorf("CaptureKeys is %q, want the configured chord", settings.CaptureKeys)
	}
	if settings.ReadLanguage != "ZH" {
		t.Errorf("ReadLanguage is %q, want ZH — the two settings are separate",
			settings.ReadLanguage)
	}
}

func TestSavingKeepsEveryLineItDoesNotName(t *testing.T) {
	t.Parallel()
	file := filepath.Join(t.TempDir(), ".env")
	original := "# my own note\nTRANS_LANGUAGE=EN-GB\n\nTRANS_API_KEY=old-key\n"
	if err := os.WriteFile(file, []byte(original), 0o600); err != nil {
		t.Fatalf("writing .env: %v", err)
	}

	if err := config.Save(file, map[string]string{
		config.ApiKeyVar: "new-key",
		config.ModelVar:  "deepseek-chat",
	}); err != nil {
		t.Fatalf("Save returned unexpected error: %v", err)
	}

	saved, err := os.ReadFile(file)
	if err != nil {
		t.Fatalf("reading .env: %v", err)
	}
	want := "# my own note\nTRANS_LANGUAGE=EN-GB\n\nTRANS_API_KEY=new-key\nTRANS_MODEL=deepseek-chat\n"
	if string(saved) != want {
		t.Errorf("the file is now:\n%s\nwant:\n%s", saved, want)
	}
}

// Clearing a setting is how it goes back to its default; leaving the line
// behind with no value would leave a setting that reads as nothing.
func TestClearingASettingTakesItsLineOutOfTheFile(t *testing.T) {
	t.Parallel()
	file := filepath.Join(t.TempDir(), ".env")
	original := "# credentials\nTRANS_API_KEY=old-key\nTRANS_LANGUAGE=EN-GB\n"
	if err := os.WriteFile(file, []byte(original), 0o600); err != nil {
		t.Fatalf("writing .env: %v", err)
	}

	if err := config.Save(file, map[string]string{config.ApiKeyVar: ""}); err != nil {
		t.Fatalf("Save returned unexpected error: %v", err)
	}

	saved, err := os.ReadFile(file)
	if err != nil {
		t.Fatalf("reading .env: %v", err)
	}
	want := "# credentials\nTRANS_LANGUAGE=EN-GB\n"
	if string(saved) != want {
		t.Errorf("the file is now:\n%s\nwant:\n%s", saved, want)
	}
}

// The settings window hands back values as strings, and reading them must give
// the same strings — quoting of its own, a line break, or a comment marker
// swallowed by the reader would all change the setting behind the author's back.
func TestWhatIsSavedReadsBackAsItWasWritten(t *testing.T) {
	t.Parallel()
	configDir := t.TempDir()
	file := filepath.Join(configDir, ".env")
	command := `translateLocally -m de-en-base | sed 's/  */ /g'`

	if err := config.Save(file, map[string]string{
		config.CommandVar:  command,
		config.LanguageVar: "EN-GB",
		config.EndpointVar: " https://translate.example/v2 ",
		config.ModelVar:    `"deepseek-chat"`,
		config.SubmitVar:   "0",
		config.PasteVar:    "ctrl+shift+v",
	}); err != nil {
		t.Fatalf("Save returned unexpected error: %v", err)
	}

	settings, err := config.Load(envFrom(map[string]string{
		"TRANS_TARGET":     "w1:p3",
		"TRANS_CONFIG_DIR": configDir,
	}))
	if err != nil {
		t.Fatalf("Load returned unexpected error: %v", err)
	}
	if settings.Options.Command != command {
		t.Errorf("Command is %q, want it exactly as it was saved", settings.Options.Command)
	}
	if settings.Options.TargetLanguage != "EN-GB" {
		t.Errorf("TargetLanguage is %q, want EN-GB", settings.Options.TargetLanguage)
	}
	if settings.Options.Endpoint != " https://translate.example/v2 " {
		t.Errorf("Endpoint is %q, want the spaces it was saved with", settings.Options.Endpoint)
	}
	if settings.Options.Model != `"deepseek-chat"` {
		t.Errorf("Model is %q, want the quotes it was saved with", settings.Options.Model)
	}
	if settings.Submit {
		t.Error("Submit is true, want the saved 0 read as off")
	}
	if settings.PasteKeys != "ctrl+shift+v" {
		t.Errorf("PasteKeys is %q, want the saved chord", settings.PasteKeys)
	}
}

func TestSavingWithoutAConfigurationFileIsRefused(t *testing.T) {
	t.Parallel()

	err := config.Save("", map[string]string{config.ProviderVar: "deepl"})
	if err == nil {
		t.Fatal("Save wrote nowhere, want it refused")
	}
	if !strings.Contains(err.Error(), "configuration file") {
		t.Errorf("Save failed with %v, want it said that there is no file", err)
	}
}

// A value with a line break in it is not one setting but two, and the second
// one would be read back as a setting nobody typed.
func TestAValueThatIsNotOneLineIsRefusedAndTheFileStaysAsItWas(t *testing.T) {
	t.Parallel()
	file := filepath.Join(t.TempDir(), ".env")
	original := "TRANS_LANGUAGE=EN-GB\n"
	if err := os.WriteFile(file, []byte(original), 0o600); err != nil {
		t.Fatalf("writing .env: %v", err)
	}

	err := config.Save(file, map[string]string{config.LanguageVar: "EN-GB\nTRANS_API_KEY=planted"})
	if err == nil {
		t.Fatal("Save accepted a value with a line break in it, want it refused")
	}
	if !strings.Contains(err.Error(), "TRANS_LANGUAGE") {
		t.Errorf("Save failed with %v, want the setting named", err)
	}

	saved, readErr := os.ReadFile(file)
	if readErr != nil {
		t.Fatalf("reading .env: %v", readErr)
	}
	if string(saved) != original {
		t.Errorf("the file is now %q, want it untouched by the refused setting", saved)
	}
}

func TestTheFirstSettingsFileIsWrittenAndThenRead(t *testing.T) {
	t.Parallel()
	configDir := t.TempDir()
	file := filepath.Join(configDir, ".env")

	if err := config.Save(file, map[string]string{config.ProviderVar: "gtranslate"}); err != nil {
		t.Fatalf("Save returned unexpected error: %v", err)
	}
	settings, err := config.Load(envFrom(map[string]string{
		"TRANS_TARGET":     "w1:p3",
		"TRANS_CONFIG_DIR": configDir,
	}))
	if err != nil {
		t.Fatalf("Load returned unexpected error: %v", err)
	}
	if settings.Provider != "gtranslate" {
		t.Errorf("Provider is %q, want the service from the file just written", settings.Provider)
	}
}

// The reader used to strip every quote at either end, so a command ending in
// one came back with it missing; only quotes wrapping the whole value are
// quoting.
func TestAQuoteOnlyCountsWhenItWrapsTheWholeValue(t *testing.T) {
	t.Parallel()
	command := `translateLocally | sed 's/  */ /g'`
	configDir := configDirContaining(t, "TRANS_COMMAND="+command+"\nTRANS_LANGUAGE=\"EN-GB\"\n")

	settings, err := config.Load(envFrom(map[string]string{
		"TRANS_TARGET":     "w1:p3",
		"TRANS_CONFIG_DIR": configDir,
	}))
	if err != nil {
		t.Fatalf("Load returned unexpected error: %v", err)
	}
	if settings.Options.Command != command {
		t.Errorf("Command is %q, want the trailing quote it was written with", settings.Options.Command)
	}
	if settings.Options.TargetLanguage != "EN-GB" {
		t.Errorf("TargetLanguage is %q, want the wrapped quotes taken off", settings.Options.TargetLanguage)
	}
}

// Prepare runs against the process's own environment, so these tests point it
// at directories of their own first.
func TestPrepareWritesTheFirstSettingsFile(t *testing.T) {
	configDir := t.TempDir()
	t.Setenv("TRANS_CONFIG_DIR", configDir)
	t.Setenv("TRANS_STATE_DIR", t.TempDir())

	config.Prepare()

	starter, err := os.ReadFile(filepath.Join(configDir, ".env"))
	if err != nil {
		t.Fatalf("reading the settings file Prepare should have written: %v", err)
	}
	if !strings.Contains(string(starter), "# Settings for trans") {
		t.Errorf("the settings file starts with %q, want the starter text", starter)
	}
}

func TestPrepareKeepsASettingsFileThatIsAlreadyThere(t *testing.T) {
	configDir := configDirContaining(t, "TRANS_PROVIDER=gtranslate\n")
	t.Setenv("TRANS_CONFIG_DIR", configDir)
	t.Setenv("TRANS_STATE_DIR", t.TempDir())

	config.Prepare()

	kept, err := os.ReadFile(filepath.Join(configDir, ".env"))
	if err != nil {
		t.Fatalf("reading .env: %v", err)
	}
	if string(kept) != "TRANS_PROVIDER=gtranslate\n" {
		t.Errorf("the settings file is now %q, want the author's own kept", kept)
	}
}

func TestAnUnreadableLimitIsRefused(t *testing.T) {
	t.Parallel()

	for _, variable := range []string{"TRANS_HISTORY_LIMIT"} {
		for _, value := range []string{"0", "viele"} {
			_, err := config.Load(envFrom(map[string]string{
				"TRANS_TARGET": "w1:p1",
				variable:       value,
			}))
			if err == nil || !strings.Contains(err.Error(), variable) {
				t.Errorf("%s=%s failed with %v, want the variable named", variable, value, err)
			}
		}
	}
}

// The theme is a name this package does not judge: whatever is written is
// handed on, and leaving it out means the terminal keeps its own palette.
func TestTheThemeIsReadAsItWasWrittenAndEmptyWhenUnset(t *testing.T) {
	t.Parallel()

	settings, err := config.Load(envFrom(map[string]string{"TRANS_TARGET": "w1:p3"}))
	if err != nil {
		t.Fatalf("Load returned unexpected error: %v", err)
	}
	if settings.Theme != "" {
		t.Errorf("Theme is %q, want it empty so the terminal's palette stands", settings.Theme)
	}

	settings, err = config.Load(envFrom(map[string]string{
		"TRANS_TARGET": "w1:p3",
		"TRANS_THEME":  "dracula",
	}))
	if err != nil {
		t.Fatalf("Load returned unexpected error: %v", err)
	}
	if settings.Theme != "dracula" {
		t.Errorf("Theme is %q, want the name from the environment", settings.Theme)
	}
}

// The draft box asks for a number of rows; nobody asking gets the default and
// a number nobody could ask for is refused by name rather than quietly changed.
func TestTheDraftRowsDefaultToSixAndAValueOutsideTheEndsIsRefused(t *testing.T) {
	t.Parallel()

	settings, err := config.Load(envFrom(map[string]string{"TRANS_TARGET": "w1:p3"}))
	if err != nil {
		t.Fatalf("Load returned unexpected error: %v", err)
	}
	if settings.DraftRows != 6 {
		t.Errorf("DraftRows is %d, want 6 by default", settings.DraftRows)
	}

	settings, err = config.Load(envFrom(map[string]string{
		"TRANS_TARGET":     "w1:p3",
		"TRANS_DRAFT_ROWS": "10",
	}))
	if err != nil {
		t.Fatalf("Load returned unexpected error: %v", err)
	}
	if settings.DraftRows != 10 {
		t.Errorf("DraftRows is %d, want 10 from TRANS_DRAFT_ROWS", settings.DraftRows)
	}

	for _, value := range []string{"99", "abc"} {
		_, err := config.Load(envFrom(map[string]string{
			"TRANS_TARGET":     "w1:p1",
			"TRANS_DRAFT_ROWS": value,
		}))
		if err == nil || !strings.Contains(err.Error(), "TRANS_DRAFT_ROWS") {
			t.Errorf("TRANS_DRAFT_ROWS=%s failed with %v, want the variable named", value, err)
		}
	}
}

// The popup asks for a width the same way: a default when nothing is said, a
// number inside the ends when it is, and a refusal when it is not.
func TestThePanelWidthDefaultsToOneHundredAndTenAndAnOutOfRangeNumberIsRefused(t *testing.T) {
	t.Parallel()

	settings, err := config.Load(envFrom(map[string]string{"TRANS_TARGET": "w1:p3"}))
	if err != nil {
		t.Fatalf("Load returned unexpected error: %v", err)
	}
	if settings.PanelWidth != 110 {
		t.Errorf("PanelWidth is %d, want 110 by default", settings.PanelWidth)
	}

	settings, err = config.Load(envFrom(map[string]string{
		"TRANS_TARGET":      "w1:p3",
		"TRANS_PANEL_WIDTH": "128",
	}))
	if err != nil {
		t.Fatalf("Load returned unexpected error: %v", err)
	}
	if settings.PanelWidth != 128 {
		t.Errorf("PanelWidth is %d, want 128 from TRANS_PANEL_WIDTH", settings.PanelWidth)
	}

	for _, value := range []string{"30", "999"} {
		_, err := config.Load(envFrom(map[string]string{
			"TRANS_TARGET":      "w1:p1",
			"TRANS_PANEL_WIDTH": value,
		}))
		if err == nil || !strings.Contains(err.Error(), "TRANS_PANEL_WIDTH") {
			t.Errorf("TRANS_PANEL_WIDTH=%s failed with %v, want the variable named", value, err)
		}
	}
}
