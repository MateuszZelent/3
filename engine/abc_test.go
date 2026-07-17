package engine

import (
	"math"
	"testing"
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
