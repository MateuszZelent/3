package script

import "testing"

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
