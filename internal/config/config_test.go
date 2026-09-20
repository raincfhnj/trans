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
		"TRANS_TARGET":      "w1:p3",
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
		"TRANS_TARGET":      "w1:p3",
		"TRANS_PROVIDER":    "acme",
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
		"TRANS_TARGET":      "w1:p3",
		"TRANS_CONFIG_DIR": configDir,
		"TRANS_API_KEY":     "key-from-environment",
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
