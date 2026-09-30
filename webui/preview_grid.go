package webui

import "math"

// Choose a proportional target depth, then fit XY to the remaining budget.
// Optional allowed sizes constrain axes such as sampled Z to source divisors.
func resolvePreviewGrid(requested [3]int, allowed [3][]int, budget int, enabled bool) [3]int {
	result := requested
	points := float64(requested[0]) * float64(requested[1]) * float64(requested[2])
	if !enabled || points <= float64(budget) {
		return result
	}
	if requested[2] > 1 {
		axes := 0
		for _, n := range requested {
			if n > 1 {
				axes++
			}
		}
		// Rounding Z down to a divisor can discard half the layers even when
		// the requested volume only slightly exceeds the budget. Keep the next
		// supported depth and fit XY area averaging around it instead.
		targetZ := max(1, int(math.Floor(float64(requested[2])*math.Pow(float64(budget)/points, 1/float64(axes)))))
		z := min(targetZ, budget)
		if len(allowed[2]) > 0 {
			z = 1
			for _, candidate := range allowed[2] {
				if candidate > requested[2] || candidate > budget {
					break
				}
				z = candidate
				if candidate >= targetZ {
					break
				}
			}
		}
		xy := resolvePreviewGrid([3]int{requested[0], requested[1], 1}, [3][]int{allowed[0], allowed[1], nil}, max(budget/z, 1), true)
		return [3]int{xy[0], xy[1], z}
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
