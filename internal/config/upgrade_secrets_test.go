package config_test

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"trans/internal/config"
	"trans/internal/secrets"
)

// writeDotenv puts a .env file in a directory of its own, which is the whole
// world these tests run in — envFrom comes from config_test.go beside them.
func writeDotenv(t *testing.T, content string) string {
	t.Helper()
	directory := t.TempDir()
	file := filepath.Join(directory, ".env")
	if err := os.WriteFile(file, []byte(content), 0o600); err != nil {
		t.Fatalf("writing the settings file: %v", err)
	}
	return directory
}

// The migration is the point of the package: a key sitting in the file comes
// out the other side wrapped, the line is gone, and nothing else in the file
// moves.
func TestAPlainKeyMovesIntoTheProtectedStore(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("DPAPI only exists on Windows")
	}
	directory := writeDotenv(t,
		"# a comment that must survive\n"+
			"TRANS_PROVIDER=deepl\n"+
			"TRANS_API_KEY=plain-key-123\n"+
			"TRANS_LANGUAGE=EN-GB\n")
	settings, err := config.Load(envFrom(map[string]string{
		"TRANS_CONFIG_DIR": directory,
	}))
	if err != nil {
		t.Fatalf("loading: %v", err)
	}

	if note := config.UpgradeSecrets(&settings); note != "" {
		t.Fatalf("migrating: %s", note)
	}

	after, err := os.ReadFile(filepath.Join(directory, ".env"))
	if err != nil {
		t.Fatalf("reading back: %v", err)
	}
	text := string(after)
	if strings.Contains(text, "plain-key-123") {
		t.Error("the key is still in the settings file in the clear")
	}
	if !strings.Contains(text, "# a comment that must survive") {
		t.Error("the migration took more than the key with it")
	}
	if !strings.Contains(text, "TRANS_LANGUAGE=EN-GB") {
		t.Error("another setting was lost in the migration")
	}

	// The file that was rewritten has its moment kept beside it, so the
	// plaintext key can be recovered if the encrypted copy is ever lost.
	matches, err := filepath.Glob(filepath.Join(directory, ".env.bak.*"))
	if err != nil || len(matches) != 1 {
		t.Fatalf("backup files: %v, %v", matches, err)
	}
	kept, err := os.ReadFile(matches[0])
	if err != nil {
		t.Fatalf("reading the backup: %v", err)
	}
	if !strings.Contains(string(kept), "plain-key-123") {
		t.Error("the backup does not hold what the file held")
	}

	// And Load finds the key where it went: the services never see a
	// difference.
	moved, err := config.Load(envFrom(map[string]string{
		"TRANS_CONFIG_DIR": directory,
	}))
	if err != nil {
		t.Fatalf("reloading: %v", err)
	}
	config.ResolveKey(&moved)
	if moved.Options.APIKey != "plain-key-123" {
		t.Errorf("the key came back as %q, want the one that was stored", moved.Options.APIKey)
	}
}

// A key written for one service keeps its own name through the move, so
// turning the migration back off would put it on the right line again.
func TestAScopedKeyKeepsItsName(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("DPAPI only exists on Windows")
	}
	directory := writeDotenv(t,
		"TRANS_PROVIDER=deepl\n"+
			"TRANS_DEEPL_API_KEY=scoped-secret\n")
	settings, err := config.Load(envFrom(map[string]string{
		"TRANS_CONFIG_DIR": directory,
	}))
	if err != nil {
		t.Fatalf("loading: %v", err)
	}
	if note := config.UpgradeSecrets(&settings); note != "" {
		t.Fatalf("migrating: %s", note)
	}

	contents, err := secrets.Load(directory, "TRANS_DEEPL_API_KEY")
	if err != nil {
		t.Fatalf("the scoped key was not filed under its own name: %v", err)
	}
	if string(contents) != "scoped-secret" {
		t.Errorf("got %q, want %q", contents, "scoped-secret")
	}
	if _, err := secrets.Load(directory, "TRANS_API_KEY"); err == nil {
		t.Error("the scoped key also landed under the plain name")
	}
}

