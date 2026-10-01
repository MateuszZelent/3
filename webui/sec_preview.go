package webui

import (
	"encoding/binary"
	"fmt"
	"math"
	"net/http"
	"sort"
	"time"

	"github.com/labstack/echo/v4"

	"github.com/mumax/3/cuda"
	"github.com/mumax/3/data"
	"github.com/mumax/3/engine"
	"github.com/mumax/3/log"
)

const previewHardLimit = 1000000

type PreviewState struct {
	previewHost           *cuda.PreviewHostBuffer
	occupancyHost         *cuda.PreviewHostBuffer
	pendingScalar         *data.Slice
	pendingCPU            *data.Slice
	pendingStride         int
	pendingDepth          int
	pendingOccupancy      *data.Slice
	occupancyCPU          *data.Slice
	occupancyGPU          *data.Slice
	sparePositions        []Vector3i
	cachedTopologyKey     string
	ServerVectorCount     int     `msgpack:"serverVectorCount"`
	TransportSampling     int     `msgpack:"transportSampling"`
	Sequence              uint64  `msgpack:"sequence"`
	Step                  int     `msgpack:"step"`
	Timestamp             int64   `msgpack:"timestamp"`
	NormScale             float64 `msgpack:"normScale"`
	FixedScale            float64 `msgpack:"fixedScale"`
	InvalidCount          int     `msgpack:"invalidCount"`
	CaptureMs             float64 `msgpack:"captureMs"`
	ProcessMs             float64 `msgpack:"processMs"`
	HardLimit             int     `msgpack:"hardLimit"`
	ws                    *WebSocketManager
	globalQuantities      []string
	layerMask             [][]float32
	planeChosenSize       [3]int
	sliceIndices          [3]int
	previewCPU            *data.Slice
	previewGPU            *data.Slice
	previewBufferSize     [3]int
	previewBufferNComp    int
	previewMeshSize       [3]int
	cachedPositionsBinary []byte
	Quantity              string       `msgpack:"quantity"`
	Unit                  string       `msgpack:"unit"`
	Component             string       `msgpack:"component"`
	Layer                 int          `msgpack:"layer"`
	Plane                 string       `msgpack:"plane"`
	SliceIndex            int          `msgpack:"sliceIndex"`
	PlaneUPossibleSizes   []int        `msgpack:"planeUPossibleSizes"`
	PlaneVPossibleSizes   []int        `msgpack:"planeVPossibleSizes"`
	PlaneUChosenSize      int          `msgpack:"planeUChosenSize"`
	PlaneVChosenSize      int          `msgpack:"planeVChosenSize"`
	AppliedPlaneUSize     int          `msgpack:"appliedPlaneUSize"`
	AppliedPlaneVSize     int          `msgpack:"appliedPlaneVSize"`
	AllLayers             bool         `msgpack:"allLayers"`
	Type                  string       `msgpack:"type"`
	VectorFieldValues     []Vector3f   `msgpack:"-"`
	VectorFieldPositions  []Vector3i   `msgpack:"-"`
	VectorValuesBinary    []byte       `msgpack:"vectorValuesBinary,omitempty"`
	VectorPositionsBinary []byte       `msgpack:"vectorPositionsBinary,omitempty"`
	VectorOccupancy       []byte       `msgpack:"vectorOccupancy,omitempty"`
	VectorCount           int          `msgpack:"vectorCount"`
	TopologyRevision      uint64       `msgpack:"topologyRevision"`
	ScalarField           [][3]float32 `msgpack:"scalarField"`
	Min                   float32      `msgpack:"min"`
	Max                   float32      `msgpack:"max"`
	Refresh               bool         `msgpack:"refresh"`
	NComp                 int          `msgpack:"nComp"`

	MaxPoints            int    `msgpack:"maxPoints"`
	DataPointsCount      int    `msgpack:"dataPointsCount"`
	XPossibleSizes       []int  `msgpack:"xPossibleSizes"`
	YPossibleSizes       []int  `msgpack:"yPossibleSizes"`
	ZPossibleSizes       []int  `msgpack:"zPossibleSizes"`
	ZChosenSize          int    `msgpack:"zChosenSize"`
	AppliedZChosenSize   int    `msgpack:"appliedZChosenSize"`
	XChosenSize          int    `msgpack:"xChosenSize"`
	YChosenSize          int    `msgpack:"yChosenSize"`
	AppliedXChosenSize   int    `msgpack:"appliedXChosenSize"`
	AppliedYChosenSize   int    `msgpack:"appliedYChosenSize"`
	AppliedLayerStride   int    `msgpack:"appliedLayerStride"`
	AutoScaleEnabled     bool   `msgpack:"autoScaleEnabled"`
	AutoDownscaled       bool   `msgpack:"autoDownscaled"`
	AutoDownscaleMessage string `msgpack:"autoDownscaleMessage"`
}

type Vector3f struct {
	X float32 `msgpack:"x"`
	Y float32 `msgpack:"y"`
	Z float32 `msgpack:"z"`
}

type Vector3i struct {
	X int `msgpack:"x"`
	Y int `msgpack:"y"`
	Z int `msgpack:"z"`
}

