package webui

import (
	"net/http"

	"github.com/labstack/echo/v4"

	"github.com/mumax/3/engine"
)

type MeshState struct {
	ws   *WebSocketManager
	Dx   float64 `msgpack:"dx"`
	Dy   float64 `msgpack:"dy"`
	Dz   float64 `msgpack:"dz"`
	Nx   int     `msgpack:"Nx"`
	Ny   int     `msgpack:"Ny"`
	Nz   int     `msgpack:"Nz"`
	Tx   float64 `msgpack:"Tx"`
	Ty   float64 `msgpack:"Ty"`
	Tz   float64 `msgpack:"Tz"`
	PBCx int     `msgpack:"PBCx"`
	PBCy int     `msgpack:"PBCy"`
	PBCz int     `msgpack:"PBCz"`
}

func initMeshAPI(e *echo.Group, ws *WebSocketManager) *MeshState {
	meshState := MeshState{ws: ws}
	meshState.Update()
	e.POST("/api/mesh", meshState.postMesh)
	return &meshState
}

func (m *MeshState) Update() {
	size, cell, world, pbc := engine.MeshSnapshot()
	m.Dx, m.Dy, m.Dz = cell[0], cell[1], cell[2]
	m.Nx, m.Ny, m.Nz = size[0], size[1], size[2]
	m.Tx, m.Ty, m.Tz = world[0], world[1], world[2]
	m.PBCx, m.PBCy, m.PBCz = pbc[0], pbc[1], pbc[2]
}

func (m *MeshState) postMesh(c echo.Context) error {
	return c.JSON(http.StatusNotImplemented, "")
}
