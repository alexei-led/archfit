package evaluation_test

import (
	"slices"
	"strings"
	"testing"

	"github.com/alexei-led/archfit/internal/assessment/evaluation"
	"github.com/alexei-led/archfit/internal/assessment/finding"
	"github.com/alexei-led/archfit/internal/assessment/result"
	"github.com/alexei-led/archfit/internal/policy"
)

const (
	ruleNoCycles = "no-cycles"
	fileModuleA  = "pkg/a/a.go"
	fileModuleB  = "pkg/b/b.go"
)

// qualifyingSeam is one distributed-monolith seam: intrusive coupling across a
// deploy-unit boundary, in the critical band.
func qualifyingSeam(id, from, to string) result.Seam {
	return result.Seam{ID: id, FromModule: from, ToModule: to, DistributedMonolith: true,
		Strength: "intrusive", Distance: "cross_deploy_unit", Severity: "critical",
		CriticalEdges: 1, ScoredEdges: 1}
}

// comparableAnchor is a reference the gate may compare against, carrying the
// given seam IDs as already-known. Injecting it directly is deliberate: the
// fail-mode behaviour must be provable without a baseline file.
func comparableAnchor(known ...string) evaluation.BaselineAnchor {
	return evaluation.BaselineAnchor{SeamsComparable: true, QualifyingSeamIDs: known}
}

func seamDiag(seams ...result.Seam) result.Result {
	return result.Result{
		Verdict: result.VerdictPass,
		Seams:   seams,
		Findings: []finding.Finding{
			{ID: "bc-active", RuleID: finding.RuleIDBCImbalanced, Kind: finding.KindAdvisory, Status: finding.StatusNew},
			{ID: "rule-gate", RuleID: ruleNoCycles, Kind: finding.KindGate, Status: finding.StatusNew},
		},
		Summary: result.Summary{GateFindings: 1, Warnings: 1},
	}
}

// TestApplySeamGate_BlocksOnlyOnNewSeamsInFailMode pins the whole gate
// contract: warn never blocks, fail blocks only on a newly introduced seam
// against a comparable reference, and a low coupling score is not an input.
func TestApplySeamGate_BlocksOnlyOnNewSeamsInFailMode(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name        string
		gate        policy.CouplingGate
		anchor      evaluation.BaselineAnchor
		wantVerdict result.Verdict
		wantGates   int
	}{
		{
			name: "warn mode is diagnostic even on a new seam",
			gate: policy.CouplingGate{Mode: policy.DistributedMonolithWarn}, anchor: comparableAnchor(),
			wantVerdict: result.VerdictPass, wantGates: 1,
		},
		{
			name: "fail mode without a comparable reference cannot claim a new seam",
			gate: policy.CouplingGate{Mode: policy.DistributedMonolithFail},
			anchor: evaluation.BaselineAnchor{
				NonComparableReason: "legacy_score_snapshot_ignored", SnapshotMismatches: []string{"score_version"}},
			wantVerdict: result.VerdictPass, wantGates: 1,
		},
		{
			name: "fail mode blocks on a newly introduced seam",
			gate: policy.CouplingGate{Mode: policy.DistributedMonolithFail}, anchor: comparableAnchor(),
			wantVerdict: result.VerdictFail, wantGates: 2,
		},
		{
			name: "fail mode leaves a pre-existing seam alone",
			gate: policy.CouplingGate{Mode: policy.DistributedMonolithFail}, anchor: comparableAnchor("seam-1"),
			wantVerdict: result.VerdictPass, wantGates: 1,
		},
		{
			name: "fail mode tolerates up to max_new_seams",
			gate: policy.CouplingGate{Mode: policy.DistributedMonolithFail, MaxNewSeams: 1}, anchor: comparableAnchor(),
			wantVerdict: result.VerdictPass, wantGates: 1,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			diag := seamDiag(qualifyingSeam("seam-1", "billing", "shipping"))
			evaluation.ApplySeamGate(&diag, tc.gate, tc.anchor)
			if diag.Verdict != tc.wantVerdict {
				t.Errorf("verdict = %q, want %q", diag.Verdict, tc.wantVerdict)
			}
			if diag.Summary.GateFindings != tc.wantGates {
				t.Errorf("gate findings = %d, want %d", diag.Summary.GateFindings, tc.wantGates)
			}
			if got := diag.Findings[0].Kind; got != finding.KindAdvisory {
				t.Errorf("BC advisory kind = %q, want advisory — the seam gate names its own seams "+
					"instead of promoting unrelated diagnostics", got)
			}
		})
	}
}

