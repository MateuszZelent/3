package engine

import (
	"encoding/json"
	"errors"
	"math"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/mumax/3/tabular"
)

func isolateOutput(t *testing.T) {
	t.Helper()
	oldTable, oldHistory, oldLifecycle := Table, tableHistory, outputLifecycle
	oldDir, oldFormat, oldTime := outputdir, StorageFormat, Time
	oldOwned := outputOwned
	oldStructured := structuredOutput
	logMu.Lock()
	oldLog, oldHist, oldClosed := logfile, hist, logClosed
	logfile = nil
	hist = "initial audit log"
	logClosed = false
	logMu.Unlock()
	Table = *newTable("table")
	tableHistory = tableHistoryState{data: make(map[string][]float64)}
	outputLifecycle = outputLifecycleState{}
	outputdir = t.TempDir() + "/"
	outputOwned = true
	StorageFormat = StorageFormatZarr
	Time = 0
	structuredOutput = structuredOutputState{metadata: make(map[string]any), datasets: make(map[string]*structuredDataset), started: time.Now()}
	t.Cleanup(func() {
		_ = CloseWithError()
		Table, tableHistory, outputLifecycle = oldTable, oldHistory, oldLifecycle
		outputdir, StorageFormat, Time = oldDir, oldFormat, oldTime
		outputOwned = oldOwned
		structuredOutput = oldStructured
		logMu.Lock()
		logfile, hist, logClosed = oldLog, oldHist, oldClosed
		logMu.Unlock()
	})
}

func TestOutputPersistsBeforeCloseAndFlushIsIdempotent(t *testing.T) {
	isolateOutput(t)
	initStructuredOutput()
	initLog()
	b, err := os.ReadFile(OD() + "log.txt")
	if err != nil || !strings.Contains(string(b), "initial audit log") {
		t.Fatalf("initial log not flushed: %q %v", b, err)
	}
	TableSave()
	m, _, err := tabular.Read(OD() + "table")
	if err != nil || m.Rows != 1 {
		t.Fatalf("first sample not committed: %+v %v", m, err)
	}
	Time = 1
	TableSave()
	LogOut("pending log")
	if err = FlushOutputs(); err != nil {
		t.Fatal(err)
	}
	if err = FlushOutputs(); err != nil {
		t.Fatal(err)
	}
	m, v, err := tabular.Read(OD() + "table")
	if err != nil || m.Rows != 2 || v[0][1] != 1 {
		t.Fatalf("flush duplicated/lost samples: %+v %v", m, err)
	}
	b, _ = os.ReadFile(OD() + "table.txt")
	if string(b) != "# t (s)\n0\n1\n" {
		t.Fatalf("text table %q", b)
	}
	b, _ = os.ReadFile(OD() + "log.txt")
	if !strings.Contains(string(b), "pending log") {
		t.Fatal("log omitted by Flush")
	}
	if err = CloseWithError(); err != nil {
		t.Fatal(err)
	}
	if err = CloseWithError(); err != nil {
		t.Fatal("second close:", err)
	}
}

