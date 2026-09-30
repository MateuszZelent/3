package webui

import "testing"

func TestPlaneAxesAndMeshChange(t *testing.T) {
	s := PreviewState{Plane: "yz", previewMeshSize: [3]int{7, 5, 3}, planeChosenSize: [3]int{7, 5, 3}, sliceIndices: [3]int{6, 4, 2}}
	s.refreshPlaneMetadata()
	if s.SliceIndex != 6 || s.PlaneUChosenSize != 5 || s.PlaneVChosenSize != 3 {
		t.Fatalf("YZ metadata incorrect: %+v", s)
	}
	s.Plane = "xz"
	s.refreshPlaneMetadata()
	if s.SliceIndex != 4 || s.PlaneUChosenSize != 7 || s.PlaneVChosenSize != 3 {
		t.Fatal("XZ metadata incorrect")
	}
	s.Plane = "xy"
	s.refreshPlaneMetadata()
	if s.SliceIndex != 2 {
		t.Fatal("XY lost its last slice")
	}
	s.Plane = "yz"
	s.previewMeshSize[0] = 2
	s.refreshPlaneMetadata()
	if s.SliceIndex != 1 {
		t.Fatal("slice was not clamped after shrinking the mesh")
	}
}

func TestPlaneBudgetKeepsVolumeSettings(t *testing.T) {
	s := PreviewState{Plane: "xz", XChosenSize: 100, YChosenSize: 50, ZChosenSize: 20, PlaneUChosenSize: 1000, PlaneVChosenSize: 1000, MaxPoints: 131072, AutoScaleEnabled: true}
	sizing := s.resolvePlaneSizing()
	if !sizing.AutoDownscaled || sizing.AppliedPoints > 131072 || sizing.AppliedDepth != 1 {
		t.Fatalf("wrong plane budget: %+v", sizing)
	}
	if s.XChosenSize != 100 || s.YChosenSize != 50 || s.ZChosenSize != 20 {
		t.Fatal("plane resolution changed volume settings")
	}
	s.AutoScaleEnabled = false
	sizing = s.resolvePlaneSizing()
	if sizing.AppliedX != 1000 || sizing.AppliedY != 1000 || sizing.AutoDownscaled {
		t.Fatal("manual resolution retained soft budget")
	}
}
