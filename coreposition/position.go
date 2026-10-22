// Package coreposition locates magnetic cores on CPU snapshots. Coordinates
// refer to cell centers, in metres in the laboratory frame.
package coreposition

import "math"

type Field struct {
	Mz    [][][]float32
	Mask  [][][]float32 // nil means full geometry
	Size  [3]int
	Cell  [3]float64
	PBC   [3]bool
	Shift [2]float64
}

type Result struct {
	Position     [3]float64
	Polarization float64
	Radius       float64 // support scale for continuity, not a measured core radius
	Valid        bool
	Reason       string
}

func invalid(reason string) Result {
	return Result{Position: [3]float64{math.NaN(), math.NaN(), math.NaN()}, Polarization: math.NaN(), Reason: reason}
}
func finite(x float64) bool { return !math.IsNaN(x) && !math.IsInf(x, 0) }

func (f Field) validate() bool {
	for a, n := range f.Size {
		if n < 1 || !finite(f.Cell[a]) || f.Cell[a] <= 0 {
			return false
		}
	}
	for _, v := range f.Shift {
		if !finite(v) {
			return false
		}
	}
	for _, data := range [][][][]float32{f.Mz, f.Mask} {
		if data == nil {
			continue
		}
		if len(data) != f.Size[2] {
			return false
		}
		for _, plane := range data {
			if len(plane) != f.Size[1] {
				return false
			}
			for _, row := range plane {
				if len(row) != f.Size[0] {
					return false
				}
			}
		}
	}
	return f.Mz != nil
}

func (f Field) weight(x, y, z int) float64 {
	if f.Mask == nil {
		return 1
	}
	return float64(f.Mask[z][y][x])
}
func (f Field) coord(a, i int) float64 { return (float64(i) - float64(f.Size[a])/2 + 0.5) * f.Cell[a] }
func wrap(d, length float64) float64   { return d - math.Floor(d/length+0.5)*length }
func (f Field) delta(a int, x, ref float64) float64 {
	d := x - ref
	if f.PBC[a] {
		d = wrap(d, float64(f.Size[a])*f.Cell[a])
	}
	return d
}
func (f Field) canonical(a int, x float64) float64 {
	if f.PBC[a] {
		return wrap(x, float64(f.Size[a])*f.Cell[a])
	}
	return x
}
func (f Field) neighbor(a, i, d int) (int, bool) {
	j := i + d
	if j >= 0 && j < f.Size[a] {
		return j, true
	}
	if f.PBC[a] {
		return (j + f.Size[a]) % f.Size[a], true
	}
	return 0, false
}

// PeakOffset fits a local concave maximum in float64. A plateau has no unique
// location. Reject flat/convex fits and extrapolation outside half a cell.
func PeakOffset(center, left, right float32) (float64, bool) {
	a, b, c := float64(left), float64(center), float64(right)
	curvature := a - 2*b + c
	if !finite(curvature) || curvature >= 0 {
		return 0, false
	}
	d := 0.5 * (a - c) / curvature
	return d, finite(d) && math.Abs(d) <= 0.5+1e-12
}

// Vortex retains the historical strongest-layer definition when layer=-1.
// An edge peak without the required neighbors is explicitly unlocalized.
func Vortex(f Field, layer int) Result {
	if !f.validate() || f.Size[0] < 3 || f.Size[1] < 3 || layer < -1 || layer >= f.Size[2] {
		return invalid("invalid grid or layer")
	}
	best := -1.0
	var at [3]int
	for z := 0; z < f.Size[2]; z++ {
		if layer >= 0 && z != layer {
			continue
		}
		for y := 0; y < f.Size[1]; y++ {
			for x := 0; x < f.Size[0]; x++ {
				if f.weight(x, y, z) <= 0 {
					continue
				}
				v := math.Abs(float64(f.Mz[z][y][x]))
				if !finite(v) {
					return invalid("nonfinite magnetization")
				}
				if v > best {
					best = v
					at = [3]int{x, y, z}
				}
			}
		}
	}
	if best <= 0 {
		return invalid("no localized vortex core")
	}
	var p [3]float64
	for a := 0; a < 2; a++ {
		lo, ok1 := f.neighbor(a, at[a], -1)
		hi, ok2 := f.neighbor(a, at[a], 1)
		if !ok1 || !ok2 {
			return invalid("vortex intersects nonperiodic boundary")
		}
		l, r := at, at
		l[a] = lo
		r[a] = hi
		if f.weight(l[0], l[1], l[2]) <= 0 || f.weight(r[0], r[1], r[2]) <= 0 {
			return invalid("vortex intersects geometry boundary")
		}
		d, ok := PeakOffset(float32(best), float32(math.Abs(float64(f.Mz[l[2]][l[1]][l[0]]))), float32(math.Abs(float64(f.Mz[r[2]][r[1]][r[0]]))))
		if !ok {
			return invalid("flat or unresolved vortex maximum")
		}
		p[a] = f.canonical(a, f.coord(a, at[a])+d*f.Cell[a]) + f.Shift[a]
	}
	p[2] = f.coord(2, at[2])
	return Result{Position: p, Polarization: float64(f.Mz[at[2]][at[1]][at[0]]), Valid: true}
}

