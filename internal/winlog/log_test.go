// Package winlog keeps a note of what a program on Windows has to say when
// there is nowhere to say it. A note is a courtesy, not a contract: nothing
// depends on one being written, and nothing may fail because one could not be.
package winlog

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// cacheDirectory points the log at a directory of the test's own, so that a note
// written by a test is a file the test can read and take away again. The log
// itself reads the directory the way any other program does, which is the thing
// worth checking.
func cacheDirectory(t *testing.T) string {
	t.Helper()

	directory := t.TempDir()
	// os.UserCacheDir answers from the environment, and which variable it reads
	// is the platform's business: both are set so that a note lands in the same
	// place wherever this runs. The log appends "trans" to whatever it is given.
	t.Setenv("XDG_CACHE_HOME", directory)
	t.Setenv("LocalAppData", directory)
	return directory
}

// A note is written where the rest of the program's files are, and what it
// wrote can be read back: the name, the line, and the time it happened.
func TestANoteIsWrittenWhereItCanBeFoundAgain(t *testing.T) {
	directory := cacheDirectory(t)

	Note("trans-window", "the clipboard holds %s, which cannot be put back", "a picture")

	path := filepath.Join(directory, "trans", "trans-window.log")
	written, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("reading the log at %s: %v", path, err)
	}

	line := strings.TrimSpace(string(written))
	if !strings.Contains(line, "the clipboard holds a picture, which cannot be put back") {
		t.Errorf("the log reads %q, want the note as it was written", line)
	}
	// RFC3339 starts with a four-digit year, which is what a person reading the
	// log afterwards has to sort it by.
	if len(line) < len("2026-01-02T15:04:05Z07:00") || line[4] != '-' || line[7] != '-' {
		t.Errorf("the log reads %q, want a timestamp in front of the note", line)
	}
}

// A second note is appended rather than replacing the first: a log that kept
// only the last line would be no use for the thing that happened before it.
func TestASecondNoteIsAppendedToTheFirst(t *testing.T) {
	directory := cacheDirectory(t)

	Note("trans-window", "the first thing")
	Note("trans-window", "the second thing")

	written, err := os.ReadFile(filepath.Join(directory, "trans", "trans-window.log"))
	if err != nil {
		t.Fatalf("reading the log: %v", err)
	}
	lines := strings.Split(strings.TrimSpace(string(written)), "\n")
	if len(lines) != 2 {
		t.Fatalf("the log has %d lines, want both notes: %q", len(lines), written)
	}
	if !strings.Contains(lines[0], "the first thing") || !strings.Contains(lines[1], "the second thing") {
		t.Errorf("the log reads %q, want the notes in the order they were written", written)
	}
}

// Each program's notes are its own file, so that two programs writing at once
// cannot lose each other's lines.
func TestEachProgramWritesItsOwnFile(t *testing.T) {
	directory := cacheDirectory(t)

	Note("trans-window", "from the panel")
	Note("trans-windowd", "from the daemon")

	wanted := map[string]string{
		"trans-window":  "from the panel",
		"trans-windowd": "from the daemon",
	}
	others := map[string]string{
		"trans-window":  "from the daemon",
		"trans-windowd": "from the panel",
	}
	for program, note := range wanted {
		written, err := os.ReadFile(filepath.Join(directory, "trans", program+".log"))
		if err != nil {
			t.Fatalf("reading %s's log: %v", program, err)
		}
		got := strings.TrimSpace(string(written))
		if !strings.Contains(got, note) {
			t.Errorf("%s's log reads %q, want its own note %q", program, got, note)
		}
		if strings.Contains(got, others[program]) {
			t.Errorf("%s's log reads %q, want the other program's note kept out of it", program, got)
		}
	}
}

// A log that cannot be written is not worth failing over, and it is certainly
// not worth taking the program down with it: whatever asked for the note had
// something more important to do, and it must be able to carry on.
func TestANoteThatCannotBeWrittenIsNotFatal(t *testing.T) {
	directory := t.TempDir()

	// The cache directory is a file rather than a directory, so there is
	// nowhere under it for the log to go.
	blocked := filepath.Join(directory, "not-a-directory")
	if err := os.WriteFile(blocked, []byte("in the way"), 0o600); err != nil {
		t.Fatalf("preparing the blocked path: %v", err)
	}
	t.Setenv("XDG_CACHE_HOME", blocked)
	t.Setenv("LocalAppData", blocked)

	defer func() {
		if recovered := recover(); recovered != nil {
			t.Fatalf("Note panicked on a log it could not write: %v", recovered)
		}
	}()
	Note("trans-window", "this has nowhere to go")
}

// A note is formatted the way fmt formats one, and a caller that passes no
// arguments still gets its line: the log is not a place for a stray percent.
func TestANoteWithNoArgumentsIsWrittenAsItStands(t *testing.T) {
	directory := cacheDirectory(t)

	Note("trans-window", "100%% of the prompt arrived")

	written, err := os.ReadFile(filepath.Join(directory, "trans", "trans-window.log"))
	if err != nil {
		t.Fatalf("reading the log: %v", err)
	}
	if got := strings.TrimSpace(string(written)); !strings.Contains(got, "100% of the prompt arrived") {
		t.Errorf("the log reads %q, want the note formatted", got)
	}
}
