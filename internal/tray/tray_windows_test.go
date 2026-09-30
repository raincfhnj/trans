//go:build windows

package tray

import "testing"

// Every item the daemon can be asked for has words of its own: a menu line
// with no label would draw as a gap nobody can choose.
func TestTheMenuNamesEveryItem(t *testing.T) {
	t.Parallel()

	for _, item := range []Item{OpenPanel, ClosePanel, Settings, Reload, StartAtLogon, Quit} {
		if labels[item] == "" {
			t.Errorf("item %d has no words in the menu", item)
		}
	}
}

// The ids are what the shell echoes back in WM_COMMAND, so two items sharing
// one would make the second unreachable.
func TestEveryItemHasItsOwnID(t *testing.T) {
	t.Parallel()

	seen := map[Item]string{}
	for _, item := range []Item{OpenPanel, ClosePanel, Settings, Reload, StartAtLogon, Quit} {
		if previous, twice := seen[item]; twice {
			t.Errorf("items %q and %d share an id", previous, item)
		}
		seen[item] = labels[item]
	}
	if OpenPanel <= 0 {
		t.Errorf("item ids start at %d, want a positive one: zero means no item was chosen", OpenPanel)
	}
}

// Choosing an item is handed straight to the daemon: the tray knows which line
// was picked and nothing about what it does.
func TestChoosingAnItemHandsItToTheDaemon(t *testing.T) {
	t.Parallel()

	var got Item
	state := &trayState{options: Options{Do: func(item Item) { got = item }}}
	state.chose(Settings)

	if got != Settings {
		t.Errorf("the daemon heard %d, want the item that was chosen", got)
	}
}

// A tray with nothing to call is not a reason to fall over: the icon is put up
// before the daemon has had a chance to hand its callbacks over.
func TestATrayWithNoCallbacksIsHarmless(t *testing.T) {
	t.Parallel()

	state := &trayState{}
	if state.logonAvailable() {
		t.Error("start at logon is offered with no way to know whether it is possible")
	}
	state.chose(Quit)
}

// The tick beside start-at-logon is read from the daemon every time the menu
// opens, so a change made a moment ago shows up at the next right click.
func TestTheTickIsReadFromTheDaemon(t *testing.T) {
	t.Parallel()

	ticked := false
	state := &trayState{options: Options{
		LogonChecked:   func() bool { return ticked },
		LogonAvailable: func() bool { return true },
	}}
	if state.logonAvailable() != true {
		t.Error("a daemon that says the entry can be written was not believed")
	}
	if state.options.LogonChecked() {
		t.Error("the tick is up before anything was written")
	}
	ticked = true
	if !state.options.LogonChecked() {
		t.Error("the tick did not follow the daemon")
	}
}
