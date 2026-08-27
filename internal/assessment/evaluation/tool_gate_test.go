package evaluation

import (
	"testing"

	"github.com/alexei-led/archfit/internal/assessment/result"
	"github.com/alexei-led/archfit/internal/model/evidence"
)

// Coverage-gap gate postures and analyzer names used by the gate table. They
// mirror the strings acquisition stamps on each gap.
const (
	gateWarn       = "warn"
	toolJscpd      = "jscpd"
	toolGoPackages = "go/packages"
)

// TestApplyToolGate verifies the hard-gate decision: --require-tools raises every
// gap to fail and stamps the verdict; an explicit per-tool fail gate trips without
// the flag; an all-warn run with no flag does not trip and leaves the verdict alone.
func TestApplyToolGate(t *testing.T) {
	t.Parallel()
	t.Run("require-tools raises all gaps to fail", func(t *testing.T) {
		t.Parallel()
		diag := result.Result{
			Verdict: result.VerdictPass,
			CoverageGaps: []evidence.CoverageGap{
				{Tool: toolGrimp, Gate: gateWarn},
				{Tool: toolJscpd, Gate: gateWarn},
			},
		}
		if !ApplyToolGate(&diag, true) {
			t.Fatal("ApplyToolGate(require=true) = false, want true (hard gate)")
		}
		if diag.Verdict != result.VerdictFail {
			t.Errorf("verdict = %q, want fail", diag.Verdict)
		}
		for _, g := range diag.CoverageGaps {
			if g.Gate != gateFail {
				t.Errorf("gap %q gate = %q, want fail", g.Tool, g.Gate)
			}
		}
	})

	t.Run("explicit fail gate trips without the flag", func(t *testing.T) {
		t.Parallel()
		diag := result.Result{
			Verdict: result.VerdictPass,
			CoverageGaps: []evidence.CoverageGap{
				{Tool: toolGrimp, Gate: gateWarn},
				{Tool: toolGoPackages, Gate: gateFail},
			},
		}
		if !ApplyToolGate(&diag, false) {
			t.Fatal("applyToolGate with a fail gap = false, want true")
		}
		if diag.Verdict != result.VerdictFail {
			t.Errorf("verdict = %q, want fail", diag.Verdict)
		}
		if diag.CoverageGaps[0].Gate != gateWarn {
			t.Errorf("warn gap was mutated to %q without --require-tools", diag.CoverageGaps[0].Gate)
		}
	})

	t.Run("gate: off survives --require-tools", func(t *testing.T) {
		t.Parallel()
		// An explicit `gate: off` is the user saying "this analyzer is not part
		// of my gate". --require-tools raises WARN gaps; deleting the off skip
		// would silently convert every opt-out into an exit-1 hard gate.
		diag := result.Result{
			Verdict: result.VerdictPass,
			CoverageGaps: []evidence.CoverageGap{
				{Tool: toolJscpd, Gate: gateOff},
			},
		}
		if ApplyToolGate(&diag, true) {
			t.Fatal("ApplyToolGate(gate: off, require=true) = true, want false")
		}
		if diag.CoverageGaps[0].Gate != gateOff {
			t.Errorf("off gap was raised to %q under --require-tools", diag.CoverageGaps[0].Gate)
		}
		if diag.Verdict != result.VerdictPass {
			t.Errorf("verdict = %q, want pass (unchanged)", diag.Verdict)
		}
	})

	t.Run("all warn, no flag, does not trip", func(t *testing.T) {
		t.Parallel()
		diag := result.Result{
			Verdict: result.VerdictPass,
			CoverageGaps: []evidence.CoverageGap{
				{Tool: toolGrimp, Gate: gateWarn},
			},
		}
		if ApplyToolGate(&diag, false) {
			t.Fatal("ApplyToolGate(all warn, require=false) = true, want false")
		}
		if diag.Verdict != result.VerdictPass {
			t.Errorf("verdict = %q, want pass (unchanged)", diag.Verdict)
		}
	})
}
