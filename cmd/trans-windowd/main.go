//go:build windows

// Command trans-windowd waits for the key combinations and opens whichever
// window they name: the panel over the window in front, the translation of
// what is selected there, or the settings window. It has no window of its own
// — Windows builds it as a program without a console, so it can sit in the
// background from a logon entry and do nothing until a chord is pressed.
//
// The combinations are TRANS_HOTKEY, TRANS_SELECT_HOTKEY and
// TRANS_CONFIG_HOTKEY, each set to `off` to leave it unclaimed. They are
// claimed when the program starts and claimed again whenever the tray's
// "Reload settings" is chosen, so a chord changed in the settings window takes
// effect without a restart. Everything else the windows read for themselves
// from the environment and the .env file, so any other setting changed there
// is picked up by the next popup.
//
// TRANS_TRAY=0 starts no tray icon: the chords work exactly as before, and the
// only way to end the program is to end the process.
package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"trans/internal/config"
	"trans/internal/tray"
	"trans/internal/win32"
	"trans/internal/winlog"
)

// window names one chord of the daemon and the window it opens. The ids are
// the ones the waiting loop answers with.
type window struct {
	id       uint32
	variable string
	chord    string
	opens    string
	label    string
}

func main() {
	settings, ok := load()
	if !ok {
		return
	}

	// The chords belong to this thread, so the tray — which is on a thread of
	// its own — can only ask this one to read them again or to end. Posting is
	// how it asks.
	here := win32.CurrentThreadID()
	startTray(&settings, here)

	wanted := []window{}
	claimed := windows.claim(&settings, &wanted)
	if claimed == 0 {
		complain("no chord was claimed, so there is nothing to wait for")
		tray.Stop()
		return
	}

	for {
		command, pressed := win32.WaitForCommand()
		switch command {
		case win32.CommandQuit:
			// The window closed, or the tray asked for the end.
			tray.Stop()
			return

		case win32.CommandReload:
			// Reading the settings is this thread's, and so is giving the old
			// chords back: RegisterHotKey answers the thread that claimed them.
			note("reading the settings again and claiming the chords anew")
			fresh, ok := load()
			if !ok {
				continue
			}
			windows.release(wanted)
			wanted = wanted[:0]
			if windows.claim(&fresh, &wanted) == 0 {
				complain("reload claimed no chord at all; the old ones are gone")
			}

		case win32.CommandHotkey:
			for _, one := range wanted {
				if one.id != pressed {
					continue
				}
				// The panel program knows where each window belongs: the
				// window in front at the moment the chord was pressed.
				if err := open(one.opens); err != nil {
					complain("%s did not open: %v", one.label, err)
				}
			}
		}
	}
}

// load reads the settings and moves a plaintext key into the protected store
// if there is one to move. The settings are what the chords come from, and
// what the windows the daemon opens will read for themselves.
func load() (config.Settings, bool) {
	config.Prepare()
	settings, err := config.Load(os.Getenv)
	if err != nil {
		complain("settings: %v", err)
		return config.Settings{}, false
	}
	if note := config.UpgradeSecrets(&settings); note != "" {
		complain("%s", note)
	}
	if note := config.ResolveKey(&settings); note != "" {
		complain("%s", note)
	}
	return settings, true
}

// chords is what claiming a chord needs from the system: the parser, the claim
// and the release. The daemon runs on the real one; a test hands in its own,
// which is how the decisions around the chords — which one is left off, which
// one another program already holds, what a reload gives back — are checked
// without a desktop to claim keys on.
type chords struct {
	keys       func(spec string) (modifiers, key uint32, err error)
	register   func(id, modifiers, key uint32) error
	unregister func(id uint32) error
}

// windows is the real set: the Win32 calls the daemon runs on.
var windows = chords{
	keys:       win32.Keys,
	register:   win32.RegisterHotkey,
	unregister: win32.UnregisterHotkey,
}

