package tabular

import (
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/mumax/3/httpfs"
)

func TestCrashTableHelper(t *testing.T) {
	dir := os.Getenv("MUMAX_TABLE_CRASH_DIR")
	if dir == "" {
		return
	}
	format, phase := os.Getenv("MUMAX_TABLE_CRASH_FORMAT"), os.Getenv("MUMAX_TABLE_CRASH_PHASE")
	w, err := New(filepath.Join(dir, "table"), format, []string{"t", "mx"}, []string{"s", ""})
	if err != nil {
		t.Fatal(err)
	}
	if err = w.Append(samples(0, 3)); err != nil {
		t.Fatal(err)
	}
	w.put = func(p string, b []byte) error {
		if err := httpfs.PutAtomic(p, b); err != nil {
			return err
		}
		if strings.HasSuffix(filepath.ToSlash(p), phase) {
			if err := os.WriteFile(filepath.Join(dir, "ready"), []byte("ready"), 0600); err != nil {
				return err
			}
			for {
				time.Sleep(time.Second)
			}
		}
		return nil
	}
	if err = w.Append(samples(3, 1026)); err != nil {
		t.Fatal(err)
	}
	t.Fatal("publication did not reach crash phase")
}

func TestKilledWriterRetainsCommittedRows(t *testing.T) {
	for _, format := range []string{"zarr", "hdf5"} {
		phases := []string{"manifest.json"}
		if format == "zarr" {
			phases = append(phases, "t/0", "t/.zarray")
		}
		for _, phase := range phases {
			t.Run(format+"-"+strings.ReplaceAll(phase, "/", "-"), func(t *testing.T) {
				dir := t.TempDir()
				child := exec.Command(os.Args[0], "-test.run=^TestCrashTableHelper$")
				child.Env = append(os.Environ(), "MUMAX_TABLE_CRASH_DIR="+dir, "MUMAX_TABLE_CRASH_FORMAT="+format, "MUMAX_TABLE_CRASH_PHASE="+phase)
				if err := child.Start(); err != nil {
					t.Fatal(err)
				}
				t.Cleanup(func() { _ = child.Process.Kill() })
				deadline := time.Now().Add(10 * time.Second)
				for {
					if _, err := os.Stat(filepath.Join(dir, "ready")); err == nil {
						break
					}
					if time.Now().After(deadline) {
						_ = child.Process.Kill()
						_ = child.Wait()
						t.Fatal("writer never reached crash phase")
					}
					time.Sleep(10 * time.Millisecond)
				}
				if err := child.Process.Kill(); err != nil {
					t.Fatal(err)
				}
				_ = child.Wait()
				m, v, err := Read(filepath.Join(dir, "table"))
				if err != nil {
					t.Fatal(err)
				}
				want := 3
				if phase == "manifest.json" {
					want = 1029
				}
				if m.Rows != want || !reflect.DeepEqual(v, samples(0, want)) {
					t.Fatalf("corrupted committed prefix after kill: rows=%d want=%d", m.Rows, want)
				}
			})
		}
	}
}
