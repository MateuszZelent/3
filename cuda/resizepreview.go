package cuda

import (
	"github.com/mumax/3/data"
	"github.com/mumax/3/util"
)

// ResizePreview batches all sampled layers into one launch per component.
func ResizePreview(dst, src *data.Slice, layer, stride int) {
	d, s := dst.Size(), src.Size()
	util.Assert(dst.NComp() == src.NComp() && stride >= 1)
	util.Assert(layer < 0 || layer < s[Z])
	cfg := make3DConf(d)
	for c := 0; c < dst.NComp(); c++ {
		k_resizepreview_async(dst.DevPtr(c), src.DevPtr(c), d[X], d[Y], d[Z], s[X], s[Y], s[Z], layer, stride, cfg)
	}
}
