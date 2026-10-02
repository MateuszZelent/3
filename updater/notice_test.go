package updater

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

const noticeFixture = `{"tag_name":"v3.12.18-20261002","published_at":"2026-10-02T12:00:00Z","assets":[{"name":"mumax3","browser_download_url":"https://github.com/MateuszZelent/3/releases/download/v3.12.18-20261002/mumax3"}]}`

func TestNewerVersion(t *testing.T) {
	for _, tc := range []struct {
		latest, current string
		want            bool
	}{
		{"v3.12.18-20261002", "v3.12.17-20261002", true},
		{"v3.12.10", "v3.12.9", true},
		{"v3.12.17-20261003", "v3.12.17-20261002", true},
		{"v3.12.17-20261002", "v3.12.17-20261002", false},
		{"v3.12.17", "v3.13.0", false},
		{"v3.12.18", "development", false},
		{"v3.12.18-rc1", "v3.12.17", false},
	} {
		if got := newerVersion(tc.latest, tc.current); got != tc.want {
			t.Errorf("%s > %s: %v", tc.latest, tc.current, got)
		}
	}
}

func TestNoticeCacheAndExpiry(t *testing.T) {
	var requests atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests.Add(1)
		if r.Header.Get("User-Agent") == "" {
			t.Error("missing user agent")
		}
		w.Write([]byte(noticeFixture))
	}))
	defer server.Close()
	dir := t.TempDir()
	now := time.Now()
	for _, at := range []time.Time{now, now.Add(time.Hour), now.Add(noticeTTL + time.Second)} {
		if got := checkNotice(context.Background(), server.Client(), server.URL, dir, at); got.Tag != "v3.12.18-20261002" {
			t.Fatalf("release: %+v", got)
		}
	}
	if requests.Load() != 2 {
		t.Fatalf("requests=%d", requests.Load())
	}
}

func TestNoticeFailuresAreThrottled(t *testing.T) {
	for name, body := range map[string]string{"rate-limit": "HTTP403", "bad-json": "broken", "prerelease": strings.Replace(noticeFixture, `"tag_name":`, `"prerelease":true,"tag_name":`, 1), "draft": strings.Replace(noticeFixture, `"tag_name":`, `"draft":true,"tag_name":`, 1), "missing-asset": `{"tag_name":"v3.12.18"}`} {
		t.Run(name, func(t *testing.T) {
			var requests atomic.Int32
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				requests.Add(1)
				if body == "HTTP403" {
					w.WriteHeader(403)
				}
				w.Write([]byte(body))
			}))
			defer server.Close()
			dir := t.TempDir()
			now := time.Now()
			for i := 0; i < 2; i++ {
				if got := checkNotice(context.Background(), server.Client(), server.URL, dir, now); got.Tag != "" {
					t.Fatal(got)
				}
			}
			if requests.Load() != 1 {
				t.Fatalf("requests=%d", requests.Load())
			}
		})
	}
}

func TestNoticeConcurrentChecksDoNotWait(t *testing.T) {
	entered, release := make(chan struct{}), make(chan struct{})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		close(entered)
		<-release
		w.Write([]byte(noticeFixture))
	}))
	defer server.Close()
	dir := t.TempDir()
	now := time.Now()
	done := make(chan struct{})
	go func() { checkNotice(context.Background(), server.Client(), server.URL, dir, now); close(done) }()
	<-entered
	second := make(chan struct{})
	go func() { checkNotice(context.Background(), server.Client(), server.URL, dir, now); close(second) }()
	select {
	case <-second:
	case <-time.After(time.Second):
		close(release)
		t.Fatal("concurrent check blocked")
	}
	close(release)
	<-done
}

func TestNoticeCancellation(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { <-r.Context().Done() }))
	defer server.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	start := time.Now()
	checkNotice(ctx, server.Client(), server.URL, t.TempDir(), time.Now())
	if time.Since(start) > time.Second {
		t.Fatal("request ignored cancellation")
	}
}

func TestStartNoticeCachedAndCancelled(t *testing.T) {
	cache := t.TempDir()
	t.Setenv("XDG_CACHE_HOME", cache)
	dir := filepath.Join(cache, "mumax3", "release-notice")
	os.MkdirAll(dir, 0700)
	data, _ := json.Marshal(noticeCache{NextCheck: time.Now().Add(time.Hour), Release: Release{Tag: "v3.12.18-20261002", PublishedAt: time.Now()}})
	if err := os.WriteFile(filepath.Join(dir, "latest.json"), data, 0600); err != nil {
		t.Fatal(err)
	}
	messages := make(chan string, 1)
	start := time.Now()
	cancel := StartReleaseNotice("v3.12.17-20261002", func(s string) { messages <- s })
	defer cancel()
	if time.Since(start) > time.Second {
		t.Fatal("startup blocked")
	}
	select {
	case msg := <-messages:
		if !strings.Contains(msg, "mumax3 --update") || !strings.Contains(msg, "v3.12.18") {
			t.Fatal(msg)
		}
	case <-time.After(time.Second):
		t.Fatal("missing cached notice")
	}
	cancel = StartReleaseNotice("development", func(s string) { t.Error("development notice") })
	cancel()
}

func TestNoticeUnwritableCacheDoesNotQuery(t *testing.T) {
	var requests atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { requests.Add(1) }))
	defer server.Close()
	dir := filepath.Join(t.TempDir(), "file")
	os.WriteFile(dir, []byte("x"), 0600)
	checkNotice(context.Background(), server.Client(), server.URL, dir, time.Now())
	if requests.Load() != 0 {
		t.Fatal("bypassed cache throttle")
	}
}
