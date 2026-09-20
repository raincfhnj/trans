//go:build !windows

// The hotkey daemon is a Windows-only program.
package main

import (
	"fmt"
	"os"
)

func main() {
	fmt.Fprintln(os.Stderr, "trans-windowd is for Windows only")
	os.Exit(1)
}
