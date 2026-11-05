package webui

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"sync"
	"time"

	"github.com/gorilla/websocket"
	"github.com/labstack/echo/v4"
	"github.com/vmihailenco/msgpack/v5"

	"github.com/mumax/3/engine"
	"github.com/mumax/3/log"
)

// WebSocketManager holds state previously stored in global variables.
type WebSocketManager struct {
	upgrader                websocket.Upgrader
	connections             *connectionManager
	previewConnections      *connectionManager
	lastPreviewBroadcast    time.Time
	lastMainBroadcast       time.Time
	mainBroadcastInterval   time.Duration
	broadcastStop           chan struct{}
	broadcastStart          sync.Once
	previewWake             chan struct{}
	previewWorkMu           sync.Mutex
	previewWorkPending      bool
	previewForcePending     bool
	previewStepCheckPending bool
	previewWorkerRunning    bool
	previewWorkerStart      sync.Once
	previewCaptureForTest   func(*PreviewState)
	mainWake                chan struct{}
	mainWorkMu              sync.Mutex
	mainWorkPending         bool
	mainWorkerRunning       bool
	mainWorkerStart         sync.Once
	mainConfigRevision      uint64
	mainUpdateForTest       func(*EngineState)
	solverLane              operationLane
	consoleLane             operationLane
	breakSolverForTest      func() error
	activity                chan struct{}
	engineState             *EngineState
	lastPreviewSnapshot     *PreviewState // immutable worker-owned encoding snapshot
	stateMu                 sync.Mutex
}

type connectionManager struct {
	conns    map[*websocket.Conn]*managedConnection
	mu       sync.Mutex
	onChange func()
}

type outboundFrame struct {
	profiles map[int]outboundFrame
	preview  *PreviewState
	full     []byte
	delta    []byte
	revision uint64
	sequence uint64
	encodeMs float64
}

type previewMutationAck struct {
	RequestID      string `json:"requestId"`
	ConfigRevision uint64 `json:"configRevision"`
}

type solverOperation struct {
	ID         uint64
	Command    string
	BreakFirst bool
}

type operationLane struct {
	queue            chan solverOperation
	workerStart      sync.Once
	mu               sync.Mutex
	cancelRequested  bool
	sequence         uint64
	runForTest       func(solverOperation) error
	startGateForTest <-chan struct{}
}

type operationKind uint8

const (
	solverOperationKind operationKind = iota
	consoleOperationKind
)

type solverOperationAck struct {
	OperationID uint64 `json:"operationId"`
	Status      string `json:"status"`
}

type managedConnection struct {
	ws           *websocket.Conn
	send         chan outboundFrame
	controlMu    sync.Mutex
	budget       int
	ackEnabled   bool
	waiting      bool
	subscribed   bool
	ackRevision  uint64
	sentRevision uint64
	sentSequence uint64
	ready        chan struct{}
	stop         chan struct{}
	failed       chan struct{}
	stopOnce     sync.Once
	failOnce     sync.Once
}

func newConnectionManager() *connectionManager {
	return &connectionManager{
		conns: make(map[*websocket.Conn]*managedConnection),
		mu:    sync.Mutex{},
	}
}

func newWebSocketManager() *WebSocketManager {
	manager := &WebSocketManager{
		activity: make(chan struct{}, 1),
		upgrader: websocket.Upgrader{
			EnableCompression: true,
			CheckOrigin: func(r *http.Request) bool {
				return true
			},
		},
		connections:           newConnectionManager(),
		previewConnections:    newConnectionManager(),
		mainBroadcastInterval: 5 * time.Second,
		broadcastStop:         make(chan struct{}),
		previewWake:           make(chan struct{}, 1),
		mainWake:              make(chan struct{}, 1),
		solverLane:            newOperationLane(),
		consoleLane:           newOperationLane(),
	}
	manager.connections.onChange = manager.notifyActivity
	manager.previewConnections.onChange = manager.notifyActivity
	return manager
}

func newOperationLane() operationLane {
	return operationLane{queue: make(chan solverOperation, 1)}
}

func (wsManager *WebSocketManager) requestPreviewWork(force bool) {
	wsManager.previewWorkMu.Lock()
	wsManager.previewWorkPending = true
	wsManager.previewForcePending = wsManager.previewForcePending || force
	wsManager.previewWorkMu.Unlock()
	select {
	case wsManager.previewWake <- struct{}{}:
	default:
	}
}

func (wsManager *WebSocketManager) requestPreviewStepCheck() {
	wsManager.previewWorkMu.Lock()
	wsManager.previewWorkPending = true
	wsManager.previewStepCheckPending = true
	wsManager.previewWorkMu.Unlock()
	select {
	case wsManager.previewWake <- struct{}{}:
	default:
	}
}

func (wsManager *WebSocketManager) takePreviewWork() (pending, force, checkStep bool) {
	wsManager.previewWorkMu.Lock()
	pending = wsManager.previewWorkPending
	force = wsManager.previewForcePending
	checkStep = wsManager.previewStepCheckPending
	if pending {
		wsManager.previewWorkerRunning = true
	}
	wsManager.previewWorkPending = false
	wsManager.previewForcePending = false
	wsManager.previewStepCheckPending = false
	wsManager.previewWorkMu.Unlock()
	return pending, force, checkStep
}

