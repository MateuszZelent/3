package mag

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/mumax/3/data"
)

func TestKernelCacheRoundTripAndNameCompatibility(t *testing.T) {
	size := [3]int{3, 2, 1}
	var source [3][3]*data.Slice
	for _, component := range kernelComponents(size) {
		s := data.NewSlice(1, size)
		for z := 0; z < size[Z]; z++ {
			for y := 0; y < size[Y]; y++ {
				for x := 0; x < size[X]; x++ {
					s.Set(0, x, y, z, float64(component[0]*100+component[1]*10+x+y)+0.25)
				}
			}
		}
		source[component[0]][component[1]] = s
		defer s.Free()
	}

	dir := t.TempDir()
	name := kernelCacheName([3]int{3, 2, 1}, [3]int{0, 0, 0}, [3]float64{1e-9, 2e-9, 3e-9}, 6, dir)
	wantName := filepath.Join(dir, "3_2_1_0_0_0_1.000000e-09_2.000000e-09_3.000000e-09_6.cache2")
	if name != wantName {
		t.Fatalf("cache name = %q, want %q", name, wantName)
	}
	if err := saveKernelCache(name, source); err != nil {
		t.Fatal(err)
	}
	got, err := loadKernelCache(name, size)
	if err != nil {
		t.Fatal(err)
	}
	for _, component := range kernelComponents(size) {
		defer got[component[0]][component[1]].Free()
		for z := 0; z < size[Z]; z++ {
			for y := 0; y < size[Y]; y++ {
				for x := 0; x < size[X]; x++ {
					if have, want := got[component[0]][component[1]].Get(0, x, y, z), source[component[0]][component[1]].Get(0, x, y, z); have != want {
						t.Fatalf("component %v at %d,%d,%d = %v, want %v", component, x, y, z, have, want)
					}
				}
			}
		}
	}
	if got[Y][X] != got[X][Y] || got[Z][X] != nil || got[Z][Y] != nil {
		t.Fatal("2D kernel symmetry is incorrect")
	}

	if err := os.WriteFile(name, []byte("truncated"), 0o666); err != nil {
		t.Fatal(err)
	}
	if _, err := loadKernelCache(name, size); err == nil {
		t.Fatal("truncated cache was accepted")
	}
}
