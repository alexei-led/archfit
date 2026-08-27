package sarif_test

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"

	"github.com/alexei-led/archfit/internal/assessment/finding"
	reportmodel "github.com/alexei-led/archfit/internal/model/report"
	"github.com/alexei-led/archfit/internal/output/sarif"
	"github.com/alexei-led/archfit/internal/relationship"
	reporttest "github.com/alexei-led/archfit/internal/testutil/report"
)

const (
	ruleInternal = "no_internal_access"
	fpGate       = "f-gate"
	fileA        = "pkg/a/a.go"
)

func sampleDiagnostic() reportmodel.Document {
	d := reportmodel.NewDocument()
	d.Verdict = reportmodel.VerdictFail
	d.Base = "main"
	d.Head = "HEAD"
	d.Metrics = []reportmodel.MetricResult{{Name: "cycle", Value: 0, Band: "green"}}
	d.Findings = reporttest.Findings(
		finding.Finding{
			ID: fpGate, Kind: "gate", RuleID: ruleInternal,
			Status: finding.StatusNew, Severity: finding.SeverityHigh,
			Edge: finding.EdgeEvidence{
				From: finding.Endpoint{Path: fileA},
				To:   finding.Endpoint{Path: "pkg/b/internal/impl.go"},
			},
			Locations: []relationship.Location{{File: fileA, Line: 5}},
			Why:       "a uses b internals",
		},
		finding.Finding{
			ID: "f-adv", Kind: "advisory", RuleID: "bc/imbalanced_coupling",
			Status: finding.StatusNew, Severity: finding.SeverityMedium,
			Edge: finding.EdgeEvidence{From: finding.Endpoint{Path: "pkg/c/c.go"}},
		},
		finding.Finding{
			ID: "f-base", Kind: "gate", RuleID: ruleInternal,
			Status: finding.StatusBaseline,
			Edge:   finding.EdgeEvidence{From: finding.Endpoint{Path: "pkg/d/d.go"}},
		},
	)
	d.State = reportmodel.NewArchitectureState()
	d.State.Verdict = reportmodel.StateBlocked
	d.State.Decision = reportmodel.StateDecision{
		HardGates: reportmodel.HardGateFail, ActiveBlockers: 1, AttentionDimensions: 1, UnknownDimensions: 8,
	}
	d.State.Dimensions.Structure.Findings = []reportmodel.FindingRef{
		{ID: fpGate, RuleID: ruleInternal, Kind: reportmodel.FindingKindGate},
	}
	d.State.Dimensions.Complexity.Metrics = []reportmodel.MetricValue{
		{Name: "max_dependency_chain", Value: 3, Unit: "count", Denominator: &reportmodel.MetricDenominator{Observed: 2, Total: 2}},
	}
	d.State.Dimensions.Complexity.Unknown = []reportmodel.UnknownFact{{
		Fact: "cognitive complexity", Reason: "not collected", Owner: reportmodel.OwnerComplexity,
	}}
	d.State.Coverage = reportmodel.StateCoverage{Measured: 1, Partial: 0, Unmeasured: 8}
	d.State.Seams = []reportmodel.Seam{{ID: "seam-ab", FromModule: "a", ToModule: "b"}}
	return d
}

func render(t *testing.T, d reportmodel.Document) (string, map[string]any) {
	t.Helper()
	var buf bytes.Buffer
	if err := sarif.New().Render(d, &buf); err != nil {
		t.Fatalf("Render: %v", err)
	}
	var doc map[string]any
	if err := json.Unmarshal(buf.Bytes(), &doc); err != nil {
		t.Fatalf("output is not valid JSON: %v", err)
	}
	return buf.String(), doc
}

func TestRenderer_Format(t *testing.T) {
	if got := sarif.New().Format(); got != "sarif" {
		t.Errorf("Format() = %q, want sarif", got)
	}
}