func (wsManager *WebSocketManager) finishPreviewWork() {
	wsManager.previewWorkMu.Lock()
	wsManager.previewWorkerRunning = false
	wsManager.previewWorkMu.Unlock()
}

func (wsManager *WebSocketManager) previewWorkScheduled() bool {
	wsManager.previewWorkMu.Lock()
	defer wsManager.previewWorkMu.Unlock()
	return wsManager.previewWorkPending || wsManager.previewWorkerRunning
}

func (wsManager *WebSocketManager) startPreviewWorker() {
	wsManager.previewWorkerStart.Do(func() { go wsManager.previewWorker() })
}

func (wsManager *WebSocketManager) startMainWorker() {
	wsManager.mainWorkerStart.Do(func() { go wsManager.mainSnapshotWorker() })
}

func (wsManager *WebSocketManager) requestMainSnapshot() {
	if wsManager.connections.count() == 0 {
		return
	}
	wsManager.startMainWorker()
	wsManager.mainWorkMu.Lock()
	wsManager.mainWorkPending = true
	wsManager.mainWorkMu.Unlock()
	select {
	case wsManager.mainWake <- struct{}{}:
	default:
	}
}

func (wsManager *WebSocketManager) mainSnapshotScheduled() bool {
	wsManager.mainWorkMu.Lock()
	defer wsManager.mainWorkMu.Unlock()
	return wsManager.mainWorkPending || wsManager.mainWorkerRunning
}

func (wsManager *WebSocketManager) takeMainSnapshotWork() bool {
	wsManager.mainWorkMu.Lock()
	defer wsManager.mainWorkMu.Unlock()
	if !wsManager.mainWorkPending {
		return false
	}
	wsManager.mainWorkPending = false
	wsManager.mainWorkerRunning = true
	return true
}

func (wsManager *WebSocketManager) finishMainSnapshotWork() {
	wsManager.mainWorkMu.Lock()
	wsManager.mainWorkerRunning = false
	wsManager.mainWorkMu.Unlock()
}

var errSolverOperationCancelled = errors.New("solver operation cancelled")

func (wsManager *WebSocketManager) reserveSolverOperation(command string, breakFirst bool) (solverOperationAck, bool) {
	return wsManager.reserveOperation(&wsManager.solverLane, solverOperationKind, command, breakFirst)
}

func (wsManager *WebSocketManager) reserveConsoleOperation(command string) (solverOperationAck, bool) {
	return wsManager.reserveOperation(&wsManager.consoleLane, consoleOperationKind, command, false)
}

func (wsManager *WebSocketManager) reserveOperation(lane *operationLane, kind operationKind, command string, breakFirst bool) (solverOperationAck, bool) {
	lane.mu.Lock()
	wsManager.stateMu.Lock()
	operationID, status, exists := wsManager.operationStateLocked(kind)
	if !exists {
		wsManager.stateMu.Unlock()
		lane.mu.Unlock()
		return solverOperationAck{}, false
	}
	if status == "pending" || status == "running" {
		ack := solverOperationAck{OperationID: operationID, Status: status}
		wsManager.stateMu.Unlock()
		lane.mu.Unlock()
		return ack, false
	}
	lane.sequence++
	operation := solverOperation{ID: lane.sequence, Command: command, BreakFirst: breakFirst}
	wsManager.startOperationStateLocked(kind, operation.ID)
	lane.cancelRequested = false
	wsManager.stateMu.Unlock()
	lane.mu.Unlock()

	wsManager.startOperationWorker(lane, kind)
	select {
	case lane.queue <- operation:
		wsManager.broadcastEngineStateWithoutPreview()
		return solverOperationAck{OperationID: operation.ID, Status: "pending"}, true
	default:
		wsManager.stateMu.Lock()
		currentID, _, exists := wsManager.operationStateLocked(kind)
		if exists && currentID == operation.ID {
			wsManager.setOperationStateLocked(kind, operation.ID, "idle", "")
		}
		wsManager.stateMu.Unlock()
		return solverOperationAck{}, false
	}
}

func (wsManager *WebSocketManager) startOperationWorker(lane *operationLane, kind operationKind) {
	lane.workerStart.Do(func() { go wsManager.operationWorker(lane, kind) })
}

func (wsManager *WebSocketManager) operationWorker(lane *operationLane, kind operationKind) {
	if gate := lane.startGateForTest; gate != nil {
		select {
		case <-gate:
		case <-wsManager.broadcastStop:
			return
		}
	}
	for {
		select {
		case <-wsManager.broadcastStop:
			return
		case operation := <-lane.queue:
			if !wsManager.startOperation(lane, kind, operation) {
				wsManager.finishOperation(kind, operation, "cancelled", errSolverOperationCancelled)
				continue
			}
			err := wsManager.executeOperation(lane, operation)
			if errors.Is(err, errSolverOperationCancelled) || wsManager.operationCancelled(lane) {
				wsManager.finishOperation(kind, operation, "cancelled", err)
			} else if err != nil {
				wsManager.finishOperation(kind, operation, "failed", err)
			} else {
				wsManager.finishOperation(kind, operation, "completed", nil)
			}
		}
	}
}

