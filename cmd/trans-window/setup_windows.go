//go:build windows

package main

import (
	"context"
	"os/exec"
	"strings"
	"time"
)

// execLookPathImpl is the real PATH lookup on Windows.
func execLookPathImpl(file string) (string, error) {
	return exec.LookPath(file)
}

// speechCheck asks Windows, through PowerShell, whether a default speech
// synthesis voice can be listed — the question System.Speech would be asked,
// without taking a dependency on it. The answer is only ever a name; when
// PowerShell is missing or slow the caller reports that instead.
func speechCheck() (string, bool) {
	// PowerShell under a profile it does not need, with a deadline: the
	// doctor must never hang on a machine where the voice registry is being
	// repaired or the profile is broken.
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	command := exec.CommandContext(ctx, "powershell", "-NoProfile", "-NonInteractive", "-Command",
		"(Get-ChildItem 'HKLM:\\SOFTWARE\\Microsoft\\Speech\\Voices\\Tokens' | Select-Object -First 1).PSChildName")
	output, err := command.Output()
	if err != nil {
		return "", false
	}
	name := strings.TrimSpace(string(output))
	return name, name != ""
}
