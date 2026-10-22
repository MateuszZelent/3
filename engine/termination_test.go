package engine

import (
	"os"
	"syscall"
	"testing"

	"github.com/mumax/3/script"
)

func isolateTermination(t *testing.T) {
	t.Helper()
	previous := termination
	termination = newTerminationState()
	t.Cleanup(func() { termination = previous })
}

func TestStopPreventsFollowingScriptInstructions(t *testing.T) {
	isolateTermination(t)
	w := script.NewWorld()
	marker := 0
	w.Func("stopNow", func() { RequestStop(syscall.SIGTERM) })
	w.Var("marker", &marker)
	tree, err := w.Compile("stopNow(); marker = 1")
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		p := recover()
		interrupted, ok := p.(Interrupted)
		if !ok || interrupted.ExitCode() != 143 || marker != 0 {
			t.Fatalf("stop was swallowed: %v marker=%d", p, marker)
		}
	}()
	tree.Eval()
	t.Fatal("execution continued")
}

func TestFirstStopReasonWins(t *testing.T) {
	isolateTermination(t)
	RequestStop(os.Interrupt)
	RequestStop(syscall.SIGTERM)
	select {
	case <-StopRequested():
	default:
		t.Fatal("missing wakeup")
	}
	if StopReason().ExitCode() != 130 {
		t.Fatal("stop reason changed")
	}
}
