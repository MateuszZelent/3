package main

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"math"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/mumax/3/tabular"
)

// Run explicitly with MUMAX_RUNTIME_BINARY pointing at the built solver in a
// CUDA-capable environment. These exercise the actual CLI and GPU, not helpers.
func TestRuntimeOutputSignals(t *testing.T) {
	binary := os.Getenv("MUMAX_RUNTIME_BINARY")
	if binary == "" {
		t.Skip("set MUMAX_RUNTIME_BINARY for actual GPU signal tests")
	}
	for _, mode := range []string{"run", "loop", "relax", "minimize", "shell", "interactive"} {
		for _, sig := range []syscall.Signal{syscall.SIGINT, syscall.SIGTERM} {
			t.Run(fmt.Sprintf("%s-%d", mode, sig), func(t *testing.T) {
				dir := t.TempDir()
				input := filepath.Join(dir, "signal.mx3")
				output := filepath.Join(dir, "result.zarr")
				command := map[string]string{"run": "Run(1000)", "loop": "for i := 0; i < 1000000000; i++ {}", "relax": "Relax()", "minimize": "Minimize()", "shell": "RunShell(\"sleep 60 & wait\")", "interactive": "Print(\"READY_FOR_INTERACTIVE_SIGNAL\")"}[mode]
				setup := `SetGridSize(128,128,1)
SetCellSize(2e-9,2e-9,2e-9)
Msat=800e3
Aex=13e-12
alpha=0.1
m=RandomMagSeed(17)
TableAutoSave(1e-12)
TableSave()
`
				suffix := "\nPrint(\"UNREACHABLE_AFTER_SIGNAL\")\n"
				if mode == "interactive" {
					suffix = "\n"
				}
				if err := os.WriteFile(input, []byte(setup+command+suffix), 0600); err != nil {
					t.Fatal(err)
				}
				cmd := exec.Command(binary, "-webui-disable", "-check-updates=false", "-hide-progress-bar", "-insecure", "-o", output, input)
				if mode == "interactive" {
					listener, err := net.Listen("tcp", "127.0.0.1:0")
					if err != nil {
						t.Fatal(err)
					}
					address := listener.Addr().String()
					if err = listener.Close(); err != nil {
						t.Fatal(err)
					}
					cmd.Args = append([]string{binary, "-i", "-http=" + address}, cmd.Args[2:]...)
				}
				stdout, err := cmd.StdoutPipe()
				if err != nil {
					t.Fatal(err)
				}
				var stderr bytes.Buffer
				cmd.Stderr = &stderr
				ready := make(chan struct{})
				done := make(chan struct{})
				if err = cmd.Start(); err != nil {
					t.Fatal(err)
				}
				t.Cleanup(func() { _ = cmd.Process.Kill() })
				var transcript strings.Builder
				go func() {
					defer close(done)
					scanner := bufio.NewScanner(stdout)
					scanner.Buffer(make([]byte, 4096), 1<<20)
					for scanner.Scan() {
						line := scanner.Text()
						transcript.WriteString(line + "\n")
						atOperation := strings.ReplaceAll(line, " ", "") == strings.ReplaceAll(command, " ", "") || (mode == "loop" && strings.HasPrefix(line, "for i :="))
						if mode == "interactive" {
							atOperation = strings.Contains(line, "entering interactive mode")
						}
						if atOperation {
							select {
							case <-ready:
							default:
								close(ready)
							}
						}
					}
				}()
				select {
				case <-ready:
				case <-time.After(25 * time.Second):
					_ = cmd.Process.Kill()
					_ = cmd.Wait()
					<-done
					t.Fatalf("solver did not reach %s: %s %s", mode, transcript.String(), stderr.String())
				}
				// Let the operation enter its loop; the prepared state makes minimization
				// and relaxation nontrivial on this grid.
				time.Sleep(50 * time.Millisecond)
				if err = cmd.Process.Signal(sig); err != nil {
					t.Fatal(err)
				}
				wait := make(chan error, 1)
				go func() { wait <- cmd.Wait() }()
				select {
				case err = <-wait:
				case <-time.After(15 * time.Second):
					_ = cmd.Process.Kill()
					<-wait
					t.Fatal("shutdown timed out")
				}
				<-done
				if cmd.ProcessState.ExitCode() != 128+int(sig) {
					t.Fatalf("exit=%d err=%v stderr=%s transcript=%s", cmd.ProcessState.ExitCode(), err, stderr.String(), transcript.String())
				}
				log, readErr := os.ReadFile(filepath.Join(output, "log.txt"))
				if readErr != nil || len(log) == 0 {
					t.Fatal("log lost:", readErr)
				}
				if strings.Contains(transcript.String(), "UNREACHABLE_AFTER_SIGNAL") {
					t.Fatal("script continued after interruption")
				}
				m, v, readErr := tabular.Read(filepath.Join(output, "table"))
				if readErr != nil || m.Rows < 1 || len(v) != len(m.Columns) {
					t.Fatalf("table lost: %+v %v", m, readErr)
				}
				assertTextMatchesStructured(t, output, m, v)
				attrs, readErr := os.ReadFile(filepath.Join(output, "run.json"))
				if readErr != nil {
					t.Fatal(readErr)
				}
				var meta map[string]any
				if err = json.Unmarshal(attrs, &meta); err != nil {
					t.Fatal(err)
				}
				if meta["completion"] != "interrupted" || meta["binary_sha256"] == nil {
					t.Fatalf("missing status/provenance: %v", meta)
				}
			})
		}
	}
}

