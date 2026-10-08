package settings_test

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"trans/internal/config"
	"trans/internal/settings"
	"trans/internal/translation"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/exp/teatest"
)

// Enough services to step through, in the order the registry would give them.
var services = []string{"deepl", "google", "openai"}

func envFrom(pairs map[string]string) func(string) string {
	return func(key string) string { return pairs[key] }
}

// The window asks for a chord to be checked rather than checking one itself,
// so these tests stay about the window rather than about Win32.
func acceptingChord(string) error { return nil }

func refusingBogus(spec string) error {
	if strings.Contains(spec, "bogus") {
		return errors.New(`"bogus" is not a key`)
	}
	return nil
}

func fullSettings(file string) config.Settings {
	return config.Settings{
		Provider: "deepl",
		Options: translation.Options{
			APIKey:         "sk-secret-123",
			TargetLanguage: "EN-GB",
			Endpoint:       "https://translate.example/v2",
			Model:          "model-x",
		},
		ConfigFile:   file,
		Submit:       true,
		Live:         true,
		Vim:          false,
		KeepDraft:    true,
		Confirm:      false,
		SelectCopy:   "ctrl+shift+c",
		Hotkey:       "ctrl+alt+t",
		SelectHotkey: "ctrl+alt+s",
		ConfigHotkey: "ctrl+alt+c",
	}
}

func newWindow(t *testing.T, options settings.Options) *teatest.TestModel {
	t.Helper()
	return teatest.NewTestModel(t, settings.New(options),
		teatest.WithInitialTermSize(100, 24))
}

// waitFor reads the window until every wanted word has been on screen,
// answering with everything that was read — which is what a test then checks
// what is not there against.
func waitFor(t *testing.T, model *teatest.TestModel, wanted ...string) []byte {
	t.Helper()
	var shown []byte
	teatest.WaitFor(t, model.Output(), func(out []byte) bool {
		if containsAll(out, wanted...) {
			shown = append([]byte(nil), out...)
			return true
		}
		return false
	}, teatest.WithDuration(3*time.Second))
	return shown
}

func containsAll(shown []byte, wanted ...string) bool {
	for _, word := range wanted {
		if !bytes.Contains(shown, []byte(word)) {
			return false
		}
	}
	return true
}

func closeWindow(t *testing.T, model *teatest.TestModel) {
	t.Helper()
	model.Send(tea.KeyMsg{Type: tea.KeyCtrlC})
	model.WaitFinished(t, teatest.WithFinalTimeout(10*time.Second))
}

func stepDown(model *teatest.TestModel, rows int) {
	for range rows {
		model.Send(tea.KeyMsg{Type: tea.KeyDown})
	}
}

func savedFile(t *testing.T, file string) string {
	t.Helper()
	contents, err := os.ReadFile(file)
	if err != nil {
		t.Fatalf("reading the saved settings: %v", err)
	}
	return string(contents)
}

func TestEverySettingIsShownWithTheValueItHas(t *testing.T) {
	t.Parallel()
	model := newWindow(t, settings.Options{
		Settings: fullSettings(""),
		Services: services,
		Chord:    acceptingChord,
	})

	waitFor(t, model,
		"service", "deepl",
		"endpoint", "https://translate.example/v2",
		"language", "EN-GB",
		"panel hotkey", "ctrl+alt+t",
		"select copy", "ctrl+shift+c",
	)
	closeWindow(t, model)
}

// The header says where the settings are written, which is the file the save
// below would put them in.
func TestTheHeaderSaysWhereTheSettingsAreWritten(t *testing.T) {
	t.Parallel()
	file := filepath.Join(t.TempDir(), "mine.env")

	model := newWindow(t, settings.Options{
		Settings: fullSettings(file),
		Services: services,
		Chord:    acceptingChord,
	})

	// The path is cut from the left when it is too long, so the file at its
	// end is what stays readable.
	waitFor(t, model, "settings", "mine.env")
	closeWindow(t, model)
}

// The window is drawn over whatever else is on the screen: a key is changed
// through this window, not read off it.
func TestTheKeyIsNotShownAsItIs(t *testing.T) {
	t.Parallel()
	model := newWindow(t, settings.Options{
		Settings: fullSettings(""),
		Services: services,
		Chord:    acceptingChord,
	})

	shown := waitFor(t, model, "api key", "••••••••")
	if bytes.Contains(shown, []byte("sk-secret-123")) {
		t.Error("the key is on screen as it stands")
	}
	closeWindow(t, model)
}