// TestApplySeamGate_NamesEachBlockingSeam pins that a blocked run ships one
// addressable gate finding per new seam, carrying the module pair.
func TestApplySeamGate_NamesEachBlockingSeam(t *testing.T) {
	t.Parallel()
	diag := seamDiag(qualifyingSeam("seam-1", "billing", "shipping"), qualifyingSeam("seam-2", "orders", "billing"))
	reasons := evaluation.ApplySeamGate(&diag,
		policy.CouplingGate{Mode: policy.DistributedMonolithFail}, comparableAnchor("seam-2"))

	var gates []finding.Finding
	for _, f := range diag.Findings {
		if f.RuleID == finding.RuleIDCouplingGate {
			gates = append(gates, f)
		}
	}
	if len(gates) != 1 {
		t.Fatalf("coupling-gate findings = %d, want exactly one for the single new seam", len(gates))
	}
	if gates[0].Edge.From.Module != "billing" || gates[0].Edge.To.Module != "shipping" {
		t.Errorf("gate finding edge = %s -> %s, want the offending module pair",
			gates[0].Edge.From.Module, gates[0].Edge.To.Module)
	}
	if !strings.Contains(gates[0].ID, "seam-1") {
		t.Errorf("gate finding ID = %q, want it keyed by the seam it blocks on", gates[0].ID)
	}
	if !strings.Contains(strings.Join(reasons, "; "), "billing -> shipping") {
		t.Errorf("reasons = %v, want the blocking seam named", reasons)
	}
}

// TestApplySeamGate_AbstentionIsDisclosed pins that an unrated gate says why:
// a present seam with no comparable reference reports the abstention rather
// than reading as a clean run.
func TestApplySeamGate_AbstentionIsDisclosed(t *testing.T) {
	t.Parallel()
	diag := seamDiag(qualifyingSeam("seam-1", "billing", "shipping"))
	reasons := evaluation.ApplySeamGate(&diag, policy.CouplingGate{Mode: policy.DistributedMonolithFail},
		evaluation.BaselineAnchor{NonComparableReason: "legacy_score_snapshot_ignored"})

	joined := strings.Join(reasons, "; ")
	if !strings.Contains(joined, "no comparable reference") {
		t.Errorf("reasons = %q, want the abstention stated", joined)
	}
	if !strings.Contains(joined, "legacy_score_snapshot_ignored") {
		t.Errorf("reasons = %q, want the reference's own reason disclosed", joined)
	}
}

// TestFinalize_BuildsRepairTasks pins that Finalize is the single place repair
// work is attached: every gate finding leaves an agent task carrying the
// validation command, and advisories leave advisory tasks.
func TestFinalize_BuildsRepairTasks(t *testing.T) {
	t.Parallel()
	const validate = "archfit check -c .archfit.yaml"
	diag := result.Result{
		Verdict: result.VerdictFail,
		Findings: []finding.Finding{
			{ID: "g1", RuleID: ruleNoCycles, Kind: finding.KindGate, Status: finding.StatusNew,
				Edge: finding.EdgeEvidence{From: finding.Endpoint{Module: "a", Path: fileModuleA}, To: finding.Endpoint{Module: "b", Path: fileModuleB}}},
			{ID: "a1", RuleID: finding.RuleIDBCImbalanced, Kind: finding.KindAdvisory, Status: finding.StatusNew,
				MatchedBy: map[string]string{"group_count": "3", "group_members": "a1,a2,a3"}},
		},
	}
	evaluation.Finalize(&diag, evaluation.FinalizeInput{
		RuleTypes:          map[string]string{ruleNoCycles: ruleForbidden},
		ValidationCommands: []string{validate},
		KnownFiles:         map[string]struct{}{fileModuleA: {}, fileModuleB: {}},
		OnDisk:             func(string) bool { return true },
	})
	if len(diag.AgentTasks) != 1 || diag.AgentTasks[0].FindingID != "g1" {
		t.Fatalf("agent tasks = %+v, want one for the gate finding", diag.AgentTasks)
	}
	if !slices.Contains(diag.AgentTasks[0].Validation, validate) {
		t.Errorf("validation = %v, want the supplied command", diag.AgentTasks[0].Validation)
	}
	if len(diag.AdvisoryTasks) != 1 || diag.AdvisoryTasks[0].FindingID != "a1" {
		t.Fatalf("advisory tasks = %+v, want one for the BC advisory", diag.AdvisoryTasks)
	}
}

