//go:build windows

package main

import (
	"fmt"
	"os"
	"runtime"

	"trans/internal/config"
	"trans/internal/service"
	"trans/internal/setup"
)

// runSetup is the first-run wizard's doctor: `trans-window setup`, with
// `trans-window setup doctor` saying the same thing. Everything it reports it
// only reads — no network, no writes beyond config.Prepare's own first
// settings file — so it is safe to run whenever something looks wrong.
func runSetup(arguments []string) error {
	jsonOutput := false

	for _, argument := range arguments {
		switch argument {
		case "doctor":
			// The explicit name of what this already is.
		case "--json":
			jsonOutput = true
		case "-h", "--help":
			fmt.Fprintln(os.Stderr, "usage: trans-window setup [--json] (or: trans-window setup doctor)")
			return nil
		default:
			return fmt.Errorf("unknown argument %q for setup", argument)
		}
	}

	report := setup.Doctor(setup.Options{
		Getenv:    os.Getenv,
		Load:      config.Load,
		Choose:    service.Choose,
		LookPath:  execLookPath,
		Speech:    speechCheck,
		Operating: runtime.GOOS,
	})
	if jsonOutput {
		return report.WriteJSON(os.Stdout)
	}
	return report.WriteHuman(os.Stdout)
}

// execLookPath is how a program on PATH is found: the same question os/exec
// asks, behind a name the tests can replace.
func execLookPath(file string) (string, error) {
	return execLookPathImpl(file)
}
