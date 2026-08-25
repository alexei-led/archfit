package evaluation

import (
	"strings"

	"github.com/alexei-led/archfit/internal/assessment/agenttask"
	"github.com/alexei-led/archfit/internal/assessment/finding"
	"github.com/alexei-led/archfit/internal/assessment/result"
	"github.com/alexei-led/archfit/internal/assessment/score"
	"github.com/alexei-led/archfit/internal/policy"
)

// FindingIDCouplingGate is the fixed synthetic coupling-gate finding ID. The
// gate emits it only when a trip has no promotable advisory, so a FAIL verdict
// never ships with zero gate findings.
const FindingIDCouplingGate = "coupling-gate"

const captureMetricName = "application_enrichment_capture"

func capturedMetricResult() result.MetricResult {
	return result.MetricResult{Name: captureMetricName, Version: captureMetricName + ".v1", Band: "info", Display: "internal capture"}
}

// BaselineAnchor is the assessment-relevant projection of a persisted baseline.
// The stage adapter reads the baseline file; assessment only decides whether
// the stored anchor is comparable with this binary's scoring.
type BaselineAnchor struct {
	CouplingScore      *int
	SnapshotMismatches []string
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

// Finalize synthesises the scorecard, applies the coupling gate, and attaches
// repair tasks to diag. It is pure: every input is an already-resolved value.
func Finalize(diag *result.Result, in FinalizeInput) Finalized {
	card := score.Synthesize(*diag)
	gate := couplingGateFor(in.Gate)
	applyCouplingGate(diag, card, gate, in.Baseline.CouplingScore)
	resolver := agenttask.NewPathResolver(in.KnownFiles, in.CrateRootDirs, in.ModuleRootDirs, in.OnDisk)
	diag.AgentTasks = agenttask.Build(diag.Findings, in.RuleTypes, in.ModulePublic, in.ValidationCommands, diag.SyntaxFacts, resolver)
	diag.AdvisoryTasks = agenttask.BuildAdvisoryTasks(diag.Findings, in.ValidationCommands)
	return Finalized{Score: card, GateReasons: score.EvaluateCouplingGate(card, gate, in.Baseline.CouplingScore).Reasons}
}

// CouplingGateAnchorStale reports whether max_drop was skipped because the
// stored score snapshot is incompatible with this binary.
func CouplingGateAnchorStale(gate policy.CouplingGate, anchor BaselineAnchor) bool {
	return gate.Enabled && gate.MaxDrop != nil && len(anchor.SnapshotMismatches) > 0
}

func couplingGateFor(g policy.CouplingGate) score.CouplingGate {
	return score.CouplingGate{Enabled: g.Enabled, MinBand: score.Band(g.MinBand), MaxDrop: g.MaxDrop}
}

// applyCouplingGate escalates a measured coupling score and promotes active
// coupling advisories into gate findings.
func applyCouplingGate(diag *result.Result, card score.Scorecard, gate score.CouplingGate, baselineScore *int) {
	trip := score.EvaluateCouplingGate(card, gate, baselineScore)
	if !trip.Tripped {
		return
	}
	diag.Verdict = result.VerdictFail
	promoted := 0
	for i := range diag.Findings {
		f := &diag.Findings[i]
		if f.RuleID != finding.RuleIDBCImbalanced || f.Kind != finding.KindAdvisory || !score.IsActiveGateFinding(*f) {
			continue
		}
		f.Kind = finding.KindGate
		promoted++
	}
	diag.Summary.GateFindings += promoted
	diag.Summary.Warnings = max(0, diag.Summary.Warnings-promoted)
	if promoted == 0 {
		diag.Findings = append(diag.Findings, finding.Finding{ID: FindingIDCouplingGate, Kind: finding.KindGate,
			RuleID: finding.RuleIDCouplingGate, Status: finding.StatusNew, Severity: finding.SeverityHigh,
			Why: strings.Join(trip.Reasons, "; ")})
		diag.Summary.GateFindings++
	}
}
