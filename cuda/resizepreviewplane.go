package cuda

import (
	"github.com/mumax/3/data"
	"github.com/mumax/3/util"
)

// ResizePreviewPlane samples XY (0,1), YZ (1,2), or XZ (0,2). layer=-1
// computes the arithmetic mean across the perpendicular axis. Each output pixel
// covers its entire fractional source area; destination storage is [u,v,1].
func ResizePreviewPlane(dst, src *data.Slice, u, v, layer int) {
	d, s := dst.Size(), src.Size()
	util.Assert(u >= 0 && u < v && v <= 2 && dst.NComp() == src.NComp() && d[2] == 1)
	n := 3 - u - v
	util.Assert(layer >= -1 && layer < s[n])
	strides := [3]int{1, s[0], s[0] * s[1]}
	cfg := make3DConf(d)
	for c := 0; c < dst.NComp(); c++ {
		k_resizepreviewplane_async(dst.DevPtr(c), src.DevPtr(c), d[0], d[1], s[u], s[v], s[n], strides[u], strides[v], strides[n], layer, cfg)
	}
}
