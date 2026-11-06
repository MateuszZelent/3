package cuda

import (
	"testing"

	"github.com/mumax/3/data"
)

// Render window drags change the capture size on every step. Views must reuse
// storage while it is large enough, and still copy each component correctly.
func TestPreviewBuffersReuseStorageAcrossSizes(t *testing.T) {
	bindTestContext(t)
	var buffers PreviewBuffers
	defer buffers.Free()

	steps := []struct {
		size  [3]int
		grows bool
	}{
		{[3]int{5, 4, 3}, true},
		{[3]int{3, 2, 2}, false},
		{[3]int{6, 4, 3}, true},
		{[3]int{2, 2, 1}, false},
	}
	for _, step := range steps {
		size := step.size
		previous := buffers.floats
		host, gpu := buffers.View(3, size)
		if gpu.Size() != size || host.Slice.Size() != size || gpu.NComp() != 3 {
			t.Fatalf("view size %v/%v, want %v", gpu.Size(), host.Slice.Size(), size)
		}
		// The allocator may hand back the same address, so compare capacity.
		if grew := buffers.floats != previous; grew != step.grows {
			t.Fatalf("size %v: storage reallocated=%v, want %v", size, grew, step.grows)
		}

		input := data.NewSlice(3, size)
		for c := 0; c < 3; c++ {
			for i := range input.Host()[c] {
				input.Host()[c][i] = float32(c*1000 + i)
			}
		}
		data.Copy(gpu, input)
		host.Copy(gpu)
		for c := 0; c < 3; c++ {
			for i, value := range host.Slice.Host()[c] {
				if want := float32(c*1000 + i); value != want {
					t.Fatalf("size %v component %d cell %d = %v, want %v", size, c, i, value, want)
				}
			}
		}
		host.Free() // a view must not release the shared storage
	}
	if buffers.floats != 6*4*3*3 {
		t.Fatalf("storage holds %d floats, want the largest request %d", buffers.floats, 6*4*3*3)
	}
}
