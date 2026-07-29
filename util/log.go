package util

// Logging and error reporting utility functions

import (
	"fmt"
	"io"
	"log"
	"os"
	"runtime"
	"strings"
	"sync"
	"time"

	"golang.org/x/term"
)

func Fatal(msg ...interface{}) {
	log.Fatal(msg...)
}

func Fatalf(format string, msg ...interface{}) {
	log.Fatalf(format, msg...)
}

// If err != nil, trigger log.Fatal(msg, err)
func FatalErr(err interface{}) {
	_, file, line, _ := runtime.Caller(1)
	if err != nil {
		log.Fatal(file, ":", line, err)
	}
}

// Panics if err is not nil. Signals a bug.
func PanicErr(err error) {
	if err != nil {
		log.Panic(err)
	}
}

// Logs the error of non-nil, plus message
func LogErr(err error, msg ...interface{}) {
	if err != nil {
		log.Println(append(msg, err)...)
	}
}

func Log(msg ...interface{}) {
	log.Println(msg...)
}

// Panics with "illegal argument" if test is false.
func Argument(test bool) {
	if !test {
		log.Panic("illegal argument")
	}
}

// Panics with msg if test is false
func AssertMsg(test bool, msg interface{}) {
	if !test {
		log.Panic(msg)
	}
}

// Panics with "assertion failed" if test is false.
func Assert(test bool) {
	if !test {
		log.Panic("assertion failed")
	}
}

// Hack to avoid cyclic dependency on engine.
var (
	progress_      func(int, int, string) = TerminalProgress
	progLock       sync.Mutex
	progressHidden bool
	progressOut    io.Writer = os.Stdout
	progressWidth            = func() int {
		if width, _, err := term.GetSize(int(os.Stdout.Fd())); err == nil && width > 0 {
			return width
		}
		return 80
	}
	progressColor = func() bool { return term.IsTerminal(int(os.Stdout.Fd())) }
)

// Set progress bar to progress/total and display msg
// if GUI is up and running.
func Progress(progress, total int, msg string) {
	progLock.Lock()
	defer progLock.Unlock()
	if progressHidden {
		return
	}
	if progress_ != nil {
		progress_(progress, total, msg)
	}
}

var (
	lastPct     = -1      // last progress percentage shown
	lastProgT   time.Time // last time we showed progress percentage
	lastProgMsg string
)

func TerminalProgress(prog, total int, msg string) {
	if total <= 0 {
		return
	}
	now := time.Now()
	pct := prog * 100 / total
	if pct < 0 {
		pct = 0
	}
	if pct > 100 {
		pct = 100
	}
	if pct == lastPct && msg == lastProgMsg {
		return
	}
	if now.Sub(lastProgT) < 100*time.Millisecond && pct != 0 && pct != 100 {
		return
	}
	width := progressWidth()
	if width < 10 {
		return
	}
	fixed := len(msg) + len(" [] 100%")
	barWidth := width - fixed
	if barWidth < 1 {
		barWidth = 1
	}
	filled := barWidth * pct / 100
	colorStart, colorEnd := "", ""
	if progressColor() {
		colorStart, colorEnd = "\033[32m", "\033[0m"
	}
	fmt.Fprintf(progressOut, "\r\033[K%s%s [%s%s] %3d%%%s", colorStart, msg, strings.Repeat("⣿", filled), strings.Repeat(" ", barWidth-filled), pct, colorEnd)
	if pct == 100 {
		fmt.Fprintln(progressOut)
	}
	lastPct, lastProgMsg, lastProgT = pct, msg, now
}

func FinishProgress(msg string) {
	Progress(1, 1, msg)
}

func SetProgressHidden(hidden bool) {
	progLock.Lock()
	defer progLock.Unlock()
	progressHidden = hidden
	if hidden {
		lastPct, lastProgMsg = -1, ""
	}
}

func PrintProgress(prog, total int, msg string) {
	pct := (prog * 100) / total
	if pct != lastPct { // only print percentage if changed
		if (time.Since(lastProgT) > time.Second) || pct == 100 || prog == 0 { // only print percentage once/second unless finished
			fmt.Println("//", msg, pct, "%")
			lastPct = pct
			lastProgT = time.Now()
		}
	}
}

// Sets the function to be used internally by Progress.
// Avoids cyclic dependency on engine.
func SetProgress(f func(int, int, string)) {
	progLock.Lock()
	defer progLock.Unlock()
	progress_ = f
}