func assertTextMatchesStructured(t *testing.T, dir string, m tabular.Manifest, values [][]float64) {
	t.Helper()
	text, err := os.ReadFile(filepath.Join(dir, "table.txt"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasSuffix(string(text), "\n") {
		t.Fatal("partial final text row")
	}
	lines := strings.Split(strings.TrimSuffix(string(text), "\n"), "\n")
	if len(lines) != m.Rows+1 {
		t.Fatalf("text/structured rows differ: text=%d structured=%d", len(lines)-1, m.Rows)
	}
	for i, line := range lines[1:] {
		fields := strings.Fields(line)
		if len(fields) != len(m.Columns) {
			t.Fatalf("row %d is empty or incomplete: %q", i, line)
		}
		for c, s := range fields {
			v, err := strconv.ParseFloat(s, 64)
			if err != nil {
				t.Fatal(err)
			}
			want := values[c][i]
			if c > 0 {
				want = float64(float32(want))
			}
			if math.IsNaN(want) {
				if !math.IsNaN(v) {
					t.Fatalf("row %d column %s lost NaN status", i, m.Columns[c])
				}
				continue
			}
			if c == 0 {
				if v != want {
					t.Fatalf("time mismatch: %g != %g", v, want)
				}
			} else if math.Abs(v-want) > math.Max(math.Abs(want)*1e-7, math.SmallestNonzeroFloat32) {
				t.Fatalf("row %d column %s differs: %g != %g", i, m.Columns[c], v, want)
			}
		}
	}
}

func TestRuntimeGoInputSignal(t *testing.T) {
	binary := os.Getenv("MUMAX_RUNTIME_BINARY")
	if binary == "" {
		t.Skip("set MUMAX_RUNTIME_BINARY for actual GPU tests")
	}
	dir := t.TempDir()
	input := filepath.Join(dir, "simulation.go")
	output := filepath.Join(dir, "result.zarr")
	marker := filepath.Join(dir, "unexpected")
	code := fmt.Sprintf(`package main
import ("os"; "github.com/mumax/3/engine")
func main(){
 defer engine.InitAndClose()()
 engine.SetGridSize(32,32,1)
 engine.SetCellSize(2e-9,2e-9,2e-9)
 engine.Msat.Set(800e3);engine.Aex.Set(13e-12)
 engine.M.Set(engine.Uniform(1,0,0))
 engine.FixDt=1e-12
 engine.TableAutoSave(1e-12);engine.TableSave()
 engine.Run(1000)
 _ = os.WriteFile(%q,[]byte("unexpected"),0600)
}`, marker)
	if err := os.WriteFile(input, []byte(code), 0600); err != nil {
		t.Fatal(err)
	}
	log, err := os.Create(filepath.Join(dir, "process.txt"))
	if err != nil {
		t.Fatal(err)
	}
	defer log.Close()
	cmd := exec.Command(binary, "-webui-disable", "-check-updates=false", "-hide-progress-bar", "-o", output, input)
	cmd.Stdout = log
	cmd.Stderr = log
	if err = cmd.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = cmd.Process.Kill() })
	deadline := time.Now().Add(40 * time.Second)
	for {
		if m, _, err := tabular.Read(filepath.Join(output, "table")); err == nil && m.Rows > 0 {
			break
		}
		if time.Now().After(deadline) {
			_ = cmd.Process.Kill()
			_ = cmd.Wait()
			b, _ := os.ReadFile(log.Name())
			t.Fatalf("Go worker never became ready: %s", b)
		}
		time.Sleep(20 * time.Millisecond)
	}
	if err = cmd.Process.Signal(syscall.SIGTERM); err != nil {
		t.Fatal(err)
	}
	done := make(chan error, 1)
	go func() { done <- cmd.Wait() }()
	select {
	case <-done:
	case <-time.After(15 * time.Second):
		_ = cmd.Process.Kill()
		<-done
		t.Fatal("Go worker shutdown timed out")
	}
	if cmd.ProcessState.ExitCode() != 143 {
		b, _ := os.ReadFile(log.Name())
		t.Fatalf("Go worker exit=%d: %s", cmd.ProcessState.ExitCode(), b)
	}
	if _, err = os.Stat(marker); !os.IsNotExist(err) {
		t.Fatal("Go code continued after interruption")
	}
	m, values, err := tabular.Read(filepath.Join(output, "table"))
	if err != nil || m.Rows < 1 {
		t.Fatalf("Go table lost: %+v %v", m, err)
	}
	assertTextMatchesStructured(t, output, m, values)
	meta, err := os.ReadFile(filepath.Join(output, "run.json"))
	if err != nil || !bytes.Contains(meta, []byte(`"completion": "interrupted"`)) {
		t.Fatalf("Go termination metadata: %s %v", meta, err)
	}
}

