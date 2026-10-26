package main

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"syscall"
	"testing"
	"time"

	"github.com/mumax/3/tabular"
	"github.com/mumax/3/zarr"
)

func TestRuntimeZarrInterruptionKeepsCommittedPrefix(t *testing.T) {
	binary := os.Getenv("MUMAX_RUNTIME_BINARY")
	if binary == "" {
		t.Skip("set MUMAX_RUNTIME_BINARY for actual GPU Zarr interruption tests")
	}

	for _, signal := range []syscall.Signal{syscall.SIGTERM, syscall.SIGKILL} {
		signal := signal
		t.Run(signal.String(), func(t *testing.T) {
			runRuntimeZarrInterruption(t, binary, signal)
		})
	}
}

func runRuntimeZarrInterruption(t *testing.T, binary string, signal syscall.Signal) {
	t.Helper()
	dir := t.TempDir()
	input := filepath.Join(dir, "interruption.mx3")
	output := filepath.Join(dir, "result.zarr")
	transcriptPath := filepath.Join(dir, "solver.log")

	// The run is intentionally much longer than the observation window. The
	// test interrupts only after two committed frames are visible.
	const script = `SetGridSize(64,16,1)
SetCellSize(2e-9,2e-9,2e-9)
Msat=800e3
Aex=13e-12
alpha=0.1
m=Uniform(1,0,0)
FixDt=1e-12
AutoSaveAs(m,"m",1e-12)
TableAutoSave(1e-12)
Run(1e-6)
`
	if err := os.WriteFile(input, []byte(script), 0o600); err != nil {
		t.Fatal(err)
	}
	transcript, err := os.Create(transcriptPath)
	if err != nil {
		t.Fatal(err)
	}
	defer transcript.Close()

	cmd := exec.Command(
		binary,
		"-webui-disable",
		"-check-updates=false",
		"-hide-progress-bar",
		"-insecure",
		"-o",
		output,
		input,
	)
	cmd.Stdout = transcript
	cmd.Stderr = transcript
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	waited := false
	t.Cleanup(func() {
		if !waited {
			_ = cmd.Process.Kill()
			_ = cmd.Wait()
		}
	})

	dataset := filepath.Join(output, "m")
	deadline := time.Now().Add(30 * time.Second)
	for {
		commit, readErr := zarr.ReadCommit(dataset)
		if readErr == nil && commit.CommittedSteps >= 2 {
			break
		}
		if time.Now().After(deadline) {
			_ = cmd.Process.Kill()
			_ = cmd.Wait()
			waited = true
			log, _ := os.ReadFile(transcriptPath)
			t.Fatalf("solver did not publish two committed frames: %v\n%s", readErr, log)
		}
		time.Sleep(50 * time.Millisecond)
	}

	if err := cmd.Process.Signal(signal); err != nil {
		t.Fatal(err)
	}
	done := make(chan error, 1)
	go func() { done <- cmd.Wait() }()
	select {
	case <-done:
		waited = true
	case <-time.After(15 * time.Second):
		_ = cmd.Process.Kill()
		<-done
		waited = true
		t.Fatal("solver did not exit within 15 seconds after interruption")
	}

	if signal == syscall.SIGTERM {
		if got := cmd.ProcessState.ExitCode(); got != 143 {
			t.Fatalf("SIGTERM exit code = %d, want 143", got)
		}
	} else {
		status, ok := cmd.ProcessState.Sys().(syscall.WaitStatus)
		if !ok || !status.Signaled() || status.Signal() != signal {
			t.Fatalf("SIGKILL wait status = %#v, want signal %d", cmd.ProcessState.Sys(), signal)
		}
	}

	// The commit head and every frame/chunk it names must remain a readable,
	// self-consistent prefix after either signal.
	commit, err := zarr.ReadCommit(dataset)
	if err != nil {
		t.Fatal(err)
	}
	if commit.CommittedSteps < 2 {
		t.Fatalf("committed prefix has only %d frames", commit.CommittedSteps)
	}
	frames, err := zarr.ReadCommittedFrames(dataset, commit)
	if err != nil {
		t.Fatal(err)
	}
	if len(frames) != commit.CommittedSteps {
		t.Fatalf("frame count = %d, want %d", len(frames), commit.CommittedSteps)
	}
	for step := 0; step < commit.CommittedSteps; step++ {
		field, readErr := zarr.ReadStep(dataset, step)
		if readErr != nil {
			t.Fatalf("ReadStep(%d): %v", step, readErr)
		}
		field.Free()
	}

	assertRuntimeRootAttributes(t, output)
	assertRuntimeTableManifest(t, output)

	if signal == syscall.SIGTERM {
		assertInterruptedRunMetadata(t, output)
		assertRuntimeProjection(t, dataset, commit, frames)
	}
}

