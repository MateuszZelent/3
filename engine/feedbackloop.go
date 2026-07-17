package engine

import (
	"fmt"
	"sync"

	"github.com/mumax/3/cuda"
	"github.com/mumax/3/data"
)

func init() {
	DeclFunc("FeedbackLoop", FeedbackLoop, "FeedbackLoop(inputMask, outputMask, gain): baseline-subtracted pickup reinjected into B_ext")
	DeclFunc("magModulatedByMask", MagModulatedByMask, "Projection of m onto a CPU or GPU mask")
	DeclFunc("magModulatedByMaskGPU", MagModulatedByMaskGPU, "GPU projection of m onto an equally-sized GPU mask")
}

// FeedbackLoop measures the projection of m onto outputMask and reinjects the
// baseline-subtracted signal through inputMask as a dynamic B_ext term.
func FeedbackLoop(inputMask, outputMask *data.Slice, gain float64) {
	validateFeedbackMask("input", inputMask)
	validateFeedbackMask("output", outputMask)

	var once sync.Once
	var baseline float64
	B_ext.AddGo(inputMask, func() float64 {
		signal := MagModulatedByMask(outputMask)
		once.Do(func() { baseline = signal })
		return gain * (signal - baseline)
	})
}

func validateFeedbackMask(name string, mask *data.Slice) {
	if mask == nil || mask.IsNil() {
		panic(UserErr(fmt.Sprintf("FeedbackLoop: %s mask is nil", name)))
	}
	if mask.NComp() != 3 {
		panic(UserErr(fmt.Sprintf("FeedbackLoop: %s mask must have 3 components", name)))
	}
	size := mask.Size()
	meshSize := Mesh().Size()
	for axis := range size {
		if size[axis] != 1 && size[axis] != meshSize[axis] {
			panic(UserErr(fmt.Sprintf("FeedbackLoop: %s mask size %v is incompatible with mesh %v", name, size, meshSize)))
		}
	}
}

// MagModulatedByMask returns sum(m*mask). Singleton mask dimensions are
// broadcast, which preserves the useful Amumax antenna-mask behavior.
func MagModulatedByMask(mask *data.Slice) float64 {
	validateFeedbackMask("output", mask)
	if mask.GPUAccess() && mask.Size() == Mesh().Size() {
		return MagModulatedByMaskGPU(mask)
	}

	mag := M.Buffer().HostCopy()
	defer mag.Free()
	maskHost := mask
	if !mask.CPUAccess() {
		maskHost = mask.HostCopy()
		defer maskHost.Free()
	}
	m := mag.Tensors()
	w := maskHost.Tensors()
	meshSize, maskSize := Mesh().Size(), mask.Size()
	var signal float64
	for iz := 0; iz < meshSize[Z]; iz++ {
		mz := iz
		if maskSize[Z] == 1 {
			mz = 0
		}
		for iy := 0; iy < meshSize[Y]; iy++ {
			my := iy
			if maskSize[Y] == 1 {
				my = 0
			}
			for ix := 0; ix < meshSize[X]; ix++ {
				mx := ix
				if maskSize[X] == 1 {
					mx = 0
				}
				for component := 0; component < 3; component++ {
					signal += float64(m[component][iz][iy][ix]) * float64(w[component][mz][my][mx])
				}
			}
		}
	}
	return signal
}

func MagModulatedByMaskGPU(mask *data.Slice) float64 {
	validateFeedbackMask("output", mask)
	if !mask.GPUAccess() || mask.Size() != Mesh().Size() {
		panic(UserErr("magModulatedByMaskGPU: mask must be GPU-accessible and match the mesh size"))
	}
	return float64(cuda.Dot(M.Buffer(), mask))
}