func TestRuntimeStorageFormatsAndFlush(t *testing.T) {
	binary := os.Getenv("MUMAX_RUNTIME_BINARY")
	if binary == "" {
		t.Skip("set MUMAX_RUNTIME_BINARY for actual GPU tests")
	}
	for _, format := range []string{"zarr", "hdf5", "ovf"} {
		t.Run(format, func(t *testing.T) {
			dir := t.TempDir()
			input := filepath.Join(dir, "format.mx3")
			output := filepath.Join(dir, "result")
			code := `SetGridSize(16,16,1)
SetCellSize(2e-9,2e-9,2e-9)
Msat=800e3
Aex=13e-12
m=Uniform(1,0,0)
precise:=0.123456789012345
TableAddVar(precise,"precise","m")
TableSave()
Save(m)
Flush()
t=1e-9
m=Uniform(0,1,0)
precise=0.987654321098765
TableSave()
Save(m)
Flush()
Flush()
`
			if err := os.WriteFile(input, []byte(code), 0600); err != nil {
				t.Fatal(err)
			}
			cmd := exec.Command(binary, "-webui-disable", "-check-updates=false", "-storage-format="+format, "-o", output, input)
			if data, err := cmd.CombinedOutput(); err != nil {
				t.Fatalf("format %s: %v\n%s", format, err, data)
			}
			if format != "ovf" {
				m, v, err := tabular.Read(filepath.Join(output, "table"))
				if err != nil {
					t.Fatal(err)
				}
				if m.Rows != 2 || v[4][0] != 0.123456789012345 || v[4][1] != 0.987654321098765 {
					t.Fatalf("double precision or rows lost: %v", v)
				}
				assertTextMatchesStructured(t, output, m, v)
			} else {
				b, err := os.ReadFile(filepath.Join(output, "table.txt"))
				if err != nil || strings.Count(string(b), "\n") != 3 || !strings.Contains(string(b), "0.12345679") {
					t.Fatalf("OVF text table lost: %q %v", b, err)
				}
			}
			b, err := os.ReadFile(filepath.Join(output, "run.json"))
			if err != nil {
				t.Fatal(err)
			}
			var meta map[string]any
			if err = json.Unmarshal(b, &meta); err != nil {
				t.Fatal(err)
			}
			if meta["completion"] != "complete" || meta["binary_sha256"] == nil || meta["input_sha256"] == nil {
				t.Fatalf("missing final status/provenance: %s", b)
			}
		})
	}
}

