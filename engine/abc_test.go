package engine

import (
	"fmt"
	"math"
	"strings"
	"testing"

	"github.com/mumax/3/data"
)

func TestABCProfilesStayNormalized(t *testing.T) {
	profiles := []struct {
		name  string
		param float64
	}{
		{"linear", 0},
		{"power", 2},
		{"tanh", 4},
		{"smootherstep", 0},
	}
	for _, test := range profiles {
		profile := selectProfile(test.name)
		previous := -1.0
		for step := 0; step <= 100; step++ {
			value := profile(float64(step)/100, test.param)
			if value < 0 || value > 1 || value < previous {
				t.Fatalf("%s profile is not normalized/monotonic at %d: %g after %g", test.name, step, value, previous)
			}
			previous = value
		}
	}
}

func TestParseBoundarySpec(t *testing.T) {
	spec := parseBoundarySpec("x-, y+")
	if !spec.XMinus || spec.XPlus || spec.YMinus || !spec.YPlus {
		t.Fatalf("unexpected boundary spec: %+v", spec)
	}
}

func TestNormalizedBoundaryRampAndPlateau(t *testing.T) {
	profile := selectProfile("smootherstep")
	if got := normalizedBoundaryValue(1, 10, 4, profile, 0); got != 1 {
		t.Fatalf("plateau value = %g, want 1", got)
	}
	if got := normalizedBoundaryValue(10, 10, 4, profile, 0); got != 0 {
		t.Fatalf("bulk value = %g, want 0", got)
	}
	middle := normalizedBoundaryValue(8, 10, 4, profile, 0)
	if math.Abs(middle-0.5) > 1e-12 {
		t.Fatalf("ramp midpoint = %g, want 0.5", middle)
	}
}

func TestABCDispersionProducesFiniteFrequency(t *testing.T) {
	state := abcDispersion(1e7, abcDispersionParams{
		Mode: "bv", Ms: 800e3, Aex: 13e-12, D: 10e-9, Heff: 50e3, SideSign: 1,
	})
	if !isFinitePositive(state.Omega) || state.H1 <= 0 || state.H2 <= 0 {
		t.Fatalf("invalid dispersion state: %+v", state)
	}
}

// Cell-centered sampling can leave both layers empty even with positive
// physical widths: one-cell axes have their only sample at 50%, beyond 49%.
func TestABCSampledAxisCoverage(t *testing.T) {
	for _, tc := range []struct {
		name, direction string
		size            [3]int
		cell            [3]float64
		x, y            bool
	}{
		{"long-x", "x", [3]int{4096, 8, 1}, [3]float64{5e-9, 5e-9, 5e-9}, true, false},
		{"long-y", "y", [3]int{8, 4096, 1}, [3]float64{5e-9, 5e-9, 5e-9}, false, true},
		{"square", "xy", [3]int{128, 128, 1}, [3]float64{5e-9, 5e-9, 5e-9}, true, true},
		{"anisotropic", "xy", [3]int{128, 64, 3}, [3]float64{2.5e-9, 10e-9, 5e-9}, true, true},
		{"empty-x", "x", [3]int{1, 1024, 1}, [3]float64{5e-9, 5e-9, 5e-9}, false, false},
		{"empty-y", "y", [3]int{1024, 1, 1}, [3]float64{5e-9, 5e-9, 5e-9}, false, false},
		{"xy-on-line-x", "xy", [3]int{1024, 1, 1}, [3]float64{5e-9, 5e-9, 5e-9}, true, false},
		{"xy-on-line-y", "xy", [3]int{1, 1024, 1}, [3]float64{5e-9, 5e-9, 5e-9}, false, true},
		{"one-sided", "x-,y+", [3]int{64, 128, 1}, [3]float64{5e-9, 5e-9, 5e-9}, true, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			spec := parseBoundarySpec(tc.direction)
			total := abcSideWidths{100e-9, 100e-9, 100e-9, 100e-9}
			ramp := total
			normalizeSideWidths(&total, &ramp, spec, float64(tc.size[X])*tc.cell[X], float64(tc.size[Y])*tc.cell[Y], tc.cell[X], tc.cell[Y])
			x, y := sampledABCAxes(spec, total, tc.cell)
			if x != tc.x || y != tc.y {
				t.Fatalf("sampled axes=(%v,%v), want (%v,%v)", x, y, tc.x, tc.y)
			}
		})
	}
}

func TestABCRejectedConfigurationPreservesExistingLayer(t *testing.T) {
	for _, tc := range []struct {
		name, direction, profile string
		size                     [3]int
	}{
		{"empty-x", "x", "smootherstep", [3]int{1, 64, 1}},
		{"empty-y", "y", "smootherstep", [3]int{64, 1, 1}},
		{"invalid-profile", "x", "invalid", [3]int{64, 64, 1}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			previousMesh, previousTerms := globalmesh_, SpongeAlpha.extraTerms
			globalmesh_ = *data.NewMesh(tc.size[X], tc.size[Y], tc.size[Z], 5e-9, 5e-9, 5e-9)
			sentinel := data.NewSlice(1, tc.size)
			SpongeAlpha.extraTerms = []mulmask{{mask: sentinel}}
			defer func() { globalmesh_ = previousMesh; SpongeAlpha.extraTerms = previousTerms; sentinel.Free() }()
			defer func() {
				failure := recover()
				if failure == nil {
					t.Fatal("invalid ABC was accepted")
				}
				if tc.profile != "invalid" && !strings.Contains(fmt.Sprint(failure), "no cell centers") {
					t.Fatalf("unexpected error: %v", failure)
				}
				if sentinel.Size() != tc.size || len(SpongeAlpha.extraTerms) != 1 || SpongeAlpha.extraTerms[0].mask != sentinel {
					t.Fatal("rejected ABC removed the previous working layer")
				}
			}()
			SetAbsorbingBoundary(100e-9, .5, tc.direction, tc.profile, 0)
		})
	}
}

func TestABCGeometryOverlap(t *testing.T) {
	for _, tc := range []struct {
		name       string
		mask, fill []float32
		overlap    bool
	}{
		{"cropped-sample", []float32{.5, 0, 0, .5}, []float32{0, 1, 1, 0}, false},
		{"sample-at-edge", []float32{.5, 0, 0, .5}, []float32{1, 1, 1, 1}, true},
		{"fractional-edge", []float32{.5, 0, 0, .5}, []float32{.01, 1, 1, 0}, true},
		{"zero-damping", []float32{0, 0, 0, 0}, []float32{1, 1, 1, 1}, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			mask := data.NewSlice(1, [3]int{4, 1, 1})
			defer mask.Free()
			geom := data.NewSlice(1, [3]int{4, 1, 1})
			defer geom.Free()
			copy(mask.Host()[0], tc.mask)
			copy(geom.Host()[0], tc.fill)
			if got := abcOverlapsGeometry(mask, geom); got != tc.overlap {
				t.Fatalf("overlap=%v, want %v", got, tc.overlap)
			}
		})
	}
}
