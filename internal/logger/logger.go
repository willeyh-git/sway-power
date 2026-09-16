// Package logger provides a console logger gated by a debug flag.
package logger

import (
	"io"
	"log"
	"os"
)

// Logger prints formatted messages to the console (stderr).
// When created with debug disabled, all output is discarded.
type Logger struct {
	log *log.Logger
}

// New returns a Logger that prints to stderr with the given prefix
// when debug is true, and discards all output otherwise.
func New(debug bool, prefix string) *Logger {
	out := io.Discard
	if debug {
		out = os.Stderr
	}
	return &Logger{log: log.New(out, prefix, 0)}
}

// Printf logs a formatted message (discarded when debug is off).
func (lg *Logger) Printf(format string, args ...any) {
	lg.log.Printf(format, args...)
}
