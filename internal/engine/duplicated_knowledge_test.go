package engine

import (
	"strings"
	"testing"
	"time"

	"github.com/alexei-led/archfit/internal/baseline"
	"github.com/alexei-led/archfit/internal/classify"
	"github.com/alexei-led/archfit/internal/config"
	"github.com/alexei-led/archfit/internal/model/coupling"
	"github.com/alexei-led/archfit/internal/model/finding"
	"github.com/alexei-led/archfit/internal/model/graph"
)

// Fixture constants for the duplicated-knowledge advisory tests.
const (
	dkFileA   = "services/a/dup.go"
	dkFileB   = "services/b/dup.go"
	dkPairKey = "a\x00b"
)

// dkClassifyCfg returns a two-module config with a cross-module clone pair and
// its evidence, mirroring what buildClonePairSet produces from jscpd clusters.
func dkClassifyCfg() config.ClassifyConfig {
	modules := map[string]config.ModuleDef{
		"a": {Paths: []string{"services/a/**"}},
		"b": {Paths: []string{"services/b/**"}},
	}
	return config.ClassifyConfig{
		Modules:               modules,
		ModuleMap:             config.BuildModuleMap(modules),
		CrossModuleClonePairs: map[string]struct{}{dkPairKey: {}},
		CloneEvidence: map[string][]graph.Location{
			dkPairKey: {{File: dkFileA, Line: 10}, {File: dkFileB, Line: 20}},
		},
	}
}

// findingsByRule filters findings by RuleID.
func findingsByRule(fnds []finding.Finding, rule string) []finding.Finding {
	var out []finding.Finding
	for _, f := range fnds {
		if f.RuleID == rule {
			out = append(out, f)
		}
	}
	return out
}

// TestDuplicatedKnowledgeAdvisory: two modules share a jscpd clone pair but NO
// import edge — the drift is invisible to the symmetric-upgrade path, so a
// report-only bc/duplicated_knowledge advisory must surface it with both
// duplicated-code locations.
func TestDuplicatedKnowledgeAdvisory(t *testing.T) {
	t.Parallel()
	g := graph.Build(nil)
	cfg := dkClassifyCfg()
	idx := classify.Run(g, cfg)
	fnds := collectAdvisories(g, idx, cfg, nil, RunInput{Now: time.Now(), Accepted: baseline.Baseline{}, Waivers: config.WaiverSet{}})

	dk := findingsByRule(fnds, RuleIDBCDuplicatedKnowledge)
	if len(dk) != 1 {
		t.Fatalf("bc/duplicated_knowledge findings = %d, want 1: %+v", len(dk), fnds)
	}
	if bc := findingsByRule(fnds, RuleIDBCImbalanced); len(bc) != 0 {
		t.Fatalf("bc/imbalanced_coupling findings = %d, want 0 for clone-only evidence: %+v", len(bc), bc)
	}
	summary := buildClassifiedEdgeSummary(idx)
	if summary.Total != 0 || summary.Scored != 0 || summary.Abstained != 0 {
		t.Fatalf("classified_edges = total %d scored %d abstained %d, want all zero for report-only clone-only evidence",
			summary.Total, summary.Scored, summary.Abstained)
	}
	f := dk[0]
	if f.Kind != finding.KindAdvisory {
		t.Errorf("Kind = %q, want advisory (report-only)", f.Kind)
	}
	if f.Status != finding.StatusNew {
		t.Errorf("Status = %q, want new", f.Status)
	}
	// Severity derives from the scored pair (symmetric 9 × distance 4 ×
	// volatility 10 → balance 6 → medium), not a hardcoded level.
	if f.Severity != finding.SeverityMedium {
		t.Errorf("Severity = %q, want medium", f.Severity)
	}
	if f.Edge.From.Module != "a" || f.Edge.To.Module != "b" {
		t.Errorf("Edge modules = %s→%s, want a→b", f.Edge.From.Module, f.Edge.To.Module)
	}
	if f.Edge.From.Path != dkFileA || f.Edge.To.Path != dkFileB {
		t.Errorf("Edge paths = %s→%s, want %s→%s", f.Edge.From.Path, f.Edge.To.Path, dkFileA, dkFileB)
	}
	if len(f.Locations) != 2 {
		t.Errorf("Locations = %d, want 2 (both clone sides)", len(f.Locations))
	}
	if got := f.MatchedBy["strength"]; got != string(coupling.StrengthSymmetric) {
		t.Errorf("matched_by.strength = %q, want symmetric", got)
	}
	if got := f.MatchedBy["score_band"]; got != "medium" {
		t.Errorf("matched_by.score_band = %q, want medium", got)
	}
	if got := f.MatchedBy["score_value"]; got != "6" {
		t.Errorf("matched_by.score_value = %q, want 6", got)
	}
	if got := f.MatchedBy["score_version"]; got != coupling.ScoreVersion {
		t.Errorf("matched_by.score_version = %q, want %s", got, coupling.ScoreVersion)
	}
	if got := f.MatchedBy["score_policy"]; got != string(config.DuplicatedKnowledgePolicyScore) {
		t.Errorf("matched_by.score_policy = %q, want %q", got, config.DuplicatedKnowledgePolicyScore)
	}
	// cheapest_move: same_owner is the terminal distance rung (no reduce_distance
	// offered) and the one-rung strength reduction (symmetric→functional) still
	// balances to medium (balance 5, same band) — no single-rung move drops the
	// band, so the book scorer offers none.
	if got := f.MatchedBy["cheapest_move"]; got != "" {
		t.Errorf("matched_by.cheapest_move = %q, want \"\" (no single-rung move drops the medium band)", got)
	}
	if !strings.Contains(f.Why, "duplicated knowledge") {
		t.Errorf("Why = %q, want duplicated-knowledge framing", f.Why)
	}
}

