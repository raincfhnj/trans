//go:build !windows

// The hotkey belongs to the Windows panel, which is a Windows program.
package main

import (
	"fmt"
	"os"
)

func main() {
	fmt.Fprintln(os.Stderr, "trans-windowd is for Windows; use the herdr plugin elsewhere")
	os.Exit(1)
}
