package engine

import (
	"image"
	_ "image/gif"
	_ "image/jpeg"
	_ "image/png"
	"os"
	"path/filepath"
	"testing"

	"github.com/mumax/3/data"
)

func TestSnapshotSyncProducesDecodableImages(t *testing.T) {
	field := data.NewSlice(3, [3]int{8, 4, 1})
	defer field.Free()
	for i := range field.Host()[0] {
		field.Host()[0][i] = 1
	}
	for _, format := range []string{"jpg", "png", "gif"} {
		t.Run(format, func(t *testing.T) {
			name := filepath.Join(t.TempDir(), "snapshot."+format)
			snapshot_sync(name, field)
			f, err := os.Open(name)
			if err != nil {
				t.Fatal(err)
			}
			defer f.Close()
			img, _, err := image.Decode(f)
			if err != nil {
				t.Fatal(err)
			}
			if img.Bounds().Dx() != 8 || img.Bounds().Dy() != 4 {
				t.Fatalf("unexpected dimensions: %v", img.Bounds())
			}
		})
	}
}

func TestSnapshotSyncReportsUnsupportedImageFormat(t *testing.T) {
	field := data.NewSlice(3, [3]int{8, 4, 1})
	defer field.Free()
	defer func() {
		if recover() == nil {
			t.Error("unsupported snapshot format was silently accepted")
		}
	}()
	snapshot_sync(filepath.Join(t.TempDir(), "snapshot.invalid"), field)
}