func (wsManager *WebSocketManager) operationCancelled(lane *operationLane) bool {
	lane.mu.Lock()
	defer lane.mu.Unlock()
	return lane.cancelRequested
}

func (wsManager *WebSocketManager) executeOperation(lane *operationLane, operation solverOperation) error {
	if wsManager.operationCancelled(lane) {
		return errSolverOperationCancelled
	}
	if lane.runForTest != nil {
		return lane.runForTest(operation)
	}
	if operation.BreakFirst {
		engine.Break()
	}
	if wsManager.operationCancelled(lane) {
		return errSolverOperationCancelled
	}
	done := make(chan error, 1)
	engine.Inject <- func() {
		if wsManager.operationCancelled(lane) {
			done <- errSolverOperationCancelled
			return
		}
		done <- engine.EvalTryRecoverError(operation.Command)
	}
	return <-done
}

func (wsManager *WebSocketManager) operationStateLocked(kind operationKind) (id uint64, status string, exists bool) {
	if wsManager.engineState == nil {
		return 0, "", false
	}
	switch kind {
	case solverOperationKind:
		if wsManager.engineState.Solver == nil {
			return 0, "", false
		}
		return wsManager.engineState.Solver.OperationID, wsManager.engineState.Solver.OperationStatus, true
	case consoleOperationKind:
		if wsManager.engineState.Console == nil {
			return 0, "", false
		}
		return wsManager.engineState.Console.OperationID, wsManager.engineState.Console.OperationStatus, true
	default:
		return 0, "", false
	}
}

func (wsManager *WebSocketManager) setOperationStateLocked(kind operationKind, id uint64, status, message string) {
	// Invalidate in-flight main snapshots so they cannot publish the old status.
	wsManager.mainConfigRevision++
	switch kind {
	case solverOperationKind:
		if wsManager.engineState != nil && wsManager.engineState.Solver != nil && wsManager.engineState.Solver.OperationID == id {
			wsManager.engineState.Solver.OperationStatus = status
			wsManager.engineState.Solver.OperationError = message
		}
	case consoleOperationKind:
		if wsManager.engineState != nil && wsManager.engineState.Console != nil && wsManager.engineState.Console.OperationID == id {
			wsManager.engineState.Console.OperationStatus = status
			wsManager.engineState.Console.OperationError = message
		}
	}
}

func (wsManager *WebSocketManager) startOperationStateLocked(kind operationKind, id uint64) {
	wsManager.mainConfigRevision++
	switch kind {
	case solverOperationKind:
		if wsManager.engineState != nil && wsManager.engineState.Solver != nil {
			wsManager.engineState.Solver.OperationID = id
			wsManager.engineState.Solver.OperationStatus = "pending"
			wsManager.engineState.Solver.OperationError = ""
		}
	case consoleOperationKind:
		if wsManager.engineState != nil && wsManager.engineState.Console != nil {
			wsManager.engineState.Console.OperationID = id
			wsManager.engineState.Console.OperationStatus = "pending"
			wsManager.engineState.Console.OperationError = ""
		}
	}
}

// startOperation serializes the pending-to-running transition with Break.
// If Break marks a pending lane first, the command never reaches Eval.
func (wsManager *WebSocketManager) startOperation(lane *operationLane, kind operationKind, operation solverOperation) bool {
	lane.mu.Lock()
	defer lane.mu.Unlock()
	if lane.cancelRequested {
		return false
	}
	wsManager.stateMu.Lock()
	id, status, exists := wsManager.operationStateLocked(kind)
	if !exists || id != operation.ID || status != "pending" {
		wsManager.stateMu.Unlock()
		return false
	}
	wsManager.setOperationStateLocked(kind, operation.ID, "running", "")
	wsManager.stateMu.Unlock()
	wsManager.broadcastEngineStateWithoutPreview()
	return true
}

func (wsManager *WebSocketManager) finishOperation(kind operationKind, operation solverOperation, status string, err error) {
	message := ""
	if err != nil && !errors.Is(err, errSolverOperationCancelled) {
		message = err.Error()
	}
	wsManager.stateMu.Lock()
	wsManager.setOperationStateLocked(kind, operation.ID, status, message)
	wsManager.stateMu.Unlock()
	wsManager.broadcastEngineStateWithoutPreview()
	wsManager.setPreviewRefresh(true)
}

func (wsManager *WebSocketManager) breakSolver() error {
	wsManager.solverLane.mu.Lock()
	wsManager.consoleLane.mu.Lock()
	wsManager.stateMu.Lock()
	if _, status, exists := wsManager.operationStateLocked(solverOperationKind); exists && (status == "pending" || status == "running") {
		wsManager.solverLane.cancelRequested = true
	}
	if _, status, exists := wsManager.operationStateLocked(consoleOperationKind); exists && (status == "pending" || status == "running") {
		wsManager.consoleLane.cancelRequested = true
	}
	wsManager.stateMu.Unlock()
	wsManager.consoleLane.mu.Unlock()
	wsManager.solverLane.mu.Unlock()
	if wsManager.breakSolverForTest != nil {
		return wsManager.breakSolverForTest()
	}
	engine.Break()
	return nil
}

