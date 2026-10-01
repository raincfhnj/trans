package config

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"regexp"
	"sort"
	"strings"

	"trans/internal/atomicfile"
)

// A setting is a name and a value on one line, so anything written under some
// other name would take the file apart.
var settingName = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*$`)

// Save writes these settings into the .env file, keeping every line it does
// not name — comments, settings left alone, and their order — as it stands. A
// value of nothing takes the setting out of the file, which is how a setting
// goes back to its default.
func Save(file string, updates map[string]string) error {
	if file == "" {
		return errors.New("there is no configuration file to write to")
	}
	// Everything is judged before anything is written, so a refused setting
	// leaves the file exactly as it was.
	for name, value := range updates {
		if !settingName.MatchString(name) {
			return fmt.Errorf("%q is not a setting's name", name)
		}
		if strings.ContainsAny(value, "\r\n") {
			return fmt.Errorf("%s cannot be written: a setting is one line, "+
				"and this value has a line break in it", name)
		}
	}

	content, err := merged(file, updates)
	if err != nil {
		return err
	}
	return write(file, content)
}

// merged reads the file as it stands and answers it with the updates in place.
// A file that is not there yet starts from nothing: the plugin is allowed to
// gain its first settings file from the window that edits them.
func merged(file string, updates map[string]string) (string, error) {
	existing, err := os.ReadFile(file)
	if err != nil && !errors.Is(err, fs.ErrNotExist) {
		return "", fmt.Errorf("reading %s: %w", file, err)
	}

	// A file written on Windows keeps its line endings.
	text := string(existing)
	ending := "\n"
	if strings.Contains(text, "\r\n") {
		ending = "\r\n"
		text = strings.ReplaceAll(text, "\r\n", "\n")
	}
	lines := strings.Split(text, "\n")
	if last := len(lines) - 1; last >= 0 && lines[last] == "" {
		// The final newline, put back below rather than kept as a blank line.
		lines = lines[:last]
	}

	seen := map[string]bool{}
	kept := make([]string, 0, len(lines)+len(updates))
	for _, line := range lines {
		name, listed := namedLine(line)
		value, changed := updates[name]
		if !listed || !changed {
			kept = append(kept, line)
			continue
		}
		seen[name] = true
		if value == "" {
			continue
		}
		kept = append(kept, name+"="+written(value))
	}

	// A setting the file did not have yet is written at the end, in name
	// order, so saving twice leaves the same file both times.
	fresh := make([]string, 0, len(updates))
	for name, value := range updates {
		if seen[name] || value == "" {
			continue
		}
		fresh = append(fresh, name+"="+written(value))
	}
	sort.Strings(fresh)

	kept = append(kept, fresh...)
	if len(kept) == 0 {
		return "", nil
	}
	return strings.Join(kept, ending) + ending, nil
}

// namedLine takes the setting's name off a line, saying whether the line is
// one at all: a comment, a blank, or a stray line without an equals sign is
// left as it stands, whatever was asked for.
func namedLine(line string) (string, bool) {
	trimmed := strings.TrimSpace(line)
	if trimmed == "" || strings.HasPrefix(trimmed, "#") {
		return "", false
	}
	name, _, found := strings.Cut(trimmed, "=")
	if !found {
		return "", false
	}
	return strings.TrimSpace(name), true
}

// written wraps a value in quotes only when reading it back would not give it
// as it stands: around leading or trailing space, or around a value that
// begins and ends with the same quote, which the reader would otherwise take
// for quoting of its own.
func written(value string) string {
	if value != strings.TrimSpace(value) {
		return `"` + value + `"`
	}
	if len(value) >= 2 {
		first, last := value[0], value[len(value)-1]
		if (first == '"' || first == '\'') && last == first {
			return `"` + value + `"`
		}
	}
	return value
}

// write replaces the file in one step: a half-written settings file would
// refuse to load at the next start, and the file is private while it holds a
// key.
func write(file, content string) error {
	return atomicfile.Write(file, []byte(content))
}
