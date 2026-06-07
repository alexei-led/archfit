package console

import (
	"fmt"
	"io"
	"strings"

	"github.com/alexei-led/archfit/internal/model/diagnostic"
)

// Renderer prints a short human-readable summary of a Diagnostic.
type Renderer struct{}

// New returns a new Renderer.
func New() *Renderer {
	return &Renderer{}
}

// Format returns "console".
func (r *Renderer) Format() string {
	return "console"
}

// Render writes a short summary to w:
//
//	[PASS] / [FAIL] / [WARN]
//	N gate finding(s)
//	exit 0 / exit 1 / exit 2
//	Run with --format json for full detail
func (r *Renderer) Render(d diagnostic.Diagnostic, w io.Writer) error {
	verdict, exitCode := verdictLabel(d.Verdict)

	lines := []string{
		verdict,
		fmt.Sprintf("%d gate finding(s)", d.Summary.GateFindings),
		fmt.Sprintf("exit %d", exitCode),
		"Run with --format json for full detail",
	}

	_, err := fmt.Fprintln(w, strings.Join(lines, "\n"))
	return err
}

// verdictLabel returns the bracketed label and exit code for a verdict.
func verdictLabel(v diagnostic.Verdict) (string, int) {
	switch v {
	case diagnostic.VerdictFail:
		return "[FAIL]", 1
	case diagnostic.VerdictWarn:
		return "[WARN]", 2
	default:
		return "[PASS]", 0
	}
}
