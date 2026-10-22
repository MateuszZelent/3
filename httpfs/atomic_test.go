package httpfs

import (
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"testing"
)

func TestAtomicReplacementFailurePreservesFile(t *testing.T) {
	p := filepath.Join(t.TempDir(), "value")
	if err := PutAtomic(p, []byte("previous")); err != nil {
		t.Fatal(err)
	}
	err := ReplaceFile(p, func(tmp string) error {
		if err := os.WriteFile(tmp, []byte("partial"), 0600); err != nil {
			return err
		}
		return errors.New("injected error")
	})
	if err == nil {
		t.Fatal("expected error")
	}
	b, err := os.ReadFile(p)
	if err != nil || string(b) != "previous" {
		t.Fatalf("lost previous file: %q %v", b, err)
	}
	left, _ := filepath.Glob(filepath.Join(filepath.Dir(p), ".mumax-write-*"))
	if len(left) != 0 {
		t.Fatal("temporary files leaked")
	}
}

func TestAtomicRemoteProtocol(t *testing.T) {
	dir := t.TempDir()
	s := httptest.NewServer(newHandler(PUTATOMIC, func(name string, data []byte, w io.Writer, q url.Values) error {
		return handlePutAtomic(filepath.Join(dir, name), data, w, q)
	}))
	defer s.Close()
	if err := PutAtomic(s.URL+"/value", []byte("committed")); err != nil {
		t.Fatal(err)
	}
	b, err := os.ReadFile(filepath.Join(dir, "value"))
	if err != nil || string(b) != "committed" {
		t.Fatalf("remote file %q: %v", b, err)
	}
}

func TestAtomicRejectsOldServerFallback(t *testing.T) {
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { _, _ = io.WriteString(w, "old server status page") }))
	defer s.Close()
	if err := PutAtomic(s.URL+"/value", []byte("data")); err == nil {
		t.Fatal("unacknowledged remote write accepted")
	}
}

func TestBufferedWriterClosedState(t *testing.T) {
	w, err := Create(filepath.Join(t.TempDir(), "log"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err = w.Write([]byte("sample")); err != nil {
		t.Fatal(err)
	}
	if err = w.Close(); err != nil {
		t.Fatal(err)
	}
	if err = w.Close(); err != nil {
		t.Fatal(err)
	}
	if _, err = w.Write([]byte("late")); !errors.Is(err, io.ErrClosedPipe) {
		t.Fatal("write after close:", err)
	}
	if err = w.Flush(); !errors.Is(err, io.ErrClosedPipe) {
		t.Fatal("flush after close:", err)
	}
}
