package engine

import (
	"fmt"
	"github.com/mumax/3/cuda"
	"sort"
	"sync"

	"github.com/mumax/3/data"
)

var (
	PreviewXDataPoints = 100
	PreviewYDataPoints = 100
)

func init() {
	DeclVar("PreviewXDataPoints", &PreviewXDataPoints, "Preferred number of x data points in the web preview")
	DeclVar("PreviewYDataPoints", &PreviewYDataPoints, "Preferred number of y data points in the web preview")
}

func EvalTryRecover(code string) {
	defer func() {
		if recovered := recover(); recovered != nil {
			LogErr(recovered)
		}
	}()
	Eval(code)
}

func LogHistory() string {
	logMu.Lock()
	defer logMu.Unlock()
	return hist
}

func IsPaused() bool { return pause }

func SolverType() int { return solvertype }

func CurrentDt() float64 { return Dt_si }

func MeshReady() bool { return globalmesh_.Size() != [3]int{} }

func MeshSnapshotSize() [3]int { return globalmesh_.Size() }

func MeshSnapshot() (size [3]int, cell [3]float64, world [3]float64, pbc [3]int) {
	size = globalmesh_.Size()
	if size == [3]int{} {
		return
	}
	cell = globalmesh_.CellSize()
	world = globalmesh_.WorldSize()
	pbc = globalmesh_.PBC()
	return
}

func GeometryQuantity() Quantity { return &geometry }

func GeometrySlice() (*data.Slice, bool) { return geometry.Slice() }

// Called on the engine thread. Nil geometry storage denotes a full universe.
func GeometryIsFull() bool { return geometry.Gpu().IsNil() }

func AvailableQuantities() map[string]Quantity {
	result := make(map[string]Quantity, len(gui_.Quants))
	for name, quantity := range gui_.Quants {
		result[name] = quantity
	}
	return result
}

type WebParameter struct {
	Name        string
	Value       string
	Description string
	Changed     bool
}

func WebParameters(region int) []WebParameter {
	result := make([]WebParameter, 0, len(gui_.Params))
	for _, parameter := range gui_.Params {
		values := parameter.getRegion(region)
		value := fmt.Sprint(values)
		if len(value) >= 2 {
			value = value[1 : len(value)-1]
		}
		result = append(result, WebParameter{
			Name:        parameter.Name(),
			Value:       value,
			Description: World.Doc[parameter.Name()],
		})
	}
	sort.Slice(result, func(i, j int) bool { return result[i].Name < result[j].Name })
	return result
}

var cachedRegionBuffer *cuda.Bytes
var cachedRegionRevision uint64
var cachedRegionIndices []int

// Called on the CUDA-owning engine thread. Buffer identity handles mesh resize.
func ExistingRegionIndices() []int {
	if regions.gpuCache == nil {
		return []int{0}
	}
	if cachedRegionBuffer == regions.gpuCache && cachedRegionRevision == regions.gpuCache.Revision && cachedRegionIndices != nil {
		return append([]int(nil), cachedRegionIndices...)
	}
	found := [NREGION]bool{}
	found[0] = true
	for _, region := range regions.HostList() {
		found[region] = true
	}
	result := make([]int, 0, NREGION)
	for region, exists := range found {
		if exists {
			result = append(result, region)
		}
	}
	cachedRegionBuffer = regions.gpuCache
	cachedRegionRevision = regions.gpuCache.Revision
	cachedRegionIndices = result
	return append([]int(nil), result...)
}

func TableAutoSavePeriod() float64 { return Table.autosave.period }

type webMetadataState struct {
	mu     sync.RWMutex
	fields map[string]interface{}
}

var WebMetadata = webMetadataState{fields: make(map[string]interface{})}

func (metadata *webMetadataState) Add(key string, value interface{}) {
	metadata.mu.Lock()
	defer metadata.mu.Unlock()
	metadata.fields[key] = value
}

func (metadata *webMetadataState) Snapshot() map[string]interface{} {
	metadata.mu.RLock()
	defer metadata.mu.RUnlock()
	result := make(map[string]interface{}, len(metadata.fields))
	for key, value := range metadata.fields {
		result[key] = value
	}
	return result
}
