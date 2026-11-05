package engine

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/mumax/3/data"
	"github.com/mumax/3/script"
	"github.com/mumax/3/zarr"
)

func TestAmumaxStorageDefaults(t *testing.T) {
	if StorageFormat != StorageFormatZarr {
		t.Fatalf("StorageFormat = %v, want Zarr", StorageFormat)
	}
	if *Flag_storage != "zarr" {
		t.Fatalf("-storage-format default = %q, want zarr", *Flag_storage)
	}
	if got, want := *Flag_cachedir, filepath.Join(os.TempDir(), "amumax_kernels"); got != want {
		t.Fatalf("-cache default = %q, want %q", got, want)
	}
}

func TestAmumaxMinimizeLimitsAreRegistered(t *testing.T) {
	for _, name := range []string{"minimizemaxsteps", "minimizemaxtimeseconds"} {
		if _, ok := World.Identifiers[name]; !ok {
			t.Fatalf("MX3 identifier %q is missing", name)
		}
	}
}

func TestPeriodicStructuredMetadataIncludesGPU(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "run.zarr") + "/"
	previousOutput, previousFormat := outputdir, StorageFormat
	previousMetadata, previousLastSave := structuredOutput.metadata, structuredOutput.lastMetadataSave
	defer func() {
		outputdir, StorageFormat = previousOutput, previousFormat
		structuredOutput.metadata, structuredOutput.lastMetadataSave = previousMetadata, previousLastSave
	}()
	outputdir, StorageFormat = dir, StorageFormatZarr
	structuredOutput.metadata = map[string]any{"start_time": "now"}
	structuredOutput.lastMetadataSave = time.Now().Add(-6 * time.Second)
	if err := zarr.WriteGroup(dir); err != nil {
		t.Fatal(err)
	}
	SetStructuredGPUInfo("test GPU")
	flushStructuredMetadata(false)
	b, err := os.ReadFile(filepath.Join(dir, ".zattrs"))
	if err != nil {
		t.Fatal(err)
	}
	var attrs map[string]any
	if err := json.Unmarshal(b, &attrs); err != nil {
		t.Fatal(err)
	}
	if attrs["gpu"] != "test GPU" {
		t.Fatalf("metadata = %#v", attrs)
	}
}

type regionwiseMetadataTestState struct {
	values    [][NREGION]float32
	updates   [NREGION]func() []float64
	timestamp float64
	gpuOK     bool
}

func snapshotRegionwiseMetadataTestState(parameter *regionwise) regionwiseMetadataTestState {
	return regionwiseMetadataTestState{
		values:    append([][NREGION]float32(nil), parameter.cpu_buf...),
		updates:   parameter.upd_reg,
		timestamp: parameter.timestamp,
		gpuOK:     parameter.gpu_ok,
	}
}

func restoreRegionwiseMetadataTestState(parameter *regionwise, state regionwiseMetadataTestState) {
	copy(parameter.cpu_buf, state.values)
	parameter.upd_reg = state.updates
	parameter.timestamp = state.timestamp
	parameter.gpu_ok = state.gpuOK
}

