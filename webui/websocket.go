package webui

import (
	"encoding/json"
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
	upgrader              websocket.Upgrader
	connections           *connectionManager
	previewConnections    *connectionManager
	lastPreviewBroadcast  time.Time
	lastMainBroadcast     time.Time
	mainBroadcastInterval time.Duration
	broadcastStop         chan struct{}
	broadcastStart        sync.Once
	engineState           *EngineState
	stateMu               sync.Mutex
}

type connectionManager struct {
	conns map[*websocket.Conn]*managedConnection
	mu    sync.Mutex
}

type outboundFrame struct {
	profiles map[int]outboundFrame
	full     []byte
	delta    []byte
	revision uint64
	sequence uint64
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
	return &WebSocketManager{
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
	go managed.writeLoop()
	return managed
}

func (cm *connectionManager) remove(ws *websocket.Conn) {
	cm.mu.Lock()
	managed := cm.conns[ws]
	delete(cm.conns, ws)
	cm.mu.Unlock()
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
			managed.controlMu.Lock()
			if !managed.subscribed {
				managed.controlMu.Unlock()
				continue
			}
			if profile, exists := frame.profiles[managed.budget]; exists {
				frame = profile
			}
			msg := frame.full
			if managed.ackEnabled && frame.revision != 0 && managed.ackRevision == frame.revision && len(frame.delta) > 0 {
				msg = frame.delta
			}
			if managed.ackEnabled && frame.sequence > 0 {
				managed.waiting = true
				managed.sentSequence = frame.sequence
				managed.sentRevision = frame.revision
			}
			managed.controlMu.Unlock()
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
		managed.budget = 131072
		if request.Budget >= 262144 {
			managed.budget = 262144
		}
		if request.Budget >= 500000 {
			managed.budget = 500000
		}
		if request.Budget >= previewHardLimit {
			managed.budget = previewHardLimit
		}
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
					wsManager.setPreviewRefresh(true)
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
	wsManager.stateMu.Lock()
	defer wsManager.stateMu.Unlock()
	if wsManager.engineState == nil {
		return
	}
	var payload any
	frame := outboundFrame{}
	if name == "preview" {
		preview := wsManager.engineState.Preview
		if preview == nil {
			return
		}
		if preview.Sequence == 0 {
			preview.Refresh = true
			preview.Update()
		}
		// Use the same wire metadata and revision convention as later frames.
		frame = encodePreviewFrame(preview, 1)
		select {
		case managed.send <- frame:
		case <-managed.stop:
		}
		return
	} else {
		wsManager.engineState.UpdateWithoutPreview()
		payload = wsManager.engineState.WithoutPreview()
	}
	message, err := msgpack.Marshal(payload)
	if err != nil {
		return
	}
	frame.full = message
	select {
	case managed.send <- frame:
	case <-managed.stop:
	}
}

func (wsManager *WebSocketManager) broadcastEngineState() {
	wsManager.stateMu.Lock()
	defer wsManager.stateMu.Unlock()
	wsManager.engineState.Update()
	msg, err := msgpack.Marshal(wsManager.engineState)
	if err != nil {
		log.Log.Err("Error marshaling combined message: %v", err)
		return
	}
	wsManager.connections.broadcast(msg)
	// Reset the refresh flag
	wsManager.engineState.Preview.Refresh = false
}

func (wsManager *WebSocketManager) broadcastPreviewState() {
	wsManager.stateMu.Lock()
	defer wsManager.stateMu.Unlock()
	wsManager.broadcastPreviewStateLocked()
}
func (wsManager *WebSocketManager) broadcastPreviewStateLocked() {
	if wsManager.engineState == nil || wsManager.engineState.Preview == nil {
		return
	}
	wsManager.engineState.Preview.Update()
	frame := encodePreviewFrame(wsManager.engineState.Preview, 1)
	frame.profiles = make(map[int]outboundFrame)
	wsManager.previewConnections.mu.Lock()
	budgets := make(map[int]bool)
	for _, connection := range wsManager.previewConnections.conns {
		connection.controlMu.Lock()
		if connection.subscribed && connection.budget > 0 {
			budgets[connection.budget] = true
		}
		connection.controlMu.Unlock()
	}
	wsManager.previewConnections.mu.Unlock()
	for budget := range budgets {
		profile, sampling := previewTransportProfile(wsManager.engineState.Preview, budget)
		if sampling > 1 {
			frame.profiles[budget] = encodePreviewFrame(profile, sampling)
		}
	}
	wsManager.previewConnections.broadcastFrame(frame)
	wsManager.engineState.Preview.Refresh = false
	wsManager.lastPreviewBroadcast = time.Now()
}

// A transfer limit is opt-in. The default profile retains the server's entire
// applied grid, including when Auto-adjust is disabled.
func previewTransportProfile(preview *PreviewState, budget int) (*PreviewState, int) {
	if preview.Type != "3D" || preview.VectorCount <= budget {
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
	return &profile, step
}

// Share each negotiated sampling profile across clients; engine capture is done once.
// Positions remain in the applied grid, preserving physical extents and source layers.
func encodePreviewFrame(state *PreviewState, sampling int) outboundFrame {
	preview := *state
	preview.TransportSampling = sampling
	if sampling == 1 {
		preview.ServerVectorCount = state.VectorCount
	}
	preview.TopologyRevision = state.TopologyRevision*32 + uint64(sampling)
	full, err := msgpack.Marshal(&preview)
	if err != nil {
		return outboundFrame{}
	}
	preview.VectorPositionsBinary = nil
	delta, err := msgpack.Marshal(&preview)
	if err != nil {
		return outboundFrame{}
	}
	return outboundFrame{full: full, delta: delta, sequence: preview.Sequence, revision: preview.TopologyRevision}
}

func (wsManager *WebSocketManager) broadcastEngineStateWithoutPreview() {
	wsManager.stateMu.Lock()
	defer wsManager.stateMu.Unlock()
	wsManager.broadcastEngineStateWithoutPreviewLocked()
}
func (wsManager *WebSocketManager) broadcastEngineStateWithoutPreviewLocked() {
	if wsManager.engineState == nil {
		return
	}
	wsManager.engineState.UpdateWithoutPreview()
	msg, err := msgpack.Marshal(wsManager.engineState.WithoutPreview())
	if err != nil {
		log.Log.Err("Error marshaling non-preview message: %v", err)
		return
	}
	wsManager.connections.broadcast(msg)
}

func (wsManager *WebSocketManager) setPreviewRefresh(refresh bool) {
	wsManager.stateMu.Lock()
	defer wsManager.stateMu.Unlock()
	if wsManager.engineState != nil && wsManager.engineState.Preview != nil {
		wsManager.engineState.Preview.Refresh = refresh
	}
}

// Derived fields can invoke a full demagnetization convolution. Use a lower
// preview cadence, never modify or replace the solver's exact field.
func (wsManager *WebSocketManager) previewNeedsRefresh() bool {
	wsManager.stateMu.Lock()
	defer wsManager.stateMu.Unlock()
	if wsManager.engineState == nil || wsManager.engineState.Preview == nil {
		return false
	}
	preview := wsManager.engineState.Preview
	if preview.Refresh {
		return true
	}
	if !engine.MeshReady() {
		return false
	}
	currentStep := 0
	engine.InjectAndWait(func() { currentStep = engine.NSteps })
	return preview.Step != currentStep
}

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

func (wsManager *WebSocketManager) startBroadcastLoop() {
	wsManager.broadcastStart.Do(func() {
		go func() {
			for {
				select {
				case <-wsManager.broadcastStop:
					return
				default:
					if wsManager.previewConnections.activeCount() > 0 && wsManager.previewDue() && wsManager.previewNeedsRefresh() {
						wsManager.broadcastPreviewState()
					}
					// Final solver output and console entries must also reach idle clients.
					if wsManager.connections.count() > 0 {
						now := time.Now()
						if wsManager.lastMainBroadcast.IsZero() || now.Sub(wsManager.lastMainBroadcast) >= wsManager.mainBroadcastInterval {
							wsManager.broadcastEngineStateWithoutPreview()
							wsManager.lastMainBroadcast = now
						}
					}
					time.Sleep(1 * time.Second)
				}
			}
		}()
	})
}