func (wsManager *WebSocketManager) notifyActivity() {
	select {
	case wsManager.activity <- struct{}{}:
	default:
	}
}

func (cm *connectionManager) add(ws *websocket.Conn) *managedConnection {
	managed := &managedConnection{
		ws:         ws,
		subscribed: true, ready: make(chan struct{}, 1),
		send:   make(chan outboundFrame, 1),
		stop:   make(chan struct{}),
		failed: make(chan struct{}),
	}
	cm.mu.Lock()
	cm.conns[ws] = managed
	cm.mu.Unlock()
	if cm.onChange != nil {
		cm.onChange()
	}
	go managed.writeLoop()
	return managed
}

func (cm *connectionManager) remove(ws *websocket.Conn) {
	cm.mu.Lock()
	managed := cm.conns[ws]
	delete(cm.conns, ws)
	cm.mu.Unlock()
	if cm.onChange != nil {
		cm.onChange()
	}
	if managed != nil {
		managed.stopOnce.Do(func() { close(managed.stop) })
	}
}

func (cm *connectionManager) count() int {
	cm.mu.Lock()
	defer cm.mu.Unlock()
	return len(cm.conns)
}

func (cm *connectionManager) broadcast(msg []byte) { cm.broadcastFrame(outboundFrame{full: msg}) }

func (cm *connectionManager) broadcastFrame(frame outboundFrame) {
	cm.mu.Lock()
	connections := make([]*managedConnection, 0, len(cm.conns))
	for _, managed := range cm.conns {
		connections = append(connections, managed)
	}
	cm.mu.Unlock()

	for _, managed := range connections {
		select {
		case <-managed.stop:
			continue
		default:
		}
		select {
		case managed.send <- frame:
		default:
			// Keep latency bounded: discard the stale queued frame and retain
			// only the newest complete state for this client.
			select {
			case <-managed.send:
			default:
			}
			select {
			case managed.send <- frame:
			case <-managed.stop:
			default:
			}
		}
	}
}

func (managed *managedConnection) writeLoop() {
	for {
		select {
		case <-managed.stop:
			return
		case frame := <-managed.send:
			if !managed.waitReady() {
				return
			}
			// ACK can take seconds: drain any superseded frame before writing.
			select {
			case latest := <-managed.send:
				frame = latest
			default:
			}
			var msg []byte
			for {
				managed.controlMu.Lock()
				if !managed.subscribed {
					managed.controlMu.Unlock()
					break
				}
				budget := managed.budget
				ackEnabled, ackRevision := managed.ackEnabled, managed.ackRevision
				managed.controlMu.Unlock()

				selected := frame
				if profile, exists := frame.profiles[budget]; exists {
					selected = profile
				} else if frame.preview != nil && budget > 0 {
					// Budgets that need no sampling share the already encoded
					// full frame; only a budget that joined after encoding
					// needs its own profile here.
					if profile, sampling := previewTransportProfile(frame.preview, budget); sampling > 1 {
						selected = encodePreviewFrame(profile, sampling)
						selected.preview = profile
					}
				}
				msg = selected.full
				if ackEnabled && selected.revision != 0 && ackRevision == selected.revision && len(selected.delta) > 0 {
					msg = selected.delta
				}

				managed.controlMu.Lock()
				if !managed.subscribed {
					managed.controlMu.Unlock()
					break
				}
				if managed.budget != budget || managed.ackEnabled != ackEnabled || managed.ackRevision != ackRevision {
					managed.controlMu.Unlock()
					continue
				}
				if managed.ackEnabled && selected.sequence > 0 {
					managed.waiting = true
					managed.sentSequence = selected.sequence
					managed.sentRevision = selected.revision
				}
				managed.controlMu.Unlock()
				break
			}
			if msg == nil {
				continue
			}
			if err := managed.ws.SetWriteDeadline(time.Now().Add(10 * time.Second)); err != nil {
				log.Log.Warn("Could not set WebSocket write deadline: %v", err)
			}
			if err := managed.ws.WriteMessage(websocket.BinaryMessage, msg); err != nil {
				log.Log.Err("Error sending message via WebSocket: %v", err)
				managed.failOnce.Do(func() { close(managed.failed) })
				_ = managed.ws.Close()
				return
			}
		}
	}
}

