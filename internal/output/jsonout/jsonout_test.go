package jsonout_test

import (
	"bytes"
	"encoding/json"
	"testing"

	"github.com/alexei-led/archfit/internal/model/coupling"
	"github.com/alexei-led/archfit/internal/model/diagnostic"
	"github.com/alexei-led/archfit/internal/model/finding"
	"github.com/alexei-led/archfit/internal/output/jsonout"
	"github.com/alexei-led/archfit/internal/score"
)

const (
	connascenceExecutionTest = "execution"
	dynamicTargetRabbitMQ    = "rabbitmq"
	semanticUnknownJSON      = "unknown"
)

// TestJSONRenderer_AdvisoryScoreFields asserts the numeric BC score fields
// (score_value + score_band) on an advisory's matched_by survive JSON encoding.
func TestJSONRenderer_AdvisoryScoreFields(t *testing.T) {
	d := diagnostic.New()
	d.Verdict = diagnostic.VerdictPass
	d.Findings = []finding.Finding{{
		ID:     "adv1",
		Kind:   "advisory",
		RuleID: "bc/imbalanced_coupling",
		MatchedBy: map[string]string{
			"score":       "multiplicative",
			"score_value": "7",
			"score_band":  "high",
		},
	}}

	var buf bytes.Buffer
	if err := jsonout.New().Render(d, score.Scorecard{}, nil, &buf); err != nil {
		t.Fatalf("Render() error = %v", err)
	}

	var got diagnostic.Diagnostic
	if err := json.Unmarshal(buf.Bytes(), &got); err != nil {
		t.Fatalf("cannot unmarshal: %v", err)
	}
	mb := got.Findings[0].MatchedBy
	if mb["score_value"] != "7" {
		t.Errorf("matched_by.score_value = %q, want %q", mb["score_value"], "7")
	}
	if mb["score_band"] != "high" {
		t.Errorf("matched_by.score_band = %q, want %q", mb["score_band"], "high")
	}
}

// TestJSONRenderer_ScoreVersion asserts the top-level score_version field is
// always present in JSON output and pins the current version literal —
// consumers key on it to detect breaking metric changes, and a version bump
// must be a deliberate, test-visible act.
func TestJSONRenderer_ScoreVersion(t *testing.T) {
	var buf bytes.Buffer
	if err := jsonout.New().Render(diagnostic.New(), score.Scorecard{}, nil, &buf); err != nil {
		t.Fatalf("Render() error = %v", err)
	}

	var raw map[string]any
	if err := json.Unmarshal(buf.Bytes(), &raw); err != nil {
		t.Fatalf("output is not valid JSON: %v", err)
	}
	got, ok := raw["score_version"]
	if !ok {
		t.Fatal("score_version field missing from JSON output")
	}
	if got != "bc_score.v6" {
		t.Errorf("score_version = %q, want %q", got, "bc_score.v6")
	}
	if got != coupling.ScoreVersion {
		t.Errorf("score_version = %q, out of sync with coupling.ScoreVersion %q", got, coupling.ScoreVersion)
	}
}

