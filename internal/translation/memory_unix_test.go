//go:build !windows

package translation_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"trans/internal/translation"
)

// Windows reports every file as readable by everyone, so the access bits are
// only something Unix keeps promises about.
func TestTheTranslationMemoryIsPrivateToItsAuthor(t *testing.T) {
	t.Parallel()
	directory := t.TempDir()

	translator := translation.Segmented(&spyTranslator{},
		translation.NewMemory(directory, 10))
	if _, err := translator.Translate(context.Background(), "Erster Satz."); err != nil {
		t.Fatalf("Translate returned unexpected error: %v", err)
	}

	info, err := os.Stat(filepath.Join(directory, translation.FileName))
	if err != nil {
		t.Fatalf("stating the memory: %v", err)
	}
	if mode := info.Mode().Perm(); mode&0o077 != 0 {
		t.Errorf("the memory has access %o, want it readable by its author alone", mode)
	}
}