func TestAssignedScalarMetadataSerializesPlainValueAndKeepsRegions(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "run.zarr") + "/"
	previousOutput, previousFormat := outputdir, StorageFormat
	previousMetadata := structuredOutput.metadata
	previousLastSave := structuredOutput.lastMetadataSave
	previousAex := snapshotRegionwiseMetadataTestState(&Aex.regionwise)
	previousDind := snapshotRegionwiseMetadataTestState(&Dind.regionwise)
	previousBExt := snapshotRegionwiseMetadataTestState(&B_ext.perRegion.regionwise)
	previousBExtRevision := B_ext.revision
	var scalarCalls, vectorCalls int
	defer func() {
		outputdir, StorageFormat = previousOutput, previousFormat
		structuredOutput.metadata = previousMetadata
		structuredOutput.lastMetadataSave = previousLastSave
		restoreRegionwiseMetadataTestState(&Aex.regionwise, previousAex)
		restoreRegionwiseMetadataTestState(&Dind.regionwise, previousDind)
		restoreRegionwiseMetadataTestState(&B_ext.perRegion.regionwise, previousBExt)
		B_ext.revision = previousBExtRevision
	}()

	outputdir, StorageFormat = dir, StorageFormatZarr
	structuredOutput.metadata = make(map[string]any)
	world := script.NewWorld()
	world.LValue("Dind", newLValueWrapper(Dind))
	world.LValue("Aex", newLValueWrapper(Aex))
	world.LValue("B_ext", newLValueWrapper(B_ext))
	world.Func("metadataScalar", func() float64 {
		scalarCalls++
		return 1.3e-11
	})
	world.Func("vector", func(x, y, z float64) data.Vector {
		vectorCalls++
		return data.Vector{x, y, z}
	})
	code, err := world.Compile(`Dind = 0.00149
Aex = metadataScalar()
B_ext = vector(0.1, 0.2, 0.3)
Dind.setRegion(1, 0.002)
Aex.setRegion(2, 2.6e-11)
B_ext.setRegion(1, vector(0.4, 0.5, 0.6))`)
	if err != nil {
		t.Fatal(err)
	}
	code.Eval()
	if scalarCalls != 1 {
		t.Fatalf("scalar function calls = %d, want 1", scalarCalls)
	}
	if vectorCalls != 2 {
		t.Fatalf("vector function calls = %d, want 2", vectorCalls)
	}
	dindRegions, ok := regionwiseMetadataSnapshot(&Dind.regionwise)
	if !ok {
		t.Fatal("Dind region snapshot was not available")
	}
	addStructuredMetadata("Dind_region_map", dindRegions)
	bExtRegions, ok := regionwiseMetadataSnapshot(&B_ext.perRegion.regionwise)
	if !ok {
		t.Fatal("B_ext region snapshot was not available")
	}
	addStructuredMetadata("B_ext_region_map", bExtRegions)

	structuredOutput.mu.Lock()
	err = writeRunMetadataLocked()
	structuredOutput.mu.Unlock()
	if err != nil {
		t.Fatal(err)
	}
	b, err := os.ReadFile(filepath.Join(dir, ".zattrs"))
	if err != nil {
		t.Fatal(err)
	}
	var attrs map[string]any
	if err := json.Unmarshal(b, &attrs); err != nil {
		t.Fatal(err)
	}
	for key, want := range map[string]string{"Dind": "0.00149", "Aex": "1.3e-11"} {
		if got := attrs[key]; got != want {
			t.Errorf("serialized %s = %#v, want plain numeric string %q; all attrs: %#v", key, got, want, attrs)
		}
	}
	vectorMetadata, ok := attrs["B_ext"].([]any)
	if !ok || len(vectorMetadata) != 3 {
		t.Fatalf("serialized B_ext = %#v, want a 3-component vector", attrs["B_ext"])
	}
	for component, want := range []float64{0.1, 0.2, 0.3} {
		if got, ok := vectorMetadata[component].(float64); !ok || got != want {
			t.Errorf("serialized B_ext component %d = %#v, want %g", component, vectorMetadata[component], want)
		}
	}
	dindRegionMap, ok := attrs["Dind_region_map"].(map[string]any)
	if !ok || len(dindRegionMap) != NREGION {
		t.Fatalf("serialized Dind region map = %#v, want %d indexed regions", attrs["Dind_region_map"], NREGION)
	}
	if got := float32(dindRegionMap["0"].(float64)); got != float32(0.00149) {
		t.Errorf("serialized Dind region 0 = %g, want %g", got, float32(0.00149))
	}
	if got := float32(dindRegionMap["1"].(float64)); got != float32(0.002) {
		t.Errorf("serialized Dind region 1 = %g, want %g", got, float32(0.002))
	}
	bExtRegionMap, ok := attrs["B_ext_region_map"].(map[string]any)
	if !ok || len(bExtRegionMap) != NREGION {
		t.Fatalf("serialized B_ext region map = %#v, want %d indexed regions", attrs["B_ext_region_map"], NREGION)
	}
	if got := bExtRegionMap["1"].([]any); len(got) != 3 || got[0].(float64) != 0.4 || got[1].(float64) != 0.5 || got[2].(float64) != 0.6 {
		t.Errorf("serialized B_ext region 1 = %v, want [0.4 0.5 0.6]", got)
	}
	if got := Dind.GetRegion(1); got != float64(float32(0.002)) {
		t.Errorf("Dind region 1 = %g, want %g", got, float64(float32(0.002)))
	}
	if got := Aex.GetRegion(2); got != float64(float32(2.6e-11)) {
		t.Errorf("Aex region 2 = %g, want %g", got, float64(float32(2.6e-11)))
	}
	if got, want := B_ext.perRegion.GetRegion(1), [3]float64{float64(float32(0.4)), float64(float32(0.5)), float64(float32(0.6))}; got != want {
		t.Errorf("B_ext region 1 = %v, want %v", got, want)
	}
}