func sameVectorPositions(a, b []Vector3i) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func packVectorValues(dst []byte, values []Vector3f) []byte {
	required := len(values) * 3 * 4
	if cap(dst) < required {
		dst = make([]byte, required)
	} else {
		dst = dst[:required]
	}
	for i, value := range values {
		offset := i * 12
		binary.LittleEndian.PutUint32(dst[offset:], math.Float32bits(value.X))
		binary.LittleEndian.PutUint32(dst[offset+4:], math.Float32bits(value.Y))
		binary.LittleEndian.PutUint32(dst[offset+8:], math.Float32bits(value.Z))
	}
	return dst
}

func packVectorPositions(dst []byte, positions []Vector3i) []byte {
	required := len(positions) * 3 * 4
	if cap(dst) < required {
		dst = make([]byte, required)
	} else {
		dst = dst[:required]
	}
	for i, position := range positions {
		offset := i * 12
		binary.LittleEndian.PutUint32(dst[offset:], uint32(int32(position.X)))
		binary.LittleEndian.PutUint32(dst[offset+4:], uint32(int32(position.Y)))
		binary.LittleEndian.PutUint32(dst[offset+8:], uint32(int32(position.Z)))
	}
	return dst
}

func (s *PreviewState) setVectorPayload(values []Vector3f, positions []Vector3i) {
	key := fmt.Sprintf("%s/%d/%v/%d/%d/%d/%d/%t", s.Quantity, s.Layer, s.previewMeshSize, s.AppliedXChosenSize, s.AppliedYChosenSize, s.AppliedZChosenSize, s.AppliedLayerStride, s.AllLayers)
	topologyChanged := key != s.cachedTopologyKey || !sameVectorPositions(s.VectorFieldPositions, positions)
	s.cachedTopologyKey = key
	s.VectorFieldValues = values
	s.VectorOccupancy = nil
	previousPositions := s.VectorFieldPositions
	s.VectorFieldPositions = positions
	s.sparePositions = previousPositions
	s.VectorCount = len(values)
	s.VectorValuesBinary = packVectorValues(s.VectorValuesBinary, values)

	if topologyChanged {
		s.TopologyRevision++
		s.cachedPositionsBinary = packVectorPositions(s.cachedPositionsBinary, positions)
	}
	// Each frame is self-contained. The websocket writer deliberately replaces
	// stale queued frames with the newest one, so a topology-changing frame may
	// be skipped for a slow client. Sending the compact position buffer with
	// every frame keeps the newest frame independently decodable.
	s.VectorPositionsBinary = s.cachedPositionsBinary
	s.ScalarField = nil
	s.DataPointsCount = len(values)
}

func (s *PreviewState) clearVectorPayload() {
	if s.VectorCount > 0 {
		s.TopologyRevision++
	}
	s.cachedTopologyKey = ""
	s.VectorFieldValues = nil
	s.VectorFieldPositions = nil
	s.VectorOccupancy = nil
	s.VectorValuesBinary = nil
	s.VectorPositionsBinary = nil
	s.cachedPositionsBinary = nil
	s.VectorCount = 0
}

func initPreviewAPI(e *echo.Group, ws *WebSocketManager) *PreviewState {
	previewState := &PreviewState{
		Quantity:             "m",
		Component:            "3D",
		Layer:                0,
		Plane:                "xy",
		AllLayers:            false,
		MaxPoints:            262144,
		HardLimit:            previewHardLimit,
		Type:                 "3D",
		VectorFieldValues:    nil,
		VectorFieldPositions: nil,
		ScalarField:          nil,
		Min:                  0,
		Max:                  0,
		Refresh:              true,
		NComp:                3,
		AutoScaleEnabled:     true,
		DataPointsCount:      0,
		XPossibleSizes:       nil,
		YPossibleSizes:       nil,
		XChosenSize:          engine.MeshSnapshotSize()[0],
		YChosenSize:          engine.MeshSnapshotSize()[1],
		ws:                   ws,
		globalQuantities:     []string{"B_demag", "B_ext", "B_eff", "B_oersted", "Edens_demag", "Edens_ext", "Edens_eff", "geom", "SpongeAlpha"},
	}
	previewState.addPossibleDownscaleSizes()
	previewState.AppliedXChosenSize = previewState.XChosenSize
	previewState.AppliedYChosenSize = previewState.YChosenSize
	previewState.AppliedLayerStride = 1
	e.POST("/api/preview/section", previewState.postPreviewSection)
	e.POST("/api/preview/planeResolution", previewState.postPlaneResolution)
	e.POST("/api/preview/fullResolution", previewState.postFullResolution)
	e.POST("/api/preview/scale", previewState.postPreviewScale)
	e.POST("/api/preview/component", previewState.postPreviewComponent)
	e.POST("/api/preview/quantity", previewState.postPreviewQuantity)
	e.POST("/api/preview/layer", previewState.postPreviewLayer)
	e.POST("/api/preview/maxpoints", previewState.postPreviewMaxPoints)
	e.POST("/api/preview/refresh", previewState.postPreviewRefresh)
	e.POST("/api/preview/XChosenSize", previewState.postXChosenSize)
	e.POST("/api/preview/YChosenSize", previewState.postYChosenSize)
	e.POST("/api/preview/ZChosenSize", previewState.postZChosenSize)
	e.POST("/api/preview/allLayers", previewState.postAllLayers)
	e.POST("/api/preview/autoScaleEnabled", previewState.postAutoScaleEnabled)

	return previewState
}

