package coreposition

import (
	"math"
	"testing"
)

func sample(size [3]int, cell [3]float64, pbc [3]bool, fn func(x, y, z float64) float64) Field {
	f := Field{Size: size, Cell: cell, PBC: pbc, Mz: make([][][]float32, size[2])}
	for z := range f.Mz {
		f.Mz[z] = make([][]float32, size[1])
		for y := range f.Mz[z] {
			f.Mz[z][y] = make([]float32, size[0])
			for x := range f.Mz[z][y] {
				f.Mz[z][y][x] = float32(fn(f.coord(0, x), f.coord(1, y), f.coord(2, z)))
			}
		}
	}
	return f
}

func skyrmionProfile(x, y, cx, cy, width, pol float64) float64 {
	x -= cx
	y -= cy
	mz := pol * (2*math.Exp(-(x*x+y*y)/(width*width)) - 1)
	inplane := 1 - math.Abs(mz)
	return mz / math.Hypot(mz, inplane)
}

func TestSkyrmionDiametersPolaritiesAndBreathing(t *testing.T) {
	for _, pol := range []int{-1, 1} {
		for _, width := range []float64{2, 4, 8, 16, 24, 32} {
			cx, cy := 11.25e-9, -7.75e-9
			f := sample([3]int{192, 160, 1}, [3]float64{1e-9, 1e-9, 1e-9}, [3]bool{}, func(x, y, z float64) float64 { return skyrmionProfile(x, y, cx, cy, width*1e-9, float64(pol)) })
			r := Skyrmion(f, SkyrmionOptions{Polarity: pol, Layer: -1, Background: -float64(pol), Threshold: 0.1})
			if !r.Valid || math.Hypot(r.Position[0]-cx, r.Position[1]-cy) > 0.1e-9 {
				t.Errorf("pol=%d width=%g center_nm=(%.6f,%.6f) valid=%v reason=%s", pol, width, r.Position[0]*1e9, r.Position[1]*1e9, r.Valid, r.Reason)
			}
		}
	}
}

func TestSkyrmionPeriodicGeometryLayersAndShift(t *testing.T) {
	cx, cy := 63.25e-9, -93.75e-9
	f := sample([3]int{128, 96, 3}, [3]float64{1e-9, 2e-9, 3e-9}, [3]bool{true, true, false}, func(x, y, z float64) float64 {
		return skyrmionProfile(wrap(x-cx, 128e-9), wrap(y-cy, 192e-9), 0, 0, 8e-9, 1)
	})
	f.Shift = [2]float64{5e-9, -2e-9}
	for _, layer := range []int{-1, 0, 2} {
		r := Skyrmion(f, SkyrmionOptions{Polarity: 1, Layer: layer, Background: -1, Threshold: 0.1, Reference: [2]float64{cx + f.Shift[0], cy + f.Shift[1]}})
		z := 0.0
		if layer >= 0 {
			z = f.coord(2, layer)
		}
		if !r.Valid || math.Abs(r.Position[0]-cx-f.Shift[0]) > 0.1e-9 || math.Abs(r.Position[1]-cy-f.Shift[1]) > 0.2e-9 || math.Abs(r.Position[2]-z) > 1e-20 {
			t.Fatalf("layer=%d result=%+v", layer, r)
		}
	}
}

func TestSkyrmionSelectsOneObjectAndDetectsLoss(t *testing.T) {
	f := sample([3]int{192, 96, 1}, [3]float64{1e-9, 1e-9, 1e-9}, [3]bool{}, func(x, y, z float64) float64 {
		center := 30e-9
		if x < 0 {
			center = -30e-9
		}
		return skyrmionProfile(x, y, center, 0, 6e-9, 1)
	})
	o := SkyrmionOptions{Polarity: 1, Layer: -1, Background: -1, Threshold: 0.1, Reference: [2]float64{29e-9, 0}}
	r := Skyrmion(f, o)
	if !r.Valid || math.Abs(r.Position[0]-30e-9) > 0.1e-9 {
		t.Fatalf("merged cores: %+v", r)
	}
	o.Reference = [2]float64{0, 0}
	o.MaxDisplacement = 3e-9
	if r = Skyrmion(f, o); r.Valid {
		t.Fatal("silently jumped to another object:", r)
	}
}