func TestDuplicatedKnowledgeSummaryPolicy(t *testing.T) {
	t.Parallel()
	g := graph.Build(nil)
	cfg := dkClassifyCfg()
	pairs := classify.CloneOnlyPairs(g, cfg)
	if len(pairs) != 1 {
		t.Fatalf("CloneOnlyPairs = %d, want 1", len(pairs))
	}

	scored := buildClassifiedEdgeSummaryWithCloneOnly(coupling.Index{}, pairs, config.DuplicatedKnowledgePolicyScore)
	if scored.CloneOnlyScored != 1 || scored.CloneOnlyAdvisory != 0 {
		t.Fatalf("score policy counters = scored %d advisory %d, want 1/0", scored.CloneOnlyScored, scored.CloneOnlyAdvisory)
	}
	if scored.Total != 1 || scored.Scored != 1 || scored.Abstained != 0 {
		t.Fatalf("score policy summary = total %d scored %d abstained %d, want 1/1/0",
			scored.Total, scored.Scored, scored.Abstained)
	}
	if scored.MeanBalance != 6.0 {
		t.Errorf("score policy MeanBalance = %v, want 6.0", scored.MeanBalance)
	}
	if got := scored.ByStrength[string(coupling.StrengthSymmetric)]; got != 1 {
		t.Errorf("score policy symmetric count = %d, want 1", got)
	}
	if got := scored.BySeverity[string(coupling.SeverityMedium)]; got != 1 {
		t.Errorf("score policy medium count = %d, want 1", got)
	}

	advisory := buildClassifiedEdgeSummaryWithCloneOnly(coupling.Index{}, pairs, config.DuplicatedKnowledgePolicyAdvisory)
	if advisory.CloneOnlyScored != 0 || advisory.CloneOnlyAdvisory != 1 {
		t.Fatalf("advisory policy counters = scored %d advisory %d, want 0/1", advisory.CloneOnlyScored, advisory.CloneOnlyAdvisory)
	}
	if advisory.Total != 0 || advisory.Scored != 0 || advisory.Abstained != 0 {
		t.Fatalf("advisory policy summary = total %d scored %d abstained %d, want 0/0/0",
			advisory.Total, advisory.Scored, advisory.Abstained)
	}
}

// TestDuplicatedKnowledgeAdvisory_EdgeExists: when an import edge connects the
// pair, the existing symmetric-upgrade path owns it — one bc/imbalanced_coupling
// advisory with symmetric strength, and NO duplicate bc/duplicated_knowledge.
func TestDuplicatedKnowledgeAdvisory_EdgeExists(t *testing.T) {
	t.Parallel()
	g := graph.Build([]graph.Facts{{
		Edges: []graph.Edge{{
			From: "file:services/a/a.go", To: "file:services/b/b.go",
			Kind: graph.EdgeKindImports, Language: graph.LangGo,
			StrengthHint: string(coupling.StrengthFunctional),
		}},
		Language: graph.LangGo,
	}})
	cfg := dkClassifyCfg()
	idx := classify.Run(g, cfg)
	fnds := collectAdvisories(g, idx, cfg, nil, RunInput{Now: time.Now(), Accepted: baseline.Baseline{}, Waivers: config.WaiverSet{}})

	if dk := findingsByRule(fnds, RuleIDBCDuplicatedKnowledge); len(dk) != 0 {
		t.Fatalf("bc/duplicated_knowledge findings = %d, want 0 when an edge exists: %+v", len(dk), dk)
	}
	bc := findingsByRule(fnds, RuleIDBCImbalanced)
	if len(bc) != 1 {
		t.Fatalf("bc/imbalanced_coupling findings = %d, want 1 (symmetric upgrade path unchanged)", len(bc))
	}
	if got := bc[0].MatchedBy["strength"]; got != string(coupling.StrengthSymmetric) {
		t.Errorf("matched_by.strength = %q, want symmetric (clone upgrade on the real edge)", got)
	}
}

// TestDuplicatedKnowledgeAdvisory_Suppression covers the two config knobs that
// silence the advisory: a human-approved label accepting the pair, and the
// coupling advisory minimum-severity floor.
func TestDuplicatedKnowledgeAdvisory_Suppression(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		mutate func(*config.ClassifyConfig)
	}{
		{
			name: "approved label accepts the pair",
			mutate: func(c *config.ClassifyConfig) {
				c.ApprovedLabels = map[string]string{dkPairKey: string(coupling.StrengthFunctional)}
			},
		},
		{
			name: "min_severity above the pair's band filters it",
			mutate: func(c *config.ClassifyConfig) {
				c.BCAdvisoryMinSeverity = "high" // pair scores medium
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			cfg := dkClassifyCfg()
			tt.mutate(&cfg)
			fnds := collectAdvisories(graph.Build(nil), coupling.Index{}, cfg, nil, RunInput{Now: time.Now(), Accepted: baseline.Baseline{}, Waivers: config.WaiverSet{}})
			if dk := findingsByRule(fnds, RuleIDBCDuplicatedKnowledge); len(dk) != 0 {
				t.Fatalf("bc/duplicated_knowledge findings = %d, want 0: %+v", len(dk), dk)
			}
		})
	}
}
