package zarr

import (
	"encoding/json"
	"math"
	"os"
	"path/filepath"
	"testing"

	"github.com/mumax/3/data"
)

func TestChunkedRoundTrip(t *testing.T) {
	src := data.NewSlice(3, [3]int{5, 4, 3})
	defer src.Free()
	for c := 0; c < src.NComp(); c++ {
		for z := 0; z < src.Size()[2]; z++ {
			for y := 0; y < src.Size()[1]; y++ {
				for x := 0; x < src.Size()[0]; x++ {
					src.Set(c, x, y, z, float64(c*1000+z*100+y*10+x)+0.25)
				}
			}
		}
	}
	dir := filepath.Join(t.TempDir(), "m")
	if err := WriteStep(dir, 0, src, [4]int{2, 3, 2, 2}); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(dir, ".zgroup")); !os.IsNotExist(err) {
		t.Fatalf("array directory must not contain .zgroup: %v", err)
	}
	got, err := ReadStep(dir, 0)
	if err != nil {
		t.Fatal(err)
	}
	defer got.Free()
	if got.Size() != src.Size() || got.NComp() != src.NComp() {
		t.Fatalf("shape = %v x %d, want %v x %d", got.Size(), got.NComp(), src.Size(), src.NComp())
	}
	for c := 0; c < src.NComp(); c++ {
		for z := 0; z < src.Size()[2]; z++ {
			for y := 0; y < src.Size()[1]; y++ {
				for x := 0; x < src.Size()[0]; x++ {
					if diff := math.Abs(got.Get(c, x, y, z) - src.Get(c, x, y, z)); diff != 0 {
						t.Fatalf("value[%d,%d,%d,%d] differs by %g", c, x, y, z, diff)
					}
				}
			}
		}
	}
}

func TestObjectAttributes(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "run.zarr")
	if err := WriteGroup(dir); err != nil {
		t.Fatal(err)
	}
	want := map[string]any{"Nx": 16, "dx": 2e-9, "start_time": "now"}
	if err := WriteObjectAttributes(dir, want); err != nil {
		t.Fatal(err)
	}
	b, err := os.ReadFile(filepath.Join(dir, ".zattrs"))
	if err != nil {
		t.Fatal(err)
	}
	var got map[string]any
	if err := json.Unmarshal(b, &got); err != nil {
		t.Fatal(err)
	}
	if got["Nx"] != float64(16) || got["dx"] != 2e-9 || got["start_time"] != "now" {
		t.Fatalf("attributes = %#v", got)
	}
}

func TestMetadataAdvancesAcrossSteps(t *testing.T) {
	src := data.NewSlice(1, [3]int{2, 2, 1})
	defer src.Free()
	dir := filepath.Join(t.TempDir(), "q")
	if err := WriteStep(dir, 0, src, [4]int{1, 2, 1, 1}); err != nil {
		t.Fatal(err)
	}
	if err := WriteStep(dir, 1, src, [4]int{1, 2, 1, 1}); err != nil {
		t.Fatal(err)
	}
	meta, err := ReadMetadata(dir)
	if err != nil {
		t.Fatal(err)
	}
	if meta.Shape != [5]int{2, 1, 2, 2, 1} {
		t.Fatalf("shape = %v", meta.Shape)
	}
}
