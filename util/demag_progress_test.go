package util

import (
	"fmt"
	"sync"
	"testing"
)

func TestDemagProgress(t *testing.T) {
	id := BeginDemagInitialization()
	SetProgressHidden(true)
	defer SetProgressHidden(false)
	Progress(25, 100, "Demag kernel calculation")
	if s := DemagProgressSnapshot(); s.Percent != 25 || !s.Active {
		t.Fatalf("hidden terminal progress lost: %+v", s)
	}
	var wg sync.WaitGroup
	for i := 0; i < 4; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < 30; j++ {
				recordDemagLog(fmt.Sprintf("Demag kernel cache %d", j))
				_ = DemagProgressSnapshot()
			}
		}()
	}
	wg.Wait()
	snapshot := DemagProgressSnapshot()
	if len(snapshot.Logs) != 80 {
		t.Fatalf("unbounded log history: %d", len(snapshot.Logs))
	}
	snapshot.Logs[0] = "modified"
	if DemagProgressSnapshot().Logs[0] == "modified" {
		t.Fatal("snapshot aliases shared history")
	}
	EndDemagInitialization(id, true)
	if s := DemagProgressSnapshot(); s.Active || s.Failed || s.Percent != 100 {
		t.Fatalf("bad completion: %+v", s)
	}
	next := BeginDemagInitialization()
	EndDemagInitialization(id, true)
	if !DemagProgressSnapshot().Active {
		t.Fatal("stale operation ended current one")
	}
	EndDemagInitialization(next, false)
	if !DemagProgressSnapshot().Failed {
		t.Fatal("missing failure status")
	}
}
