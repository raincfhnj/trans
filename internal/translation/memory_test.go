package translation_test

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"trans/internal/translation"
)

func TestASentencePaidForOnceIsNotPaidForAgainAfterARestart(t *testing.T) {
	t.Parallel()
	directory := t.TempDir()
	const draft = "Erster Satz. Zweiter Satz!"

	first := &spyTranslator{}
	opening := translation.Segmented(first, translation.NewMemory(directory, 10))
	translated, err := opening.Translate(context.Background(), draft)
	if err != nil {
		t.Fatalf("first Translate returned unexpected error: %v", err)
	}
	if len(first.sent()) != 2 {
		t.Fatalf("the first session sent %v, want one request per sentence", first.sent())
	}

	// The panel is closed and a new one opens on the same directory: what the
	// first one paid for is on disk before it asked for it a second time.
	second := &spyTranslator{}
	again := translation.Segmented(second, translation.NewMemory(directory, 10))
	repeated, err := again.Translate(context.Background(), draft)
	if err != nil {
		t.Fatalf("second Translate returned unexpected error: %v", err)
	}
	if sent := second.sent(); len(sent) != 0 {
		t.Errorf("the second session sent %v, want nothing — both sentences were already paid for", sent)
	}
	if repeated != translated {
		t.Errorf("the restarted session returned %q, want the translation kept from the first %q",
			repeated, translated)
	}
}

func TestTheTranslationMemoryKeepsOnlyTheNewestSentences(t *testing.T) {
	t.Parallel()
	directory := t.TempDir()
	const draft = "Erster Satz. Zweiter Satz. Dritter Satz."

	first := &spyTranslator{}
	opening := translation.Segmented(first, translation.NewMemory(directory, 2))
	if _, err := opening.Translate(context.Background(), draft); err != nil {
		t.Fatalf("first Translate returned unexpected error: %v", err)
	}

	// The file holds the two newest sentences and nothing of the one the ring
	// evicted: the sentence after the NUL is what the cache looks up, the
	// sentences before it are there as context.
	kept, err := os.ReadFile(filepath.Join(directory, translation.FileName))
	if err != nil {
		t.Fatalf("reading the memory: %v", err)
	}
	sentences := sentencesIn(t, kept)
	if sentences["Erster Satz."] {
		t.Errorf("the evicted sentence is still in the memory: %v", sentences)
	}
	if !sentences["Zweiter Satz."] || !sentences["Dritter Satz."] {
		t.Errorf("the memory holds %v, want the two newest sentences", sentences)
	}

	// A fresh session starts from what the file holds — with room for one more,
	// so what it learned can be told apart from what it read — and pays only
	// for the sentence that was dropped.
	second := &spyTranslator{}
	again := translation.Segmented(second, translation.NewMemory(directory, 3))
	if _, err := again.Translate(context.Background(), draft); err != nil {
		t.Fatalf("second Translate returned unexpected error: %v", err)
	}
	sent := second.sent()
	if len(sent) != 1 || strings.TrimSpace(sent[0]) != "Erster Satz." {
		t.Errorf("the second session sent %v, want only the evicted sentence", sent)
	}
}

// sentencesIn is what the file keeps as cache keys: the sentence after the
// NUL, which is what a translation is looked up by.
func sentencesIn(t *testing.T, kept []byte) map[string]bool {
	t.Helper()

	decoder := json.NewDecoder(bytes.NewReader(kept))
	found := map[string]bool{}
	for {
		var line struct {
			Key string `json:"key"`
		}
		if err := decoder.Decode(&line); err != nil {
			break
		}
		if _, sentence, split := strings.Cut(line.Key, "\x00"); split {
			found[sentence] = true
		}
	}
	return found
}

