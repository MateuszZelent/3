package engine

// Management of output directory.

import (
	"github.com/mumax/3/events"
	"github.com/mumax/3/httpfs"
	"github.com/mumax/3/util"
	"os"
	"strings"
)

var (
	outputdir         string // Output directory
	InputFile         string
	outputOwned       bool // only the accepted output directory may be finalized
	outputLockRelease func() error
)

func OD() string {
	if outputdir == "" {
		panic("output not yet initialized")
	}
	return outputdir
}

// SetOD sets the output directory where auto-saved files will be stored.
// The -o flag can also be used for this purpose.
func InitIO(inputfile, od string, force bool) {
	if outputdir != "" {
		panic("output directory already set")
	}

	InputFile = inputfile
	if !strings.HasSuffix(od, "/") {
		od += "/"
	}
	outputdir = od
	if strings.HasPrefix(outputdir, "http://") {
		httpfs.SetWD(outputdir + "/../")
	}
	LogOut("output directory:", outputdir)

	if *Flag_skipexist && !strings.HasPrefix(od, "http://") {
		if _, err := os.Stat(strings.TrimSuffix(od, "/")); err == nil {
			reason := "output directory already exists; skipping due to -skip-exist: " + outputdir
			LogOut(reason)
			_ = events.Emit(events.Event{Event: "worker_skipped", Error: reason})
			os.Exit(0)
		} else if !os.IsNotExist(err) {
			util.FatalErr(err)
		}
	}
	var lockErr error
	outputLockRelease, lockErr = httpfs.AcquireOutputLock(od)
	CheckRecoverable(lockErr)
	defer func() {
		if !outputOwned && outputLockRelease != nil {
			_ = outputLockRelease()
			outputLockRelease = nil
		}
	}()

	if force {
		CheckRecoverable(httpfs.Remove(od))
	} else if entries, err := httpfs.ReadDir(od); err == nil {
		for _, entry := range entries {
			// The legacy server creates its capture stream before starting the
			// worker. It is retained and is never one of our output writers.
			if entry != "stdout.txt" {
				panic(UserErr("output directory is not empty; choose a new -o directory, use -skip-exist, or explicitly use -f to replace it: " + od))
			}
		}
	}

	if err := httpfs.Mkdir(od); err != nil {
		if _, readErr := httpfs.ReadDir(od); readErr != nil {
			CheckRecoverable(err)
		}
	}
	outputOwned = true
	initStructuredOutput()

	initLog()
	initBib()
	startOutputFlusher()
}