type SkyrmionOptions struct {
	Polarity        int // required: +1 or -1
	Layer           int // -1: volume-weighted position across connected layers
	Background      float64
	Threshold       float64    // suppress weak background contrast; default 0.1
	Reference       [2]float64 // laboratory coordinates, used to choose one object
	SearchRadius    float64    // zero: full domain; positive: ROI around Reference
	MaxDisplacement float64    // zero: no additional association limit
	HardThreshold   bool       // legacy bubble weighting; otherwise subtract threshold
}

type component struct {
	first    [3]int
	center   [3]float64
	mass     float64
	count    int
	boundary bool
}

// Skyrmion selects a connected opposite-background core, then computes its
// weighted magnetization-contrast centroid. Other cores receive their own
// nearest-center territory, so their moments never form a fictitious center.
func Skyrmion(f Field, o SkyrmionOptions) Result {
	if !f.validate() || o.Layer < -1 || o.Layer >= f.Size[2] || (o.Polarity != 1 && o.Polarity != -1) || !finite(o.Background) || !finite(o.Threshold) || o.Threshold < 0 || o.Threshold >= 1 || !finite(o.SearchRadius) || o.SearchRadius < 0 || !finite(o.MaxDisplacement) || o.MaxDisplacement < 0 {
		return invalid("invalid skyrmion tracking parameters")
	}
	for _, v := range o.Reference {
		if !finite(v) {
			return invalid("invalid tracking reference")
		}
	}
	pol := float64(o.Polarity)
	if pol*o.Background >= 0 {
		return invalid("background must oppose core polarity")
	}
	nx, ny, nz := f.Size[0], f.Size[1], f.Size[2]
	n := nx * ny * nz
	labels := make([]int, n)
	weights := make([]float64, n)
	index := func(x, y, z int) int { return (z*ny+y)*nx + x }
	decode := func(i int) [3]int { return [3]int{i % nx, (i / nx) % ny, i / (nx * ny)} }
	ref := [2]float64{o.Reference[0] - f.Shift[0], o.Reference[1] - f.Shift[1]}
	minMz, maxMz := math.Inf(1), math.Inf(-1)
	for z := 0; z < nz; z++ {
		for y := 0; y < ny; y++ {
			for x := 0; x < nx; x++ {
				i := index(x, y, z)
				labels[i] = -1
				if o.Layer >= 0 && z != o.Layer {
					continue
				}
				g := f.weight(x, y, z)
				if !finite(g) {
					return invalid("nonfinite geometry")
				}
				if g <= 0 {
					continue
				}
				dx, dy := f.delta(0, f.coord(0, x), ref[0]), f.delta(1, f.coord(1, y), ref[1])
				if o.SearchRadius > 0 && math.Hypot(dx, dy) > o.SearchRadius {
					continue
				}
				mz := pol * float64(f.Mz[z][y][x])
				if !finite(mz) {
					return invalid("nonfinite magnetization")
				}
				minMz = math.Min(minMz, mz)
				maxMz = math.Max(maxMz, mz)
				contrast := pol * (float64(f.Mz[z][y][x]) - o.Background)
				weights[i] = math.Max(0, contrast-o.Threshold) * g
				if o.HardThreshold && contrast >= o.Threshold {
					weights[i] = contrast * g
				}
				if mz > 0 {
					labels[i] = 0
				}
			}
		}
	}
	if maxMz <= 0.1 || minMz >= -0.1 {
		return invalid("no resolved core and opposite background")
	}
	var groups []component
	queue := make([]int, 0)
	for start := 0; start < n; start++ {
		if labels[start] != 0 {
			continue
		}
		if len(groups) >= 256 {
			return invalid("too many core candidates")
		}
		id := len(groups) + 1
		first := decode(start)
		g := component{first: first}
		queue = append(queue[:0], start)
		labels[start] = id
		for head := 0; head < len(queue); head++ {
			i := queue[head]
			at := decode(i)
			w := weights[i]
			g.mass += w
			g.count++
			for a := 0; a < 3; a++ {
				g.center[a] += w * f.delta(a, f.coord(a, at[a]), f.coord(a, first[a]))
			}
			for a := 0; a < 3; a++ {
				if a == 2 && o.Layer >= 0 {
					continue
				}
				for _, d := range []int{-1, 1} {
					j, ok := f.neighbor(a, at[a], d)
					if !ok {
						if a < 2 {
							g.boundary = true
						}
						continue
					}
					next := at
					next[a] = j
					ni := index(next[0], next[1], next[2])
					if a < 2 && (f.weight(next[0], next[1], next[2]) <= 0 || weights[ni] == 0) {
						g.boundary = true
					}
					if labels[ni] == 0 {
						labels[ni] = id
						queue = append(queue, ni)
					}
				}
			}
		}
		if g.mass > 0 {
			for a := 0; a < 3; a++ {
				g.center[a] = f.canonical(a, f.coord(a, first[a])+g.center[a]/g.mass)
			}
		}
		groups = append(groups, g)
	}
	selected := -1
	distance := math.Inf(1)
	for i, g := range groups {
		if g.count < 3 || g.mass <= 0 {
			continue
		}
		d := math.Hypot(f.delta(0, g.center[0], ref[0]), f.delta(1, g.center[1], ref[1]))
		if d < distance {
			selected = i
			distance = d
		}
	}
	if selected < 0 {
		return invalid("core is unresolved on this grid")
	}
	g := groups[selected]
	if g.boundary {
		return invalid("skyrmion intersects geometry, ROI or nonperiodic boundary")
	}
	if o.MaxDisplacement > 0 && distance > o.MaxDisplacement {
		return invalid("tracked skyrmion lost")
	}
	mass := 0.0
	var moment [3]float64
	var peak float64
	for i, w := range weights {
		if w <= 0 {
			continue
		}
		at := decode(i)
		nearest := -1
		best := math.Inf(1)
		for c, other := range groups {
			if other.count < 3 || other.mass <= 0 {
				continue
			}
			d := 0.0
			axes := 2
			if o.Layer < 0 {
				axes = 3
			}
			for a := 0; a < axes; a++ {
				v := f.delta(a, f.coord(a, at[a]), other.center[a])
				d += v * v
			}
			if d < best {
				best = d
				nearest = c
			}
		}
		if nearest != selected {
			continue
		}
		mass += w
		for a := 0; a < 3; a++ {
			moment[a] += w * f.delta(a, f.coord(a, at[a]), g.center[a])
		}
		if labels[i] == selected+1 {
			peak = math.Max(peak, pol*float64(f.Mz[at[2]][at[1]][at[0]]))
		}
	}
	if mass <= 0 || !finite(mass) {
		return invalid("zero skyrmion contrast")
	}
	var position [3]float64
	for a := 0; a < 3; a++ {
		position[a] = f.canonical(a, g.center[a]+moment[a]/mass)
	}
	position[0] += f.Shift[0]
	position[1] += f.Shift[1]
	radius := math.Sqrt(float64(g.count) * f.Cell[0] * f.Cell[1] / math.Pi)
	if o.Layer < 0 {
		radius /= math.Sqrt(float64(nz))
	}
	return Result{Position: position, Polarization: pol * peak, Radius: radius, Valid: true}
}
