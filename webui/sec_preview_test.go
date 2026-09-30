package webui

import (
	"encoding/binary"
	"math"
	"testing"
	"time"

	"github.com/mumax/3/engine"
)

func TestAddPossibleDownscaleSizesDoesNotWaitForMesh(t *testing.T) {
	if engine.MeshReady() {
		t.Skip("the engine mesh is already initialized")
	}

	state := &PreviewState{}
	done := make(chan bool, 1)
	go func() {
		done <- state.addPossibleDownscaleSizes()
	}()

	select {
	case initialized := <-done:
		if initialized {
			t.Fatal("preview sizes initialized without an engine mesh")
		}
	case <-time.After(250 * time.Millisecond):
		t.Fatal("preview size initialization waited for a mesh")
	}
}

func TestSetVectorPayloadPacksCompleteBinaryFrame(t *testing.T) {
	state := &PreviewState{}
	values := []Vector3f{{X: 1.5, Y: -2.25, Z: 3}}
	positions := []Vector3i{{X: 4, Y: -5, Z: 6}}

	state.setVectorPayload(values, positions)

	if state.VectorCount != 1 || state.TopologyRevision != 1 {
		t.Fatalf("unexpected vector metadata: count=%d revision=%d", state.VectorCount, state.TopologyRevision)
	}
	if len(state.VectorValuesBinary) != 12 || len(state.VectorPositionsBinary) != 12 {
		t.Fatalf("unexpected packed sizes: values=%d positions=%d", len(state.VectorValuesBinary), len(state.VectorPositionsBinary))
	}
	gotX := math.Float32frombits(binary.LittleEndian.Uint32(state.VectorValuesBinary[0:4]))
	gotY := math.Float32frombits(binary.LittleEndian.Uint32(state.VectorValuesBinary[4:8]))
	gotZ := math.Float32frombits(binary.LittleEndian.Uint32(state.VectorValuesBinary[8:12]))
	if gotX != 1.5 || gotY != -2.25 || gotZ != 3 {
		t.Fatalf("unexpected packed vector: (%g, %g, %g)", gotX, gotY, gotZ)
	}
	if got := int32(binary.LittleEndian.Uint32(state.VectorPositionsBinary[4:8])); got != -5 {
		t.Fatalf("unexpected packed Y position: %d", got)
	}

	// Values may change without a topology revision, but each frame still carries
	// positions so it remains valid if stale websocket frames are discarded.
	state.setVectorPayload([]Vector3f{{X: 9}}, positions)
	if state.TopologyRevision != 1 {
		t.Fatalf("value-only update changed topology revision to %d", state.TopologyRevision)
	}
	if len(state.VectorPositionsBinary) != 12 {
		t.Fatalf("value-only frame omitted positions: %d bytes", len(state.VectorPositionsBinary))
	}
}

func TestPreviewSizingManualZ(t *testing.T) {
	state := &PreviewState{
		XChosenSize: 100, YChosenSize: 100, ZChosenSize: 10,
		XPossibleSizes: possibleDownscaleSizes(500), YPossibleSizes: possibleDownscaleSizes(500),
		AllLayers: true, Type: "3D", MaxPoints: 131072, AutoScaleEnabled: true,
	}
	sizing := state.resolvePreviewSizing(500)
	if sizing.AppliedDepth != 10 || sizing.LayerStride != 50 || sizing.AutoDownscaled {
		t.Fatalf("manual Z sampling ignored or mislabeled: %+v", sizing)
	}
	state.applyResolvedSizing(sizing)
	if state.AppliedZChosenSize != 10 || state.AutoDownscaleMessage != "" {
		t.Fatalf("wrong UI metadata: %+v", state)
	}
	state.AllLayers = false
	sizing = state.resolvePreviewSizing(1)
	if sizing.AppliedDepth != 1 || sizing.LayerStride != 1 {
		t.Fatalf("Z control changed single-layer mode: %+v", sizing)
	}
	state.AllLayers = true
	state.ZChosenSize = 500
	state.XChosenSize, state.YChosenSize = 500, 500
	sizing = state.resolvePreviewSizing(500)
	if sizing.AppliedX != 50 || sizing.AppliedY != 50 || sizing.AppliedDepth != 50 || sizing.LayerStride != 10 {
		t.Fatalf("cube is not sampled equally: %+v", sizing)
	}
	state.applyResolvedSizing(sizing)
	if !state.AutoDownscaled || state.AppliedZChosenSize != 50 {
		t.Fatalf("auto scaling metadata missing: %+v", state)
	}
}
