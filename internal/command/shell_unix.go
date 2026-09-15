//go:build !windows

package command

import (
	"context"
	"os/exec"
	"syscall"
)

// shellCommand puts the command line in a shell. The command line may start
// several programs, and killing the shell alone leaves them running and holding
// the output open, so they are given a process group of their own.
func shellCommand(ctx context.Context, commandLine string) *exec.Cmd {
	running := exec.CommandContext(ctx, "sh", "-c", commandLine) // #nosec G204
	running.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	running.Cancel = func() error {
		return syscall.Kill(-running.Process.Pid, syscall.SIGKILL)
	}
	return running
}
