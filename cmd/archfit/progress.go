package main

import (
	"fmt"
	"io"
	"os"
	"strconv"
	"time"
)

// progressReporter writes phase-progress lines to stderr while analyze runs, so
// a long run never looks hung. It is stream-aware:
//
//   - On a TTY (auto), each phase prints an in-progress line that is completed
//     in place (carriage return) with its elapsed time.
//   - When stderr is piped/redirected, or progress=plain is forced, it prints
//     one clean line per finished phase (no carriage returns — log-safe).
//   - When disabled (not a TTY, CI, TERM=dumb, --quiet, or --progress=none) it
//     is a no-op.
//
// Progress is ALWAYS stderr-only; stdout (JSON/SARIF/text result) is never
// touched, so `archfit --json | jq` and the determinism double-run stay clean.
type progressReporter struct {
	w        io.Writer
	enabled  bool
	live     bool // true = TTY in-place completion; false = one line per phase
	total    int
	n        int
	curLabel string
	curStart time.Time
	runStart time.Time
	pending  bool
}

// progress mode names (values of the --progress flag).
const (
	progressAuto  = "auto"
	progressPlain = "plain"
	progressNone  = "none"
)

// newProgressReporter resolves whether/how to show progress once, up front.
func newProgressReporter(w io.Writer, total int, mode string, quiet bool, now time.Time) *progressReporter {
	enabled, live := resolveProgress(w, mode, quiet)
	return &progressReporter{w: w, enabled: enabled, live: live, total: total, runStart: now}
}

// resolveProgress decides visibility (enabled) and animation (live). The target
// stream for the decision is stderr (w) — animation only when it is a TTY.
func resolveProgress(w io.Writer, mode string, quiet bool) (enabled, live bool) {
	if quiet || mode == progressNone {
		return false, false
	}
	// CI and dumb terminals: never animate; show plain lines only if explicitly forced.
	if os.Getenv("CI") != "" || os.Getenv("TERM") == "dumb" {
		return mode == progressPlain, false
	}
	tty := isTerminal(w)
	switch mode {
	case progressPlain:
		return true, false
	default: // auto / unset
		return tty, tty
	}
}

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

// banner prints the t=0 header so output appears immediately, before any slow work.
func (r *progressReporter) banner(line string) {
	if !r.enabled {
		return
	}
	_, _ = fmt.Fprintf(r.w, "%s\n\n", line)
}

// advance completes the current phase (if any) and starts the next.
func (r *progressReporter) advance(label string) {
	if !r.enabled {
		return
	}
	r.complete()
	r.n++
	r.curLabel = label
	r.curStart = time.Now()
	r.pending = true
	if r.live {
		// In-progress line, no newline; complete() rewrites it in place.
		_, _ = fmt.Fprintf(r.w, "[%d/%d] %s …", r.n, r.total, label)
	}
}

// complete finishes the current phase line with its elapsed time.
func (r *progressReporter) complete() {
	if !r.enabled || !r.pending {
		return
	}
	elapsed := fmtDur(time.Since(r.curStart))
	if r.live {
		_, _ = fmt.Fprintf(r.w, "\r[%d/%d] %s  done  %s\n", r.n, r.total, r.curLabel, elapsed)
	} else {
		_, _ = fmt.Fprintf(r.w, "[%d/%d] %s  %s\n", r.n, r.total, r.curLabel, elapsed)
	}
	r.pending = false
}

// finish completes the last phase and prints the total wall-clock time.
func (r *progressReporter) finish() {
	if !r.enabled {
		return
	}
	r.complete()
	_, _ = fmt.Fprintf(r.w, "Done in %s\n", fmtDur(time.Since(r.runStart)))
}

// fmtDur renders a duration as a compact "X.Xs".
func fmtDur(d time.Duration) string {
	return strconv.FormatFloat(d.Seconds(), 'f', 1, 64) + "s"
}
