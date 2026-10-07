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

// TestRender_CarriesRuleRationaleAndAlternatives pins the rule rationale in
// SARIF: the message is the finding why, which ends with the rationale, and
// the declared alternatives ride in the result properties only when a rule
// declares them.
func TestRender_CarriesRuleRationaleAndAlternatives(t *testing.T) {
	d := sampleDiagnostic()
	d.Findings[0].Why = "a uses b internals — Domain code stays free of I/O"
	d.Findings[0].Alternatives = []string{"Depend on a port"}
	_, doc := render(t, d)
	results := doc["runs"].([]any)[0].(map[string]any)["results"].([]any)
	for _, raw := range results {
		res := raw.(map[string]any)
		props := res["properties"].(map[string]any)
		alternatives, present := props["allowed_alternatives"]
		switch res["fingerprints"].(map[string]any)["archfit/v1"] {
		case fpGate:
			if msg := res["message"].(map[string]any)["text"]; msg != d.Findings[0].Why {
				t.Errorf("message = %v, want the why with its rationale", msg)
			}
			if got, _ := alternatives.([]any); len(got) != 1 || got[0] != "Depend on a port" {
				t.Errorf("allowed_alternatives = %v, want [Depend on a port]", alternatives)
			}
		default:
			if present {
				t.Errorf("a finding with no alternatives carries allowed_alternatives: %v", props)
			}
		}
	}
}

// TestRender_BaselineStateFollowsBaselineMembership pins the SARIF 2.1.0
// baseline fields. baselineState answers one question — is this result in the
// accepted baseline — so it is written only when a baseline file was loaded:
// a baselined finding is unchanged, a finding the baseline no longer sees is
// absent, and every other one is new, a waived one included (the baseline is
// matched before waivers, so a waived finding is not in it). Acceptance is a
// separate fact: a baselined or waived result carries an external, accepted
// suppression. partialFingerprints carry the location-independent finding ID.
func TestRender_BaselineStateFollowsBaselineMembership(t *testing.T) {
	const (
		idNew, idBaseline, idWaived, idExpired, idFixed = "f-new", "f-baseline", "f-waived", "f-expired", "f-fixed"
		stateNew                                        = "new"
	)
	findings := []finding.Finding{
		{ID: idNew, Kind: finding.KindGate, RuleID: ruleInternal, Status: finding.StatusNew},
		{ID: idBaseline, Kind: finding.KindGate, RuleID: ruleInternal, Status: finding.StatusBaseline},
		{ID: idWaived, Kind: finding.KindGate, RuleID: ruleInternal, Status: finding.StatusWaived},
		{ID: idExpired, Kind: finding.KindGate, RuleID: ruleInternal, Status: finding.StatusExpiredWaiver},
		{ID: idFixed, Kind: finding.KindGate, RuleID: ruleInternal, Status: finding.StatusFixed},
	}
	present, missing := true, false
	tests := []struct {
		name      string
		reference *reportmodel.StateComparison
		state     map[string]string // finding ID → baselineState; absent key means no baselineState
	}{
		{name: "a loaded baseline", reference: &reportmodel.StateComparison{BaselinePresent: &present},
			state: map[string]string{idNew: stateNew, idBaseline: "unchanged", idWaived: stateNew, idExpired: stateNew, idFixed: "absent"}},
		{name: "no baseline file", reference: &reportmodel.StateComparison{BaselinePresent: &missing}},
		{name: "no gate reference"},
	}
	wantSuppressed := map[string]bool{idBaseline: true, idWaived: true}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			d := sampleDiagnostic()
			d.Findings = reporttest.Findings(findings...)
			d.State.GateReference = tc.reference
			_, doc := render(t, d)
			for _, raw := range doc["runs"].([]any)[0].(map[string]any)["results"].([]any) {
				res := raw.(map[string]any)
				id := res["fingerprints"].(map[string]any)["archfit/v1"].(string)
				got, _ := res["baselineState"].(string)
				if got != tc.state[id] {
					t.Errorf("%s baselineState = %q, want %q", id, got, tc.state[id])
				}
				sup, _ := res["suppressions"].([]any)
				if wantSuppressed[id] != (len(sup) == 1) {
					t.Errorf("%s suppressions = %v, want suppressed=%t", id, sup, wantSuppressed[id])
				}
				if len(sup) == 1 {
					s := sup[0].(map[string]any)
					if s["kind"] != "external" || s["status"] != "accepted" || s["justification"] == "" {
						t.Errorf("%s suppression = %v, want an external accepted suppression with a justification", id, s)
					}
				}
				pf, _ := res["partialFingerprints"].(map[string]any)
				if pf["primaryLocationLineHash"] != id {
					t.Errorf("%s partialFingerprints = %v, want primaryLocationLineHash = the finding ID", id, pf)
				}
			}
		})
	}
}
