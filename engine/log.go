package engine

import (
	"fmt"
	"io"
	"os"
	"strings"
	"sync"
	"unicode/utf8"

	"github.com/mumax/3/httpfs"
)

const (
	consoleHistoryLimit            = 256 * 1024
	consoleHistoryTruncationMarker = "// [console history truncated]"
)

var (
	logMu        sync.Mutex
	logClosed    bool
	hist         string         // bounded console history for GUI
	logBootstrap []string       // complete GUI-formatted output accumulated before log file creation
	logfile      io.WriteCloser // saves history of input commands +  output
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
	_, err = fmt.Fprintln(logfile, strings.Join(logBootstrap, "\n"))
	CheckRecoverable(err)
	CheckRecoverable(httpfs.Sync(logfile))
	logBootstrap = nil
}

func log2GUI(msg string) {
	bootstrapMessage := msg
	if len(bootstrapMessage) > 1000 {
		bootstrapMessage = bootstrapMessage[:1000-len("...")] + "..."
	}
	if logfile == nil && !logClosed {
		logBootstrap = append(logBootstrap, bootstrapMessage)
	}
	msg = strings.ToValidUTF8(msg, "\uFFFD")
	if len(msg) > 1000 {
		cut := 1000 - len("...")
		for cut > 0 && !utf8.RuneStart(msg[cut]) {
			cut--
		}
		msg = msg[:cut] + "..."
	}
	hist = appendConsoleHistory(hist, msg)
	// TODO: push to web ?
}

func appendConsoleHistory(history, msg string) string {
	if history != "" {
		history += "\n"
	}
	history += msg
	if len(history) <= consoleHistoryLimit {
		return history
	}
	marker := consoleHistoryTruncationMarker + "\n"
	remaining := consoleHistoryLimit - len(marker)
	tail := history[len(history)-remaining:]
	for len(tail) > 0 && !utf8.RuneStart(tail[0]) {
		tail = tail[1:]
	}
	return marker + tail
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
