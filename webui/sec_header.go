package webui

import (
	"github.com/mumax/3/engine"
)

type HeaderState struct {
	Path    string  `msgpack:"path"`
	Status  string  `msgpack:"status"`
	Version *string `msgpack:"version"`
}

func initHeaderAPI() *HeaderState {
	status := ""
	if engine.IsPaused() {
		status = "paused"
	} else {
		status = "running"
	}
	version := engine.VERSION
	return &HeaderState{
		Path:    engine.OD(),
		Status:  status,
		Version: &version,
	}
}

func (h *HeaderState) Update() {
	status := ""
	if engine.IsPaused() {
		status = "paused"
	} else {
		status = "running"
	}
	h.Path = engine.OD()
	h.Status = status
}
