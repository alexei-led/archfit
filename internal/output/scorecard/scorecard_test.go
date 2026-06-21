package scorecard

import (
	"bytes"
	"strings"
	"testing"

	"github.com/alexei-led/archfit/internal/model/diagnostic"
	"github.com/alexei-led/archfit/internal/model/finding"
)

// Test literal constants (deduplicated for goconst).
const (
	confHigh   = "high"
	bandInfo   = "info"
	bandStrong = "strong"
	sevMedium  = "medium"
)

// goldenDiagnostic is a fixed, fully-measured Diagnostic whose rendered scorecard
// is asserted byte-for-byte below. Regenerate the golden deliberately and inspect
// the diff if the output format changes.
func goldenDiagnostic() diagnostic.Diagnostic {
	d := diagnostic.New()
	d.ConfigHash = "abc123"
	d.Metrics = []diagnostic.MetricResult{
		{Name: "encapsulation", Value: 1.0, Display: "1.00", Band: bandStrong, Confidence: confHigh},
		{Name: "coverage", Value: 1.0, Display: "1.00", Band: bandStrong, Confidence: confHigh},
		{Name: "cycle", Value: 0, Display: "0", Band: bandStrong, Confidence: confHigh},
		{Name: "blast_radius", Value: 0, Display: "0", Band: bandInfo, Confidence: confHigh},
		{Name: "instability", Value: 0, Display: "0", Band: bandInfo, Confidence: confHigh},
		{Name: "propagation_cost", Value: 0.05, Display: "0.05", Band: bandInfo, Confidence: confHigh},
		{Name: "structural_weight", Value: 0, Display: "0", Band: bandInfo, Confidence: confHigh},
		{Name: "hidden_coupling", Value: 0, Display: "0", Band: bandInfo, Confidence: confHigh},
		{Name: "functional_candidates", Value: 0, Display: "0", Band: bandInfo, Confidence: confHigh},
		{Name: "change_coupling", Value: 0, Display: "0", Band: bandInfo, Confidence: confHigh},
		{Name: "change_amplification", Value: 0, Display: "0", Band: bandInfo, Confidence: confHigh},
		{Name: "architecture_fitness", Value: 1.0, Display: "3/3", Band: bandInfo, Confidence: confHigh},
	}
	d.Findings = []finding.Finding{
		{
			ID: "a->b", RuleID: "bc/imbalanced_coupling", Kind: "advisory",
			Status: finding.StatusNew, Severity: sevMedium,
			Edge: finding.EdgeEvidence{From: finding.Endpoint{Module: "a"}, To: finding.Endpoint{Module: "b"}},
			MatchedBy: map[string]string{
				"strength": "functional", "distance": "cross_module_same_owner",
				"volatility": sevMedium, "score_value": "5", "score_band": sevMedium, "group_count": "2",
			},
		},
	}
	d.ToolCoverage = []diagnostic.Coverage{
		{Tool: "go/packages", Status: "ok"},
		{Tool: "scip", Status: "ok"},
		{Tool: "gitnexus", Status: "ok"},
		{Tool: "lizard", Status: "ok"},
		{Tool: "jscpd", Status: "ok"},
	}
	return d
}

const golden = `# archfit scorecard

**Rubric version:** 1
**Overall:** 92/100 (strong)
**Config hash:** ` + "`abc123`" + `

## Dimensions

### boundary_integrity — 100/100 (strong) · confidence: high
no gate-level boundary violations; intended boundaries hold
- encapsulation 1.00 (strong)
- 0 active gate violations

### coupling_balance — 50/100 (mixed) · confidence: high
coupling carries elevated maintenance effort but no distributed-monolith edges
- 2 BC edges (1 rollups); weighted mean maintenance-effort 5.0/10
- worst-case high/high/high (distributed-monolith) edges: 0

### dependency_graph_health — 99/100 (strong) · confidence: high
internal dependency-graph shape: cycles, blast-radius hubs, instability, and propagation cost (not external dependency hygiene)
- import cycles: 0
- blast-radius hubs: 0
- unstable modules (I>0.7): 0
- propagation cost: 0.05

### cohesion_modularity — 100/100 (strong) · confidence: high
cohesion: god modules, hidden coupling, and duplication (cohesion = high strength + low distance is healthy, not penalised)
- god modules (LOC skew): 0
- hidden-coupling pairs: 0
- cross-module duplication pairs: 0

### change_locality — 100/100 (strong) · confidence: high
change locality: how much change crosses intended module boundaries
- co-changing module pairs: 0
- change-amplifying hubs: 0

### architecture_fitness — 100/100 (strong) · confidence: high
architecture intent enforced by executable fitness checks
- enforcement signals present: 3/3

### analysis_confidence — 100/100 (strong) · confidence: high · meta (scores the review, not the architecture)
review trustworthiness given tool coverage and evidence depth
- file extraction coverage 1.00
- scip: ok
- gitnexus: ok
- lizard: ok
- jscpd: ok
`

