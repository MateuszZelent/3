package main

import (
	"fmt"
	"os"
	"runtime/debug"

	"github.com/mumax/3/engine"
)

type exitCode int

var stopSimulationSignals func()

func main() { os.Exit(runWithCleanup(runCLI)) }

func runWithCleanup(run func()) (status int) {
	defer func() {
		if p := recover(); p != nil {
			switch e := p.(type) {
			case exitCode:
				status = int(e)
			case engine.Interrupted:
				status = e.ExitCode()
			default:
				status = 1
				engine.RecordRunError(fmt.Errorf("%v", p))
			}
			if _, ok := p.(exitCode); !ok {
				fmt.Fprintln(os.Stderr, p)
				func() { defer func() { _ = recover() }(); engine.LogErr(p) }()
				if _, ok := p.(error); !ok {
					fmt.Fprintln(os.Stderr, string(debug.Stack()))
				}
			}
		}
		if reason := engine.StopReason(); reason != nil {
			status = reason.ExitCode()
		}
		if err := engine.CloseWithError(); err != nil {
			fmt.Fprintln(os.Stderr, "output finalization:", err)
			if status == 0 {
				status = 1
			}
		}
		if stopSimulationSignals != nil {
			stopSimulationSignals()
		}
	}()
	run()
	return 0
}
