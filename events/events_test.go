package events

import (
	"bytes"
	"errors"
	"os"
	"strconv"
	"testing"
)

func TestEmitAndReadJSONLines(t *testing.T) {
	reader, writer, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	defer reader.Close()
	t.Setenv(EnvFD, strconv.FormatUint(uint64(writer.Fd()), 10))
	if err := Emit(Event{Event: "webui_ready", ListenHost: "127.0.0.1", ListenPort: 35369}); err != nil {
		t.Fatal(err)
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	var got Event
	if err := Read(reader, func(event Event) error { got = event; return nil }); err != nil {
		t.Fatal(err)
	}
	if got.Event != "webui_ready" || got.ListenPort != 35369 {
		t.Fatalf("event = %+v", got)
	}
}

func TestReadRejectsMalformedEvent(t *testing.T) {
	err := Read(bytes.NewBufferString(`{"event":`), nil)
	if err == nil {
		t.Fatal("malformed event was accepted")
	}
}

func TestReadPropagatesCallbackError(t *testing.T) {
	want := errors.New("stop")
	err := Read(bytes.NewBufferString(`{"event":"worker_failed"}`), func(Event) error { return want })
	if !errors.Is(err, want) {
		t.Fatalf("error = %v, want %v", err, want)
	}
}
