package cuda

import (
	"math"
	"testing"

	"github.com/mumax/3/cuda/cu"
	"github.com/mumax/3/data"
)

// A non-cubic signed field distinguishes every axis and every boundary layer.
// The reference integrates intersections with source cells instead of following
// the GPU's loop bounds. Opposite layers must cancel in the arithmetic mean.
func TestPreviewPlanesSlicesAndMean(t *testing.T) {
	bindTestContext(t)
	major, minor := cu.Device(0).ComputeCapability()
	previousCC := cudaCC
	cudaCC = major*10 + minor
	defer func() { cudaCC = previousCC }()
	size := [3]int{7, 5, 3}
	input := data.NewSlice(1, size)
	for z := 0; z < size[2]; z++ {
		for y := 0; y < size[1]; y++ {
			for x := 0; x < size[0]; x++ {
				input.Host()[0][(z*size[1]+y)*size[0]+x] = float32((x-3)*100 + (y-2)*10 + z - 1)
			}
		}
	}
	gpu := NewSlice(1, size)
	defer gpu.Free()
	data.Copy(gpu, input)
	for _, axes := range [][2]int{{0, 1}, {1, 2}, {0, 2}} {
		u, v := axes[0], axes[1]
		n := 3 - u - v
		for _, outSize := range [][3]int{{3, 2, 1}, {size[u], size[v], 1}} {
			for _, layer := range []int{0, size[n] - 1, -1} {
				out := NewSlice(1, outSize)
				host := NewPreviewHostBuffer(1, outSize)
				ResizePreviewPlane(out, gpu, u, v, layer)
				host.Copy(out)
				out.Free()
				for j := 0; j < outSize[1]; j++ {
					for i := 0; i < outSize[0]; i++ {
						lo := [2]float64{float64(i*size[u]) / float64(outSize[0]), float64(j*size[v]) / float64(outSize[1])}
						hi := [2]float64{float64((i+1)*size[u]) / float64(outSize[0]), float64((j+1)*size[v]) / float64(outSize[1])}
						sum, weight := 0.0, 0.0
						for z := 0; z < size[2]; z++ {
							for y := 0; y < size[1]; y++ {
								for x := 0; x < size[0]; x++ {
									coords := [3]int{x, y, z}
									if layer >= 0 && coords[n] != layer {
										continue
									}
									coverage := 1.0
									for axis, sourceAxis := range axes {
										coverage *= math.Max(0, math.Min(hi[axis], float64(coords[sourceAxis]+1))-math.Max(lo[axis], float64(coords[sourceAxis])))
									}
									sum += coverage * float64(input.Host()[0][(z*size[1]+y)*size[0]+x])
									weight += coverage
								}
							}
						}
						want := sum / weight
						got := float64(host.Slice.Host()[0][j*outSize[0]+i])
						if math.Abs(got-want) > 1e-4 {
							t.Fatalf("axes=%v layer=%d size=%v pixel=%d,%d got=%g want=%g", axes, layer, outSize, i, j, got, want)
						}
					}
				}
				host.Free()
			}
		}
	}
}
