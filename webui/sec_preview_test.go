package webui

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"math"
	"testing"
	"time"

	"github.com/mumax/3/data"
	"github.com/mumax/3/engine"
	"github.com/vmihailenco/msgpack/v5"
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
	if sizing.AppliedX != 51 || sizing.AppliedY != 51 || sizing.AppliedDepth != 50 || sizing.LayerStride != 10 {
		t.Fatalf("cube budget or sampled depth is wrong: %+v", sizing)
	}
	state.applyResolvedSizing(sizing)
	if !state.AutoDownscaled || state.AppliedZChosenSize != 50 {
		t.Fatalf("auto scaling metadata missing: %+v", state)
	}
}

func TestVectorSnapshotOccupancyFiniteAndFixedScale(t *testing.T) {
	cpu := data.NewSlice(3, [3]int{4, 1, 1})
	cpu.Host()[0][1] = 2
	cpu.Host()[1][2] = float32(math.Inf(1))
	cpu.Host()[2][3] = float32(math.NaN())
	occupancy := data.NewSlice(1, [3]int{4, 1, 1})
	for i := range occupancy.Host()[0] {
		occupancy.Host()[0][i] = 1
	}
	state := &PreviewState{pendingCPU: cpu, pendingOccupancy: occupancy, FixedScale: 4}
	state.processVectorSnapshot()
	if state.VectorCount != 2 || state.InvalidCount != 2 || state.VectorFieldValues[1].X != .5 || state.NormScale != 4 {
		t.Fatalf("invalid snapshot: %+v", state)
	}
	if state.VectorFieldValues[0] != (Vector3f{}) {
		t.Fatal("occupied zero vector was removed")
	}
	revision := state.TopologyRevision
	state.Layer = 1
	state.processVectorSnapshot()
	if state.TopologyRevision == revision {
		t.Fatal("layer change did not invalidate topology")
	}
}

func TestPreviewHardLimitWithoutAutoscale(t *testing.T) {
	state := &PreviewState{XChosenSize: 1000, YChosenSize: 1000, AllLayers: true, Type: "3D", MaxPoints: 8}
	sizing := state.resolvePreviewSizing(1000)
	if sizing.AppliedPoints > previewHardLimit || !sizing.AutoDownscaled {
		t.Fatalf("hard limit bypass: %+v", sizing)
	}
}

func TestWireProfileRevisionsIncludeSampling(t *testing.T) {
	state := &PreviewState{TopologyRevision: 1, Sequence: 12, VectorCount: 1}
	state.setVectorPayload([]Vector3f{{X: 1}}, []Vector3i{{X: 2}})
	full := encodePreviewFrame(state, 1)
	sampled := encodePreviewFrame(state, 2)
	if full.revision == sampled.revision {
		t.Fatal("sampling shares topology revision")
	}
	var decoded map[string]interface{}
	if err := msgpack.Unmarshal(sampled.full, &decoded); err != nil {
		t.Fatal(err)
	}
	if _, ok := decoded["vectorPositionsBinary"]; !ok {
		t.Fatal("profile keyframe is not self-contained")
	}
	if err := msgpack.Unmarshal(sampled.delta, &decoded); err != nil {
		t.Fatal(err)
	}
}

func TestVectorSnapshotNormalizesSubnormalValues(t *testing.T) {
	cpu := data.NewSlice(3, [3]int{1, 1, 1})
	cpu.Host()[0][0] = math.SmallestNonzeroFloat32
	occupancy := data.NewSlice(1, [3]int{1, 1, 1})
	occupancy.Host()[0][0] = 1
	state := &PreviewState{pendingCPU: cpu, pendingOccupancy: occupancy}
	state.processVectorSnapshot()
	if len(state.VectorFieldValues) != 1 || state.VectorFieldValues[0].X != 1 {
		t.Fatalf("normalization overflow: %v", state.VectorFieldValues)
	}
}

func TestFullGeometrySnapshotPreservesOccupiedZeroWithoutMask(t *testing.T) {
	cpu := data.NewSlice(3, [3]int{1, 1, 1})
	state := &PreviewState{pendingCPU: cpu}
	state.processVectorSnapshot()
	if state.VectorCount != 1 {
		t.Fatal("full geometry zero was discarded without occupancy buffer")
	}
}

func TestPreviewBudgetUsesAreaAveragingRatherThanSliderPresets(t *testing.T) {
	state := &PreviewState{
		XChosenSize: 500, YChosenSize: 500,
		XPossibleSizes: possiblePreviewXYSizes(500), YPossibleSizes: possiblePreviewXYSizes(500),
		MaxPoints: 131072, AutoScaleEnabled: true,
	}
	sizing := state.resolvePreviewSizing(1)
	if sizing.AppliedX != 362 || sizing.AppliedY != 362 || sizing.AppliedPoints != 131044 {
		t.Fatalf("automatic resolution wastes budget: %+v", sizing)
	}
	state.AutoScaleEnabled = false
	sizing = state.resolvePreviewSizing(1)
	if sizing.AppliedPoints != 250000 || sizing.AutoDownscaled {
		t.Fatalf("Auto-adjust off retained soft limit: %+v", sizing)
	}
}

