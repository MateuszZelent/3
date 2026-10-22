package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"github.com/mumax/3/util"
)

func TestLegacyKillAllowsWorkerCleanup(t *testing.T) {
	dir := t.TempDir()
	marker := filepath.Join(dir, "cleaned")
	cmd := exec.Command("/bin/sh", "-c", "trap 'echo cleaned > \"$1\"; exit 0' TERM; echo ready > \"$2\"; while :; do sleep 1; done", "worker", marker, filepath.Join(dir, "ready"))
	util.ConfigureProcessGroup(cmd)
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = cmd.Process.Kill() })
	deadline := time.Now().Add(3 * time.Second)
	for {
		if _, err := os.Stat(filepath.Join(dir, "ready")); err == nil {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("helper did not start")
		}
		time.Sleep(10 * time.Millisecond)
	}
	job := &Process{Cmd: cmd}
	WLock()
	previous := Processes["cleanup-test"]
	Processes["cleanup-test"] = job
	WUnlock()
	t.Cleanup(func() {
		WLock()
		if previous == nil {
			delete(Processes, "cleanup-test")
		} else {
			Processes["cleanup-test"] = previous
		}
		WUnlock()
	})
	if message := Kill("cleanup-test"); message != "" {
		t.Fatal(message)
	}
	done := make(chan error, 1)
	go func() { done <- cmd.Wait() }()
	select {
	case err := <-done:
		if err != nil {
			t.Fatal("worker was killed before cleanup:", err)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("worker did not stop")
	}
	if _, err := os.Stat(marker); err != nil {
		t.Fatal("SIGTERM cleanup was skipped:", err)
	}
}

func TestLegacyKillBeforeStartIsRecorded(t *testing.T) {
	job := &Process{Cmd: exec.Command("unused")}
	WLock()
	previous := Processes["pending-test"]
	Processes["pending-test"] = job
	WUnlock()
	t.Cleanup(func() {
		WLock()
		if previous == nil {
			delete(Processes, "pending-test")
		} else {
			Processes["pending-test"] = previous
		}
		WUnlock()
	})
	if message := Kill("pending-test"); message != "" {
		t.Fatal(message)
	}
	if !job.Killed {
		t.Fatal("pending cancellation lost")
	}
}
