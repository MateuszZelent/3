package webui

import "testing"

func TestRegionResolutionAndBounds(t *testing.T) {
	mesh := [3]int{4096, 8, 4}
	r := PreviewRegion{Enabled: true, Start: [3]int{128, 1, 1}, End: [3]int{160, 7, 4}, Mode: "native", Samples: mesh, MaxPoints: 65536}
	if err := r.validate(mesh); err != nil {
		t.Fatal(err)
	}
	if got := r.requested(); got != [3]int{32, 6, 3} {
		t.Fatal(got)
	}
	r.Mode = "custom"
	r.Samples = [3]int{16, 8, 1}
	if got := r.requested(); got != [3]int{16, 6, 1} {
		t.Fatal(got)
	}
	bad := r
	bad.End[0] = 4097
	if bad.validate(mesh) == nil {
		t.Fatal("out of bounds accepted")
	}
	bad = r
	bad.Start[1] = bad.End[1]
	if bad.validate(mesh) == nil {
		t.Fatal("empty range accepted")
	}
	bad = r
	bad.MaxPoints = previewHardLimit + 1
	if bad.validate(mesh) == nil {
		t.Fatal("unbounded payload accepted")
	}
	r.clamp([3]int{64, 4, 2})
	if r.Start != [3]int{32, 0, 0} || r.End != [3]int{64, 4, 2} {
		t.Fatal(r)
	}
	large := [3]int{4096, 256, 4}
	applied := resolvePreviewGrid(large, [3][]int{}, previewHardLimit, true)
	if applied[0]*applied[1]*applied[2] > previewHardLimit {
		t.Fatal(applied)
	}
	for a := range applied {
		if applied[a] < 1 || applied[a] > large[a] {
			t.Fatal(applied)
		}
	}
}
