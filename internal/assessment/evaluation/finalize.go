package evaluation

import (
	"github.com/alexei-led/archfit/v3/internal/assessment/agenttask"
	"github.com/alexei-led/archfit/v3/internal/assessment/finding"
	"github.com/alexei-led/archfit/v3/internal/assessment/result"
	"github.com/alexei-led/archfit/v3/internal/assessment/score"
	"github.com/alexei-led/archfit/v3/internal/policy"
)

// FindingIDCouplingGate prefixes the coupling-gate findings. One finding is
// emitted per newly introduced distributed-monolith seam, keyed by seam ID, so
// a blocking verdict always names the seams it blocked on and each one is
// individually addressable.
const FindingIDCouplingGate = "coupling-gate"

// The legacy seam gate has no stored input; task 34 removes it.
const noStoredReferenceReason = "stored architecture-state references are no longer supported"

// FinalizeInput carries the explicit values assessment needs to score, gate,
// and build repair tasks. Every field is a value the stage already resolved:
// assessment neither reads configuration nor touches the filesystem.
type FinalizeInput struct {
	Gate               policy.CouplingGate
	RuleTypes          map[string]string
	ModulePublic       map[string][]string
	ValidationCommands []string
	KnownFiles         map[string]struct{}
	CrateRootDirs      map[string]string
	ModuleRootDirs     map[string]string
	OnDisk             func(string) bool
}

// Finalized is the assessment finalization outcome. Score is the synthesised
// scorecard; GateReasons explain a tripped coupling gate and are disclosed by
// the analyze command only.
type Finalized struct {
	Score       score.Scorecard
	GateReasons []string
}

// finalize synthesises the scorecard, applies the coupling seam gate, and
// attaches repair tasks to diag. It is pure: every input is an already-resolved
// value.
//
// The internal scorecard is still computed for metric diagnostics, config
// comparison, and AI review, but it never reaches the architecture verdict.
// Only the seam ledger decides the coupling gate.
func finalize(diag *result.Result, in FinalizeInput) Finalized {
	card := score.Synthesize(*diag)
	trip := score.EvaluateSeamGate(diag.Seams, seamGateFor(in.Gate), score.SeamReference{Reasons: []string{noStoredReferenceReason}})
	applySeamGate(diag, trip)
	resolver := agenttask.NewPathResolver(in.KnownFiles, in.CrateRootDirs, in.ModuleRootDirs, in.OnDisk)
	diag.AgentTasks = agenttask.Build(diag.Findings, in.RuleTypes, in.ModulePublic, in.ValidationCommands, diag.SyntaxFacts, diag.Seams, resolver)
	diag.AdvisoryTasks = agenttask.BuildAdvisoryTasks(diag.Findings, in.ValidationCommands)
	return Finalized{Score: card, GateReasons: trip.Reasons}
}

func seamGateFor(g policy.CouplingGate) score.SeamGate {
	return score.SeamGate{Mode: string(g.Mode), MaxNewSeams: g.MaxNewSeams}
}

// applySeamGate escalates the verdict and emits one gate finding per newly
// introduced distributed-monolith seam.
//
// It deliberately does not promote existing coupling advisories. Promotion was
// scalar-gate behaviour: the old gate tripped on a repository number, so it had
// to borrow findings to point at. The seam gate already knows exactly which
// seams tripped it, so it says so directly and leaves every other advisory as
// the diagnostic it is.
func applySeamGate(diag *result.Result, trip score.SeamGateResult) {
	if !trip.Blocked {
		return
	}
	diag.Verdict = result.VerdictFail
	for _, s := range trip.New {
		diag.Findings = append(diag.Findings, finding.Finding{
			ID: FindingIDCouplingGate + "/" + s.ID, Kind: finding.KindGate,
			RuleID: finding.RuleIDCouplingGate, Status: finding.StatusNew, Severity: finding.SeverityHigh,
			Edge: finding.EdgeEvidence{
				From: finding.Endpoint{Module: s.FromModule},
				To:   finding.Endpoint{Module: s.ToModule},
			},
			Why: "newly introduced distributed-monolith seam: " + s.FromModule + " -> " + s.ToModule +
				" couples at " + s.Strength + " across " + s.Distance +
				" (coupling.gate.distributed_monolith.mode: fail)",
		})
		diag.Summary.GateFindings++
	}
}
