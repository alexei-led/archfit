package evaluation

import (
	"github.com/alexei-led/archfit/internal/assessment/agenttask"
	"github.com/alexei-led/archfit/internal/assessment/finding"
	"github.com/alexei-led/archfit/internal/assessment/result"
	"github.com/alexei-led/archfit/internal/assessment/score"
	"github.com/alexei-led/archfit/internal/policy"
)

// FindingIDCouplingGate prefixes the coupling-gate findings. One finding is
// emitted per newly introduced distributed-monolith seam, keyed by seam ID, so
// a blocking verdict always names the seams it blocked on and each one is
// individually addressable.
const FindingIDCouplingGate = "coupling-gate"

// BaselineAnchor is the assessment-relevant projection of a persisted baseline.
// The stage adapter reads the baseline file; assessment only decides whether
// the stored anchor is comparable with this binary's scoring.
//
// It carries no score anchor: schema v2 retired the scalar drop check, so a
// stored coupling score can no longer move any decision. What it carries
// instead is whether the stored seam snapshot may be compared at all.
type BaselineAnchor struct {
	// SnapshotMismatches names stored inputs that differ from this binary.
	// They become non-comparability reasons rather than a silent skip.
	SnapshotMismatches []string
	// SeamsComparable reports that the persisted baseline carries a seam
	// snapshot taken under the same config, module map, labels, and rubric.
	// False is the safe default: a baseline written before the seam ledger
	// existed has no seams, and treating its absence as "no seams then" would
	// report every current seam as newly introduced.
	SeamsComparable bool
	// QualifyingSeamIDs are the reference's own distributed-monolith seam IDs.
	QualifyingSeamIDs []string
	// NonComparableReason explains, in one line, why SeamsComparable is false.
	NonComparableReason string
}

// seamReference projects the anchor into the gate's comparison reference. A
// non-comparable anchor always carries at least one reason: an abstaining gate
// has to say why.
func (a BaselineAnchor) seamReference() score.SeamReference {
	if !a.SeamsComparable {
		reason := a.NonComparableReason
		if reason == "" {
			reason = "no comparable seam snapshot: the stored baseline predates the architecture-state contract"
		}
		return score.SeamReference{Reasons: append([]string{reason}, a.SnapshotMismatches...)}
	}
	ids := make(map[string]struct{}, len(a.QualifyingSeamIDs))
	for _, id := range a.QualifyingSeamIDs {
		ids[id] = struct{}{}
	}
	return score.SeamReference{Comparable: true, QualifyingSeamIDs: ids}
}

// FinalizeInput carries the explicit values assessment needs to score, gate,
// and build repair tasks. Every field is a value the stage already resolved:
// assessment neither reads configuration nor touches the filesystem.
type FinalizeInput struct {
	Gate               policy.CouplingGate
	Baseline           BaselineAnchor
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
// The scorecard is still computed — it remains a per-seam diagnostic and feeds
// the legacy output field — but it no longer reaches the gate. Only the seam
// ledger decides the coupling gate.
func finalize(diag *result.Result, in FinalizeInput) Finalized {
	card := score.Synthesize(*diag)
	trip := score.EvaluateSeamGate(diag.Seams, seamGateFor(in.Gate), in.Baseline.seamReference())
	applySeamGate(diag, trip)
	resolver := agenttask.NewPathResolver(in.KnownFiles, in.CrateRootDirs, in.ModuleRootDirs, in.OnDisk)
	diag.AgentTasks = agenttask.Build(diag.Findings, in.RuleTypes, in.ModulePublic, in.ValidationCommands, diag.SyntaxFacts, resolver)
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
