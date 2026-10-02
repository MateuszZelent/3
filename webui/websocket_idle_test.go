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

// Leave the engine's injection queue unserved: idle publication must never
// ask the solver to read a field, download CUDA memory or build a snapshot.
func assertNoIdleEngineWork(t *testing.T, manager *WebSocketManager) {
	t.Helper()
	done := make(chan struct{})
	go func() {
		manager.broadcastTick()
		manager.broadcastPreviewState()
		manager.broadcastEngineStateWithoutPreview()
		close(done)
	}()
	select {
	case <-engine.Inject:
		t.Fatal("publication without recipients injected work into the solver")
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("idle publication did not return")
	}
	if manager.engineState.Preview.Sequence != 0 || !manager.engineState.Preview.Refresh {
		t.Fatal("idle publication captured or consumed a pending preview refresh")
	}
	if !manager.lastMainBroadcast.IsZero() || !manager.lastPreviewBroadcast.IsZero() {
		t.Fatal("idle publication advanced broadcast timestamps")
	}
}

func TestIdlePublicationDoesNotCaptureOrPublish(t *testing.T) {
	manager := newWebSocketManager()
	// Other sections deliberately remain nil: updating the main snapshot would
	// panic, independently of the preview's engine injection assertion.
	manager.engineState = &EngineState{Preview: &PreviewState{Refresh: true}}
	assertNoIdleEngineWork(t, manager)
	for _, test := range []struct {
		name       string
		connection *managedConnection
	}{
		{"unsubscribed", &managedConnection{subscribed: false}},
		{"awaiting-render-ack", &managedConnection{subscribed: true, waiting: true}},
	} {
		t.Run(test.name, func(t *testing.T) {
			socket := &websocket.Conn{}
			manager.previewConnections.conns[socket] = test.connection
			assertNoIdleEngineWork(t, manager)
			delete(manager.previewConnections.conns, socket)
		})
	}
}

func TestBroadcastSleepsWithoutClientsAndResumesAfterReconnect(t *testing.T) {
	manager := newWebSocketManager()
	e := echo.New()
	e.GET("/", func(c echo.Context) error {
		return manager.websocketEntrypointFor(c, manager.previewConnections, "preview", func() {})
	})
	server := httptest.NewServer(e)
	defer server.Close()
	wait := func() chan bool {
		result := make(chan bool, 1)
		go func() { result <- manager.waitForBroadcastClients() }()
		return result
	}
	assertSleeping := func(result chan bool) {
		t.Helper()
		select {
		case <-result:
			t.Fatal("broadcast loop woke with no clients")
		case <-time.After(30 * time.Millisecond):
		}
	}
	for i := 0; i < 2; i++ {
		result := wait()
		assertSleeping(result)
		client, _, err := websocket.DefaultDialer.Dial("ws"+strings.TrimPrefix(server.URL, "http")+"/", nil)
		if err != nil {
			t.Fatal(err)
		}
		select {
		case ok := <-result:
			if !ok {
				t.Fatal("broadcast stopped instead of resuming")
			}
		case <-time.After(time.Second):
			t.Fatal("connection did not wake broadcaster")
		}
		client.Close()
		deadline := time.Now().Add(time.Second)
		for manager.previewConnections.count() != 0 {
			if time.Now().After(deadline) {
				t.Fatal("client was not removed")
			}
			time.Sleep(time.Millisecond)
		}
		manager.engineState = &EngineState{Preview: &PreviewState{Refresh: true}}
		assertNoIdleEngineWork(t, manager)
	}
	result := wait()
	assertSleeping(result)
	close(manager.broadcastStop)
	select {
	case ok := <-result:
		if ok {
			t.Fatal("stop resumed broadcasting")
		}
	case <-time.After(time.Second):
		t.Fatal("idle broadcaster ignored shutdown")
	}
}

func TestMetricsAreLazyWithoutMainClients(t *testing.T) {
	manager := newWebSocketManager()
	e := echo.New()
	metrics := initMetricsAPI(e.Group(""), manager)
	metrics.Update()
	metrics.collector.mu.Lock()
	defer metrics.collector.mu.Unlock()
	if metrics.collector.busy || metrics.collector.result != nil || !metrics.lastGPUUpdate.IsZero() {
		t.Fatal("metrics collector started without a UI client")
	}
}

func TestQueuedPreviewCaptureIsSkippedAfterDisconnect(t *testing.T) {
	manager := newWebSocketManager()
	socket := &websocket.Conn{}
	manager.previewConnections.conns[socket] = &managedConnection{subscribed: true}
	preview := &PreviewState{ws: manager, Refresh: true}
	done := make(chan struct{})
	go func() { preview.Update(); close(done) }()
	select {
	case task := <-engine.Inject:
		// Simulate disconnect while the busy solver has a capture pending.
		manager.previewConnections.mu.Lock()
		delete(manager.previewConnections.conns, socket)
		manager.previewConnections.mu.Unlock()
		task()
	case <-time.After(time.Second):
		t.Fatal("capture was not queued")
	}
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("cancelled capture did not return")
	}
	if preview.Sequence != 0 || preview.Timestamp != 0 || !preview.Refresh {
		t.Fatal("capture continued after its last recipient disconnected")
	}
}

func TestQueuedParameterRefreshIsSkippedWithoutClients(t *testing.T) {
	manager := newWebSocketManager()
	parameters := &ParametersState{ws: manager, Regions: []int{23}}
	done := make(chan struct{})
	go func() { parameters.Update(); close(done) }()
	select {
	case task := <-engine.Inject:
		task()
	case <-time.After(time.Second):
		t.Fatal("parameter refresh was not queued")
	}
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("cancelled parameter refresh did not return")
	}
	if len(parameters.Regions) != 1 || parameters.Regions[0] != 23 {
		t.Fatal("parameter refresh read regions without recipients")
	}
}
