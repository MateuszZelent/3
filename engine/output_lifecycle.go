package engine

import (
	"errors"
	"fmt"
	"os"
	"sync"
	"time"

	"github.com/mumax/3/httpfs"
	"github.com/mumax/3/timer"
)

// Only the engine owner closes output. The background flusher touches CPU
// buffers and never evaluates a quantity or calls CUDA.
type outputLifecycleState struct {
	mu        sync.Mutex
	flushMu   sync.Mutex
	stop      chan struct{}
	done      chan struct{}
	err       error
	closed    bool
	closeOnce sync.Once
	runErr    error
}

var outputLifecycle outputLifecycleState

func RecordRunError(err error) {
	outputLifecycle.mu.Lock()
	defer outputLifecycle.mu.Unlock()
	if outputLifecycle.runErr == nil {
		outputLifecycle.runErr = err
	}
}
func runFailed() bool {
	outputLifecycle.mu.Lock()
	defer outputLifecycle.mu.Unlock()
	return outputLifecycle.runErr != nil
}

func recordOutputError(err error) {
	if err == nil {
		return
	}
	outputLifecycle.mu.Lock()
	defer outputLifecycle.mu.Unlock()
	// Keep the first failure, rather than growing memory on every timer tick.
	if outputLifecycle.err == nil {
		outputLifecycle.err = err
		fmt.Fprintln(os.Stderr, "output:", err)
	}
}

func appendFinalOutputError(err error) {
	if err == nil {
		return
	}
	outputLifecycle.mu.Lock()
	defer outputLifecycle.mu.Unlock()
	outputLifecycle.err = errors.Join(outputLifecycle.err, err)
	fmt.Fprintln(os.Stderr, "output finalization:", err)
}

func OutputError() error {
	outputLifecycle.mu.Lock()
	defer outputLifecycle.mu.Unlock()
	return outputLifecycle.err
}

func captureOutputPanic() {
	if p := recover(); p != nil {
		recordOutputError(fmt.Errorf("asynchronous output: %v", p))
	}
}

func outputAttempt(name string, f func() error) (err error) {
	defer func() {
		if p := recover(); p != nil {
			err = fmt.Errorf("%s: %v", name, p)
		}
	}()
	if err = f(); err != nil {
		return fmt.Errorf("%s: %w", name, err)
	}
	return nil
}

func startOutputFlusher() {
	outputLifecycle.mu.Lock()
	if outputLifecycle.stop != nil || outputLifecycle.closed {
		outputLifecycle.mu.Unlock()
		return
	}
	stop, done := make(chan struct{}), make(chan struct{})
	outputLifecycle.stop = stop
	outputLifecycle.done = done
	outputLifecycle.mu.Unlock()
	go func() {
		defer close(done)
		tick := time.NewTicker(TableAutoflushRate * time.Second)
		defer tick.Stop()
		for {
			select {
			case <-stop:
				return
			case <-tick.C:
				recordOutputError(flushBufferedOutput())
			}
		}
	}()
}

func flushBufferedOutput() error {
	outputLifecycle.flushMu.Lock()
	defer outputLifecycle.flushMu.Unlock()
	return errors.Join(
		outputAttempt("table text", Table.Flush),
		outputAttempt("structured table", tableHistory.flushStructured),
		outputAttempt("log", flushLog),
		outputAttempt("HDF5 fields", flushHDF5Output),
		outputAttempt("bibliography", func() error {
			if bibfile != nil {
				return httpfs.Sync(bibfile)
			}
			return nil
		}),
	)
}

// FlushOutputs persists existing samples without creating a new sample or
// closing a writer. It is safe to call repeatedly when no new samples arrived.
func FlushOutputs() error {
	drainOutput()
	return errors.Join(OutputError(), flushBufferedOutput(), outputAttempt("run metadata", func() error { return flushRunMetadata(true) }))
}

func Flush() { CheckRecoverable(FlushOutputs()) }

// CloseWithError attempts every independent output even if another fails. The
// stored result also survives subsequent calls and partial initialization.
func CloseWithError() error {
	outputLifecycle.closeOnce.Do(func() {
		outputLifecycle.mu.Lock()
		stop, done := outputLifecycle.stop, outputLifecycle.done
		if stop != nil {
			close(stop)
		}
		outputLifecycle.mu.Unlock()
		if done != nil {
			<-done
		}
		drainOutput()
		result := errors.Join(
			outputAttempt("references log", func() error { LogUsedRefs(); return nil }),
			flushBufferedOutput(),
			outputAttempt("structured backend", closeStructuredOutput),
			outputAttempt("table close", Table.close),
			outputAttempt("log close", closeLog),
			outputAttempt("bibliography close", func() error {
				if bibfile != nil {
					return bibfile.Close()
				}
				return nil
			}),
		)
		if *Flag_sync {
			timer.Print(os.Stdout)
		}
		appendFinalOutputError(result)
		appendFinalOutputError(outputAttempt("run metadata", finalizeStructuredMetadata))
		if outputLockRelease != nil {
			appendFinalOutputError(outputAttempt("output lock", outputLockRelease))
			outputLockRelease = nil
		}
		outputLifecycle.mu.Lock()
		outputLifecycle.closed = true
		outputLifecycle.mu.Unlock()
	})
	return OutputError()
}