func TestOutputWallClockFlushRunsWithoutSolverOrUI(t *testing.T) {
	isolateOutput(t)
	initStructuredOutput()
	initLog()
	TableSave()
	Time = 1
	TableSave()
	LogOut("wall-clock flush")
	startOutputFlusher()
	deadline := time.Now().Add(4 * time.Second)
	for time.Now().Before(deadline) {
		m, _, err := tabular.Read(OD() + "table")
		b, _ := os.ReadFile(OD() + "log.txt")
		if err == nil && m.Rows == 2 && strings.Contains(string(b), "wall-clock flush") {
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatal("background flush did not publish pending buffers")
}

type failingOutput struct{ closed bool }

func (w *failingOutput) Write(p []byte) (int, error) { return len(p), nil }
func (w *failingOutput) Flush() error                { return errors.New("injected text flush error") }
func (w *failingOutput) Close() error                { w.closed = true; return errors.New("injected close error") }

func TestCloseAttemptsEveryOutputAfterFailures(t *testing.T) {
	isolateOutput(t)
	text, log := &failingOutput{}, &failingOutput{}
	Table.output = text
	logfile = log
	structuredOutput.metadata["bad"] = math.NaN()
	err := CloseWithError()
	if err == nil || !strings.Contains(err.Error(), "NaN") || !strings.Contains(err.Error(), "text flush") || !text.closed || !log.closed {
		t.Fatalf("incomplete finalization: %v text=%v log=%v", err, text.closed, log.closed)
	}
	if again := CloseWithError(); again != err {
		t.Fatal("finalization error was not retained")
	}
}

func TestHistoryMemoryAndGlobalSamplingAreBounded(t *testing.T) {
	isolateOutput(t)
	StorageFormat = StorageFormatOVF
	for i := 0; i <= 2*tablePreviewRows; i++ {
		recordTableHistory([]string{"t"}, []string{"s"}, []float64{float64(i)})
	}
	if len(tableHistory.data["t"]) > tablePreviewRows || tableHistory.offset == 0 {
		t.Fatal("history was not compacted")
	}
	_, _, data := TablePreviewSnapshot([]string{"t"}, 2, 3)
	for _, v := range data["t"] {
		if int(v)%3 != 0 {
			t.Fatal("sampling lost global alignment:", v)
		}
	}
}

func TestAsyncFailureUnwindsInsteadOfKillingProcess(t *testing.T) {
	isolateOutput(t)
	queOutput(func() { panic("injected async failure") })
	drainOutput()
	if err := OutputError(); err == nil || !strings.Contains(err.Error(), "injected async failure") {
		t.Fatal("missing asynchronous error:", err)
	}
}

func TestRejectedDirectoryRemainsUntouchedByCleanup(t *testing.T) {
	isolateOutput(t)
	dir := outputdir
	outputdir = ""
	outputOwned = false
	previous := []byte(`{"keep":"existing simulation"}`)
	if err := os.WriteFile(dir+".zattrs", previous, 0600); err != nil {
		t.Fatal(err)
	}
	func() {
		defer func() {
			if p := recover(); p == nil {
				t.Error("existing directory accepted")
			}
		}()
		InitIO("new.mx3", dir, false)
	}()
	if err := CloseWithError(); err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile(dir + ".zattrs")
	if err != nil || string(got) != string(previous) {
		t.Fatalf("cleanup altered rejected output: %q %v", got, err)
	}
}

func TestFinalMetadataCannotReportSuccessAfterWriteFailure(t *testing.T) {
	isolateOutput(t)
	Table.output = &failingOutput{}
	if err := CloseWithError(); err == nil {
		t.Fatal("write failure was swallowed")
	}
	b, err := os.ReadFile(OD() + "run.json")
	if err != nil {
		t.Fatal(err)
	}
	var metadata map[string]any
	if err = json.Unmarshal(b, &metadata); err != nil {
		t.Fatal(err)
	}
	if metadata["completion"] != "failed" {
		t.Fatalf("false success metadata: %s", b)
	}
}

func TestFullOVFSnapshotSurvivesPreviewCompaction(t *testing.T) {
	isolateOutput(t)
	StorageFormat = StorageFormatOVF
	for i := 0; i <= 2*tablePreviewRows; i++ {
		Time = float64(i)
		TableSave()
	}
	_, _, values := TableHistorySnapshot()
	if len(values["t"]) != 2*tablePreviewRows+1 || values["t"][0] != 0 || values["t"][2*tablePreviewRows] != 2*tablePreviewRows {
		t.Fatalf("full history mismatch: rows=%d expected=%d preview_offset=%d format=%v", len(values["t"]), 2*tablePreviewRows+1, tableHistory.offset, tableHistory.format)
	}
}

func TestBreakPersistsPendingSamplesBeforeReturning(t *testing.T) {
	isolateOutput(t)
	previousPause := pause
	defer func() { pause = previousPause }()
	initStructuredOutput()
	initLog()
	TableSave()
	Time = 1
	TableSave()
	LogOut("pending before Break")
	done := make(chan struct{})
	go func() { Break(); close(done) }()
	select {
	case task := <-Inject:
		task()
	case <-time.After(time.Second):
		t.Fatal("Break did not reach the engine owner")
	}
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("Break did not return")
	}
	m, _, err := tabular.Read(OD() + "table")
	if err != nil || m.Rows != 2 || !pause {
		t.Fatalf("Break lost pending table: %+v %v", m, err)
	}
	b, err := os.ReadFile(OD() + "log.txt")
	if err != nil || !strings.Contains(string(b), "pending before Break") {
		t.Fatalf("Break lost log: %q %v", b, err)
	}
}