func TestJSONRenderer_ClassifiedEdgesDistanceTransparency(t *testing.T) {
	d := diagnostic.New()
	d.DistanceContext = &diagnostic.DistanceContext{
		OwnerModel:                "single_owner_degenerate",
		DistanceBasis:             map[string]int{"code_structure": 2, "ownership": 1},
		DeployUnitDetectedModules: 1,
		DeclaredExternalSystems:   1,
		Interpretation:            "same-owner is the lowest cross-module distance",
	}
	d.ClassifiedEdges = &diagnostic.ClassifiedEdgeSummary{
		Scored:           3,
		ConnectedModules: 2,
		ByDistanceBasis:  map[string]int{"code_structure": 2, "ownership": 1},
		TailRisk: &diagnostic.CouplingTailRiskSummary{
			WorstBalance:          2,
			LowerDecileBalance:    2,
			HighOrWorseEdges:      1,
			HighOrWorseSharePct:   33,
			CriticalEdges:         1,
			CloneOnlyScored:       1,
			CloneOnlyWorstBalance: 4,
		},
		DistanceCompression: &diagnostic.DistanceCompressionSummary{
			CompressedMiddleRungs: true,
			ImplementedRungs:      []int{2, 4, 7, 9, 10},
			OmittedRungs:          []int{3, 5, 6, 8},
			OmittedRungReasons: []diagnostic.DistanceOmittedRungReason{
				{Rung: 8, Reason: "declared external_systems use D=10"},
			},
			Rationale: "D=3/D=5/D=6/D=8 remain compressed",
		},
	}

	var buf bytes.Buffer
	if err := jsonout.New().Render(d, score.Scorecard{}, nil, &buf); err != nil {
		t.Fatalf("Render() error = %v", err)
	}

	var got diagnostic.Diagnostic
	if err := json.Unmarshal(buf.Bytes(), &got); err != nil {
		t.Fatalf("output is not valid JSON: %v", err)
	}
	if got.DistanceContext == nil {
		t.Fatal("distance_context missing from JSON output")
	}
	if got.DistanceContext.OwnerModel != "single_owner_degenerate" {
		t.Fatalf("distance_context.owner_model = %q", got.DistanceContext.OwnerModel)
	}
	if got.DistanceContext.DeployUnitDetectedModules != 1 || got.DistanceContext.DeclaredExternalSystems != 1 {
		t.Fatalf("distance_context evidence counts = %+v", got.DistanceContext)
	}
	if got.ClassifiedEdges == nil {
		t.Fatal("classified_edges missing from JSON output")
	}
	if got.ClassifiedEdges.ConnectedModules != 2 {
		t.Errorf("connected_modules = %d, want 2", got.ClassifiedEdges.ConnectedModules)
	}
	if got.ClassifiedEdges.ByDistanceBasis["code_structure"] != 2 {
		t.Errorf("by_distance_basis.code_structure = %d, want 2", got.ClassifiedEdges.ByDistanceBasis["code_structure"])
	}
	if got.ClassifiedEdges.DistanceCompression == nil || !got.ClassifiedEdges.DistanceCompression.CompressedMiddleRungs {
		t.Fatalf("distance_compression = %+v, want compressed_middle_rungs=true", got.ClassifiedEdges.DistanceCompression)
	}
	if reasons := got.ClassifiedEdges.DistanceCompression.OmittedRungReasons; len(reasons) != 1 || reasons[0].Rung != 8 {
		t.Fatalf("omitted_rung_reasons = %+v, want D=8 reason", reasons)
	}
	if got.ClassifiedEdges.TailRisk == nil {
		t.Fatal("tail_risk missing from JSON output")
	}
	if got.ClassifiedEdges.TailRisk.WorstBalance != 2 || got.ClassifiedEdges.TailRisk.HighOrWorseEdges != 1 {
		t.Fatalf("tail_risk = %+v, want worst=2 high_or_worse=1", got.ClassifiedEdges.TailRisk)
	}
}

func TestJSONRenderer_SemanticStrengthOverlay(t *testing.T) {
	d := diagnostic.New()
	d.SemanticStrengthOverlay = &diagnostic.SemanticStrengthOverlay{
		ByLanguage: map[string]diagnostic.SemanticStrengthOverlayStats{
			"python": {CandidateEdges: 3, Applied: 2, Missed: 1, Before: map[string]int{semanticUnknownJSON: 3}, After: map[string]int{"intrusive": 2, semanticUnknownJSON: 1}},
			"rust":   {CandidateEdges: 2, Applied: 0, Missed: 2, Before: map[string]int{semanticUnknownJSON: 2}, After: map[string]int{semanticUnknownJSON: 2}},
		},
	}

	var buf bytes.Buffer
	if err := jsonout.New().Render(d, score.Scorecard{}, nil, &buf); err != nil {
		t.Fatalf("Render() error = %v", err)
	}

	var raw struct {
		SemanticStrengthOverlay diagnostic.SemanticStrengthOverlay `json:"semantic_strength_overlay"`
	}
	if err := json.Unmarshal(buf.Bytes(), &raw); err != nil {
		t.Fatalf("output is not valid JSON: %v", err)
	}
	got := raw.SemanticStrengthOverlay.ByLanguage["python"]
	if got.CandidateEdges != 3 || got.Applied != 2 || got.Missed != 1 {
		t.Fatalf("Python overlay = %+v, want candidate/applied/missed 3/2/1", got)
	}
	if got.Before[semanticUnknownJSON] != 3 || got.After["intrusive"] != 2 {
		t.Fatalf("Python overlay distributions = before %+v after %+v", got.Before, got.After)
	}
	gotRust := raw.SemanticStrengthOverlay.ByLanguage["rust"]
	if gotRust.CandidateEdges != 2 || gotRust.Applied != 0 || gotRust.Missed != 2 {
		t.Fatalf("Rust zero-hit overlay = %+v, want candidate/applied/missed 2/0/2", gotRust)
	}
}

