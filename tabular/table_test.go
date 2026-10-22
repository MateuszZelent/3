package tabular

import (
	"errors"
	"math/rand"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/mumax/3/httpfs"
)

func samples(start, n int) [][]float64 {
	a, b := make([]float64, n), make([]float64, n)
	for i := range a {
		a[i] = float64(start + i)
		b[i] = -a[i] - 0.25
	}
	return [][]float64{a, b}
}

func TestCommittedTables(t *testing.T) {
	for _, format := range []string{"zarr", "hdf5"} {
		t.Run(format, func(t *testing.T) {
			dir := filepath.Join(t.TempDir(), "table")
			w, err := New(dir, format, []string{"t", "mx"}, []string{"s", ""})
			if err != nil {
				t.Fatal(err)
			}
			for _, part := range []struct{ start, n int }{{0, 3}, {3, 1026}, {1029, 4096}} {
				if err = w.Append(samples(part.start, part.n)); err != nil {
					t.Fatal(err)
				}
				m, v, err := Read(dir)
				if err != nil {
					t.Fatal(err)
				}
				if m.Rows != part.start+part.n || !reflect.DeepEqual(v, samples(0, m.Rows)) {
					t.Fatalf("unaligned rows: %+v", m)
				}
			}
			before, err := os.Stat(filepath.Join(dir, "manifest.json"))
			if err != nil {
				t.Fatal(err)
			}
			if err = w.Append(samples(0, 0)); err != nil {
				t.Fatal(err)
			}
			after, _ := os.Stat(filepath.Join(dir, "manifest.json"))
			if before.ModTime() != after.ModTime() {
				t.Fatal("empty flush rewrote manifest")
			}
			if len(w.tail[0]) != w.Rows()%ChunkRows {
				t.Fatal("unbounded tail")
			}
			if _, err = New(dir, format, []string{"t", "mx"}, []string{"s", ""}); err == nil {
				t.Fatal("overwrote existing table")
			}
		})
	}
}

func TestInterruptedPublicationRetainsPrefix(t *testing.T) {
	for _, format := range []string{"zarr", "hdf5"} {
		t.Run(format, func(t *testing.T) {
			dir := filepath.Join(t.TempDir(), "table")
			w, err := New(dir, format, []string{"t", "mx"}, []string{"s", ""})
			if err != nil {
				t.Fatal(err)
			}
			if err = w.Append(samples(0, 3)); err != nil {
				t.Fatal(err)
			}
			for _, failure := range []string{"manifest.json", "mx/0"} {
				if format == "hdf5" && failure != "manifest.json" {
					continue
				}
				w.put = func(p string, b []byte) error {
					if strings.HasSuffix(filepath.ToSlash(p), failure) {
						return errors.New("simulated interrupted publication")
					}
					return httpfs.PutAtomic(p, b)
				}
				if err = w.Append(samples(3, 1026)); err == nil {
					t.Fatal("expected write failure")
				}
				m, v, err := Read(dir)
				if err != nil {
					t.Fatal(err)
				}
				if m.Rows != 3 || !reflect.DeepEqual(v, samples(0, 3)) || w.Rows() != 3 {
					t.Fatal("previously committed data changed")
				}
			}
			w.put = httpfs.PutAtomic
			if err = w.Append(samples(3, 1026)); err != nil {
				t.Fatal(err)
			}
			m, v, err := Read(dir)
			if err != nil {
				t.Fatal(err)
			}
			if m.Rows != 1029 || !reflect.DeepEqual(v, samples(0, 1029)) {
				t.Fatal("retry duplicated or lost rows")
			}
		})
	}
}

func TestRejectInvalidSchema(t *testing.T) {
	for _, columns := range [][]string{{"t", "t"}, {"../escape"}, {"a/b"}, {".zattrs"}} {
		units := make([]string, len(columns))
		if _, err := New(filepath.Join(t.TempDir(), "table"), "zarr", columns, units); err == nil {
			t.Fatal("accepted", columns)
		}
	}
}

func TestAppendCostDoesNotGrowWithCommittedHistory(t *testing.T) {
	for _, format := range []string{"zarr", "hdf5"} {
		t.Run(format, func(t *testing.T) {
			dir := filepath.Join(t.TempDir(), "table")
			w, err := New(dir, format, []string{"t", "value"}, []string{"s", ""})
			if err != nil {
				t.Fatal(err)
			}
			n := 32*ChunkRows + 512
			rng := rand.New(rand.NewSource(17))
			data := make([][]float64, 2)
			for c := range data {
				data[c] = make([]float64, n)
				for i := range data[c] {
					data[c][i] = rng.Float64()
				}
			}
			if err = w.Append(data); err != nil {
				t.Fatal(err)
			}
			first := filepath.Join(dir, "t", "0")
			if format == "hdf5" {
				first = filepath.Join(dir, "000000000000.h5")
			}
			before, err := os.Stat(first)
			if err != nil {
				t.Fatal(err)
			}
			written := 0
			w.put = func(p string, b []byte) error { written += len(b); return httpfs.PutAtomic(p, b) }
			if err = w.Append([][]float64{{1}, {2}}); err != nil {
				t.Fatal(err)
			}
			after, err := os.Stat(first)
			if err != nil {
				t.Fatal(err)
			}
			if before.ModTime() != after.ModTime() {
				t.Fatal("append rewrote a completed old chunk")
			}
			if written > 2*ChunkRows*8+4096 {
				t.Fatalf("append rewrote history instead of a bounded tail: %d bytes", written)
			}
		})
	}
}
