package webui

import (
	"encoding/json"
	"fmt"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/gorilla/websocket"
	"github.com/labstack/echo/v4"
	"github.com/vmihailenco/msgpack/v5"

	"github.com/mumax/3/engine"
)

func TestPreviewMutationReturnsVersionedAckWithoutCapturingInline(t *testing.T) {
	manager := newWebSocketManager()
	preview := &PreviewState{ws: manager, MaxPoints: 131072}
	manager.engineState = &EngineState{Preview: preview}
	e := echo.New()
	req := httptest.NewRequest("POST", "/api/preview/maxpoints", strings.NewReader(`{"maxPoints":64000,"requestId":"request-7"}`))
	req.Header.Set("Content-Type", "application/json")
	recorder := httptest.NewRecorder()
	c := e.NewContext(req, recorder)
	if err := preview.postPreviewMaxPoints(c); err != nil {
		t.Fatal(err)
	}
	var ack previewMutationAck
	if err := json.Unmarshal(recorder.Body.Bytes(), &ack); err != nil {
		t.Fatal(err)
	}
	if recorder.Code != 200 || ack.RequestID != "request-7" || ack.ConfigRevision != 1 {
		t.Fatalf("wrong mutation ack: status=%d ack=%+v body=%s", recorder.Code, ack, recorder.Body.String())
	}
	if preview.ConfigRevision != ack.ConfigRevision || !preview.Refresh || preview.MaxPoints != 64000 {
		t.Fatalf("mutation and ack revision differ: preview revision=%d refresh=%v max=%d ack=%+v", preview.ConfigRevision, preview.Refresh, preview.MaxPoints, ack)
	}
	select {
	case <-engine.Inject:
		t.Fatal("preview mutation synchronously queued engine capture")
	default:
	}
}

func TestPreviewWorkerDiscardsStaleSnapshotAndCoalescesRefreshes(t *testing.T) {
	manager := newWebSocketManager()
	t.Cleanup(func() { close(manager.broadcastStop) })
	preview := &PreviewState{ws: manager, Type: "2D", Refresh: true, ConfigRevision: 1}
	manager.engineState = &EngineState{Preview: preview}
	socket := &websocket.Conn{}
	managed := &managedConnection{subscribed: true, budget: 100, send: make(chan outboundFrame, 1)}
	manager.previewConnections.conns[socket] = managed
	started, release := make(chan struct{}), make(chan struct{})
	var captures atomic.Int32
	manager.previewCaptureForTest = func(s *PreviewState) {
		if captures.Add(1) == 1 {
			close(started)
			<-release
		}
		s.Sequence++
		s.ScalarField = [][3]float32{{1, 2, 3}}
		s.DataPointsCount = 1
		s.ServerDataPointsCount = 1
	}
	manager.startPreviewWorker()
	manager.requestPreviewWork(false)
	select {
	case <-started:
	case <-time.After(time.Second):
		t.Fatal("preview worker did not start capture")
	}
	updateReq := httptest.NewRequest("POST", "/api/preview/maxpoints", strings.NewReader(`{"maxPoints":64000,"requestId":"new-request"}`))
	updateReq.Header.Set("Content-Type", "application/json")
	updateRecorder := httptest.NewRecorder()
	updateDone := make(chan error, 1)
	go func() { updateDone <- preview.postPreviewMaxPoints(echo.New().NewContext(updateReq, updateRecorder)) }()
	select {
	case err := <-updateDone:
		if err != nil || updateRecorder.Code != 200 {
			t.Fatalf("configuration mutation blocked or failed during engine capture: status=%d err=%v", updateRecorder.Code, err)
		}
	case <-time.After(250 * time.Millisecond):
		t.Fatal("configuration mutation waited for preview engine capture")
	}
	if !manager.stateMu.TryLock() {
		t.Fatal("preview capture still holds the global state lock")
	}
	manager.stateMu.Unlock()
	for i := 0; i < 20; i++ {
		manager.requestPreviewWork(false)
	}
	close(release)
	var frame outboundFrame
	select {
	case frame = <-managed.send:
	case <-time.After(time.Second):
		t.Fatal("latest preview frame was not published")
	}
	var decoded PreviewState
	if err := msgpack.Unmarshal(frame.full, &decoded); err != nil {
		t.Fatal(err)
	}
	if captures.Load() != 2 {
		t.Fatalf("refreshes were not coalesced, captures=%d", captures.Load())
	}
	if decoded.ConfigRevision != 2 || decoded.MaxPoints != 64000 || preview.Sequence != 1 {
		t.Fatalf("stale snapshot was published: decoded rev=%d maxPoints=%d published rev=%d sequence=%d", decoded.ConfigRevision, decoded.MaxPoints, preview.ConfigRevision, preview.Sequence)
	}
}