func (s *PreviewState) getQuantity() engine.Quantity {
	quantity, exists := engine.AvailableQuantities()[s.Quantity]
	if !exists {
		log.Log.Err("Quantity not found: %v", s.Quantity)
	}
	return quantity
}

func (s *PreviewState) getComponent() int {
	return compStringToIndex(s.Component)
}

func (s *PreviewState) quantitySlice(quantity engine.Quantity) (*data.Slice, bool) {
	if buffered, ok := quantity.(interface {
		Slice() (*data.Slice, bool)
	}); ok {
		return buffered.Slice()
	}
	return engine.ValueOf(quantity), true
}

func (s *PreviewState) previewBuffers(nComp int, size [3]int) (*data.Slice, *data.Slice) {
	if s.previewCPU != nil && s.previewGPU != nil && s.previewBufferNComp == nComp && s.previewBufferSize == size {
		return s.previewCPU, s.previewGPU
	}
	if s.previewGPU != nil {
		s.previewGPU.Free()
	}
	if s.previewHost != nil {
		s.previewHost.Free()
	}
	s.previewHost = cuda.NewPreviewHostBuffer(nComp, size)
	s.previewCPU = s.previewHost.Slice
	s.previewGPU = cuda.NewSlice(nComp, size)
	s.previewBufferNComp = nComp
	s.previewBufferSize = size
	return s.previewCPU, s.previewGPU
}

func (s *PreviewState) Update() {
	start := time.Now()
	s.pendingScalar = nil
	s.pendingCPU = nil
	s.pendingOccupancy = nil

	engine.InjectAndWait(func() {
		if !s.addPossibleDownscaleSizes() {
			return
		}
		s.Layer = min(max(s.Layer, 0), max(engine.MeshSnapshotSize()[2]-1, 0))
		s.refreshPlaneMetadata()
		s.Step = engine.NSteps
		s.UpdateQuantityBuffer()
	})
	s.CaptureMs = float64(time.Since(start).Microseconds()) / 1000
	start = time.Now()
	// The broadcaster holds stateMu through capture and processing. Reusable CPU
	// buffers cannot be overwritten by another capture while processing them here.
	if s.pendingCPU != nil {
		s.processVectorSnapshot()
	}
	if s.pendingScalar != nil {
		s.UpdateScalarField(s.pendingScalar.Scalars())
	}
	s.ProcessMs = float64(time.Since(start).Microseconds()) / 1000
	s.Sequence++
	s.Timestamp = time.Now().UnixMilli()
}

type previewSizing struct {
	RequestedX      int
	RequestedY      int
	AppliedX        int
	AppliedY        int
	RequestedDepth  int
	AppliedDepth    int
	LayerStride     int
	RequestedPoints int
	AppliedPoints   int
	AutoDownscaled  bool
}

func (s *PreviewState) UpdateQuantityBuffer() {
	defer func() {
		if r := recover(); r != nil {
			log.Log.Warn("Recovered from panic in UpdateQuantityBuffer: %v", r)
			s.ScalarField = nil
			s.clearVectorPayload()
			s.DataPointsCount = 0
		}
	}()

	if s.XChosenSize == 0 || s.YChosenSize == 0 {
		log.Log.Debug("XChosenSize or YChosenSize is 0")
		return
	}

	componentCount := 1
	if s.Type == "3D" {
		componentCount = s.NComp
	}
	quantity := s.getQuantity()
	s.Unit = engine.UnitOf(quantity)
	GPUIn, recycleInput := s.quantitySlice(quantity)
	if recycleInput {
		defer cuda.Recycle(GPUIn)
	}

	if s.Type == "2D" {
		s.updatePlaneScalar(GPUIn)
		return
	}

	depthLayers := 1
	if s.AllLayers && s.Type == "3D" {
		depthLayers = maxInt(GPUIn.Size()[2], 1)
	}
	sizing := s.resolvePreviewSizing(depthLayers)
	s.applyResolvedSizing(sizing)

	if sizing.AppliedX == 0 || sizing.AppliedY == 0 {
		log.Log.Debug("Applied preview size is 0")
		return
	}

	if s.AllLayers && s.Type == "3D" {
		s.updateAllLayers(GPUIn, componentCount, sizing.LayerStride)
		return
	}

	CPUOut, GPUOut := s.previewBuffers(componentCount, [3]int{sizing.AppliedX, sizing.AppliedY, 1})
	cuda.ResizePreview(GPUOut, GPUIn, s.Layer, 1)
	s.previewHost.Copy(GPUOut)
	s.captureVectorCPU(CPUOut, 1, 1)
}

func (s *PreviewState) normalizeVectors(f *data.Slice) {
	a := f.Vectors()
	maxnormSquared := 0.0
	for i := range a[0] {
		for j := range a[0][i] {
			for k := range a[0][i][j] {
				x, y, z := a[0][i][j][k], a[1][i][j][k], a[2][i][j][k]
				normSquared := float64(x)*float64(x) + float64(y)*float64(y) + float64(z)*float64(z)
				if normSquared > maxnormSquared {
					maxnormSquared = normSquared
				}
			}
		}
	}
	if maxnormSquared == 0 {
		return
	}
	factor := float32(1 / math.Sqrt(maxnormSquared))

	for i := range a[0] {
		for j := range a[0][i] {
			for k := range a[0][i][j] {
				a[0][i][j][k] *= factor
				a[1][i][j][k] *= factor
				a[2][i][j][k] *= factor
			}
		}
	}
}

