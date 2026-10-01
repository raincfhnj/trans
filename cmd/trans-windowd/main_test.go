//go:build windows

package main

import (
	"errors"
	"fmt"
	"strings"
	"testing"

	"trans/internal/config"
)

// settingsWith is three chords and nothing else: the daemon reads the rest of
// its settings for the windows it opens, not for the keys it claims.
func settingsWith(panel, selection, settings string) config.Settings {
	return config.Settings{
		Hotkey:       panel,
		SelectHotkey: selection,
		ConfigHotkey: settings,
	}
}

// speakers replaces the two ways the daemon talks with a recorder, so a test
// hears what it decided rather than reading a log file afterwards.
func speakers(t *testing.T) (said, troubles *[]string) {
	t.Helper()
	said, troubles = &[]string{}, &[]string{}

	wasNote, wasComplain := note, complain
	note = func(format string, arguments ...any) {
		*said = append(*said, fmt.Sprintf(format, arguments...))
	}
	complain = func(format string, arguments ...any) {
		*troubles = append(*troubles, fmt.Sprintf(format, arguments...))
	}
	t.Cleanup(func() { note, complain = wasNote, wasComplain })
	return said, troubles
}

// claimable is a chords set that takes everything it is given, remembering
// what it was asked for — the shape of a machine where nothing is in the way.
func claimable(taken *[]string, ids *[]uint32) chords {
	return chords{
		keys: func(spec string) (uint32, uint32, error) {
			if spec == "nonsense" {
				return 0, 0, errors.New("not a key")
			}
			return 1, 2, nil
		},
		register: func(id, _, _ uint32) error {
			*taken = append(*taken, "claim")
			*ids = append(*ids, id)
			return nil
		},
		unregister: func(id uint32) error {
			*taken = append(*taken, "release")
			*ids = append(*ids, id)
			return nil
		},
	}
}

// The three chords are claimed under the ids the waiting loop answers with, in
// the order the menu and the settings window expect them.
func TestTheThreeChordsAreClaimedUnderTheirOwnIDs(t *testing.T) {
	said, troubles := speakers(t)
	settings := settingsWith("ctrl+alt+t", "ctrl+alt+s", "ctrl+alt+c")

	taken, ids := []string{}, []uint32{}
	wanted := []window{}
	claimed := claimable(&taken, &ids).claim(&settings, &wanted)

	if claimed != 3 {
		t.Fatalf("claimed %d chords, want all three", claimed)
	}
	if len(wanted) != 3 {
		t.Fatalf("kept %d chords, want all three", len(wanted))
	}
	for index, want := range []struct {
		id    uint32
		opens string
	}{{1, "open"}, {2, "select"}, {3, "settings"}} {
		if wanted[index].id != want.id || wanted[index].opens != want.opens {
			t.Errorf("chord %d is id %d opening %q, want id %d opening %q",
				index, wanted[index].id, wanted[index].opens, want.id, want.opens)
		}
	}
	if len(*troubles) != 0 {
		t.Errorf("a clean claim complained: %v", *troubles)
	}
	if len(*said) != 3 {
		t.Errorf("said %v, want one line per claimed chord", *said)
	}
}

// A chord set to "off" is left unclaimed and said out loud, and the other two
// are claimed as usual: one setting turned off is not the daemon giving up.
func TestAChordTurnedOffIsLeftToNobody(t *testing.T) {
	said, troubles := speakers(t)
	settings := settingsWith("off", "ctrl+alt+s", "ctrl+alt+c")

	taken, ids := []string{}, []uint32{}
	wanted := []window{}
	claimed := claimable(&taken, &ids).claim(&settings, &wanted)

	if claimed != 2 {
		t.Errorf("claimed %d chords, want the two that are set", claimed)
	}
	if len(*troubles) != 0 {
		t.Errorf("turning a chord off was treated as trouble: %v", *troubles)
	}
	found := false
	for _, line := range *said {
		if strings.Contains(line, "TRANS_HOTKEY") && strings.Contains(line, "off") {
			found = true
		}
	}
	if !found {
		t.Errorf("nothing said which chord was left off: %v", *said)
	}
}

// A chord that cannot be read is reported and skipped; the chords after it are
// still claimed, because one typo in the settings is not the end of the daemon.
func TestAChordThatCannotBeReadDoesNotStopTheOthers(t *testing.T) {
	_, troubles := speakers(t)
	settings := settingsWith("nonsense", "ctrl+alt+s", "ctrl+alt+c")

	taken, ids := []string{}, []uint32{}
	wanted := []window{}
	claimed := claimable(&taken, &ids).claim(&settings, &wanted)

	if claimed != 2 {
		t.Errorf("claimed %d chords, want the readable ones", claimed)
	}
	if len(*troubles) != 1 || !strings.Contains((*troubles)[0], "nonsense") {
		t.Errorf("trouble is %v, want the unreadable chord named", *troubles)
	}
}