// claim asks Windows for each chord the settings name, answering how many were
// taken and filling wanted with the ones that were. A chord another program of
// the author's already holds is reported and left to it: the other two still
// work, and a chord that could not be claimed is not worth ending the program
// over.
func (c chords) claim(settings *config.Settings, wanted *[]window) int {
	all := []window{
		{id: 1, variable: config.HotkeyVar, chord: settings.Hotkey, opens: "open", label: "the panel"},
		{id: 2, variable: config.SelectHotkeyVar, chord: settings.SelectHotkey, opens: "select", label: "the selection"},
		{id: 3, variable: config.ConfigHotkeyVar, chord: settings.ConfigHotkey, opens: "settings", label: "the settings window"},
	}

	claimed := 0
	for _, one := range all {
		if isOff(one.chord) {
			note("%s will not be claimed: %s is off", one.label, one.variable)
			continue
		}
		modifiers, key, err := c.keys(one.chord)
		if err != nil {
			complain("the chord for %s (%s=%q) cannot be used: %v",
				one.label, one.variable, one.chord, err)
			continue
		}
		if err := c.register(one.id, modifiers, key); err != nil {
			complain("the chord for %s (%s=%q) could not be claimed "+
				"(is another one running?): %v", one.label, one.variable, one.chord, err)
			continue
		}
		claimed++
		*wanted = append(*wanted, one)
		note("waiting for %s on %s", one.label, one.chord)
	}
	return claimed
}

// release gives every claimed chord back, so the reload can claim the new ones
// under the same ids: RegisterHotKey refuses an id that is already taken on
// this thread.
func (c chords) release(wanted []window) {
	for _, one := range wanted {
		if err := c.unregister(one.id); err != nil {
			complain("the chord for %s (%s) could not be given back: %v",
				one.label, one.chord, err)
		}
	}
}

// startTray puts the icon up unless the settings say not to. The setting is read
// where every other setting is read — `TRANS_TRAY=0`, in the environment or in
// the file — so turning the icon off needs no second way of saying so. A tray
// that cannot be drawn is not a reason for the chords to stop working: what it
// has to say goes to the log and the daemon carries on without it.
func startTray(settings *config.Settings, here uint32) {
	if !settings.Tray {
		note("no tray icon: TRANS_TRAY=0")
		return
	}

	options := tray.Options{
		Tip:            "trans — the panel is a chord away",
		LogonChecked:   logonEnabled,
		LogonAvailable: logonAvailable,
		Do: func(item tray.Item) {
			switch item {
			case tray.OpenPanel:
				if err := open("open"); err != nil {
					complain("the panel did not open: %v", err)
				}
			case tray.ClosePanel:
				// Every window asked to close is written down with the handle
				// and title it carried, so a close that reached something else
				// can be read back afterwards instead of guessed at from a
				// count. The count is still the last word on the line.
				closed := win32.ClosePanels()
				for _, window := range closed {
					note("asking %#x %q to close", window.Handle, window.Title)
				}
				note("closed %d panel window(s)", len(closed))
			case tray.Settings:
				if err := open("settings"); err != nil {
					complain("the settings window did not open: %v", err)
				}
			case tray.Reload:
				// Not this thread's to do: the chords belong to the daemon's
				// own, which is waiting for exactly this request.
				if err := win32.PostReload(here); err != nil {
					complain("the settings could not be read again: %v", err)
				}
			case tray.StartAtLogon:
				toggleLogon()
			case tray.Quit:
				if err := win32.PostQuit(here); err != nil {
					complain("the program could not be ended: %v", err)
				}
			}
		},
	}

	go func() {
		if err := tray.Run(options); err != nil {
			complain("no tray icon: %v", err)
		}
	}()
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
// worth keeping anyway, so it goes next to the drafts. The two ways of saying
// it are variables rather than functions so that a test can hear them: what
// the daemon decides about a chord is exactly what it says about one.
var (
	complain = func(format string, arguments ...any) {
		winlog.Note("windowd", "trouble: "+format, arguments...)
	}
	note = func(format string, arguments ...any) {
		winlog.Note("windowd", format, arguments...)
	}
)