// TestApplySeamGate_IgnoresTheRepositoryScore pins the migration's whole point:
// a catastrophic repository coupling score with no qualifying seam does not
// block, and a healthy one with a new qualifying seam does. The scalar is not
// an input in either direction.
func TestApplySeamGate_IgnoresTheRepositoryScore(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name        string
		seams       []result.Seam
		wantVerdict result.Verdict
	}{
		{
			name: "no qualifying seam never blocks, whatever the score says",
			seams: []result.Seam{{
				ID: "seam-1", FromModule: "a", ToModule: "b", Severity: "critical",
				Scores: result.SeamScoreDistribution{N: 40, Min: 1, Median: 1, Max: 2, Mean: 1.2},
			}},
			wantVerdict: result.VerdictPass,
		},
		{
			name: "one qualifying seam blocks, whatever the score says",
			seams: []result.Seam{func() result.Seam {
				s := qualifyingSeam("seam-1", "billing", "shipping")
				s.Scores = result.SeamScoreDistribution{N: 40, Min: 9, Median: 10, Max: 10, Mean: 9.8}
				return s
			}()},
			wantVerdict: result.VerdictFail,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			diag := seamDiag(tc.seams...)
			evaluation.ApplySeamGate(&diag, policy.CouplingGate{Mode: policy.DistributedMonolithFail}, comparableAnchor())
			if diag.Verdict != tc.wantVerdict {
				t.Errorf("verdict = %q, want %q", diag.Verdict, tc.wantVerdict)
			}
		})
	}
}

// TestFinalize_PreservesPerEdgeAdvisoryEvidence pins that retiring gate
// promotion did not cost the per-edge evidence a reviewer reads. A coupling
// advisory still carries its own cheapest move and score value, so `explain`
// and the advisory task list can still say what to change and why.
func TestFinalize_PreservesPerEdgeAdvisoryEvidence(t *testing.T) {
	t.Parallel()
	diag := result.Result{
		Verdict: result.VerdictPass,
		Findings: []finding.Finding{{
			ID: "bc-1", RuleID: finding.RuleIDBCImbalanced, Kind: finding.KindAdvisory, Status: finding.StatusNew,
			Edge: finding.EdgeEvidence{
				From: finding.Endpoint{Module: "a", Path: fileModuleA},
				To:   finding.Endpoint{Module: "b", Path: fileModuleB},
			},
			MatchedBy: map[string]string{"cheapest_move": "reduce_strength", "score_value": "3",
				"group_count": "2", "group_members": "bc-1,bc-2"},
		}},
	}

	evaluation.Finalize(&diag, evaluation.FinalizeInput{
		Gate:               policy.CouplingGate{Mode: policy.DistributedMonolithWarn},
		ValidationCommands: []string{"archfit check -c .archfit.yaml"},
		KnownFiles:         map[string]struct{}{fileModuleA: {}, fileModuleB: {}},
		OnDisk:             func(string) bool { return true },
	})

	if diag.Findings[0].Kind != finding.KindAdvisory {
		t.Errorf("advisory kind = %q, want advisory — the seam gate promotes nothing", diag.Findings[0].Kind)
	}
	if len(diag.AdvisoryTasks) != 1 {
		t.Fatalf("advisory tasks = %+v, want one for the coupling advisory", diag.AdvisoryTasks)
	}
	task := diag.AdvisoryTasks[0]
	if task.CheapestMove != "reduce_strength" {
		t.Errorf("cheapest_move = %q, want the per-edge remediation preserved", task.CheapestMove)
	}
	if task.ScoreValue != 3 {
		t.Errorf("score_value = %d, want the per-edge balance preserved", task.ScoreValue)
	}
	if len(task.TopFiles) == 0 {
		t.Error("top_files = [], want the per-edge file evidence preserved")
	}
}
