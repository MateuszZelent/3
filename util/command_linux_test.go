//go:build linux

package util

import (
	"context"
	"errors"
	"os/exec"
	"testing"
	"time"
)

func TestCommandCancellationStopsShellAndGrandchild(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cmd := CommandContext(ctx, "/bin/sh", "-c", "sleep 60 & wait")
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	cancel()
	done := make(chan error, 1)
	go func() { done <- cmd.Wait() }()
	select {
	case err := <-done:
		var exit *exec.ExitError
		if !errors.As(err, &exit) {
			t.Fatalf("expected canceled child exit: %v", err)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("cancellation left a child or inherited pipe running")
	}
}