// Credits bound in-flight preview frames; old clients retain full-frame fallback.
// Slow rendering or background tabs keep their single credit outstanding until
// ACK/resync or disconnect. A render stall must not force a reconnect loop.
func (managed *managedConnection) waitReady() bool {
	for {
		managed.controlMu.Lock()
		waiting := managed.waiting && managed.ackEnabled
		managed.controlMu.Unlock()
		if !waiting {
			return true
		}
		select {
		case <-managed.ready:
		case <-managed.stop:
			return false
		}
	}
}
func (managed *managedConnection) control(data []byte) {
	var request struct {
		Budget    int    `json:"maxPoints"`
		Protocol  int    `json:"protocol"`
		Ack       uint64 `json:"ack"`
		Revision  uint64 `json:"revision"`
		Subscribe *bool  `json:"subscribe"`
		Resync    bool   `json:"resync"`
	}
	if json.Unmarshal(data, &request) != nil {
		return
	} // legacy "ok"
	managed.controlMu.Lock()
	if request.Protocol == 2 {
		managed.ackEnabled = true
		if managed.budget == 0 {
			managed.budget = previewHardLimit
		}
	}
	if request.Budget >= 8 && request.Budget <= previewHardLimit {
		managed.budget = request.Budget
		managed.ackRevision = 0
		managed.waiting = false
	}
	if request.Subscribe != nil {
		managed.subscribed = *request.Subscribe
		if !managed.subscribed {
			managed.waiting = false
		}
	}
	if request.Resync {
		managed.ackRevision = 0
		managed.waiting = false
	}
	if request.Ack == managed.sentSequence && managed.sentSequence > 0 {
		if request.Revision == managed.sentRevision {
			managed.ackRevision = request.Revision
		}
		managed.waiting = false
	}
	managed.controlMu.Unlock()
	select {
	case managed.ready <- struct{}{}:
	default:
	}
}
func (cm *connectionManager) activeCount() int {
	cm.mu.Lock()
	defer cm.mu.Unlock()
	count := 0
	for _, managed := range cm.conns {
		managed.controlMu.Lock()
		if managed.subscribed && !managed.waiting {
			count++
		}
		managed.controlMu.Unlock()
	}
	return count
}

func (wsManager *WebSocketManager) websocketEntrypoint(c echo.Context) error {
	return wsManager.websocketEntrypointFor(c, wsManager.connections, "main", nil)
}

func (wsManager *WebSocketManager) websocketPreviewEntrypoint(c echo.Context) error {
	return wsManager.websocketEntrypointFor(c, wsManager.previewConnections, "preview", nil)
}

func (wsManager *WebSocketManager) websocketEntrypointFor(c echo.Context, cm *connectionManager, name string, onConnect func()) error {
	log.Log.Debug("New %s WebSocket connection, upgrading...", name)
	ws, err := wsManager.upgrader.Upgrade(c.Response(), c.Request(), nil)
	log.Log.Debug("New %s WebSocket connection upgraded", name)
	if err != nil {
		log.Log.Err("Error upgrading %s websocket connection: %v", name, err)
		return err
	}
	ws.EnableWriteCompression(true)
	defer func() {
		if err := ws.Close(); err != nil {
			log.Log.Err("Error closing %s websocket: %v", name, err)
		}
	}()

	ws.SetReadLimit(4096)
	managed := cm.add(ws)
	defer cm.remove(ws)
	if name == "main" {
		engine.InteractiveClientConnected()
		defer engine.InteractiveClientDisconnected()
	}
	if onConnect != nil {
		onConnect()
	} else {
		wsManager.sendInitialState(managed, name)
	}

	// Channel to signal when to stop the goroutine
	done := make(chan struct{})
	go func() {
		for {
			_, data, err := ws.ReadMessage()
			if err != nil {
				close(done)
				return
			}
			if name == "preview" {
				managed.control(data)
				var req struct {
					Subscribe *bool `json:"subscribe"`
					Resync    bool  `json:"resync"`
					Budget    int   `json:"maxPoints"`
				}
				if json.Unmarshal(data, &req) == nil && (req.Resync || req.Budget > 0 || (req.Subscribe != nil && *req.Subscribe)) {
					wsManager.broadcastPreviewState()
				}
			}
		}
	}()

	select {
	case <-done:
		log.Log.Debug("%s websocket connection closed by client", name)
		return nil
	case <-wsManager.broadcastStop:
		return nil
	case <-managed.failed:
		return nil
	}
}

// Initial state is targeted: opening a second tab must not invalidate other clients.
func (wsManager *WebSocketManager) sendInitialState(managed *managedConnection, name string) {
	if name == "preview" {
		wsManager.stateMu.Lock()
		var preview *PreviewState
		if wsManager.engineState != nil {
			preview = wsManager.engineState.Preview
		}
		if preview == nil {
			wsManager.stateMu.Unlock()
			return
		}
		snapshot := wsManager.lastPreviewSnapshot
		if snapshot != nil && snapshot.ConfigRevision != preview.ConfigRevision {
			snapshot = nil
		}
		wsManager.stateMu.Unlock()
		if snapshot == nil || snapshot.Sequence == 0 {
			wsManager.requestPreviewWork(false)
			return
		}
		frame := encodePreviewFrame(snapshot, 1)
		frame.preview = snapshot
		select {
		case managed.send <- frame:
		case <-managed.stop:
		}
		return
	}
	wsManager.requestMainSnapshot()
}

