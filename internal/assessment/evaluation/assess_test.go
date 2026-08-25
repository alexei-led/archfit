// Behavior tests for the assessment stage seam. They pin what Assess and Score
// decide — verdict, report assembly, acquisition-resolved evidence attachment,
// health disclosure, repair-task validation command, and the required-analyzer
// gate — so the capability can move without moving semantics.
package evaluation_test

import (
	"strings"
	"testing"
	"time"

	"github.com/alexei-led/archfit/internal/assessment/evaluation"
	"github.com/alexei-led/archfit/internal/assessment/result"
	modevidence "github.com/alexei-led/archfit/internal/model/evidence"
	"github.com/alexei-led/archfit/internal/policy"
	"github.com/alexei-led/archfit/internal/relationship"
	"github.com/alexei-led/archfit/internal/scope"
)

const (
	assessModA    = "a"
	assessModB    = "b"
	assessFileA   = "a/a.go"
	assessFileB   = "b/b.go"
	assessCfgPath = "/repo/.archfit.yaml"
	assessRoot    = "/repo"
	assessHash    = "cfg-hash"
	assessOwner   = "codeowners"
	assessCore    = "core"
	assessGrimp   = "grimp"
)

func assessPolicy() policy.PolicySnapshot {
	modules := map[string]policy.ModuleDef{
		assessModA: {Paths: []string{"a/**"}, Subdomain: assessCore},
		assessModB: {Paths: []string{"b/**"}, Subdomain: "supporting"},
	}
	topology := policy.TopologyView{Modules: modules, ModuleMap: policy.BuildModuleMap(modules)}
	return policy.New(topology, policy.RelationshipPolicy{Topology: topology},
		policy.AssessmentPolicy{Topology: topology}, policy.GatePolicy{}, nil, nil)
}

// assessRelationships is one classified cross-module edge: enough for the
// projector to assemble a real diagnostic without running an extractor.
func assessRelationships() relationship.AnalysisResult {
	return relationship.AnalysisResult{
		Relationships: relationship.Set{
			Nodes: []relationship.Node{{ID: "file:" + assessFileA, Path: assessFileA}, {ID: "file:" + assessFileB, Path: assessFileB}},
			Edges: []relationship.Edge{{
				FromPath: assessFileA, ToPath: assessFileB, FromModule: assessModA, ToModule: assessModB,
				Kind: "imports", Strength: relationship.StrengthFunctional,
				Distance: relationship.DistanceCrossModuleSameOwner,
			}},
		},
		Evidence: relationship.AnalysisEvidence{
			DynamicImports: []modevidence.DynamicImport{{Module: assessModA, Count: 2}},
			RuntimeModules: []modevidence.RuntimeAsyncModule{{Module: assessModA, IntegrationKind: "queue", Count: 1}},
			RuntimeEdges:   []modevidence.RuntimeAsyncEdge{{FromModule: assessModA, Target: "nats", IntegrationKind: "queue", Count: 1}},
		},
	}
}

func assessInput() evaluation.AssessInput {
	return evaluation.AssessInput{
		Facts: evaluation.Observations{
			Coverage:  []modevidence.Coverage{{Tool: "go/packages", Status: modevidence.StatusOK}},
			FileFacts: []modevidence.FileFact{{Module: assessModA}},
		},
		Relationships: assessRelationships(),
		Policy:        assessPolicy(),
		Scope:         scope.Scope{Root: assessRoot, Mode: scope.ModeFull},
		Now:           time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC),
		Advisory:      true,
		ConfigSource:  assessCfgPath,
		ConfigHash:    assessHash,
		OwnerSource:   assessOwner,
		MarkedCoverage: []modevidence.Coverage{
			{Tool: assessGrimp, Status: modevidence.StatusDisabled, Reason: "language analysis disabled by config"},
		},
		CoverageGaps:            []modevidence.CoverageGap{{Tool: "cargo", Gate: "warn"}},
		ConfigWarnings:          []string{"decision needed: module a has no volatility"},
		VolatilityCorroboration: &modevidence.VolatilityCorroboration{Source: "git_history", Status: "ok"},
	}
}

