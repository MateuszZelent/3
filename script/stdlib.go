package script

import (
	"fmt"
	"math"
	"math/rand"
	"strconv"
	"time"
)

// Loads standard functions into the world.
func (w *World) LoadStdlib() {

	// literals
	w.declare("true", boolLit(true))
	w.declare("false", boolLit(false))

	// math
	w.Func("int", intCast, "Snaps values within four float64 steps of an integer to that integer, otherwise truncates toward zero; rejects NaN, infinity and results outside the int range.")
	w.Func("abs", math.Abs)
	w.Func("acos", math.Acos)
	w.Func("acosh", math.Acosh)
	w.Func("asin", math.Asin)
	w.Func("asinh", math.Asinh)
	w.Func("atan", math.Atan)
	w.Func("atanh", math.Atanh)
	w.Func("cbrt", math.Cbrt)
	w.Func("ceil", math.Ceil)
	w.Func("cos", math.Cos)
	w.Func("round", math.Round)
	w.Func("cosh", math.Cosh)
	w.Func("erf", math.Erf)
	w.Func("erfc", math.Erfc)
	w.Func("exp", math.Exp)
	w.Func("exp2", math.Exp2)
	w.Func("expm1", math.Expm1)
	w.Func("floor", math.Floor)
	w.Func("gamma", math.Gamma)
	w.Func("j0", math.J0)
	w.Func("j1", math.J1)
	w.Func("log", math.Log)
	w.Func("log10", math.Log10)
	w.Func("log1p", math.Log1p)
	w.Func("log2", math.Log2)
	w.Func("logb", math.Logb)
	w.Func("sin", math.Sin)
	w.Func("sinh", math.Sinh)
	w.Func("sqrt", math.Sqrt)
	w.Func("tan", math.Tan)
	w.Func("tanh", math.Tanh)
	w.Func("trunc", math.Trunc)
	w.Func("y0", math.Y0)
	w.Func("y1", math.Y1)
	w.Func("ilogb", math.Ilogb)
	w.Func("pow10", math.Pow10)
	w.Func("atan2", math.Atan2)
	w.Func("hypot", math.Hypot)
	w.Func("remainder", math.Remainder)
	w.Func("max", math.Max)
	w.Func("min", math.Min)
	w.Func("mod", math.Mod)
	w.Func("pow", math.Pow)
	w.Func("yn", math.Yn)
	w.Func("jn", math.Jn)
	w.Func("ldexp", math.Ldexp)
	w.Func("isInf", math.IsInf)
	w.Func("isNaN", math.IsNaN)
	w.Func("norm", norm, "Standard normal distribution")
	w.Func("heaviside", heaviside, "Returns 1 if x>0, 0 if x<0, and 0.5 if x==0")
	w.Func("sinc", sinc, "Sinc returns sin(x)/x. If x=0, then Sinc(x) returns 1.")
	w.Func("randSeed", intseed, "Sets the random number seed for rand(), randExp(), randNorm() and randInt().")
	w.Func("rand", rng.Float64, "Random number between 0 and 1")
	w.Func("randExp", rng.ExpFloat64, "Exponentially distributed random number between 0 and +inf, mean=1")
	w.Func("randNorm", rng.NormFloat64, "Standard normal random number")
	w.Func("randInt", randInt, "Random non-negative integer")
	w.declare("pi", floatLit(math.Pi))
	w.declare("inf", floatLit(math.Inf(1)))

	//string
	w.Func("sprint", fmt.Sprint, "Print all arguments to string with automatic formatting")
	w.Func("sprintf", fmt.Sprintf, "Print to string with C-style formatting.")

	//time
	w.Func("now", time.Now, "Returns the current time")
	w.Func("since", time.Since, "Returns the time elapsed since argument")
}

var rng = rand.New(rand.NewSource(0))

// intCast is an explicit conversion; implicit conversions still use safe_int.
func intCast(x float64) int {
	if math.IsNaN(x) || math.IsInf(x, 0) {
		panic(fmt.Errorf("int(%v): expected a finite number", x))
	}
	// Correct small floating point errors in expressions such as 500e-9 / 1e-9.
	// Walking representable values avoids a tolerance that grows to whole units
	// for large numbers or applies an arbitrary absolute threshold near zero.
	nearest := math.Round(x)
	probe := x
	for step := 0; step < 4 && probe != nearest; step++ {
		probe = math.Nextafter(probe, nearest)
	}
	if probe == nearest {
		x = nearest
	}
	truncated := math.Trunc(x)
	// Use an exclusive upper bound: float64(maxInt) rounds up on 64-bit hosts.
	limit := math.Ldexp(1, strconv.IntSize-1)
	if truncated < -limit || truncated >= limit {
		panic(fmt.Errorf("int(%v): result outside %d-bit int range", x, strconv.IntSize))
	}
	return int(truncated)
}

// script does not know int64
func intseed(seed int)      { rng.Seed(int64(seed)) }
func randInt(upper int) int { return rng.Int() % upper }

func heaviside(x float64) float64 {
	switch {
	default:
		return 1
	case x == 0:
		return 0.5
	case x < 0:
		return 0
	}
}

func norm(x float64) float64 {
	return (1 / math.Sqrt(2*math.Pi)) * math.Exp(-0.5*x*x)
}

func sinc(x float64) float64 {
	if x == 0 {
		return 1
	} else {
		return math.Sin(x) / x
	}
}
