package win32_test

import (
	"testing"

	"trans/internal/win32"
)

var windows = []win32.Window{
	{Handle: 0x1, Title: "C:\\WINDOWS\\system32\\cmd.exe"},
	{Handle: 0x2, Title: "C:\\dev\\my project — opencode"},
	{Handle: 0x3, Title: "opencode — claude code (workspace)"},
}

func TestFindTakesTheFirstWindowWhoseTitleMatches(t *testing.T) {
	t.Parallel()

	found, ok := win32.Find(windows, "opencode")
	if !ok {
		t.Fatal("a window that is there was not found")
	}
	if found.Handle != 0x2 {
		t.Errorf("Find picked %#x, want the first window whose title matches", found.Handle)
	}
}

func TestFindIgnoresCase(t *testing.T) {
	t.Parallel()

	if found, ok := win32.Find(windows, "OpenCode"); !ok || found.Handle != 0x2 {
		t.Errorf("Find(%q) picked %#x, want case to make no difference", "OpenCode", found.Handle)
	}
}

// A title is full of brackets and colons that mean something in a regular
// expression, and the author typing one means the window, not the pattern.
func TestATitleWithBracketsInItIsFoundAsTheTextItIs(t *testing.T) {
	t.Parallel()

	title := "C:\\dev\\my [project] (main) — opencode"
	present := []win32.Window{{Handle: 0x9, Title: title}}

	found, ok := win32.Find(present, "my [project] (main)")
	if !ok {
		t.Fatalf("a title with brackets in it was not found")
	}
	if found.Handle != 0x9 {
		t.Errorf("Find picked %#x, want the window the text names", found.Handle)
	}
}

// A setting that names no window as it stands is read as a pattern, which is
// what naming several windows at once looks like.
func TestASettingThatIsNoTitleIsReadAsAPattern(t *testing.T) {
	t.Parallel()

	found, ok := win32.Find(windows, "^C:\\\\WINDOWS.*cmd\\.exe$")
	if !ok {
		t.Fatal("a pattern matching a window was not used")
	}
	if found.Handle != 0x1 {
		t.Errorf("Find picked %#x, want the window the pattern matches", found.Handle)
	}
}

func TestFindSaysWhenNothingMatches(t *testing.T) {
	t.Parallel()

	if window, ok := win32.Find(windows, "notepad"); ok {
		t.Errorf("Find answered %#x for a window that is not there", window.Handle)
	}
}

func TestATargetSettingThatLooksLikeAHandleIsOne(t *testing.T) {
	t.Parallel()

	target := win32.ParseTarget("0x2a1b")
	if target.Handle != 0x2a1b {
		t.Errorf("ParseTarget read %#x, want the handle as written", target.Handle)
	}
	if target.Pattern != "" {
		t.Errorf("ParseTarget also kept the pattern %q", target.Pattern)
	}
}

func TestATargetSettingThatDoesNotLookLikeAHandleIsATitle(t *testing.T) {
	t.Parallel()

	target := win32.ParseTarget("Windows Terminal")
	if target.Handle != 0 {
		t.Errorf("ParseTarget read %#x out of a title", target.Handle)
	}
	if target.Pattern != "Windows Terminal" {
		t.Errorf("ParseTarget kept %q, want the title", target.Pattern)
	}
}

// A title with a character in it nobody types — the zero-width space a window
// manager puts between words — is still found by the words that are there.
func TestATitleWithAnInvisibleCharacterInItIsStillFound(t *testing.T) {
	t.Parallel()

	present := []win32.Window{{Handle: 0x7, Title: "Microsoft\u200b Edge"}}

	found, ok := win32.Find(present, "Microsoft Edge")
	if !ok {
		t.Fatal("a title with a zero-width space in it was not found")
	}
	if found.Handle != 0x7 {
		t.Errorf("Find picked %#x, want the window the text names", found.Handle)
	}
}
