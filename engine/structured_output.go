package engine

import (
	"fmt"
	"math"
	"path"
	"reflect"
	"strings"
	"sync"
	"time"

	"github.com/mumax/3/cuda"
	"github.com/mumax/3/hdf5"
	"github.com/mumax/3/httpfs"
	"github.com/mumax/3/script"
	"github.com/mumax/3/zarr"
)

type StorageFormatType int

const (
	StorageFormatOVF StorageFormatType = iota
	StorageFormatZarr
	StorageFormatHDF5
)

// StorageFormat defaults to Zarr, matching Amumax. OVF remains available with
// -storage-format=ovf or StorageFormat = OVF.
var StorageFormat = StorageFormatZarr

type storageFormatValue struct{}

func (*storageFormatValue) Eval() interface{}  { return StorageFormat }
func (*storageFormatValue) Type() reflect.Type { return reflect.TypeOf(StorageFormatType(0)) }
func (*storageFormatValue) SetValue(value interface{}) {
	drainOutput()
	format := value.(StorageFormatType)
	if format < StorageFormatOVF || format > StorageFormatHDF5 {
		panic(UserErr(fmt.Sprintf("invalid StorageFormat %d", format)))
	}
	StorageFormat = format
	if outputdir != "" {
		CheckRecoverable(ensureStructuredBackend(format))
	}
}

// Chunking uses Amumax's API convention: values are requested numbers of
// chunks along x, y, z and component axes, not chunk lengths.
type Chunking struct {
	X, Y, Z, C int
}

func Chunk(x, y, z, components int) Chunking {
	return Chunking{X: x, Y: y, Z: z, C: components}
}

func init() {
	script.AddMetadata = addStructuredMetadata
	DeclROnly("OVF", StorageFormatOVF, "StorageFormat = OVF keeps standard mumax3 OVF output")
	DeclROnly("ZARR", StorageFormatZarr, "StorageFormat = ZARR selects chunked Zarr v2 output")
	DeclROnly("HDF5", StorageFormatHDF5, "StorageFormat = HDF5 selects per-quantity HDF5 output")
	DeclLValue("StorageFormat", &storageFormatValue{}, "Storage backend used by Save and AutoSave")
	DeclFunc("Chunk", Chunk, "Chunk(nx, ny, nz, ncomp) requests a number of chunks along each axis")
	DeclFunc("SaveAsChunk", SaveAsChunk, "Save a quantity as a structured dataset using explicit chunk counts")
	DeclFunc("AutoSaveAs", AutoSaveAs, "Auto-save a quantity as a named structured dataset")
	DeclFunc("AutoSaveAsChunk", AutoSaveAsChunk, "Auto-save a named structured dataset using explicit chunk counts")
}

func addStructuredMetadata(key string, value interface{}) {
	if key == "" || value == nil {
		return
	}
	structuredOutput.mu.Lock()
	defer structuredOutput.mu.Unlock()
	switch reflected := reflect.ValueOf(value); reflected.Kind() {
	case reflect.Bool, reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64,
		reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64,
		reflect.Float32, reflect.Float64, reflect.String:
		structuredOutput.metadata[key] = value
	case reflect.Array, reflect.Slice:
		structuredOutput.metadata[key] = fmt.Sprint(value)
	case reflect.Ptr:
		if !reflected.IsNil() {
			structuredOutput.metadata[key] = fmt.Sprint(reflected.Elem().Interface())
		}
	}
}

type structuredDataset struct {
	name       string
	quantity   Quantity
	period     float64
	nextTime   float64
	times      []float64
	requested  Chunking
	chunkSizes [4]int
	format     StorageFormatType
}

type structuredOutputState struct {
	mu               sync.Mutex
	datasets         map[string]*structuredDataset
	hdf5             *hdf5.MultiWriter
	metadata         map[string]any
	started          time.Time
	lastMetadataSave time.Time
}

var structuredOutput = structuredOutputState{
	datasets: make(map[string]*structuredDataset),
	metadata: make(map[string]any),
}

func ensureStructuredBackend(format StorageFormatType) error {
	switch format {
	case StorageFormatOVF:
		return nil
	case StorageFormatZarr:
		return zarr.WriteGroup(OD())
	case StorageFormatHDF5:
		if structuredOutput.hdf5 == nil {
			writer, err := hdf5.NewMultiWriter(OD())
			if err != nil {
				return err
			}
			structuredOutput.hdf5 = writer
		}
		return nil
	default:
		return fmt.Errorf("unsupported storage format %d", format)
	}
}

func initStructuredOutput() {
	CheckRecoverable(ensureStructuredBackend(StorageFormat))
	structuredOutput.started = StartTime
	structuredOutput.lastMetadataSave = time.Now()
	structuredOutput.metadata["start_time"] = StartTime.Format(time.UnixDate)
	if StorageFormat == StorageFormatZarr {
		CheckRecoverable(zarr.WriteObjectAttributes(OD(), structuredOutput.metadata))
	}
}

