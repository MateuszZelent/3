package webui

import (
	"fmt"
	"math"
	"net/http"
	"strconv"

	"github.com/labstack/echo/v4"

	"github.com/mumax/3/engine"
)

type SolverState struct {
	ws              *WebSocketManager
	Type            string   `msgpack:"type"`
	Steps           *int     `msgpack:"steps"`
	Time            *float64 `msgpack:"time"`
	Dt              float64  `msgpack:"dt"`
	ErrPerStep      *float64 `msgpack:"errPerStep"`
	MaxTorque       *float64 `msgpack:"maxTorque"`
	Fixdt           *float64 `msgpack:"fixdt"`
	Mindt           *float64 `msgpack:"mindt"`
	Maxdt           *float64 `msgpack:"maxdt"`
	Maxerr          *float64 `msgpack:"maxerr"`
	OperationID     uint64   `msgpack:"operationId"`
	OperationStatus string   `msgpack:"operationStatus"`
	OperationError  string   `msgpack:"operationError"`
}

func initSolverAPI(e *echo.Group, ws *WebSocketManager) *SolverState {
	solverState := SolverState{
		ws:              ws,
		Type:            getSolverName(engine.SolverType()),
		Steps:           &engine.NSteps,
		Time:            &engine.Time,
		Dt:              engine.CurrentDt(),
		ErrPerStep:      &engine.LastErr,
		MaxTorque:       &engine.LastTorque,
		Fixdt:           &engine.FixDt,
		Mindt:           &engine.MinDt,
		Maxdt:           &engine.MaxDt,
		Maxerr:          &engine.MaxErr,
		OperationStatus: "idle",
	}

	e.POST("/api/solver/type", solverState.postSolverType)
	e.POST("/api/solver/run", solverState.postSolverRun)
	e.POST("/api/solver/steps", solverState.postSolverSteps)
	e.POST("/api/solver/minimize", solverState.postSolverMinimize)
	e.POST("/api/solver/relax", solverState.postSolverRelax)
	e.POST("/api/solver/break", solverState.postSolverBreak)
	e.POST("/api/solver/fixdt", solverState.postSolverFixDt)
	e.POST("/api/solver/mindt", solverState.postSolverMinDt)
	e.POST("/api/solver/maxdt", solverState.postSolverMaxDt)
	e.POST("/api/solver/maxerr", solverState.postSolverMaxErr)
	return &solverState
}

func (s *SolverState) Update() {
	s.Type = getSolverName(engine.SolverType())
	s.Dt = engine.CurrentDt()
}

func getSolverType(typeStr string) int {
	solvertypes := map[string]int{"bw_euler": -1, "euler": 1, "heun": 2, "rk23": 3, "rk4": 4, "rk45": 5, "rkf56": 6}
	if solver, ok := solvertypes[typeStr]; ok {
		return solver
	}
	return 0
}

func getSolverName(solver int) string {
	solvernames := map[int]string{-1: "bw_euler", 1: "euler", 2: "heun", 3: "rk23", 4: "rk4", 5: "rk45", 6: "rkf56"}
	if name, ok := solvernames[solver]; ok {
		return name
	}
	return ""
}

func (s SolverState) postSolverType(c echo.Context) error {
	type Response struct {
		Type string `msgpack:"type"`
	}

	res := new(Response)
	if err := c.Bind(res); err != nil {
		return c.JSON(http.StatusBadRequest, echo.Map{"error": "Invalid request payload"})
	}
	engine.InjectAndWait(func() {
		solver := getSolverType(res.Type)

		// euler must have fixed time step
		if solver == engine.EULER && engine.FixDt == 0 {
			engine.EvalTryRecover("FixDt = 1e-15")
		}
		if solver == engine.BACKWARD_EULER && engine.FixDt == 0 {
			engine.EvalTryRecover("FixDt = 1e-13")
		}

		engine.EvalTryRecover(fmt.Sprint("SetSolver(", solver, ")"))
	})

	s.ws.broadcastEngineStateWithoutPreview()
	return c.JSON(http.StatusOK, engine.SolverType())
}

func (s SolverState) postSolverRun(c echo.Context) error {
	type Request struct {
		Runtime string `msgpack:"runtime"`
	}

	req := new(Request)
	if err := c.Bind(req); err != nil {
		return c.JSON(http.StatusBadRequest, echo.Map{
			"error": "Invalid request payload: " + err.Error(),
		})
	}
	duration, err := strconv.ParseFloat(req.Runtime, 64)
	if err != nil || math.IsNaN(duration) || math.IsInf(duration, 0) || duration < 0 {
		return c.JSON(http.StatusBadRequest, echo.Map{"error": "Runtime must be a finite, nonnegative number"})
	}
	return s.acceptLongOperation(c, fmt.Sprintf("Run(%.17g)", duration))
}

