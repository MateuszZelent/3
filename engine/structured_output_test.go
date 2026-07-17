package engine

import "testing"

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