func SetStructuredGPUInfo(info string) {
	structuredOutput.mu.Lock()
	defer structuredOutput.mu.Unlock()
	structuredOutput.metadata["gpu"] = info
}

func flushStructuredMetadata(force bool) {
	if StorageFormat != StorageFormatZarr || outputdir == "" {
		return
	}
	structuredOutput.mu.Lock()
	defer structuredOutput.mu.Unlock()
	if !force && time.Since(structuredOutput.lastMetadataSave) < 5*time.Second {
		return
	}
	CheckRecoverable(zarr.WriteObjectAttributes(OD(), structuredOutput.metadata))
	structuredOutput.lastMetadataSave = time.Now()
}

func updateStructuredMeshMetadata() {
	if StorageFormat != StorageFormatZarr || outputdir == "" {
		return
	}
	structuredOutput.mu.Lock()
	defer structuredOutput.mu.Unlock()
	mesh := Mesh()
	size, cell, pbc := mesh.Size(), mesh.CellSize(), mesh.PBC()
	for index, name := range []string{"x", "y", "z"} {
		structuredOutput.metadata["N"+name] = size[index]
		structuredOutput.metadata["d"+name] = cell[index]
		structuredOutput.metadata["T"+name] = float64(size[index]) * cell[index]
		structuredOutput.metadata["PBC"+name] = pbc[index]
	}
	CheckRecoverable(zarr.WriteObjectAttributes(OD(), structuredOutput.metadata))
	structuredOutput.lastMetadataSave = time.Now()
}

func activeStructuredFormat() StorageFormatType {
	if StorageFormat == StorageFormatOVF {
		// Amumax-only APIs such as SaveAsChunk have no OVF interpretation.
		return StorageFormatZarr
	}
	return StorageFormat
}

func SaveAsChunk(quantity Quantity, name string, requested Chunking) {
	saveStructured(quantity, name, requested, activeStructuredFormat())
}

func AutoSaveAs(quantity Quantity, name string, period float64) {
	AutoSaveAsChunk(quantity, name, period, Chunk(1, 1, 1, 1))
}

func AutoSaveAsChunk(quantity Quantity, name string, period float64, requested Chunking) {
	name = validateDatasetName(name)
	if period < 0 {
		panic(UserErr("AutoSaveAsChunk: period must be non-negative"))
	}
	if period == 0 {
		dataset := structuredOutput.datasets[name]
		if dataset == nil {
			panic(UserErr("AutoSaveAsChunk: dataset has not been initialized: " + name))
		}
		dataset.period = 0
		return
	}
	dataset := getOrCreateStructuredDataset(quantity, name, requested, activeStructuredFormat())
	dataset.period = period
	dataset.nextTime = Time
}

func autoSaveStructured(quantity Quantity, period float64) {
	AutoSaveAs(quantity, NameOf(quantity), period)
}

func saveStructured(quantity Quantity, name string, requested Chunking, format StorageFormatType) {
	name = validateDatasetName(name)
	dataset := getOrCreateStructuredDataset(quantity, name, requested, format)
	dataset.save()
}

func getOrCreateStructuredDataset(quantity Quantity, name string, requested Chunking, format StorageFormatType) *structuredDataset {
	if existing := structuredOutput.datasets[name]; existing != nil {
		if existing.quantity != quantity {
			panic(UserErr("structured dataset already uses a different quantity: " + name))
		}
		if existing.requested != requested || existing.format != format {
			panic(UserErr("structured dataset already uses different chunks or format: " + name))
		}
		return existing
	}
	CheckRecoverable(ensureStructuredBackend(format))
	dataset := &structuredDataset{
		name:       name,
		quantity:   quantity,
		requested:  requested,
		chunkSizes: resolveChunkSizes(quantity, requested),
		format:     format,
	}
	structuredOutput.datasets[name] = dataset
	if format == StorageFormatZarr {
		dir := OD() + name
		CheckRecoverable(httpfs.Remove(dir))
		CheckRecoverable(httpfs.Mkdir(dir))
	}
	return dataset
}

func (dataset *structuredDataset) save() {
	dataset.times = append(dataset.times, Time)
	step := len(dataset.times) - 1
	times := append([]float64(nil), dataset.times...)
	buffer := ValueOf(dataset.quantity)
	defer cuda.Recycle(buffer)
	host := buffer.HostCopy()
	queOutput(func() {
		defer host.Free()
		var err error
		switch dataset.format {
		case StorageFormatZarr:
			dir := OD() + dataset.name
			err = zarr.WriteStep(dir, step, host, dataset.chunkSizes)
			if err == nil {
				err = zarr.WriteAttributes(dir, zarr.Attributes{Times: times, Name: dataset.name, Unit: UnitOf(dataset.quantity)})
			}
		case StorageFormatHDF5:
			err = structuredOutput.hdf5.SaveArray(dataset.name, step, host.Tensors(), host.Size(), host.NComp())
		default:
			err = fmt.Errorf("structured save requested with format %d", dataset.format)
		}
		if err != nil {
			panic(err)
		}
	})
}

