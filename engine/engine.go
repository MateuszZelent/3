/*
engine does the simulation bookkeeping, I/O and GUI.

space-dependence:
value: space-independent
param: region-dependent parameter (always input)
field: fully space-dependent field

TODO: godoc everything
*/
package engine

import (
	"fmt"
	"runtime"
	"sync"
	"time"

	"github.com/mumax/3/cuda/cu"
)

const VERSION = "mumax 3.12"

var UNAME = fmt.Sprintf("%s [%s_%s %s(%s) CUDA-%d.%d]",
	VERSION, runtime.GOOS, runtime.GOARCH, runtime.Version(), runtime.Compiler,
	cu.CUDA_VERSION/1000, (cu.CUDA_VERSION%1000)/10)

var StartTime = time.Now()

var (
	busyLock sync.Mutex
	busy     bool // are we so busy we can't respond from run loop? (e.g. calc kernel)
)

// We set SetBusy(true) when the simulation is too busy to accept GUI input on Inject channel.
// E.g. during kernel init.
func SetBusy(b bool) {
	busyLock.Lock()
	defer busyLock.Unlock()
	busy = b
}

func GetBusy() bool {
	busyLock.Lock()
	defer busyLock.Unlock()
	return busy
}

// Close flushes and closes every output. CLI callers should use CloseWithError
// to preserve the primary error and report cleanup failures with a nonzero exit.
func Close() {
	if err := CloseWithError(); err != nil {
		panic(err)
	}
}
