package webui

import "math"

// Reduce all nontrivial axes proportionally. Allowed sizes divide the source
// mesh, so GPU XY averaging and Z sampling always cover the entire volume.
func resolvePreviewGrid(requested [3]int, allowed [3][]int, budget int, enabled bool) [3]int {
	result := requested
	points := float64(requested[0]) * float64(requested[1]) * float64(requested[2])
	if !enabled || points <= float64(budget) {
		return result
	}
	for points > float64(budget) {
		axes := 0
		for _, n := range result {
			if n > 1 {
				axes++
			}
		}
		if axes == 0 {
			break
		}
		scale := math.Pow(float64(budget)/points, 1/float64(axes))
		for axis, n := range result {
			target := max(1, int(math.Floor(float64(n)*scale)))
			result[axis] = target
			if len(allowed[axis]) > 0 {
				result[axis] = 1
				for _, candidate := range allowed[axis] {
					if candidate > target {
						break
					}
					result[axis] = candidate
				}
			}
		}
		// Thin axes can hit one before the others; recompute with the remaining axes.
		points = float64(result[0]) * float64(result[1]) * float64(result[2])
	}
	return result
}
