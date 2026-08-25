package evaluation

import (
	"github.com/alexei-led/archfit/internal/assessment/result"
)

// gateOff / gateWarn / gateFail are the coverage-gap gate postures stamped on
// each gap by acquisition. warn (default) degrades a missing tool's metrics to
// n/a and reports it without failing; fail is the opt-in hard gate
// (languages.<x>.gate: fail / --require-tools); off is an explicit opt-out that
// --require-tools does not overrule.
const (
	gateOff  = "off"
	gateFail = "fail"
)

// applyToolGate finalises the hard-gate decision for a check/scan run:
// --require-tools raises every coverage gap that is not an explicit opt-out to
// fail, and any gap that gates fail stamps the verdict fail so the rendered
// output reflects the policy failure. Returns true when the run must exit 1.
//
// The gate: off carve-out is what lets buildCoverageGaps report a gap it used to
// swallow. A gap is a disclosure ("this analyzer did not run here"); the gate is
// a policy ("and that fails the build"). Conflating them is why an opt-out had
// to be silent to stay non-blocking, and that silence read downstream as "the
// language is not in this tree". Acquisition resolves each gap's configured posture; this function only
// applies the run-level override and the verdict consequence. Idempotent and render-order safe:
// callers invoke it before rendering so the output shows the effective gate.
func applyToolGate(diag *result.Result, requireTools bool) bool {
	failed := false
	for i := range diag.CoverageGaps {
		// gate: off is the user's explicit statement that this analyzer must not
		// fail the build, and --require-tools does not overrule it. The gap is
		// still reported: it exists so the run discloses which analyzer went
		// unmeasured over a language the tree actually contains, which is a
		// different question from whether that failure gates.
		if diag.CoverageGaps[i].Gate == gateOff {
			continue
		}
		if requireTools {
			diag.CoverageGaps[i].Gate = gateFail
		}
		if diag.CoverageGaps[i].Gate == gateFail {
			failed = true
		}
	}
	if failed {
		diag.Verdict = result.VerdictFail
	}
	return failed
}
