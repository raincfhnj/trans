//go:build !windows

package tray

import (
	"errors"
	"testing"
)

// Everywhere but Windows there is no notification area to put an icon on, and
// the daemon hears about it instead of getting a tray that is not there.
func TestRunSaysThereIsNoTrayHere(t *testing.T) {
	t.Parallel()

	if err := Run(Options{}); !errors.Is(err, ErrUnavailable) {
		t.Errorf("Run answered %v, want the refusal of a system without a tray", err)
	}
}

// The item ids are the same numbers on every system, so the daemon's own list
// of them compiles and means the same thing wherever the tree is built.
func TestTheItemIDsMatchTheOnesWindowsUses(t *testing.T) {
	t.Parallel()

	for index, item := range []Item{OpenPanel, ClosePanel, Settings, Reload, StartAtLogon, Quit} {
		if int(item) != index+1 {
			t.Errorf("item %d is %d, want the number Windows gives it", index+1, item)
		}
	}
}
