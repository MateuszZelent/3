package mag

import (
	"math"

	"github.com/mumax/3/data"
	"github.com/mumax/3/util"
)

// OerstedKernel returns the padded real-space Biot-Savart kernel K(r) =
// mu0/(4*pi) * dV * r/|r|^3. Periodic boundaries are intentionally rejected;
// the source Amumax implementation does not define them either.
func OerstedKernel(gridSize [3]int, cellSize [3]float64, pbc [3]int) [3]*data.Slice {
	for axis := 0; axis < 3; axis++ {
		if pbc[axis] != 0 {
			panic("Oersted field does not support periodic boundary conditions")
		}
	}
	size := padSize(gridSize, pbc)
	util.Argument(size[X] > 0 && size[Y] > 0 && size[Z] > 0)
	util.Argument(cellSize[X] > 0 && cellSize[Y] > 0 && cellSize[Z] > 0)
	var kernel [3]*data.Slice
	for component := 0; component < 3; component++ {
		kernel[component] = data.NewSlice(1, size)
	}
	kx, ky, kz := kernel[X].Scalars(), kernel[Y].Scalars(), kernel[Z].Scalars()
	r1, r2 := kernelRanges(size, pbc)
	prefactor := 1e-7 * cellSize[X] * cellSize[Y] * cellSize[Z]
	for iz := r1[Z]; iz <= r2[Z]; iz++ {
		zw, rz := wrap(iz, size[Z]), float64(iz)*cellSize[Z]
		for iy := r1[Y]; iy <= r2[Y]; iy++ {
			yw, ry := wrap(iy, size[Y]), float64(iy)*cellSize[Y]
			for ix := r1[X]; ix <= r2[X]; ix++ {
				xw, rx := wrap(ix, size[X]), float64(ix)*cellSize[X]
				r2value := rx*rx + ry*ry + rz*rz
				if r2value == 0 {
					continue
				}
				scale := prefactor / (math.Sqrt(r2value) * r2value)
				kx[zw][yw][xw] += float32(scale * rx)
				ky[zw][yw][xw] += float32(scale * ry)
				kz[zw][yw][xw] += float32(scale * rz)
			}
		}
	}
	return kernel
}
