package main

import (
	"strings"
	"testing"
)

func TestChildArgsCanonicalizesSchedulerAssignments(t *testing.T) {
	args, err := childArgs(map[string]string{
		"g":                "7",
		"gpu":              "8",
		"webui-addr":       ":4000",
		"http":             ":4001",
		"debug":            "true",
		"t":                "proxy.example:35369",
		"max_gpus":         "8",
		"failfast":         "true",
		"webui-public-url": "https://public.example/{port}",
	}, workerAssignment{GPU: 2, WebAddr: "127.0.0.1:35371"})
	if err != nil {
		t.Fatal(err)
	}
	joined := strings.Join(args, " ")
	if !strings.Contains(joined, "-gpu=2") || !strings.Contains(joined, "-http=127.0.0.1:35371") {
		t.Fatalf("assignment was not authoritative: %v", args)
	}
	if strings.Contains(joined, "-gpu=7") || strings.Contains(joined, "-gpu=8") || strings.Contains(joined, "-http=:4000") || strings.Contains(joined, "-http=:4001") {
		t.Fatalf("scheduler values were overridden: %v", args)
	}
	if !strings.Contains(joined, "-debug=true") || !strings.Contains(joined, "-tunnel=proxy.example:35371") {
		t.Fatalf("ordinary flags were not passed canonically: %v", args)
	}
	for _, arg := range args {
		if strings.HasPrefix(arg, "-gpu=") && arg != "-gpu=2" {
			t.Fatalf("unexpected GPU argument %q in %v", arg, args)
		}
		if strings.HasPrefix(arg, "-http=") && arg != "-http=127.0.0.1:35371" {
			t.Fatalf("unexpected HTTP argument %q in %v", arg, args)
		}
	}
}

func TestValidateWorkerPortCapacityUsesActualGPUIDs(t *testing.T) {
	web, err := newQueueWebAddress("127.0.0.1:65534")
	if err != nil {
		t.Fatal(err)
	}
	if err := validateWorkerPortCapacity(web, []int{0}); err != nil {
		t.Fatalf("one worker at 65534 should fit: %v", err)
	}
	if err := validateWorkerPortCapacity(web, []int{0, 1}); err == nil {
		t.Fatal("accepted a worker port above 65535")
	}
}

func TestQueueSkippedStateIsTerminal(t *testing.T) {
	s := &stateTab{jobs: []job{{uid: 0, inFile: "already.zarr", state: JobStarting}}}
	s.SetSkipped(0)
	if s.jobs[0].state != JobSkipped {
		t.Fatalf("state = %q, want skipped", s.jobs[0].state)
	}
	s.Finish(s.jobs[0], runSkipped)
	if s.jobs[0].state != JobSkipped {
		t.Fatalf("finish changed skipped state to %q", s.jobs[0].state)
	}
}
