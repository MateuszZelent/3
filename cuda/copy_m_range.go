package cuda

import (
	"unsafe"

	"github.com/mumax/3/cuda/cu"
	"github.com/mumax/3/data"
	"github.com/mumax/3/util"
)

// CopyMRange copies a rectangular block between single-component GPU slices.
// A temporary device snapshot makes overlapping in-place copies deterministic.
func CopyMRange(dst, src *data.Slice, dst0, src0, box [3]int, wrap bool) {
	util.Argument(dst.NComp() == 1 && src.NComp() == 1)
	util.Argument(dst.Size() == src.Size())
	size := dst.Size()
	for axis := 0; axis < 3; axis++ {
		util.Argument(box[axis] >= 0)
		if !wrap {
			util.Argument(src0[axis] >= 0 && src0[axis]+box[axis] <= size[axis])
			util.Argument(dst0[axis] >= 0 && dst0[axis]+box[axis] <= size[axis])
		}
	}
	if box[0] == 0 || box[1] == 0 || box[2] == 0 {
		return
	}

	snapshot := NewSlice(1, size)
	defer snapshot.Free()
	data.Copy(snapshot, src)

	for z := 0; z < box[2]; z++ {
		for y := 0; y < box[1]; y++ {
			for x := 0; x < box[0]; {
				sx, sy, sz := src0[0]+x, src0[1]+y, src0[2]+z
				dx, dy, dz := dst0[0]+x, dst0[1]+y, dst0[2]+z
				if wrap {
					sx, sy, sz = wrapIndex(sx, size[0]), wrapIndex(sy, size[1]), wrapIndex(sz, size[2])
					dx, dy, dz = wrapIndex(dx, size[0]), wrapIndex(dy, size[1]), wrapIndex(dz, size[2])
				}
				run := box[0] - x
				if wrap {
					run = min(run, size[0]-sx, size[0]-dx)
				}
				srcIndex := (sz*size[1]+sy)*size[0] + sx
				dstIndex := (dz*size[1]+dy)*size[0] + dx
				srcPtr := unsafe.Pointer(uintptr(snapshot.DevPtr(0)) + uintptr(srcIndex)*cu.SIZEOF_FLOAT32)
				dstPtr := unsafe.Pointer(uintptr(dst.DevPtr(0)) + uintptr(dstIndex)*cu.SIZEOF_FLOAT32)
				MemCpy(dstPtr, srcPtr, int64(run)*cu.SIZEOF_FLOAT32)
				x += run
			}
		}
	}
}

func wrapIndex(index, length int) int {
	index %= length
	if index < 0 {
		index += length
	}
	return index
}
