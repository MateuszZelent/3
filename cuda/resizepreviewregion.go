package cuda

import (
	"github.com/mumax/3/data"
	"github.com/mumax/3/util"
)

// ResizePreviewRegion averages only source cells inside the half-open window.
func ResizePreviewRegion(dst, src *data.Slice, origin, size [3]int) {
	d, s := dst.Size(), src.Size()
	util.Assert(dst.NComp() == src.NComp())
	for a := 0; a < 3; a++ {
		util.Assert(origin[a] >= 0 && size[a] > 0 && origin[a]+size[a] <= s[a] && d[a] > 0 && d[a] <= size[a])
	}
	cfg := make3DConf(d)
	for c := 0; c < dst.NComp(); c++ {
		kResizepreviewregionAsync(dst.DevPtr(c), src.DevPtr(c), d[X], d[Y], d[Z], s[X], s[Y], s[Z], origin[X], origin[Y], origin[Z], size[X], size[Y], size[Z], cfg)
	}
}
