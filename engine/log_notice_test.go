package engine

import (
	"bytes"
	"strings"
	"sync"
	"testing"
	"unicode/utf8"
)

func TestConsoleHistoryIsBoundedWithUTF8TruncationMarker(t *testing.T) {
	history := ""
	for i := 0; i < 3; i++ {
		history = appendConsoleHistory(history, strings.Repeat("λ", consoleHistoryLimit/2))
	}
	if len(history) > consoleHistoryLimit || !utf8.ValidString(history) {
		t.Fatalf("history exceeds limit or split a UTF-8 rune: bytes=%d valid=%v", len(history), utf8.ValidString(history))
	}
	if !strings.HasPrefix(history, consoleHistoryTruncationMarker) {
		t.Fatalf("truncated history has no leading marker: %.64q", history)
	}
}

func TestConsoleHistoryBoundDoesNotTruncateFileBootstrap(t *testing.T) {
	logMu.Lock()
	savedHist, savedBootstrap, savedFile, savedClosed := hist, logBootstrap, logfile, logClosed
	hist, logBootstrap, logfile, logClosed = "", nil, nil, false
	for i := 0; i < 300; i++ {
		log2GUI(strings.Repeat("λ", 500))
	}
	if len(hist) > consoleHistoryLimit || !utf8.ValidString(hist) || !strings.HasPrefix(hist, consoleHistoryTruncationMarker) {
		logMu.Unlock()
		t.Fatalf("GUI history is not safely bounded: bytes=%d valid=%v", len(hist), utf8.ValidString(hist))
	}
	bootstrap := strings.Join(logBootstrap, "\n")
	if len(bootstrap) <= consoleHistoryLimit || strings.Count(bootstrap, "\n") != 299 {
		got := len(bootstrap)
		logMu.Unlock()
		t.Fatalf("pre-init file bootstrap was truncated: bytes=%d", got)
	}
	hist, logBootstrap, logfile, logClosed = savedHist, savedBootstrap, savedFile, savedClosed
	logMu.Unlock()
}

type noticeLogBuffer struct {
	bytes.Buffer
	closed     bool
	lateWrites bool
}

func (b *noticeLogBuffer) Close() error { b.closed = true; return nil }
func (b *noticeLogBuffer) Write(p []byte) (int, error) {
	if b.closed {
		b.lateWrites = true
	}
	return b.Buffer.Write(p)
}

func TestBackgroundNoticeLogging(t *testing.T) {
	logMu.Lock()
	savedHist, savedBootstrap, savedFile, savedClosed := hist, logBootstrap, logfile, logClosed
	hist = ""
	logBootstrap = nil
	buffer := &noticeLogBuffer{}
	logfile = buffer
	logClosed = false
	logMu.Unlock()
	defer func() {
		logMu.Lock()
		hist, logBootstrap, logfile, logClosed = savedHist, savedBootstrap, savedFile, savedClosed
		logMu.Unlock()
	}()
	var workers sync.WaitGroup
	for i := 0; i < 20; i++ {
		workers.Add(1)
		go func() { defer workers.Done(); LogReleaseNotice("new release"); _ = LogHistory() }()
	}
	workers.Wait()
	closeLog()
	before := LogHistory()
	LogReleaseNotice("late release")
	if LogHistory() != before || buffer.lateWrites {
		t.Fatal("notification logged after close")
	}
	if bytes.Count(buffer.Bytes(), []byte("new release")) != 20 {
		t.Fatal("missing or duplicate log entries")
	}
}
