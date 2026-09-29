//go:build windows

// Command trans-windowd waits for the key combinations and opens whichever
// window they name: the panel over the window in front, the translation of
// what is selected there, or the settings window. It has no window of its own
// — Windows builds it as a program without a console, so it can sit in the
// background from a logon entry and do nothing until a chord is pressed.
//
// The combinations are TRANS_HOTKEY, TRANS_SELECT_HOTKEY and
// TRANS_CONFIG_HOTKEY, each set to `off` to leave it unclaimed, and they are
// claimed once at the start: a chord changed in the settings window takes
// effect at the next start of this program, which the settings window says.
// Everything else the windows read for themselves from the environment and the
// .env file, so any other setting changed there is picked up by the next
// popup.
package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"trans/internal/config"
	"trans/internal/win32"
	"trans/internal/winlog"
)

// window names one chord of the daemon and the window it opens. The ids are
// the ones WaitForHotkey answers with.
type window struct {
	id       uint32
	variable string
	chord    string
	opens    string
	label    string
}

func main() {
	config.Prepare()
	settings, err := config.Load(os.Getenv)
	if err != nil {
		complain("settings: %v", err)
		return
	}

	wanted := []window{
		{id: 1, variable: config.HotkeyVar, chord: settings.Hotkey, opens: "open", label: "the panel"},
		{id: 2, variable: config.SelectHotkeyVar, chord: settings.SelectHotkey, opens: "select", label: "the selection"},
		{id: 3, variable: config.ConfigHotkeyVar, chord: settings.ConfigHotkey, opens: "settings", label: "the settings window"},
	}

	claimed := 0
	for _, one := range wanted {
		switch {
		case isOff(one.chord):
			note("%s will not be claimed: %s is off", one.label, one.variable)
		default:
			modifiers, key, err := win32.Keys(one.chord)
			if err != nil {
				complain("the chord for %s (%s=%q) cannot be used: %v",
					one.label, one.variable, one.chord, err)
				continue
			}
			if err := win32.RegisterHotkey(one.id, modifiers, key); err != nil {
				complain("the chord for %s (%s=%q) could not be claimed "+
					"(is another one running?): %v", one.label, one.variable, one.chord, err)
				continue
			}
			claimed++
			note("waiting for %s on %s", one.label, one.chord)
		}
	}
	if claimed == 0 {
		complain("no chord was claimed, so there is nothing to wait for")
		return
	}

	for {
		pressed := win32.WaitForHotkey()
		if pressed == 0 {
			// The program is ending, not the chord pressed.
			return
		}
		for _, one := range wanted {
			if one.id != pressed {
				continue
			}
			// The panel program knows where each window belongs: the window in
			// front at the moment the chord was pressed.
			if err := open(one.opens); err != nil {
				complain("%s did not open: %v", one.label, err)
			}
		}
	}
}

// isOff is how a chord is switched down: "off" travels through the settings
// file like any other value, and leaving it unclaimed is this program's part
// of honouring it.
func isOff(chord string) bool {
	return strings.EqualFold(strings.TrimSpace(chord), "off")
}

// open hands the work to the panel program under the subcommand for the window
// that was asked for.
func open(subcommand string) error {
	program, err := os.Executable()
	if err != nil {
		return err
	}
	panel := filepath.Join(filepath.Dir(program), "trans-window.exe")
	if _, err := os.Stat(panel); err != nil {
		return fmt.Errorf("%s is not next to this program", filepath.Base(panel))
	}
	return win32.SpawnQuietly(panel, []string{subcommand}, os.Environ())
}

// A program with no console has nowhere to say something; what it has to say is
// worth keeping anyway, so it goes next to the drafts.
func complain(format string, arguments ...any) {
	winlog.Note("windowd", "trouble: "+format, arguments...)
}

func note(format string, arguments ...any) {
	winlog.Note("windowd", format, arguments...)
}
