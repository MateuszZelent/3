package cuda

import (
	"math"
	"testing"

	"github.com/mumax/3/cuda/cu"
	"github.com/mumax/3/data"
)

func TestPreviewRegionNativeAndWeightedAverage(t *testing.T) {
	bindTestContext(t)
	major, minor := cu.Device(0).ComputeCapability()
	previous := cudaCC
	cudaCC = major*10 + minor
	defer func() { cudaCC = previous }()
	size := [3]int{11, 7, 5}
	origin := [3]int{3, 2, 1}
	region := [3]int{7, 4, 3}
	input := data.NewSlice(3, size)
	for c := 0; c < 3; c++ {
		for z := 0; z < size[2]; z++ {
			for y := 0; y < size[1]; y++ {
				for x := 0; x < size[0]; x++ {
					input.Host()[c][(z*size[1]+y)*size[0]+x] = float32((c+1)*1000 + (x-5)*100 + (y-3)*10 + z - 2)
				}
			}
		}
	}
	gpu := NewSlice(3, size)
	defer gpu.Free()
	data.Copy(gpu, input)
	for _, outSize := range [][3]int{region, {3, 3, 2}, {1, 1, 1}} {
		out := NewSlice(3, outSize)
		host := NewPreviewHostBuffer(3, outSize)
		ResizePreviewRegion(out, gpu, origin, region)
		host.Copy(out)
		out.Free()
		for c := 0; c < 3; c++ {
			for iz := 0; iz < outSize[2]; iz++ {
				for iy := 0; iy < outSize[1]; iy++ {
					for ix := 0; ix < outSize[0]; ix++ {
						coords := [3]int{ix, iy, iz}
						var lo, hi [3]float64
						for a := range lo {
							lo[a] = float64(origin[a]) + float64(coords[a]*region[a])/float64(outSize[a])
							hi[a] = float64(origin[a]) + float64((coords[a]+1)*region[a])/float64(outSize[a])
						}
						sum, weight := 0., 0.
						for z := 0; z < size[2]; z++ {
							for y := 0; y < size[1]; y++ {
								for x := 0; x < size[0]; x++ {
									at := [3]int{x, y, z}
									w := 1.
									for a := range at {
										w *= math.Max(0, math.Min(hi[a], float64(at[a]+1))-math.Max(lo[a], float64(at[a])))
									}
									sum += w * float64(input.Host()[c][(z*size[1]+y)*size[0]+x])
									weight += w
								}
							}
						}
						got := float64(host.Slice.Host()[c][(iz*outSize[1]+iy)*outSize[0]+ix])
						want := sum / weight
						if math.Abs(got-want) > 0.001 {
							t.Fatalf("%v component%d at%v got%g want%g", outSize, c, coords, got, want)
						}
					}
				}
			}
		}
		host.Free()
	}
}
