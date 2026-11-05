package webui

import (
	"encoding/json"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/labstack/echo/v4"
)

func TestLongSolverOperationReturnsAcceptedRejectsDuplicateAndBreaks(t *testing.T) {
	manager := newWebSocketManager()
	t.Cleanup(func() { close(manager.broadcastStop) })
	solver := &SolverState{ws: manager, OperationStatus: "idle"}
	manager.engineState = &EngineState{Solver: solver, Preview: &PreviewState{ws: manager}}
	started, broken := make(chan struct{}), make(chan struct{})
	manager.solverLane.runForTest = func(op solverOperation) error {
		if op.Command != "Relax()" {
			t.Errorf("unexpected command: %q", op.Command)
		}
		if !op.BreakFirst {
			t.Error("dedicated Relax operation did not preserve pre-run Break behavior")
		}
		close(started)
		<-broken
		return nil
	}
	manager.breakSolverForTest = func() error {
		close(broken)
		return nil
	}
	state := &SolverState{ws: manager}

	accepted := invokeSolverHandler(t, state.postSolverRelax)
	var ack solverOperationAck
	if err := json.Unmarshal(accepted.Body.Bytes(), &ack); err != nil {
		t.Fatal(err)
	}
	if accepted.Code != 202 || ack.OperationID != 1 || ack.Status != "pending" {
		t.Fatalf("wrong accepted response: status=%d ack=%+v body=%s", accepted.Code, ack, accepted.Body.String())
	}
	select {
	case <-started:
	case <-time.After(time.Second):
		t.Fatal("solver worker did not start the accepted operation")
	}

	duplicate := invokeSolverHandler(t, state.postSolverMinimize)
	if duplicate.Code != 409 || !strings.Contains(duplicate.Body.String(), `"operationId":1`) {
		t.Fatalf("duplicate solver operation was not rejected: status=%d body=%s", duplicate.Code, duplicate.Body.String())
	}
	brokenResponse := invokeSolverHandler(t, state.postSolverBreak)
	if brokenResponse.Code != 200 {
		t.Fatalf("Break unavailable during long operation: status=%d body=%s", brokenResponse.Code, brokenResponse.Body.String())
	}
	waitSolverStatus(t, manager, solver, "cancelled")
}

func TestPendingSolverOperationCanBeCancelledBeforeWorkerStarts(t *testing.T) {
	manager := newWebSocketManager()
	t.Cleanup(func() { close(manager.broadcastStop) })
	solver := &SolverState{ws: manager, OperationStatus: "idle"}
	manager.engineState = &EngineState{Solver: solver, Preview: &PreviewState{ws: manager}}
	gate := make(chan struct{})
	manager.solverLane.startGateForTest = gate
	manager.breakSolverForTest = func() error { return nil }
	var ran atomic.Bool
	manager.solverLane.runForTest = func(solverOperation) error { ran.Store(true); return nil }
	ack, accepted := manager.reserveSolverOperation("Run(1)", true)
	if !accepted || ack.Status != "pending" || ack.OperationID == 0 {
		t.Fatalf("operation was not reserved: ack=%+v accepted=%v", ack, accepted)
	}
	if err := manager.breakSolver(); err != nil {
		t.Fatal(err)
	}
	close(gate)
	waitSolverStatus(t, manager, solver, "cancelled")
	if ran.Load() {
		t.Fatal("cancelled pending Run command executed after Break")
	}
}

func TestConsoleCommandRunsWhileSolverIsBusyAndBreakCancelsBothLanes(t *testing.T) {
	manager := newWebSocketManager()
	t.Cleanup(func() { close(manager.broadcastStop) })
	solver := &SolverState{ws: manager, OperationStatus: "idle"}
	consoleState := &ConsoleState{ws: manager, OperationStatus: "idle"}
	manager.engineState = &EngineState{Solver: solver, Console: consoleState, Preview: &PreviewState{ws: manager}}
	solverStarted, consoleStarted := make(chan struct{}), make(chan struct{})
	solverRelease, consoleRelease := make(chan struct{}), make(chan struct{})
	var releaseOnce, consoleReleaseOnce sync.Once
	manager.solverLane.runForTest = func(op solverOperation) error {
		if op.Command != "Relax()" || !op.BreakFirst {
			t.Errorf("dedicated solver operation changed: %+v", op)
		}
		close(solverStarted)
		<-solverRelease
		return nil
	}
	manager.consoleLane.runForTest = func(op solverOperation) error {
		if op.Command != "print(1)" || op.BreakFirst {
			t.Errorf("console command changed or broke solver affinity: %+v", op)
		}
		close(consoleStarted)
		<-consoleRelease
		return nil
	}
	manager.breakSolverForTest = func() error {
		releaseOnce.Do(func() { close(solverRelease) })
		consoleReleaseOnce.Do(func() { close(consoleRelease) })
		return nil
	}
	console := &ConsoleState{ws: manager}

	rejected := invokeConsoleHandler(t, console.postConsoleCommand, `{"command":"runshell('forbidden')"}`)
	if rejected.Code != 400 {
		t.Fatalf("RunShell validation was bypassed: status=%d body=%s", rejected.Code, rejected.Body.String())
	}

	acceptedSolver := invokeSolverHandler(t, (&SolverState{ws: manager}).postSolverRelax)
	if acceptedSolver.Code != 202 {
		t.Fatalf("solver operation was not accepted: status=%d body=%s", acceptedSolver.Code, acceptedSolver.Body.String())
	}
	select {
	case <-solverStarted:
	case <-time.After(time.Second):
		t.Fatal("solver operation did not become active")
	}

	accepted := invokeConsoleHandler(t, console.postConsoleCommand, `{"command":"print(1)"}`)
	var ack solverOperationAck
	if err := json.Unmarshal(accepted.Body.Bytes(), &ack); err != nil {
		t.Fatal(err)
	}
	if accepted.Code != 202 || ack.OperationID != 1 || ack.Status != "pending" {
		t.Fatalf("console command was not accepted asynchronously: status=%d ack=%+v body=%s", accepted.Code, ack, accepted.Body.String())
	}
	select {
	case <-consoleStarted:
	case <-time.After(time.Second):
		t.Fatal("console operation did not run while solver was busy")
	}
	duplicate := invokeConsoleHandler(t, console.postConsoleCommand, `{"command":"Relax()"}`)
	if duplicate.Code != 409 {
		t.Fatalf("concurrent console operation was not rejected: status=%d body=%s", duplicate.Code, duplicate.Body.String())
	}
	if response := invokeSolverHandler(t, solver.postSolverBreak); response.Code != 200 {
		t.Fatalf("Break unavailable during active operations: status=%d body=%s", response.Code, response.Body.String())
	}
	waitSolverStatus(t, manager, solver, "cancelled")
	waitConsoleStatus(t, manager, consoleState, "cancelled")
}

