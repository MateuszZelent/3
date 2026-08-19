package webui

import (
	"net/http"

	"github.com/labstack/echo/v4"

	"github.com/mumax/3/engine"
	"github.com/mumax/3/log"
)

type Field struct {
	Name        string `msgpack:"name"`
	Value       string `msgpack:"value"`
	Description string `msgpack:"description"`
	Changed     bool   `msgpack:"changed"`
}

func (f *Field) IsDefault(value string) bool {
	return true
}

type ParametersState struct {
	ws             *WebSocketManager
	Regions        []int   `msgpack:"regions"`
	Fields         []Field `msgpack:"fields"`
	SelectedRegion int     `msgpack:"selectedRegion"`
}

func initParameterAPI(e *echo.Group, ws *WebSocketManager) *ParametersState {
	parametersState := ParametersState{
		ws: ws,
		// Reading the region map downloads CUDA memory. Start with region zero;
		// the first WebSocket update refreshes this state through the engine's
		// injection queue, on the CUDA-owning OS thread.
		Regions:        []int{0},
		SelectedRegion: 0,
	}
	parametersState.getFields()
	e.POST("/api/parameter/selected-region", parametersState.postSelectParameterRegion)
	return &parametersState
}

func (s *ParametersState) Update() {
	engine.InjectAndWait(func() {
		s.Regions = engine.ExistingRegionIndices()
		s.getFields()
	})
}

func (s *ParametersState) getFields() {
	parameters := engine.WebParameters(s.SelectedRegion)
	fields := make([]Field, 0, len(parameters))
	for _, param := range parameters {
		field := Field{
			Name:        param.Name,
			Value:       param.Value,
			Description: param.Description,
			Changed:     param.Changed,
		}
		fields = append(fields, field)
	}
	s.Fields = fields
}

func (s *ParametersState) postSelectParameterRegion(c echo.Context) error {
	type Request struct {
		SelectedRegion int `msgpack:"selectedRegion"`
	}
	req := new(Request)
	if err := c.Bind(req); err != nil {
		log.Log.Err("%v", err)
		return c.JSON(http.StatusBadRequest, echo.Map{"error": "Invalid request payload"})
	}

	s.SelectedRegion = req.SelectedRegion
	s.ws.setPreviewRefresh(true)
	s.ws.broadcastEngineState()
	return c.JSON(http.StatusOK, nil)
}
