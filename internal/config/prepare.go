package config

import (
	"os"
	"path/filepath"
)

// Prepare gives the process the directories the plugin uses — settings in the
// config directory, kept drafts in the state directory — before Load is asked
// to read them, and writes the first settings file so there is something to
// edit. Both the panel and the daemon call it: without it a .env would only be
// found by an invocation that happened to be started with TRANS_CONFIG_DIR set.
func Prepare() {
	if os.Getenv(configDirVar) == "" {
		if directory, err := os.UserConfigDir(); err == nil {
			_ = os.Setenv(configDirVar, filepath.Join(directory, "trans"))
		}
	}
	if os.Getenv(stateDirVar) == "" {
		if directory, err := os.UserCacheDir(); err == nil {
			state := filepath.Join(directory, "trans", "state")
			_ = os.MkdirAll(state, 0o700)
			_ = os.Setenv(stateDirVar, state)
		}
	}
	writeStarterEnv()
}

// A directory with nothing in it is not much of a start, since the settings are
// files rather than a screen of options.
func writeStarterEnv() {
	directory := os.Getenv(configDirVar)
	if directory == "" {
		return
	}
	file := filepath.Join(directory, dotenvName)
	if _, err := os.Stat(file); err == nil {
		return
	}
	if err := os.MkdirAll(directory, 0o700); err != nil {
		return
	}
	starter := "" +
		"# Settings for trans. A line here is a setting; an environment\n" +
		"# variable of the same name wins over it.\n" +
		"#\n" +
		"# Without a key, the free service needs no account at all:\n" +
		"#   TRANS_PROVIDER=gtranslate\n" +
		"#\n" +
		"# A key means DeepL, and free keys end in :fx:\n" +
		"#   TRANS_API_KEY=your-key\n" +
		"#\n" +
		"# Any OpenAI-compatible API, a gateway or a model on this machine:\n" +
		"#   TRANS_PROVIDER=openai\n" +
		"#   TRANS_API_KEY=sk-...\n" +
		"#   TRANS_ENDPOINT=https://api.deepseek.com/v1\n" +
		"#   TRANS_MODEL=deepseek-chat\n" +
		"#\n" +
		"# The daemon's chords: off turns one off, and the settings window\n" +
		"# writes these files:\n" +
		"#   TRANS_HOTKEY=ctrl+alt+t\n" +
		"#   TRANS_SELECT_HOTKEY=ctrl+alt+s\n" +
		"#   TRANS_CONFIG_HOTKEY=ctrl+alt+c\n"
	_ = os.WriteFile(file, []byte(starter), 0o600)
}
