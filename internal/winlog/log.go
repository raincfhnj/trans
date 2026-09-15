// Package winlog writes down what a program on Windows has to say when there is
// nowhere to say it: the panel opens in a window of its own and the hotkey has no
// window at all, so a failure would otherwise only be a disappearing process.
package winlog

import (
	"fmt"
	"os"
	"path/filepath"
	"time"
)

// Note appends one line to this program's log, next to the kept drafts. A log
// that cannot be written is not worth failing over: nothing depends on it.
func Note(program, format string, arguments ...any) {
	directory, err := os.UserCacheDir()
	if err != nil {
		return
	}
	directory = filepath.Join(directory, "trans")
	if err := os.MkdirAll(directory, 0o700); err != nil {
		return
	}

	file, err := os.OpenFile(filepath.Join(directory, program+".log"), os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o600)
	if err != nil {
		return
	}
	defer file.Close()

	_, _ = fmt.Fprintf(file, "%s %s\n", time.Now().Format(time.RFC3339), fmt.Sprintf(format, arguments...))
}
