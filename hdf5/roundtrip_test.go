package hdf5

import (
	"path/filepath"
	"testing"
)

func TestRoundTrip(t *testing.T) {
	dir := t.TempDir()
	w, err := NewMultiWriter(dir)
	if err != nil {
		t.Fatal(err)
	}
	data := make([][][][]float32, 3)
	for c := range data {
		data[c] = make([][][]float32, 2)
		for z := range data[c] {
			data[c][z] = make([][]float32, 3)
			for y := range data[c][z] {
				data[c][z][y] = make([]float32, 4)
				for x := range data[c][z][y] {
					data[c][z][y][x] = float32(c*1000 + z*100 + y*10 + x)
				}
			}
		}
	}
	if err := w.SaveArray("m", 0, data, [3]int{4, 3, 2}, 3); err != nil {
		t.Fatal(err)
	}
	if err := w.SaveTimestamps("m", []float64{0}); err != nil {
		t.Fatal(err)
	}
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}
	got, err := ReadArray(filepath.Join(dir, "m", "m.h5"), "/0")
	if err != nil {
		t.Fatal(err)
	}
	defer got.Free()
	if got.Size() != [3]int{4, 3, 2} || got.NComp() != 3 {
		t.Fatalf("shape = %v x %d", got.Size(), got.NComp())
	}
	if got.Get(2, 3, 2, 1) != float64(data[2][1][2][3]) {
		t.Fatalf("last value = %g", got.Get(2, 3, 2, 1))
	}
}
