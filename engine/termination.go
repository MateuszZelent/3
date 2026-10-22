package engine

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"sync"
	"sync/atomic"
	"syscall"

	"github.com/mumax/3/script"
	"github.com/mumax/3/util"
)

type Interrupted struct{ Signal os.Signal }

func (e Interrupted) Error() string { return fmt.Sprintf("simulation interrupted by %s", e.Signal) }
func (e Interrupted) ExitCode() int {
	if sig, ok := e.Signal.(syscall.Signal); ok {
		return 128 + int(sig)
	}
	return 1
}

type terminationState struct {
	reason atomic.Pointer[Interrupted]
	once   sync.Once
	done   chan struct{}
	ctx    context.Context
	cancel context.CancelFunc
}

func newTerminationState() *terminationState {
	ctx, cancel := context.WithCancel(context.Background())
	return &terminationState{done: make(chan struct{}), ctx: ctx, cancel: cancel}
}

var termination = newTerminationState()

func init() {
	script.BeforeStatement = CheckStop
	util.FatalHandler = func(err error) { panic(util.FatalError{Err: err}) }
}

// RequestStop is nonblocking and can be called from a signal goroutine. The
// first reason wins; only the engine owner observes it and unwinds CUDA work.
func RequestStop(sig os.Signal) {
	termination.once.Do(func() { termination.reason.Store(&Interrupted{sig}); close(termination.done); termination.cancel() })
}

func SimulationContext() context.Context { return termination.ctx }

func StopRequested() <-chan struct{} { return termination.done }
func StopReason() *Interrupted       { return termination.reason.Load() }

func CheckStop() {
	if reason := StopReason(); reason != nil {
		panic(*reason)
	}
	if err := OutputError(); err != nil {
		panic(util.FatalError{Err: err})
	}
}

func InstallSignalHandlers() func() {
	signals := make(chan os.Signal, 2)
	signal.Notify(signals, os.Interrupt, syscall.SIGTERM)
	stop := make(chan struct{})
	go func() {
		for {
			select {
			case sig := <-signals:
				RequestStop(sig)
			case <-stop:
				return
			}
		}
	}()
	var once sync.Once
	return func() { once.Do(func() { signal.Stop(signals); close(stop) }) }
}
