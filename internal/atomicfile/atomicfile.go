// Package atomicfile replaces a file in one step. The content goes into a
// fresh file next to the destination and is moved into place, so a reader sees
// either the whole old file or the whole new one — never half of each, which
// for a settings file would refuse to load and for a record of prompts would
// lose the entries on one side of the tear.
//
// The one place this package writes is the one place the mode has to be
// explained: the fresh file is created for its owner alone (`0600`), which
// POSIX filesystems honour and Windows does not — `os.Chmod` there sets the
// read-only bit and nothing else. On Windows the protection is the directory's
// own access control list, which is why the directory is made here with `0700`
// before anything is written into it. The mode is still asked for: it costs
// nothing where it works, and it keeps a loosened file from staying loose after
// the next write.
package atomicfile

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
)

// privateDir is the mode the containing directory is asked for. It matters on
// POSIX filesystems and is inert on Windows, where the parent's access control
// list is what protects the file — and creating the directory is the right
// thing to do either way: the state directory does not exist on a fresh
// installation, and a write that cannot make its own directory fails for no
// good reason.
const privateDir = 0o700

// privateFile is the mode the file is created with, before it is moved into
// place. See the package comment for what it does and does not do on Windows.
const privateFile = 0o600

// Write replaces path with data. The file is created beside its destination —
// the same filesystem, so the move is atomic — and every way out of here
// removes the half-made file, whether the write finished or not.
func Write(path string, data []byte) error {
	if path == "" {
		return errors.New("there is no file to write to")
	}

	directory := filepath.Dir(path)
	if err := os.MkdirAll(directory, privateDir); err != nil {
		return fmt.Errorf("making %s: %w", directory, err)
	}

	fresh, err := os.CreateTemp(directory, filepath.Base(path)+"-*")
	if err != nil {
		return fmt.Errorf("writing %s: %w", path, err)
	}
	// Harmless after the rename, and the way out when anything below fails.
	defer os.Remove(fresh.Name())

	// The three failures below are recorded rather than exercised: each needs a
	// filesystem that fails Chmod, Write or Close on a file it just created,
	// and no standard library hook offers one. A caller that meets them still
	// gets the path and the reason; the half-made file is already on its way
	// out through the defer above.
	if err := fresh.Chmod(privateFile); err != nil {
		_ = fresh.Close()
		return fmt.Errorf("keeping %s private: %w", path, err)
	}
	if _, err := fresh.Write(data); err != nil {
		_ = fresh.Close()
		return fmt.Errorf("writing %s: %w", path, err)
	}
	if err := fresh.Close(); err != nil {
		return fmt.Errorf("writing %s: %w", path, err)
	}
	if err := os.Rename(fresh.Name(), path); err != nil {
		return fmt.Errorf("replacing %s: %w", path, err)
	}
	return nil
}
