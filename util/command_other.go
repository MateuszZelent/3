//go:build !linux

package util

import (
	"context"
	"os"
	"os/exec"
	"time"
)

func CommandContext(ctx context.Context, name string, args ...string) *exec.Cmd {
	cmd := exec.CommandContext(ctx, name, args...)
	cmd.WaitDelay = time.Second
	return cmd
}

func ConfigureProcessGroup(cmd *exec.Cmd) {}
func SignalProcessGroup(cmd *exec.Cmd, sig os.Signal) error {
	if cmd == nil || cmd.Process == nil {
		return nil
	}
	return cmd.Process.Signal(sig)
}
