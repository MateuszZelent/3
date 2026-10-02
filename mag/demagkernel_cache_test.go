package mag

import (
	"bytes"
	"encoding/binary"
	"math"
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

// Independent bulk encoder verifies compatibility with the previous raw wire format,
// including block boundaries and exact Float32 bit patterns.
func TestKernelCacheLegacyBytesAndBlockTail(t *testing.T) {
	for _, size := range [][3]int{{kernelCacheBlockBytes/4 + 3, 1, 1}, {7, 5, 3}} {
		var source [3][3]*data.Slice
		var legacy bytes.Buffer
		for _, c := range kernelComponents(size) {
			s := data.NewSlice(1, size)
			source[c[0]][c[1]] = s
			defer s.Free()
			for i := range s.Host()[0] {
				s.Host()[0][i] = float32(i%257-128) * 0.125
			}
			s.Host()[0][0] = math.Float32frombits(0x80000000)
			if err := binary.Write(&legacy, binary.LittleEndian, s.Host()[0]); err != nil {
				t.Fatal(err)
			}
		}
		name := filepath.Join(t.TempDir(), "legacy.cache2")
		if err := saveKernelCache(name, source); err != nil {
			t.Fatal(err)
		}
		wire, err := os.ReadFile(name)
		if err != nil {
			t.Fatal(err)
		}
		if !bytes.Equal(wire, legacy.Bytes()) {
			t.Fatal("wire format changed")
		}
		got, err := loadKernelCache(name, size)
		if err != nil {
			t.Fatal(err)
		}
		for _, c := range kernelComponents(size) {
			defer got[c[0]][c[1]].Free()
			for i, v := range got[c[0]][c[1]].Host()[0] {
				if math.Float32bits(v) != math.Float32bits(source[c[0]][c[1]].Host()[0][i]) {
					t.Fatal("bit mismatch", c, i)
				}
			}
		}
		if got[Y][X] != got[X][Y] || got[Z][X] != got[X][Z] || got[Z][Y] != got[Y][Z] {
			t.Fatal("symmetry aliases lost")
		}
	}
}

func TestKernelCacheFailedSavePreservesExisting(t *testing.T) {
	name := filepath.Join(t.TempDir(), "keep.cache2")
	original := []byte("existing valid cache")
	if err := os.WriteFile(name, original, 0600); err != nil {
		t.Fatal(err)
	}
	var kernel [3][3]*data.Slice
	kernel[0][0] = data.NewSlice(1, [3]int{2, 2, 2})
	defer kernel[0][0].Free()
	if err := saveKernelCache(name, kernel); err == nil {
		t.Fatal("missing components accepted")
	}
	wire, _ := os.ReadFile(name)
	if !bytes.Equal(wire, original) {
		t.Fatal("failed save changed existing file")
	}
}

func BenchmarkKernelCacheWrite(b *testing.B) {
	size := [3]int{64, 64, 64}
	var source [3][3]*data.Slice
	for _, c := range kernelComponents(size) {
		source[c[0]][c[1]] = data.NewSlice(1, size)
		defer source[c[0]][c[1]].Free()
	}
	for _, mode := range []string{"legacy-per-float", "stream-block"} {
		b.Run(mode, func(b *testing.B) {
			name := filepath.Join(b.TempDir(), "kernel.cache2")
			b.SetBytes(6 * 64 * 64 * 64 * 4)
			b.ResetTimer()
			for n := 0; n < b.N; n++ {
				if mode == "stream-block" {
					if err := saveKernelCache(name, source); err != nil {
						b.Fatal(err)
					}
				} else {
					f, err := os.Create(name)
					if err != nil {
						b.Fatal(err)
					}
					for _, c := range kernelComponents(size) {
						for _, v := range source[c[0]][c[1]].Host()[0] {
							if err := binary.Write(f, binary.LittleEndian, v); err != nil {
								b.Fatal(err)
							}
						}
					}
					if err := f.Sync(); err != nil {
						b.Fatal(err)
					}
					if err := f.Close(); err != nil {
						b.Fatal(err)
					}
				}
			}
		})
	}
}

func BenchmarkKernelCacheRead(b *testing.B) {
	size := [3]int{64, 64, 64}
	var source [3][3]*data.Slice
	for _, c := range kernelComponents(size) {
		source[c[0]][c[1]] = data.NewSlice(1, size)
		defer source[c[0]][c[1]].Free()
	}
	name := filepath.Join(b.TempDir(), "kernel.cache2")
	if err := saveKernelCache(name, source); err != nil {
		b.Fatal(err)
	}
	for _, mode := range []string{"legacy-full-file", "stream-block"} {
		b.Run(mode, func(b *testing.B) {
			b.SetBytes(6 * 64 * 64 * 64 * 4)
			b.ReportAllocs()
			b.ResetTimer()
			for n := 0; n < b.N; n++ {
				if mode == "stream-block" {
					kernel, err := loadKernelCache(name, size)
					if err != nil {
						b.Fatal(err)
					}
					for _, c := range kernelComponents(size) {
						kernel[c[0]][c[1]].Free()
					}
				} else {
					wire, err := os.ReadFile(name)
					if err != nil {
						b.Fatal(err)
					}
					offset := 0
					for range kernelComponents(size) {
						slice := data.NewSlice(1, size)
						values := slice.Host()[0]
						for i := range values {
							values[i] = math.Float32frombits(binary.LittleEndian.Uint32(wire[offset:]))
							offset += 4
						}
						slice.Free()
					}
				}
			}
		})
	}
}
