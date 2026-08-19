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
	"sync/atomic"
	"syscall"
	"testing"
	"time"
)

func TestQueueFailfastHelperProcess(t *testing.T) {
	if os.Getenv("MUMAX_QUEUE_FAILFAST_HELPER") != "1" {
		return
	}
	child := exec.Command("sleep", "60")
	if err := child.Start(); err != nil {
		t.Fatal(err)
	}
	pidFile := os.Getenv("MUMAX_QUEUE_FAILFAST_PID_FILE")
	if err := os.WriteFile(pidFile, []byte(fmt.Sprintf("%d\n%d\n", os.Getpid(), child.Process.Pid)), 0600); err != nil {
		t.Fatal(err)
	}
	signals := make(chan os.Signal, 1)
	signal.Notify(signals, syscall.SIGTERM, os.Interrupt)
	if os.Getenv("MUMAX_QUEUE_FAILFAST_MODE") == "fail" {
		ticker := time.NewTicker(10 * time.Millisecond)
		defer ticker.Stop()
		for {
			select {
			case <-signals:
				_ = child.Wait()
				return
			case <-ticker.C:
				if _, err := os.Stat(os.Getenv("MUMAX_QUEUE_FAILFAST_RELEASE")); err == nil {
					_ = child.Process.Signal(syscall.SIGTERM)
					_ = child.Wait()
					os.Exit(1)
				}
			}
		}
	}
	<-signals
	_ = child.Wait()
}

func TestQueueFailfastKillsAllWorkerProcessGroups(t *testing.T) {
	oldFactory := workerCommand
	oldFailfast := *flag_failfast
	defer func() {
		workerCommand = oldFactory
		*flag_failfast = oldFailfast
	}()
	*flag_failfast = true

	tempDir := t.TempDir()
	releaseFile := filepath.Join(tempDir, "release")
	failPIDFile := filepath.Join(tempDir, "fail.pids")
	sleepPIDFile := filepath.Join(tempDir, "sleep.pids")
	var stopRequested atomic.Bool

	workerCommand = func(ctx context.Context, args []string) *exec.Cmd {
		mode := "sleep"
		if len(args) > 0 && strings.HasSuffix(args[len(args)-1], "fail.mx3") {
			mode = "fail"
		}
		pidFile := sleepPIDFile
		if mode == "fail" {
			pidFile = failPIDFile
		}
		fullArgs := append([]string{"-test.run=^TestQueueFailfastHelperProcess$"}, args...)
		cmd := exec.CommandContext(ctx, os.Args[0], fullArgs...)
		cmd.Env = append(os.Environ(),
			"MUMAX_QUEUE_FAILFAST_HELPER=1",
			"MUMAX_QUEUE_FAILFAST_MODE="+mode,
			"MUMAX_QUEUE_FAILFAST_PID_FILE="+pidFile,
			"MUMAX_QUEUE_FAILFAST_RELEASE="+releaseFile,
		)
		return cmd
	}
	t.Cleanup(func() {
		_ = os.WriteFile(releaseFile, []byte("cleanup"), 0600)
		stopRunning()
	})

	type result struct {
		file    string
		outcome runOutcome
	}
	results := make(chan result, 2)
	runWorker := func(uid int, file string, cancelled func() bool, onFailed func(error)) {
		results <- result{file: file, outcome: run(uid, file, 0, "",
			func(int) {}, func(string) {}, func(string) {}, onFailed, func(error) {}, func() {}, func(int) {}, func() {}, cancelled)}
	}
	go runWorker(31, "fail.mx3", func() bool { return false }, func(error) {
		stopRequested.Store(true)
	})
	go runWorker(32, "sleep.mx3", stopRequested.Load, func(error) {})

	readPIDs := func(path string) []int {
		data, err := os.ReadFile(path)
		if err != nil {
			return nil
		}
		var pids []int
		for _, line := range strings.Split(strings.TrimSpace(string(data)), "\n") {
			pid, err := strconv.Atoi(strings.TrimSpace(line))
			if err == nil {
				pids = append(pids, pid)
			}
		}
		return pids
	}
	deadline := time.Now().Add(5 * time.Second)
	var pids []int
	for time.Now().Before(deadline) {
		failPIDs := readPIDs(failPIDFile)
		sleepPIDs := readPIDs(sleepPIDFile)
		if len(failPIDs) == 2 && len(sleepPIDs) == 2 {
			pids = append(failPIDs, sleepPIDs...)
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	if len(pids) != 4 {
		t.Fatal("failfast helpers did not publish both process groups")
	}
	if err := os.WriteFile(releaseFile, []byte("release"), 0600); err != nil {
		t.Fatal(err)
	}

	var outcomes []result
	for i := 0; i < 2; i++ {
		select {
		case outcome := <-results:
			outcomes = append(outcomes, outcome)
		case <-time.After(10 * time.Second):
			t.Fatal("failfast workers did not finish")
		}
	}
	var failed, cancelled bool
	for _, outcome := range outcomes {
		switch outcome.outcome {
		case runFailed:
			failed = true
		case runCancelled:
			cancelled = true
		}
	}
	if !failed || !cancelled {
		t.Fatalf("outcomes = %+v, want one failed and one cancelled", outcomes)
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
			t.Fatalf("PID %d is still alive after failfast", pid)
		}
	}
}
