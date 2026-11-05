package engine

import (
	"math"
	"strings"
	"testing"
)

func TestValidateFFTFrequencyGridBounds(t *testing.T) {
	if got, err := validateFFTFrequencyGrid(0, 16383, 1); err != nil || got != 16384 {
		t.Fatalf("accepted boundary grid: bins=%d err=%v", got, err)
	}
	for _, test := range []struct {
		name string
		min  float64
		max  float64
		step float64
	}{
		{name: "nan minimum", min: math.NaN(), max: 1, step: 0.1},
		{name: "infinite maximum", min: 0, max: math.Inf(1), step: 0.1},
		{name: "infinite step", min: 0, max: 1, step: math.Inf(1)},
		{name: "zero step", min: 0, max: 1, step: 0},
		{name: "reversed", min: 1, max: 0, step: 0.1},
		{name: "too many bins", min: 0, max: 16384, step: 1},
		{name: "overflowing quotient", min: -math.MaxFloat64, max: math.MaxFloat64, step: math.SmallestNonzeroFloat64},
	} {
		t.Run(test.name, func(t *testing.T) {
			if bins, err := validateFFTFrequencyGrid(test.min, test.max, test.step); err == nil {
				t.Fatalf("accepted invalid grid with %d bins", bins)
			}
		})
	}
	_, err := validateFFTFrequencyGrid(0, 16384, 1)
	if err == nil || !strings.Contains(err.Error(), "16384") {
		t.Fatalf("oversized grid error should state the bin cap: %v", err)
	}
}
