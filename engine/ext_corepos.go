package engine

import (
	"math"
	"sync"

	"github.com/mumax/3/coreposition"
)

// The third legacy component is polarization. New skyrmion coordinates use
// three spatial coordinates and publish polarization as a separate scalar.
var CorePos = NewVectorValue("ext_corepos", "m", "Vortex X,Y in metres; legacy third component is dimensionless polarization", corePos)
var CoreValid = NewScalarValue("ext_corevalid", "", "Whether the vortex position is localized", func() float64 { return validNumber(coreResult("vortex")) })
var SkyrmionPos = NewVectorValue("ext_skyrmionpos", "m", "Skyrmion position in laboratory coordinates", func() []float64 { r := coreResult("skyrmion"); return r.Position[:] })
var SkyrmionValid = NewScalarValue("ext_skyrmionvalid", "", "Whether the tracked skyrmion is localized", func() float64 { return validNumber(coreResult("skyrmion")) })
var SkyrmionPolarization = NewScalarValue("ext_skyrmionpol", "", "Magnetization polarization of the tracked core", func() float64 { return coreResult("skyrmion").Polarization })
var CoreTrackingEnabled bool
var CoreLayer = -1
var SkyrmionLayer = -1
var SkyrmionPolarity = 1
var SkyrmionBackground = -1.0
var SkyrmionThreshold = 0.1
var SkyrmionSeedX, SkyrmionSeedY float64
var SkyrmionSearchRadius, SkyrmionMaxDisplacement float64

var coreState struct {
	mu          sync.RWMutex
	mode        string
	status      string
	previous    coreposition.Result
	hasPrevious bool
}

// Only the engine owner accesses the sampling cache. Status publication uses
// coreState.mu and never initiates GPU reads from the WebUI publisher.
var coreSample struct {
	active  bool
	field   *coreposition.Field
	results map[string]coreposition.Result
}

func init() {
	DeclFunc("CoreTrack", EnableCoreTracking, "Track the vortex position and validity in the table")
	DeclFunc("SkyrmionTrack", EnableSkyrmionTracking, "Track a skyrmion with core polarity +1 or -1; enable TableAutoSave separately")
	DeclFunc("SkyrmionReset", ResetSkyrmionTracking, "Reset association to SkyrmionSeedX and SkyrmionSeedY")
	DeclVar("CoreLayer", &CoreLayer, "Vortex layer index; -1 selects the strongest layer (legacy definition)")
	DeclVar("SkyrmionLayer", &SkyrmionLayer, "Skyrmion layer index; -1 aggregates connected layers")
	DeclVar("SkyrmionPolarity", &SkyrmionPolarity, "Expected core polarity, +1 or -1")
	DeclVar("SkyrmionBackground", &SkyrmionBackground, "Background mz opposite to the core polarity")
	DeclVar("SkyrmionThreshold", &SkyrmionThreshold, "Background contrast removed before centroid weighting, default 0.1")
	DeclVar("SkyrmionSeedX", &SkyrmionSeedX, "Initial reference X in laboratory metres")
	DeclVar("SkyrmionSeedY", &SkyrmionSeedY, "Initial reference Y in laboratory metres")
	DeclVar("SkyrmionSearchRadius", &SkyrmionSearchRadius, "Search radius around the reference in metres; zero uses the domain")
	DeclVar("SkyrmionMaxDisplacement", &SkyrmionMaxDisplacement, "Maximum displacement between samples; zero uses the previous core scale")
}

func validNumber(r coreposition.Result) float64 {
	if r.Valid {
		return 1
	}
	return 0
}
func setCoreMode(mode string) {
	coreState.mu.Lock()
	coreState.mode = mode
	CoreTrackingEnabled = true
	coreState.mu.Unlock()
	addStructuredMetadata("core_tracking_mode", mode)
}
func CoreTrackingStatus() (mode, status string) {
	coreState.mu.RLock()
	defer coreState.mu.RUnlock()
	return coreState.mode, coreState.status
}
func ResetSkyrmionTracking() {
	coreState.mu.Lock()
	defer coreState.mu.Unlock()
	coreState.hasPrevious = false
	coreState.status = "waiting for a sample"
}