// TestRenderer_Golden asserts the exact rendered scorecard for a fixed Diagnostic.
func TestRenderer_Golden(t *testing.T) {
	var buf bytes.Buffer
	if err := New().Render(goldenDiagnostic(), &buf); err != nil {
		t.Fatalf("Render: %v", err)
	}
	if got := buf.String(); got != golden {
		t.Errorf("scorecard output mismatch:\n--- got ---\n%s\n--- want ---\n%s", got, golden)
	}
}

// TestRenderer_DoubleRun asserts the scorecard format is deterministic: two
// renders of the same Diagnostic are byte-identical (CI determinism gate).
func TestRenderer_DoubleRun(t *testing.T) {
	d := goldenDiagnostic()
	var a, b bytes.Buffer
	if err := New().Render(d, &a); err != nil {
		t.Fatalf("Render a: %v", err)
	}
	if err := New().Render(d, &b); err != nil {
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

// TestRenderer_RequiredToolsMissing asserts the coverage-gap block renders after
// the dimensions so absent evidence is never mistaken for a strong result.
func TestRenderer_RequiredToolsMissing(t *testing.T) {
	d := goldenDiagnostic()
	d.CoverageGaps = []diagnostic.CoverageGap{
		{Tool: "go/packages", InstallCmd: "https://go.dev/dl", AffectedMetrics: []string{"coverage", "coupling_balance"}, Gate: "warn"},
	}

	var buf bytes.Buffer
	if err := New().Render(d, &buf); err != nil {
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

// TestRenderer_Delta asserts the compact delta-count block renders for a delta
// run and is omitted when there is no delta.
func TestRenderer_Delta(t *testing.T) {
	d := goldenDiagnostic()
	d.Delta = &diagnostic.DeltaReport{
		New:             []string{"n1", "n2"},
		SeverityChanged: []string{"s1"},
		TouchedByDelta:  []string{"t1"},
		Resolved:        []string{"r1"},
	}

	var buf bytes.Buffer
	if err := New().Render(d, &buf); err != nil {
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

	// Absent when no delta.
	var plain bytes.Buffer
	if err := New().Render(goldenDiagnostic(), &plain); err != nil {
		t.Fatalf("Render plain: %v", err)
	}
	if strings.Contains(plain.String(), "## Delta") {
		t.Errorf("non-delta scorecard should not contain a Delta block\nfull output:\n%s", plain.String())
	}
}

// TestRenderer_RequiredToolsMissingAbsentWhenEmpty asserts the section is omitted
// when every required tool ran (no coverage gap).
func TestRenderer_RequiredToolsMissingAbsentWhenEmpty(t *testing.T) {
	var buf bytes.Buffer
	if err := New().Render(goldenDiagnostic(), &buf); err != nil {
		t.Fatalf("Render: %v", err)
	}
	if strings.Contains(buf.String(), "Required tools missing") {
		t.Errorf("required-tools section should be omitted when no gap\nfull output:\n%s", buf.String())
	}
}

// TestRenderer_EmptyDiagnostic asserts the renderer never panics on a near-empty
// Diagnostic and still emits all seven dimension headers.
func TestRenderer_EmptyDiagnostic(t *testing.T) {
	var buf bytes.Buffer
	if err := New().Render(diagnostic.New(), &buf); err != nil {
		t.Fatalf("Render: %v", err)
	}
	out := buf.String()
	for _, dim := range []string{
		"boundary_integrity", "coupling_balance", "dependency_graph_health",
		"cohesion_modularity", "change_locality", "architecture_fitness",
		"analysis_confidence",
	} {
		if !strings.Contains(out, "### "+dim+" ") {
			t.Errorf("missing dimension header for %q in:\n%s", dim, out)
		}
	}
}
