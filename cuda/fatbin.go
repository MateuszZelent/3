package cuda

import (
	"log"

	"github.com/mumax/3/cuda/cu"
)

// load PTX code for function name, find highest SM that matches our card.
func fatbinLoad(sm map[int]string, fn string) cu.Function {
	cc := determineCC()
	code := sm[cc]
	if code == "" {
		// PTX targeted at an older virtual architecture can be JIT-compiled for
		// newer GPUs. Generated extension kernels may therefore provide fewer
		// entries than the core mumax3 kernel set.
		best := 0
		for candidate, candidateCode := range sm {
			if candidateCode != "" && candidate <= cc && candidate > best {
				best = candidate
				code = candidateCode
			}
		}
	}
	if code == "" {
		panic("no compatible PTX image for CUDA kernel " + fn)
	}
	return cu.ModuleLoadData(code).GetFunction(fn)
}

var UseCC = 0

func determineCC() int {
	if UseCC != 0 {
		return UseCC
	}

	for k := range madd2_map {
		if k > UseCC && ccIsOK(k) {
			UseCC = k
		}
	}
	if UseCC == 0 {
		log.Fatalln("\nNo binary for GPU. Your nvidia driver may be out-of-date\n")
	}
	return UseCC
}

// check whether compute capability cc works
func ccIsOK(cc int) (ok bool) {
	defer func() {
		if err := recover(); err == cu.ERROR_NO_BINARY_FOR_GPU {
			ok = false
		}
	}()
	cu.ModuleLoadData(madd2_map[cc]).GetFunction("madd2")
	return true
}
