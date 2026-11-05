package script

import (
	"reflect"
	"testing"
)

func TestAssignmentMetadataIsCapturedOnceAndNotInsideLoop(t *testing.T) {
	world := NewWorld()
	var captured []string
	previous := AddMetadata
	AddMetadata = func(key string, value interface{}) {
		captured = append(captured, key)
	}
	defer func() { AddMetadata = previous }()

	code, err := world.Compile("a := 1\nfor i := 0; i < 3; i++ { a += 1 }")
	if err != nil {
		t.Fatal(err)
	}
	code.Eval()
	if len(captured) != 1 || captured[0] != "a" {
		t.Fatalf("captured assignments = %v, want [a]", captured)
	}
}

type metadataSnapshotLvalue struct {
	value    float64
	assigned bool
}

func (l *metadataSnapshotLvalue) Eval() interface{}  { return l.value }
func (l *metadataSnapshotLvalue) Type() reflect.Type { return reflect.TypeOf(float64(0)) }
func (l *metadataSnapshotLvalue) Child() []Expr      { return nil }
func (l *metadataSnapshotLvalue) Fix() Expr          { return NewConst(l) }
func (l *metadataSnapshotLvalue) SetValue(value interface{}) {
	l.value = value.(float64)
	l.assigned = true
}
func (l *metadataSnapshotLvalue) MetadataSnapshot() (interface{}, bool) {
	if !l.assigned {
		return nil, false
	}
	return l.value + 1, true
}

func TestAssignmentMetadataUsesTargetSnapshotAfterSet(t *testing.T) {
	world := NewWorld()
	target := new(metadataSnapshotLvalue)
	world.LValue("a", target)
	var captured interface{}
	previous := AddMetadata
	AddMetadata = func(_ string, value interface{}) { captured = value }
	defer func() { AddMetadata = previous }()

	code, err := world.Compile("a = 2")
	if err != nil {
		t.Fatal(err)
	}
	code.Eval()
	if !target.assigned || target.value != 2 {
		t.Fatalf("assigned target = %v (assigned=%v), want 2", target.value, target.assigned)
	}
	if captured != float64(3) {
		t.Fatalf("metadata snapshot = %#v, want target-derived value 3", captured)
	}
}
