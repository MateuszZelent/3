package webui

import (
	"fmt"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gorilla/websocket"
	"github.com/labstack/echo/v4"

	"github.com/mumax/3/engine"
)

func TestWebSocketLifecycleCountsOnlyMainClients(t *testing.T) {
	wsManager := newWebSocketManager()
	wsManager.engineState = &EngineState{Preview: &PreviewState{}}
	e := echo.New()
	e.GET("/ws", func(c echo.Context) error {
		return wsManager.websocketEntrypointFor(c, wsManager.connections, "main", func() {})
	})
	e.GET("/ws/preview", func(c echo.Context) error {
		return wsManager.websocketEntrypointFor(c, wsManager.previewConnections, "preview", func() {})
	})
	server := httptest.NewServer(e)
	defer server.Close()

	dial := func(path string) *websocket.Conn {
		t.Helper()
		url := "ws" + strings.TrimPrefix(server.URL, "http") + path
		conn, _, err := websocket.DefaultDialer.Dial(url, nil)
		if err != nil {
			t.Fatal(err)
		}
		return conn
	}
	waitClients := func(want int) {
		t.Helper()
		deadline := time.Now().Add(time.Second)
		for time.Now().Before(deadline) {
			if got := engine.InteractiveActiveClients(); got == want {
				return
			}
			time.Sleep(time.Millisecond)
		}
		t.Fatalf("active clients = %d, want %d", engine.InteractiveActiveClients(), want)
	}

	baseline := engine.InteractiveActiveClients()
	mainConn := dial("/ws")
	waitClients(baseline + 1)

	previewConn := dial("/ws/preview")
	waitClients(baseline + 1)
	_ = previewConn.Close()
	waitClients(baseline + 1)

	_ = mainConn.Close()
	waitClients(baseline)
}

func TestPreviewCreditsKeyframesAndLatestOnly(t *testing.T) {
	manager := newWebSocketManager()
	cm := manager.previewConnections
	ready := make(chan *managedConnection, 1)
	e := echo.New()
	e.GET("/", func(c echo.Context) error {
		socket, err := manager.upgrader.Upgrade(c.Response(), c.Request(), nil)
		if err != nil {
			return err
		}
		defer socket.Close()
		connection := cm.add(socket)
		defer cm.remove(socket)
		connection.control([]byte(`{"protocol":2}`))
		ready <- connection
		for {
			_, data, err := socket.ReadMessage()
			if err != nil {
				return nil
			}
			connection.control(data)
		}
	})
	server := httptest.NewServer(e)
	defer server.Close()
	client, _, err := websocket.DefaultDialer.Dial("ws"+strings.TrimPrefix(server.URL, "http")+"/", nil)
	if err != nil {
		t.Fatal(err)
	}
	defer client.Close()
	connection := <-ready
	received := make(chan string, 10)
	go func() {
		for {
			_, data, err := client.ReadMessage()
			if err != nil {
				return
			}
			received <- string(data)
		}
	}()
	receive := func(want string) {
		t.Helper()
		select {
		case got := <-received:
			if got != want {
				t.Fatalf("got %q want %q", got, want)
			}
		case <-time.After(time.Second):
			t.Fatalf("timeout awaiting %q", want)
		}
	}
	send := func(seq, rev uint64) {
		cm.broadcastFrame(outboundFrame{full: []byte(fmt.Sprintf("full%d", seq)), delta: []byte(fmt.Sprintf("delta%d", seq)), sequence: seq, revision: rev})
	}
	ack := func(seq, rev uint64) {
		if err := client.WriteJSON(map[string]any{"ack": seq, "revision": rev}); err != nil {
			t.Fatal(err)
		}
	}
	send(1, 1)
	receive("full1")
	send(2, 2)
	send(3, 2)
	ack(99, 1) // A stale/wrong ACK must not release credit.
	select {
	case got := <-received:
		t.Fatalf("frame before matching ACK: %s", got)
	case <-time.After(50 * time.Millisecond):
	}
	ack(1, 1)
	receive("full3") // Missed topology must be repaired with a keyframe.
	ack(3, 2)
	send(4, 2)
	receive("delta4")
	if err := client.WriteJSON(map[string]any{"resync": true}); err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(time.Second)
	for {
		connection.controlMu.Lock()
		revision := connection.ackRevision
		connection.controlMu.Unlock()
		if revision == 0 {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("resync ignored")
		}
		time.Sleep(time.Millisecond)
	}
	send(5, 2)
	receive("full5")
}
