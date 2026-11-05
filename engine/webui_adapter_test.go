package engine

import "testing"

func TestEvalTryRecoverErrorReturnsCompileErrors(t *testing.T) {
	if err := EvalTryRecoverError(")"); err == nil {
		t.Fatal("script compilation error was reported as success")
	}
}
