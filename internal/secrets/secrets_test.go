package secrets

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

// The round trip is what the whole package exists for: bytes in, the same
// bytes back, with nothing readable in between.
func TestARoundTripKeepsTheValue(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("DPAPI only exists on Windows")
	}
	directory := t.TempDir()
	original := []byte("sk-round-trip")

	if err := Store(directory, "TRANS_API_KEY", original, CurrentUser); err != nil {
		t.Fatalf("storing: %v", err)
	}
	got, err := Load(directory, "TRANS_API_KEY")
	if err != nil {
		t.Fatalf("loading: %v", err)
	}
	if !bytes.Equal(got, original) {
		t.Errorf("got %q, want %q", got, original)
	}
}

// What lands on the disk must not be the key itself: if it were, the store
// would be no better than the .env line it replaces.
func TestTheFileNeverHoldsTheValue(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("DPAPI only exists on Windows")
	}
	directory := t.TempDir()
	secret := []byte("sk-never-plain")

	if err := Store(directory, "TRANS_API_KEY", secret, CurrentUser); err != nil {
		t.Fatalf("storing: %v", err)
	}
	encoded, err := os.ReadFile(filepath.Join(directory, "secrets.json"))
	if err != nil {
		t.Fatalf("reading the store: %v", err)
	}
	if bytes.Contains(encoded, secret) {
		t.Error("the store holds the key in the clear")
	}
}

// A flipped byte anywhere in the ciphertext is a different message as far as
// DPAPI is concerned, and the caller must hear that rather than get garbage.
func TestATamperedBlobIsRefused(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("DPAPI only exists on Windows")
	}
	directory := t.TempDir()
	if err := Store(directory, "TRANS_API_KEY", []byte("sk-tamper"), CurrentUser); err != nil {
		t.Fatalf("storing: %v", err)
	}

	contents, err := read(directory)
	if err != nil {
		t.Fatalf("reading the store: %v", err)
	}
	if contents == nil || len(contents.Records) != 1 {
		t.Fatalf("the store holds %v records", contents)
	}
	// Somewhere past the header is encrypted data; a byte there cannot
	// decrypt to what it was, and DPAPI has to say so.
	contents.Records[0].Protected[len(contents.Records[0].Protected)-8] ^= 0xFF
	if err := write(directory, contents); err != nil {
		t.Fatalf("writing back: %v", err)
	}

	if _, err := Load(directory, "TRANS_API_KEY"); err == nil {
		t.Error("a tampered blob decrypted anyway")
	}
}

// A store with nothing in it answers "not found", which is a different thing
// from "could not be read": the caller may have no key on purpose.
func TestAMissingFileIsNotFound(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("DPAPI only exists on Windows")
	}
	if _, err := Load(t.TempDir(), "TRANS_API_KEY"); !errors.Is(err, ErrNotFound) {
		t.Errorf("got %v, want ErrNotFound", err)
	}
}

// The other systems the tree builds for cannot keep a secret this way, and
// saying so plainly beats pretending.
func TestOtherSystemsRefuse(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("this is the refusal for everything but Windows")
	}
	if _, err := Encrypt([]byte("x"), "TRANS_API_KEY", CurrentUser); !errors.Is(err, ErrUnsupported) {
		t.Errorf("Encrypt: got %v, want ErrUnsupported", err)
	}
	if _, err := Decrypt([]byte("x"), "TRANS_API_KEY"); !errors.Is(err, ErrUnsupported) {
		t.Errorf("Decrypt: got %v, want ErrUnsupported", err)
	}
}

// A record is keyed by its variable, so storing one twice leaves one record —
// and a second variable beside it.
func TestStoringTwiceKeepsOneRecordPerVariable(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("DPAPI only exists on Windows")
	}
	directory := t.TempDir()
	if err := Store(directory, "TRANS_API_KEY", []byte("first"), CurrentUser); err != nil {
		t.Fatalf("first store: %v", err)
	}
	if err := Store(directory, "TRANS_API_KEY", []byte("second"), CurrentUser); err != nil {
		t.Fatalf("second store: %v", err)
	}
	if err := Store(directory, "TRANS_DEEPL_API_KEY", []byte("deepl"), CurrentUser); err != nil {
		t.Fatalf("scoped store: %v", err)
	}

	contents, err := read(directory)
	if err != nil {
		t.Fatalf("reading: %v", err)
	}
	if contents == nil || len(contents.Records) != 2 {
		t.Fatalf("got %v records, want 2", contents)
	}
	got, err := Load(directory, "TRANS_API_KEY")
	if err != nil {
		t.Fatalf("loading: %v", err)
	}
	if string(got) != "second" {
		t.Errorf("got %q, want %q", got, "second")
	}
}

// Deleting one variable must not take the others with it.
func TestDeleteLeavesTheOtherRecords(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("DPAPI only exists on Windows")
	}
	directory := t.TempDir()
	if err := Store(directory, "TRANS_API_KEY", []byte("keep"), CurrentUser); err != nil {
		t.Fatalf("storing: %v", err)
	}
	if err := Store(directory, "TRANS_DEEPL_API_KEY", []byte("drop"), CurrentUser); err != nil {
		t.Fatalf("storing scoped: %v", err)
	}
	if err := Delete(directory, "TRANS_DEEPL_API_KEY"); err != nil {
		t.Fatalf("deleting: %v", err)
	}
	if _, err := Load(directory, "TRANS_DEEPL_API_KEY"); !errors.Is(err, ErrNotFound) {
		t.Errorf("the deleted record answers %v, want ErrNotFound", err)
	}
	if _, err := Load(directory, "TRANS_API_KEY"); err != nil {
		t.Errorf("the kept record was lost: %v", err)
	}
}
