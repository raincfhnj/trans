package translation

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
)

// FileName is what the written memory is called in the state directory, next
// to the drafts and the record of sent prompts.
const FileName = "tm.jsonl"

// rememberedSentence is one pair as it lies in the file: the key the cache
// looks up — the sentence with the one before it, as the service was told it —
// and what it translated to.
type rememberedSentence struct {
	Key        string `json:"key"`
	Translated string `json:"translated"`
}

// Memory is the sentence cache that outlives the process: what was paid for
// once is not paid for again after the panel is closed and opened. It owns the
// file and the write that did not get through; the sentences themselves stay
// in the segmented cache, which hands the whole memory over to be written
// whenever a sentence joins it.
type Memory struct {
	mu    sync.Mutex
	path  string
	limit int
	// loaded is what the file held when it was opened, oldest first: the same
	// order the ring keeps, so the oldest entry is still the first to go.
	loaded []rememberedSentence
	// pending is a write that failed, kept so the composition root can try it
	// again on the way out.
	pending []byte
}

// NewMemory opens the memory file in the directory the panel keeps its state
// in. An empty directory means the memory is held for this session alone.
func NewMemory(directory string, limit int) *Memory {
	m := &Memory{limit: max(limit, 1)}
	if directory != "" {
		m.path = filepath.Join(directory, FileName)
	}
	m.loaded = m.read()
	return m
}

// read is what the file held, oldest first. A file that is not there is the
// normal case, and one that cannot be read is a cold cache rather than a
// failure: the sentences would cost another request each to have again,
// nothing more, and the next one learned is written over whatever was there.
// A line that does not parse is passed over rather than losing everything
// around it.
func (m *Memory) read() []rememberedSentence {
	if m.path == "" {
		return nil
	}

	file, err := os.Open(m.path)
	if err != nil {
		return nil
	}
	defer file.Close()

	var kept []rememberedSentence
	lines := bufio.NewScanner(file)
	// One line is one sentence with its translation, and a translation of a
	// pasted passage can be long.
	lines.Buffer(make([]byte, 0, 64*1024), 8<<20)
	for lines.Scan() {
		var sentence rememberedSentence
		if err := json.Unmarshal(lines.Bytes(), &sentence); err != nil || sentence.Key == "" {
			continue
		}
		kept = append(kept, sentence)
	}
	if len(kept) > m.limit {
		kept = kept[len(kept)-m.limit:]
	}
	return kept
}

// remember writes the whole memory out — every sentence the cache holds, in
// the order it holds them. It is called with the cache's own lock held, so the
// pair it encodes is one consistent state of that cache; the memory's own lock
// only orders the write against a Flush on the way out.
//
// Every newly finished sentence is written at once rather than on a timer:
// the writes are small, and a panel closed by its window hanging up must not
// lose what it learned.
func (m *Memory) remember(order []string, known map[string]string) {
	payload, err := m.encode(order, known)
	if err != nil {
		// Sentences and their translations marshal or they do not marshal; if
		// that ever fails, the memory keeps what it had rather than writing a
		// hole over it.
		return
	}

	m.mu.Lock()
	defer m.mu.Unlock()
	if err := m.write(payload); err != nil {
		// The memory is an optimization: a sentence that cannot be written is
		// one that costs another request later, never the prompt itself. The
		// bytes are kept so the composition root can try once more on exit.
		m.pending = payload
		return
	}
	m.pending = nil
}

// encode is the cache as JSON lines, oldest first — the order that lets a
// restarted panel evict the same sentences the running one would have.
func (m *Memory) encode(order []string, known map[string]string) ([]byte, error) {
	var out bytes.Buffer
	encoder := json.NewEncoder(&out)
	for _, key := range order {
		translated, found := known[key]
		if !found {
			continue
		}
		if err := encoder.Encode(rememberedSentence{Key: key, Translated: translated}); err != nil {
			return nil, err
		}
	}
	return out.Bytes(), nil
}

// write is one atomic, private write: a fresh file moved into place, so the
// memory is either the old one or the new one and stays readable by its author
// alone even if something loosened the old file's access.
func (m *Memory) write(payload []byte) error {
	if m.path == "" {
		return nil
	}

	fresh, err := os.CreateTemp(filepath.Dir(m.path), "tm-*")
	if err != nil {
		return fmt.Errorf("keeping the translation memory: %w", err)
	}
	defer os.Remove(fresh.Name())

	// The memory is made of the author's own prompts and their English.
	if err := fresh.Chmod(0o600); err != nil {
		_ = fresh.Close()
		return fmt.Errorf("keeping the translation memory private: %w", err)
	}
	if _, err := fresh.Write(payload); err != nil {
		_ = fresh.Close()
		return fmt.Errorf("keeping the translation memory: %w", err)
	}
	if err := fresh.Close(); err != nil {
		return fmt.Errorf("keeping the translation memory: %w", err)
	}
	if err := os.Rename(fresh.Name(), m.path); err != nil {
		return fmt.Errorf("keeping the translation memory: %w", err)
	}
	return nil
}

// Flush retries the one write that did not get through. Everything else was
// already on disk as it was learned, so this does nothing on a good day — and
// nothing at all when there is no memory to write.
func (m *Memory) Flush() error {
	if m == nil {
		return nil
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.pending == nil {
		return nil
	}
	if err := m.write(m.pending); err != nil {
		return fmt.Errorf("the translation memory could not be written: %w", err)
	}
	m.pending = nil
	return nil
}