func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}

func ceilDiv(n, d int) int {
	if d <= 0 {
		return 0
	}
	return (n + d - 1) / d
}

func floorAllowedSize(arr []int, target int) int {
	if target <= 1 || len(arr) == 0 {
		return maxInt(target, 1)
	}
	best := 1
	for _, value := range arr {
		if value > target {
			break
		}
		best = value
	}
	return best
}

func (s *PreviewState) resolvePreviewSizing(depthLayers int) previewSizing {
	requestedX := maxInt(s.XChosenSize, 1)
	requestedY := maxInt(s.YChosenSize, 1)
	if len(s.XPossibleSizes) > 0 && !containsInt(s.XPossibleSizes, requestedX) {
		requestedX = closestInArray(s.XPossibleSizes, requestedX)
	}
	if len(s.YPossibleSizes) > 0 && !containsInt(s.YPossibleSizes, requestedY) {
		requestedY = closestInArray(s.YPossibleSizes, requestedY)
	}

	depthLayers = maxInt(depthLayers, 1)
	requestedZ := depthLayers
	zSizes := possibleDownscaleSizes(depthLayers)
	if s.AllLayers && s.Type == "3D" && s.ZChosenSize > 0 {
		requestedZ = floorAllowedSize(zSizes, min(s.ZChosenSize, depthLayers))
	}
	requested := [3]int{requestedX, requestedY, requestedZ}
	budget := min(maxInt(s.MaxPoints, 8), previewHardLimit)
	enabled := s.AutoScaleEnabled
	if !enabled {
		budget = previewHardLimit
		enabled = float64(requestedX)*float64(requestedY)*float64(requestedZ) > float64(previewHardLimit)
	}
	// XY area averaging supports any destination size. Restricting automatic
	// sizes to slider presets can waste most of the budget in large jumps.
	applied := resolvePreviewGrid(requested, [3][]int{nil, nil, zSizes}, budget, enabled)
	return previewSizing{
		RequestedX: requestedX, RequestedY: requestedY,
		AppliedX: applied[0], AppliedY: applied[1],
		RequestedDepth: requestedZ, AppliedDepth: applied[2],
		LayerStride:     depthLayers / applied[2],
		RequestedPoints: requestedX * requestedY * requestedZ,
		AppliedPoints:   applied[0] * applied[1] * applied[2],
		AutoDownscaled:  applied != requested,
	}
}

func (s *PreviewState) applyResolvedSizing(sizing previewSizing) {
	s.AppliedXChosenSize = sizing.AppliedX
	s.AppliedYChosenSize = sizing.AppliedY
	s.AppliedLayerStride = sizing.LayerStride
	s.AppliedZChosenSize = sizing.AppliedDepth
	s.AutoDownscaled = sizing.AutoDownscaled
	if !sizing.AutoDownscaled {
		s.AutoDownscaleMessage = ""
		return
	}

	requestedShape := fmt.Sprintf("%dx%d", sizing.RequestedX, sizing.RequestedY)
	appliedShape := fmt.Sprintf("%dx%d", sizing.AppliedX, sizing.AppliedY)
	if sizing.RequestedDepth > 1 {
		requestedShape = fmt.Sprintf("%s x %d", requestedShape, sizing.RequestedDepth)
		appliedShape = fmt.Sprintf("%s x %d", appliedShape, sizing.AppliedDepth)
	}
	limit := min(maxInt(s.MaxPoints, 8), previewHardLimit)
	if !s.AutoScaleEnabled {
		limit = previewHardLimit
	}
	reason := "Preview auto-scaled"
	if !s.AutoScaleEnabled {
		reason = "Preview reduced by safety limit"
	}
	message := fmt.Sprintf("%s from %s to %s to stay within %d points", reason, requestedShape, appliedShape, limit)
	if sizing.LayerStride > 1 {
		message = fmt.Sprintf("%s (sampling every %d layer)", message, sizing.LayerStride)
	}
	s.AutoDownscaleMessage = message
}

func (s *PreviewState) updateAllLayers(GPUIn *data.Slice, componentCount int, layerStride int) {
	stride := max(layerStride, 1)
	depth := ceilDiv(GPUIn.Size()[2], stride)
	CPUOut, GPUOut := s.previewBuffers(componentCount, [3]int{s.AppliedXChosenSize, s.AppliedYChosenSize, depth})
	cuda.ResizePreview(GPUOut, GPUIn, -1, stride)
	s.previewHost.Copy(GPUOut)
	s.captureVectorCPU(CPUOut, stride, depth)
}

func (s *PreviewState) captureVectorCPU(cpu *data.Slice, stride, depth int) {
	s.pendingCPU = cpu
	s.pendingStride = stride
	s.pendingDepth = depth
	// Preserve occupied cells even if opposing vectors average to zero. Geometry
	// is independent of vector magnitude; it is never inferred from a zero vector.
	if engine.GeometryIsFull() {
		s.pendingOccupancy = nil
		return
	}
	geom, recycle := engine.GeometrySlice()
	if recycle {
		defer cuda.Recycle(geom)
	}
	size := cpu.Size()
	s.occupancyBuffers(size)
	layer := -1
	if !s.AllLayers {
		layer = s.Layer
	}
	cuda.ResizePreview(s.occupancyGPU, geom, layer, stride)
	s.occupancyHost.Copy(s.occupancyGPU)
	s.pendingOccupancy = s.occupancyCPU
}

