package webui

import (
	"encoding/binary"
	"math"
	"testing"
)

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
