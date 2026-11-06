package webui

import (
	"net/http"
	"strings"

	"github.com/labstack/echo/v5"

	"github.com/mumax/3/engine"
)

type ConsoleState struct {
	ws              *WebSocketManager
	Hist            string `msgpack:"hist"`
	OperationID     uint64 `msgpack:"operationId"`
	OperationStatus string `msgpack:"operationStatus"`
	OperationError  string `msgpack:"operationError"`
}

func (s *ConsoleState) Update() { s.Hist = engine.LogHistory() }

func initConsoleAPI(e *echo.Group, ws *WebSocketManager) *ConsoleState {
	state := &ConsoleState{
		ws:              ws,
		Hist:            engine.LogHistory(),
		OperationStatus: "idle",
	}
	e.POST("/api/console/command", state.postConsoleCommand)
	return state
}

func (s ConsoleState) postConsoleCommand(c *echo.Context) error {
	// TODO: return error if the command wrong
	type Request struct {
		Command string `msgpack:"command"`
	}

	req := new(Request)
	if err := c.Bind(req); err != nil {
		return c.JSON(http.StatusBadRequest, map[string]any{"error": "Invalid request payload"})
	}

	// Check if "RunShell" is in the command, case-insensitive
	if strings.Contains(strings.ToLower(req.Command), "runshell(") {
		return c.JSON(http.StatusBadRequest, map[string]any{"error": "RunShell command not allowed through the WebUI"})
	}

	return (SolverState{ws: s.ws}).acceptConsoleOperation(c, req.Command)
}
