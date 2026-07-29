package updater

import (
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

type roundTripFunc func(*http.Request) (*http.Response, error)

func (function roundTripFunc) RoundTrip(request *http.Request) (*http.Response, error) {
	return function(request)
}

func clientReturning(status int, body string) *http.Client {
	return &http.Client{Transport: roundTripFunc(func(request *http.Request) (*http.Response, error) {
		return &http.Response{
			StatusCode: status,
			Status:     http.StatusText(status),
			Body:       io.NopCloser(strings.NewReader(body)),
			Request:    request,
		}, nil
	})}
}

func TestApplyToAtomicallyReplacesExecutable(t *testing.T) {
	executable := filepath.Join(t.TempDir(), "mumax3")
	if err := os.WriteFile(executable, []byte("old"), 0o751); err != nil {
		t.Fatal(err)
	}
	before, err := os.Stat(executable)
	if err != nil {
		t.Fatal(err)
	}
	if err := ApplyTo("https://example.invalid/mumax3", executable, clientReturning(http.StatusOK, "new-binary")); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(executable)
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != "new-binary" {
		t.Fatalf("binary = %q", data)
	}
	info, err := os.Stat(executable)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != before.Mode().Perm() {
		t.Fatalf("mode = %o, want %o", info.Mode().Perm(), before.Mode().Perm())
	}
}

func TestApplyToPreservesBinaryOnHTTPError(t *testing.T) {
	executable := filepath.Join(t.TempDir(), "mumax3")
	if err := os.WriteFile(executable, []byte("old"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := ApplyTo("https://example.invalid/mumax3", executable, clientReturning(http.StatusNotFound, "missing")); err == nil {
		t.Fatal("HTTP error was accepted")
	}
	data, _ := os.ReadFile(executable)
	if string(data) != "old" {
		t.Fatalf("binary changed to %q", data)
	}
}
