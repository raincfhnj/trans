// The portable half of the store's tests: nothing here needs access bits or
// symbolic links, so it runs on every platform — including the one the panel
// ships on. The tests that do need a POSIX filesystem are in draft_test.go.
package draft_test

import (
	"os"
	"path/filepath"
	"testing"

	"trans/internal/draft"
)

func TestADraftComesBackForThePaneItWasWrittenFor(t *testing.T) {
	t.Parallel()
	store := draft.NewStore(t.TempDir())

	if err := store.For("w1:p3").Save("Bitte behebe den Test"); err != nil {
		t.Fatalf("Save returned unexpected error: %v", err)
	}

	if kept, _ := store.For("w1:p3").Load(); kept != "Bitte behebe den Test" {
		t.Errorf("Load returned %q, want the saved draft", kept)
	}
	if other, _ := store.For("w1:p9").Load(); other != "" {
		t.Errorf("another pane sees %q, want its own empty draft", other)
	}
}

func TestASentDraftIsForgotten(t *testing.T) {
	t.Parallel()
	store := draft.NewStore(t.TempDir())
	slot := store.For("w1:p3")

	if err := slot.Save("Bitte behebe den Test"); err != nil {
		t.Fatalf("Save returned unexpected error: %v", err)
	}
	if err := slot.Clear(); err != nil {
		t.Fatalf("Clear returned unexpected error: %v", err)
	}

	if kept, _ := slot.Load(); kept != "" {
		t.Errorf("Load returned %q after clearing, want nothing", kept)
	}
}

func TestSavingNothingLeavesNoFileBehind(t *testing.T) {
	t.Parallel()
	directory := t.TempDir()
	slot := draft.NewStore(directory).For("w1:p3")

	if err := slot.Save("Bitte behebe den Test"); err != nil {
		t.Fatalf("Save returned unexpected error: %v", err)
	}
	if err := slot.Save("   \n  "); err != nil {
		t.Fatalf("Save returned unexpected error: %v", err)
	}

	entries, err := os.ReadDir(directory)
	if err != nil {
		t.Fatalf("reading the store: %v", err)
	}
	if len(entries) != 0 {
		t.Errorf("the store holds %d files, want a blank draft to remove its own", len(entries))
	}
}

func TestAPaneIdNeverEscapesTheStoreDirectory(t *testing.T) {
	t.Parallel()
	directory := t.TempDir()
	store := draft.NewStore(directory)

	if err := store.For("../../escaped").Save("Bitte behebe den Test"); err != nil {
		t.Fatalf("Save returned unexpected error: %v", err)
	}

	entries, err := os.ReadDir(directory)
	if err != nil {
		t.Fatalf("reading the store: %v", err)
	}
	if len(entries) != 1 {
		t.Fatalf("the store holds %d files, want the draft inside it", len(entries))
	}
	if kept, _ := store.For("../../escaped").Load(); kept != "Bitte behebe den Test" {
		t.Errorf("Load returned %q, want the draft back", kept)
	}
}

func TestWithoutAStoreDirectoryNothingIsKeptAndNothingFails(t *testing.T) {
	t.Parallel()
	slot := draft.NewStore("").For("w1:p3")

	if err := slot.Save("Bitte behebe den Test"); err != nil {
		t.Errorf("Save returned %v, want a missing store to be no error", err)
	}
	if kept, _ := slot.Load(); kept != "" {
		t.Errorf("Load returned %q, want nothing", kept)
	}
	if err := slot.Clear(); err != nil {
		t.Errorf("Clear returned %v, want a missing store to be no error", err)
	}
}

func TestAMissingDraftIsSimplyEmpty(t *testing.T) {
	t.Parallel()

	text, err := draft.NewStore(t.TempDir()).For("w1:p1").Load()
	if err != nil {
		t.Errorf("Load returned %v for a pane with no draft, want no error", err)
	}
	if text != "" {
		t.Errorf("Load returned %q, want nothing", text)
	}
}

// A draft is written beside its own file first and moved into place, so a
// draft that is being replaced is never half of one thought and half of
// another. The leftover of the write must not be left in the store either.
func TestReplacingADraftLeavesNothingElseInTheStore(t *testing.T) {
	t.Parallel()
	directory := t.TempDir()
	slot := draft.NewStore(directory).For("w1:p3")

	if err := slot.Save("erste Fassung"); err != nil {
		t.Fatalf("Save returned unexpected error: %v", err)
	}
	if err := slot.Save("zweite Fassung"); err != nil {
		t.Fatalf("Save returned unexpected error: %v", err)
	}

	entries, err := os.ReadDir(directory)
	if err != nil {
		t.Fatalf("reading the store: %v", err)
	}
	if len(entries) != 1 {
		t.Errorf("the store holds %d files, want the draft alone", len(entries))
	}
	if kept, _ := slot.Load(); kept != "zweite Fassung" {
		t.Errorf("Load returned %q, want the newer draft", kept)
	}
	if _, err := os.Stat(filepath.Join(directory, entries[0].Name())); err != nil {
		t.Errorf("stating the draft: %v", err)
	}
}
