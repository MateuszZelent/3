package engine

import (
	"math"
	"testing"
)

func TestClosestSevenSmooth(t *testing.T) {
	for _, tc := range []struct{ input, want int }{{1, 1}, {64, 64}, {127, 126}, {129, 128}, {257, 256}} {
		if got := closestSevenSmooth(tc.input); got != tc.want {
			t.Fatalf("closestSevenSmooth(%d) = %d, want %d", tc.input, got, tc.want)
		}
	}
}

func TestAmumaxShapes(t *testing.T) {
	if !EqTriangle(2)(0, 0, 0) || EqTriangle(2)(2, 2, 0) {
		t.Fatal("EqTriangle membership is incorrect")
	}
	if !Hexagon(1)(0, 0, 0) || Hexagon(1)(2, 0, 0) {
		t.Fatal("Hexagon membership is incorrect")
	}
	if !Diamond(2, 2)(0, 0, 0) || Diamond(2, 2)(1, 1, 0) {
		t.Fatal("Diamond membership is incorrect")
	}
	if !Squircle(2, 4, 1, 0.5)(0, 0, 0) || Squircle(2, 4, 1, 0.5)(0, 0, 1) {
		t.Fatal("Squircle membership is incorrect")
	}
	if !SinWaveguide2(4, 2, 1, 2, 1, 0, 0)(0, 0, 0) {
		t.Fatal("SinWaveguide2 should contain its centerline")
	}
}

func TestRadialTexture(t *testing.T) {
	cfg := Radial(1, -1)
	v := cfg(1, 0, 0)
	if math.Abs(v[X]-1) > 1e-12 || v[Y] != 0 || v[Z] != 0 {
		t.Fatalf("Radial(1, -1) at +x = %v", v)
	}
	center := cfg(0, 0, 0)
	if center[X] != 0 || center[Y] != 0 || center[Z] != -1 {
		t.Fatalf("Radial center = %v, want [0 0 -1]", center)
	}
}
