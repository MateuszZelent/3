package webui

import (
	"testing"

	"github.com/labstack/echo/v4"
)

func TestInitParameterAPIDoesNotRequireCUDAContext(t *testing.T) {
	e := echo.New()
	state := initParameterAPI(e.Group(""), newWebSocketManager())
	if len(state.Regions) != 1 || state.Regions[0] != 0 {
		t.Fatalf("initial regions = %v, want [0]", state.Regions)
	}
}
