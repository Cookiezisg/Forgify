package sandbox

import (
	"context"
	"os/exec"
	"runtime"
	"syscall"
)

// commandContext creates an exec.Cmd with proper process group settings
// so the entire process tree can be killed on timeout.
func commandContext(ctx context.Context, name string, args ...string) *exec.Cmd {
	cmd := exec.CommandContext(ctx, name, args...)
	if runtime.GOOS != "windows" {
		cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	}
	return cmd
}
