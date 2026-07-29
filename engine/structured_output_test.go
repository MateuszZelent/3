package engine

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

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
