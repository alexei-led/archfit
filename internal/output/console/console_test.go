package console_test

import (
	"bytes"
	"strings"
	"testing"

	"github.com/alexei-led/archfit/internal/model/diagnostic"
	"github.com/alexei-led/archfit/internal/output/console"
)

func TestRenderer_Format(t *testing.T) {
	r := console.New()
	if got := r.Format(); got != "text" {
		t.Errorf("Format() = %q, want %q", got, "text")
	}
}

func TestRenderer_Render(t *testing.T) {
	tests := []struct {
		name         string
		verdict      diagnostic.Verdict
		gateFindings int
		wantVerdict  string
		wantCount    string
		wantExitHint string
	}{
		{
			name:         "pass",
			verdict:      diagnostic.VerdictPass,
			gateFindings: 0,
			wantVerdict:  "[PASS]",
			wantCount:    "0 gate finding(s)",
			wantExitHint: "exit 0",
		},
		{
			name:         "fail with findings",
			verdict:      diagnostic.VerdictFail,
			gateFindings: 3,
			wantVerdict:  "[FAIL]",
			wantCount:    "3 gate finding(s)",
			wantExitHint: "exit 1",
		},
		{
			name:         "warn",
			verdict:      diagnostic.VerdictWarn,
			gateFindings: 1,
			wantVerdict:  "[WARN]",
			wantCount:    "1 gate finding(s)",
			wantExitHint: "exit 2",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			d := diagnostic.New()
			d.Verdict = tt.verdict
			d.Summary.GateFindings = tt.gateFindings

			r := console.New()
			var buf bytes.Buffer

			if err := r.Render(d, &buf); err != nil {
				t.Fatalf("Render() error = %v", err)
			}

			out := buf.String()

			for _, want := range []string{tt.wantVerdict, tt.wantCount, tt.wantExitHint, "--format json"} {
				if !strings.Contains(out, want) {
					t.Errorf("output missing %q\nfull output:\n%s", want, out)
				}
			}
		})
	}
}
