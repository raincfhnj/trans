//go:build !windows

// The panel is a Windows program: a window opened over a terminal is something
// only Windows does this way.
package main

import (
	"fmt"
	"os"
)

func main() {
	fmt.Fprintln(os.Stderr, "trans-window is for Windows only")
	os.Exit(1)
}