func TestJSONRenderer_ConnascenceSummary(t *testing.T) {
	d := diagnostic.New()
	d.Connascence = &diagnostic.ConnascenceReport{
		EdgesWithEvidence: 1,
		AbstainedEdges:    2,
		TotalEvidence:     2,
		ByKind:            map[string]int{"name": 1, "type": 1},
		BySource:          map[string]int{"go/types": 2},
		Unmeasured:        []string{"position", connascenceExecutionTest},
		Roadmap: []diagnostic.ConnascenceRoadmapItem{
			{Kind: "name", CurrentStatus: "deterministic_static", Sources: []string{"go/types"}},
			{Kind: connascenceExecutionTest, CurrentStatus: "unmeasured_dynamic", RelatedSignals: []string{"runtime_async_edges"}, UpgradeTrigger: "deterministic runtime ordering"},
		},
	}

	var buf bytes.Buffer
	if err := jsonout.New().Render(d, score.Scorecard{}, nil, &buf); err != nil {
		t.Fatalf("Render() error = %v", err)
	}

	var raw struct {
		Connascence diagnostic.ConnascenceReport `json:"connascence"`
	}
	if err := json.Unmarshal(buf.Bytes(), &raw); err != nil {
		t.Fatalf("output is not valid JSON: %v", err)
	}
	if raw.Connascence.EdgesWithEvidence != 1 || raw.Connascence.AbstainedEdges != 2 || raw.Connascence.TotalEvidence != 2 {
		t.Fatalf("connascence summary = %+v, want counters 1/2/2", raw.Connascence)
	}
	if raw.Connascence.ByKind["type"] != 1 || raw.Connascence.BySource["go/types"] != 2 {
		t.Fatalf("connascence maps = kinds %+v sources %+v", raw.Connascence.ByKind, raw.Connascence.BySource)
	}
	if len(raw.Connascence.Roadmap) != 2 {
		t.Fatalf("connascence roadmap = %+v, want 2 entries", raw.Connascence.Roadmap)
	}
	if got := raw.Connascence.Roadmap[1]; got.Kind != connascenceExecutionTest || got.CurrentStatus != "unmeasured_dynamic" || len(got.RelatedSignals) != 1 {
		t.Fatalf("dynamic roadmap entry = %+v, want execution/unmeasured_dynamic with related signal", got)
	}
}