func finiteVector(x, y, z float32) bool {
	return !math.IsNaN(float64(x)) && !math.IsNaN(float64(y)) && !math.IsNaN(float64(z)) && !math.IsInf(float64(x), 0) && !math.IsInf(float64(y), 0) && !math.IsInf(float64(z), 0)
}

func (s *PreviewState) processVectorSnapshot() {
	cpu := s.pendingCPU
	host := cpu.Host()
	size := cpu.Size()
	limit := cpu.Len()
	var occupancy []float32
	if s.pendingOccupancy != nil {
		occupancy = s.pendingOccupancy.Host()[0]
	}
	values := s.VectorFieldValues[:0]
	if cap(values) < limit {
		values = make([]Vector3f, 0, limit)
	}
	// Keep the previous list intact until topology comparison; double-buffer it.
	positions := s.sparePositions[:0]
	if cap(positions) < limit {
		positions = make([]Vector3i, 0, limit)
	}
	var occupied []byte
	if occupancy != nil {
		occupied = make([]byte, 0, limit)
	}
	s.InvalidCount = 0
	maxNorm := 0.0
	for i := 0; i < limit; i++ {
		x, y, z := host[0][i], float32(0), float32(0)
		if len(host) == 3 {
			y, z = host[1][i], host[2][i]
		}
		if !finiteVector(x, y, z) {
			s.InvalidCount++
			continue
		}
		zero := x == 0 && y == 0 && z == 0
		if zero && occupancy != nil && occupancy[i] <= 0 {
			continue
		}
		norm := float64(x)*float64(x) + float64(y)*float64(y) + float64(z)*float64(z)
		maxNorm = math.Max(maxNorm, norm)
		sourceZ := 0
		if s.AllLayers {
			sourceZ = (i/(size[0]*size[1]))*s.pendingStride + s.pendingStride/2
		}
		positions = append(positions, Vector3i{X: i % size[0], Y: (i / size[0]) % size[1], Z: sourceZ})
		values = append(values, Vector3f{x, y, z})
		if occupancy != nil {
			flag := byte(0)
			if occupancy[i] > 0 {
				flag = 1
			}
			occupied = append(occupied, flag)
		}
	}
	s.NormScale = math.Sqrt(maxNorm)
	if s.FixedScale > 0 {
		s.NormScale = s.FixedScale
	}
	if s.NormScale > 0 {
		n := 0
		for i, value := range values {
			normalized := Vector3f{float32(float64(value.X) / s.NormScale), float32(float64(value.Y) / s.NormScale), float32(float64(value.Z) / s.NormScale)}
			if !finiteVector(normalized.X, normalized.Y, normalized.Z) {
				s.InvalidCount++
				continue
			}
			values[n] = normalized
			positions[n] = positions[i]
			if occupied != nil {
				occupied[n] = occupied[i]
			}
			n++
		}
		values = values[:n]
		positions = positions[:n]
		if occupied != nil {
			occupied = occupied[:n]
		}
	}
	s.setVectorPayload(values, positions)
	s.VectorOccupancy = occupied
}

func (s *PreviewState) UpdateVectorField(vectorField [3][][][]float32) {
	yLen := len(vectorField[0][0])
	xLen := len(vectorField[0][0][0])
	maxCount := xLen * yLen

	valArray := make([]Vector3f, 0, maxCount)
	posArray := make([]Vector3i, 0, maxCount)
	for posx := 0; posx < xLen; posx++ {
		for posy := 0; posy < yLen; posy++ {
			valx := vectorField[0][0][posy][posx]
			valy := vectorField[1][0][posy][posx]
			valz := vectorField[2][0][posy][posx]
			if (valx == 0 && valy == 0 && valz == 0) || !finiteVector(valx, valy, valz) {
				continue
			}
			posArray = append(posArray, Vector3i{X: posx, Y: posy, Z: 0})
			valArray = append(valArray, Vector3f{X: valx, Y: valy, Z: valz})
		}
	}
	s.setVectorPayload(valArray, posArray)
}

func (s *PreviewState) UpdateScalarField(scalarField [][][]float32) {
	xLen := len(scalarField[0][0])
	yLen := len(scalarField[0])
	min, max := float32(0), float32(0)
	hasValue := false

	valArray := s.ScalarField[:0]
	if cap(valArray) < xLen*yLen {
		valArray = make([][3]float32, 0, xLen*yLen)
	}
	s.InvalidCount = 0
	for posx := 0; posx < xLen; posx++ {
		for posy := 0; posy < yLen; posy++ {
			if !contains(s.globalQuantities, s.Quantity) && s.layerMask != nil && s.layerMask[posy][posx] == 0 {
				continue
			}
			val := scalarField[0][posy][posx]
			if math.IsNaN(float64(val)) || math.IsInf(float64(val), 0) {
				s.InvalidCount++
				continue
			}
			if !hasValue {
				min, max = val, val
				hasValue = true
			} else {
				if val < min {
					min = val
				}
				if val > max {
					max = val
				}
			}
			valArray = append(valArray, [3]float32{float32(posx), float32(posy), val})
		}
	}
	if len(valArray) == 0 {
		log.Log.Warn("No data in scalar field")
		s.Min = 0
		s.Max = 0
		s.ScalarField = nil
		s.clearVectorPayload()
		s.DataPointsCount = 0
		return
	}

	s.Min = min
	s.Max = max
	s.ScalarField = valArray
	s.clearVectorPayload()
	s.DataPointsCount = len(valArray)
}

