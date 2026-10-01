package atomicfile_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"trans/internal/atomicfile"
)

// The point of writing this way: whatever is read is one whole version.
func TestWritingReplacesTheWholeFile(t *testing.T) {
	t.Parallel()

	path := filepath.Join(t.TempDir(), "story.txt")
	if err := atomicfile.Write(path, []byte("first")); err != nil {
		t.Fatalf("writing: %v", err)
	}
	if err := atomicfile.Write(path, []byte("second")); err != nil {
		t.Fatalf("writing again: %v", err)
	}

	kept, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("reading: %v", err)
	}
	if string(kept) != "second" {
		t.Errorf("the file holds %q, want %q", kept, "second")
	}
}

// A directory that is not there yet is made, with the writing in it: a fresh
// installation has no state directory, and failing to create one would mean
// nothing is ever kept.
func TestWritingMakesTheDirectoryItNeeds(t *testing.T) {
	t.Parallel()

	path := filepath.Join(t.TempDir(), "state", "deeper", "story.txt")
	if err := atomicfile.Write(path, []byte("kept")); err != nil {
		t.Fatalf("writing into a directory that is not there: %v", err)
	}

	kept, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("reading: %v", err)
	}
	if string(kept) != "kept" {
		t.Errorf("the file holds %q, want %q", kept, "kept")
	}
}

// A half-made file is not left behind for anyone to find, whichever way the
// write ended.
func TestNoLeftoverFileIsKept(t *testing.T) {
	t.Parallel()

	directory := t.TempDir()
	path := filepath.Join(directory, "story.txt")
	if err := atomicfile.Write(path, []byte("kept")); err != nil {
		t.Fatalf("writing: %v", err)
	}

	entries, err := os.ReadDir(directory)
	if err != nil {
		t.Fatalf("reading the directory: %v", err)
	}
	if len(entries) != 1 || entries[0].Name() != "story.txt" {
		t.Errorf("the directory holds %v, want the finished file alone", names(entries))
	}
}

// A path that cannot be written to says which file it was about.
func TestAFailedWriteSaysWhatItWasWriting(t *testing.T) {
	t.Parallel()

	// A file where a directory would have to be: the directory cannot be made.
	blocked := filepath.Join(t.TempDir(), "in-the-way")
	if err := os.WriteFile(blocked, []byte("not a directory"), 0o600); err != nil {
		t.Fatalf("setting up: %v", err)
	}

	err := atomicfile.Write(filepath.Join(blocked, "story.txt"), []byte("kept"))
	if err == nil {
		t.Fatal("writing under a file succeeded, want a failure")
	}
	if !strings.Contains(err.Error(), blocked) {
		t.Errorf("the failure reads %q, want the path in it", err)
	}
}

// Nothing is written nowhere: an empty path is a caller's mistake, not a file
// to be made in the working directory.
func TestWritingWithoutAPathIsRefused(t *testing.T) {
	t.Parallel()

	if err := atomicfile.Write("", []byte("kept")); err == nil {
		t.Fatal("writing to an empty path succeeded, want a refusal")
	}
}

func names(entries []os.DirEntry) []string {
	out := make([]string, 0, len(entries))
	for _, entry := range entries {
		out = append(out, entry.Name())
	}
	return out
}
