package main

import (
	"io"
	"os"
)

// isTerminal reports whether w is a character device (a TTY). Pure stdlib — no
// isatty/x/term dependency.
func isTerminal(w io.Writer) bool {
	f, ok := w.(*os.File)
	if !ok {
		return false
	}
	info, err := f.Stat()
	if err != nil {
		return false
	}
	return info.Mode()&os.ModeCharDevice != 0
}
