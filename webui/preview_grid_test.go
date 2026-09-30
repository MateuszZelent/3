package webui

import "testing"

func TestPreviewGridBudget(t *testing.T) {
	for _, test := range []struct {
		name   string
		size   [3]int
		budget int
		want   [3]int
	}{
		{"cube250", [3]int{250, 250, 250}, 131072, [3]int{50, 50, 50}},
		{"cube500", [3]int{500, 500, 500}, 131072, [3]int{50, 50, 50}},
		{"single layer", [3]int{500, 500, 1}, 131072, [3]int{250, 250, 1}},
		{"manual depth", [3]int{100, 100, 10}, 131072, [3]int{100, 100, 10}},
	} {
		t.Run(test.name, func(t *testing.T) {
			var allowed [3][]int
			for axis, n := range test.size {
				for d := 1; d <= n; d++ {
					if n%d == 0 {
						allowed[axis] = append(allowed[axis], d)
					}
				}
			}
			got := resolvePreviewGrid(test.size, allowed, test.budget, true)
			if got != test.want {
				t.Fatalf("got %v, want %v", got, test.want)
			}
			if unscaled := resolvePreviewGrid(test.size, allowed, test.budget, false); unscaled != test.size {
				t.Fatalf("manual settings changed: %v", unscaled)
			}
		})
	}
	// Once thin axes reach one, the long axis still must fit the budget.
	got := resolvePreviewGrid([3]int{1000000, 2, 2}, [3][]int{}, 8, true)
	if got[0]*got[1]*got[2] > 8 {
		t.Fatalf("exceeded budget: %v", got)
	}
}
