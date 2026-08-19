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
	outputdir string // Output directory
	InputFile string
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

	if force {
		httpfs.Remove(od)
	}

	_ = httpfs.Mkdir(od)
	initStructuredOutput()

	initLog()
	initBib()
}
