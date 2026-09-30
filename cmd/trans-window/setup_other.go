//go:build !windows

package main

import "errors"

// The panel is a Windows program; on anything else there is nothing to check
// but the fact itself. The file exists so that `go build ./...` on a Linux CI
// sees the same package shape a Windows build does.
func runSetup(arguments []string) error {
	return errors.New("trans-window setup is for Windows only")
}

func execLookPathImpl(file string) (string, error) {
	return "", errors.New("not on Windows")
}

func speechCheck() (string, bool) {
	return "", false
}
