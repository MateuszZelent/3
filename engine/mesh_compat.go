package engine

import (
	"fmt"
	"math"
	"reflect"
)

// Amumax-style mesh declarations allow any two of N, d and T per axis.
type meshCompatibilityState struct {
	n        [3]int
	d, total [3]float64
	hasN     [3]bool
	hasD     [3]bool
	hasTotal [3]bool
	pbc      [3]int
}

var meshCompat meshCompatibilityState

type meshCompatFloat struct {
	axis int
	kind byte // 'd' or 'T'
}

func (value *meshCompatFloat) Type() reflect.Type { return reflect.TypeOf(float64(0)) }
func (value *meshCompatFloat) Eval() interface{} {
	if MeshReady() {
		if value.kind == 'd' {
			return Mesh().CellSize()[value.axis]
		}
		return Mesh().WorldSize()[value.axis]
	}
	if value.kind == 'd' {
		return meshCompat.d[value.axis]
	}
	return meshCompat.total[value.axis]
}
func (value *meshCompatFloat) SetValue(raw interface{}) {
	number := raw.(float64)
	if number <= 0 || math.IsNaN(number) || math.IsInf(number, 0) {
		panic(UserErr("mesh dimension must be finite and positive"))
	}
	if MeshReady() {
		setCreatedMeshFloat(value.axis, value.kind, number)
		return
	}
	if value.kind == 'd' {
		meshCompat.d[value.axis], meshCompat.hasD[value.axis] = number, true
	} else {
		meshCompat.total[value.axis], meshCompat.hasTotal[value.axis] = number, true
	}
	tryCreateCompatibilityMesh()
}

type meshCompatInt struct {
	axis int
	kind byte // 'N' or 'P'
}

func (value *meshCompatInt) Type() reflect.Type { return reflect.TypeOf(int(0)) }
func (value *meshCompatInt) Eval() interface{} {
	if MeshReady() {
		if value.kind == 'N' {
			return Mesh().Size()[value.axis]
		}
		return Mesh().PBC()[value.axis]
	}
	if value.kind == 'N' {
		return meshCompat.n[value.axis]
	}
	return meshCompat.pbc[value.axis]
}
func (value *meshCompatInt) SetValue(raw interface{}) {
	number := raw.(int)
	if value.kind == 'P' {
		if number < 0 {
			panic(UserErr("PBC repetitions must be non-negative"))
		}
		meshCompat.pbc[value.axis] = number
		lazy_pbc[value.axis] = number
		if MeshReady() {
			pbc := Mesh().PBC()
			pbc[value.axis] = number
			SetPBC(pbc[X], pbc[Y], pbc[Z])
		}
		return
	}
	if number <= 0 {
		panic(UserErr("mesh cell count must be positive"))
	}
	if MeshReady() {
		size, cell, pbc := Mesh().Size(), Mesh().CellSize(), Mesh().PBC()
		size[value.axis] = number
		SetMesh(size[X], size[Y], size[Z], cell[X], cell[Y], cell[Z], pbc[X], pbc[Y], pbc[Z])
		return
	}
	meshCompat.n[value.axis], meshCompat.hasN[value.axis] = number, true
	tryCreateCompatibilityMesh()
}

func init() {
	for axis, suffix := range []string{"x", "y", "z"} {
		DeclLValue("N"+suffix, &meshCompatInt{axis: axis, kind: 'N'}, "Mesh cell count along "+suffix)
		DeclLValue("d"+suffix, &meshCompatFloat{axis: axis, kind: 'd'}, "Mesh cell size along "+suffix+" (m)")
		DeclLValue("T"+suffix, &meshCompatFloat{axis: axis, kind: 'T'}, "Physical mesh size along "+suffix+" (m)")
		DeclLValue("PBC"+suffix, &meshCompatInt{axis: axis, kind: 'P'}, "Periodic repetitions along "+suffix)
	}
}

func setCreatedMeshFloat(axis int, kind byte, number float64) {
	size, cell, pbc := Mesh().Size(), Mesh().CellSize(), Mesh().PBC()
	if kind == 'd' {
		cell[axis] = number
	} else {
		cell[axis] = number / float64(size[axis])
	}
	SetMesh(size[X], size[Y], size[Z], cell[X], cell[Y], cell[Z], pbc[X], pbc[Y], pbc[Z])
}

func tryCreateCompatibilityMesh() {
	if MeshReady() {
		return
	}
	for axis := 0; axis < 3; axis++ {
		count := boolInt(meshCompat.hasN[axis]) + boolInt(meshCompat.hasD[axis]) + boolInt(meshCompat.hasTotal[axis])
		if count < 2 {
			return
		}
		if count > 2 {
			suffix := 'x' + rune(axis)
			panic(UserErr(fmt.Sprintf("mesh axis %c: define only two of N%c, d%c and T%c", suffix, suffix, suffix, suffix)))
		}
	}
	for axis := 0; axis < 3; axis++ {
		meshCompat.n[axis], meshCompat.d[axis], meshCompat.total[axis] = resolveCompatibilityAxis(
			meshCompat.n[axis], meshCompat.d[axis], meshCompat.total[axis],
			meshCompat.hasN[axis], meshCompat.hasD[axis], meshCompat.hasTotal[axis],
		)
	}
	SetMesh(meshCompat.n[X], meshCompat.n[Y], meshCompat.n[Z],
		meshCompat.d[X], meshCompat.d[Y], meshCompat.d[Z],
		meshCompat.pbc[X], meshCompat.pbc[Y], meshCompat.pbc[Z])
}

func resolveCompatibilityAxis(n int, d, total float64, hasN, hasD, hasTotal bool) (int, float64, float64) {
	switch {
	case hasN && hasD:
		total = float64(n) * d
	case hasN && hasTotal:
		d = total / float64(n)
	case hasD && hasTotal:
		n = max(1, int(math.Round(total/d)))
		// Cell size is corrected after rounding so the requested total size is exact.
		d = total / float64(n)
	default:
		panic(UserErr("mesh axis requires exactly two of N, d and T"))
	}
	return n, d, total
}

func syncMeshCompatibility(size [3]int, cell [3]float64, pbc [3]int) {
	meshCompat.n, meshCompat.d, meshCompat.pbc = size, cell, pbc
	for axis := 0; axis < 3; axis++ {
		meshCompat.total[axis] = float64(size[axis]) * cell[axis]
		meshCompat.hasN[axis], meshCompat.hasD[axis], meshCompat.hasTotal[axis] = true, true, true
	}
}

func compatibilitySetGridSize(size [3]int) {
	for axis, number := range size {
		meshCompat.n[axis], meshCompat.hasN[axis] = number, true
	}
	tryCreateCompatibilityMesh()
}

func compatibilitySetCellSize(cell [3]float64) {
	for axis, number := range cell {
		meshCompat.d[axis], meshCompat.hasD[axis] = number, true
	}
	tryCreateCompatibilityMesh()
}

func compatibilitySetTotalSize(total [3]float64) {
	for axis, number := range total {
		meshCompat.total[axis], meshCompat.hasTotal[axis] = number, true
	}
	tryCreateCompatibilityMesh()
}

func boolInt(value bool) int {
	if value {
		return 1
	}
	return 0
}
