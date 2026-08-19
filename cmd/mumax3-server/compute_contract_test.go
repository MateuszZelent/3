package main

import (
	"testing"

	"github.com/mumax/3/events"
)

func TestLegacyWorkerReadyUsesActualAddress(t *testing.T) {
	got, ok := parseWebUIReadyLine("//webui-ready 127.0.0.1:35371/proxy/35371")
	if !ok || got != "127.0.0.1:35371/proxy/35371" {
		t.Fatalf("address = %q, ok = %v", got, ok)
	}
	if _, ok := parseWebUIReadyLine("//webui-ready 127.0.0.1:0"); ok {
		t.Fatal("accepted port 0")
	}
}

func TestLegacyWorkerEventUsesActualAddress(t *testing.T) {
	got, ok := eventGUIAddress(events.Event{Event: "webui_ready", ListenHost: "127.0.0.1", ListenPort: 35372, BasePath: "/proxy/35372"})
	if !ok || got != "127.0.0.1:35372/proxy/35372" {
		t.Fatalf("address = %q, ok = %v", got, ok)
	}
}
