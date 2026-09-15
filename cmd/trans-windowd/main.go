//go:build windows

// Command trans-windowd waits for a key combination and opens the panel over
// the window the author is working in. It has no window of its own — Windows
// builds it as a program without a console, so it can sit in the background
// from a logon entry and do nothing until the hotkey is pressed.
//
// The combination is HERDR_TRANS_HOTKEY, `ctrl+alt+t` unless it says
// otherwise. Everything else the panel reads for itself, from the environment
// and from the plugin's .env, so a setting changed there is picked up by the
// next popup rather than needing this program restarted.
package main

import (
	"fmt"
	"os"
	"path/filepath"

	"trans/internal/win32"
	"trans/internal/winlog"
)

func main() {

	spec := os.Getenv("HERDR_TRANS_HOTKEY")
	if spec == "" {
		spec = "ctrl+alt+t"
	}

	modifiers, key, err := win32.Keys(spec)
	if err != nil {
		complain("the hotkey %q cannot be used: %v", spec, err)
		return
	}
	if err := win32.RegisterHotkey(modifiers, key); err != nil {
		complain("the hotkey %q could not be claimed (is another one running?): %v", spec, err)
		return
	}
	note("waiting for %s", spec)

	for win32.WaitForHotkey() {
		if err := openPanel(); err != nil {
			complain("the panel did not open: %v", err)
		}
	}
}

// openPanel hands the work to the panel program, which knows where it belongs:
// the window in front at the moment the hotkey was pressed.
func openPanel() error {
	program, err := os.Executable()
	if err != nil {
		return err
	}
	panel := filepath.Join(filepath.Dir(program), "trans-window.exe")
	if _, err := os.Stat(panel); err != nil {
		return fmt.Errorf("%s is not next to this program", filepath.Base(panel))
	}
	return win32.SpawnQuietly(panel, []string{"open"}, os.Environ())
}

// A program with no console has nowhere to say something; what it has to say is
// worth keeping anyway, so it goes next to the drafts.
func complain(format string, arguments ...any) {
	winlog.Note("windowd", "trouble: "+format, arguments...)
}

func note(format string, arguments ...any) {
	winlog.Note("windowd", format, arguments...)
}
