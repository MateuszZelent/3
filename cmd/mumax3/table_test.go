package main

import (
	"bytes"
	"path/filepath"
	"testing"

	"github.com/mumax/3/tabular"
)

func TestExportCommittedTable(t *testing.T) {
	for _, format := range []string{"zarr", "hdf5"} {
		t.Run(format, func(t *testing.T) {
			dir := filepath.Join(t.TempDir(), "table")
			w, err := tabular.New(dir, format, []string{"t", "position"}, []string{"s", "m"})
			if err != nil {
				t.Fatal(err)
			}
			if err = w.Append([][]float64{{0, 1}, {0.123456789012345, 0.987654321098765}}); err != nil {
				t.Fatal(err)
			}
			var out bytes.Buffer
			if err = exportTable(dir, &out); err != nil {
				t.Fatal(err)
			}
			want := "# t (s)\tposition (m)\n0\t0.123456789012345\n1\t0.987654321098765\n"
			if out.String() != want {
				t.Fatalf("export lost schema or precision: %q", out.String())
			}
		})
	}
}
