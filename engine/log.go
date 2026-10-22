package engine

import (
	"fmt"
	"github.com/mumax/3/httpfs"
	"io"
	"os"
	"sync"
)

var (
	logMu     sync.Mutex
	logClosed bool
	hist      string         // console history for GUI
	logfile   io.WriteCloser // saves history of input commands +  output
)

// Special error that is not fatal when paniced on and called from GUI
// E.g.: try to set bad grid size: panic on UserErr, recover, print error, carry on.
type UserErr string

func (e UserErr) Error() string { return string(e) }

func CheckRecoverable(err error) {
	if err != nil {
		panic(UserErr(err.Error()))
	}
}

func LogIn(msg ...interface{}) {
	logMu.Lock()
	defer logMu.Unlock()
	str := sprint(msg...)
	log2GUI(str)
	log2File(str)
	fmt.Println(str)
}

func LogOut(msg ...interface{}) {
	logMu.Lock()
	defer logMu.Unlock()
	str := "//" + sprint(msg...)
	log2GUI(str)
	log2File(str)
	fmt.Println(str)
}

func LogErr(msg ...interface{}) {
	logMu.Lock()
	defer logMu.Unlock()
	str := "//" + sprint(msg...)
	log2GUI(str)
	log2File(str)
	fprintln(os.Stderr, str)
}

func log2File(msg string) {
	if logfile != nil && !logClosed {
		fprintln(logfile, msg)
	}
}

func initLog() {
	logMu.Lock()
	defer logMu.Unlock()
	if logfile != nil {
		panic("log already inited")
	}
	// open log file and flush what was logged before the file existed
	var err error
	logfile, err = httpfs.Create(OD() + "log.txt")
	if err != nil {
		panic(err)
	}
	logClosed = false
	_, err = fmt.Fprintln(logfile, hist)
	CheckRecoverable(err)
	CheckRecoverable(httpfs.Sync(logfile))
}

func log2GUI(msg string) {
	if len(msg) > 1000 {
		msg = msg[:1000-len("...")] + "..."
	}
	if hist != "" { // prepend newline
		hist += "\n"
	}
	hist += msg
	// TODO: push to web ?
}

// like fmt.Sprint but with spaces between args
func sprint(msg ...interface{}) string {
	str := fmt.Sprintln(msg...)
	str = str[:len(str)-1] // strip newline
	return str
}

// LogReleaseNotice safely accepts the one-off background update notice, including
// before initLog. Close excludes late notifications without waiting for HTTP.
func LogReleaseNotice(msg string) {
	logMu.Lock()
	defer logMu.Unlock()
	if logClosed {
		return
	}
	str := "//" + msg
	log2GUI(str)
	log2File(str)
	fmt.Println(str)
}

func flushLog() error {
	logMu.Lock()
	defer logMu.Unlock()
	if logfile == nil || logClosed {
		return nil
	}
	return httpfs.Sync(logfile)
}

func closeLog() error {
	logMu.Lock()
	defer logMu.Unlock()
	if logClosed {
		return nil
	}
	logClosed = true
	if logfile == nil {
		return nil
	}
	err := logfile.Close()
	logfile = nil
	return err
}
