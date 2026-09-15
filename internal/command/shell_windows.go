//go:build windows

package command

import (
	"context"
	"os/exec"
)

// shellCommand puts the command line in a shell. Windows has no sh; the command
// interpreter is the one every install has. Killing the process tree is left to
// the platform: a command that ignores the deadline must not hold the popup.
func shellCommand(ctx context.Context, commandLine string) *exec.Cmd {
	return exec.CommandContext(ctx, "cmd.exe", "/C", commandLine) // #nosec G204
}
