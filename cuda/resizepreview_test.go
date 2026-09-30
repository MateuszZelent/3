package cuda

import (
	"github.com/mumax/3/cuda/cu"
	"github.com/mumax/3/data"
	"math"
	"testing"
)

// Compare GPU area coverage and signed projection against an independent CPU reference.
func TestResizePreviewPrimeVolume(t *testing.T) {
	bindTestContext(t)
	major, minor := cu.Device(0).ComputeCapability()
	previousCC := cudaCC
	cudaCC = major*10 + minor
	defer func() { cudaCC = previousCC }()
	size := [3]int{7, 5, 6}
	input := data.NewSlice(1, size)
	for z := 0; z < size[2]; z++ {
		for y := 0; y < size[1]; y++ {
			for x := 0; x < size[0]; x++ {
				input.Host()[0][(z*size[1]+y)*size[0]+x] = float32((x+2*y+1)*(z+1)) * float32(1-2*(z%2))
			}
		}
	}
	gpu := NewSlice(1, size)
	defer gpu.Free()
	data.Copy(gpu, input)
	for _, projection := range []bool{false, true} {
		outSize := [3]int{3, 2, 3}
		layer := -1
		if projection {
			outSize[2] = 1
			layer = -2
		}
		output := NewSlice(1, outSize)
		staging := NewPreviewHostBuffer(1, outSize)
		host := staging.Slice
		ResizePreview(output, gpu, layer, 2)
		staging.Copy(output)
		output.Free()
		for z := 0; z < outSize[2]; z++ {
			for y := 0; y < outSize[1]; y++ {
				for x := 0; x < outSize[0]; x++ {
					reference := func(sourceZ int) float64 {
						x0, x1 := float64(x*size[0])/3, float64((x+1)*size[0])/3
						y0, y1 := float64(y*size[1])/2, float64((y+1)*size[1])/2
						sum, weight := 0.0, 0.0
						for sy := 0; sy < size[1]; sy++ {
							for sx := 0; sx < size[0]; sx++ {
								w := math.Max(0, math.Min(x1, float64(sx+1))-math.Max(x0, float64(sx))) * math.Max(0, math.Min(y1, float64(sy+1))-math.Max(y0, float64(sy)))
								sum += w * float64(input.Host()[0][(sourceZ*size[1]+sy)*size[0]+sx])
								weight += w
							}
						}
						return sum / weight
					}
					want := reference(z*2 + 1)
					if projection {
						want = reference(0)
						for sz := 1; sz < size[2]; sz++ {
							v := reference(sz)
							if math.Abs(v) > math.Abs(want) {
								want = v
							}
						}
					}
					got := float64(host.Host()[0][(z*outSize[1]+y)*outSize[0]+x])
					if math.Abs(got-want) > 1e-4 {
						t.Fatalf("projection=%v cell=%d,%d,%d got=%g want=%g", projection, x, y, z, got, want)
					}
				}
			}
		}
		staging.Free()
	}
}