func (wsManager *WebSocketManager) broadcastPreviewState() {
	if wsManager.previewConnections.activeCount() == 0 {
		return
	}
	wsManager.requestPreviewWork(false)
}
func (wsManager *WebSocketManager) broadcastPreviewStateLocked(requestID string) previewMutationAck {
	if wsManager.engineState == nil || wsManager.engineState.Preview == nil {
		return previewMutationAck{RequestID: requestID}
	}
	preview := wsManager.engineState.Preview
	preview.ConfigRevision++
	preview.Refresh = true
	wsManager.requestPreviewWork(false)
	return previewMutationAck{RequestID: requestID, ConfigRevision: preview.ConfigRevision}
}

func (wsManager *WebSocketManager) previewWorker() {
	scratch := &PreviewState{ws: wsManager}
	for {
		select {
		case <-wsManager.broadcastStop:
			return
		case <-wsManager.previewWake:
		}
		for {
			pending, force, checkStep := wsManager.takePreviewWork()
			if !pending {
				break
			}
			if wsManager.previewConnections.activeCount() == 0 {
				wsManager.finishPreviewWork()
				continue
			}
			wsManager.stateMu.Lock()
			if wsManager.engineState == nil || wsManager.engineState.Preview == nil {
				wsManager.stateMu.Unlock()
				wsManager.finishPreviewWork()
				continue
			}
			published := wsManager.engineState.Preview
			revision := published.ConfigRevision
			beforeSequence := published.Sequence
			refreshRequested := published.Refresh
			previousMesh := published.Mesh
			copyPreviewConfig(scratch, published, wsManager)
			cached := wsManager.lastPreviewSnapshot
			wsManager.stateMu.Unlock()

			captureForce := force || cached == nil
			needsCapture := captureForce || checkStep || refreshRequested || beforeSequence == 0
			snapshot := cached
			captured := false
			if needsCapture {
				if wsManager.previewCaptureForTest != nil {
					wsManager.previewCaptureForTest(scratch)
				} else {
					scratch.update(captureForce)
				}
				captured = scratch.Sequence != beforeSequence
				if captured {
					snapshot = clonePreviewState(scratch)
				}
			}
			if checkStep && !captureForce && !refreshRequested && !captured {
				wsManager.finishPreviewWork()
				continue
			}
			if snapshot == nil {
				wsManager.finishPreviewWork()
				continue
			}
			if captured {
				snapshot.ConfigRevision = revision
				if previousMesh != nil && snapshot.Mesh != nil && !samePreviewMesh(previousMesh, snapshot.Mesh) {
					wsManager.stateMu.Lock()
					if wsManager.engineState != nil && wsManager.engineState.Preview != nil {
						current := wsManager.engineState.Preview
						if current.ConfigRevision == revision {
							current.ConfigRevision++
							current.Refresh = true
							revision = current.ConfigRevision
							snapshot.ConfigRevision = revision
						}
					}
					wsManager.stateMu.Unlock()
				}
			}
			frames, encodeMs := wsManager.encodePreviewProfiles(snapshot)

			wsManager.stateMu.Lock()
			if wsManager.engineState == nil || wsManager.engineState.Preview == nil {
				wsManager.stateMu.Unlock()
				wsManager.finishPreviewWork()
				continue
			}
			current := wsManager.engineState.Preview
			if current.ConfigRevision != revision || wsManager.previewConnections.activeCount() == 0 {
				stale := current.ConfigRevision != revision && wsManager.previewConnections.activeCount() > 0
				if stale {
					// A discarded capture may have advanced its private topology cache.
					// Force the next worker result to keyframe relative to the last
					// published topology so deltas cannot omit changed coordinates.
					scratch.TopologyRevision = current.TopologyRevision
					scratch.Mesh = current.Mesh
					scratch.cachedTopologyKey = ""
					scratch.cachedPositionsBinary = nil
					scratch.VectorFieldValues = nil
					scratch.VectorFieldPositions = nil
					scratch.VectorPositionsBinary = nil
					scratch.VectorOccupancy = nil
					scratch.VectorCount = current.VectorCount
					scratch.ScalarField = nil
				}
				wsManager.stateMu.Unlock()
				if stale {
					wsManager.requestPreviewWork(false)
				}
				wsManager.finishPreviewWork()
				continue
			}
			if captured {
				copyPreviewOutput(current, snapshot)
				wsManager.lastPreviewSnapshot = snapshot
			}
			current.Refresh = false
			current.EncodeMs = encodeMs
			wsManager.previewConnections.broadcastFrame(frames)
			wsManager.lastPreviewBroadcast = time.Now()
			wsManager.stateMu.Unlock()
			wsManager.finishPreviewWork()
		}
	}
}

func (wsManager *WebSocketManager) encodePreviewProfiles(snapshot *PreviewState) (outboundFrame, float64) {
	started := time.Now()
	frame := encodePreviewFrame(snapshot, 1)
	frame.preview = snapshot
	frame.profiles = make(map[int]outboundFrame)
	for _, budget := range wsManager.previewBudgets() {
		profile, sampling := previewTransportProfile(snapshot, budget)
		if sampling <= 1 {
			continue
		}
		encoded := encodePreviewFrame(profile, sampling)
		encoded.preview = profile
		frame.profiles[budget] = encoded
	}
	return frame, float64(time.Since(started).Microseconds()) / 1000
}

