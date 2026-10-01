package atomicfile_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"trans/internal/atomicfile"
)

// The write that cannot finish is the one worth a test: the promise this
// package makes is that a reader finds either the old file or the new one, and
// a failure is where that promise is kept or broken. A directory that already
// holds something cannot be replaced by a file, so the move fails — on Windows
// and on Linux alike, and for a reason that does not depend on permissions.
func TestAWriteThatCannotReplaceItsTargetLeavesWhatWasThere(t *testing.T) {
	t.Parallel()

	directory := t.TempDir()
	target := filepath.Join(directory, "story.txt")
	if err := os.MkdirAll(target, 0o700); err != nil {
		t.Fatalf("setting up: %v", err)
	}
	inside := filepath.Join(target, "chapter-one.txt")
	if err := os.WriteFile(inside, []byte("the first chapter"), 0o600); err != nil {
		t.Fatalf("setting up: %v", err)
	}

	err := atomicfile.Write(target, []byte("a story"))
	if err == nil {
		t.Fatal("replacing a directory with a file succeeded, want a failure")
	}
	if !strings.Contains(err.Error(), target) {
		t.Errorf("the failure reads %q, want the path in it", err)
	}

	// What was there is still there, whole: the point of writing beside the
	// destination is that the destination is never the thing being written.
	kept, err := os.ReadFile(inside)
	if err != nil {
		t.Fatalf("reading what was there: %v", err)
	}
	if string(kept) != "the first chapter" {
		t.Errorf("the file inside holds %q, want it untouched", kept)
	}
}

// A write that failed half way through leaves no half-made file next to the one
// it was replacing: the next write would find it there, and so would a person
// looking in the state directory.
func TestAFailedWriteLeavesNoHalfMadeFile(t *testing.T) {
	t.Parallel()

	directory := t.TempDir()
	target := filepath.Join(directory, "story.txt")
	if err := os.MkdirAll(target, 0o700); err != nil {
		t.Fatalf("setting up: %v", err)
	}

	if err := atomicfile.Write(target, []byte("a story")); err == nil {
		t.Fatal("replacing a directory with a file succeeded, want a failure")
	}

	entries, err := os.ReadDir(directory)
	if err != nil {
		t.Fatalf("reading the directory: %v", err)
	}
	if len(entries) != 1 || entries[0].Name() != "story.txt" {
		t.Errorf("the directory holds %v, want the destination alone", names(entries))
	}
}