func TestPreviewStepCheckDoesNotRebroadcastUnchangedSnapshot(t *testing.T) {
	manager := newWebSocketManager()
	t.Cleanup(func() { close(manager.broadcastStop) })
	preview := &PreviewState{ws: manager, Type: "2D", Sequence: 7, ConfigRevision: 3}
	manager.engineState = &EngineState{Preview: preview}
	manager.lastPreviewSnapshot = clonePreviewState(preview)
	connection := &managedConnection{subscribed: true, send: make(chan outboundFrame, 1)}
	manager.previewConnections.conns[&websocket.Conn{}] = connection
	var captures atomic.Int32
	manager.previewCaptureForTest = func(*PreviewState) { captures.Add(1) }
	manager.startPreviewWorker()
	manager.requestPreviewStepCheck()
	deadline := time.Now().Add(time.Second)
	for manager.previewWorkScheduled() && time.Now().Before(deadline) {
		time.Sleep(time.Millisecond)
	}
	if manager.previewWorkScheduled() {
		t.Fatal("preview step check did not finish")
	}
	if captures.Load() != 1 {
		t.Fatalf("expected one lightweight capture check, got %d", captures.Load())
	}
	select {
	case frame := <-connection.send:
		t.Fatalf("unchanged preview snapshot was rebroadcast: %d bytes", len(frame.full))
	default:
	}
}

func TestPreviewMeshChangeAdvancesConfigRevisionWithMatchingFrame(t *testing.T) {
	manager := newWebSocketManager()
	t.Cleanup(func() { close(manager.broadcastStop) })
	oldMesh := &MeshState{Nx: 8, Ny: 6, Nz: 2, Dx: 1e-9, Dy: 2e-9, Dz: 3e-9}
	newMesh := &MeshState{Nx: 9, Ny: 6, Nz: 2, Dx: 1e-9, Dy: 2e-9, Dz: 3e-9}
	preview := &PreviewState{ws: manager, Type: "2D", Sequence: 1, ConfigRevision: 7, Refresh: true, Mesh: oldMesh}
	manager.engineState = &EngineState{Preview: preview}
	manager.lastPreviewSnapshot = clonePreviewState(preview)
	connection := &managedConnection{subscribed: true, send: make(chan outboundFrame, 1)}
	manager.previewConnections.conns[&websocket.Conn{}] = connection
	manager.previewCaptureForTest = func(state *PreviewState) {
		state.Sequence++
		mesh := *newMesh
		state.Mesh = &mesh
	}
	manager.startPreviewWorker()
	manager.requestPreviewWork(false)

	select {
	case frame := <-connection.send:
		var decoded PreviewState
		if err := msgpack.Unmarshal(frame.full, &decoded); err != nil {
			t.Fatal(err)
		}
		if decoded.ConfigRevision != 8 || decoded.Mesh == nil || !samePreviewMesh(decoded.Mesh, newMesh) {
			t.Fatalf("mesh change did not invalidate and match the preview revision: revision=%d mesh=%+v", decoded.ConfigRevision, decoded.Mesh)
		}
	case <-time.After(time.Second):
		t.Fatal("mesh-change preview was not published")
	}
	if preview.ConfigRevision != 8 {
		t.Fatalf("live preview revision=%d, want 8", preview.ConfigRevision)
	}
}