func (wsManager *WebSocketManager) previewBudgets() []int {
	wsManager.previewConnections.mu.Lock()
	defer wsManager.previewConnections.mu.Unlock()
	seen := make(map[int]bool)
	for _, connection := range wsManager.previewConnections.conns {
		connection.controlMu.Lock()
		if connection.subscribed && connection.budget > 0 {
			seen[connection.budget] = true
		}
		connection.controlMu.Unlock()
	}
	budgets := make([]int, 0, len(seen))
	for budget := range seen {
		budgets = append(budgets, budget)
	}
	return budgets
}

// A transfer limit is per-client and opt-in. It samples the transmitted points
// while preserving the complete applied grid and original cell coordinates.
func previewTransportProfile(preview *PreviewState, budget int) (*PreviewState, int) {
	if budget < 1 {
		return preview, 1
	}
	if preview.Type == "2D" {
		serverCount := len(preview.ScalarField)
		if serverCount <= budget {
			return preview, 1
		}
		step := 2
		var profile PreviewState
		for {
			profile = *preview
			profile.ScalarField = make([][3]float32, 0, min(serverCount, budget+1))
			for _, point := range preview.ScalarField {
				x, y := int(point[0]), int(point[1])
				if float32(x) == point[0] && float32(y) == point[1] && x%step == 0 && y%step == 0 {
					profile.ScalarField = append(profile.ScalarField, point)
				}
			}
			if len(profile.ScalarField) <= budget {
				break
			}
			step *= 2
		}
		profile.DataPointsCount = len(profile.ScalarField)
		profile.ServerDataPointsCount = serverCount
		return &profile, step
	}
	if preview.VectorCount <= budget {
		return preview, 1
	}
	step := 2
	var profile PreviewState
	for {
		profile = *preview
		profile.VectorFieldValues = nil
		profile.VectorFieldPositions = nil
		profile.VectorOccupancy = nil
		for i, position := range preview.VectorFieldPositions {
			z := position.Z / max(preview.AppliedLayerStride, 1)
			if position.X%step == 0 && position.Y%step == 0 && (!preview.AllLayers || z%step == 0) {
				profile.VectorFieldPositions = append(profile.VectorFieldPositions, position)
				profile.VectorFieldValues = append(profile.VectorFieldValues, preview.VectorFieldValues[i])
				if len(preview.VectorOccupancy) == len(preview.VectorFieldPositions) {
					profile.VectorOccupancy = append(profile.VectorOccupancy, preview.VectorOccupancy[i])
				}
			}
		}
		if len(profile.VectorFieldValues) <= budget {
			break
		}
		step *= 2
	}
	profile.VectorCount = len(profile.VectorFieldValues)
	profile.VectorValuesBinary = packVectorValues(nil, profile.VectorFieldValues)
	profile.VectorPositionsBinary = packVectorPositions(nil, profile.VectorFieldPositions)
	profile.DataPointsCount = profile.VectorCount
	profile.ServerVectorCount = preview.VectorCount
	profile.ServerDataPointsCount = preview.VectorCount
	return &profile, step
}

// Share each negotiated sampling profile across clients; engine capture is done once.
// Positions remain in the applied grid, preserving physical extents and source layers.
func encodePreviewFrame(state *PreviewState, sampling int) outboundFrame {
	preview := *state
	preview.TransportSampling = sampling
	if sampling == 1 {
		preview.ServerVectorCount = state.VectorCount
		if state.Type == "2D" {
			preview.ServerDataPointsCount = len(state.ScalarField)
			preview.DataPointsCount = len(state.ScalarField)
		} else {
			preview.ServerDataPointsCount = state.VectorCount
		}
	}
	preview.TopologyRevision = state.TopologyRevision<<32 | uint64(sampling)
	// The payload carries the previous frame's encode time; measuring this one
	// would require marshaling the (up to 1M point) frame a second time.
	started := time.Now()
	full, err := msgpack.Marshal(&preview)
	if err != nil {
		return outboundFrame{}
	}
	preview.VectorPositionsBinary = nil
	delta, err := msgpack.Marshal(&preview)
	if err != nil {
		return outboundFrame{}
	}
	encodeMs := float64(time.Since(started).Microseconds()) / 1000
	return outboundFrame{full: full, delta: delta, sequence: preview.Sequence, revision: preview.TopologyRevision, encodeMs: encodeMs}
}

func (wsManager *WebSocketManager) broadcastEngineStateWithoutPreview() {
	wsManager.stateMu.Lock()
	if wsManager.engineState != nil {
		wsManager.mainConfigRevision++
	}
	wsManager.stateMu.Unlock()
	wsManager.requestMainSnapshot()
}
func (wsManager *WebSocketManager) broadcastEngineStateWithoutPreviewLocked() {
	if wsManager.engineState == nil {
		return
	}
	wsManager.mainConfigRevision++
	wsManager.requestMainSnapshot()
}

