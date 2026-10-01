//go:build windows

package win32

import (
	"slices"
	"testing"
)

// TestHeldKeysChecksEveryModifier pins the keys a chord holds down, so no
// modifier can be quietly dropped on the way to the key press. A dropped
// modifier is not a smaller chord: win+v sent as a bare v is a literal letter
// typed into the target where a paste was meant.
//
// This only exercises the pure mapping; no key event is sent.
func TestHeldKeysChecksEveryModifier(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name       string
		modifiers  uint32
		wantKeySet []uintptr
	}{
		{name: "no modifier", modifiers: 0, wantKeySet: []uintptr{}},
		{name: "ctrl", modifiers: modControl, wantKeySet: []uintptr{vkControl}},
		{name: "shift", modifiers: modShift, wantKeySet: []uintptr{vkShift}},
		{name: "alt", modifiers: modAlt, wantKeySet: []uintptr{vkMenu}},
		{name: "win", modifiers: modWin, wantKeySet: []uintptr{vkLWin}},
		{
			name:       "ctrl+shift",
			modifiers:  modControl | modShift,
			wantKeySet: []uintptr{vkControl, vkShift},
		},
		{
			name:       "ctrl+alt+shift",
			modifiers:  modControl | modShift | modAlt,
			wantKeySet: []uintptr{vkControl, vkShift, vkMenu},
		},
		{
			name:       "all four, held in the order they are released in reverse",
			modifiers:  modControl | modShift | modAlt | modWin,
			wantKeySet: []uintptr{vkControl, vkShift, vkMenu, vkLWin},
		},
	}

	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			held := heldKeys(test.modifiers)
			if !slices.Equal(held, test.wantKeySet) {
				t.Errorf("heldKeys(%#x) = %v, want %v", test.modifiers, held, test.wantKeySet)
			}
		})
	}
}

// TestAWinChordHoldsTheWinKey checks the whole path for the chord that was
// broken: Keys accepts win+v, and the keys held for it must include the Windows
// key rather than nothing.
//
// This only reads the mapping; no key event is sent.
func TestAWinChordHoldsTheWinKey(t *testing.T) {
	t.Parallel()

	modifiers, key, err := Keys("win+v")
	if err != nil {
		t.Fatalf("Keys: %v", err)
	}
	if modifiers != modWin {
		t.Errorf("Keys read %#x as the modifiers, want %#x", modifiers, modWin)
	}
	if uintptr(key) != vkV {
		t.Errorf("Keys read %#x as the key, want %#x", key, vkV)
	}
	held := heldKeys(modifiers)
	if !slices.Equal(held, []uintptr{vkLWin}) {
		t.Errorf("heldKeys for win+v = %v, want [%#x]", held, vkLWin)
	}
}

// The super spelling is the same modifier as win, so it holds the same key.
func TestASuperChordHoldsTheWinKeyToo(t *testing.T) {
	t.Parallel()

	modifiers, _, err := Keys("super+v")
	if err != nil {
		t.Fatalf("Keys: %v", err)
	}
	held := heldKeys(modifiers)
	if !slices.Equal(held, []uintptr{vkLWin}) {
		t.Errorf("heldKeys for super+v = %v, want [%#x]", held, vkLWin)
	}
}

// F12 is reserved by Windows and pressing it as a chord is pressing it into
// whatever has the keyboard, which no setting may ask for. The answer says that
// rather than "not a key", which is what a person would otherwise be left to
// guess at.
func TestF12IsRefusedAsAChord(t *testing.T) {
	t.Parallel()

	if _, _, err := Keys("f12"); err == nil {
		t.Fatal("Keys accepted f12, want it refused as a key Windows keeps for itself")
	}
	if _, _, err := Keys("ctrl+f12"); err == nil {
		t.Fatal("Keys accepted ctrl+f12, want it refused as a key Windows keeps for itself")
	}
}
