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
	"trans/internal/frame"
	"trans/internal/settings"
	"trans/internal/translation"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/exp/teatest"
	"github.com/muesli/termenv"
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
		DraftRows:    config.DefaultDraftRows,
		PanelWidth:   config.DefaultPanelWidth,
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

	stepDown(model, 15) // select copy
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

	stepDown(model, 16) // panel hotkey
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

// The theme row steps through the themes the frame offers, auto first and
// then its own names in its own order, and the window puts the colours on as
// the arrows move — so the theme is picked by looking at it rather than by
// closing the panel and opening it again.
func TestTheThemeRowStepsThroughTheFrameThemesAndDressesTheWindow(t *testing.T) {
	t.Parallel()
	previous := lipgloss.ColorProfile()
	lipgloss.SetColorProfile(termenv.ANSI)
	t.Cleanup(func() { lipgloss.SetColorProfile(previous) })

	model := newWindow(t, settings.Options{
		Settings: fullSettings(""),
		Services: services,
		Chord:    acceptingChord,
	})

	// The window opens on the palette with no name: slot 5 is the accent it
	// has always drawn with, and the row says auto.
	shown := waitFor(t, model, "theme", "auto")
	if !bytes.Contains(shown, []byte("\x1b[35m")) {
		t.Error("the window is not drawn with the accent of the palette with no name")
	}
	stepDown(model, 11) // theme

	// Every arrow lands on the next theme the frame names, and by the time it
	// is named the window is already wearing it: the border is lit with the
	// slot that theme puts its accent in.
	accents := map[string]string{
		"ocean": "\x1b[34m", "forest": "\x1b[32m",
		"amber": "\x1b[33m", "mono": "\x1b[37m",
	}
	for _, theme := range frame.ThemeNames() {
		accent, known := accents[theme]
		if !known {
			t.Fatalf("no accent slot is recorded for the theme %q", theme)
		}
		model.Send(tea.KeyMsg{Type: tea.KeyRight})
		shown = waitFor(t, model, theme, accent)
		if !bytes.Contains(shown, []byte(accent)) {
			t.Errorf("stepping to %s did not repaint the window in the accent of that theme", theme)
		}
	}

	// One more arrow and the themes are behind the row: it is back on auto,
	// and the window is back on the palette with no name.
	model.Send(tea.KeyMsg{Type: tea.KeyRight})
	shown = waitFor(t, model, "auto", "\x1b[35m")
	if !bytes.Contains(shown, []byte("\x1b[35m")) {
		t.Error("leaving the themes behind did not put the window back on its own palette")
	}
	closeWindow(t, model)
}

// The draft rows row is a number: an arrow moves it by one, and the save
// writes the number it stopped on.
func TestTheDraftRowsArrowStepsItAndTheSaveWritesTheNumber(t *testing.T) {
	t.Parallel()
	file := filepath.Join(t.TempDir(), ".env")

	model := newWindow(t, settings.Options{
		Settings: fullSettings(file),
		Services: services,
		Chord:    acceptingChord,
	})

	stepDown(model, 12) // draft rows, which stands on 6
	model.Send(tea.KeyMsg{Type: tea.KeyRight})
	model.Send(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("s")})

	waitFor(t, model, "settings written")
	if saved := savedFile(t, file); !strings.Contains(saved, "TRANS_DRAFT_ROWS=7") {
		t.Errorf("the file does not hold the stepped number:\n%s", saved)
	}
	closeWindow(t, model)
}

// A number row keeps its arrows inside the range it stands between: however
// far right they are pressed, the number stops at the high end of its range.
func TestTheDraftRowsArrowNeverTakesTheNumberPastItsUpperEnd(t *testing.T) {
	t.Parallel()
	file := filepath.Join(t.TempDir(), ".env")

	model := newWindow(t, settings.Options{
		Settings: fullSettings(file),
		Services: services,
		Chord:    acceptingChord,
	})

	stepDown(model, 12) // draft rows, which stands on 6
	for range 20 {
		model.Send(tea.KeyMsg{Type: tea.KeyRight})
	}
	model.Send(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("s")})

	waitFor(t, model, "settings written")
	saved := savedFile(t, file)
	if !strings.Contains(saved, "TRANS_DRAFT_ROWS=16") {
		t.Errorf("the arrows took the number past the top of its range:\n%s", saved)
	}
	closeWindow(t, model)
}

// A number written outside the range its row stands between is refused by
// name — the row and its variable — and the file stays as it was, the way a
// chord that cannot be pressed is refused.
func TestANumberOutsideItsRangeIsRefusedAndTheFileStaysAsItWas(t *testing.T) {
	t.Parallel()
	file := filepath.Join(t.TempDir(), ".env")
	original := "TRANS_LANGUAGE=EN-GB\n"
	if err := os.WriteFile(file, []byte(original), 0o600); err != nil {
		t.Fatalf("writing .env: %v", err)
	}

	model := newWindow(t, settings.Options{
		Settings: fullSettings(file),
		Services: services,
		Chord:    acceptingChord,
	})

	stepDown(model, 12) // draft rows
	model.Send(tea.KeyMsg{Type: tea.KeyEnter})
	model.Send(tea.KeyMsg{Type: tea.KeyBackspace}) // the 6 that was standing there
	model.Send(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("99")})
	model.Send(tea.KeyMsg{Type: tea.KeyEnter})
	model.Send(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("s")})

	shown := waitFor(t, model, "draft rows", "TRANS_DRAFT_ROWS",
		"a whole number between 4 and 16")
	if bytes.Contains(shown, []byte("settings written")) {
		t.Error("the number outside its range was written anyway")
	}
	if saved := savedFile(t, file); saved != original {
		t.Errorf("the file is now %q, want it untouched by the refused number", saved)
	}
	closeWindow(t, model)
}

// A number inside its range is saved the way any other row is: the window
// says the settings were written, and the file holds what was typed.
func TestANumberInsideItsRangeIsWrittenToTheFile(t *testing.T) {
	t.Parallel()
	file := filepath.Join(t.TempDir(), ".env")

	model := newWindow(t, settings.Options{
		Settings: fullSettings(file),
		Services: services,
		Chord:    acceptingChord,
	})

	stepDown(model, 12) // draft rows
	model.Send(tea.KeyMsg{Type: tea.KeyEnter})
	model.Send(tea.KeyMsg{Type: tea.KeyBackspace}) // the 6 that was standing there
	model.Send(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("10")})
	model.Send(tea.KeyMsg{Type: tea.KeyEnter})
	model.Send(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("s")})

	waitFor(t, model, "settings written")
	if saved := savedFile(t, file); !strings.Contains(saved, "TRANS_DRAFT_ROWS=10") {
		t.Errorf("the file does not hold what was written:\n%s", saved)
	}
	closeWindow(t, model)
}
