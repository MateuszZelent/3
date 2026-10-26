package zarr

import (
	"os"
	"path/filepath"
	"testing"
)

func TestWriteSeriesRetiredPreservesPreviousOutput(t *testing.T) {
	dir := t.TempDir()
	metadata := filepath.Join(dir, ".zarray")
	chunk := filepath.Join(dir, "0")
	if err := os.WriteFile(metadata, []byte("old metadata"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(chunk, []byte("old chunk"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := WriteSeries(dir, []float64{1, 2}); err == nil {
		t.Fatal("retired helper must reject writes")
	}
	for filename, expected := range map[string]string{metadata: "old metadata", chunk: "old chunk"} {
		got, err := os.ReadFile(filename)
		if err != nil || string(got) != expected {
			t.Fatalf("%s changed: %q, %v", filename, got, err)
		}
	}
}