// The service row steps through what the registry offers, and a save puts the
// one it stopped on into the file — keeping every line it was not asked about.
func TestTheServiceStepsThroughAndSavingKeepsTheOtherLines(t *testing.T) {
	t.Parallel()
	file := filepath.Join(t.TempDir(), ".env")
	original := "# my own settings\nTRANS_LANGUAGE=EN-GB\n"
	if err := os.WriteFile(file, []byte(original), 0o600); err != nil {
		t.Fatalf("writing .env: %v", err)
	}

	model := newWindow(t, settings.Options{
		Settings: fullSettings(file),
		Services: services,
		Chord:    acceptingChord,
	})

	model.Send(tea.KeyMsg{Type: tea.KeyRight})
	waitFor(t, model, "google")

	model.Send(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("s")})
	waitFor(t, model, "settings written")

	saved := savedFile(t, file)
	if !strings.Contains(saved, "TRANS_PROVIDER=google") {
		t.Errorf("the file does not hold the chosen service:\n%s", saved)
	}
	if !strings.Contains(saved, "# my own settings") || !strings.Contains(saved, "TRANS_LANGUAGE=EN-GB") {
		t.Errorf("the lines the window was not asked about are gone:\n%s", saved)
	}
	closeWindow(t, model)
}

// The flag rows are switched with an arrow, and what is written is the same
// 1 or 0 the reader takes as on and off.
func TestAFlagIsSwitchedWithAnArrowAndSavedAsItIsRead(t *testing.T) {
	t.Parallel()
	file := filepath.Join(t.TempDir(), ".env")

	model := newWindow(t, settings.Options{
		Settings: fullSettings(file),
		Services: services,
		Chord:    acceptingChord,
	})

	stepDown(model, 6) // submit
	model.Send(tea.KeyMsg{Type: tea.KeyLeft})
	model.Send(tea.KeyMsg{Type: tea.KeyCtrlS})

	waitFor(t, model, "settings written")
	if saved := savedFile(t, file); !strings.Contains(saved, "TRANS_SUBMIT=0") {
		t.Errorf("the file does not hold the switched-off flag:\n%s", saved)
	}
	closeWindow(t, model)
}

// A row that is written takes enter to open, and what is typed there is what
// the save writes.
func TestARowIsEditedWithEnterAndKeptWithEnter(t *testing.T) {
	t.Parallel()
	file := filepath.Join(t.TempDir(), ".env")

	model := newWindow(t, settings.Options{
		Settings: fullSettings(file),
		Services: services,
		Chord:    acceptingChord,
	})

	stepDown(model, 3) // model
	model.Send(tea.KeyMsg{Type: tea.KeyEnter})
	waitFor(t, model, "enter", "keep")

	// The old value stands in the writing; the new one goes after it.
	model.Send(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("-v2")})
	model.Send(tea.KeyMsg{Type: tea.KeyEnter})
	model.Send(tea.KeyMsg{Type: tea.KeyCtrlS})

	waitFor(t, model, "settings written")
	if saved := savedFile(t, file); !strings.Contains(saved, "TRANS_MODEL=model-x-v2") {
		t.Errorf("the file does not hold what was written:\n%s", saved)
	}
	closeWindow(t, model)
}

// Escape does not take changes with it by accident: the first press says what
// dropping them means, the second one leaves.
func TestEscapeAsksBeforeDroppingUnsavedChanges(t *testing.T) {
	t.Parallel()
	model := newWindow(t, settings.Options{
		Settings: fullSettings(""),
		Services: services,
		Chord:    acceptingChord,
	})

	model.Send(tea.KeyMsg{Type: tea.KeyRight}) // service: deepl → google
	waitFor(t, model, "google")

	model.Send(tea.KeyMsg{Type: tea.KeyEsc})
	waitFor(t, model, "unsaved", "esc again")

	model.Send(tea.KeyMsg{Type: tea.KeyEsc})
	model.WaitFinished(t, teatest.WithFinalTimeout(10*time.Second))
}

func TestEscapeClosesWhenNothingHasChanged(t *testing.T) {
	t.Parallel()
	model := newWindow(t, settings.Options{
		Settings: fullSettings(""),
		Services: services,
		Chord:    acceptingChord,
	})
	waitFor(t, model, "service")

	model.Send(tea.KeyMsg{Type: tea.KeyEsc})
	model.WaitFinished(t, teatest.WithFinalTimeout(10*time.Second))
}