func (wsManager *WebSocketManager) mainSnapshotWorker() {
	for {
		select {
		case <-wsManager.broadcastStop:
			return
		case <-wsManager.mainWake:
		}
		for wsManager.takeMainSnapshotWork() {
			if wsManager.connections.count() == 0 {
				wsManager.finishMainSnapshotWork()
				continue
			}
			wsManager.stateMu.Lock()
			state := cloneEngineStateWithoutPreview(wsManager.engineState)
			revision := wsManager.mainConfigRevision
			wsManager.stateMu.Unlock()
			if state == nil {
				wsManager.finishMainSnapshotWork()
				continue
			}

			if wsManager.mainUpdateForTest != nil {
				wsManager.mainUpdateForTest(state)
			} else if err := updateEngineStateSnapshot(state); err != nil {
				log.Log.Err("Error updating non-preview snapshot: %v", err)
				wsManager.finishMainSnapshotWork()
				continue
			}
			msg, err := msgpack.Marshal(state.WithoutPreview())
			if err != nil {
				log.Log.Err("Error marshaling non-preview message: %v", err)
				wsManager.finishMainSnapshotWork()
				continue
			}

			wsManager.stateMu.Lock()
			stale := wsManager.mainConfigRevision != revision
			if !stale {
				// HTTP handlers read the published state (metrics refresh
				// timers, table columns), so keep it in step with what was sent.
				storeEngineStateWithoutPreview(wsManager.engineState, state)
			}
			if !stale && wsManager.connections.count() > 0 {
				wsManager.connections.broadcast(msg)
				wsManager.lastMainBroadcast = time.Now()
			}
			active := wsManager.connections.count() > 0
			wsManager.stateMu.Unlock()
			wsManager.finishMainSnapshotWork()
			if stale && active {
				wsManager.requestMainSnapshot()
			}
		}
	}
}

func updateEngineStateSnapshot(state *EngineState) (err error) {
	defer func() {
		if recovered := recover(); recovered != nil {
			err = fmt.Errorf("%v", recovered)
		}
	}()
	state.UpdateWithoutPreview()
	return nil
}

func (wsManager *WebSocketManager) setPreviewRefresh(refresh bool) {
	wsManager.stateMu.Lock()
	if wsManager.engineState != nil && wsManager.engineState.Preview != nil {
		if refresh {
			wsManager.engineState.Preview.ConfigRevision++
			wsManager.engineState.Preview.Refresh = true
		}
	}
	wsManager.stateMu.Unlock()
	if refresh {
		wsManager.requestPreviewWork(false)
	}
}

// Derived fields can invoke a full demagnetization convolution. Use a lower
// preview cadence, never modify or replace the solver's exact field.
func (wsManager *WebSocketManager) previewDue() bool {
	wsManager.stateMu.Lock()
	defer wsManager.stateMu.Unlock()
	interval := time.Second
	if wsManager.engineState != nil && wsManager.engineState.Preview != nil {
		if wsManager.engineState.Preview.Refresh {
			return true
		}
		quantity := wsManager.engineState.Preview.Quantity
		if quantity != "m" && quantity != "geom" && quantity != "regions" {
			interval = 5 * time.Second
		}
	}
	return time.Since(wsManager.lastPreviewBroadcast) >= interval
}

// No timer, solver injection, capture, encoding or metric collection is needed
// while the server only listens for clients. Connection changes wake this loop.
func (wsManager *WebSocketManager) waitForBroadcastClients() bool {
	for wsManager.connections.count() == 0 && wsManager.previewConnections.count() == 0 {
		select {
		case <-wsManager.broadcastStop:
			return false
		case <-wsManager.activity:
		}
	}
	return true
}

func (wsManager *WebSocketManager) broadcastTick() {
	if wsManager.previewConnections.activeCount() > 0 && !wsManager.previewWorkScheduled() && wsManager.previewDue() {
		wsManager.requestPreviewStepCheck()
	}
	// Final solver output and console entries must also reach idle clients.
	if wsManager.connections.count() > 0 && !wsManager.mainSnapshotScheduled() {
		wsManager.stateMu.Lock()
		now := time.Now()
		due := wsManager.lastMainBroadcast.IsZero() || now.Sub(wsManager.lastMainBroadcast) >= wsManager.mainBroadcastInterval
		wsManager.stateMu.Unlock()
		if due {
			wsManager.broadcastEngineStateWithoutPreview()
		}
	}
}

func (wsManager *WebSocketManager) startBroadcastLoop() {
	wsManager.startPreviewWorker()
	wsManager.startMainWorker()
	wsManager.broadcastStart.Do(func() {
		go func() {
			for wsManager.waitForBroadcastClients() {
				wsManager.broadcastTick()
				timer := time.NewTimer(time.Second)
				select {
				case <-wsManager.broadcastStop:
					timer.Stop()
					return
				case <-wsManager.activity:
					timer.Stop()
				case <-timer.C:
				}
			}
		}()
	})
}