func TestTimeDependentScalarMetadataDoesNotEvaluateBeforeTimeUpdate(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "run.zarr") + "/"
	previousOutput, previousFormat := outputdir, StorageFormat
	previousMetadata := structuredOutput.metadata
	previousLastSave := structuredOutput.lastMetadataSave
	previousAex := snapshotRegionwiseMetadataTestState(&Aex.regionwise)
	previousTime := Time
	const functionName = "codex_metadata_time_probe"
	previousFunction, hadFunction := World.Identifiers[functionName]
	var calls int
	World.Func(functionName, func(t float64) float64 {
		calls++
		return t + 3
	})
	defer func() {
		outputdir, StorageFormat = previousOutput, previousFormat
		structuredOutput.metadata = previousMetadata
		structuredOutput.lastMetadataSave = previousLastSave
		Time = previousTime
		restoreRegionwiseMetadataTestState(&Aex.regionwise, previousAex)
		if hadFunction {
			World.Identifiers[functionName] = previousFunction
		} else {
			delete(World.Identifiers, functionName)
		}
	}()

	outputdir, StorageFormat = dir, StorageFormatZarr
	structuredOutput.metadata = make(map[string]any)
	Aex.setRegions(0, NREGION, []float64{0})
	code, err := World.Compile("Aex = " + functionName + "(t)")
	if err != nil {
		t.Fatal(err)
	}
	code.Eval()
	if _, ok := regionwiseMetadataSnapshot(&Aex.regionwise); ok {
		t.Fatal("time-dependent scalar snapshot should retain its unevaluated expression")
	}
	if calls != 0 {
		t.Fatalf("time-dependent scalar calls during assignment/metadata capture = %d, want 0", calls)
	}
	structuredOutput.mu.Lock()
	err = writeRunMetadataLocked()
	structuredOutput.mu.Unlock()
	if err != nil {
		t.Fatal(err)
	}
	if calls != 0 {
		t.Fatalf("time-dependent scalar calls during .zattrs serialization = %d, want 0", calls)
	}

	Time = previousTime + 1
	if got, want := Aex.GetRegion(0), float64(float32(Time+3)); got != want {
		t.Fatalf("Aex(t) = %g at updated time, want %g", got, want)
	}
	if calls == 0 {
		t.Fatal("time-dependent scalar was not evaluated when the parameter updated")
	}
}

func TestClosestDivisor(t *testing.T) {
	for _, tc := range []struct{ value, requested, want int }{
		{64, 3, 2},
		{64, 7, 8},
		{60, 7, 6},
		{3, 2, 1},
	} {
		if got := closestDivisor(tc.value, tc.requested); got != tc.want {
			t.Fatalf("closestDivisor(%d, %d) = %d, want %d", tc.value, tc.requested, got, tc.want)
		}
	}
}

func TestValidateDatasetName(t *testing.T) {
	for _, bad := range []string{"", ".", "..", "../escape", "/absolute"} {
		func() {
			defer func() {
				if recover() == nil {
					t.Errorf("validateDatasetName(%q) did not panic", bad)
				}
			}()
			validateDatasetName(bad)
		}()
	}
	if got := validateDatasetName("fields/m"); got != "fields/m" {
		t.Fatalf("valid name changed to %q", got)
	}
}

func TestStructuredFileReferences(t *testing.T) {
	file, dataset, ok := parseHDF5Reference("run/m/m.h5:/4")
	if !ok || file != "run/m/m.h5" || dataset != "/4" {
		t.Fatalf("HDF5 reference parsed as %q %q %v", file, dataset, ok)
	}
	dir, step, ok := parseZarrReference("run.zarr/m/3.0.0.0.0")
	if !ok || dir != "run.zarr/m" || step != 3 {
		t.Fatalf("Zarr chunk reference parsed as %q %d %v", dir, step, ok)
	}
	dir, step, ok = parseZarrReference("run.zarr/m:7")
	if !ok || dir != "run.zarr/m" || step != 7 {
		t.Fatalf("Zarr dataset reference parsed as %q %d %v", dir, step, ok)
	}
}

func TestTablePreviewSnapshotBoundedAndIndependent(t *testing.T) {
	previous := tableHistory
	defer func() { tableHistory = previous }()
	tableHistory = tableHistoryState{columns: []string{"t", "unused"}, units: []string{"s", ""}, data: map[string][]float64{"t": {0, 1, 2, 3, 4, 5, 6, 7, 8, 9}, "unused": {9}}}
	columns, _, values := TablePreviewSnapshot([]string{"t"}, 2, 3)
	if len(columns) != 2 || len(values) != 1 || len(values["t"]) != 2 || values["t"][0] != 6 || values["t"][1] != 9 {
		t.Fatalf("unexpected bounded snapshot: %v %v", columns, values)
	}
	values["t"][0] = 100
	if tableHistory.data["t"][6] != 6 {
		t.Fatal("snapshot aliases scientific history")
	}
	_, _, values = TablePreviewSnapshot(nil, 0, 1)
	if len(values) != 0 {
		t.Fatal("metadata request copied history")
	}
}
