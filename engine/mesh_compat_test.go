package engine

import (
	"math"
	"testing"
)

func TestResolveCompatibilityAxis(t *testing.T) {
	tests := []struct {
		name             string
		n                int
		d, total         float64
		hasN, hasD, hasT bool
		wantN            int
		wantD, wantT     float64
	}{
		{"N+d", 10, 2e-9, 0, true, true, false, 10, 2e-9, 20e-9},
		{"N+T", 10, 0, 20e-9, true, false, true, 10, 2e-9, 20e-9},
		{"d+T", 0, 3e-9, 20e-9, false, true, true, 7, 20e-9 / 7, 20e-9},
	}
	for _, test := range tests {
		n, d, total := resolveCompatibilityAxis(test.n, test.d, test.total, test.hasN, test.hasD, test.hasT)
		if n != test.wantN || math.Abs(d-test.wantD) > 1e-20 || math.Abs(total-test.wantT) > 1e-20 {
			t.Fatalf("%s: got N=%d d=%g T=%g, want N=%d d=%g T=%g", test.name, n, d, total, test.wantN, test.wantD, test.wantT)
		}
	}
}

func TestAmumaxMeshSyntaxCompiles(t *testing.T) {
	_, err := World.Compile("Tx=100e-9; Ty=40e-9; Tz=10e-9; dx=10e-9; dy=10e-9; dz=5e-9")
	if err != nil {
		t.Fatal(err)
	}
}
