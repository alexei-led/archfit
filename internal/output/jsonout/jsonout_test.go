package jsonout_test

import (
	"bytes"
	"encoding/json"
	"testing"

	"github.com/alexei-led/archfit/internal/assessment/finding"
	"github.com/alexei-led/archfit/internal/model/report"
	"github.com/alexei-led/archfit/internal/output/jsonout"
	"github.com/alexei-led/archfit/internal/relationship/coupling"
	reporttest "github.com/alexei-led/archfit/internal/testutil/report"
)

const (
	connascenceExecutionTest        = "execution"
	dynamicTargetRabbitMQ           = "rabbitmq"
	semanticUnknownJSON             = "unknown"
	distanceCandidateRuntimeEdges   = "runtime_async_edges"
	distanceCandidateModuleApp      = "app"
	distanceCandidateKindMessageQ   = "message_queue"
	distanceCandidatePublisherFile  = "app/publisher.go"
	distanceCandidateExternalAction = "external_systems"
	jsonHigh                        = "high"
	jsonLow                         = "low"
	dimCouplingBalance              = "coupling_balance"
)

// TestJSONRenderer_AdvisoryScoreFields asserts the numeric BC score fields
// (score_value + score_band) on an advisory's matched_by survive JSON encoding.
func TestJSONRenderer_AdvisoryScoreFields(t *testing.T) {
	d := report.NewDocument()
	d.Verdict = report.VerdictPass
	d.Findings = reporttest.Findings(finding.Finding{
		ID:     "adv1",
		Kind:   "advisory",
		RuleID: "bc/imbalanced_coupling",
		MatchedBy: map[string]string{
			"score":       "multiplicative",
			"score_value": "7",
			"score_band":  jsonHigh,
		},
	})

	var buf bytes.Buffer
	if err := jsonout.New().Render(d, &buf); err != nil {
		t.Fatalf("Render() error = %v", err)
	}

	var got report.Document
	if err := json.Unmarshal(buf.Bytes(), &got); err != nil {
		t.Fatalf("cannot unmarshal: %v", err)
	}
	mb := got.Findings[0].MatchedBy
	if mb["score_value"] != "7" {
		t.Errorf("matched_by.score_value = %q, want %q", mb["score_value"], "7")
	}
	if mb["score_band"] != jsonHigh {
		t.Errorf("matched_by.score_band = %q, want %q", mb["score_band"], jsonHigh)
	}
}

// TestJSONRenderer_ScoreVersion asserts the top-level score_version field is
// always present in JSON output and pins the current version literal —
// consumers key on it to detect breaking metric changes, and a version bump
// must be a deliberate, test-visible act.
func TestJSONRenderer_AdvisoryTasks(t *testing.T) {
	d := report.NewDocument()
	d.AdvisoryTasks = []report.AdvisoryTask{{
		FindingID:    "rollup-1",
		RuleID:       "bc/imbalanced_coupling",
		Status:       string(finding.StatusNew),
		Severity:     string(finding.SeverityHigh),
		GroupCount:   3,
		GroupMembers: []string{"id1", "id2"},
		Goal:         "Review grouped advisories.",
		CheapestMove: "reduce_distance",
		ScoreValue:   8,
		TopFiles:     []string{"a.go", "b.go"},
		Constraints:  []string{"report-only"},
		Validation:   []string{"archfit check"},
	}}

	var buf bytes.Buffer
	if err := jsonout.New().Render(d, &buf); err != nil {
		t.Fatalf("Render() error = %v", err)
	}

	var got report.Document
	if err := json.Unmarshal(buf.Bytes(), &got); err != nil {
		t.Fatalf("cannot unmarshal: %v", err)
	}
	if len(got.AdvisoryTasks) != 1 {
		t.Fatalf("advisory_tasks = %d, want 1", len(got.AdvisoryTasks))
	}
	task := got.AdvisoryTasks[0]
	if task.GroupCount != 3 || task.CheapestMove != "reduce_distance" || task.TopFiles[0] != "a.go" {
		t.Fatalf("advisory task did not round-trip: %+v", task)
	}
}