func assertRuntimeRootAttributes(t *testing.T, output string) {
	t.Helper()
	b, err := os.ReadFile(filepath.Join(output, ".zattrs"))
	if err != nil {
		t.Fatal(err)
	}
	var attrs map[string]any
	if err := json.Unmarshal(b, &attrs); err != nil {
		t.Fatalf("root .zattrs is not readable JSON: %v", err)
	}
	if attrs == nil {
		t.Fatal("root .zattrs is null")
	}
}

func assertRuntimeTableManifest(t *testing.T, output string) {
	t.Helper()
	reader, err := tabular.Open(filepath.Join(output, "table"))
	if err != nil {
		t.Fatalf("table manifest: %v", err)
	}
	manifest := reader.Metadata()
	if manifest.Rows < 1 || len(manifest.Columns) == 0 {
		t.Fatalf("invalid table manifest: %+v", manifest)
	}
	rows := 0
	err = reader.Walk(func(start int, values [][]float64) error {
		if start != rows || len(values) != len(manifest.Columns) {
			return fmt.Errorf("non-contiguous table batch at %d after %d", start, rows)
		}
		if len(values) == 0 {
			return fmt.Errorf("empty table batch at %d", start)
		}
		for _, column := range values {
			if len(column) != len(values[0]) {
				return fmt.Errorf("unaligned table batch at %d", start)
			}
		}
		rows += len(values[0])
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if rows != manifest.Rows {
		t.Fatalf("table manifest rows=%d but reader walked %d", manifest.Rows, rows)
	}
}

func assertInterruptedRunMetadata(t *testing.T, output string) {
	t.Helper()
	b, err := os.ReadFile(filepath.Join(output, "run.json"))
	if err != nil {
		t.Fatal(err)
	}
	var metadata map[string]any
	if err := json.Unmarshal(b, &metadata); err != nil {
		t.Fatal(err)
	}
	if metadata["completion"] != "interrupted" {
		t.Fatalf("run completion = %v, want interrupted", metadata["completion"])
	}
}

func assertRuntimeProjection(t *testing.T, dataset string, commit zarr.Commit, frames []zarr.FrameRecord) {
	t.Helper()
	metadata, err := zarr.ReadMetadata(dataset)
	if err != nil {
		t.Fatal(err)
	}
	if metadata.Shape[0] != commit.CommittedSteps {
		t.Fatalf("projected shape = %v, want %d committed steps", metadata.Shape, commit.CommittedSteps)
	}
	attrs, err := os.ReadFile(filepath.Join(dataset, ".zattrs"))
	if err != nil {
		t.Fatal(err)
	}
	var projected zarr.Attributes
	if err := json.Unmarshal(attrs, &projected); err != nil {
		t.Fatal(err)
	}
	if len(projected.Times) != len(frames) {
		t.Fatalf("projected times=%d, frames=%d", len(projected.Times), len(frames))
	}
	for i, frame := range frames {
		if projected.Times[i] != frame.Time {
			t.Fatalf("projected time[%d]=%g, frame=%g", i, projected.Times[i], frame.Time)
		}
	}
}