func TestJSONRenderer_DynamicConnascenceSignals(t *testing.T) {
	d := diagnostic.New()
	d.DynamicConnascenceSignals = &diagnostic.DynamicConnascenceSignals{
		ReportOnlyReason: "runtime trace evidence is absent",
		Unmeasured:       []string{connascenceExecutionTest, string(coupling.ConnascenceTiming)},
		Signals: []diagnostic.DynamicConnascenceSignal{{
			Kind:               "runtime_async",
			RelatedConnascence: []string{connascenceExecutionTest, string(coupling.ConnascenceTiming)},
			Measured:           false,
			ReportOnlyReason:   "runtime trace evidence is absent",
			Module:             "app",
			Target:             dynamicTargetRabbitMQ,
			IntegrationKind:    "message_queue",
			Count:              2,
			Sites: []diagnostic.DynamicConnascenceSite{{
				File: "app/publisher.go", Line: 12, Kind: "message_queue", Language: "go", Target: dynamicTargetRabbitMQ,
			}},
		}},
	}

	var buf bytes.Buffer
	if err := jsonout.New().Render(d, score.Scorecard{}, nil, &buf); err != nil {
		t.Fatalf("Render() error = %v", err)
	}

	var raw struct {
		Dynamic diagnostic.DynamicConnascenceSignals `json:"dynamic_connascence_signals"`
	}
	if err := json.Unmarshal(buf.Bytes(), &raw); err != nil {
		t.Fatalf("output is not valid JSON: %v", err)
	}
	if len(raw.Dynamic.Signals) != 1 {
		t.Fatalf("dynamic connascence signals = %+v, want one", raw.Dynamic.Signals)
	}
	got := raw.Dynamic.Signals[0]
	if got.Kind != "runtime_async" || got.Measured || got.Module != "app" || got.Target != dynamicTargetRabbitMQ || got.Count != 2 {
		t.Fatalf("dynamic connascence signal = %+v", got)
	}
	if len(got.RelatedConnascence) != 2 || got.RelatedConnascence[0] != connascenceExecutionTest {
		t.Fatalf("related connascence = %+v", got.RelatedConnascence)
	}
	if len(raw.Dynamic.Unmeasured) != 2 || raw.Dynamic.Unmeasured[1] != string(coupling.ConnascenceTiming) {
		t.Fatalf("unmeasured = %+v", raw.Dynamic.Unmeasured)
	}
}

// TestJSONRenderer_Delta verifies the delta block round-trips with snake_case
// bucket keys, omits empty buckets, and is omitted entirely when nil.
func TestJSONRenderer_Delta(t *testing.T) {
	d := diagnostic.New()
	d.Verdict = diagnostic.VerdictWarn
	d.Delta = &diagnostic.DeltaReport{
		New:      []string{"n1"},
		Resolved: []string{"r1"},
	}

	var buf bytes.Buffer
	if err := jsonout.New().Render(d, score.Scorecard{}, nil, &buf); err != nil {
		t.Fatalf("Render() error = %v", err)
	}

	var raw struct {
		Delta map[string]any `json:"delta"`
	}
	if err := json.Unmarshal(buf.Bytes(), &raw); err != nil {
		t.Fatalf("output is not valid JSON: %v", err)
	}
	if raw.Delta == nil {
		t.Fatal("delta block missing from JSON output")
	}
	if _, ok := raw.Delta["new"]; !ok {
		t.Error("delta.new missing")
	}
	if _, ok := raw.Delta["resolved"]; !ok {
		t.Error("delta.resolved missing")
	}
	if _, ok := raw.Delta["existing"]; ok {
		t.Error("delta.existing should be omitted when empty")
	}

	var got diagnostic.Diagnostic
	if err := json.Unmarshal(buf.Bytes(), &got); err != nil {
		t.Fatalf("cannot unmarshal back into Diagnostic: %v", err)
	}
	if got.Delta == nil || len(got.Delta.New) != 1 || got.Delta.New[0] != "n1" {
		t.Errorf("round-trip delta = %+v, want New=[n1]", got.Delta)
	}

	// Omitted entirely when nil.
	plain := diagnostic.New()
	var pbuf bytes.Buffer
	if err := jsonout.New().Render(plain, score.Scorecard{}, nil, &pbuf); err != nil {
		t.Fatalf("Render() error = %v", err)
	}
	if bytes.Contains(pbuf.Bytes(), []byte("\"delta\"")) {
		t.Errorf("delta should be omitted when nil\noutput: %s", pbuf.String())
	}
}

func TestJSONRenderer_Format(t *testing.T) {
	r := jsonout.New()
	if got := r.Format(); got != "json" {
		t.Errorf("Format() = %q, want %q", got, "json")
	}
}

