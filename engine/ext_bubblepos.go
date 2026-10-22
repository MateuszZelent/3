package engine

import (
	"github.com/mumax/3/coreposition"
	"math"
)

var (
	BubblePos      = NewVectorValue("ext_bubblepos", "m", "Bubble core position", bubblePos)
	BubbleDist     = NewScalarValue("ext_bubbledist", "m", "Bubble traveled distance", bubbleDist)
	BubbleSpeed    = NewScalarValue("ext_bubblespeed", "m/s", "Bubble velocity", bubbleSpeed)
	BubbleMz       = 1.0
	BackGroundTilt = 0.25
	BubbleLayer    = -1
)

func init() {
	DeclVar("ext_BubbleLayer", &BubbleLayer, "Bubble layer index; -1 averages connected layers")
	DeclVar("ext_BubbleMz", &BubbleMz, "Center magnetization 1.0 or -1.0  (default = 1.0)")
	DeclVar("ext_BackGroundTilt", &BackGroundTilt, "Size of in-plane component of background magnetization. All values below this one are rounded down to perfectly out-of-plane to improve position calculation  (default = 0.25)")
}

func bubblePos() []float64 {
	if BubbleMz != 1 && BubbleMz != -1 {
		panic(UserErr("ext_BubbleMz should be +1 or -1"))
	}
	r := coreposition.Skyrmion(coreField(), coreposition.SkyrmionOptions{Polarity: int(BubbleMz), Layer: BubbleLayer, Background: -BubbleMz, Threshold: BackGroundTilt, HardThreshold: true})
	return []float64{r.Position[X], r.Position[Y], 0}
}

var (
	prevBpos   = [2]float64{-1e99, -1e99}
	bdist      = 0.0
	bubbleLost bool
)

func bubbleDist() float64 {
	pos := bubblePos()
	if math.IsNaN(pos[X]) || math.IsNaN(pos[Y]) {
		bubbleLost = true
		return math.NaN()
	}
	if bubbleLost {
		prevBpos = [2]float64{pos[X], pos[Y]}
		bubbleLost = false
		return bdist
	}
	if prevBpos == [2]float64{-1e99, -1e99} {
		prevBpos = [2]float64{pos[X], pos[Y]}
		return 0
	}

	w := Mesh().WorldSize()
	dx := pos[X] - prevBpos[X]
	dy := pos[Y] - prevBpos[Y]
	prevBpos = [2]float64{pos[X], pos[Y]}

	pbc := Mesh().PBC()
	// PBC wrap
	if pbc[X] > 0 && dx > w[X]/2 {
		dx -= w[X]
	}
	if pbc[X] > 0 && dx < -w[X]/2 {
		dx += w[X]
	}
	if pbc[Y] > 0 && dy > w[Y]/2 {
		dy -= w[Y]
	}
	if pbc[Y] > 0 && dy < -w[Y]/2 {
		dy += w[Y]
	}

	bdist += math.Sqrt(dx*dx + dy*dy)
	return bdist
}

var (
	prevBdist  = 0.0
	prevBt     = -999.0
	prevBspeed float64
)

func bubbleSpeed() float64 {
	dist := bubbleDist()
	if math.IsNaN(dist) {
		prevBt = -999
		return math.NaN()
	}
	if Time == prevBt {
		return prevBspeed
	}

	if prevBt < 0 {
		prevBdist = dist
		prevBt = Time
		return 0
	}

	v := (dist - prevBdist) / (Time - prevBt)

	prevBt = Time
	prevBdist = dist
	prevBspeed = v

	return v
}

func backgroundAdjust(arg float32) float64 {
	if float64(arg) < BackGroundTilt {
		return float64(0)
	}
	return float64(arg)
}
