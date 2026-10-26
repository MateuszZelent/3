package zarr

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	"github.com/mumax/3/data"
)

func testSlice() *data.Slice {
	src := data.NewSlice(1, [3]int{2, 1, 1})
	src.Set(0, 0, 0, 0, 1)
	src.Set(0, 1, 0, 0, 2)
	return src
}

func TestWriteStepReturnsAfterWorkerIOFailure(t *testing.T) {
	// With one worker and two chunks, the first failed job leaves the producer
	// trying to send the second job to a worker that has already returned. This
	// used to deadlock WriteStep indefinitely.
	previous := runtime.GOMAXPROCS(1)
	defer runtime.GOMAXPROCS(previous)

	src := testSlice()
	defer src.Free()
	dir := filepath.Join(t.TempDir(), "m")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	// A directory at the first chunk key makes the atomic rename fail while
	// leaving a second job for the producer to attempt to enqueue.
	if err := os.Mkdir(filepath.Join(dir, "0.0.0.0.0"), 0o755); err != nil {
		t.Fatal(err)
	}

	done := make(chan error, 1)
	go func() { done <- WriteStep(dir, 0, src, [4]int{1, 1, 1, 1}) }()
	select {
	case err := <-done:
		if err == nil {
			t.Fatal("WriteStep succeeded despite an atomic chunk-write failure")
		}
	case <-time.After(750 * time.Millisecond):
		t.Fatal("WriteStep deadlocked after a worker returned an I/O error")
	}
}

func TestWriteStepRejectsInvalidMetadataAndNoncontiguousAppend(t *testing.T) {
	t.Run("invalid existing metadata", func(t *testing.T) {
		src := testSlice()
		defer src.Free()
		dir := filepath.Join(t.TempDir(), "m")
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
		original := []byte("{\n")
		if err := os.WriteFile(filepath.Join(dir, ".zarray"), original, 0o644); err != nil {
			t.Fatal(err)
		}

		err := WriteStep(dir, 0, src, [4]int{1, 1, 1, 1})
		if err == nil {
			t.Fatal("WriteStep accepted malformed existing .zarray")
		}
		got, readErr := os.ReadFile(filepath.Join(dir, ".zarray"))
		if readErr != nil {
			t.Fatal(readErr)
		}
		if string(got) != string(original) {
			t.Fatalf("invalid .zarray was replaced: got %q", got)
		}
	})

	t.Run("gap", func(t *testing.T) {
		src := testSlice()
		defer src.Free()
		dir := filepath.Join(t.TempDir(), "m")
		if err := WriteStep(dir, 0, src, [4]int{1, 1, 1, 1}); err != nil {
			t.Fatal(err)
		}
		if err := WriteStep(dir, 2, src, [4]int{1, 1, 1, 1}); err == nil {
			t.Fatal("WriteStep accepted a step with a gap after step 0")
		}
		metadata, err := ReadMetadata(dir)
		if err != nil {
			t.Fatal(err)
		}
		if metadata.Shape[0] != 1 {
			t.Fatalf("shape advanced after rejected gap: %v", metadata.Shape)
		}
		if _, err := os.Stat(filepath.Join(dir, "2.0.0.0.0")); !os.IsNotExist(err) {
			t.Fatalf("gap chunk was published: %v", err)
		}
	})

	t.Run("backwards", func(t *testing.T) {
		src := testSlice()
		defer src.Free()
		dir := filepath.Join(t.TempDir(), "m")
		if err := WriteStep(dir, 0, src, [4]int{1, 1, 1, 1}); err != nil {
			t.Fatal(err)
		}
		if err := WriteStep(dir, 1, src, [4]int{1, 1, 1, 1}); err != nil {
			t.Fatal(err)
		}
		if err := WriteStep(dir, 1, src, [4]int{1, 1, 1, 1}); err == nil {
			t.Fatal("WriteStep accepted a step at or before the committed end")
		}
		metadata, err := ReadMetadata(dir)
		if err != nil {
			t.Fatal(err)
		}
		if metadata.Shape[0] != 2 {
			t.Fatalf("shape changed after rejected backwards append: %v", metadata.Shape)
		}
	})
}
