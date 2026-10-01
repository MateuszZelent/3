package script

import (
	"fmt"
	"math"
	"strconv"
	"strings"
	"testing"
)

func TestExplicitInt(t *testing.T) {
	w := NewWorld()
	for src, want := range map[string]int{
		"int(3.8)": 3, "int(-3.8)": -3, "int(3)": 3,
		"int(-3)": -3, "int(0.9)": 0, "int(-0.9)": 0,
		"int(0)": 0, "int(int(3.8))": 3,
	} {
		if got := w.MustEval(src); got != want {
			t.Errorf("%s = %v (%T), want int %d", src, got, got, want)
		}
	}
	if got := w.MustEval("int(500e-9 / 1e-9)"); got != 500 {
		t.Fatalf("floating point correction = %v, want 500", got)
	}
	w.Func("cells", func(x, y, z int) int { return x + y + z })
	w.MustExec(`Tx := 500e-9; dx := 1e-9; nCellX := int(Tx / dx)`)
	if got := w.MustEval("cells(nCellX, nCellX, nCellX)"); got != 1500 {
		t.Fatalf("grid sum = %v, want 1500", got)
	}
	assertIntPanic(t, func() { w.MustEval("cells(3.8, 1, 1)") }, "can not use 3.8 as int")
}

func TestExplicitIntTolerance(t *testing.T) {
	w := NewWorld()
	x := 0.0
	w.Var("x", &x)
	for _, target := range []float64{1, -1, 500, -500} {
		for _, direction := range []float64{math.Inf(-1), math.Inf(1)} {
			x = target
			for steps := 1; steps <= 5; steps++ {
				x = math.Nextafter(x, direction)
				want := int(target)
				if steps > 4 {
					want = int(math.Trunc(x))
				}
				if got := w.MustEval("int(x)"); got != want {
					t.Errorf("int(%v), %d steps from %v = %v, want %d", x, steps, target, got, want)
				}
			}
		}
	}
	for _, value := range []float64{499.999999, -499.999999, 1e-20, -1e-20} {
		x = value
		if got := w.MustEval("int(x)"); got != int(math.Trunc(x)) {
			t.Errorf("int(%v) = %v, want truncation", x, got)
		}
	}
}

func TestExplicitIntLimits(t *testing.T) {
	limit := math.Ldexp(1, strconv.IntSize-1)
	w := NewWorld()
	x := 0.0
	w.Var("x", &x)
	for _, value := range []float64{-limit, math.Nextafter(limit, 0), math.Nextafter(-limit, 0)} {
		x = value
		if got := w.MustEval("int(x)"); got != int(math.Trunc(x)) {
			t.Errorf("int(%v) = %v", x, got)
		}
	}
	for _, value := range []float64{math.NaN(), math.Inf(1), math.Inf(-1)} {
		x = value
		assertIntPanic(t, func() { w.MustEval("int(x)") }, "expected a finite number")
	}
	for _, value := range []float64{limit, -limit - 4096, math.MaxFloat64} {
		x = value
		assertIntPanic(t, func() { w.MustEval("int(x)") }, "result outside")
	}
}

func assertIntPanic(t *testing.T, f func(), message string) {
	t.Helper()
	defer func() {
		if p := recover(); p == nil || !strings.Contains(fmt.Sprint(p), message) {
			t.Errorf("panic = %v, want message containing %q", p, message)
		}
	}()
	f()
}
