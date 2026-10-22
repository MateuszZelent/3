//go:build linux

package util

import (
	"context"
	"os"
	"os/exec"
	"syscall"
	"time"
)

// CommandContext owns a fresh process group, including shell grandchildren.
// Cancellation is for auxiliary commands, not simulation workers which first
// receive a graceful SIGTERM through their owner's worker lifecycle.
func CommandContext(ctx context.Context, name string, args ...string) *exec.Cmd {
	cmd := exec.CommandContext(ctx, name, args...)
	ConfigureProcessGroup(cmd)
	cmd.Cancel = func() error {
		if cmd.Process == nil {
			return os.ErrProcessDone
		}
		err := syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
		if err == syscall.ESRCH {
			return os.ErrProcessDone
		}
		return err
	}
	cmd.WaitDelay = time.Second
	return cmd
}

func ConfigureProcessGroup(cmd *exec.Cmd) {
	if cmd.SysProcAttr == nil {
		cmd.SysProcAttr = &syscall.SysProcAttr{}
	}
	cmd.SysProcAttr.Setpgid = true
}

func SignalProcessGroup(cmd *exec.Cmd, sig os.Signal) error {
	if cmd == nil || cmd.Process == nil {
		return nil
	}
	if s, ok := sig.(syscall.Signal); ok && cmd.SysProcAttr != nil && cmd.SysProcAttr.Setpgid {
		return syscall.Kill(-cmd.Process.Pid, s)
	}
	return cmd.Process.Signal(sig)
}