func TestJSONRenderer_Render(t *testing.T) {
	tests := []struct {
		name    string
		diag    diagnostic.Diagnostic
		verdict diagnostic.Verdict
	}{
		{
			name:    "pass verdict",
			diag:    func() diagnostic.Diagnostic { d := diagnostic.New(); d.Verdict = diagnostic.VerdictPass; return d }(),
			verdict: diagnostic.VerdictPass,
		},
		{
			name:    "fail verdict",
			diag:    func() diagnostic.Diagnostic { d := diagnostic.New(); d.Verdict = diagnostic.VerdictFail; return d }(),
			verdict: diagnostic.VerdictFail,
		},
		{
			name:    "warn verdict",
			diag:    func() diagnostic.Diagnostic { d := diagnostic.New(); d.Verdict = diagnostic.VerdictWarn; return d }(),
			verdict: diagnostic.VerdictWarn,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := jsonout.New()
			var buf bytes.Buffer

			if err := r.Render(tt.diag, score.Scorecard{}, nil, &buf); err != nil {
				t.Fatalf("Render() error = %v", err)
			}

			// Output must be valid JSON.
			var raw map[string]any
			if err := json.Unmarshal(buf.Bytes(), &raw); err != nil {
				t.Fatalf("output is not valid JSON: %v\noutput: %s", err, buf.String())
			}

			// schema_version must be present.
			sv, ok := raw["schema_version"]
			if !ok {
				t.Fatal("schema_version field missing from JSON output")
			}
			if sv != diagnostic.SchemaVersion {
				t.Errorf("schema_version = %q, want %q", sv, diagnostic.SchemaVersion)
			}

			// Verify round-trip: unmarshal back into Diagnostic.
			var got diagnostic.Diagnostic
			if err := json.Unmarshal(buf.Bytes(), &got); err != nil {
				t.Fatalf("cannot unmarshal back into Diagnostic: %v", err)
			}
			if got.Verdict != tt.verdict {
				t.Errorf("verdict = %q, want %q", got.Verdict, tt.verdict)
			}
			if got.SchemaVersion != diagnostic.SchemaVersion {
				t.Errorf("SchemaVersion = %q, want %q", got.SchemaVersion, diagnostic.SchemaVersion)
			}

			// agent_tasks must serialize as [] not null.
			agentTasks, ok := raw["agent_tasks"]
			if !ok {
				t.Fatal("agent_tasks field missing from JSON output")
			}
			tasks, ok := agentTasks.([]any)
			if !ok {
				t.Fatalf("agent_tasks is not an array, got %T", agentTasks)
			}
			if len(tasks) != 0 {
				t.Errorf("agent_tasks len = %d, want 0", len(tasks))
			}

			// file_facts must serialize as [] not null.
			fileFacts, ok := raw["file_facts"]
			if !ok {
				t.Fatal("file_facts field missing from JSON output")
			}
			if _, ok := fileFacts.([]any); !ok {
				t.Fatalf("file_facts is not an array, got %T", fileFacts)
			}
		})
	}
}

// TestJSONRenderer_Render_FileFacts verifies the full facts block round-trips
// through JSON with snake_case keys.
func TestJSONRenderer_Render_FileFacts(t *testing.T) {
	d := diagnostic.New()
	d.FileFacts = []diagnostic.FileFact{
		{
			Module:               "tui.polling_state",
			Files:                []string{"src/tui/polling_state.py"},
			InboundModuleFanIn:   23,
			OutboundDestinations: 2,
			LOC:                  310,
		},
		{
			Module: "config",
			Files:  []string{},
		},
	}

	var buf bytes.Buffer
	if err := jsonout.New().Render(d, score.Scorecard{}, nil, &buf); err != nil {
		t.Fatalf("Render() error = %v", err)
	}

	var raw struct {
		FileFacts []map[string]any `json:"file_facts"`
	}
	if err := json.Unmarshal(buf.Bytes(), &raw); err != nil {
		t.Fatalf("output is not valid JSON: %v", err)
	}
	if len(raw.FileFacts) != 2 {
		t.Fatalf("file_facts len = %d, want 2", len(raw.FileFacts))
	}

	first := raw.FileFacts[0]
	if first["module"] != "tui.polling_state" {
		t.Errorf("module = %v, want tui.polling_state", first["module"])
	}
	if first["inbound_module_fanin"] != float64(23) {
		t.Errorf("inbound_module_fanin = %v, want 23", first["inbound_module_fanin"])
	}

	var got diagnostic.Diagnostic
	if err := json.Unmarshal(buf.Bytes(), &got); err != nil {
		t.Fatalf("cannot unmarshal back into Diagnostic: %v", err)
	}
	if got.FileFacts[0].LOC != 310 {
		t.Errorf("round-trip LOC = %d, want 310", got.FileFacts[0].LOC)
	}
}