func TestTheTranslationMemoryReadsBackThroughABrokenLine(t *testing.T) {
	t.Parallel()
	directory := t.TempDir()
	file := filepath.Join(directory, translation.FileName)

	first := &spyTranslator{}
	opening := translation.Segmented(first, translation.NewMemory(directory, 10))
	if _, err := opening.Translate(context.Background(), "Erster Satz."); err != nil {
		t.Fatalf("first Translate returned unexpected error: %v", err)
	}

	kept, err := os.ReadFile(file)
	if err != nil {
		t.Fatalf("reading the memory: %v", err)
	}
	// A torn write leaves half a JSON line behind; it must cost only itself.
	if err := os.WriteFile(file, append([]byte("{torn\n"), kept...), 0o600); err != nil {
		t.Fatalf("corrupting the memory: %v", err)
	}

	second := &spyTranslator{}
	again := translation.Segmented(second, translation.NewMemory(directory, 10))
	if _, err := again.Translate(context.Background(), "Erster Satz."); err != nil {
		t.Fatalf("Translate after the torn line returned unexpected error: %v", err)
	}
	if sent := second.sent(); len(sent) != 0 {
		t.Errorf("the session after the torn line sent %v, want the intact line read back", sent)
	}
}

// The cache sits outside protection, so what it writes is the sentence with
// the code held out of it — which is also why the code itself never reaches
// the disk in the memory.
func TestTheMemoryKeepsTheProtectedTextAndNeverTheCode(t *testing.T) {
	t.Parallel()
	directory := t.TempDir()

	service := &spyTranslator{}
	translator := translation.Protecting(
		translation.Segmented(service, translation.NewMemory(directory, 10)))
	if _, err := translator.Translate(context.Background(), "Bitte prüf `foo()` im Test."); err != nil {
		t.Fatalf("Translate returned unexpected error: %v", err)
	}

	kept, err := os.ReadFile(filepath.Join(directory, translation.FileName))
	if err != nil {
		t.Fatalf("reading the memory: %v", err)
	}
	if bytes.Contains(kept, []byte("foo()")) {
		t.Errorf("the code itself was written to the memory:\n%s", kept)
	}
	if !bytes.Contains(kept, []byte("⟦0⟧")) {
		t.Errorf("the protected sentence was not written to the memory:\n%s", kept)
	}
}

func TestAWriteThatCannotGoThroughIsTriedAgainOnTheWayOut(t *testing.T) {
	t.Parallel()
	// The directory the memory lives in does not exist yet, so every write
	// fails until it is made.
	directory := filepath.Join(t.TempDir(), "not-yet")
	memory := translation.NewMemory(directory, 10)

	opening := translation.Segmented(&spyTranslator{}, memory)
	if _, err := opening.Translate(context.Background(), "Erster Satz."); err != nil {
		t.Fatalf("Translate returned unexpected error: %v", err)
	}

	if err := os.MkdirAll(directory, 0o700); err != nil {
		t.Fatalf("making the directory: %v", err)
	}
	if err := memory.Flush(); err != nil {
		t.Fatalf("Flush returned %v, want the kept write to get through", err)
	}

	kept, err := os.ReadFile(filepath.Join(directory, translation.FileName))
	if err != nil {
		t.Fatalf("reading the memory: %v", err)
	}
	if !bytes.Contains(kept, []byte("Erster Satz.")) {
		t.Errorf("the memory holds %q, want the sentence that failed its first write", kept)
	}

	// Nothing is left over, so a second Flush has nothing to do.
	if err := memory.Flush(); err != nil {
		t.Errorf("Flush after the retry returned %v, want nothing left to write", err)
	}
}

func TestThereIsNothingToFlushWhenTheMemoryIsOnlyForThisSession(t *testing.T) {
	t.Parallel()

	memory := translation.NewMemory("", 10)
	opening := translation.Segmented(&spyTranslator{}, memory)
	if _, err := opening.Translate(context.Background(), "Erster Satz."); err != nil {
		t.Fatalf("Translate returned unexpected error: %v", err)
	}
	if err := memory.Flush(); err != nil {
		t.Errorf("Flush returned %v, want nothing to write without a directory", err)
	}
}