// A chord another program already holds is reported and left to it: the ids
// that were claimed are the ones the loop will answer for.
func TestAChordAnotherProgramHoldsIsLeftToIt(t *testing.T) {
	_, troubles := speakers(t)
	settings := settingsWith("ctrl+alt+t", "ctrl+alt+s", "ctrl+alt+c")

	taken, ids := []string{}, []uint32{}
	half := claimable(&taken, &ids)
	wasRegister := half.register
	half.register = func(id, modifiers, key uint32) error {
		if id == 2 {
			return errors.New("RegisterHotKey was refused")
		}
		return wasRegister(id, modifiers, key)
	}

	wanted := []window{}
	if claimed := half.claim(&settings, &wanted); claimed != 2 {
		t.Errorf("claimed %d chords, want the two that were free", claimed)
	}
	if len(wanted) != 2 || wanted[0].id != 1 || wanted[1].id != 3 {
		t.Errorf("kept %v, want the panel and the settings window", wanted)
	}
	if len(*troubles) != 1 || !strings.Contains((*troubles)[0], "another one running") {
		t.Errorf("trouble is %v, want the refused chord explained", *troubles)
	}
}

// Every chord turned off leaves nothing to wait for, which the caller reports
// and ends the program on.
func TestEveryChordOffClaimsNothing(t *testing.T) {
	speakers(t)
	settings := settingsWith("off", "OFF", " off ")

	taken, ids := []string{}, []uint32{}
	wanted := []window{}
	if claimed := claimable(&taken, &ids).claim(&settings, &wanted); claimed != 0 {
		t.Errorf("claimed %d chords, want none", claimed)
	}
	if len(wanted) != 0 {
		t.Errorf("kept %v, want nothing to wait for", wanted)
	}
}

// A reload gives back exactly the chords it holds — no more, no fewer — so the
// new settings can claim the same ids again.
func TestReleaseGivesBackEveryClaimedChord(t *testing.T) {
	_, troubles := speakers(t)

	taken, ids := []string{}, []uint32{}
	wanted := []window{}
	settings := settingsWith("ctrl+alt+t", "ctrl+alt+s", "ctrl+alt+c")
	releaser := claimable(&taken, &ids)
	releaser.claim(&settings, &wanted)

	taken, ids = []string{}, []uint32{}
	releaser.release(wanted)

	if len(ids) != 3 {
		t.Fatalf("gave back %v, want the three that were claimed", ids)
	}
	for index, id := range ids {
		if id != uint32(index+1) {
			t.Errorf("gave back id %d at position %d, want them in order", id, index)
		}
	}
	if len(*troubles) != 0 {
		t.Errorf("a clean release complained: %v", *troubles)
	}
}

// A chord that cannot be given back is said out loud rather than swallowed:
// the id stays taken, and the next claim of it will fail for a reason worth
// having in the log.
func TestAChordThatCannotBeGivenBackIsReported(t *testing.T) {
	_, troubles := speakers(t)

	stubborn := chords{
		keys:     func(string) (uint32, uint32, error) { return 0, 0, nil },
		register: func(uint32, uint32, uint32) error { return nil },
		unregister: func(uint32) error {
			return errors.New("UnregisterHotKey was refused")
		},
	}
	stubborn.release([]window{{id: 1, label: "the panel", chord: "ctrl+alt+t"}})

	if len(*troubles) != 1 || !strings.Contains((*troubles)[0], "the panel") {
		t.Errorf("trouble is %v, want the chord that would not go", *troubles)
	}
}

// TRANS_TRAY=0 is how the icon is left out, and the daemon says so rather than
// starting one and hiding it. The setting is read where the rest are read, so it
// arrives here as a field rather than as a second look at the environment.
func TestTheTrayCanBeTurnedOff(t *testing.T) {
	said, troubles := speakers(t)

	startTray(&config.Settings{Tray: false}, 0)

	if len(*troubles) != 0 {
		t.Errorf("turning the tray off was treated as trouble: %v", *troubles)
	}
	found := false
	for _, line := range *said {
		if strings.Contains(line, "TRANS_TRAY=0") {
			found = true
		}
	}
	if !found {
		t.Errorf("nothing said the tray was left out: %v", *said)
	}
}

// isOff is the whole of how a chord is switched down, so the values a person
// is likely to write all count as off — and no other setting does.
func TestOnlyTheWordOffTurnsAChordOff(t *testing.T) {
	t.Parallel()

	for _, off := range []string{"off", "OFF", "Off", " off ", "\toff\n"} {
		if !isOff(off) {
			t.Errorf("%q is not read as off", off)
		}
	}
	for _, on := range []string{"", "ctrl+alt+t", "offkey", "of", "off "} {
		if on == "off " {
			continue
		}
		if isOff(on) {
			t.Errorf("%q is read as off, want it left as a chord", on)
		}
	}
}
