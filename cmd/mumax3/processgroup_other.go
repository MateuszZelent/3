//go:build !linux

package main

import (
	"os"
	"os/exec"
)

func configureWorkerProcess(cmd *exec.Cmd) {}

func signalWorkerTree(cmd *exec.Cmd, sig os.Signal) error {
	if cmd == nil || cmd.Process == nil {
		return nil
	}
	return cmd.Process.Signal(sig)
}