// TRANS_KEYS=plain is how a user says the file is theirs to keep: nothing
// moves, nothing breaks.
func TestPlainModeLeavesTheFileAlone(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("DPAPI only exists on Windows")
	}
	directory := writeDotenv(t, "TRANS_API_KEY=keep-me\n")
	settings, err := config.Load(envFrom(map[string]string{
		"TRANS_CONFIG_DIR": directory,
		"TRANS_KEYS":       "plain",
	}))
	if err != nil {
		t.Fatalf("loading: %v", err)
	}
	if note := config.UpgradeSecrets(&settings); note != "" {
		t.Fatalf("migrating in plain mode: %s", note)
	}

	after, err := os.ReadFile(filepath.Join(directory, ".env"))
	if err != nil {
		t.Fatalf("reading back: %v", err)
	}
	if !strings.Contains(string(after), "TRANS_API_KEY=keep-me") {
		t.Error("plain mode did not keep the key in the file")
	}
	if matches, _ := filepath.Glob(filepath.Join(directory, ".env.bak.*")); len(matches) != 0 {
		t.Errorf("plain mode left %d backup files behind", len(matches))
	}
}

// Nothing to move is not an occasion for writing anything: the file is
// untouched and no backup is taken of a file that did not change.
func TestNothingToMoveChangesNothing(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("DPAPI only exists on Windows")
	}
	directory := writeDotenv(t, "TRANS_PROVIDER=gtranslate\n")
	settings, err := config.Load(envFrom(map[string]string{
		"TRANS_CONFIG_DIR": directory,
	}))
	if err != nil {
		t.Fatalf("loading: %v", err)
	}
	if note := config.UpgradeSecrets(&settings); note != "" {
		t.Fatalf("migrating: %s", note)
	}
	if matches, _ := filepath.Glob(filepath.Join(directory, ".env.bak.*")); len(matches) != 0 {
		t.Errorf("a backup was taken with nothing to migrate: %v", matches)
	}
}

// Calling it again after the move finds nothing and writes nothing, which is
// what lets the daemon and the panel both call it at start.
func TestRunningTwiceIsHarmless(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("DPAPI only exists on Windows")
	}
	directory := writeDotenv(t, "TRANS_API_KEY=once-only\n")
	settings, err := config.Load(envFrom(map[string]string{
		"TRANS_CONFIG_DIR": directory,
	}))
	if err != nil {
		t.Fatalf("loading: %v", err)
	}
	if note := config.UpgradeSecrets(&settings); note != "" {
		t.Fatalf("first run: %s", note)
	}
	again, err := config.Load(envFrom(map[string]string{
		"TRANS_CONFIG_DIR": directory,
	}))
	if err != nil {
		t.Fatalf("reloading: %v", err)
	}
	if note := config.UpgradeSecrets(&again); note != "" {
		t.Fatalf("second run: %s", note)
	}
	if matches, _ := filepath.Glob(filepath.Join(directory, ".env.bak.*")); len(matches) != 1 {
		t.Errorf("got %d backups after two runs, want 1", len(matches))
	}
}

// Where DPAPI does not exist the migration stands down with no note: the
// key simply stays where it was, and the tree still builds and tests.
func TestUnsupportedSystemsStandDown(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("this is the refusal for everything but Windows")
	}
	directory := writeDotenv(t, "TRANS_API_KEY=stay-put\n")
	settings, err := config.Load(envFrom(map[string]string{
		"TRANS_CONFIG_DIR": directory,
	}))
	if err != nil {
		t.Fatalf("loading: %v", err)
	}
	if note := config.UpgradeSecrets(&settings); note != "" {
		t.Errorf("got note %q, want none", note)
	}
	after, err := os.ReadFile(filepath.Join(directory, ".env"))
	if err != nil {
		t.Fatalf("reading back: %v", err)
	}
	if !strings.Contains(string(after), "TRANS_API_KEY=stay-put") {
		t.Error("the key did not stay put")
	}
	config.ResolveKey(&settings)
	if settings.Options.APIKey != "stay-put" {
		t.Errorf("the key came back as %q", settings.Options.APIKey)
	}
}
