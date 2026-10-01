//go:build windows

package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"trans/internal/config"
)

// The name a window is kept under between sessions: its title, because a
// handle is different every time a terminal starts, and the target it was
// opened for when the title is nothing.
func TestAWindowIsNamedByItsTitleAndFallsBackToTheTarget(t *testing.T) {
	t.Parallel()

	settings := &config.Settings{Target: "0x1a2b3c"}
	cases := []struct {
		name  string
		title string
		want  string
	}{
		{name: "the title wins", title: "notes.md — pane", want: "notes.md — pane"},
		{name: "a blank title falls back", title: "", want: "0x1a2b3c"},
		{name: "so does a title of spaces", title: "   ", want: "0x1a2b3c"},
	}

	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			if key := windowKey(settings, test.title); key != test.want {
				t.Errorf("the window is named %q, want %q", key, test.want)
			}
		})
	}
}

// Nothing is kept when the settings say not to keep it, and nothing is kept
// when there is nowhere to keep it: a store with no directory would write into
// whatever directory the panel happened to start in.
func TestDraftsAreKeptOnlyWhenThereIsSomewhereToKeepThem(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name     string
		settings *config.Settings
		kept     bool
	}{
		{name: "kept when asked for", settings: &config.Settings{KeepDraft: true, History: true, StateDir: "state"}, kept: true},
		{name: "turned off", settings: &config.Settings{KeepDraft: false, History: false, StateDir: "state"}, kept: false},
		{name: "nowhere to keep them", settings: &config.Settings{KeepDraft: true, History: true}, kept: false},
	}

	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			if got := drafts(test.settings, "a pane") != nil; got != test.kept {
				t.Errorf("drafts kept: %v, want %v", got, test.kept)
			}
			if got := historyLog(test.settings, "a pane") != nil; got != test.kept {
				t.Errorf("history kept: %v, want %v", got, test.kept)
			}
		})
	}
}

// The record can be turned off on its own, with the drafts left on: they are
// two different things to keep.
func TestTheRecordCanBeTurnedOffOnItsOwn(t *testing.T) {
	t.Parallel()

	settings := &config.Settings{KeepDraft: true, History: false, StateDir: "state"}
	if drafts(settings, "a pane") == nil {
		t.Error("the draft was dropped with the record")
	}
	if historyLog(settings, "a pane") != nil {
		t.Error("the record was kept after being turned off")
	}
}

// What the command line takes, and what it refuses: a wizard that read an
// argument it does not know would report on a machine the caller did not ask
// about.
func TestSetupTakesItsOwnArgumentsAndRefusesTheRest(t *testing.T) {
	t.Setenv("TRANS_CONFIG_DIR", t.TempDir())
	t.Setenv("TRANS_STATE_DIR", t.TempDir())

	accepted := [][]string{nil, {"doctor"}, {"--json"}, {"doctor", "--json"}, {"-h"}, {"--help"}}
	for _, arguments := range accepted {
		if err := runSetup(arguments); err != nil {
			t.Errorf("setup %v failed with %v, want it accepted", arguments, err)
		}
	}

	for _, arguments := range [][]string{{"--teleport"}, {"doctor", "--now"}, {"-j"}} {
		err := runSetup(arguments)
		if err == nil {
			t.Errorf("setup %v was accepted, want it refused", arguments)
			continue
		}
		if !strings.Contains(err.Error(), "unknown argument") {
			t.Errorf("setup %v failed with %v, want the argument named", arguments, err)
		}
	}
}

// The doctor reads the settings the panel would read, from the directory the
// environment names: the same file, so what it reports is what the panel does.
func TestSetupReadsTheSettingsThePanelWouldRead(t *testing.T) {
	directory := t.TempDir()
	t.Setenv("TRANS_CONFIG_DIR", directory)
	t.Setenv("TRANS_STATE_DIR", t.TempDir())

	if err := runSetup([]string{"doctor", "--json"}); err != nil {
		t.Fatalf("setup doctor failed with %v", err)
	}

	if _, err := os.Stat(filepath.Join(directory, ".env")); err != nil {
		t.Errorf("the settings file was not written where the doctor said it reads: %v", err)
	}
}