// Capture current geometry for every frame, including moving-window changes.
// Full geometry needs no mask transfer. Other shapes reuse pinned staging and
// sample the exact same plane and reduction as the displayed scalar field.
func (s *PreviewState) ensureMask(xSize, ySize int) {
	if engine.GeometryIsFull() {
		s.layerMask = nil
		return
	}
	geom, recycle := engine.GeometrySlice()
	if recycle {
		defer cuda.Recycle(geom)
	}
	s.occupancyBuffers([3]int{xSize, ySize, 1})
	u, v, _ := previewPlaneAxes(s.Plane)
	layer := s.SliceIndex
	if s.AllLayers {
		layer = -1
	}
	cuda.ResizePreviewPlane(s.occupancyGPU, geom, u, v, layer)
	s.occupancyHost.Copy(s.occupancyGPU)
	s.layerMask = s.occupancyCPU.Scalars()[0]
}

func (s *PreviewState) occupancyBuffers(size [3]int) {
	if s.occupancyCPU == nil || s.occupancyCPU.Size() != size {
		if s.occupancyGPU != nil {
			s.occupancyGPU.Free()
		}
		if s.occupancyHost != nil {
			s.occupancyHost.Free()
		}
		s.occupancyHost = cuda.NewPreviewHostBuffer(1, size)
		s.occupancyCPU = s.occupancyHost.Slice
		s.occupancyGPU = cuda.NewSlice(1, size)
	}
}

func contains(arr []string, val string) bool {
	for _, item := range arr {
		if item == val {
			return true
		}
	}
	return false
}

func closestInArray(arr []int, target int) int {
	closest := arr[0]
	minDiff := math.Abs(float64(target - closest))

	for _, num := range arr {
		diff := math.Abs(float64(target - num))
		if diff < minDiff {
			minDiff = diff
			closest = num
		}
	}

	return closest
}

func compStringToIndex(comp string) int {
	switch comp {
	case "x":
		return 0
	case "y":
		return 1
	case "z":
		return 2
	case "3D":
		return -1
	case "None":
		return 0
	}
	log.Log.ErrAndExit("Invalid component string")
	return -2
}

// A valid destination size is a positive integer less than or equal to srcsize that evenly divides srcsize.
func possibleDownscaleSizes(srcSize int) []int {
	if srcSize <= 0 {
		return nil
	}

	sizes := make([]int, 0)
	for dstsize := 1; dstsize <= srcSize; dstsize++ {
		if srcSize%dstsize == 0 {
			sizes = append(sizes, dstsize)
		}
	}
	return sizes
}

// addPossibleDownscaleSizes refreshes the preview sizes after the script has
// initialized the mesh. It must not wait here: Start() is called before the
// script is evaluated in interactive mode.
func (s *PreviewState) addPossibleDownscaleSizes() bool {
	meshSize, _, _, _ := engine.MeshSnapshot()
	if meshSize[0] <= 0 || meshSize[1] <= 0 {
		return false
	}
	if s.previewMeshSize == meshSize && len(s.XPossibleSizes) > 0 && len(s.YPossibleSizes) > 0 {
		return true
	}

	xPossibleSizes := possiblePreviewXYSizes(meshSize[0])
	yPossibleSizes := possiblePreviewXYSizes(meshSize[1])
	if len(xPossibleSizes) == 0 || len(yPossibleSizes) == 0 {
		log.Log.Err("No possible sizes found for mesh %v", meshSize)
		return false
	}

	s.XPossibleSizes = xPossibleSizes
	s.YPossibleSizes = yPossibleSizes
	s.ZPossibleSizes = possibleDownscaleSizes(meshSize[2])
	if s.ZChosenSize <= 0 {
		s.ZChosenSize = meshSize[2]
	} else {
		s.ZChosenSize = closestInArray(s.ZPossibleSizes, s.ZChosenSize)
	}
	if engine.PreviewXDataPoints != 0 {
		s.XChosenSize = closestInArray(s.XPossibleSizes, engine.PreviewXDataPoints)
	} else {
		s.XChosenSize = closestInArray(s.XPossibleSizes, 100)
	}
	if engine.PreviewYDataPoints != 0 {
		s.YChosenSize = closestInArray(s.YPossibleSizes, engine.PreviewYDataPoints)
	} else {
		s.YChosenSize = closestInArray(s.YPossibleSizes, 100)
	}
	for axis, size := range meshSize {
		if s.planeChosenSize[axis] <= 0 {
			s.planeChosenSize[axis] = min(size, 100)
		}
		s.planeChosenSize[axis] = closestInArray(possiblePreviewXYSizes(size), s.planeChosenSize[axis])
		s.sliceIndices[axis] = min(max(s.sliceIndices[axis], 0), size-1)
	}
	s.previewMeshSize = meshSize
	s.refreshPlaneMetadata()
	return true
}

