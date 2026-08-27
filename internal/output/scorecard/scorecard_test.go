package scorecard

import (
	"bytes"
	"io"
	"strings"
	"testing"

	reportmodel "github.com/alexei-led/archfit/internal/model/report"
)

// Test literal constants (deduplicated for goconst).
const (
	confHigh = "high"
)

// goldenDocument is a fixed document whose rendered state scorecard is asserted
// byte-for-byte below. Regenerate the golden deliberately and inspect the diff
// if the output format changes.
func goldenDocument() reportmodel.Document {
	d := reportmodel.NewDocument()
	d.State = reportmodel.NewArchitectureState()
	d.State.Verdict = reportmodel.StateNeedsAttention
	d.State.Decision = reportmodel.StateDecision{
		HardGates: reportmodel.HardGatePass, ActiveBlockers: 0, AttentionDimensions: 1, UnknownDimensions: 8,
	}
	d.State.Comparison = reportmodel.StateComparison{
		Status: reportmodel.ComparisonNotRequested, ConfigHash: "abc123",
		RubricVersion: reportmodel.ScoreVersion, Reasons: []string{},
	}
	d.State.Dimensions.Coupling = reportmodel.DimensionState{
		Name: reportmodel.DimensionCoupling, Owner: reportmodel.OwnerCoupling,
		Status: reportmodel.MeasurementMeasured, Confidence: confHigh, Gate: reportmodel.GateWarn,
		Coverage: reportmodel.DimensionCoverage{Basis: "cross-boundary edges scored", Observed: 2, Total: 2},
		Metrics: []reportmodel.MetricValue{
			{Name: "critical_edges", Value: 1, Unit: "count"},
			{Name: "abstained_share", Value: 0.25, Unit: "ratio", Denominator: &reportmodel.MetricDenominator{Observed: 1, Total: 4}},
		},
	}
	d.State.Dimensions.Drift.Unknown = []reportmodel.UnknownFact{{
		Fact: "architecture drift", Reason: "no comparable architecture-state reference is stored", Owner: reportmodel.OwnerDrift,
	}}
	d.State.Dimensions.Drift.Delta = &reportmodel.DimensionDelta{
		Status: reportmodel.ComparisonNonComparable, Reasons: []string{"no comparable architecture-state reference is stored"},
	}
	d.State.Coverage = reportmodel.StateCoverage{Measured: 1, Partial: 0, Unmeasured: 8}
	// Two findings with DIFFERENT lifecycle statuses: the finding index is the
	// scorecard's only reference to finding identity, and an accepted finding
	// that vanished here would break cross-format parity silently.
	d.State.Findings = []reportmodel.Finding{
		{ID: "f1", RuleID: "bc/imbalanced_coupling", Status: "new"},
		{ID: "f2", RuleID: "no_direct_b_dependency", Status: "accepted"},
	}
	return d
}

func render(d reportmodel.Document, w io.Writer) error { return New().Render(d, w) }

const golden = `# archfit architecture state

**Verdict:** NEEDS ATTENTION
**Hard gates:** pass — 0 active blocker(s)
**Attention:** 1 dimension(s) flagged
**Coverage:** 1 measured / 0 partial / 8 unmeasured (of 9)
**Rubric version:** bc_score.v6
**Config hash:** ` + "`abc123`" + `

## Dimensions

### intent — unmeasured · gate: not_applicable · confidence: unrated
owner: policy+assessment/evaluation
denominator: none — this dimension measured nothing

### structure — unmeasured · gate: not_applicable · confidence: unrated
owner: relationship/facts
denominator: none — this dimension measured nothing

### modularity — unmeasured · gate: not_applicable · confidence: unrated
owner: assessment/metrics
denominator: none — this dimension measured nothing

### coupling — measured · gate: warn · confidence: high
owner: relationship/analysis
denominator: cross-boundary edges scored 2/2
- critical_edges: 1 count
- abstained_share: 0.25 ratio (1/4)

### change_locality — unmeasured · gate: not_applicable · confidence: unrated
owner: history/git
denominator: none — this dimension measured nothing

### complexity — unmeasured · gate: not_applicable · confidence: unrated
owner: syntax+evidence/acquisition
denominator: none — this dimension measured nothing

### testability — unmeasured · gate: not_applicable · confidence: unrated
owner: syntax/fileclass
denominator: none — this dimension measured nothing

### operations — unmeasured · gate: not_applicable · confidence: unrated
owner: policy+evidence/acquisition
denominator: none — this dimension measured nothing

### drift — unmeasured · gate: not_applicable · confidence: unrated
owner: assessment/decision
denominator: none — this dimension measured nothing
- not measured — architecture drift: no comparable architecture-state reference is stored
- delta: non_comparable
  - no comparable architecture-state reference is stored

## Comparison

- status: not_requested
- reference: none

## Finding index (2)

- ` + "`f1`" + ` new bc/imbalanced_coupling
- ` + "`f2`" + ` accepted no_direct_b_dependency
`

// TestRenderer_Golden asserts the exact rendered state scorecard.
func TestRenderer_Golden(t *testing.T) {
	var buf bytes.Buffer
	if err := render(goldenDocument(), &buf); err != nil {
		t.Fatalf("Render: %v", err)
	}
	if got := buf.String(); got != golden {
		t.Errorf("scorecard output mismatch:\n--- got ---\n%s\n--- want ---\n%s", got, golden)
	}
}