func TestSkyrmionMaskAndInvalidStates(t *testing.T) {
	f := sample([3]int{128, 96, 1}, [3]float64{1e-9, 1e-9, 1e-9}, [3]bool{}, func(x, y, z float64) float64 { return skyrmionProfile(x, y, 2e-9, -3e-9, 6e-9, 1) })
	mask := sample(f.Size, f.Cell, f.PBC, func(x, y, z float64) float64 {
		if math.Hypot(x, y) < 35e-9 {
			return 1
		}
		return 0
	})
	f.Mask = mask.Mz
	r := Skyrmion(f, SkyrmionOptions{Polarity: 1, Layer: -1, Background: -1, Threshold: 0.1})
	if !r.Valid || math.Hypot(r.Position[0]-2e-9, r.Position[1]+3e-9) > 0.1e-9 {
		t.Fatal(r)
	}
	for _, uniform := range []float64{-1, 1, math.NaN()} {
		f = sample(f.Size, f.Cell, f.PBC, func(x, y, z float64) float64 { return uniform })
		if r = Skyrmion(f, SkyrmionOptions{Polarity: 1, Layer: -1, Background: -1, Threshold: 0.1}); r.Valid || finite(r.Position[0]) {
			t.Fatal("invalid field produced a valid position:", r)
		}
	}
	f = sample([3]int{64, 64, 1}, [3]float64{1e-9, 1e-9, 1e-9}, [3]bool{}, func(x, y, z float64) float64 { return skyrmionProfile(x, y, 31e-9, 0, 6e-9, 1) })
	if r = Skyrmion(f, SkyrmionOptions{Polarity: 1, Layer: -1, Background: -1, Threshold: 0.1}); r.Valid {
		t.Fatal("truncated core accepted:", r)
	}
}

func TestVortexInterpolationBoundariesAndSmallGrids(t *testing.T) {
	cx, cy := 11.25e-9, -7.75e-9
	f := sample([3]int{128, 96, 1}, [3]float64{1e-9, 2e-9, 1e-9}, [3]bool{}, func(x, y, z float64) float64 { return -math.Exp(-((x-cx)*(x-cx) + (y-cy)*(y-cy)) / 16e-18) })
	f.Shift = [2]float64{3e-9, 5e-9}
	r := Vortex(f, -1)
	if !r.Valid || math.Abs(r.Position[0]-cx-3e-9) > 0.1e-9 || math.Abs(r.Position[1]-cy-5e-9) > 0.2e-9 || r.Polarization >= 0 {
		t.Fatal(r)
	}
	for _, n := range []int{1, 2, 3} {
		f = sample([3]int{n, n, 1}, [3]float64{1e-9, 1e-9, 1e-9}, [3]bool{}, func(x, y, z float64) float64 { return 1 })
		if r = Vortex(f, -1); r.Valid {
			t.Fatal("unresolved grid:", r)
		}
	}
	if _, ok := PeakOffset(1, 1, 1); ok {
		t.Fatal("plateau accepted")
	}
	if _, ok := PeakOffset(0, 1, 1); ok {
		t.Fatal("convex peak accepted")
	}
}

func TestSkyrmionOddGridTiltAndLayerAggregation(t *testing.T) {
	for _, layer := range []int{-1, 0, 2} {
		f := sample([3]int{129, 97, 3}, [3]float64{1e-9, 1.5e-9, 2e-9}, [3]bool{}, func(x, y, z float64) float64 {
			// The core bends by 1nm per layer. Small smooth mz noise and a
			// tilted background remain below the configured contrast threshold.
			v := skyrmionProfile(x, y, 4.25e-9+0.5*z, -2.75e-9, 8e-9, 1)
			return math.Max(-1, math.Min(1, v+0.008+0.001*math.Sin(x/1e-9+y/2e-9)))
		})
		r := Skyrmion(f, SkyrmionOptions{Polarity: 1, Layer: layer, Background: -1, Threshold: 0.1})
		expected := 4.25e-9
		if layer >= 0 {
			expected += 0.5 * f.coord(2, layer)
		}
		if !r.Valid || math.Abs(r.Position[0]-expected) > 0.1e-9 || math.Abs(r.Position[1]+2.75e-9) > 0.15e-9 {
			t.Fatalf("layer=%d result=%+v", layer, r)
		}
	}
}

func TestVortexPeriodicBoundaryAndGeometryMask(t *testing.T) {
	cx, cy := 31.25e-9, -30.75e-9
	f := sample([3]int{64, 64, 1}, [3]float64{1e-9, 1e-9, 1e-9}, [3]bool{true, true, false}, func(x, y, z float64) float64 {
		dx, dy := wrap(x-cx, 64e-9), wrap(y-cy, 64e-9)
		return math.Exp(-(dx*dx + dy*dy) / 16e-18)
	})
	r := Vortex(f, -1)
	if !r.Valid || math.Abs(r.Position[0]-cx) > 0.1e-9 || math.Abs(r.Position[1]-cy) > 0.1e-9 {
		t.Fatal(r)
	}
	mask := sample(f.Size, f.Cell, f.PBC, func(x, y, z float64) float64 {
		if x > 30e-9 {
			return 0
		}
		return 1
	})
	f.Mask = mask.Mz
	if r = Vortex(f, -1); r.Valid {
		t.Fatal("unlocalized peak at geometry edge:", r)
	}
}
