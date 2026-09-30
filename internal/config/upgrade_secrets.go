package config

import (
	"errors"
	"os"
	"path/filepath"
	"time"

	"trans/internal/secrets"
)

// TRANS_KEYS says how the provider key is kept on disk: "dpapi" wraps it
// with Windows Data Protection (the default), "plain" leaves the .env line
// alone and turns the migration off. The name itself lives with the other
// settings in config.go; the two values it takes live here.
const (
	keysDPAPI = "dpapi"
	keysPlain = "plain"
)

// UpgradeSecrets moves a plaintext API key out of the .env file and into the
// protected store, then leaves Load to find it there. The daemon and the
// panel call it right after Load; calling it twice is harmless, because the
// second call finds nothing to move.
//
// The order matters: the file is copied aside first, the encrypted copy is
// written next, and only then does the plaintext line go — so a failure at
// any point leaves the key recoverable rather than gone.
//
// On systems without DPAPI, and when TRANS_KEYS=plain, it does nothing and
// answers a note for the caller to log: a key the user chose to keep in the
// file still works, and refusing to start would be rude.
func UpgradeSecrets(settings Settings) string {
	if settings.ConfigFile == "" || settings.Keys == keysPlain {
		return ""
	}

	variable, value := movable(settings)
	if variable == "" {
		return ""
	}

	if err := backup(settings.ConfigFile); err != nil {
		return note(variable, err)
	}
	directory := filepath.Dir(settings.ConfigFile)
	if err := secrets.Store(directory, variable, []byte(value), secrets.CurrentUser); err != nil {
		// Nothing to protect with here: leave the line where it is rather
		// than half-migrate.
		if errors.Is(err, secrets.ErrUnsupported) {
			return ""
		}
		return note(variable, err)
	}
	// The value now lives in secrets.json; the line goes, and everything
	// else in the file stays exactly as it was.
	if err := Save(settings.ConfigFile, map[string]string{variable: ""}); err != nil {
		return note(variable, err)
	}
	return ""
}

// note says what went wrong in a way a log line can carry: which setting and
// why, without the value anywhere in it.
func note(variable string, err error) string {
	return variable + " was left in the settings file: " + err.Error()
}

// movable answers which setting holds a plaintext key worth moving and what
// it holds. The scoped variable wins over the plain one, the same way Load
// reads them, so a key written for one service is filed back under that
// service's name later.
//
// Only keys that came from the file are candidates: one handed over in the
// environment is the caller's to manage, and moving it would change nothing
// anyway — the environment still wins at the next Load.
func movable(settings Settings) (string, string) {
	stored := readDotenv(settings.ConfigFile)
	if len(stored) == 0 {
		return "", ""
	}
	if scoped := ScopedKeyVar(settings.Provider); scoped != "" {
		if value := stored[scoped]; value != "" {
			return scoped, value
		}
	}
	if value := stored[ApiKeyVar]; value != "" {
		return ApiKeyVar, value
	}
	return "", ""
}

// backup keeps the file as it stood before the rewrite, under a name with
// the moment in it. The plaintext key is about to leave, and a recovery
// copy that costs nothing is worth more than any promise about atomic
// writes. No backup is taken when there is nothing to migrate — the file is
// not being touched then, and a .env.bak with no story to tell is just
// litter beside the settings.
func backup(file string) error {
	content, err := os.ReadFile(file)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil
		}
		return err
	}
	stamp := time.Now().Format("20060102-150405")
	return os.WriteFile(file+".bak."+stamp, content, 0o600)
}

// ResolveKey fills the key in from the protected store when the settings
// carry none, so the services see Options.APIKey exactly as before the
// migration. It runs on every system: where DPAPI does not exist the store
// answers ErrUnsupported and the key is simply left as it was.
func ResolveKey(settings *Settings) {
	if settings.Options.APIKey != "" || settings.ConfigFile == "" {
		return
	}
	if settings.Keys == keysPlain {
		return
	}
	variable := ScopedKeyVar(settings.Provider)
	if variable == "" {
		variable = ApiKeyVar
	}
	value, err := secrets.Load(filepath.Dir(settings.ConfigFile), variable)
	if err != nil {
		return
	}
	settings.Options.APIKey = string(value)
}

// KeysMode is TRANS_KEYS as the settings window shows it, defaulting to the
// protection that is on unless the user turned it off.
func (s Settings) KeysMode() string {
	if s.Keys == keysPlain {
		return keysPlain
	}
	return keysDPAPI
}
