package webui

import (
	"net/http"

	"github.com/labstack/echo/v4"
	"github.com/mumax/3/cuda"
	"github.com/mumax/3/data"
)

func previewPlaneAxes(plane string) (u, v, normal int) {
	switch plane {
	case "yz":
		return 1, 2, 0
	case "xz":
		return 0, 2, 1
	default:
		return 0, 1, 2
	}
}

func validPreviewPlane(plane string) bool { return plane == "xy" || plane == "yz" || plane == "xz" }

func (s *PreviewState) refreshPlaneMetadata() {
	if !validPreviewPlane(s.Plane) {
		s.Plane = "xy"
	}
	u, v, n := previewPlaneAxes(s.Plane)
	if s.previewMeshSize[n] < 1 {
		return
	}
	s.SliceIndex = min(max(s.sliceIndices[n], 0), s.previewMeshSize[n]-1)
	s.sliceIndices[n] = s.SliceIndex
	s.PlaneUPossibleSizes = possiblePreviewXYSizes(s.previewMeshSize[u])
	s.PlaneVPossibleSizes = possiblePreviewXYSizes(s.previewMeshSize[v])
	s.PlaneUChosenSize = s.planeChosenSize[u]
	s.PlaneVChosenSize = s.planeChosenSize[v]
}

func (s *PreviewState) resolvePlaneSizing() previewSizing {
	planar := PreviewState{
		XChosenSize: s.PlaneUChosenSize, YChosenSize: s.PlaneVChosenSize,
		XPossibleSizes: s.PlaneUPossibleSizes, YPossibleSizes: s.PlaneVPossibleSizes,
		MaxPoints: s.MaxPoints, AutoScaleEnabled: s.AutoScaleEnabled,
	}
	return planar.resolvePreviewSizing(1)
}

func (s *PreviewState) updatePlaneScalar(input *data.Slice) {
	sizing := s.resolvePlaneSizing()
	s.applyResolvedSizing(sizing)
	s.AppliedPlaneUSize, s.AppliedPlaneVSize = sizing.AppliedX, sizing.AppliedY
	cpu, gpu := s.previewBuffers(1, [3]int{sizing.AppliedX, sizing.AppliedY, 1})
	component := 0
	if input.NComp() > 1 {
		component = s.getComponent()
	}
	u, v, _ := previewPlaneAxes(s.Plane)
	layer := s.SliceIndex
	if s.AllLayers {
		layer = -1
	}
	// Geometry uses the same plane and reduction, so occupied zero-valued cells
	// remain visible while empty projected columns are excluded.
	if !contains(s.globalQuantities, s.Quantity) {
		s.ensureMask(sizing.AppliedX, sizing.AppliedY)
	}
	cuda.ResizePreviewPlane(gpu, input.Comp(component), u, v, layer)
	s.previewHost.Copy(gpu)
	s.pendingScalar = cpu
}

// Atomically configure the plane, reduction and slice. Keep the last slice of
// each axis when switching planes; a mesh change clamps them before capture.
func (s *PreviewState) postPreviewSection(c echo.Context) error {
	s.ws.stateMu.Lock()
	defer s.ws.stateMu.Unlock()
	var req struct {
		Plane      string `msgpack:"plane"`
		Mode       string `msgpack:"mode"`
		SliceIndex *int   `msgpack:"sliceIndex"`
	}
	if err := c.Bind(&req); err != nil {
		return c.JSON(http.StatusBadRequest, echo.Map{"error": "Invalid request payload"})
	}
	if !s.addPossibleDownscaleSizes() {
		return c.JSON(http.StatusBadRequest, echo.Map{"error": "Mesh is not initialized"})
	}
	plane := s.Plane
	if req.Plane != "" {
		plane = req.Plane
	}
	if !validPreviewPlane(plane) || (req.Mode != "" && req.Mode != "single" && req.Mode != "average") {
		return c.JSON(http.StatusBadRequest, echo.Map{"error": "Invalid plane or reduction mode"})
	}
	_, _, normal := previewPlaneAxes(plane)
	if req.SliceIndex != nil && (*req.SliceIndex < 0 || *req.SliceIndex >= s.previewMeshSize[normal]) {
		return c.JSON(http.StatusBadRequest, echo.Map{"error": "Slice outside mesh"})
	}
	s.Plane = plane
	if req.SliceIndex != nil {
		s.sliceIndices[normal] = *req.SliceIndex
	}
	if req.Mode != "" {
		s.AllLayers = req.Mode == "average"
	}
	s.refreshPlaneMetadata()
	s.Refresh = true
	s.ws.broadcastPreviewStateLocked()
	return c.JSON(http.StatusOK, nil)
}

func (s *PreviewState) postPlaneResolution(c echo.Context) error {
	s.ws.stateMu.Lock()
	defer s.ws.stateMu.Unlock()
	var req struct {
		USize *int `msgpack:"uSize"`
		VSize *int `msgpack:"vSize"`
	}
	if err := c.Bind(&req); err != nil {
		return c.JSON(http.StatusBadRequest, echo.Map{"error": "Invalid request payload"})
	}
	if !s.addPossibleDownscaleSizes() {
		return c.JSON(http.StatusBadRequest, echo.Map{"error": "Mesh is not initialized"})
	}
	s.refreshPlaneMetadata()
	if (req.USize != nil && !containsInt(s.PlaneUPossibleSizes, *req.USize)) || (req.VSize != nil && !containsInt(s.PlaneVPossibleSizes, *req.VSize)) {
		return c.JSON(http.StatusBadRequest, echo.Map{"error": "Resolution outside plane presets"})
	}
	u, v, _ := previewPlaneAxes(s.Plane)
	if req.USize != nil {
		s.planeChosenSize[u] = *req.USize
	}
	if req.VSize != nil {
		s.planeChosenSize[v] = *req.VSize
	}
	s.refreshPlaneMetadata()
	s.Refresh = true
	s.ws.broadcastPreviewStateLocked()
	return c.JSON(http.StatusOK, nil)
}
