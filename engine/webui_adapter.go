package engine

import (
	"fmt"
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

func LogHistory() string { return hist }

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

func ExistingRegionIndices() []int {
	if regions.gpuCache == nil {
		return []int{0}
	}
	found := map[byte]struct{}{0: {}}
	for _, region := range regions.HostList() {
		found[region] = struct{}{}
	}
	result := make([]int, 0, len(found))
	for region := range found {
		result = append(result, int(region))
	}
	sort.Ints(result)
	return result
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
