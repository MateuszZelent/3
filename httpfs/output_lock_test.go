package httpfs

import (
	"os"
	"path/filepath"
	"sync"
	"testing"
)

func TestAcquireOutputLockHasSingleOwnerAndCanBeReacquired(t *testing.T) {
	SetWD("")
	output := filepath.Join(t.TempDir(), "run.zarr")
	const contenders = 32

	type result struct {
		release func() error
		err     error
	}
	start := make(chan struct{})
	results := make(chan result, contenders)
	var wg sync.WaitGroup
	for i := 0; i < contenders; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			release, err := AcquireOutputLock(output)
			results <- result{release: release, err: err}
		}()
	}
	close(start)
	wg.Wait()
	close(results)

	owners := 0
	var winner func() error
	for result := range results {
		if result.err == nil {
			owners++
			winner = result.release
		}
	}
	if owners != 1 {
		t.Fatalf("concurrent lock owners = %d, want exactly one", owners)
	}
	lockPath := output + ".zarr-owner"
	if info, err := os.Stat(lockPath); err != nil || !info.IsDir() {
		t.Fatalf("lock directory = %v, stat error = %v", info, err)
	}

	if _, err := AcquireOutputLock(output); err == nil {
		t.Fatal("second acquisition succeeded while the first lock was held")
	}
	if err := winner(); err != nil {
		t.Fatal("release failed:", err)
	}
	if _, err := os.Stat(lockPath); !os.IsNotExist(err) {
		t.Fatalf("lock remains after release: %v", err)
	}

	reacquired, err := AcquireOutputLock(output)
	if err != nil {
		t.Fatal("reacquisition failed:", err)
	}
	if err := reacquired(); err != nil {
		t.Fatal("second release failed:", err)
	}
}

func TestReleaseRefusesChangedOwner(t *testing.T) {
	output := filepath.Join(t.TempDir(), "run.zarr")
	release, err := AcquireOutputLock(output)
	if err != nil {
		t.Fatal(err)
	}
	lock := output + ".zarr-owner"
	if err := os.WriteFile(filepath.Join(lock, "owner.json"), []byte(`{"token":"other"}`), 0600); err != nil {
		t.Fatal(err)
	}
	if err := release(); err == nil {
		t.Fatal("released a different owner's lock")
	}
	if _, err := os.Stat(lock); err != nil {
		t.Fatalf("new owner's lock disappeared: %v", err)
	}
}
