package webui

import (
	"fmt"
	"net/http"

	"github.com/labstack/echo/v4"
	"github.com/mumax/3/cuda"
	"github.com/mumax/3/data"
	"github.com/mumax/3/engine"
)

// Bounds are source-cell edges: Start inclusive, End exclusive. Global resolution
// settings remain untouched, allowing return to the original full-domain view.
type PreviewRegion struct {
	Enabled   bool   `json:"enabled" msgpack:"enabled"`
	Start     [3]int `json:"start" msgpack:"start"`
	End       [3]int `json:"end" msgpack:"end"`
	Mode      string `json:"mode" msgpack:"mode"`
	Samples   [3]int `json:"samples" msgpack:"samples"`
	MaxPoints int    `json:"maxPoints" msgpack:"maxPoints"`
	RequestID string `json:"requestId" msgpack:"requestId"`
}

func (r PreviewRegion) size() (s [3]int) {
	for a := range s {
		s[a] = r.End[a] - r.Start[a]
	}
	return
}
func (r PreviewRegion) validate(mesh [3]int) error {
	if r.Mode != "native" && r.Mode != "custom" {
		return fmt.Errorf("Choose native or custom resolution")
	}
	if r.MaxPoints < 8 || r.MaxPoints > previewHardLimit || len(r.RequestID) > 128 {
		return fmt.Errorf("Invalid region budget or request ID")
	}
	for a := 0; a < 3; a++ {
		if r.Start[a] < 0 || r.End[a] > mesh[a] || r.End[a] <= r.Start[a] || r.Samples[a] < 1 || r.Samples[a] > mesh[a] {
			return fmt.Errorf("Invalid region bounds or sample count on axis %d", a)
		}
	}
	return nil
}
func (r *PreviewRegion) clamp(mesh [3]int) {
	for a, n := range mesh {
		width := min(max(r.End[a]-r.Start[a], 1), n)
		r.Start[a] = min(max(r.Start[a], 0), n-width)
		r.End[a] = r.Start[a] + width
		r.Samples[a] = min(max(r.Samples[a], 1), n)
	}
}
func (r PreviewRegion) requested() (s [3]int) {
	s = r.size()
	if r.Mode == "custom" {
		for a := range s {
			s[a] = min(max(r.Samples[a], 1), s[a])
		}
	}
	return
}
func (s *PreviewState) postPreviewRegion(c echo.Context) error {
	s.ws.stateMu.Lock()
	defer s.ws.stateMu.Unlock()
	var r PreviewRegion
	if err := c.Bind(&r); err != nil {
		return c.JSON(http.StatusBadRequest, echo.Map{"error": "Invalid region payload"})
	}
	if err := r.validate(engine.MeshSnapshotSize()); err != nil {
		return c.JSON(http.StatusBadRequest, echo.Map{"error": err.Error()})
	}
	if r.Enabled && !s.Region.Enabled {
		s.regionFullAllLayers = s.AllLayers
	}
	if !r.Enabled && s.Region.Enabled {
		s.AllLayers = s.regionFullAllLayers
	}
	s.Region = r
	if r.Enabled {
		s.AllLayers = true
	}
	// Capture changes atomically; the response and websocket frame acknowledge the
	// same request ID, so a slow response cannot overwrite a newer slider draft.
	s.ws.broadcastPreviewStateLocked()
	return c.JSON(http.StatusOK, s.Region)
}
func (s *PreviewState) updateRegion(src *data.Slice, components int) {
	s.Region.clamp(src.Size())
	s.AllLayers = true
	requested := s.Region.requested()
	budget := s.Region.MaxPoints
	if s.Region.Mode == "native" {
		budget = previewHardLimit
	}
	applied := resolvePreviewGrid(requested, [3][]int{}, budget, true)
	s.AppliedXChosenSize = applied[0]
	s.AppliedYChosenSize = applied[1]
	s.AppliedZChosenSize = applied[2]
	s.AppliedLayerStride = 1
	s.AutoDownscaled = applied != requested
	s.AutoDownscaleMessage = ""
	if s.AutoDownscaled {
		s.AutoDownscaleMessage = fmt.Sprintf("Window averaged from %v to %v within %d points. Select a smaller window for native cells.", requested, applied, budget)
	}
	cpu, gpu := s.previewBuffers(components, applied)
	cuda.ResizePreviewRegion(gpu, src, s.Region.Start, s.Region.size())
	s.previewHost.Copy(gpu)
	s.captureVectorCPU(cpu, 1, applied[2])
}