// TestAssessAssemblesTheDiagnosticFromOwnedEvidence pins that assessment
// attaches what its owners already resolved instead of re-deriving it: the run
// identity from acquisition, the report-only rollups from relationship analysis.
func TestAssessAssemblesTheDiagnosticFromOwnedEvidence(t *testing.T) {
	t.Parallel()
	got, err := evaluation.Assess(assessInput())
	if err != nil {
		t.Fatal(err)
	}
	diag := got.Diagnostic
	if diag.SchemaVersion != result.SchemaVersion {
		t.Errorf("schema_version = %q, want %q", diag.SchemaVersion, result.SchemaVersion)
	}
	if diag.ConfigHash != assessHash || diag.OwnerSource != assessOwner {
		t.Errorf("run identity = hash %q owner %q, want the acquisition context verbatim", diag.ConfigHash, diag.OwnerSource)
	}
	if len(diag.DynamicImports) != 1 || diag.DynamicImports[0].Module != assessModA {
		t.Errorf("dynamic imports = %+v, want the relationship stage's rollup", diag.DynamicImports)
	}
	if len(diag.RuntimeAsync) != 1 || len(diag.RuntimeAsyncEdges) != 1 {
		t.Errorf("runtime async = %+v / %+v, want the relationship stage's rollups", diag.RuntimeAsync, diag.RuntimeAsyncEdges)
	}
	if len(diag.FileFacts) != 1 {
		t.Errorf("file facts = %+v, want the acquisition-built block", diag.FileFacts)
	}
	if diag.VolatilityCorroboration == nil || diag.VolatilityCorroboration.Source != "git_history" {
		t.Errorf("volatility corroboration = %+v, want the acquisition-resolved block", diag.VolatilityCorroboration)
	}
	if diag.DistanceContext == nil || diag.DistanceContext.OwnerModel == "" {
		t.Errorf("distance context = %+v, want it derived from the run policy", diag.DistanceContext)
	}
	// Rule and metric evaluation reads the RAW coverage rows, so a config opt-out
	// cannot move a measured metric. The marked copy lands only in Score.
	if len(diag.ToolCoverage) != 1 || diag.ToolCoverage[0].Status != modevidence.StatusOK {
		t.Errorf("tool coverage during evaluation = %+v, want the raw rows", diag.ToolCoverage)
	}
}

// TestScoreStampsAcquisitionEvidenceAndRepairCommand pins the second half: the
// scorecard, the repair-task validation command the agent contract promises, and
// the acquisition-resolved coverage evidence replacing the raw rows.
func TestScoreStampsAcquisitionEvidenceAndRepairCommand(t *testing.T) {
	t.Parallel()
	in := assessInput()
	assessed, err := evaluation.Assess(in)
	if err != nil {
		t.Fatal(err)
	}
	diag := assessed.Diagnostic
	scored := evaluation.Score(&diag, evaluation.ScoreInput{
		Policy: in.Policy, Facts: in.Facts, ConfigSource: assessCfgPath, ScanRoot: assessRoot,
		Root: assessRoot, MarkedCoverage: in.MarkedCoverage, CoverageGaps: in.CoverageGaps,
		ConfigWarnings: in.ConfigWarnings,
	})
	if scored.HardGate {
		t.Error("a warn-gated coverage gap must not hard-gate without --require-tools")
	}
	if len(diag.ToolCoverage) != 1 || diag.ToolCoverage[0].Status != modevidence.StatusDisabled {
		t.Errorf("tool coverage after scoring = %+v, want the marked rows", diag.ToolCoverage)
	}
	if len(diag.CoverageGaps) != 1 || len(diag.ConfigWarnings) != 1 {
		t.Errorf("gaps = %+v warnings = %+v, want the acquisition-resolved blocks", diag.CoverageGaps, diag.ConfigWarnings)
	}
	if scored.Score.RubricVersion == 0 {
		t.Error("scorecard was not synthesised")
	}
}

