// Package events defines the private parent-worker event protocol.
package events

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"strconv"
	"sync"
)

const EnvFD = "MUMAX_EVENT_FD"

type Event struct {
	Event      string `json:"event"`
	ListenHost string `json:"listen_host,omitempty"`
	ListenPort int    `json:"listen_port,omitempty"`
	BasePath   string `json:"base_path,omitempty"`
	PublicURL  string `json:"public_url,omitempty"`
	Error      string `json:"error,omitempty"`
}

var writer struct {
	sync.Mutex
	file *os.File
}

// Emit writes one JSON event to the inherited event descriptor, if present.
// Direct invocations without MUMAX_EVENT_FD remain fully compatible.
func Emit(event Event) error {
	fdText := os.Getenv(EnvFD)
	if fdText == "" {
		return nil
	}
	fd, err := strconv.Atoi(fdText)
	if err != nil || fd < 0 {
		return fmt.Errorf("invalid %s=%q", EnvFD, fdText)
	}
	writer.Lock()
	defer writer.Unlock()
	if writer.file == nil {
		writer.file = os.NewFile(uintptr(fd), "mumax-event")
		if writer.file == nil {
			return errors.New("cannot open inherited event descriptor")
		}
	}
	return json.NewEncoder(writer.file).Encode(event)
}

// Read consumes JSON Lines and invokes fn for every valid event.
func Read(r io.Reader, fn func(Event) error) error {
	scanner := bufio.NewScanner(r)
	scanner.Buffer(make([]byte, 4*1024), 256*1024)
	for scanner.Scan() {
		var event Event
		if err := json.Unmarshal(scanner.Bytes(), &event); err != nil {
			return fmt.Errorf("invalid worker event: %w", err)
		}
		if event.Event == "" {
			return errors.New("worker event has no event type")
		}
		if fn != nil {
			if err := fn(event); err != nil {
				return err
			}
		}
	}
	return scanner.Err()
}
