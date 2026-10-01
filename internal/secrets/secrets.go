// Package secrets keeps the provider API key off the disk in plaintext. On
// Windows the key is wrapped with the Data Protection API so only this user's
// sessions can read it back; on other systems there is nothing to wrap it
// with, and every function answers ErrUnsupported.
//
// The store lives beside the .env file in the configuration directory, as
// secrets.json: one record per variable, each holding the variable name it
// stands for and the protected value. Nothing here reads or writes the .env
// file itself — that stays config's business.
package secrets

import (
	"errors"
	"os"
	"path/filepath"

	"trans/internal/atomicfile"
)

// ErrUnsupported is what every function answers where DPAPI does not exist,
// so callers can tell "not here" from "your key is wrong".
var ErrUnsupported = errors.New("secrets: protected storage is only available on Windows")

// ErrNotFound is answered when the store has no record for a variable.
var ErrNotFound = errors.New("secrets: no stored value for that setting")

// Scope says whose identity the value is bound to. CurrentUser is the
// default: the ciphertext is readable only by this account's logon sessions,
// which is what a personal API key wants. LocalMachine binds it to the
// machine instead, so any user of this computer can decrypt it — useful when
// a service runs under another account, and documented here because it is
// easy to reach for by accident.
type Scope int

const (
	CurrentUser Scope = iota
	LocalMachine
)

// Record is one stored variable: the name it replaces in the settings and the
// protected bytes that hold its value.
type Record struct {
	Variable  string `json:"variable"`
	Protected []byte `json:"protected"`
}

// File is the whole store, kept as one JSON document so a single atomic
// replace keeps it consistent.
type File struct {
	Records []Record `json:"records"`
}

// path is where the store sits for a configuration directory: next to .env,
// never anywhere else, so deleting the directory takes the key with it.
func path(directory string) string {
	return filepath.Join(directory, "secrets.json")
}

// write replaces the store in one step, through the same private write the
// drafts and the settings file use: a half-written store would refuse to load
// at the next start, and this file holds the only copy of the key. The name of
// the file being written is the shared write's business too — a fixed one would
// be a name two programs could take turns clobbering.
func write(directory string, contents *File) error {
	if directory == "" {
		return errors.New("secrets: no configuration directory to store in")
	}
	encoded, err := marshal(contents)
	if err != nil {
		return err
	}
	return atomicfile.Write(path(directory), encoded)
}

// read opens the store, answering nil (with no error) when there is no file
// yet: an empty store is a normal state, not a failure.
func read(directory string) (*File, error) {
	if directory == "" {
		return nil, ErrNotFound
	}
	encoded, err := os.ReadFile(path(directory))
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, nil
		}
		return nil, err
	}
	return unmarshal(encoded)
}
