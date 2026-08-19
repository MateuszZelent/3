package main

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"
)

func TestQueueShutdownHelperProcess(t *testing.T) {
	if os.Getenv("MUMAX_QUEUE_SHUTDOWN_HELPER") != "1" {
		return
	}
	child := exec.Command("sleep", "60")
	err := child.Start()
	if err != nil {
		t.Fatal(err)
	}
	pidFile := os.Getenv("MUMAX_QUEUE_PID_FILE")
	if err := os.WriteFile(pidFile, []byte(fmt.Sprintf("%d\n%d\n", os.Getpid(), child.Process.Pid)), 0600); err != nil {
		t.Fatal(err)
	}
	signals := make(chan os.Signal, 1)
	signal.Notify(signals, syscall.SIGTERM, os.Interrupt)
	<-signals
	_ = child.Wait()
}

func TestQueueStopKillsWorkerProcessGroup(t *testing.T) {
	pidFile := filepath.Join(t.TempDir(), "pids")
	oldFactory := workerCommand
	defer func() { workerCommand = oldFactory }()
	workerCommand = func(ctx context.Context, args []string) *exec.Cmd {
		fullArgs := append([]string{"-test.run=^TestQueueShutdownHelperProcess$"}, args...)
		cmd := exec.CommandContext(ctx, os.Args[0], fullArgs...)
		cmd.Env = append(os.Environ(),
			"MUMAX_QUEUE_SHUTDOWN_HELPER=1",
			"MUMAX_QUEUE_PID_FILE="+pidFile,
		)
		return cmd
	}

	outcome := make(chan runOutcome, 1)
	go func() {
		outcome <- run(17, "shutdown-helper.mx3", 0, "",
			func(int) {}, func(string) {}, func(string) {}, func(error) {}, func(error) {}, func() {}, func(int) {}, func() {}, func() bool { return true })
	}()
	var pids []int
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		data, err := os.ReadFile(pidFile)
		if err == nil {
			for _, line := range strings.Split(strings.TrimSpace(string(data)), "\n") {
				if pid, parseErr := strconv.Atoi(strings.TrimSpace(line)); parseErr == nil {
					pids = append(pids, pid)
				}
			}
			if len(pids) == 2 {
				break
			}
		}
		time.Sleep(10 * time.Millisecond)
	}
	if len(pids) != 2 {
		t.Fatal("worker helper did not publish both PIDs")
	}
	stopRunning()
	select {
	case got := <-outcome:
		if got != runCancelled {
			t.Fatalf("outcome = %v, want runCancelled", got)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("worker did not stop after process-group shutdown")
	}
	for _, pid := range pids {
		deadline := time.Now().Add(2 * time.Second)
		for time.Now().Before(deadline) {
			if err := syscall.Kill(pid, 0); err != nil {
				break
			}
			time.Sleep(10 * time.Millisecond)
		}
		if err := syscall.Kill(pid, 0); err == nil {
			t.Fatalf("PID %d is still alive", pid)
		}
	}
}
