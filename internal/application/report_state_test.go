package application

import (
	"bytes"
	"encoding/json"
	"reflect"
	"testing"

	"github.com/alexei-led/archfit/internal/assessment/finding"
	"github.com/alexei-led/archfit/internal/assessment/result"
	"github.com/alexei-led/archfit/internal/assessment/score"
	"github.com/alexei-led/archfit/internal/assessment/state"
	"github.com/alexei-led/archfit/internal/model/evidence"
	"github.com/alexei-led/archfit/internal/model/report"
)

// Shared literals for the shadow-state fixtures.
const (
	stateHeadRef     = "HEAD"
	stateToolSCIP    = "scip"
	stateMetricEdges = "internal_edges"
	stateSevHigh     = "high"
)

// stateFixture is a small assessment result carrying one of each fact the
// shadow state projection reads.
func stateFixture() result.Result {
	diagnostic := result.New()
	diagnostic.Verdict = result.VerdictWarn
	diagnostic.Head = stateHeadRef
	diagnostic.ConfigHash = "cfg-hash"
	diagnostic.ToolCoverage = []evidence.Coverage{
		{Tool: "go/packages", Version: "go1.24", Status: evidence.StatusOK},
		{Tool: stateToolSCIP, Status: evidence.StatusPartial, Reason: "empty index"},
	}
	diagnostic.VolatilityCorroboration = &evidence.VolatilityCorroboration{
		Source: "git", Status: "measured", CommitWindow: 500, CommitsScanned: 500,
	}
	return diagnostic
}

// TestProjectReportPopulatesShadowState asserts the contract-freeze projection
// carries the facts the result already has, and claims nothing it has not
// measured.
func TestProjectReportPopulatesShadowState(t *testing.T) {
	document := ProjectReport(stateFixture(), score.Scorecard{}, nil, false)
	state := document.State

	if state.SchemaVersion != report.StateSchemaVersion {
		t.Errorf("SchemaVersion = %q, want %q", state.SchemaVersion, report.StateSchemaVersion)
	}
	if state.Measurement.SourceRef != stateHeadRef {
		t.Errorf("Measurement.SourceRef = %q, want HEAD", state.Measurement.SourceRef)
	}
	if state.Measurement.HistoryDepth != 500 || state.Measurement.HistoryWindow != "500 commits" {
		t.Errorf("history = (%d, %q), want (500, \"500 commits\")", state.Measurement.HistoryDepth, state.Measurement.HistoryWindow)
	}
	if got := state.Measurement.ToolVersions["go/packages"]; got != "go1.24" {
		t.Errorf("ToolVersions[go/packages] = %q, want go1.24", got)
	}
	if _, recorded := state.Measurement.ToolVersions[stateToolSCIP]; recorded {
		t.Error("a tool that reported no version must not gain a fabricated one")
	}
	if len(state.Coverage.Tools) != 2 || state.Coverage.Tools[1].Status != evidence.StatusPartial {
		t.Errorf("Coverage.Tools = %+v, want both rows in acquisition order", state.Coverage.Tools)
	}
	if state.Comparison.ConfigHash != "cfg-hash" || state.Comparison.RubricVersion != report.ScoreVersion {
		t.Errorf("Comparison = %+v, want the run's config hash and rubric version", state.Comparison)
	}

	if state.Coverage.Measured+state.Coverage.Partial+state.Coverage.Unmeasured != report.DimensionCount {
		t.Errorf("coverage counts %+v do not sum to %d", state.Coverage, report.DimensionCount)
	}
	if state.Coverage.Unmeasured != report.DimensionCount {
		t.Errorf("Coverage.Unmeasured = %d, want %d: the fixture measured nothing", state.Coverage.Unmeasured, report.DimensionCount)
	}
	if state.Decision.UnknownDimensions != report.DimensionCount {
		t.Errorf("Decision.UnknownDimensions = %d, want %d", state.Decision.UnknownDimensions, report.DimensionCount)
	}
	for _, dim := range state.Dimensions.All() {
		if len(dim.Unknown) == 0 {
			t.Errorf("%s is unmeasured with no stated reason", dim.Name)
			continue
		}
		if dim.Unknown[0].Owner != dim.Owner {
			t.Errorf("%s unknown owner = %q, want the dimension's own owner %q", dim.Name, dim.Unknown[0].Owner, dim.Owner)
		}
	}
}