func (s *PreviewState) updatePreviewType() {
	var fieldType string
	isVectorField := (s.NComp == 3 || s.NComp == 1) && s.Component == "3D"
	if isVectorField {
		fieldType = "3D"
	} else {
		fieldType = "2D"
	}
	if fieldType != s.Type {
		s.Type = fieldType
		s.Refresh = true
	}
}

func (s *PreviewState) validateComponent() {
	s.NComp = s.getQuantity().NComp()
	switch s.NComp {
	case 1:
		if s.Component != "3D" {
			s.Component = "None"
		}
	case 3:
		if s.Component == "None" {
			s.Component = "3D"
		}
	default:
		log.Log.Err("Invalid number of components")
		// reset to default
		s.Quantity = "m"
		s.Component = "3D"
	}
}

func (s *PreviewState) postPreviewComponent(c echo.Context) error {
	s.ws.stateMu.Lock()
	defer s.ws.stateMu.Unlock()
	type Request struct {
		Component string `msgpack:"component"`
	}
	req := new(Request)
	if err := c.Bind(req); err != nil {
		log.Log.Err("%v", err)
		return c.JSON(http.StatusBadRequest, echo.Map{"error": "Invalid request payload"})
	}
	if req.Component != "x" && req.Component != "y" && req.Component != "z" && req.Component != "3D" && req.Component != "None" {
		return c.JSON(http.StatusBadRequest, echo.Map{"error": "Invalid component"})
	}
	s.Component = req.Component
	s.validateComponent()
	s.updatePreviewType()
	s.Refresh = true
	s.ws.broadcastPreviewStateLocked()
	return c.JSON(http.StatusOK, nil)
}

func (s *PreviewState) postPreviewQuantity(c echo.Context) error {
	s.ws.stateMu.Lock()
	defer s.ws.stateMu.Unlock()
	type Request struct {
		Quantity string `msgpack:"quantity"`
	}
	req := new(Request)
	if err := c.Bind(req); err != nil {
		log.Log.Err("%v", err)
		return c.JSON(http.StatusBadRequest, echo.Map{"error": "Invalid request payload"})
	}
	_, exists := engine.AvailableQuantities()[req.Quantity]
	if !exists {
		return c.JSON(http.StatusBadRequest, echo.Map{"error": "Quantity not found"})
	}
	s.Quantity = req.Quantity
	s.validateComponent()
	s.Refresh = true
	s.updatePreviewType()
	s.ws.broadcastPreviewStateLocked()
	return c.JSON(http.StatusOK, nil)
}

func (s *PreviewState) postPreviewLayer(c echo.Context) error {
	s.ws.stateMu.Lock()
	defer s.ws.stateMu.Unlock()
	type Request struct {
		Layer int `msgpack:"layer"`
	}
	req := new(Request)
	if err := c.Bind(req); err != nil {
		log.Log.Err("%v", err)
		return c.JSON(http.StatusBadRequest, echo.Map{"error": "Invalid request payload"})
	}

	axis := 2
	if s.Type == "2D" {
		_, _, axis = previewPlaneAxes(s.Plane)
	}
	if req.Layer < 0 || req.Layer >= engine.MeshSnapshotSize()[axis] {
		return c.JSON(http.StatusBadRequest, echo.Map{"error": "Layer outside mesh"})
	}
	if s.Type == "2D" {
		s.SliceIndex = req.Layer
		s.sliceIndices[axis] = req.Layer
	} else {
		s.Layer = req.Layer
	}
	s.Refresh = true
	s.ws.broadcastPreviewStateLocked()
	return c.JSON(http.StatusOK, nil)
}

func (s *PreviewState) postPreviewMaxPoints(c echo.Context) error {
	s.ws.stateMu.Lock()
	defer s.ws.stateMu.Unlock()
	type Request struct {
		MaxPoints int `msgpack:"maxPoints"`
	}
	req := new(Request)
	if err := c.Bind(req); err != nil {
		log.Log.Err("%v", err)
		return c.JSON(http.StatusBadRequest, echo.Map{"error": "Invalid request payload"})
	}
	if req.MaxPoints < 8 || req.MaxPoints > previewHardLimit {
		return c.JSON(http.StatusBadRequest, echo.Map{"error": "MaxPoints must be between 8 and 1000000"})
	}
	s.MaxPoints = req.MaxPoints
	s.Refresh = true
	s.ws.broadcastPreviewStateLocked()
	return c.JSON(http.StatusOK, nil)
}

func (s *PreviewState) postPreviewRefresh(c echo.Context) error {
	s.ws.stateMu.Lock()
	defer s.ws.stateMu.Unlock()
	s.ws.broadcastPreviewStateLocked()
	return c.JSON(http.StatusOK, nil)
}

func containsInt(arr []int, target int) bool {
	for _, v := range arr {
		if v == target {
			return true
		}
	}
	return false
}

// Resolve every axis against the current server mesh in one operation. Browser
// metadata can lag after a Console mesh change or an All layers toggle.
func (s *PreviewState) postFullResolution(c echo.Context) error {
	s.ws.stateMu.Lock()
	defer s.ws.stateMu.Unlock()
	if !s.addPossibleDownscaleSizes() {
		return c.JSON(http.StatusBadRequest, echo.Map{"error": "Mesh is not initialized"})
	}
	s.XChosenSize = s.XPossibleSizes[len(s.XPossibleSizes)-1]
	s.YChosenSize = s.YPossibleSizes[len(s.YPossibleSizes)-1]
	s.ZChosenSize = max(s.previewMeshSize[2], 1)
	s.planeChosenSize = s.previewMeshSize
	s.refreshPlaneMetadata()
	s.Refresh = true
	s.ws.broadcastPreviewStateLocked()
	return c.JSON(http.StatusOK, nil)
}