func TestPreviewTransferLimit62500IsOptIn(t *testing.T) {
	state := &PreviewState{Type: "3D", AutoScaleEnabled: false, AppliedXChosenSize: 1000, AppliedYChosenSize: 1000}
	values := make([]Vector3f, previewHardLimit)
	positions := make([]Vector3i, previewHardLimit)
	for i := range positions {
		values[i].X = 1
		positions[i] = Vector3i{X: i % 1000, Y: i / 1000}
	}
	state.setVectorPayload(values, positions)
	connection := &managedConnection{ready: make(chan struct{}, 1)}
	connection.control([]byte(`{"protocol":2}`))
	profile, sampling := previewTransportProfile(state, connection.budget)
	if profile.VectorCount != 1000000 || sampling != 1 {
		t.Fatalf("default client secretly limits manual resolution: %d, sampling %d", profile.VectorCount, sampling)
	}
	connection.control([]byte(`{"maxPoints":131072}`))
	profile, sampling = previewTransportProfile(state, connection.budget)
	if profile.VectorCount != 62500 || sampling != 4 || profile.ServerVectorCount != 1000000 {
		t.Fatalf("explicit sampled profile metadata wrong: %d / %d, sampling %d", profile.VectorCount, profile.ServerVectorCount, sampling)
	}
	connection.control([]byte(`{"maxPoints":1000000}`))
	profile, sampling = previewTransportProfile(state, connection.budget)
	if profile.VectorCount != 1000000 || sampling != 1 || state.VectorCount != 1000000 {
		t.Fatal("raising transfer limit did not restore original grid")
	}
}

func TestPreviewClientBudgetProfiles(t *testing.T) {
	for _, budget := range []int{131072, 262144, 500000, 1000000} {
		connection := &managedConnection{ready: make(chan struct{}, 1)}
		connection.control([]byte(fmt.Sprintf(`{"protocol":2,"maxPoints":%d}`, budget)))
		if connection.budget != budget {
			t.Fatalf("negotiated %d as %d", budget, connection.budget)
		}
	}
}

func TestInitialPreviewUsesWireMetadata(t *testing.T) {
	state := &PreviewState{Type: "3D", Sequence: 12, TopologyRevision: 3, VectorCount: 1}
	state.setVectorPayload([]Vector3f{{X: 1}}, []Vector3i{{}})
	manager := newWebSocketManager()
	manager.engineState = &EngineState{Preview: state}
	connection := &managedConnection{send: make(chan outboundFrame, 1), stop: make(chan struct{})}
	manager.sendInitialState(connection, "preview")
	initial := <-connection.send
	expected := encodePreviewFrame(state, 1)
	if initial.revision != expected.revision {
		t.Fatalf("initial revision %d differs from stream %d", initial.revision, expected.revision)
	}
	var decoded PreviewState
	if err := msgpack.Unmarshal(initial.full, &decoded); err != nil {
		t.Fatal(err)
	}
	if decoded.ServerVectorCount != 1 || decoded.TransportSampling != 1 {
		t.Fatalf("initial counts/sampling missing: %d/%d", decoded.ServerVectorCount, decoded.TransportSampling)
	}
}

func TestPreviewNearHardLimitKeepsDepth(t *testing.T) {
	state := &PreviewState{XChosenSize: 64, YChosenSize: 64, ZChosenSize: 256, AllLayers: true, Type: "3D", MaxPoints: 131072, AutoScaleEnabled: false}
	sizing := state.resolvePreviewSizing(256)
	if sizing.AppliedDepth != 256 || sizing.AppliedX != 62 || sizing.AppliedY != 62 || sizing.AppliedPoints != 984064 {
		t.Fatalf("small safety reduction discarded half the layers: %+v", sizing)
	}
}

func TestVolumeOccupancySurvivesGlobalFieldAndTransferSampling(t *testing.T) {
	cpu := data.NewSlice(3, [3]int{4, 1, 1})
	occupancy := data.NewSlice(1, [3]int{4, 1, 1})
	for i := range cpu.Host()[0] {
		cpu.Host()[0][i] = 1
	}
	occupancy.Host()[0][0] = 1
	occupancy.Host()[0][3] = 1
	state := &PreviewState{Type: "3D", pendingCPU: cpu, pendingOccupancy: occupancy, AppliedXChosenSize: 4, AppliedYChosenSize: 1}
	state.processVectorSnapshot()
	if !bytes.Equal(state.VectorOccupancy, []byte{1, 0, 0, 1}) {
		t.Fatalf("global field lost geometry: %v", state.VectorOccupancy)
	}
	profile, sampling := previewTransportProfile(state, 2)
	if sampling != 2 || !bytes.Equal(profile.VectorOccupancy, []byte{1, 0}) {
		t.Fatalf("sampled occupancy mismatch: %v step %d", profile.VectorOccupancy, sampling)
	}
	state.pendingOccupancy = nil
	state.processVectorSnapshot()
	if state.VectorOccupancy != nil {
		t.Fatal("full geometry retained stale mask")
	}
}

func TestScalarVolumeSnapshotAndDimension(t *testing.T) {
	cpu := data.NewSlice(1, [3]int{2, 1, 2})
	copy(cpu.Host()[0], []float32{-2, 6, -4, 8})
	state := &PreviewState{NComp: 1, Component: "3D", pendingCPU: cpu, pendingStride: 1, AllLayers: true}
	state.updatePreviewType()
	if state.Type != "3D" {
		t.Fatal("scalar volume selected 2D")
	}
	state.processVectorSnapshot()
	if state.VectorCount != 4 || state.NormScale != 8 || state.VectorFieldValues[0] != (Vector3f{X: -.25}) || state.VectorFieldValues[3] != (Vector3f{X: 1}) {
		t.Fatalf("scalar volume lost physical data: %+v", state)
	}
	state.Component = "None"
	state.updatePreviewType()
	if state.Type != "2D" {
		t.Fatal("scalar section selected 3D")
	}
}