// TestShadowStateProjectsTheAssessmentVerdict pins that the projection carries
// the assessment's decision instead of re-deriving one. A renderer must not be
// able to reach a different conclusion than the run did.
func TestShadowStateProjectsTheAssessmentVerdict(t *testing.T) {
	for _, tc := range []struct {
		name         string
		verdict      state.Verdict
		hardGates    state.HardGateState
		blockers     int
		wantVerdict  report.StateVerdict
		wantGates    report.HardGateState
		wantBlockers int
	}{
		{
			name: "needs attention", verdict: state.NeedsAttention, hardGates: state.HardGatePass,
			wantVerdict: report.StateNeedsAttention, wantGates: report.HardGatePass,
		},
		{
			name: "blocked by findings", verdict: state.Blocked, hardGates: state.HardGateFail, blockers: 2,
			wantVerdict: report.StateBlocked, wantGates: report.HardGateFail, wantBlockers: 2,
		},
		{
			name: "blocked with no finding", verdict: state.Blocked, hardGates: state.HardGateFail,
			wantVerdict: report.StateBlocked, wantGates: report.HardGateFail,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			diagnostic := stateFixture()
			diagnostic.State.Verdict = tc.verdict
			diagnostic.State.Decision.HardGates = tc.hardGates
			diagnostic.State.Decision.ActiveBlockers = tc.blockers

			projected := ProjectReport(diagnostic, score.Scorecard{}, nil, false).State
			if projected.Verdict != tc.wantVerdict {
				t.Errorf("Verdict = %q, want %q", projected.Verdict, tc.wantVerdict)
			}
			if projected.Decision.HardGates != tc.wantGates {
				t.Errorf("Decision.HardGates = %q, want %q", projected.Decision.HardGates, tc.wantGates)
			}
			if projected.Decision.ActiveBlockers != tc.wantBlockers {
				t.Errorf("Decision.ActiveBlockers = %d, want %d", projected.Decision.ActiveBlockers, tc.wantBlockers)
			}
		})
	}
}

// TestShadowStateProjectsEveryEnvelopeField pins the assessment-to-report
// mapping: the two vocabularies are separate by architecture rule, so a field
// silently dropped here would be invisible everywhere else.
func TestShadowStateProjectsEveryEnvelopeField(t *testing.T) {
	diagnostic := stateFixture()
	measured := state.NewDimension(state.DimensionStructure, state.OwnerStructure)
	measured.Status = state.Measured
	measured.Confidence = state.ConfidenceMedium
	measured.Gate = state.GateWarn
	measured.Coverage = state.Coverage{Basis: "edges", Observed: 7, Total: 9}
	measured.Metrics = []state.MetricValue{{
		Name: stateMetricEdges, Value: 7, Unit: "count",
		Denominator: &state.MetricDenominator{Observed: 7, Total: 9}, Provenance: []string{"relationship/analysis"},
	}}
	measured.Findings = []state.FindingRef{{ID: "f1", RuleID: "dep", Kind: report.FindingKindGate, Severity: stateSevHigh, Status: "new"}}
	measured.Unknown = []state.UnknownFact{{Fact: "x", Reason: "y", Owner: state.OwnerStructure}}
	measured.Delta = &state.Delta{Status: state.ComparisonNonComparable, Reasons: []string{"r"},
		Metrics:     []state.MetricDelta{{Name: stateMetricEdges, Before: 5, After: 7, Change: 2}},
		NewFindings: []string{"f1"}}
	diagnostic.State.Dimensions.Structure = measured

	got := ProjectReport(diagnostic, score.Scorecard{}, nil, false).State.Dimensions.Structure
	want := report.DimensionState{
		Name: state.DimensionStructure, Owner: state.OwnerStructure,
		Status: report.MeasurementMeasured, Confidence: report.ConfidenceMedium, Gate: report.GateWarn,
		Coverage: report.DimensionCoverage{Basis: "edges", Observed: 7, Total: 9},
		Metrics: []report.MetricValue{{Name: stateMetricEdges, Value: 7, Unit: "count",
			Denominator: &report.MetricDenominator{Observed: 7, Total: 9}, Provenance: []string{"relationship/analysis"}}},
		Findings: []report.FindingRef{{ID: "f1", RuleID: "dep", Kind: report.FindingKindGate, Severity: stateSevHigh, Status: "new"}},
		Unknown:  []report.UnknownFact{{Fact: "x", Reason: "y", Owner: state.OwnerStructure}},
		Delta: &report.DimensionDelta{Status: report.ComparisonNonComparable, Reasons: []string{"r"},
			Metrics:     []report.MetricDelta{{Name: stateMetricEdges, Before: 5, After: 7, Change: 2}},
			NewFindings: []string{"f1"}},
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("projected envelope =\n%+v\nwant\n%+v", got, want)
	}
}

