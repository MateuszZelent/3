package zarr

import (
	"encoding/json"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"testing"

	"github.com/mumax/3/data"
)

func committedTestSlice() *data.Slice {
	src := data.NewSlice(1, [3]int{2, 1, 1})
	src.Set(0, 0, 0, 0, 1)
	src.Set(0, 1, 0, 0, 2)
	return src
}

func frameRecordPath(dir string, step int) string {
	return filepath.Join(dir, ".frames", fmt.Sprintf("%020d.json", step))
}

func committedChunkPath(dir string, step int) string {
	return filepath.Join(dir, fmt.Sprintf("%d.0.0.0.0", step))
}

func frameTimestamp(t *testing.T, filename string) float64 {
	t.Helper()
	b, err := os.ReadFile(filename)
	if err != nil {
		t.Fatal(err)
	}
	var record map[string]json.RawMessage
	if err := json.Unmarshal(b, &record); err != nil {
		t.Fatalf("decode %s: %v", filename, err)
	}
	// The record name is deliberately accepted through the public semantic
	// field used by the implementation (time/timestamp/t), while the test
	// requires that one real timestamp is present and numeric.
	for _, key := range []string{"time", "timestamp", "t"} {
		raw, ok := record[key]
		if !ok {
			continue
		}
		var value float64
		if err := json.Unmarshal(raw, &value); err != nil {
			t.Fatalf("record %s.%s is not numeric: %v", filename, key, err)
		}
		return value
	}
	t.Fatalf("record %s has no time/timestamp field: %s", filename, b)
	return 0
}

func TestCommittedFramesPreserveTimesAndProjection(t *testing.T) {
	src := committedTestSlice()
	defer src.Free()
	dir := filepath.Join(t.TempDir(), "run.zarr")
	chunks := [4]int{1, 1, 1, 1}
	const firstTime = 0.125
	const secondTime = 0.375

	if err := WriteCommittedStep(dir, 0, src, chunks, firstTime, "m", "A/m"); err != nil {
		t.Fatal(err)
	}
	if err := WriteCommittedStep(dir, 1, src, chunks, secondTime, "m", "A/m"); err != nil {
		t.Fatal(err)
	}

	commit, err := ReadCommit(dir)
	if err != nil {
		t.Fatal(err)
	}
	if commit.CommittedSteps != 2 {
		t.Fatalf("committed steps = %d, want 2", commit.CommittedSteps)
	}
	if commit.Metadata.Shape[0] != 2 {
		t.Fatalf("committed shape = %v, want two steps", commit.Metadata.Shape)
	}
	if commit.LastRecordSHA256 == "" {
		t.Fatal("commit has no last-record checksum")
	}
	if got := frameTimestamp(t, frameRecordPath(dir, 0)); got != firstTime {
		t.Fatalf("first frame time = %.17g, want %.17g", got, firstTime)
	}
	if got := frameTimestamp(t, frameRecordPath(dir, 1)); got != secondTime {
		t.Fatalf("second frame time = %.17g, want %.17g", got, secondTime)
	}

	if err := PublishProjection(dir); err != nil {
		t.Fatal(err)
	}
	metadata, err := ReadMetadata(dir)
	if err != nil {
		t.Fatal(err)
	}
	if metadata.Shape[0] != 2 {
		t.Fatalf("projected shape = %v, want two steps", metadata.Shape)
	}
	attrsBytes, err := os.ReadFile(filepath.Join(dir, ".zattrs"))
	if err != nil {
		t.Fatal(err)
	}
	var attrs Attributes
	if err := json.Unmarshal(attrsBytes, &attrs); err != nil {
		t.Fatal(err)
	}
	if len(attrs.Times) != 2 || attrs.Times[0] != firstTime || attrs.Times[1] != secondTime {
		t.Fatalf("projected times = %#v, want [%g %g]", attrs.Times, firstTime, secondTime)
	}
	if attrs.Name != "m" || attrs.Unit != "A/m" {
		t.Fatalf("projected attributes = %#v", attrs)
	}
}

func TestFailedChunkDoesNotAdvanceCommittedFrame(t *testing.T) {
	src := committedTestSlice()
	defer src.Free()
	dir := filepath.Join(t.TempDir(), "run.zarr")
	chunks := [4]int{1, 1, 1, 1}
	if err := WriteCommittedStep(dir, 0, src, chunks, 0.25, "m", "A/m"); err != nil {
		t.Fatal(err)
	}

	// Occupying the first step-1 chunk with a directory forces the atomic
	// publication of that chunk to fail. The writer must leave the step-0
	// commit and frame record intact.
	if err := os.Mkdir(committedChunkPath(dir, 1), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := WriteCommittedStep(dir, 1, src, chunks, 0.5, "m", "A/m"); err == nil {
		t.Fatal("WriteCommittedStep succeeded despite a chunk publication failure")
	}

	commit, err := ReadCommit(dir)
	if err != nil {
		t.Fatal(err)
	}
	if commit.CommittedSteps != 1 || commit.Metadata.Shape[0] != 1 {
		t.Fatalf("failed write advanced commit: steps=%d shape=%v", commit.CommittedSteps, commit.Metadata.Shape)
	}
	if _, err := os.Stat(frameRecordPath(dir, 1)); !os.IsNotExist(err) {
		t.Fatalf("failed frame record was published: %v", err)
	}
	if err := PublishProjection(dir); err != nil {
		t.Fatal(err)
	}
	metadata, err := ReadMetadata(dir)
	if err != nil {
		t.Fatal(err)
	}
	if metadata.Shape[0] != 1 {
		t.Fatalf("projection advanced after failed write: %v", metadata.Shape)
	}
	attrsBytes, err := os.ReadFile(filepath.Join(dir, ".zattrs"))
	if err != nil {
		t.Fatal(err)
	}
	var attrs Attributes
	if err := json.Unmarshal(attrsBytes, &attrs); err != nil {
		t.Fatal(err)
	}
	if len(attrs.Times) != 1 || math.Abs(attrs.Times[0]-0.25) > 0 {
		t.Fatalf("projection contains failed frame time: %#v", attrs.Times)
	}
}
