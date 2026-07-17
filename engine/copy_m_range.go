package engine

import "github.com/mumax/3/cuda"

func init() {
	DeclFunc("CopyMRange", CopyMRange, "Copy a block of m: CopyMRange(dx,dy,dz, sx,sy,sz, width,height,depth, wrap)")
}

func CopyMRange(dx, dy, dz, sx, sy, sz, width, height, depth int, wrap bool) {
	buffer := M.Buffer()
	destination := [3]int{dx, dy, dz}
	source := [3]int{sx, sy, sz}
	box := [3]int{width, height, depth}
	for component := 0; component < buffer.NComp(); component++ {
		cuda.CopyMRange(buffer.Comp(component), buffer.Comp(component), destination, source, box, wrap)
	}
}