func TestPendingJobsInBothLanesCanBeCancelledBeforeWorkersStart(t *testing.T) {
	manager := newWebSocketManager()
	t.Cleanup(func() { close(manager.broadcastStop) })
	solver := &SolverState{ws: manager, OperationStatus: "idle"}
	consoleState := &ConsoleState{ws: manager, OperationStatus: "idle"}
	manager.engineState = &EngineState{Solver: solver, Console: consoleState}
	gate := make(chan struct{})
	manager.solverLane.startGateForTest = gate
	manager.consoleLane.startGateForTest = gate
	manager.breakSolverForTest = func() error { return nil }
	var solverRan, consoleRan atomic.Bool
	manager.solverLane.runForTest = func(solverOperation) error { solverRan.Store(true); return nil }
	manager.consoleLane.runForTest = func(solverOperation) error { consoleRan.Store(true); return nil }
	if _, accepted := manager.reserveSolverOperation("Run(1)", true); !accepted {
		t.Fatal("solver operation was not reserved")
	}
	if _, accepted := manager.reserveConsoleOperation("print(1)"); !accepted {
		t.Fatal("console operation was not reserved while solver was pending")
	}
	if err := manager.breakSolver(); err != nil {
		t.Fatal(err)
	}
	close(gate)
	waitSolverStatus(t, manager, solver, "cancelled")
	waitConsoleStatus(t, manager, consoleState, "cancelled")
	if solverRan.Load() || consoleRan.Load() {
		t.Fatalf("Break allowed pending jobs to run: solver=%v console=%v", solverRan.Load(), consoleRan.Load())
	}
}

func invokeSolverHandler(t *testing.T, handler func(echo.Context) error) *httptest.ResponseRecorder {
	t.Helper()
	recorder := httptest.NewRecorder()
	context := echo.New().NewContext(httptest.NewRequest("POST", "/", strings.NewReader(`{}`)), recorder)
	context.Request().Header.Set("Content-Type", "application/json")
	if err := handler(context); err != nil {
		t.Fatal(err)
	}
	return recorder
}

func invokeConsoleHandler(t *testing.T, handler func(echo.Context) error, body string) *httptest.ResponseRecorder {
	t.Helper()
	recorder := httptest.NewRecorder()
	context := echo.New().NewContext(httptest.NewRequest("POST", "/", strings.NewReader(body)), recorder)
	context.Request().Header.Set("Content-Type", "application/json")
	if err := handler(context); err != nil {
		t.Fatal(err)
	}
	return recorder
}

func waitSolverStatus(t *testing.T, manager *WebSocketManager, solver *SolverState, status string) {
	t.Helper()
	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		manager.stateMu.Lock()
		current := solver.OperationStatus
		manager.stateMu.Unlock()
		if current == status {
			return
		}
		time.Sleep(time.Millisecond)
	}
	manager.stateMu.Lock()
	current := solver.OperationStatus
	manager.stateMu.Unlock()
	t.Fatalf("solver status %q, want %q", current, status)
}

func waitConsoleStatus(t *testing.T, manager *WebSocketManager, console *ConsoleState, status string) {
	t.Helper()
	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		manager.stateMu.Lock()
		current := console.OperationStatus
		manager.stateMu.Unlock()
		if current == status {
			return
		}
		time.Sleep(time.Millisecond)
	}
	manager.stateMu.Lock()
	current := console.OperationStatus
	manager.stateMu.Unlock()
	t.Fatalf("console operation status %q, want %q", current, status)
}
