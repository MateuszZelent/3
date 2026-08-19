package main

import (
	"bytes"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/mumax/3/events"
)

func TestWorkerEventAddressUsesActualPortAndPath(t *testing.T) {
	addr, ok := workerEventAddress(events.Event{Event: "webui_ready", ListenHost: "127.0.0.1", ListenPort: 35369, BasePath: "/proxy/35369"})
	if !ok || addr != "127.0.0.1:35369/proxy/35369" {
		t.Fatalf("address = %q, ok = %v", addr, ok)
	}
	if _, ok := workerEventAddress(events.Event{Event: "webui_ready", ListenHost: "127.0.0.1", ListenPort: 65536}); ok {
		t.Fatal("accepted an out-of-range worker port")
	}
}

func TestWorkerTunnelArgumentSeparatesFixedPorts(t *testing.T) {
	got, err := workerTunnelArgument("proxy.example:35369", 2)
	if err != nil {
		t.Fatal(err)
	}
	if got != "proxy.example:35371" {
		t.Fatalf("tunnel argument = %q", got)
	}
	got, err = workerTunnelArgument("proxy.example:0", 2)
	if err != nil {
		t.Fatal(err)
	}
	if got != "proxy.example:0" {
		t.Fatalf("dynamic tunnel argument = %q", got)
	}
	if _, err := workerTunnelArgument("proxy.example:65535", 1); err == nil {
		t.Fatal("accepted an overflowing tunnel port")
	}
}

func TestExplicitPublicURLTemplateUsesActualPort(t *testing.T) {
	r := httptest.NewRequest("GET", "http://queue.example:35367/", nil)
	got := expandPublicURLTemplate(r, "https://public.example/proxy/{port}", "127.0.0.1:35369")
	if got != "https://public.example/proxy/35369/" {
		t.Fatalf("public URL = %q", got)
	}
}

func TestQueueStateAndTunnelReadiness(t *testing.T) {
	s := &stateTab{jobs: []job{{uid: 0, inFile: "job.mx3", state: JobQueued}}, tunnelRequired: true}
	j, ok := s.StartNext("127.0.0.1:35368")
	if !ok || j.state != JobStarting {
		t.Fatalf("started job = %+v, ok = %v", j, ok)
	}
	s.SetReady(0, "127.0.0.1:35369")
	var before bytes.Buffer
	s.RenderHTML(&before, httptest.NewRequest("GET", "http://queue:35367/", nil))
	if strings.Contains(before.String(), "href=") || !strings.Contains(before.String(), "waiting for tunnel_ready") {
		t.Fatalf("link became active before tunnel readiness: %s", before.String())
	}
	s.SetPublicURL(0, "https://proxy.example/35369/")
	var after bytes.Buffer
	s.RenderHTML(&after, httptest.NewRequest("GET", "http://queue:35367/", nil))
	if !strings.Contains(after.String(), `href="https://proxy.example/35369/"`) {
		t.Fatalf("missing tunnel URL: %s", after.String())
	}
	s.Finish(j, runSucceeded)
	if s.jobs[0].state != JobSucceeded {
		t.Fatalf("final state = %q", s.jobs[0].state)
	}
}

func TestUntrustedForwardedHeadersAreIgnored(t *testing.T) {
	r := httptest.NewRequest("GET", "http://queue.example:35367/", nil)
	r.RemoteAddr = "192.0.2.10:1234"
	r.Header.Set("Forwarded", "host=attacker.example;proto=https")
	if got := queueJobURL(r, "127.0.0.1:35369"); got != "http://queue.example:35369/" {
		t.Fatalf("URL = %q", got)
	}
}
