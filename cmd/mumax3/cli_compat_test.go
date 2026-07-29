package main

import (
	"bytes"
	"flag"
	"strings"
	"testing"
)

func TestAmumaxCLIFlagsAreRegistered(t *testing.T) {
	for _, name := range []string{
		"d", "debug", "v", "version", "vet", "u", "update", "c", "cache",
		"g", "gpu", "i", "interactive", "o", "output-dir", "p", "paranoid",
		"s", "silent", "sync", "f", "force-clean", "skip-exist", "hide-progress-bar",
		"t", "tunnel", "insecure", "fft", "storage-format", "webui-disable",
		"webui-addr", "webui-queue-addr",
	} {
		if flag.Lookup(name) == nil {
			t.Errorf("CLI flag %q is missing", name)
		}
	}
	if flag.Lookup("new-engine") != nil {
		t.Fatal("new-engine must not be migrated")
	}
}

func TestUsageMentionsTemplateCommand(t *testing.T) {
	var output bytes.Buffer
	previous := flag.CommandLine.Output()
	flag.CommandLine.SetOutput(&output)
	defer flag.CommandLine.SetOutput(previous)
	flag.Usage()
	usage := output.String()
	if !strings.Contains(usage, "template [--flat] [--run]") {
		t.Fatalf("usage = %q", output.String())
	}
	if !strings.Contains(usage, "mumax3 build: version=") || !strings.Contains(usage, " commit=") || !strings.Contains(usage, " built=") {
		t.Fatalf("usage does not contain build identity: %q", usage)
	}
}

func TestBuildSummaryIncludesInjectedIdentity(t *testing.T) {
	oldVersion, oldCommit, oldDate := buildVersion, commitHash, buildDate
	defer func() { buildVersion, commitHash, buildDate = oldVersion, oldCommit, oldDate }()
	buildVersion, commitHash, buildDate = "v3.11.2", "abcdef12", "2026-07-29T13:30:00Z"
	want := "mumax3 build: version=v3.11.2 commit=abcdef12 built=2026-07-29T13:30:00Z"
	if got := buildSummary(); got != want {
		t.Fatalf("buildSummary() = %q, want %q", got, want)
	}
}
