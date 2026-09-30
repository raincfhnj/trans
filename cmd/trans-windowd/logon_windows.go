//go:build windows

package main

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"golang.org/x/sys/windows/registry"
)

// runKey is where Windows looks for the programs to start when a person logs
// on. Writing there is how the tray's one marked item does what it says; it is
// the user's own key, so no administrator is needed and removing the entry is
// the same call.
const runKey = `Software\Microsoft\Windows\CurrentVersion\Run`

// valueName is the name the entry is filed under. It is also what the tray
// reads back to know whether to show the tick.
const valueName = "trans-windowd"

// logonCommand is what the entry runs: this program, by the path it was
// started from, quoted because a path may hold spaces.
func logonCommand() (string, error) {
	program, err := os.Executable()
	if err != nil {
		return "", err
	}
	return `"` + program + `"`, nil
}

// logonAvailable says whether this copy of the program has a path worth
// writing down. A daemon started out of a build directory or a temporary
// folder is gone the next time Windows looks: an entry pointing at a file that
// is not there is worse than no entry, so the item is left out instead.
func logonAvailable() bool {
	program, err := os.Executable()
	if err != nil {
		return false
	}
	if _, err := os.Stat(program); err != nil {
		return false
	}
	// A go-build cache or temp path is where `go run` puts the program; the
	// two markers are enough to recognize one without guessing at layouts.
	lower := strings.ToLower(program)
	return !strings.Contains(lower, `\go-build`) &&
		!strings.Contains(lower, `\temp\`)
}

// logonEnabled is whether the entry is there and points at this program.
func logonEnabled() bool {
	key, err := registry.OpenKey(registry.CURRENT_USER, runKey, registry.QUERY_VALUE)
	if err != nil {
		return false
	}
	defer key.Close()

	written, _, err := key.GetStringValue(valueName)
	if err != nil {
		return false
	}
	wanted, err := logonCommand()
	if err != nil {
		return false
	}
	return strings.EqualFold(strings.TrimSpace(written), wanted)
}

// setLogon writes or removes the entry, answering the state it left behind.
// A missing entry is not an error when removing it: the key is already the way
// the caller wanted it.
func setLogon(start bool) (bool, error) {
	if !start {
		key, err := registry.OpenKey(registry.CURRENT_USER, runKey, registry.SET_VALUE)
		if err != nil {
			if errors.Is(err, os.ErrNotExist) {
				return false, nil
			}
			return false, err
		}
		defer key.Close()
		if err := key.DeleteValue(valueName); err != nil && !errors.Is(err, os.ErrNotExist) {
			return false, err
		}
		return false, nil
	}

	command, err := logonCommand()
	if err != nil {
		return false, err
	}
	key, _, err := registry.CreateKey(registry.CURRENT_USER, runKey, registry.SET_VALUE)
	if err != nil {
		return false, fmt.Errorf("opening %s: %w", runKey, err)
	}
	defer key.Close()
	if err := key.SetStringValue(valueName, command); err != nil {
		return false, err
	}
	return true, nil
}

// toggleLogon flips the entry and says where it ended up, which is what the
// tray's tick should show afterwards.
func toggleLogon() bool {
	next := !logonEnabled()
	state, err := setLogon(next)
	if err != nil {
		complain("start at logon could not be changed: %v", err)
		return logonEnabled()
	}
	if state {
		note("this program will start at logon: %s", filepath.Base(valueName))
	} else {
		note("this program will no longer start at logon")
	}
	return state
}
