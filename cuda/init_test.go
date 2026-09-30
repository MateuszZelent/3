package cuda

import (
	"github.com/mumax/3/cuda/cu"
	"runtime"
	"testing"
)

var testContext cu.Context

// needed for all other tests.
func init() {
	cu.Init(0)
	testContext = cu.CtxCreate(cu.CTX_SCHED_AUTO, 0)
	cu.CtxSetCurrent(testContext)
}

// Go runs individual tests in new goroutines, which can migrate across native
// threads. CUDA driver contexts must be bound to each test's locked OS thread.
func bindTestContext(t testing.TB) {
	runtime.LockOSThread()
	cu.CtxSetCurrent(testContext)
	t.Cleanup(runtime.UnlockOSThread)
}