// TestRenderer_DoubleRun asserts the format is deterministic: two renders of the
// same document are byte-identical (CI determinism gate).
func TestRenderer_DoubleRun(t *testing.T) {
	d := goldenDocument()
	var a, b bytes.Buffer
	if err := render(d, &a); err != nil {
		t.Fatalf("Render a: %v", err)
	}
	if err := render(d, &b); err != nil {
		t.Fatalf("Render b: %v", err)
	}
	if !bytes.Equal(a.Bytes(), b.Bytes()) {
		t.Errorf("scorecard render not deterministic:\n a=%q\n b=%q", a.String(), b.String())
	}
}

// TestRenderer_Format asserts the format name.
func TestRenderer_Format(t *testing.T) {
	if got := New().Format(); got != "scorecard" {
		t.Errorf("Format() = %q, want scorecard", got)
	}
}

// TestRenderer_CarriesNoRepositoryScore is the migration contract for this
// format: the scorecard reports nine dimensions, never one averaged number.
func TestRenderer_CarriesNoRepositoryScore(t *testing.T) {
	var buf bytes.Buffer
	if err := render(goldenDocument(), &buf); err != nil {
		t.Fatalf("Render: %v", err)
	}
	out := buf.String()
	for _, forbidden := range []string{"**Overall:**", "/100", "archfit scorecard"} {
		if strings.Contains(out, forbidden) {
			t.Errorf("scorecard carries %q: it is a state report, not an architecture score:\n%s", forbidden, out)
		}
	}
}

// TestRenderer_ListsEveryDimension: all nine envelopes appear, so an unmeasured
// one cannot be silently omitted.
func TestRenderer_ListsEveryDimension(t *testing.T) {
	var buf bytes.Buffer
	if err := render(goldenDocument(), &buf); err != nil {
		t.Fatalf("Render: %v", err)
	}
	out := buf.String()
	for _, name := range []string{
		reportmodel.DimensionIntent, reportmodel.DimensionStructure, reportmodel.DimensionModularity,
		reportmodel.DimensionCoupling, reportmodel.DimensionChangeLocality, reportmodel.DimensionComplexity,
		reportmodel.DimensionTestability, reportmodel.DimensionOperations, reportmodel.DimensionDrift,
	} {
		if !strings.Contains(out, "### "+name+" — ") {
			t.Errorf("missing dimension block for %q:\n%s", name, out)
		}
	}
}

// TestRenderer_RequiredToolsMissing asserts the coverage-gap block renders after
// the dimensions so absent evidence is never mistaken for a healthy result.
func TestRenderer_RequiredToolsMissing(t *testing.T) {
	d := goldenDocument()
	d.CoverageGaps = []reportmodel.CoverageGap{
		{Tool: "go/packages", InstallCmd: "https://go.dev/dl", AffectedMetrics: []string{"coverage", "coupling_balance"}, Gate: "warn"},
	}

	var buf bytes.Buffer
	if err := render(d, &buf); err != nil {
		t.Fatalf("Render: %v", err)
	}
	out := buf.String()

	if !strings.Contains(out, "## Required tools missing (1)") {
		t.Fatalf("missing required-tools section\nfull output:\n%s", out)
	}
	if !strings.Contains(out, "**go/packages** [gate: warn] — affects coverage, coupling_balance; install: `https://go.dev/dl`") {
		t.Errorf("required-tools line not rendered as expected\nfull output:\n%s", out)
	}
}

// TestRenderer_RequiredToolsMissingAbsentWhenEmpty asserts the section is
// omitted when every required tool ran (no coverage gap).
func TestRenderer_RequiredToolsMissingAbsentWhenEmpty(t *testing.T) {
	var buf bytes.Buffer
	if err := render(goldenDocument(), &buf); err != nil {
		t.Fatalf("Render: %v", err)
	}
	if strings.Contains(buf.String(), "Required tools missing") {
		t.Errorf("required-tools section should be omitted when no gap\nfull output:\n%s", buf.String())
	}
}

// TestRenderer_Delta asserts the finding-lifecycle bucket block renders for a
// delta run and is omitted when there is no delta.
func TestRenderer_Delta(t *testing.T) {
	d := goldenDocument()
	d.Delta = &reportmodel.DeltaReport{
		New:             []string{"n1", "n2"},
		SeverityChanged: []string{"s1"},
		TouchedByDelta:  []string{"t1"},
		Resolved:        []string{"r1"},
	}

	var buf bytes.Buffer
	if err := render(d, &buf); err != nil {
		t.Fatalf("Render: %v", err)
	}
	out := buf.String()

	for _, want := range []string{
		"## Delta",
		"- new: 2",
		"- severity changed: 1",
		"- touched by this change: 1",
		"- pre-existing: 0",
		"- resolved: 1",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("delta block missing %q\nfull output:\n%s", want, out)
		}
	}

	var plain bytes.Buffer
	if err := render(goldenDocument(), &plain); err != nil {
		t.Fatalf("Render plain: %v", err)
	}
	if strings.Contains(plain.String(), "## Delta") {
		t.Errorf("non-delta scorecard should not contain a Delta block\nfull output:\n%s", plain.String())
	}
}

// TestRenderer_EmptyDocument asserts the renderer never panics on a zero
// document and still emits all nine dimension blocks.
func TestRenderer_EmptyDocument(t *testing.T) {
	var buf bytes.Buffer
	if err := render(reportmodel.NewDocument(), &buf); err != nil {
		t.Fatalf("Render: %v", err)
	}
	out := buf.String()
	if strings.Count(out, "### ") != reportmodel.DimensionCount {
		t.Errorf("expected %d dimension blocks, got:\n%s", reportmodel.DimensionCount, out)
	}
}
