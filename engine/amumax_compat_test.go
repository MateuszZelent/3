package engine

import "testing"

func TestReadArrayFromString(t *testing.T) {
	array := ReadArrayFromString("1, -2.5, 3e2")
	if array.Len() != 3 || array.Get(0) != 1 || array.Get(1) != -2.5 || array.Get(2) != 300 {
		t.Fatalf("parsed array = %#v", array.data)
	}
}

func TestReadArrayFromStringRejectsInvalidInput(t *testing.T) {
	defer func() {
		if recover() == nil {
			t.Fatal("invalid array did not panic")
		}
	}()
	ReadArrayFromString("1, nope")
}

func TestRunWithoutPrecessionRestoresState(t *testing.T) {
	previous := Precess
	Precess = true
	defer func() { Precess = previous }()
	RunWithoutPrecession(-1)
	if !Precess {
		t.Fatal("precession state was not restored")
	}
}

func TestRunShellRequiresInsecureFlag(t *testing.T) {
	previous := *Flag_insecure
	*Flag_insecure = false
	defer func() { *Flag_insecure = previous }()
	defer func() {
		if recover() == nil {
			t.Fatal("RunShell executed without -insecure")
		}
	}()
	RunShell("true")
}
