package webui

import (
	"testing"
	"time"

	"github.com/labstack/echo/v5"
	"github.com/mumax/3/engine"
	"github.com/vmihailenco/msgpack/v5"
)

// Exercise successive detached WebSocket snapshots, including publication back
// into the live state. No CUDA work is needed to reproduce stale telemetry.
func TestSolverTelemetryRefreshesDetachedSnapshots(t *testing.T) {
	e := &EngineState{Solver: initSolverAPI(echo.New().Group(""), nil)}
	previous := *cloneEngineStateWithoutPreview(e).Solver
	defer func() {
		engine.NSteps = *previous.Steps
		engine.Time = *previous.Time
		engine.Dt_si = previous.Dt
		engine.LastErr = *previous.ErrPerStep
		engine.LastTorque = *previous.MaxTorque
		engine.FixDt = *previous.Fixdt
		engine.MinDt = *previous.Mindt
		engine.MaxDt = *previous.Maxdt
		engine.MaxErr = *previous.Maxerr
	}()
	var first *EngineState
	for i := 1; i <= 2; i++ {
		engine.NSteps = i * 100
		engine.Time = float64(i) * 1e-9
		engine.Dt_si = float64(i) * 2e-13
		engine.LastErr = float64(i) * 1e-6
		engine.LastTorque = float64(i) * 1e-3
		engine.FixDt = float64(i) * 1e-13
		engine.MinDt = float64(i) * 1e-15
		engine.MaxDt = float64(i) * 1e-11
		engine.MaxErr = float64(i) * 1e-5
		snapshot := cloneEngineStateWithoutPreview(e)
		done := make(chan struct{})
		go func() { snapshot.Solver.Update(); close(done) }()
		select {
		case task := <-engine.Inject:
			task()
			<-done
		case <-done: // The old implementation reads directly and leaves copies stale.
		case <-time.After(time.Second):
			t.Fatal("solver update did not complete or inject work")
		}
		payload, err := msgpack.Marshal(snapshot.WithoutPreview())
		if err != nil {
			t.Fatal(err)
		}
		var received EngineStateNoPreview
		if err := msgpack.Unmarshal(payload, &received); err != nil {
			t.Fatal(err)
		}
		s := received.Solver
		if s.Dt != engine.Dt_si || *s.Steps != engine.NSteps || *s.Time != engine.Time || *s.ErrPerStep != engine.LastErr || *s.MaxTorque != engine.LastTorque || *s.Fixdt != engine.FixDt || *s.Mindt != engine.MinDt || *s.Maxdt != engine.MaxDt || *s.Maxerr != engine.MaxErr {
			t.Fatalf("frame %d has stale telemetry: steps=%d time=%g, want steps=%d time=%g", i, *s.Steps, *s.Time, engine.NSteps, engine.Time)
		}
		storeEngineStateWithoutPreview(e, snapshot)
		if first == nil {
			first = snapshot
		}
	}
	if *first.Solver.Steps != 100 || *first.Solver.Time != 1e-9 {
		t.Fatal("refresh mutated a previously published snapshot")
	}
}
