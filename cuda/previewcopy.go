package cuda

import (
	"github.com/mumax/3/cuda/cu"
	"github.com/mumax/3/data"
	"github.com/mumax/3/util"
	"unsafe"
)

// PreviewHostBuffer owns a contiguous, pinned host allocation. It is used only
// on the engine's CUDA thread; Copy completes before CPU processing can read it.
// Several component DMAs are queued behind resize kernels with one final barrier.
type PreviewHostBuffer struct {
	Slice      *data.Slice
	ptr        unsafe.Pointer
	components []unsafe.Pointer
}

func NewPreviewHostBuffer(nComp int, size [3]int) *PreviewHostBuffer {
	bytes := int64(prod(size)) * cu.SIZEOF_FLOAT32
	ptr := cu.MemAllocHost(bytes * int64(nComp))
	components := make([]unsafe.Pointer, nComp)
	for c := range components {
		components[c] = unsafe.Add(ptr, int64(c)*bytes)
	}
	return &PreviewHostBuffer{Slice: data.SliceFromPtrs(size, data.CPUMemory, components), ptr: ptr, components: components}
}

func (b *PreviewHostBuffer) Copy(src *data.Slice) {
	util.Assert(b.Slice.Size() == src.Size() && b.Slice.NComp() == src.NComp())
	bytes := int64(src.Len()) * cu.SIZEOF_FLOAT32
	for c, dst := range b.components {
		cu.MemcpyDtoHAsync(dst, cu.DevicePtr(uintptr(src.DevPtr(c))), bytes, stream0)
	}
	Sync()
}

func (b *PreviewHostBuffer) Free() {
	if b != nil && b.ptr != nil {
		cu.MemFreeHost(b.ptr)
		b.ptr = nil
		b.Slice.Disable()
		b.components = nil
	}
}
