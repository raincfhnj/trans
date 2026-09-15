//go:build !windows

// The panel is a Windows program: there is no herdr to host it anywhere else,
// and a window opened over a terminal is something only Windows does this way.
package main

import (
	"fmt"
	"os"
)

func main() {
	fmt.Fprintln(os.Stderr, "trans-window is for Windows; use the herdr plugin elsewhere")
	os.Exit(1)
}