func TestJSONRenderer_ScoreVersion(t *testing.T) {
	var buf bytes.Buffer
	if err := jsonout.New().Render(report.NewDocument(), &buf); err != nil {
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
	if got != report.ScoreVersion {
		t.Errorf("score_version = %q, out of sync with report.ScoreVersion %q", got, report.ScoreVersion)
	}
}

func TestJSONRenderer_ScorecardProjectionAndComparableDelta(t *testing.T) {
	const mixedBand = "mixed"
	d := report.NewDocument()
	d.Score = report.Scorecard{
		Overall: 46, OverallBand: mixedBand,
		Dimensions: []report.Dimension{{Name: dimCouplingBalance, Value: 46, Band: mixedBand}},
	}
	d.BaseScore = &report.Scorecard{
		Overall: 40, OverallBand: mixedBand,
		Dimensions: []report.Dimension{{Name: "modularity", Value: 55, Band: mixedBand}},
	}

	var buf bytes.Buffer
	if err := jsonout.New().Render(d, &buf); err != nil {
		t.Fatalf("Render() error = %v", err)
	}
	var raw struct {
		Score struct {
			Overall int `json:"overall"`
		} `json:"score"`
		CouplingBalance *struct {
			Name  string `json:"name"`
			Value int    `json:"value"`
		} `json:"coupling_balance"`
		ScoreDelta struct {
			OverallDelta int               `json:"overall_delta"`
			Dimensions   []json.RawMessage `json:"dimensions"`
		} `json:"score_delta"`
	}
	if err := json.Unmarshal(buf.Bytes(), &raw); err != nil {
		t.Fatalf("decode scorecard JSON: %v", err)
	}
	if raw.Score.Overall != 46 || raw.CouplingBalance == nil || raw.CouplingBalance.Name != dimCouplingBalance || raw.CouplingBalance.Value != 46 {
		t.Fatalf("score projection lost data: %+v", raw)
	}
	if raw.ScoreDelta.OverallDelta != 6 {
		t.Fatalf("overall delta = %d, want 6", raw.ScoreDelta.OverallDelta)
	}
	if len(raw.ScoreDelta.Dimensions) != 0 {
		t.Fatalf("head-only dimension must not be compared against an invented zero base: %s", raw.ScoreDelta.Dimensions)
	}
}

func TestJSONRenderer_ClassifiedEdgesDistanceTransparency(t *testing.T) {
	d := report.NewDocument()
	d.DistanceContext = &report.DistanceContext{
		OwnerModel:                "single_owner_degenerate",
		DistanceBasis:             map[string]int{"code_structure": 2, "ownership": 1},
		DeployUnitDetectedModules: 1,
		DeclaredExternalSystems:   1,
		RuntimeAsyncRelations:     2,
		RuntimeAsyncKinds:         map[string]int{"message_queue": 1, "event_bus": 1},
		Interpretation:            "same-owner is the lowest cross-module distance",
		RuntimeInterpretation:     "async runtime bridges reduce lifecycle coupling and therefore increase perceived distance",
	}
	d.ClassifiedEdges = &report.ClassifiedEdgeSummary{
		Scored:           3,
		ConnectedModules: 2,
		ByDistanceBasis:  map[string]int{"code_structure": 2, "ownership": 1},
		TailRisk: &report.CouplingTailRiskSummary{
			WorstBalance:          2,
			LowerDecileBalance:    2,
			HighOrWorseEdges:      1,
			HighOrWorseSharePct:   33,
			CriticalEdges:         1,
			CloneOnlyScored:       1,
			CloneOnlyWorstBalance: 4,
		},
		DistanceCompression: &report.DistanceCompressionSummary{
			CompressedMiddleRungs:       true,
			ImplementedRungs:            []int{2, 4, 7, 9, 10},
			OmittedRungs:                []int{3, 5, 6, 8},
			CodeStructureBoundaryCounts: []report.DistanceCount{{Value: 2, Count: 3}, {Value: 5, Count: 1}},
			CodeStructureAncestorDepths: []report.DistanceCount{{Value: 0, Count: 1}, {Value: 1, Count: 3}},
			OmittedRungReasons: []report.DistanceOmittedRungReason{
				{Rung: 8, Reason: "declared external_systems use D=10"},
			},
			Rationale: "D=3/D=5/D=6/D=8 remain compressed",
		},
	}

	var buf bytes.Buffer
	if err := jsonout.New().Render(d, &buf); err != nil {
		t.Fatalf("Render() error = %v", err)
	}

	var got report.Document
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
	if got.DistanceContext.RuntimeAsyncRelations != 2 || got.DistanceContext.RuntimeAsyncKinds["message_queue"] != 1 {
		t.Fatalf("distance_context runtime evidence = %+v", got.DistanceContext)
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
	if spans := got.ClassifiedEdges.DistanceCompression.CodeStructureBoundaryCounts; len(spans) != 2 || spans[0].Value != 2 || spans[1].Value != 5 {
		t.Fatalf("boundary counts = %+v, want values 2 and 5", spans)
	}
	if depths := got.ClassifiedEdges.DistanceCompression.CodeStructureAncestorDepths; len(depths) != 2 || depths[0].Value != 0 || depths[1].Value != 1 {
		t.Fatalf("ancestor depths = %+v, want values 0 and 1", depths)
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

func TestJSONRenderer_VolatilityCorroboration(t *testing.T) {
	d := report.NewDocument()
	d.VolatilityCorroboration = &report.VolatilityCorroboration{
		Source:         "git_history",
		Status:         "ok",
		CommitWindow:   500,
		CommitsScanned: 42,
		ModulesTouched: 2,
		TopTouched: []report.VolatilityTouch{
			{Module: "cmd/archfit", TouchCommits: 12, DeclaredVolatility: jsonHigh},
			{Module: "internal/output", TouchCommits: 5, DeclaredVolatility: jsonLow},
		},
		Caveat: "Supporting evidence only.",
	}

	var buf bytes.Buffer
	if err := jsonout.New().Render(d, &buf); err != nil {
		t.Fatalf("Render() error = %v", err)
	}

	var got report.Document
	if err := json.Unmarshal(buf.Bytes(), &got); err != nil {
		t.Fatalf("output is not valid JSON: %v", err)
	}
	if got.VolatilityCorroboration == nil {
		t.Fatal("volatility_corroboration missing from JSON output")
	}
	if got.VolatilityCorroboration.Status != "ok" || got.VolatilityCorroboration.ModulesTouched != 2 {
		t.Fatalf("volatility_corroboration = %+v", got.VolatilityCorroboration)
	}
	if len(got.VolatilityCorroboration.TopTouched) != 2 || got.VolatilityCorroboration.TopTouched[0].Module != "cmd/archfit" {
		t.Fatalf("top_touched = %+v", got.VolatilityCorroboration.TopTouched)
	}
}

func TestJSONRenderer_ConnascenceStrengthInferred(t *testing.T) {
	d := report.NewDocument()
	d.Connascence = &report.ConnascenceReport{
		EdgesWithEvidence:     2,
		AbstainedEdges:        1,
		TotalEvidence:         3,
		StrengthInferredEdges: 1,
		ByKind: map[string]int{
			string(coupling.ConnascenceAlgorithm): 1,
			string(coupling.ConnascenceMeaning):   1,
			string(coupling.ConnascenceName):      1,
		},
	}

	var buf bytes.Buffer
	if err := jsonout.New().Render(d, &buf); err != nil {
		t.Fatalf("Render() error = %v", err)
	}

	var got report.Document
	if err := json.Unmarshal(buf.Bytes(), &got); err != nil {
		t.Fatalf("output is not valid JSON: %v", err)
	}
	if got.Connascence == nil || got.Connascence.StrengthInferredEdges != 1 {
		t.Fatalf("connascence = %+v, want strength_inferred_edges=1", got.Connascence)
	}
}

func TestJSONRenderer_SemanticStrengthOverlay(t *testing.T) {
	d := report.NewDocument()
	d.SemanticStrengthOverlay = &report.SemanticStrengthOverlay{
		ByLanguage: map[string]report.SemanticStrengthOverlayStats{
			"python": {CandidateEdges: 3, Applied: 2, Missed: 1, Before: map[string]int{semanticUnknownJSON: 3}, After: map[string]int{"intrusive": 2, semanticUnknownJSON: 1}},
			"rust":   {CandidateEdges: 2, Applied: 0, Missed: 2, Before: map[string]int{semanticUnknownJSON: 2}, After: map[string]int{semanticUnknownJSON: 2}},
		},
	}

	var buf bytes.Buffer
	if err := jsonout.New().Render(d, &buf); err != nil {
		t.Fatalf("Render() error = %v", err)
	}

	var raw struct {
		SemanticStrengthOverlay report.SemanticStrengthOverlay `json:"semantic_strength_overlay"`
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
	d := report.NewDocument()
	d.Connascence = &report.ConnascenceReport{
		EdgesWithEvidence: 1,
		AbstainedEdges:    2,
		TotalEvidence:     2,
		ByKind:            map[string]int{"name": 1, "type": 1},
		BySource:          map[string]int{"go/types": 2},
		Unmeasured:        []string{"position", connascenceExecutionTest},
		Roadmap: []report.ConnascenceRoadmapItem{
			{Kind: "name", CurrentStatus: "deterministic_static", Sources: []string{"go/types"}},
			{Kind: connascenceExecutionTest, CurrentStatus: "unmeasured_dynamic", RelatedSignals: []string{distanceCandidateRuntimeEdges}, UpgradeTrigger: "deterministic runtime ordering"},
		},
	}

	var buf bytes.Buffer
	if err := jsonout.New().Render(d, &buf); err != nil {
		t.Fatalf("Render() error = %v", err)
	}

	var raw struct {
		Connascence report.ConnascenceReport `json:"connascence"`
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
	d := report.NewDocument()
	d.DynamicConnascenceSignals = &report.DynamicConnascenceSignals{
		ReportOnlyReason: "runtime trace evidence is absent",
		Unmeasured:       []string{connascenceExecutionTest, string(coupling.ConnascenceTiming)},
		Signals: []report.DynamicConnascenceSignal{{
			Kind:               "runtime_async",
			RelatedConnascence: []string{connascenceExecutionTest, string(coupling.ConnascenceTiming)},
			Measured:           false,
			ReportOnlyReason:   "runtime trace evidence is absent",
			Module:             distanceCandidateModuleApp,
			Target:             dynamicTargetRabbitMQ,
			IntegrationKind:    distanceCandidateKindMessageQ,
			Count:              2,
			Sites: []report.DynamicConnascenceSite{{
				File: distanceCandidatePublisherFile, Line: 12, Kind: distanceCandidateKindMessageQ, Language: "go", Target: dynamicTargetRabbitMQ,
			}},
		}},
	}

	var buf bytes.Buffer
	if err := jsonout.New().Render(d, &buf); err != nil {
		t.Fatalf("Render() error = %v", err)
	}

	var raw struct {
		Dynamic report.DynamicConnascenceSignals `json:"dynamic_connascence_signals"`
	}
	if err := json.Unmarshal(buf.Bytes(), &raw); err != nil {
		t.Fatalf("output is not valid JSON: %v", err)
	}
	if len(raw.Dynamic.Signals) != 1 {
		t.Fatalf("dynamic connascence signals = %+v, want one", raw.Dynamic.Signals)
	}
	got := raw.Dynamic.Signals[0]
	if got.Kind != "runtime_async" || got.Measured || got.Module != distanceCandidateModuleApp || got.Target != dynamicTargetRabbitMQ || got.Count != 2 {
		t.Fatalf("dynamic connascence signal = %+v", got)
	}
	if len(got.RelatedConnascence) != 2 || got.RelatedConnascence[0] != connascenceExecutionTest {
		t.Fatalf("related connascence = %+v", got.RelatedConnascence)
	}
	if len(raw.Dynamic.Unmeasured) != 2 || raw.Dynamic.Unmeasured[1] != string(coupling.ConnascenceTiming) {
		t.Fatalf("unmeasured = %+v", raw.Dynamic.Unmeasured)
	}
}

func TestJSONRenderer_DistanceConfigCandidates(t *testing.T) {
	d := report.NewDocument()
	d.DistanceConfigCandidates = []report.DistanceConfigCandidate{{
		SourceBlock:           distanceCandidateRuntimeEdges,
		Module:                distanceCandidateModuleApp,
		Target:                dynamicTargetRabbitMQ,
		IntegrationKind:       distanceCandidateKindMessageQ,
		Count:                 2,
		SuggestedReviewAction: distanceCandidateExternalAction,
		EvidenceSites: []report.DistanceConfigEvidenceSite{{
			File: distanceCandidatePublisherFile, Line: 12, Kind: distanceCandidateKindMessageQ, Language: "go", Target: dynamicTargetRabbitMQ,
		}},
	}}

	var buf bytes.Buffer
	if err := jsonout.New().Render(d, &buf); err != nil {
		t.Fatalf("Render() error = %v", err)
	}

	var raw struct {
		Candidates []report.DistanceConfigCandidate `json:"distance_config_candidates"`
	}
	if err := json.Unmarshal(buf.Bytes(), &raw); err != nil {
		t.Fatalf("output is not valid JSON: %v", err)
	}
	if len(raw.Candidates) != 1 {
		t.Fatalf("distance config candidates = %+v, want one", raw.Candidates)
	}
	got := raw.Candidates[0]
	if got.SourceBlock != distanceCandidateRuntimeEdges || got.Module != distanceCandidateModuleApp || got.Target != dynamicTargetRabbitMQ || got.SuggestedReviewAction != distanceCandidateExternalAction {
		t.Fatalf("distance config candidate = %+v", got)
	}
	if len(got.EvidenceSites) != 1 || got.EvidenceSites[0].File != distanceCandidatePublisherFile {
		t.Fatalf("distance config candidate sites = %+v", got.EvidenceSites)
	}
}

// TestJSONRenderer_Delta verifies the delta block round-trips with snake_case
// bucket keys, omits empty buckets, and is omitted entirely when nil.
func TestJSONRenderer_Delta(t *testing.T) {
	d := report.NewDocument()
	d.Verdict = report.VerdictWarn
	d.Delta = &report.DeltaReport{
		New:      []string{"n1"},
		Resolved: []string{"r1"},
	}

	var buf bytes.Buffer
	if err := jsonout.New().Render(d, &buf); err != nil {
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

	var got report.Document
	if err := json.Unmarshal(buf.Bytes(), &got); err != nil {
		t.Fatalf("cannot unmarshal back into Diagnostic: %v", err)
	}
	if got.Delta == nil || len(got.Delta.New) != 1 || got.Delta.New[0] != "n1" {
		t.Errorf("round-trip delta = %+v, want New=[n1]", got.Delta)
	}

	// Omitted entirely when nil.
	plain := report.NewDocument()
	var pbuf bytes.Buffer
	if err := jsonout.New().Render(plain, &pbuf); err != nil {
		t.Fatalf("Render() error = %v", err)
	}
	if bytes.Contains(pbuf.Bytes(), []byte("\"delta\"")) {
		t.Errorf("delta should be omitted when nil\noutput: %s", pbuf.String())
	}
}

// TestJSONRenderer_GitFindingDelta pins the --base git-origin block: it rides
// the embedded report document (no envelope field of its own), keeps every ID list a
// non-null array, and is omitted entirely on a run without --base.
func TestJSONRenderer_GitFindingDelta(t *testing.T) {
	d := report.NewDocument()
	d.GitFindingDelta = &report.GitFindingDelta{
		BaseRef:                 "main",
		ComparisonStatus:        report.GitComparisonUnknown,
		IntroducedFindingIDs:    []string{},
		PreExistingFindingIDs:   []string{"f1"},
		UnknownOriginFindingIDs: []string{"f2"},
		ComparisonReasons:       []string{"scip: head ok, base absent"},
	}

	var buf bytes.Buffer
	if err := jsonout.New().Render(d, &buf); err != nil {
		t.Fatalf("Render() error = %v", err)
	}
	var envelope struct {
		GitFindingDelta json.RawMessage `json:"git_finding_delta"`
	}
	if err := json.Unmarshal(buf.Bytes(), &envelope); err != nil {
		t.Fatalf("output is not valid JSON: %v", err)
	}
	if bytes.Contains(envelope.GitFindingDelta, []byte("null")) {
		t.Errorf("git_finding_delta lists must never render as null: %s", envelope.GitFindingDelta)
	}

	var got report.Document
	if err := json.Unmarshal(buf.Bytes(), &got); err != nil {
		t.Fatalf("cannot unmarshal back into Diagnostic: %v", err)
	}
	if got.GitFindingDelta == nil {
		t.Fatalf("git_finding_delta missing from JSON output: %s", buf.String())
	}
	if got.GitFindingDelta.BaseRef != "main" || got.GitFindingDelta.ComparisonStatus != report.GitComparisonUnknown {
		t.Errorf("round-trip git_finding_delta = %+v", got.GitFindingDelta)
	}
	if len(got.GitFindingDelta.PreExistingFindingIDs) != 1 || got.GitFindingDelta.PreExistingFindingIDs[0] != "f1" {
		t.Errorf("pre_existing_finding_ids = %v, want [f1]", got.GitFindingDelta.PreExistingFindingIDs)
	}

	var pbuf bytes.Buffer
	if err := jsonout.New().Render(report.NewDocument(), &pbuf); err != nil {
		t.Fatalf("Render() error = %v", err)
	}
	if bytes.Contains(pbuf.Bytes(), []byte("\"git_finding_delta\"")) {
		t.Errorf("git_finding_delta should be omitted without --base\noutput: %s", pbuf.String())
	}
}

func TestJSONRenderer_Format(t *testing.T) {
	if got := jsonout.New().Format(); got != "legacy-json" {
		t.Errorf("legacy Format() = %q, want %q", got, "legacy-json")
	}
	if got := jsonout.NewState().Format(); got != "json" {
		t.Errorf("primary Format() = %q, want %q", got, "json")
	}
}

// TestStateRenderer_RendersTheContractAtTheRoot pins the primary JSON shape: a
// consumer reads .verdict, .decision, and the nine .dimensions keys without
// unwrapping an envelope, and no repository scalar is reachable from the root.
func TestStateRenderer_RendersTheContractAtTheRoot(t *testing.T) {
	doc := report.Document{State: report.NewArchitectureState()}
	doc.State.Verdict = report.StateNeedsAttention

	var buf bytes.Buffer
	if err := jsonout.NewState().Render(doc, &buf); err != nil {
		t.Fatalf("Render: %v", err)
	}

	var root map[string]json.RawMessage
	if err := json.Unmarshal(buf.Bytes(), &root); err != nil {
		t.Fatalf("decode: %v\n%s", err, buf.String())
	}
	for _, key := range []string{"schema_version", "verdict", "decision", "comparison", "measurement", "dimensions", "coverage", "findings", "agent_tasks", "seams"} {
		if _, present := root[key]; !present {
			t.Errorf("primary JSON is missing the %q key", key)
		}
	}
	for _, forbidden := range []string{"score", "overall", "overall_band", "band", "score_version", dimCouplingBalance, "score_delta"} {
		if _, present := root[forbidden]; present {
			t.Errorf("primary JSON carries a top-level %q key: the architecture state has no repository scalar", forbidden)
		}
	}
	var dims map[string]json.RawMessage
	if err := json.Unmarshal(root["dimensions"], &dims); err != nil {
		t.Fatalf("decode dimensions: %v", err)
	}
	if len(dims) != report.DimensionCount {
		t.Errorf("dimensions = %d keys, want %d", len(dims), report.DimensionCount)
	}
}

// TestStateRenderer_IsByteStable: two renders of the same document must not
// differ. A format whose bytes move between identical runs cannot carry a
// comparable baseline at all.
func TestStateRenderer_IsByteStable(t *testing.T) {
	doc := report.Document{State: report.NewArchitectureState()}
	doc.State.Measurement.ToolVersions = map[string]string{"go/packages": "1.24", "loc": "1", "ast-grep": "0.2"}

	var first, second bytes.Buffer
	for _, w := range []*bytes.Buffer{&first, &second} {
		if err := jsonout.NewState().Render(doc, w); err != nil {
			t.Fatalf("Render: %v", err)
		}
	}
	if first.String() != second.String() {
		t.Errorf("two renders differ:\n%s\n%s", first.String(), second.String())
	}
}

func TestJSONRenderer_Render(t *testing.T) {
	tests := []struct {
		name    string
		diag    report.Document
		verdict report.Verdict
	}{
		{
			name: "pass verdict",
			diag: func() report.Document {
				d := report.NewDocument()
				d.Verdict = report.VerdictPass
				return d
			}(),
			verdict: report.VerdictPass,
		},
		{
			name: "fail verdict",
			diag: func() report.Document {
				d := report.NewDocument()
				d.Verdict = report.VerdictFail
				return d
			}(),
			verdict: report.VerdictFail,
		},
		{
			name: "warn verdict",
			diag: func() report.Document {
				d := report.NewDocument()
				d.Verdict = report.VerdictWarn
				return d
			}(),
			verdict: report.VerdictWarn,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := jsonout.New()
			var buf bytes.Buffer

			if err := r.Render(tt.diag, &buf); err != nil {
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
			if sv != report.SchemaVersion {
				t.Errorf("schema_version = %q, want %q", sv, report.SchemaVersion)
			}

			// Verify round-trip: unmarshal back into Diagnostic.
			var got report.Document
			if err := json.Unmarshal(buf.Bytes(), &got); err != nil {
				t.Fatalf("cannot unmarshal back into Diagnostic: %v", err)
			}
			if got.Verdict != tt.verdict {
				t.Errorf("verdict = %q, want %q", got.Verdict, tt.verdict)
			}
			if got.SchemaVersion != report.SchemaVersion {
				t.Errorf("SchemaVersion = %q, want %q", got.SchemaVersion, report.SchemaVersion)
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
	d := report.NewDocument()
	d.FileFacts = []report.FileFact{
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
	if err := jsonout.New().Render(d, &buf); err != nil {
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

	var got report.Document
	if err := json.Unmarshal(buf.Bytes(), &got); err != nil {
		t.Fatalf("cannot unmarshal back into Diagnostic: %v", err)
	}
	if got.FileFacts[0].LOC != 310 {
		t.Errorf("round-trip LOC = %d, want 310", got.FileFacts[0].LOC)
	}
}