// TestShadowStateCoverageCountsTheProjectedEnvelopes pins that the coverage
// summary is recomputed from what was actually projected, so the counts and the
// envelopes cannot disagree.
func TestShadowStateCoverageCountsTheProjectedEnvelopes(t *testing.T) {
	diagnostic := stateFixture()
	diagnostic.State.Dimensions.Structure.Status = state.Measured
	diagnostic.State.Dimensions.Coupling.Status = state.Partial

	coverage := ProjectReport(diagnostic, score.Scorecard{}, nil, false).State.Coverage
	if coverage.Measured != 1 || coverage.Partial != 1 || coverage.Unmeasured != report.DimensionCount-2 {
		t.Errorf("coverage = %+v, want 1 measured, 1 partial, %d unmeasured", coverage, report.DimensionCount-2)
	}
	if coverage.Measured+coverage.Partial+coverage.Unmeasured != report.DimensionCount {
		t.Errorf("coverage counts %+v do not sum to %d", coverage, report.DimensionCount)
	}
}

// TestShadowStateComparisonIsNeverComparableYet asserts a --base run reports a
// named non-comparable reason instead of a comparison it cannot yet perform:
// the model and labels hashes the contract requires do not exist.
func TestShadowStateComparisonIsNeverComparableYet(t *testing.T) {
	t.Run("without base", func(t *testing.T) {
		state := ProjectReport(stateFixture(), score.Scorecard{}, nil, false).State
		if state.Comparison.Status != report.ComparisonNotRequested {
			t.Errorf("Comparison.Status = %q, want not_requested", state.Comparison.Status)
		}
		if len(state.Comparison.Reasons) != 0 {
			t.Errorf("Comparison.Reasons = %v, want empty when no comparison was asked for", state.Comparison.Reasons)
		}
	})

	t.Run("with base", func(t *testing.T) {
		diagnostic := stateFixture()
		diagnostic.Base = blockBaseRef

		state := ProjectReport(diagnostic, score.Scorecard{}, nil, false).State
		if state.Comparison.Status != report.ComparisonNonComparable {
			t.Errorf("Comparison.Status = %q, want non_comparable", state.Comparison.Status)
		}
		if state.Comparison.BaseRef != blockBaseRef || len(state.Comparison.Reasons) == 0 {
			t.Errorf("Comparison = %+v, want the base ref and a named reason", state.Comparison)
		}
	})
}

// TestShadowStateReferencesFindingsWithoutCopyingThem asserts findings and
// agent tasks reach the state unchanged, so identity, status, and ordering keep
// exactly one owner.
func TestShadowStateReferencesFindingsWithoutCopyingThem(t *testing.T) {
	diagnostic := stateFixture()
	diagnostic.Findings = []finding.Finding{
		{ID: "f1", RuleID: "no_internal_access", Kind: report.FindingKindGate, Status: finding.StatusNew},
		{ID: "f2", RuleID: "bc/imbalanced_coupling", Kind: report.FindingKindAdvisory, Status: finding.StatusNew},
	}
	diagnostic.AgentTasks = []result.AgentTask{{FindingID: "f1", RuleID: "no_internal_access"}}

	document := ProjectReport(diagnostic, score.Scorecard{}, nil, false)
	state := document.State

	if len(state.Findings) != len(document.Findings) {
		t.Fatalf("state carries %d findings, document carries %d", len(state.Findings), len(document.Findings))
	}
	if !reflect.DeepEqual(state.Findings, document.Findings) {
		t.Errorf("state findings differ from document findings:\n%+v\n%+v", state.Findings, document.Findings)
	}
	if len(state.AgentTasks) != 1 || state.AgentTasks[0].FindingID != "f1" {
		t.Errorf("AgentTasks = %+v, want the assessment-owned task list verbatim", state.AgentTasks)
	}
}

// TestShadowStateIsNotOnTheDiagnosticWire is the postcondition of the
// contract-freeze task: the state is projected but the existing JSON envelope
// does not move. The committed byte-identical baselines are the end-to-end
// proof; this is the unit-level guard.
func TestShadowStateIsNotOnTheDiagnosticWire(t *testing.T) {
	document := ProjectReport(stateFixture(), score.Scorecard{}, nil, false)

	encoded, err := json.Marshal(document)
	if err != nil {
		t.Fatalf("marshal document: %v", err)
	}
	for _, key := range []string{"schema_version\":\"archfit.architecture-state.v1", "\"dimensions\"", "\"hard_gates\""} {
		if bytes.Contains(encoded, []byte(key)) {
			t.Errorf("diagnostic envelope leaked architecture-state key %q:\n%s", key, encoded)
		}
	}
}

// TestShadowStateProjectionIsDeterministic asserts two projections of the same
// result encode identically. The state must be byte-stable before any renderer
// is allowed to depend on it.
func TestShadowStateProjectionIsDeterministic(t *testing.T) {
	encode := func() []byte {
		t.Helper()
		out, err := json.Marshal(ProjectReport(stateFixture(), score.Scorecard{}, nil, false).State)
		if err != nil {
			t.Fatalf("marshal state: %v", err)
		}
		return out
	}
	if first, second := encode(), encode(); !bytes.Equal(first, second) {
		t.Fatalf("state projection is not deterministic:\nfirst:  %s\nsecond: %s", first, second)
	}
}