func (s SolverState) postSolverSteps(c echo.Context) error {
	type Request struct {
		Steps string `msgpack:"steps"`
	}

	req := new(Request)
	if err := c.Bind(req); err != nil {
		return c.JSON(http.StatusBadRequest, echo.Map{"error": "Invalid request payload"})
	}
	steps, err := strconv.Atoi(req.Steps)
	if err != nil || steps < 0 {
		return c.JSON(http.StatusBadRequest, echo.Map{"error": "Steps must be a nonnegative integer"})
	}
	return s.acceptLongOperation(c, fmt.Sprintf("Steps(%d)", steps))
}

func (s SolverState) postSolverRelax(c echo.Context) error {
	return s.acceptLongOperation(c, "Relax()")
}

func (s SolverState) postSolverMinimize(c echo.Context) error {
	return s.acceptLongOperation(c, "Minimize()")
}

func (s SolverState) postSolverBreak(c echo.Context) error {
	if err := s.ws.breakSolver(); err != nil {
		return c.JSON(http.StatusInternalServerError, echo.Map{"error": err.Error()})
	}
	s.ws.broadcastEngineStateWithoutPreview()
	return c.JSON(http.StatusOK, "")
}

func (s SolverState) acceptLongOperation(c echo.Context, command string) error {
	ack, accepted := s.ws.reserveSolverOperation(command, true)
	return writeOperationAck(c, ack, accepted)
}

func (s SolverState) acceptConsoleOperation(c echo.Context, command string) error {
	ack, accepted := s.ws.reserveConsoleOperation(command)
	return writeOperationAck(c, ack, accepted)
}

func writeOperationAck(c echo.Context, ack solverOperationAck, accepted bool) error {
	if !accepted {
		if ack.OperationID != 0 {
			return c.JSON(http.StatusConflict, echo.Map{
				"error":       "An operation in this lane is already pending or running",
				"operationId": ack.OperationID,
				"status":      ack.Status,
			})
		}
		return c.JSON(http.StatusServiceUnavailable, echo.Map{"error": "Operation queue is unavailable"})
	}
	return c.JSON(http.StatusAccepted, ack)
}

func (s SolverState) postSolverFixDt(c echo.Context) error {
	type Request struct {
		Fixdt float64 `msgpack:"fixdt"`
	}

	req := new(Request)
	if err := c.Bind(req); err != nil {
		return c.JSON(http.StatusBadRequest, echo.Map{"error": "Invalid request payload"})
	}

	engine.InjectAndWait(func() { engine.EvalTryRecover("FixDt = " + strconv.FormatFloat(req.Fixdt, 'f', -1, 64)) })
	s.ws.broadcastEngineStateWithoutPreview()
	return c.JSON(http.StatusOK, "")
}

func (s SolverState) postSolverMinDt(c echo.Context) error {
	type Request struct {
		Mindt float64 `msgpack:"mindt"`
	}

	req := new(Request)
	if err := c.Bind(req); err != nil {
		return c.JSON(http.StatusBadRequest, echo.Map{"error": "Invalid request payload"})
	}

	engine.InjectAndWait(func() { engine.EvalTryRecover("MinDt = " + strconv.FormatFloat(req.Mindt, 'f', -1, 64)) })
	s.ws.broadcastEngineStateWithoutPreview()
	return c.JSON(http.StatusOK, "")
}

func (s SolverState) postSolverMaxDt(c echo.Context) error {
	type Request struct {
		Maxdt float64 `msgpack:"maxdt"`
	}

	req := new(Request)
	if err := c.Bind(req); err != nil {
		return c.JSON(http.StatusBadRequest, echo.Map{"error": "Invalid request payload"})
	}

	engine.InjectAndWait(func() { engine.EvalTryRecover("MaxDt = " + strconv.FormatFloat(req.Maxdt, 'f', -1, 64)) })
	s.ws.broadcastEngineStateWithoutPreview()
	return c.JSON(http.StatusOK, "")
}

func (s SolverState) postSolverMaxErr(c echo.Context) error {
	type Request struct {
		Maxerr float64 `msgpack:"maxerr"`
	}

	req := new(Request)
	if err := c.Bind(req); err != nil {
		return c.JSON(http.StatusBadRequest, echo.Map{"error": "Invalid request payload"})
	}

	engine.InjectAndWait(func() { engine.EvalTryRecover("MaxErr = " + strconv.FormatFloat(req.Maxerr, 'f', -1, 64)) })
	s.ws.broadcastEngineStateWithoutPreview()
	return c.JSON(http.StatusOK, "")
}
