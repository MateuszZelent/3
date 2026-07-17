package mag

import (
	"math"
	"testing"
)

func TestOerstedKernelIsAntisymmetricAndHasZeroSelfTerm(t *testing.T) {
	kernel := OerstedKernel([3]int{3, 1, 1}, [3]float64{1e-9, 1e-9, 1e-9}, [3]int{})
	defer func() {
		for _, component := range kernel {
			component.Free()
		}
	}()
	kx := kernel[X].Scalars()
	size := kernel[X].Size()
	if kx[0][0][0] != 0 {
		t.Fatalf("self term = %g, want 0", kx[0][0][0])
	}
	positive := float64(kx[0][0][1])
	negative := float64(kx[0][0][size[X]-1])
	if positive == 0 || math.Abs(positive+negative) > math.Abs(positive)*1e-6 {
		t.Fatalf("kernel is not antisymmetric: K(+dx)=%g K(-dx)=%g", positive, negative)
	}
}
