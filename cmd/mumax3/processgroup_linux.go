//go:build linux

package main

import (
	"os"
	"os/exec"
	"syscall"
)

func configureWorkerProcess(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
}

func signalWorkerTree(cmd *exec.Cmd, sig os.Signal) error {
	if cmd == nil || cmd.Process == nil {
		return nil
	}
	if signal, ok := sig.(syscall.Signal); ok {
		return syscall.Kill(-cmd.Process.Pid, signal)
	}
	return cmd.Process.Signal(sig)
}