func TestRender_ShapeAndLevels(t *testing.T) {
	out, doc := render(t, sampleDiagnostic())

	if doc["version"] != "2.1.0" {
		t.Errorf("version = %v, want 2.1.0", doc["version"])
	}
	if !strings.Contains(out, "sarif-2.1.0.json") {
		t.Error("missing $schema reference")
	}

	runs := doc["runs"].([]any)
	if len(runs) != 1 {
		t.Fatalf("runs = %d, want 1", len(runs))
	}
	run := runs[0].(map[string]any)

	// Rules: two distinct IDs, sorted.
	rules := run["tool"].(map[string]any)["driver"].(map[string]any)["rules"].([]any)
	if len(rules) != 2 {
		t.Fatalf("rules = %d, want 2 distinct", len(rules))
	}
	if rules[0].(map[string]any)["id"] != "bc/imbalanced_coupling" {
		t.Errorf("rules not sorted: first = %v", rules[0])
	}

	// Results: 3 findings → error, warning, note.
	results := run["results"].([]any)
	if len(results) != 3 {
		t.Fatalf("results = %d, want 3", len(results))
	}
	levels := map[string]string{}
	for _, raw := range results {
		res := raw.(map[string]any)
		levels[res["fingerprints"].(map[string]any)["archfit/v1"].(string)] = res["level"].(string)
	}
	want := map[string]string{fpGate: "error", "f-adv": "warning", "f-base": "note"}
	for fp, lvl := range want {
		if levels[fp] != lvl {
			t.Errorf("level[%s] = %q, want %q", fp, levels[fp], lvl)
		}
	}

	// Location with line for the gate finding.
	first := results[0].(map[string]any)
	loc := first["locations"].([]any)[0].(map[string]any)["physicalLocation"].(map[string]any)
	if loc["artifactLocation"].(map[string]any)["uri"] != fileA {
		t.Errorf("location uri = %v", loc)
	}
	if loc["region"].(map[string]any)["startLine"] != float64(5) {
		t.Errorf("startLine = %v, want 5", loc["region"])
	}

	// The architecture state, the metrics, and the seam ledger ride in run
	// properties: SARIF is exempt from human-layout parity, not fact parity.
	props := run["properties"].(map[string]any)
	if props["verdict"] != string(reportmodel.StateBlocked) {
		t.Errorf("properties.verdict = %v, want %q", props["verdict"], reportmodel.StateBlocked)
	}
	if props["schema_version"] != reportmodel.StateSchemaVersion {
		t.Errorf("properties.schema_version = %v, want %q", props["schema_version"], reportmodel.StateSchemaVersion)
	}
	if dims := props["dimensions"].([]any); len(dims) != reportmodel.DimensionCount {
		t.Errorf("properties.dimensions = %d, want %d", len(dims), reportmodel.DimensionCount)
	} else {
		for _, raw := range dims {
			dim := raw.(map[string]any)
			if dim["name"] == "complexity" {
				metrics := dim["metrics"].([]any)
				if len(metrics) != 1 || metrics[0].(map[string]any)["name"] != "max_dependency_chain" {
					t.Errorf("complexity metrics = %v", metrics)
				}
				unknown := dim["unknown"].([]any)
				if len(unknown) != 1 || unknown[0].(map[string]any)["fact"] != "cognitive complexity" {
					t.Errorf("complexity unknown = %v", unknown)
				}
			}
		}
	}
	if len(props["seams"].([]any)) != 1 {
		t.Errorf("properties.seams = %v", props["seams"])
	}
	if props["decision"].(map[string]any)["hard_gates"] != string(reportmodel.HardGateFail) {
		t.Errorf("properties.decision = %v", props["decision"])
	}
	if len(props["metrics"].([]any)) != 1 {
		t.Errorf("properties.metrics = %v", props["metrics"])
	}

	// No timestamps anywhere (determinism).
	if strings.Contains(out, "startTimeUtc") || strings.Contains(out, "endTimeUtc") {
		t.Error("SARIF output must not contain timestamps")
	}
}

func TestRender_Deterministic(t *testing.T) {
	d := sampleDiagnostic()
	var a, b bytes.Buffer
	if err := sarif.New().Render(d, &a); err != nil {
		t.Fatal(err)
	}
	if err := sarif.New().Render(d, &b); err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(a.Bytes(), b.Bytes()) {
		t.Error("double render differs — must be byte-identical")
	}
}

func TestRender_EmptyDiagnostic(t *testing.T) {
	d := reportmodel.NewDocument()
	d.Verdict = reportmodel.VerdictPass
	_, doc := render(t, d)
	run := doc["runs"].([]any)[0].(map[string]any)
	if results := run["results"].([]any); len(results) != 0 {
		t.Errorf("results = %d, want 0", len(results))
	}
}

// TestRender_ResultPropertiesCarryStateGrouping: a result keeps its rule ID and
// fingerprint unchanged, and gains the dimension that owns it plus an explicit
// gate flag, so a consumer never has to re-derive blocker-vs-diagnostic from
// kind and status.
func TestRender_ResultPropertiesCarryStateGrouping(t *testing.T) {
	_, doc := render(t, sampleDiagnostic())
	results := doc["runs"].([]any)[0].(map[string]any)["results"].([]any)

	byFingerprint := map[string]map[string]any{}
	for _, raw := range results {
		res := raw.(map[string]any)
		byFingerprint[res["fingerprints"].(map[string]any)["archfit/v1"].(string)] = res
	}

	gate := byFingerprint[fpGate]
	if gate == nil {
		t.Fatal("the gate finding lost its archfit/v1 fingerprint")
	}
	if gate["ruleId"] != ruleInternal {
		t.Errorf("ruleId = %v, want %q — finding identity must survive the state cutover", gate["ruleId"], ruleInternal)
	}
	props := gate["properties"].(map[string]any)
	if props["gate"] != true {
		t.Errorf("gate flag = %v, want true", props["gate"])
	}
	if props["dimension"] != reportmodel.DimensionStructure {
		t.Errorf("dimension = %v, want %q", props["dimension"], reportmodel.DimensionStructure)
	}

	// A finding no dimension references (baselined) carries no dimension key
	// rather than an invented one.
	baselined := byFingerprint["f-base"]["properties"].(map[string]any)
	if _, present := baselined["dimension"]; present {
		t.Errorf("a baselined finding must not be attributed to a dimension: %v", baselined)
	}
	if baselined["gate"] != true {
		t.Errorf("a baselined gate finding keeps its kind: %v", baselined)
	}
}