func (state *structuredOutputState) saveIfNeeded() {
	for _, dataset := range state.datasets {
		if dataset.period > 0 && Time >= dataset.nextTime {
			dataset.save()
			dataset.nextTime += dataset.period
		}
	}
}

func closeStructuredOutput() {
	drainOutput()
	if StorageFormat == StorageFormatZarr && outputdir != "" {
		structuredOutput.mu.Lock()
		structuredOutput.metadata["steps"] = NSteps
		structuredOutput.metadata["end_time"] = time.Now().Format(time.UnixDate)
		structuredOutput.metadata["total_time"] = time.Since(structuredOutput.started).String()
		CheckRecoverable(zarr.WriteObjectAttributes(OD(), structuredOutput.metadata))
		structuredOutput.mu.Unlock()
	}
	if structuredOutput.hdf5 != nil {
		for _, dataset := range structuredOutput.datasets {
			if dataset.format == StorageFormatHDF5 && len(dataset.times) > 0 {
				CheckRecoverable(structuredOutput.hdf5.SaveTimestamps(dataset.name, dataset.times))
			}
		}
	}
	CheckRecoverable(tableHistory.flushStructured())
	if structuredOutput.hdf5 != nil {
		CheckRecoverable(structuredOutput.hdf5.Close())
		structuredOutput.hdf5 = nil
	}
}

func resolveChunkSizes(quantity Quantity, requested Chunking) [4]int {
	dimensions := [4]int{SizeOf(quantity)[X], SizeOf(quantity)[Y], SizeOf(quantity)[Z], quantity.NComp()}
	counts := [4]int{requested.X, requested.Y, requested.Z, requested.C}
	var sizes [4]int
	for axis, count := range counts {
		if count < 1 || count > dimensions[axis] {
			panic(UserErr(fmt.Sprintf("Chunk: requested %d chunks for dimension of length %d", count, dimensions[axis])))
		}
		resolved := closestDivisor(dimensions[axis], count)
		if resolved != count {
			LogOut(fmt.Sprintf("Chunk: adjusted axis %d chunk count from %d to %d", axis, count, resolved))
		}
		sizes[axis] = dimensions[axis] / resolved
	}
	return sizes
}

func closestDivisor(value, requested int) int {
	best, bestDistance := 1, math.MaxInt
	for divisor := 1; divisor <= value; divisor++ {
		if value%divisor == 0 {
			distance := absInt(divisor - requested)
			if distance < bestDistance {
				best, bestDistance = divisor, distance
			}
		}
	}
	return best
}

func absInt(value int) int {
	if value < 0 {
		return -value
	}
	return value
}

func validateDatasetName(name string) string {
	name = path.Clean(strings.TrimSpace(name))
	if name == "." || name == "" || path.IsAbs(name) || name == ".." || strings.HasPrefix(name, "../") {
		panic(UserErr("invalid structured dataset name"))
	}
	return name
}

type tableHistoryState struct {
	mu      sync.RWMutex
	columns []string
	units   []string
	data    map[string][]float64
}

var tableHistory = tableHistoryState{data: make(map[string][]float64)}

func recordTableHistory(columns, units []string, values []float64) {
	tableHistory.mu.Lock()
	defer tableHistory.mu.Unlock()
	if len(tableHistory.columns) == 0 {
		tableHistory.columns = append([]string(nil), columns...)
		tableHistory.units = append([]string(nil), units...)
	}
	if len(columns) != len(tableHistory.columns) || len(values) != len(columns) {
		panic("table columns changed after output started")
	}
	for index, column := range columns {
		tableHistory.data[column] = append(tableHistory.data[column], values[index])
	}
}

func TableHistorySnapshot() (columns, units []string, values map[string][]float64) {
	tableHistory.mu.RLock()
	defer tableHistory.mu.RUnlock()
	columns = append([]string(nil), tableHistory.columns...)
	units = append([]string(nil), tableHistory.units...)
	values = make(map[string][]float64, len(tableHistory.data))
	for name, data := range tableHistory.data {
		values[name] = append([]float64(nil), data...)
	}
	return
}

func (history *tableHistoryState) flushStructured() error {
	history.mu.RLock()
	defer history.mu.RUnlock()
	if len(history.columns) == 0 || StorageFormat == StorageFormatOVF {
		return nil
	}
	if StorageFormat == StorageFormatZarr {
		tableDir := OD() + "table"
		if err := zarr.WriteGroup(tableDir); err != nil {
			return err
		}
		for _, column := range history.columns {
			if err := zarr.WriteSeries(tableDir+"/"+column, history.data[column]); err != nil {
				return err
			}
		}
		return nil
	}
	if StorageFormat == StorageFormatHDF5 {
		for _, column := range history.columns {
			if err := structuredOutput.hdf5.SaveTableColumn(column, history.data[column]); err != nil {
				return err
			}
		}
	}
	return nil
}
