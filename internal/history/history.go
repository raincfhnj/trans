// Package history keeps the prompts a panel has already delivered, so the same
// sentence can be found again instead of being remembered and retyped. The file
// holds the author's own prompts, and is written the way the draft store is:
// through the same private write, and never left half-written.
package history

import (
	"bufio"
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"trans/internal/atomicfile"
	"trans/internal/promptflow"
)

// fileName is the record's name in the state directory. One file for every
// window: the panel is opened per window, but the prompts belong to whoever
// was writing them, not to one pane.
const fileName = "history.jsonl"

// Entry is one prompt that reached the agent: what was written, what was sent
// or typed, and where it went.
type Entry struct {
	At          time.Time `json:"at"`
	Source      string    `json:"source"`
	Translation string    `json:"translation"`
	Window      string    `json:"window"`
	Delivery    string    `json:"delivery"`
}

// Store is the record itself, in the directory the panel keeps its state in.
type Store struct {
	path  string
	limit int
}

// NewStore takes the state directory and how many prompts are worth keeping.
// An empty directory means nothing is recorded at all.
func NewStore(directory string, limit int) Store {
	path := ""
	if directory != "" {
		path = filepath.Join(directory, fileName)
	}
	// A limit that lets nothing in is not a setting anybody meant to make.
	return Store{path: path, limit: max(limit, 1)}
}

// For is the record as one window writes into it: every entry says which
// window it was delivered into, so a prompt written for one pane can be found
// again after it was delivered somewhere else.
func (s Store) For(window string) Log {
	return Log{store: s, window: window}
}

// Log is one window's way into the record.
type Log struct {
	store  Store
	window string
}

// Record keeps a prompt that reached the agent. A line is appended as it is;
// only a record grown past its limit is rewritten whole, and then it is
// rewritten the way the draft is written — a fresh file moved into place — so
// the record is either the old one or the new one, never half of either.
func (l Log) Record(source, translation string, how promptflow.Delivery) error {
	if l.store.path == "" {
		return nil
	}

	kept, err := l.store.read()
	if err != nil {
		return err
	}
	entry := Entry{
		At:          time.Now(),
		Source:      source,
		Translation: translation,
		Window:      l.window,
		Delivery:    how.String(),
	}
	kept = append(kept, entry)

	// The prompt sent last week is the one still worth finding again, so the
	// oldest goes first.
	if len(kept) > l.store.limit {
		return l.store.rewrite(kept[len(kept)-l.store.limit:])
	}
	return l.store.append(&entry)
}

// Entries is the record, newest first: the prompt sent a minute ago is the one
// worth seeing first. A line that does not parse is passed over rather than
// hiding everything written around it.
func (l Log) Entries() ([]Entry, error) {
	if l.store.path == "" {
		return nil, nil
	}

	kept, err := l.store.read()
	if err != nil {
		return nil, err
	}
	newestFirst := make([]Entry, 0, len(kept))
	for index := len(kept) - 1; index >= 0; index-- {
		newestFirst = append(newestFirst, kept[index])
	}
	return newestFirst, nil
}

// Forget drops one entry out of the record, leaving the rest as they were.
// The entry is taken as a pointer because it is heavy to pass around: it goes
// through an interface, and the panel only ever has one of them at a time.
func (l Log) Forget(wanted *Entry) error {
	if l.store.path == "" {
		return nil
	}

	kept, err := l.store.read()
	if err != nil {
		return err
	}
	left := make([]Entry, 0, len(kept))
	for _, entry := range kept {
		if entry == *wanted {
			continue
		}
		left = append(left, entry)
	}
	if len(left) == len(kept) {
		return nil
	}
	return l.store.rewrite(left)
}

// read is the whole record in the order it was written, oldest first. A line
// that is not an entry — a torn write, a byte out of place — is skipped; one
// bad line must not hide every prompt written around it.
func (s Store) read() ([]Entry, error) {
	file, err := os.Open(s.path)
	switch {
	case errors.Is(err, os.ErrNotExist):
		return nil, nil
	case err != nil:
		return nil, fmt.Errorf("reading the record of sent prompts: %w", err)
	}
	defer file.Close()

	var kept []Entry
	lines := bufio.NewScanner(file)
	// A prompt can be long, and a JSON line holds it twice over — as the
	// source and escaped for the file.
	lines.Buffer(make([]byte, 0, 64*1024), 8<<20)
	for lines.Scan() {
		var entry Entry
		if err := json.Unmarshal(lines.Bytes(), &entry); err != nil {
			continue
		}
		kept = append(kept, entry)
	}
	if err := lines.Err(); err != nil {
		return nil, fmt.Errorf("reading the record of sent prompts: %w", err)
	}
	return kept, nil
}

// append adds one entry to the record. The line is opened for appending, so
// two panels writing at the same time each land their own line rather than
// one overwriting the other.
func (s Store) append(entry *Entry) error {
	line, err := json.Marshal(entry)
	if err != nil {
		return fmt.Errorf("keeping the record of sent prompts: %w", err)
	}

	file, err := os.OpenFile(s.path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o600)
	if err != nil {
		return fmt.Errorf("keeping the record of sent prompts: %w", err)
	}
	if _, err := file.Write(append(line, '\n')); err != nil {
		_ = file.Close()
		return fmt.Errorf("keeping the record of sent prompts: %w", err)
	}
	if err := file.Close(); err != nil {
		return fmt.Errorf("keeping the record of sent prompts: %w", err)
	}
	return nil
}

// rewrite writes the record whole: a fresh file, written the way the draft is
// written, moved into place over the old one. The record stays private even if
// something loosened the old file's access, and it is never left half of one
// record and half of another.
func (s Store) rewrite(kept []Entry) error {
	var whole bytes.Buffer
	encoder := json.NewEncoder(&whole)
	for _, entry := range kept {
		if err := encoder.Encode(entry); err != nil {
			return fmt.Errorf("keeping the record of sent prompts: %w", err)
		}
	}

	if err := atomicfile.Write(s.path, whole.Bytes()); err != nil {
		return fmt.Errorf("keeping the record of sent prompts: %w", err)
	}
	return nil
}
