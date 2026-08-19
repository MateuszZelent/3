package main

import (
	"bytes"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"
)

func TestSelectQueueGPUs(t *testing.T) {
	tests := []struct {
		name        string
		deviceCount int
		explicitGPU bool
		gpu         int
		maxGPUs     int
		want        []int
		wantErr     bool
	}{
		{name: "all", deviceCount: 4, want: []int{0, 1, 2, 3}},
		{name: "limited", deviceCount: 4, maxGPUs: 2, want: []int{0, 1}},
		{name: "limit above available", deviceCount: 2, maxGPUs: 8, want: []int{0, 1}},
		{name: "explicit GPU", deviceCount: 4, explicitGPU: true, gpu: 3, maxGPUs: 2, want: []int{3}},
		{name: "negative limit", deviceCount: 4, maxGPUs: -1, wantErr: true},
		{name: "invalid explicit GPU", deviceCount: 2, explicitGPU: true, gpu: 2, wantErr: true},
		{name: "no GPUs", deviceCount: 0, wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := selectQueueGPUs(tt.deviceCount, tt.explicitGPU, tt.gpu, tt.maxGPUs)
			if (err != nil) != tt.wantErr {
				t.Fatalf("error = %v, wantErr = %v", err, tt.wantErr)
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Fatalf("GPU IDs = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestQueueWebAddress(t *testing.T) {
	web, err := newQueueWebAddress("0.0.0.0:35367/proxy/35367")
	if err != nil {
		t.Fatal(err)
	}
	if got, want := web.listenAddress(), "0.0.0.0:35367"; got != want {
		t.Fatalf("listen address = %q, want %q", got, want)
	}
	if got, want := web.jobAddress(1), "0.0.0.0:35369/proxy/35369"; got != want {
		t.Fatalf("second worker address = %q, want %q", got, want)
	}

	if _, err := newQueueWebAddress("127.0.0.1:35367/proxy/queue"); err == nil {
		t.Fatal("accepted a proxy queue path that cannot identify worker routes")
	}
}

func TestQueueJobURL(t *testing.T) {
	for _, tt := range []struct {
		name        string
		requestPath string
		host        string
		headers     map[string]string
		webAddr     string
		want        string
	}{
		{
			name:        "localhost",
			requestPath: "/",
			host:        "localhost:35367",
			webAddr:     "127.0.0.1:35368",
			want:        "http://localhost:35368/",
		},
		{
			name:        "LAN address",
			requestPath: "/",
			host:        "192.168.1.20:35367",
			webAddr:     "0.0.0.0:35368",
			want:        "http://192.168.1.20:35368/",
		},
		{
			name:        "reverse proxy forwarded origin and prefix",
			requestPath: "/",
			host:        "127.0.0.1:35367",
			headers: map[string]string{
				"Forwarded":          "for=192.0.2.10;host=sim.example.org;proto=https",
				"X-Forwarded-Prefix": "/mumax/35367",
			},
			webAddr: "127.0.0.1:35368",
			want:    "https://sim.example.org/mumax/35368/",
		},
		{
			name:        "configured proxy path",
			requestPath: "/proxy/35367",
			host:        "cluster.example.org",
			webAddr:     "127.0.0.1:35368/proxy/35368",
			want:        "http://cluster.example.org/proxy/35368/",
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			r := httptest.NewRequest("GET", "http://"+tt.host+tt.requestPath, nil)
			r.Host = tt.host
			if tt.name == "reverse proxy forwarded origin and prefix" {
				r.RemoteAddr = "127.0.0.1:1234"
			}
			for key, value := range tt.headers {
				r.Header.Set(key, value)
			}
			if got := queueJobURL(r, tt.webAddr); got != tt.want {
				t.Fatalf("queue job URL = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestQueueHTMLUsesPublicLinkAndEscapesJobName(t *testing.T) {
	s := NewStateTab([]string{`<script>alert("bad")</script>.mx3`})
	s.jobs[0].webAddr = "127.0.0.1:35368"
	s.jobs[0].state = JobReady
	r := httptest.NewRequest("GET", "http://localhost:35367", nil)
	var output bytes.Buffer
	s.RenderHTML(&output, r)
	html := output.String()
	if !strings.Contains(html, `href="http://localhost:35368/"`) {
		t.Fatalf("missing public worker link in %q", html)
	}
	if strings.Contains(html, "<script>") || !strings.Contains(html, "&lt;script&gt;") {
		t.Fatalf("job name was not HTML escaped: %q", html)
	}
}
