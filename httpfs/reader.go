package httpfs

// Utility functions on top of standard httpfs protocol

import (
	"bufio"
	"bytes"
	"io"
	"sync"
)

const BUFSIZE = 16 * 1024 * 1024 // bufio buffer size

// create a file for writing, clobbers previous content if any.
func Create(URL string) (WriteCloseFlusher, error) {
	if err := Remove(URL); err != nil {
		return nil, err
	}
	err := Touch(URL)
	if err != nil {
		return nil, err
	}
	return &bufWriter{buf: bufio.NewWriterSize(&appendWriter{URL, 0}, BUFSIZE)}, nil
}

func MustCreate(URL string) WriteCloseFlusher {
	f, err := Create(URL)
	if err != nil {
		panic(err)
	}
	return f
}

type WriteCloseFlusher interface {
	io.WriteCloser
	Flush() error
}

// open a file for reading
func Open(URL string) (io.ReadCloser, error) {
	data, err := Read(URL)
	if err != nil {
		return nil, err
	}
	return io.NopCloser(bytes.NewReader(data)), nil
}

func MustOpen(URL string) io.ReadCloser {
	f, err := Open(URL)
	if err != nil {
		panic(err)
	}
	return f
}

type bufWriter struct {
	mu       sync.Mutex
	buf      *bufio.Writer
	closed   bool
	closeErr error
}

func (w *bufWriter) Write(p []byte) (int, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.closed {
		return 0, io.ErrClosedPipe
	}
	return w.buf.Write(p)
}
func (w *bufWriter) Close() error {
	w.mu.Lock()
	defer w.mu.Unlock()
	if !w.closed {
		w.closeErr = w.buf.Flush()
		w.closed = true
	}
	return w.closeErr
}
func (w *bufWriter) Flush() error {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.closed {
		return io.ErrClosedPipe
	}
	return w.buf.Flush()
}

type appendWriter struct {
	URL       string
	byteCount int64
}

func (w *appendWriter) Write(p []byte) (int, error) {
	err := AppendSize(w.URL, p, w.byteCount)
	if err != nil {
		return 0, err // don't know how many bytes written
	}
	w.byteCount += int64(len(p))
	return len(p), nil
}
