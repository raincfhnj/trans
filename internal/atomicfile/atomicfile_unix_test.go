//go:build !windows

package atomicfile_test

import (
	"os"
	"path/filepath"
	"testing"

	"trans/internal/atomicfile"
)

// What is written is the author's own: readable by them and by nobody else,
// wherever the filesystem keeps access bits. Windows does not, which is why
// the directory is asked for the same way and why this test is not for it.
func TestWhatIsWrittenIsPrivate(t *testing.T) {
	t.Parallel()

	path := filepath.Join(t.TempDir(), "story.txt")
	if err := atomicfile.Write(path, []byte("mine")); err != nil {
		t.Fatalf("writing: %v", err)
	}

	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("stating: %v", err)
	}
	if mode := info.Mode().Perm(); mode != 0o600 {
		t.Errorf("the file has access %o, want 600", mode)
	}
}