func EnableCoreTracking() { TableAdd(CorePos); TableAdd(CoreValid); setCoreMode("vortex") }
func EnableSkyrmionTracking(polarity int) {
	if polarity != 1 && polarity != -1 {
		panic(UserErr("SkyrmionTrack polarity must be +1 or -1"))
	}
	SkyrmionPolarity = polarity
	SkyrmionBackground = -float64(polarity)
	ResetSkyrmionTracking()
	TableAdd(SkyrmionPos)
	TableAdd(SkyrmionValid)
	TableAdd(SkyrmionPolarization)
	setCoreMode("skyrmion")
}

func beginCoreSample() {
	coreSample.active = true
	coreSample.field = nil
	coreSample.results = nil
}
func endCoreSample() { coreSample.active = false; coreSample.field = nil; coreSample.results = nil }

func coreField() coreposition.Field {
	if coreSample.active && coreSample.field != nil {
		return *coreSample.field
	}
	mesh := Mesh()
	pbc := mesh.PBC()
	f := coreposition.Field{Mz: M.Buffer().Comp(Z).HostCopy().Scalars(), Size: mesh.Size(), Cell: mesh.CellSize(), PBC: [3]bool{pbc[0] > 0, pbc[1] > 0, pbc[2] > 0}, Shift: [2]float64{GetShiftPos(), GetShiftYPos()}}
	if g := geometry.Gpu(); !g.IsNil() {
		f.Mask = g.Comp(0).HostCopy().Scalars()
	}
	if coreSample.active {
		coreSample.field = &f
	}
	return f
}

func coreResult(mode string) coreposition.Result {
	if coreSample.active {
		if r, ok := coreSample.results[mode]; ok {
			return r
		}
	}
	f := coreField()
	var result coreposition.Result
	if mode == "vortex" {
		result = coreposition.Vortex(f, CoreLayer)
	} else {
		reference := [2]float64{SkyrmionSeedX, SkyrmionSeedY}
		maxDistance := SkyrmionMaxDisplacement
		coreState.mu.RLock()
		previous, hasPrevious := coreState.previous, coreState.hasPrevious
		coreState.mu.RUnlock()
		if hasPrevious {
			reference = [2]float64{previous.Position[0], previous.Position[1]}
			if maxDistance == 0 {
				maxDistance = math.Max(3*math.Max(f.Cell[0], f.Cell[1]), 2*previous.Radius)
			}
		}
		result = coreposition.Skyrmion(f, coreposition.SkyrmionOptions{Polarity: SkyrmionPolarity, Layer: SkyrmionLayer, Background: SkyrmionBackground, Threshold: SkyrmionThreshold, Reference: reference, SearchRadius: SkyrmionSearchRadius, MaxDisplacement: maxDistance})
	}
	coreState.mu.Lock()
	if mode == "skyrmion" && result.Valid {
		coreState.previous = result
		coreState.hasPrevious = true
	}
	if coreState.mode == mode {
		coreState.status = result.Reason
		if result.Valid {
			coreState.status = "tracking"
		}
	}
	coreState.mu.Unlock()
	if coreSample.active {
		if coreSample.results == nil {
			coreSample.results = make(map[string]coreposition.Result)
		}
		coreSample.results[mode] = result
	}
	return result
}

func corePos() []float64 {
	r := coreResult("vortex")
	return []float64{r.Position[0], r.Position[1], r.Polarization}
}

func tableComponentUnit(q Quantity, c int) string {
	if named, ok := q.(*namedTableQuantity); ok {
		return tableComponentUnit(named.Quantity, c)
	}
	if NameOf(q) == "ext_corepos" && c == 2 {
		return ""
	}
	return UnitOf(q)
}
