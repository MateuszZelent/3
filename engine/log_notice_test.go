package engine

import (
	"bytes"
	"sync"
	"testing"
)

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
	savedHist, savedFile, savedClosed := hist, logfile, logClosed
	hist = ""
	buffer := &noticeLogBuffer{}
	logfile = buffer
	logClosed = false
	logMu.Unlock()
	defer func() { logMu.Lock(); hist, logfile, logClosed = savedHist, savedFile, savedClosed; logMu.Unlock() }()
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
