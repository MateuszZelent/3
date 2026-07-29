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
	if !strings.Contains(output.String(), "template [--flat] [--run]") {
		t.Fatalf("usage = %q", output.String())
	}
}
