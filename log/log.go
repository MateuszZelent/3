// Package log provides the small formatted logger surface used by the Amumax
// web backend while the simulation engine keeps its native logging system.
package log

import (
	"fmt"
	stdliblog "log"
)

type Logger struct {
	debug bool
	Hist  string
}

var Log = &Logger{}

func (l *Logger) SetDebug(enabled bool) { l.debug = enabled }

func (l *Logger) write(prefix, format string, args ...interface{}) {
	message := fmt.Sprintf(format, args...)
	if l.Hist != "" {
		l.Hist += "\n"
	}
	l.Hist += prefix + message
	stdliblog.Print(prefix + message)
}

func (l *Logger) Debug(format string, args ...interface{}) {
	if l.debug {
		l.write("DEBUG: ", format, args...)
	}
}
func (l *Logger) Info(format string, args ...interface{}) { l.write("", format, args...) }
func (l *Logger) Warn(format string, args ...interface{}) { l.write("WARNING: ", format, args...) }
func (l *Logger) Err(format string, args ...interface{})  { l.write("ERROR: ", format, args...) }
func (l *Logger) ErrAndExit(format string, args ...interface{}) {
	panic(fmt.Errorf(format, args...))
}
func (l *Logger) PanicIfError(err error) {
	if err != nil {
		panic(err)
	}
}
