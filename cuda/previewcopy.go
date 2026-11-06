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

// PreviewBuffers is grow-only device and pinned host storage for preview
// captures. Allocating or freeing either kind of memory synchronizes the
// device; while a large simulation occupies the GPU this takes seconds, and a
// 3D render window changes the capture size on every slider step.
type PreviewBuffers struct {
	floats int64 // capacity in float32 values, shared by all components
	device unsafe.Pointer
	host   unsafe.Pointer
}

// View returns pinned host and device slices of the given size backed by the
// shared storage, growing it when needed. Views must not be freed; they stay
// valid until a later View call grows the storage.
func (b *PreviewBuffers) View(nComp int, size [3]int) (*PreviewHostBuffer, *data.Slice) {
	length := int64(prod(size))
	required := length * int64(nComp)
	if b.device == nil || required > b.floats {
		data.EnableGPU(memFree, cu.MemFreeHost, MemCpy, MemCpyDtoH, MemCpyHtoD)
		b.Free()
		b.floats = required
		b.device = MemAlloc(required * cu.SIZEOF_FLOAT32)
		b.host = cu.MemAllocHost(required * cu.SIZEOF_FLOAT32)
	}
	stride := length * cu.SIZEOF_FLOAT32
	device := make([]unsafe.Pointer, nComp)
	host := make([]unsafe.Pointer, nComp)
	for c := range nComp {
		device[c] = unsafe.Add(b.device, int64(c)*stride)
		host[c] = unsafe.Add(b.host, int64(c)*stride)
	}
	view := &PreviewHostBuffer{Slice: data.SliceFromPtrs(size, data.CPUMemory, host), components: host}
	return view, data.SliceFromPtrs(size, data.GPUMemory, device)
}

func (b *PreviewBuffers) Free() {
	if b.device != nil {
		memFree(b.device)
		b.device = nil
	}
	if b.host != nil {
		cu.MemFreeHost(b.host)
		b.host = nil
	}
	b.floats = 0
}
