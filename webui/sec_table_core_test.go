package webui

import (
	"math"
	"testing"
)

func TestCorePlotValidityAndDetector(t *testing.T) {
	s := TablePlotState{CoreMode: "skyrmion", MaxPoints: 10, previewColumns: []string{"t", "ext_skyrmionposx", "ext_skyrmionposy", "ext_skyrmionvalid"}, previewData: map[string][]float64{
		"t": {0, 1, 2}, "ext_skyrmionposx": {1, math.NaN(), 3}, "ext_skyrmionposy": {4, 5, 6}, "ext_skyrmionvalid": {1, 0, 1},
	}}
	s.GetCorePosData()
	if len(s.CorePos) != 3 || s.CorePos[0][3] != 1 || s.CorePos[1][3] != 0 || s.CorePos[1][1] != 0 || s.CorePos[2][1] != 3 {
		t.Fatalf("invalid positions leaked into the UI: %v", s.CorePos)
	}
	s.MaxPoints = 1
	s.GetCorePosData()
	if len(s.CorePos) != 1 || s.CorePos[0][0] != 2 {
		t.Fatal(s.CorePos)
	}
	s.MaxPoints = -1
	s.GetCorePosData()
	if len(s.CorePos) != 0 {
		t.Fatal(s.CorePos)
	}
}

func TestLegacyCorePlotRejectsNonfiniteWithoutValidityColumn(t *testing.T) {
	s := TablePlotState{MaxPoints: 10, previewColumns: []string{"t", "ext_coreposx", "ext_coreposy"}, previewData: map[string][]float64{"t": {0}, "ext_coreposx": {math.Inf(1)}, "ext_coreposy": {2}}}
	s.GetCorePosData()
	if len(s.CorePos) != 1 || s.CorePos[0][3] != 0 || s.CoreStatus == "" {
		t.Fatal(s.CorePos, s.CoreStatus)
	}
}

func TestTableBoundsIgnoreInvalidCoreSamples(t *testing.T) {
	s := TablePlotState{Data: [][]float64{{math.NaN(), math.NaN()}, {2, 3}, {4, -1}}}
	s.GetMinMaxXY()
	if s.XMin != 2 || s.XMax != 4 || s.YMin != -1 || s.YMax != 3 {
		t.Fatal(s)
	}
	s.Data = [][]float64{{math.Inf(1), 0}}
	s.GetMinMaxXY()
	if s.XMin != 0 || s.YMax != 0 {
		t.Fatal(s)
	}
}