// TestScoreRequireToolsEscalatesAGap is the --require-tools contract: a
// disclosed gap that is not an explicit opt-out becomes a hard failure.
func TestScoreRequireToolsEscalatesAGap(t *testing.T) {
	t.Parallel()
	in := assessInput()
	assessed, err := evaluation.Assess(in)
	if err != nil {
		t.Fatal(err)
	}
	diag := assessed.Diagnostic
	scored := evaluation.Score(&diag, evaluation.ScoreInput{
		Policy: in.Policy, Facts: in.Facts, ConfigSource: assessCfgPath, ScanRoot: assessRoot,
		Root: assessRoot, MarkedCoverage: in.MarkedCoverage, CoverageGaps: in.CoverageGaps,
		RequireTools: true, ApplyToolGate: true,
	})
	if !scored.HardGate || diag.Verdict != result.VerdictFail {
		t.Fatalf("hardGate = %t verdict = %q, want a required-analyzer failure", scored.HardGate, diag.Verdict)
	}
}

// TestScoreWithoutApplyToolGateLeavesTheVerdictAlone pins that only analyze and
// check may turn a coverage gap into a failure. baseline, explain, enrich,
// config compare, and the --base sub-run render a verdict nothing consumes as
// an exit code, so rewriting it there is a report regression, not a gate.
func TestScoreWithoutApplyToolGateLeavesTheVerdictAlone(t *testing.T) {
	t.Parallel()
	in := assessInput()
	assessed, err := evaluation.Assess(in)
	if err != nil {
		t.Fatal(err)
	}
	diag := assessed.Diagnostic
	before := diag.Verdict
	scored := evaluation.Score(&diag, evaluation.ScoreInput{
		Policy: in.Policy, Facts: in.Facts, ConfigSource: assessCfgPath, ScanRoot: assessRoot,
		Root: assessRoot, MarkedCoverage: in.MarkedCoverage, CoverageGaps: in.CoverageGaps,
		RequireTools: true,
	})
	if scored.HardGate {
		t.Error("hardGate = true, want no hard gate outside analyze/check")
	}
	if diag.Verdict != before {
		t.Errorf("verdict = %q, want it unchanged at %q", diag.Verdict, before)
	}
}

// TestAssessRejectsAnInvalidRuleDefinition pins that a bad policy fails the
// stage rather than measuring a tree against rules that cannot fire.
func TestAssessRejectsAnInvalidRuleDefinition(t *testing.T) {
	t.Parallel()
	in := assessInput()
	in.Policy = policy.New(policy.TopologyView{}, policy.RelationshipPolicy{}, policy.AssessmentPolicy{},
		policy.GatePolicy{Rules: policy.RuleConfig{Rules: []policy.RuleDef{{ID: "bad", Type: "bogus_type"}}}}, nil, nil)
	if _, err := evaluation.Assess(in); err == nil || !strings.Contains(err.Error(), "unknown rule type") {
		t.Fatalf("Assess error = %v, want an unknown-rule-type failure", err)
	}
}

// TestAssessDisclosesAnUnscoredGraph pins the health disclosure: a run that
// classified edges but scored none tells the user which command fixes it.
func TestAssessDisclosesAnUnscoredGraph(t *testing.T) {
	t.Parallel()
	in := assessInput()
	in.Relationships.Evidence.ClassifiedEdges = &relationship.ClassifiedEdgeSummary{Total: 12, Scored: 0}
	got, err := evaluation.Assess(in)
	if err != nil {
		t.Fatal(err)
	}
	joined := strings.Join(got.Warnings, "\n")
	if !strings.Contains(joined, "0 of 12 edges scored") || !strings.Contains(joined, "archfit config update") {
		t.Fatalf("warnings = %q, want an actionable unscored-graph disclosure", joined)
	}
}