func (s *PreviewState) postXChosenSize(c echo.Context) error {
	s.ws.stateMu.Lock()
	defer s.ws.stateMu.Unlock()
	type Request struct {
		XChosenSize int `msgpack:"xChosenSize"`
	}
	req := new(Request)
	if err := c.Bind(req); err != nil {
		log.Log.Err("%v", err)
		return c.JSON(http.StatusBadRequest, echo.Map{"error": "Invalid request payload"})
	}
	if !containsInt(s.XPossibleSizes, req.XChosenSize) {
		return c.JSON(http.StatusBadRequest, echo.Map{"error": "Invalid xChosenSize"})
	}
	s.XChosenSize = req.XChosenSize
	s.Refresh = true
	s.ws.broadcastPreviewStateLocked()
	return c.JSON(http.StatusOK, nil)
}

func (s *PreviewState) postYChosenSize(c echo.Context) error {
	s.ws.stateMu.Lock()
	defer s.ws.stateMu.Unlock()
	type Request struct {
		YChosenSize int `msgpack:"yChosenSize"`
	}
	req := new(Request)
	if err := c.Bind(req); err != nil {
		log.Log.Err("%v", err)
		return c.JSON(http.StatusBadRequest, echo.Map{"error": "Invalid request payload"})
	}
	if !containsInt(s.YPossibleSizes, req.YChosenSize) {
		return c.JSON(http.StatusBadRequest, echo.Map{"error": "Invalid yChosenSize"})
	}
	s.YChosenSize = req.YChosenSize
	s.Refresh = true
	s.ws.broadcastPreviewStateLocked()
	return c.JSON(http.StatusOK, nil)
}

func (s *PreviewState) postZChosenSize(c echo.Context) error {
	s.ws.stateMu.Lock()
	defer s.ws.stateMu.Unlock()
	var req struct {
		ZChosenSize int `msgpack:"zChosenSize"`
	}
	if err := c.Bind(&req); err != nil || !containsInt(s.ZPossibleSizes, req.ZChosenSize) {
		return c.JSON(http.StatusBadRequest, echo.Map{"error": "Invalid zChosenSize"})
	}
	s.ZChosenSize = req.ZChosenSize
	s.Refresh = true
	s.ws.broadcastPreviewStateLocked()
	return c.JSON(http.StatusOK, nil)
}

func (s *PreviewState) postAllLayers(c echo.Context) error {
	s.ws.stateMu.Lock()
	defer s.ws.stateMu.Unlock()
	type Request struct {
		AllLayers bool `msgpack:"allLayers"`
	}
	req := new(Request)
	if err := c.Bind(req); err != nil {
		log.Log.Err("%v", err)
		return c.JSON(http.StatusBadRequest, echo.Map{"error": "Invalid request payload"})
	}
	s.AllLayers = req.AllLayers
	s.Refresh = true
	s.ws.broadcastPreviewStateLocked()
	return c.JSON(http.StatusOK, nil)
}

func (s *PreviewState) postAutoScaleEnabled(c echo.Context) error {
	s.ws.stateMu.Lock()
	defer s.ws.stateMu.Unlock()
	type Request struct {
		AutoScaleEnabled bool `msgpack:"autoScaleEnabled"`
	}
	req := new(Request)
	if err := c.Bind(req); err != nil {
		log.Log.Err("%v", err)
		return c.JSON(http.StatusBadRequest, echo.Map{"error": "Invalid request payload"})
	}
	s.AutoScaleEnabled = req.AutoScaleEnabled
	s.Refresh = true
	s.ws.broadcastPreviewStateLocked()
	return c.JSON(http.StatusOK, nil)
}

func (s *PreviewState) postPreviewScale(c echo.Context) error {
	var req struct {
		Scale float64 `json:"scale"`
	}
	if err := c.Bind(&req); err != nil || req.Scale < 0 || math.IsNaN(req.Scale) || math.IsInf(req.Scale, 0) {
		return c.JSON(http.StatusBadRequest, echo.Map{"error": "Scale must be finite and nonnegative (0 = adaptive)"})
	}
	s.ws.stateMu.Lock()
	defer s.ws.stateMu.Unlock()
	s.FixedScale = req.Scale
	s.Refresh = true
	s.ws.broadcastPreviewStateLocked()
	return c.JSON(http.StatusOK, nil)
}

// Include practical sizes even for prime dimensions; exact coverage is provided
// by fractional-area resize rather than silently dropping edge cells.
func possiblePreviewXYSizes(n int) []int {
	if n < 1 {
		return nil
	}
	found := map[int]bool{n: true}
	for _, v := range possibleDownscaleSizes(n) {
		found[v] = true
	}
	for v := 1; v <= min(n, 128); v++ {
		found[v] = true
	}
	for v := 128; v < n; v *= 2 {
		found[min(v, n)] = true
	}
	result := make([]int, 0, len(found))
	for v := range found {
		result = append(result, v)
	}
	sort.Ints(result)
	return result
}