func TestRuntimeQueueSignalPersistsWorker(t *testing.T) {
	binary := os.Getenv("MUMAX_RUNTIME_BINARY")
	if binary == "" {
		t.Skip("set MUMAX_RUNTIME_BINARY for actual GPU tests")
	}
	for _, sig := range []syscall.Signal{syscall.SIGINT, syscall.SIGTERM} {
		t.Run(fmt.Sprint(sig), func(t *testing.T) {
			dir := t.TempDir()
			first, second := filepath.Join(dir, "first.mx3"), filepath.Join(dir, "second.mx3")
			code := []byte("SetGridSize(32,32,1)\nSetCellSize(2e-9,2e-9,2e-9)\nMsat=800e3\nAex=13e-12\nm=Uniform(1,0,0)\nFixDt=1e-12\nTableAutoSave(1e-12)\nRun(1000)\n")
			for _, p := range []string{first, second} {
				if err := os.WriteFile(p, code, 0600); err != nil {
					t.Fatal(err)
				}
			}
			log, err := os.Create(filepath.Join(dir, "queue.txt"))
			if err != nil {
				t.Fatal(err)
			}
			defer log.Close()
			cmd := exec.Command(binary, "-webui-disable", "-check-updates=false", "-hide-progress-bar", "-max_gpus=1", first, second)
			cmd.Stdout = log
			cmd.Stderr = log
			if err = cmd.Start(); err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { _ = cmd.Process.Kill() })
			output := strings.TrimSuffix(first, ".mx3") + ".zarr"
			deadline := time.Now().Add(20 * time.Second)
			for {
				if m, _, err := tabular.Read(filepath.Join(output, "table")); err == nil && m.Rows > 0 {
					break
				}
				if time.Now().After(deadline) {
					_ = cmd.Process.Kill()
					_ = cmd.Wait()
					b, _ := os.ReadFile(log.Name())
					t.Fatalf("queue worker never became ready: %s", b)
				}
				time.Sleep(20 * time.Millisecond)
			}
			if err = cmd.Process.Signal(sig); err != nil {
				t.Fatal(err)
			}
			done := make(chan error, 1)
			go func() { done <- cmd.Wait() }()
			select {
			case <-done:
			case <-time.After(15 * time.Second):
				_ = cmd.Process.Kill()
				<-done
				t.Fatal("queue shutdown timed out")
			}
			if cmd.ProcessState.ExitCode() != 128+int(sig) {
				b, _ := os.ReadFile(log.Name())
				t.Fatalf("queue exit %d: %s", cmd.ProcessState.ExitCode(), b)
			}
			m, v, err := tabular.Read(filepath.Join(output, "table"))
			if err != nil {
				t.Fatal(err)
			}
			assertTextMatchesStructured(t, output, m, v)
			if _, err = os.Stat(strings.TrimSuffix(second, ".mx3") + ".zarr"); !os.IsNotExist(err) {
				t.Fatal("queued job started during shutdown")
			}
		})
	}
}