// Ctrl+C is the way out of every window here, even from the middle of writing.
func TestCtrlCClosesEvenWhileARowIsBeingWritten(t *testing.T) {
	t.Parallel()
	model := newWindow(t, settings.Options{
		Settings: fullSettings(""),
		Services: services,
		Chord:    acceptingChord,
	})

	stepDown(model, 1) // api key opens for writing
	model.Send(tea.KeyMsg{Type: tea.KeyEnter})
	waitFor(t, model, "enter", "keep")

	model.Send(tea.KeyMsg{Type: tea.KeyCtrlC})
	model.WaitFinished(t, teatest.WithFinalTimeout(10*time.Second))
}

// A variable set in the environment wins over the file this window writes, so
// the row says so rather than letting a save look like it took effect.
func TestASettingFromTheEnvironmentIsMarked(t *testing.T) {
	t.Parallel()
	model := newWindow(t, settings.Options{
		Settings: fullSettings(""),
		Getenv:   envFrom(map[string]string{"TRANS_LIVE": "0"}),
		Services: services,
		Chord:    acceptingChord,
	})

	shown := waitFor(t, model, "live", "env")
	if !bytes.Contains(shown, []byte(" live")) {
		t.Error("the marked setting is not on screen at all")
	}
	closeWindow(t, model)
}

// A chord that cannot be pressed is refused by name — the row and its
// variable — and the file stays as it was.
func TestAChordThatCannotBePressedIsRefusedByName(t *testing.T) {
	t.Parallel()
	file := filepath.Join(t.TempDir(), ".env")
	original := "TRANS_LANGUAGE=EN-GB\n"
	if err := os.WriteFile(file, []byte(original), 0o600); err != nil {
		t.Fatalf("writing .env: %v", err)
	}

	model := newWindow(t, settings.Options{
		Settings: fullSettings(file),
		Services: services,
		Chord:    refusingBogus,
	})

	stepDown(model, 12) // select copy
	model.Send(tea.KeyMsg{Type: tea.KeyEnter})
	model.Send(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("bogus")})
	model.Send(tea.KeyMsg{Type: tea.KeyEnter})
	model.Send(tea.KeyMsg{Type: tea.KeyCtrlS})

	waitFor(t, model, "select copy", "TRANS_SELECT_COPY", "is not a key")
	if saved := savedFile(t, file); saved != original {
		t.Errorf("the file is now %q, want it untouched by the refused chord", saved)
	}
	closeWindow(t, model)
}

// The daemon waits on its chords from the start, so saving one says which
// program has to be started again for it to press.
func TestASavedHotkeySaysTheDaemonNeedsRestarting(t *testing.T) {
	t.Parallel()
	file := filepath.Join(t.TempDir(), ".env")

	model := newWindow(t, settings.Options{
		Settings: fullSettings(file),
		Services: services,
		Chord:    acceptingChord,
	})

	stepDown(model, 13) // panel hotkey
	model.Send(tea.KeyMsg{Type: tea.KeyEnter})
	model.Send(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("p")})
	model.Send(tea.KeyMsg{Type: tea.KeyEnter})
	model.Send(tea.KeyMsg{Type: tea.KeyCtrlS})

	waitFor(t, model, "settings written", "restart trans-windowd")
	if saved := savedFile(t, file); !strings.Contains(saved, "TRANS_HOTKEY=ctrl+alt+tp") {
		t.Errorf("the file does not hold the new chord:\n%s", saved)
	}
	closeWindow(t, model)
}

// Saving without a change is a question the window can answer itself.
func TestSavingWithNothingChangedSaysSo(t *testing.T) {
	t.Parallel()
	model := newWindow(t, settings.Options{
		Settings: fullSettings(""),
		Services: services,
		Chord:    acceptingChord,
	})
	waitFor(t, model, "service")

	model.Send(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("s")})
	waitFor(t, model, "nothing has changed")
	closeWindow(t, model)
}

// A save into nowhere is said the same way as any other refusal: as what went
// wrong, not as a save that worked.
func TestSavingWithNoConfigurationFileIsRefused(t *testing.T) {
	t.Parallel()
	model := newWindow(t, settings.Options{
		Settings: fullSettings(""),
		Services: services,
		Chord:    acceptingChord,
	})
	waitFor(t, model, "service")

	model.Send(tea.KeyMsg{Type: tea.KeyRight})
	waitFor(t, model, "google")
	model.Send(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("s")})

	waitFor(t, model, "no configuration file")
	closeWindow(t, model)
}
