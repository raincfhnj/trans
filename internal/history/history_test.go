package history_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"trans/internal/history"
	"trans/internal/promptflow"
)

func TestAPromptThatWasDeliveredCanBeReadBackWithWhereItWent(t *testing.T) {
	log := history.NewStore(t.TempDir(), 10).For("notes.md — pane")

	if err := log.Record("Bitte prüf den Test", "Please check the test", promptflow.Sending); err != nil {
		t.Fatalf("recording the prompt: %v", err)
	}

	entries, err := log.Entries()
	if err != nil {
		t.Fatalf("reading the record: %v", err)
	}
	if len(entries) != 1 {
		t.Fatalf("got %d entries, want 1", len(entries))
	}

	delivered := entries[0]
	if delivered.Source != "Bitte prüf den Test" {
		t.Errorf("source = %q, want the prompt as written", delivered.Source)
	}
	if delivered.Translation != "Please check the test" {
		t.Errorf("translation = %q, want what the agent received", delivered.Translation)
	}
	if delivered.Window != "notes.md — pane" {
		t.Errorf("window = %q, want the window it was delivered into", delivered.Window)
	}
	if delivered.Delivery != "sending" {
		t.Errorf("delivery = %q, want \"sending\"", delivered.Delivery)
	}
	if delivered.At.IsZero() {
		t.Error("the entry has no time, so the newest cannot be told from the oldest")
	}
}

func TestEveryWindowWritesIntoTheSameRecord(t *testing.T) {
	store := history.NewStore(t.TempDir(), 10)

	if err := store.For("one").Record("first prompt", "erste Aufforderung", promptflow.Sending); err != nil {
		t.Fatalf("recording for the first window: %v", err)
	}
	if err := store.For("two").Record("second prompt", "zweite Aufforderung", promptflow.Typing); err != nil {
		t.Fatalf("recording for the second window: %v", err)
	}

	entries, err := store.For("two").Entries()
	if err != nil {
		t.Fatalf("reading the record: %v", err)
	}
	if len(entries) != 2 {
		t.Fatalf("got %d entries, want 2", len(entries))
	}
	if entries[0].Window != "two" || entries[1].Window != "one" {
		t.Errorf("windows = %q then %q, want the newest entry first", entries[0].Window, entries[1].Window)
	}
	if entries[0].Delivery != "typing" {
		t.Errorf("delivery = %q, want \"typing\"", entries[0].Delivery)
	}
}

func TestTheOldestPromptsAreDroppedOnceTheRecordIsFull(t *testing.T) {
	directory := t.TempDir()
	log := history.NewStore(directory, 2).For("pane")

	for _, prompt := range []string{"first prompt", "second prompt", "third prompt"} {
		if err := log.Record(prompt, "", promptflow.Sending); err != nil {
			t.Fatalf("recording %q: %v", prompt, err)
		}
	}

	entries, err := log.Entries()
	if err != nil {
		t.Fatalf("reading the record: %v", err)
	}
	if len(entries) != 2 {
		t.Fatalf("got %d entries, want the limit of 2", len(entries))
	}
	if entries[0].Source != "third prompt" || entries[1].Source != "second prompt" {
		t.Errorf("kept %q then %q, want the newest two prompts", entries[0].Source, entries[1].Source)
	}

	// The record is rewritten whole when it is trimmed, so the file holds the
	// two entries and nothing of the third's neighbour's line, and no scratch
	// file is left lying about.
	contents, err := os.ReadFile(filepath.Join(directory, "history.jsonl"))
	if err != nil {
		t.Fatalf("reading the file: %v", err)
	}
	if lines := strings.Count(string(contents), "\n"); lines != 2 {
		t.Errorf("the file holds %d lines, want 2 after the trim", lines)
	}
	leftovers, err := os.ReadDir(directory)
	if err != nil {
		t.Fatalf("listing the directory: %v", err)
	}
	if len(leftovers) != 1 {
		t.Errorf("the directory holds %d files, want only the record", len(leftovers))
	}
}

func TestALineThatIsNotAnEntryDoesNotHideTheOnesAroundIt(t *testing.T) {
	directory := t.TempDir()
	log := history.NewStore(directory, 10).For("pane")

	for _, prompt := range []string{"first prompt", "second prompt"} {
		if err := log.Record(prompt, "", promptflow.Sending); err != nil {
			t.Fatalf("recording %q: %v", prompt, err)
		}
	}
	file := filepath.Join(directory, "history.jsonl")
	kept, err := os.ReadFile(file)
	if err != nil {
		t.Fatalf("reading the file: %v", err)
	}
	// A torn write leaves half a JSON line behind; it must cost only itself.
	if err := os.WriteFile(file, append([]byte("{torn\n"), kept...), 0o600); err != nil {
		t.Fatalf("corrupting the file: %v", err)
	}
	if err := log.Record("third prompt", "", promptflow.Sending); err != nil {
		t.Fatalf("recording after the torn line: %v", err)
	}

	entries, err := log.Entries()
	if err != nil {
		t.Fatalf("reading the record: %v", err)
	}
	if len(entries) != 3 {
		t.Fatalf("got %d entries, want 3 — only the torn line is lost", len(entries))
	}
	if entries[0].Source != "third prompt" || entries[2].Source != "first prompt" {
		t.Errorf("got %q first and %q last, want the record in order", entries[0].Source, entries[2].Source)
	}
}

func TestAPromptCanBeTakenOutOfTheRecordAgain(t *testing.T) {
	directory := t.TempDir()
	log := history.NewStore(directory, 10).For("pane")

	for _, prompt := range []string{"first prompt", "second prompt"} {
		if err := log.Record(prompt, "", promptflow.Sending); err != nil {
			t.Fatalf("recording %q: %v", prompt, err)
		}
	}
	entries, err := log.Entries()
	if err != nil {
		t.Fatalf("reading the record: %v", err)
	}
	if err := log.Forget(&entries[0]); err != nil {
		t.Fatalf("forgetting the newest prompt: %v", err)
	}

	remaining, err := log.Entries()
	if err != nil {
		t.Fatalf("reading the record: %v", err)
	}
	if len(remaining) != 1 || remaining[0].Source != "first prompt" {
		t.Fatalf("kept %d entries, want only the untouched one", len(remaining))
	}
	contents, err := os.ReadFile(filepath.Join(directory, "history.jsonl"))
	if err != nil {
		t.Fatalf("reading the file: %v", err)
	}
	if lines := strings.Count(string(contents), "\n"); lines != 1 {
		t.Errorf("the file holds %d lines, want 1 after the removal", lines)
	}
}

func TestNothingIsRecordedWhenNoDirectoryWasGiven(t *testing.T) {
	log := history.NewStore("", 10).For("pane")

	if err := log.Record("prompt", "translation", promptflow.Sending); err != nil {
		t.Errorf("recording without a directory: %v", err)
	}
	entries, err := log.Entries()
	if err != nil {
		t.Errorf("reading without a directory: %v", err)
	}
	if entries != nil {
		t.Errorf("got %d entries, want none", len(entries))
	}
	if err := log.Forget(&history.Entry{}); err != nil {
		t.Errorf("forgetting without a directory: %v", err)
	}
}
