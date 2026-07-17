package cuda

import (
	"github.com/mumax/3/data"
	"github.com/mumax/3/util"
	"unsafe"
)

// Select and resize one layer for interactive output
func Resize(dst, src *data.Slice, layer int) {
	dstsize := dst.Size()
	srcsize := src.Size()
	util.Assert(dstsize[Z] == 1)
	util.Assert(dst.NComp() == 1 && src.NComp() == 1)

	scalex := srcsize[X] / dstsize[X]
	scaley := srcsize[Y] / dstsize[Y]
	util.Assert(scalex > 0 && scaley > 0)

	cfg := make3DConf(dstsize)

	k_resize_async(dst.DevPtr(0), dstsize[X], dstsize[Y], dstsize[Z],
		src.DevPtr(0), srcsize[X], srcsize[Y], srcsize[Z], layer, scalex, scaley, cfg)
}

// ResizeLayerTo downsamples one source layer into one layer of a larger
// destination volume. Launches stay asynchronous, allowing callers to batch
// many layers before a single GPU-to-CPU copy.
func ResizeLayerTo(dst, src *data.Slice, dstLayer, srcLayer int) {
	dstsize := dst.Size()
	srcsize := src.Size()
	util.Assert(dst.NComp() == 1 && src.NComp() == 1)
	util.Assert(dstLayer >= 0 && dstLayer < dstsize[Z])
	util.Assert(srcLayer >= 0 && srcLayer < srcsize[Z])

	scalex := srcsize[X] / dstsize[X]
	scaley := srcsize[Y] / dstsize[Y]
	util.Assert(scalex > 0 && scaley > 0)

	layerSizeBytes := uintptr(dstsize[X] * dstsize[Y] * data.SIZEOF_FLOAT32)
	dstPtr := unsafe.Add(dst.DevPtr(0), uintptr(dstLayer)*layerSizeBytes)
	layerSize := [3]int{dstsize[X], dstsize[Y], 1}
	cfg := make3DConf(layerSize)

	k_resize_async(dstPtr, dstsize[X], dstsize[Y], 1,
		src.DevPtr(0), srcsize[X], srcsize[Y], srcsize[Z], srcLayer, scalex, scaley, cfg)
}
