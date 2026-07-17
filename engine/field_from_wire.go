package engine

import (
	"math"

	"github.com/mumax/3/cuda"
	"github.com/mumax/3/data"
	"github.com/mumax/3/mag"
)

func init() {
	DeclFunc("fieldFromWireVectorMask", FieldFromWireVectorMask, "Magnetic field mask of a rectangular wire parallel to y")
	DeclFunc("cpwVectorMask", CPWVectorMask, "CPU magnetic field mask of a three-conductor coplanar waveguide")
	DeclFunc("cpwVectorMaskGPU", CPWVectorMaskGPU, "GPU magnetic field mask of a three-conductor coplanar waveguide")
}

func fieldFromWire(current, x, z, halfWidth, halfHeight float64) (float64, float64, float64) {
	if halfWidth <= 0 || halfHeight <= 0 {
		panic(UserErr("fieldFromWire: width and height must be positive"))
	}
	const epsilon = 1e-12
	ax, bx := halfWidth-x, -halfWidth-x
	bz, nbz := halfHeight-z, -halfHeight-z
	if math.Abs(ax) < epsilon {
		ax = math.Copysign(epsilon, ax)
	}
	if math.Abs(bx) < epsilon {
		bx = math.Copysign(epsilon, bx)
	}
	if math.Abs(bz) < epsilon {
		bz = math.Copysign(epsilon, bz)
	}
	if math.Abs(nbz) < epsilon {
		nbz = math.Copysign(epsilon, nbz)
	}

	t1 := (halfWidth - x) * (0.5*math.Log((bz*bz+ax*ax)/(nbz*nbz+ax*ax)) + (bz/ax)*math.Atan(ax/bz) - (nbz/ax)*math.Atan(ax/nbz))
	t2 := -(-halfWidth - x) * (0.5*math.Log((bz*bz+bx*bx)/(bx*bx+nbz*nbz)) + (bz/bx)*math.Atan(bx/bz) - (nbz/bx)*math.Atan(bx/nbz))
	bxField := current * mag.Mu0 / (8 * math.Pi * halfWidth * halfHeight) * (t1 + t2)
	t3 := bz * (0.5*math.Log((bz*bz+ax*ax)/(bz*bz+bx*bx)) + (ax/bz)*math.Atan(bz/ax) - (bx/bz)*math.Atan(bz/bx))
	t4 := -nbz * (0.5*math.Log((nbz*nbz+ax*ax)/(nbz*nbz+bx*bx)) + (ax/nbz)*math.Atan(nbz/ax) - (bx/nbz)*math.Atan(nbz/bx))
	bzField := -current * mag.Mu0 / (8 * math.Pi * halfWidth * halfHeight) * (t3 + t4)
	return bxField, 0, bzField
}

func FieldFromWireVectorMask(current, width, height, xCenter, zCenter float64) *data.Slice {
	size := Mesh().Size()
	mask := data.NewSlice(3, size)
	for z := 0; z < size[Z]; z++ {
		for y := 0; y < size[Y]; y++ {
			for x := 0; x < size[X]; x++ {
				position := Index2Coord(x, y, z)
				bx, by, bz := fieldFromWire(current, position[X]-xCenter, position[Z]-zCenter, width/2, height/2)
				mask.Set(0, x, y, z, bx)
				mask.Set(1, x, y, z, by)
				mask.Set(2, x, y, z, bz)
			}
		}
	}
	return mask
}

func CPWVectorMask(current, width, height, distance, xOffset, zOffset float64) *data.Slice {
	size := Mesh().Size()
	mask := data.NewSlice(3, size)
	for z := 0; z < size[Z]; z++ {
		for y := 0; y < size[Y]; y++ {
			for x := 0; x < size[X]; x++ {
				position := Index2Coord(x, y, z)
				bx1, by1, bz1 := fieldFromWire(-current, position[X]-distance-xOffset, position[Z]-zOffset, width/2, height/2)
				bx2, by2, bz2 := fieldFromWire(current, position[X]-xOffset, position[Z]-zOffset, width/2, height/2)
				bx3, by3, bz3 := fieldFromWire(-current, position[X]+distance-xOffset, position[Z]-zOffset, width/2, height/2)
				mask.Set(0, x, y, z, bx1+bx2+bx3)
				mask.Set(1, x, y, z, by1+by2+by3)
				mask.Set(2, x, y, z, bz1+bz2+bz3)
			}
		}
	}
	return mask
}

func CPWVectorMaskGPU(current, width, height, distance, xOffset, zOffset float64) *data.Slice {
	host := CPWVectorMask(current, width, height, distance, xOffset, zOffset)
	defer host.Free()
	return cuda.GPUCopy(host)
}
