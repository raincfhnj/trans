//go:build !windows

package history_test

import (
	"os"
	"path/filepath"
	"testing"

	"trans/internal/history"
	"trans/internal/promptflow"
)

// Windows reports every file as readable by everyone, so the access bits are
// only something Unix keeps promises about.
func TestTheRecordIsPrivateToItsAuthor(t *testing.T) {
	directory := t.TempDir()
	// One prompt fits; the second rewrites the file whole, so both ways of
	// writing are covered.
	log := history.NewStore(directory, 1).For("pane")

	if err := log.Record("prompt", "translation", promptflow.Sending); err != nil {
		t.Fatalf("recording the prompt: %v", err)
	}
	if err := log.Record("another prompt", "", promptflow.Sending); err != nil {
		t.Fatalf("recording a prompt that trims the record: %v", err)
	}

	info, err := os.Stat(filepath.Join(directory, "history.jsonl"))
	if err != nil {
		t.Fatalf("stating the record: %v", err)
	}
	if mode := info.Mode().Perm(); mode&0o077 != 0 {
		t.Errorf("the record has access %o, want it readable by its author alone", mode)
	}
}