func TestMainSnapshotEngineWaitDoesNotHoldStateLockOrPublishStaleConfig(t *testing.T) {
	manager := newWebSocketManager()
	t.Cleanup(func() { close(manager.broadcastStop) })
	preview := &PreviewState{ws: manager, MaxPoints: 131072}
	params := &ParametersState{ws: manager, Regions: []int{0}}
	manager.engineState = &EngineState{
		Header:    &HeaderState{},
		Console:   &ConsoleState{},
		Preview:   preview,
		Solver:    &SolverState{},
		Mesh:      &MeshState{},
		Params:    params,
		TablePlot: &TablePlotState{MaxPoints: 10, Step: 1, XColumn: "t", YColumn: "mx"},
		Metrics:   &MetricsState{ws: manager, Error: "test collector disabled"},
		Fft:       &FftState{},
	}
	connection := &managedConnection{subscribed: true, send: make(chan outboundFrame, 2)}
	manager.connections.conns[&websocket.Conn{}] = connection
	started, release := make(chan struct{}), make(chan struct{})
	secondStarted, secondRelease := make(chan struct{}), make(chan struct{})
	var releaseOnce, secondReleaseOnce sync.Once
	t.Cleanup(func() {
		releaseOnce.Do(func() { close(release) })
		secondReleaseOnce.Do(func() { close(secondRelease) })
	})
	var updates atomic.Int32
	manager.mainUpdateForTest = func(*EngineState) {
		switch updates.Add(1) {
		case 1:
			close(started)
			<-release
		case 2:
			close(secondStarted)
			<-secondRelease
		}
	}
	manager.requestMainSnapshot()
	select {
	case <-started:
	case <-time.After(time.Second):
		t.Fatal("main snapshot did not start its simulated engine refresh")
	}
	if !manager.stateMu.TryLock() {
		t.Fatal("main engine wait holds the global state lock")
	}
	manager.stateMu.Unlock()

	updateReq := httptest.NewRequest("POST", "/api/preview/maxpoints", strings.NewReader(`{"maxPoints":64000,"requestId":"fast-request"}`))
	updateReq.Header.Set("Content-Type", "application/json")
	updateRecorder := httptest.NewRecorder()
	updateDone := make(chan error, 1)
	go func() { updateDone <- preview.postPreviewMaxPoints(echo.New().NewContext(updateReq, updateRecorder)) }()
	select {
	case err := <-updateDone:
		if err != nil || updateRecorder.Code != 200 {
			t.Fatalf("preview config update did not finish during main engine wait: status=%d err=%v", updateRecorder.Code, err)
		}
	case <-time.After(250 * time.Millisecond):
		t.Fatal("preview config update blocked behind main engine wait")
	}

	manager.stateMu.Lock()
	params.SelectedRegion = 7
	manager.broadcastEngineStateWithoutPreviewLocked()
	manager.stateMu.Unlock()
	releaseOnce.Do(func() { close(release) })
	select {
	case <-secondStarted:
	case <-time.After(time.Second):
		t.Fatal("latest main snapshot did not start")
	}
	select {
	case frame := <-connection.send:
		t.Fatalf("stale main snapshot was published before latest refresh: %d bytes", len(frame.full))
	case <-time.After(20 * time.Millisecond):
	}
	secondReleaseOnce.Do(func() { close(secondRelease) })
	var frame outboundFrame
	select {
	case frame = <-connection.send:
	case <-time.After(time.Second):
		t.Fatal("latest main snapshot was not published")
	}
	var decoded struct {
		Parameters *ParametersState `msgpack:"parameters"`
	}
	if err := msgpack.Unmarshal(frame.full, &decoded); err != nil {
		t.Fatal(err)
	}
	if decoded.Parameters == nil || decoded.Parameters.SelectedRegion != 7 {
		t.Fatalf("published stale main configuration: %+v", decoded.Parameters)
	}
}

func TestMainSnapshotPublishesUpdatedStateForHTTPHandlers(t *testing.T) {
	manager := newWebSocketManager()
	t.Cleanup(func() { close(manager.broadcastStop) })
	metrics := &MetricsState{ws: manager}
	tablePlot := &TablePlotState{MaxPoints: 10, Step: 1, XColumn: "t", YColumn: "mx"}
	manager.engineState = &EngineState{
		Header:    &HeaderState{},
		Console:   &ConsoleState{},
		Preview:   &PreviewState{ws: manager},
		Solver:    &SolverState{},
		Mesh:      &MeshState{},
		Params:    &ParametersState{ws: manager},
		TablePlot: tablePlot,
		Metrics:   metrics,
		Fft:       &FftState{},
	}
	connection := &managedConnection{subscribed: true, send: make(chan outboundFrame, 2)}
	manager.connections.conns[&websocket.Conn{}] = connection
	refreshed := time.Unix(1700000000, 0)
	manager.mainUpdateForTest = func(state *EngineState) {
		state.TablePlot.previewColumns = []string{"t", "mx", "E_total"}
		state.Metrics.lastGPUUpdate = refreshed
	}
	manager.requestMainSnapshot()
	select {
	case <-connection.send:
	case <-time.After(time.Second):
		t.Fatal("main snapshot was not published")
	}

	manager.stateMu.Lock()
	defer manager.stateMu.Unlock()
	if !tablePlot.ColumnExists("E_total") {
		t.Fatalf("table column added by the snapshot is unknown to handlers: %v", tablePlot.previewColumns)
	}
	if !metrics.lastGPUUpdate.Equal(refreshed) {
		t.Fatalf("metrics refresh time was not published: %v", metrics.lastGPUUpdate)
	}
}

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
