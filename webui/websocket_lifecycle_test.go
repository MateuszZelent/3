package webui

import (
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
