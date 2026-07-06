package engine

import (
	"context"
	"strings"
	"testing"

	"github.com/alexei-led/archfit/internal/classify"
	"github.com/alexei-led/archfit/internal/config"
	"github.com/alexei-led/archfit/internal/model/coupling"
	"github.com/alexei-led/archfit/internal/model/finding"
	"github.com/alexei-led/archfit/internal/model/graph"
	"github.com/alexei-led/archfit/internal/ports"
)

const toolNameScipTest = "scip"

// TestBCRiskClause_DistanceAware verifies the advisory text only names
// "distributed-monolith risk" for high-distance critical edges; a low-distance
// critical edge is framed as local coupling, never distributed-monolith (which
// would invite a cargo-cult "introduce a contract" fix).
func TestBCRiskClause_DistanceAware(t *testing.T) {
	mk := func(d coupling.Distance) coupling.Classification {
		return coupling.Classification{Distance: d, Severity: coupling.SeverityCritical}
	}
	if got := bcRiskClause(mk(coupling.DistanceCrossDeployUnit)); !strings.Contains(got, "distributed-monolith risk") {
		t.Errorf("high-distance critical: %q, want distributed-monolith risk", got)
	}
	got := bcRiskClause(mk(coupling.DistanceCrossModuleSameOwner))
	if strings.Contains(got, "distributed-monolith risk") {
		t.Errorf("low-distance critical: %q, must NOT claim distributed-monolith risk", got)
	}
	if !strings.Contains(got, "not a distributed monolith") {
		t.Errorf("low-distance critical: %q, want 'not a distributed monolith' framing", got)
	}

	// High severity splits on the same distance test: "across a boundary" only
	// when the distance really is high; a low-distance high edge names its
	// cascade as contained instead.
	mkHigh := func(d coupling.Distance) coupling.Classification {
		return coupling.Classification{Distance: d, Severity: coupling.SeverityHigh}
	}
	if got := bcRiskClause(mkHigh(coupling.DistanceCrossDeployUnit)); !strings.Contains(got, "across a boundary") {
		t.Errorf("high-distance high: %q, want 'across a boundary' framing", got)
	}
	gotHigh := bcRiskClause(mkHigh(coupling.DistanceCrossModuleSameOwner))
	if strings.Contains(gotHigh, "across a boundary") {
		t.Errorf("low-distance high: %q, must NOT claim a boundary crossing", gotHigh)
	}
	if !strings.Contains(gotHigh, "at low distance") {
		t.Errorf("low-distance high: %q, want 'at low distance' framing", gotHigh)
	}

	// Below high severity the clause is distance-agnostic by design.
	if got := bcRiskClause(coupling.Classification{Severity: coupling.SeverityMedium}); !strings.Contains(got, "unbalanced coupling") {
		t.Errorf("medium severity: %q, want the generic unbalanced-coupling clause", got)
	}
}

// TestBCRiskClause_NamesActualStrength guards against the hardcoded-narrative bug:
// bcRiskClause used to assert "high-strength coupling" for every critical edge
// regardless of matched_by.strength (verified wrong on ccgram: 15/16 critical
// edges were actually StrengthModel, ordinal 3/10 — low). The clause must name
// the real strength level and must never claim "high-strength" for a low-ordinal
// strength.
func TestBCRiskClause_NamesActualStrength(t *testing.T) {
	tests := []struct {
		strength coupling.Strength
		wantWord string
	}{
		{coupling.StrengthContract, "contract"},
		{coupling.StrengthModel, "model"},
		{coupling.StrengthFunctional, "functional"},
		{coupling.StrengthSymmetric, "symmetric"},
		{coupling.StrengthIntrusive, "intrusive"},
	}
	for _, dist := range []coupling.Distance{
		coupling.DistanceCrossModuleSameOwner,
		coupling.DistanceCrossModuleDiffOwner,
		coupling.DistanceCrossDeployUnit,
	} {
		for _, tc := range tests {
			cl := coupling.Classification{
				Strength:   tc.strength,
				Distance:   dist,
				Volatility: coupling.VolatilityHigh,
				Severity:   coupling.SeverityCritical,
			}
			got := bcRiskClause(cl)
			if !strings.Contains(got, tc.wantWord) {
				t.Errorf("strength=%s distance=%s: clause %q does not name actual strength %q", tc.strength, dist, got, tc.wantWord)
			}
			if tc.strength != coupling.StrengthIntrusive && tc.strength != coupling.StrengthSymmetric && strings.Contains(got, "high-strength") {
				t.Errorf("strength=%s distance=%s: clause %q falsely claims high-strength", tc.strength, dist, got)
			}
		}
	}
}

// TestEnrichEdges_GoTypeInfoHintAuthoritative guards the F2 strength-accuracy
// fix: SCIP strength must NOT override a Go edge's compiler-grade type-info hint
// (SCIP-go is coarser), but MUST refine non-Go edges and Go edges with no hint.
func TestEnrichEdges_GoTypeInfoHintAuthoritative(t *testing.T) {
	fn := string(coupling.StrengthFunctional)
	md := string(coupling.StrengthModel)
	scip := map[string]string{
		"a.go\x00pkg/b": fn, // would coarsen the Go model hint
		"c.ts\x00pkg/d": fn, // refines the TS heuristic hint
		"e.go\x00pkg/f": fn, // fills the empty Go hint
	}
	facts := graph.Facts{Edges: []graph.Edge{
		{From: "file:a.go", To: "pkg:pkg/b", Kind: graph.EdgeKindImports, Language: graph.LangGo, StrengthHint: md},
		{From: "file:c.ts", To: "pkg:pkg/d", Kind: graph.EdgeKindImports, Language: "typescript", StrengthHint: md},
		{From: "file:e.go", To: "pkg:pkg/f", Kind: graph.EdgeKindImports, Language: graph.LangGo, StrengthHint: ""},
	}}
	scipConnascence := map[string][]graph.ConnascenceHint{
		"a.go\x00pkg/b": {{Kind: graph.ConnascenceAlgorithm, Source: toolNameScipTest, Detail: "symbol reference"}},
	}
	enrichEdges(context.Background(), ports.NopSymbolResolver{}, scip, scipConnascence, facts)

	want := []string{md, fn, fn}
	for i, w := range want {
		if got := facts.Edges[i].StrengthHint; got != w {
			t.Errorf("edge %d (%s): StrengthHint = %q, want %q", i, facts.Edges[i].Language, got, w)
		}
	}
	if got := facts.Edges[0].ConnascenceHints; len(got) != 1 || got[0].Kind != graph.ConnascenceAlgorithm {
		t.Fatalf("Go edge SCIP connascence = %+v, want algorithm hint appended without strength override", got)
	}
}

func TestBuildConnascenceReport(t *testing.T) {
	idx := coupling.Index{
		"a\x00b\x00imports": {
			Connascence: []coupling.ConnascenceEvidence{
				{Kind: coupling.ConnascenceName, Source: "go/types"},
				{Kind: coupling.ConnascenceType, Source: "go/types"},
			},
		},
		"c\x00d\x00imports": {
			Connascence: []coupling.ConnascenceEvidence{
				{Kind: coupling.ConnascenceAlgorithm, Source: toolNameScipTest},
				{Kind: coupling.ConnascencePosition, Source: toolNameScipTest},
			},
		},
		"e\x00f\x00imports": {},
	}

	r := buildConnascenceReport(idx)
	if r.EdgesWithEvidence != 2 {
		t.Errorf("EdgesWithEvidence = %d, want 2", r.EdgesWithEvidence)
	}
	if r.AbstainedEdges != 1 {
		t.Errorf("AbstainedEdges = %d, want 1", r.AbstainedEdges)
	}
	if r.TotalEvidence != 4 {
		t.Errorf("TotalEvidence = %d, want 4", r.TotalEvidence)
	}
	if r.ByKind[string(coupling.ConnascenceName)] != 1 || r.ByKind[string(coupling.ConnascenceType)] != 1 || r.ByKind[string(coupling.ConnascenceAlgorithm)] != 1 || r.ByKind[string(coupling.ConnascencePosition)] != 1 {
		t.Errorf("ByKind = %+v, want name/type/algorithm/position counts", r.ByKind)
	}
	if r.BySource["go/types"] != 2 || r.BySource[toolNameScipTest] != 2 {
		t.Errorf("BySource = %+v, want go/types=2 scip=2", r.BySource)
	}
	if len(r.Unmeasured) == 0 {
		t.Fatal("Unmeasured is empty; dynamic categories must be disclosed")
	}
	for _, kind := range r.Unmeasured {
		if kind == string(coupling.ConnascencePosition) {
			t.Fatalf("position has deterministic evidence and must not be reported unmeasured: %+v", r.Unmeasured)
		}
	}
}

// TestBuildClassifiedEdgeSummary_DistributedMonolith verifies that the DM counter
// counts only critical-band edges at HIGH distance (different owner / deploy unit).
// A critical edge at cross_module_same_owner is local coupling, not a distributed
// monolith, and must not be counted.
func TestBuildClassifiedEdgeSummary_DistributedMonolith(t *testing.T) {
	key := func(from, to, kind string) string { return from + "\x00" + to + "\x00" + kind }
	crit := coupling.EdgeScore{Scored: true, Balance: 2, Band: coupling.SeverityCritical}
	idx := coupling.Index{
		// critical + high distance (different owner) → distributed-monolith
		key("a", "b", "import"): {Distance: coupling.DistanceCrossModuleDiffOwner, Strength: coupling.StrengthFunctional, Volatility: coupling.VolatilityHigh, Score: crit},
		// critical + high distance (deploy unit) → distributed-monolith
		key("a", "c", "import"): {Distance: coupling.DistanceCrossDeployUnit, Strength: coupling.StrengthIntrusive, Volatility: coupling.VolatilityHigh, Score: crit},
		// critical + LOW distance (same owner) → NOT distributed-monolith (local)
		key("a", "d", "import"): {Distance: coupling.DistanceCrossModuleSameOwner, Strength: coupling.StrengthModel, Volatility: coupling.VolatilityHigh, Score: crit},
		// non-critical + high distance → NOT distributed-monolith
		key("a", "e", "import"): {Distance: coupling.DistanceCrossModuleDiffOwner, Strength: coupling.StrengthContract, Volatility: coupling.VolatilityLow, Score: coupling.EdgeScore{Scored: true, Balance: 8, Band: coupling.SeverityLow}},
	}
	s := buildClassifiedEdgeSummary(idx)
	if s.DistributedMonolith != 2 {
		t.Errorf("DistributedMonolith = %d, want 2 (critical AND high-distance only)", s.DistributedMonolith)
	}
	if got := s.BySeverity[string(coupling.SeverityCritical)]; got != 3 {
		t.Errorf("critical band = %d, want 3 (all three critical edges counted)", got)
	}
}

// TestBuildClassifiedEdgeSummary verifies the aggregate distribution counts and
// MeanBalance computed from a coupling.Index.
func TestBuildClassifiedEdgeSummary(t *testing.T) {
	// edge key format: "from\x00to\x00kind"
	key := func(from, to, kind string) string { return from + "\x00" + to + "\x00" + kind }

	t.Run("empty index → all zeros, MeanBalance=0", func(t *testing.T) {
		s := buildClassifiedEdgeSummary(coupling.Index{})
		if s.Total != 0 {
			t.Errorf("Total = %d, want 0", s.Total)
		}
		if s.Scored != 0 {
			t.Errorf("Scored = %d, want 0", s.Scored)
		}
		if s.Abstained != 0 {
			t.Errorf("Abstained = %d, want 0", s.Abstained)
		}
		if s.SameModule != 0 {
			t.Errorf("SameModule = %d, want 0", s.SameModule)
		}
		if s.MeanBalance != 0.0 {
			t.Errorf("MeanBalance = %v, want 0.0", s.MeanBalance)
		}
	})

	t.Run("mix of same_module, scored, abstained → correct totals and mean", func(t *testing.T) {
		// 2 same_module edges (excluded from balance)
		// 3 scored cross-boundary edges with balances 2, 6, 10 → mean = 6.0
		// 1 abstained cross-boundary edge (strength unknown)
		idx := coupling.Index{
			key("a", "b", "import"): {
				Distance: coupling.DistanceSameModule,
				Strength: coupling.StrengthFunctional,
				Score:    coupling.EdgeScore{Scored: false},
			},
			key("a", "c", "import"): {
				Distance: coupling.DistanceSameModule,
				Strength: coupling.StrengthModel,
				Score:    coupling.EdgeScore{Scored: false},
			},
			// scored: balance 2 → critical band
			key("a", "d", "import"): {
				Distance:   coupling.DistanceCrossDeployUnit,
				Strength:   coupling.StrengthSymmetric,
				Volatility: coupling.VolatilityHigh,
				Score: coupling.EdgeScore{
					Scored:  true,
					Balance: 2,
					Band:    coupling.SeverityCritical,
				},
			},
			// scored: balance 6 → medium band
			key("b", "d", "import"): {
				Distance:   coupling.DistanceCrossModuleDiffOwner,
				Strength:   coupling.StrengthFunctional,
				Volatility: coupling.VolatilityMedium,
				Score: coupling.EdgeScore{
					Scored:  true,
					Balance: 6,
					Band:    coupling.SeverityMedium,
				},
			},
			// scored: balance 10 → none band
			key("b", "e", "import"): {
				Distance:   coupling.DistanceCrossModuleSameOwner,
				Strength:   coupling.StrengthContract,
				Volatility: coupling.VolatilityLow,
				Score: coupling.EdgeScore{
					Scored:  true,
					Balance: 10,
					Band:    coupling.SeverityNone,
				},
			},
			// abstained: unknown strength
			key("c", "e", "import"): {
				Distance:   coupling.DistanceCrossModuleDiffOwner,
				Strength:   coupling.StrengthUnknown,
				Volatility: coupling.VolatilityHigh,
				Score:      coupling.EdgeScore{Scored: false},
			},
		}

		s := buildClassifiedEdgeSummary(idx)

		if s.Total != 6 {
			t.Errorf("Total = %d, want 6", s.Total)
		}
		if s.SameModule != 2 {
			t.Errorf("SameModule = %d, want 2", s.SameModule)
		}
		if s.Scored != 3 {
			t.Errorf("Scored = %d, want 3", s.Scored)
		}
		if s.Abstained != 1 {
			t.Errorf("Abstained = %d, want 1", s.Abstained)
		}
		wantMean := (2.0 + 6.0 + 10.0) / 3.0
		if s.MeanBalance != wantMean {
			t.Errorf("MeanBalance = %v, want %v", s.MeanBalance, wantMean)
		}
		// BySeverity: critical=1, medium=1, none=1, abstained=1
		if s.BySeverity[string(coupling.SeverityCritical)] != 1 {
			t.Errorf("BySeverity[critical] = %d, want 1", s.BySeverity[string(coupling.SeverityCritical)])
		}
		if s.BySeverity[string(coupling.SeverityMedium)] != 1 {
			t.Errorf("BySeverity[medium] = %d, want 1", s.BySeverity[string(coupling.SeverityMedium)])
		}
		if s.BySeverity["abstained"] != 1 {
			t.Errorf("BySeverity[abstained] = %d, want 1", s.BySeverity["abstained"])
		}
		// ByStrength should count only cross-boundary edges (4 total: sym, func, contract, unknown)
		crossTotal := 0
		for _, n := range s.ByStrength {
			crossTotal += n
		}
		if crossTotal != 4 {
			t.Errorf("ByStrength total = %d, want 4 (cross-boundary only)", crossTotal)
		}
		// External should be zero: no DistanceUnknown edges in this fixture.
		if s.External != 0 {
			t.Errorf("External = %d, want 0", s.External)
		}
	})

	t.Run("DistanceUnknown edges excluded from scored/abstained, counted as External", func(t *testing.T) {
		// 1 internal scored edge (cross_module_same_owner, contract, balanced)
		// 1 internal abstained edge (cross_module_diff_owner, unknown strength)
		// 2 external edges (DistanceUnknown): one scored, one not — both go to External
		// This proves the exclusion is keyed on Distance==DistanceUnknown, not on
		// whether the scorer happened to score the edge.
		idx := coupling.Index{
			// internal scored
			key("internal/a", "internal/b", "import"): {
				Distance: coupling.DistanceCrossModuleSameOwner,
				Strength: coupling.StrengthContract,
				Score:    coupling.EdgeScore{Scored: true, Balance: 9, Band: coupling.SeverityNone},
			},
			// internal abstained (known distance, unknown strength)
			key("internal/a", "internal/c", "import"): {
				Distance: coupling.DistanceCrossModuleDiffOwner,
				Strength: coupling.StrengthUnknown,
				Score:    coupling.EdgeScore{Scored: false},
			},
			// external: stdlib/third-party (DistanceUnknown) — NOT scored into balance
			key("internal/a", "fmt", "import"): {
				Distance: coupling.DistanceUnknown,
				Strength: coupling.StrengthFunctional,
				Score:    coupling.EdgeScore{Scored: false},
			},
			// external: synthetic non-Go style (DistanceUnknown) — language-agnostic check
			// Simulates a Rust dependency crate or TS node_modules edge.
			key("crate::mymod", "serde::de", "use"): {
				Distance: coupling.DistanceUnknown,
				Strength: coupling.StrengthUnknown,
				Score:    coupling.EdgeScore{Scored: false},
			},
		}

		s := buildClassifiedEdgeSummary(idx)

		if s.Total != 4 {
			t.Errorf("Total = %d, want 4", s.Total)
		}
		if s.SameModule != 0 {
			t.Errorf("SameModule = %d, want 0", s.SameModule)
		}
		// External edges excluded from coupling_balance denominator.
		if s.External != 2 {
			t.Errorf("External = %d, want 2 (both DistanceUnknown edges)", s.External)
		}
		// Only internal edges counted in Scored/Abstained.
		if s.Scored != 1 {
			t.Errorf("Scored = %d, want 1 (internal scored only)", s.Scored)
		}
		if s.Abstained != 1 {
			t.Errorf("Abstained = %d, want 1 (internal abstained only)", s.Abstained)
		}
		// MeanBalance from internal-only scored edge.
		if s.MeanBalance != 9.0 {
			t.Errorf("MeanBalance = %v, want 9.0 (internal edge only)", s.MeanBalance)
		}
		// ByStrength/ByDistance/ByVolatility must NOT include external edges.
		if s.ByDistance[string(coupling.DistanceUnknown)] != 0 {
			t.Errorf("ByDistance[unknown] = %d, want 0 (external edges not counted in distribution)",
				s.ByDistance[string(coupling.DistanceUnknown)])
		}
		internalCrossTotal := 0
		for _, n := range s.ByStrength {
			internalCrossTotal += n
		}
		if internalCrossTotal != 2 {
			t.Errorf("ByStrength total = %d, want 2 (internal cross-boundary only)", internalCrossTotal)
		}
	})
}

func TestBuildClassifiedEdgeSummary_TailRiskIncludesCloneOnlyContribution(t *testing.T) {
	key := func(from, to, kind string) string { return from + "\x00" + to + "\x00" + kind }
	idx := coupling.Index{
		key("a", "b", "import"): {
			Distance: coupling.DistanceCrossModuleSameOwner,
			Strength: coupling.StrengthContract,
			Score:    coupling.EdgeScore{Scored: true, Balance: 10, Band: coupling.SeverityNone},
		},
		key("a", "c", "import"): {
			Distance: coupling.DistanceCrossModuleDiffOwner,
			Strength: coupling.StrengthFunctional,
			Score:    coupling.EdgeScore{Scored: true, Balance: 4, Band: coupling.SeverityHigh},
		},
		key("a", "d", "import"): {
			Distance: coupling.DistanceCrossDeployUnit,
			Strength: coupling.StrengthIntrusive,
			Score:    coupling.EdgeScore{Scored: true, Balance: 2, Band: coupling.SeverityCritical},
		},
	}
	cloneOnly := []classify.CloneOnlyPair{{
		FromModule: "clone-a",
		ToModule:   "clone-b",
		Classification: coupling.Classification{
			Distance: coupling.DistanceCrossModuleDiffOwner,
			Strength: coupling.StrengthSymmetric,
			Score:    coupling.EdgeScore{Scored: true, Balance: 3, Band: coupling.SeverityHigh},
		},
	}}

	s := buildClassifiedEdgeSummaryWithCloneOnly(idx, cloneOnly, config.DuplicatedKnowledgePolicyScore)

	if s.TailRisk == nil {
		t.Fatal("TailRisk is nil, want scored-edge tail summary")
	}
	if s.TailRisk.WorstBalance != 2 {
		t.Errorf("WorstBalance = %d, want 2", s.TailRisk.WorstBalance)
	}
	if s.TailRisk.LowerDecileBalance != 2 {
		t.Errorf("LowerDecileBalance = %d, want 2", s.TailRisk.LowerDecileBalance)
	}
	if s.TailRisk.HighOrWorseEdges != 3 {
		t.Errorf("HighOrWorseEdges = %d, want 3", s.TailRisk.HighOrWorseEdges)
	}
	if s.TailRisk.HighOrWorseSharePct != 75 {
		t.Errorf("HighOrWorseSharePct = %d, want 75", s.TailRisk.HighOrWorseSharePct)
	}
	if s.TailRisk.CriticalEdges != 1 {
		t.Errorf("CriticalEdges = %d, want 1", s.TailRisk.CriticalEdges)
	}
	if s.TailRisk.DistributedMonolithEdges != 1 {
		t.Errorf("DistributedMonolithEdges = %d, want 1", s.TailRisk.DistributedMonolithEdges)
	}
	if s.TailRisk.CloneOnlyScored != 1 {
		t.Errorf("CloneOnlyScored = %d, want 1", s.TailRisk.CloneOnlyScored)
	}
	if s.TailRisk.CloneOnlyHighOrWorseEdges != 1 {
		t.Errorf("CloneOnlyHighOrWorseEdges = %d, want 1", s.TailRisk.CloneOnlyHighOrWorseEdges)
	}
	if s.TailRisk.CloneOnlyWorstBalance != 3 {
		t.Errorf("CloneOnlyWorstBalance = %d, want 3", s.TailRisk.CloneOnlyWorstBalance)
	}

	s = buildClassifiedEdgeSummaryWithCloneOnly(idx, cloneOnly, config.DuplicatedKnowledgePolicyAdvisory)
	if s.TailRisk.CloneOnlyScored != 0 || s.TailRisk.CloneOnlyHighOrWorseEdges != 0 || s.TailRisk.CloneOnlyWorstBalance != 0 {
		t.Errorf("advisory policy tail risk counted clone-only pair: %+v", s.TailRisk)
	}
}

func TestBuildClassifiedEdgeSummary_DistanceBasisCompressionAndConnectedModules(t *testing.T) {
	key := func(from, to, kind string) string { return from + "\x00" + to + "\x00" + kind }
	modules := map[string]config.ModuleDef{
		"a": {Paths: []string{"a/**"}},
		"b": {Paths: []string{"b/**"}},
		"c": {Paths: []string{"c/**"}},
	}
	idx := coupling.Index{
		key("file:a/x.go", "file:b/y.go", "import"): {
			Distance:      coupling.DistanceCrossModuleSameOwner,
			DistanceBasis: coupling.DistanceBasisStructure,
			Strength:      coupling.StrengthContract,
			Score:         coupling.EdgeScore{Scored: true, Balance: 9, Band: coupling.SeverityNone},
		},
		key("file:b/y.go", "file:c/z.go", "import"): {
			Distance:      coupling.DistanceCrossModuleDiffOwner,
			DistanceBasis: coupling.DistanceBasisOwnership,
			Strength:      coupling.StrengthFunctional,
			Score:         coupling.EdgeScore{Scored: true, Balance: 6, Band: coupling.SeverityMedium},
		},
		key("file:a/x.go", "pkg:github.com/acme/api", "import"): {
			Distance:      coupling.DistanceExternal,
			DistanceBasis: coupling.DistanceBasisExternal,
			Strength:      coupling.StrengthFunctional,
			Score:         coupling.EdgeScore{Scored: true, Balance: 8, Band: coupling.SeverityLow},
		},
	}

	s := buildClassifiedEdgeSummaryForRun(idx, nil, config.DuplicatedKnowledgePolicyAdvisory, config.BuildModuleMap(modules))

	if s.ConnectedModules != 3 {
		t.Errorf("ConnectedModules = %d, want 3", s.ConnectedModules)
	}
	if s.ByDistanceBasis[string(coupling.DistanceBasisStructure)] != 1 {
		t.Errorf("ByDistanceBasis[code_structure] = %d, want 1", s.ByDistanceBasis[string(coupling.DistanceBasisStructure)])
	}
	if s.ByDistanceBasis[string(coupling.DistanceBasisOwnership)] != 1 {
		t.Errorf("ByDistanceBasis[ownership] = %d, want 1", s.ByDistanceBasis[string(coupling.DistanceBasisOwnership)])
	}
	if s.ByDistanceBasis[string(coupling.DistanceBasisExternal)] != 1 {
		t.Errorf("ByDistanceBasis[declared_external] = %d, want 1", s.ByDistanceBasis[string(coupling.DistanceBasisExternal)])
	}
	if s.DistanceCompression == nil {
		t.Fatal("DistanceCompression is nil")
	}
	if !s.DistanceCompression.CompressedMiddleRungs {
		t.Error("CompressedMiddleRungs = false, want true")
	}
	if !strings.Contains(s.DistanceCompression.Rationale, "D=3") || !strings.Contains(s.DistanceCompression.Rationale, "D=8") {
		t.Errorf("Rationale = %q, want D=3 and D=8 compression disclosure", s.DistanceCompression.Rationale)
	}
}

// TestBuildClassifiedEdgeSummary_DeclaredExternal pins the D=10 rung's summary
// arithmetic: declared external edges enter the scored distribution and count
// in DeclaredExternal, while the undeclared remainder keeps the External exclusion.
func TestBuildClassifiedEdgeSummary_DeclaredExternal(t *testing.T) {
	key := func(from, to, kind string) string { return from + "\x00" + to + "\x00" + kind }
	// 1 internal scored edge, 1 declared-external scored edge (D=10),
	// 1 declared-external abstained edge (unknown strength — abstain-not-fake
	// holds at the new rung), 1 undeclared external (excluded as before).
	idx := coupling.Index{
		key("internal/a", "internal/b", "import"): {
			Distance: coupling.DistanceCrossModuleSameOwner,
			Strength: coupling.StrengthContract,
			Score:    coupling.EdgeScore{Scored: true, Balance: 9, Band: coupling.SeverityNone},
		},
		key("internal/a", "github.com/aws/aws-sdk-go-v2/service/s3", "import"): {
			Distance: coupling.DistanceExternal,
			Strength: coupling.StrengthFunctional,
			Score:    coupling.EdgeScore{Scored: true, Balance: 8, Band: coupling.SeverityLow},
		},
		key("internal/a", "github.com/aws/aws-sdk-go-v2/service/sqs", "import"): {
			Distance: coupling.DistanceExternal,
			Strength: coupling.StrengthUnknown,
			Score:    coupling.EdgeScore{Scored: false},
		},
		key("internal/a", "fmt", "import"): {
			Distance: coupling.DistanceUnknown,
			Strength: coupling.StrengthFunctional,
			Score:    coupling.EdgeScore{Scored: false},
		},
	}

	s := buildClassifiedEdgeSummary(idx)

	if s.DeclaredExternal != 2 {
		t.Errorf("DeclaredExternal = %d, want 2", s.DeclaredExternal)
	}
	if s.External != 1 {
		t.Errorf("External = %d, want 1 (undeclared only)", s.External)
	}
	// Declared external edges enter the Scored/Abstained distribution.
	if s.Scored != 2 {
		t.Errorf("Scored = %d, want 2 (internal + declared external)", s.Scored)
	}
	if s.Abstained != 1 {
		t.Errorf("Abstained = %d, want 1 (declared external with unknown strength)", s.Abstained)
	}
	if wantMean := (9.0 + 8.0) / 2.0; s.MeanBalance != wantMean {
		t.Errorf("MeanBalance = %v, want %v", s.MeanBalance, wantMean)
	}
	if s.ByDistance[string(coupling.DistanceExternal)] != 2 {
		t.Errorf("ByDistance[declared_external] = %d, want 2", s.ByDistance[string(coupling.DistanceExternal)])
	}
}

// TestBuildClassifiedEdgeSummary_LabeledLLM verifies the labeled_llm bucket
// counts cross-boundary edges whose strength came from an approved
// llm-provenance label — and only those (same-module edges are excluded with
// the rest of the same-module distribution).
func TestBuildClassifiedEdgeSummary_LabeledLLM(t *testing.T) {
	key := func(from, to, kind string) string { return from + "\x00" + to + "\x00" + kind }
	idx := coupling.Index{
		key("internal/a", "internal/b", "import"): {
			Distance:               coupling.DistanceCrossModuleSameOwner,
			Strength:               coupling.StrengthModel,
			StrengthFromLLM:        true,
			StrengthFromNonHighLLM: true,
			Score:                  coupling.EdgeScore{Scored: true, Balance: 7, Band: coupling.SeverityNone},
		},
		key("internal/a", "internal/c", "import"): {
			Distance: coupling.DistanceCrossModuleSameOwner,
			Strength: coupling.StrengthContract,
			Score:    coupling.EdgeScore{Scored: true, Balance: 9, Band: coupling.SeverityNone},
		},
		key("internal/a", "internal/a/sub", "import"): {
			Distance:        coupling.DistanceSameModule,
			Strength:        coupling.StrengthModel,
			StrengthFromLLM: true, // must not count: same-module edges stay out of the distribution
			Score:           coupling.EdgeScore{Scored: true, Balance: 3, Band: coupling.SeverityNone},
		},
	}

	s := buildClassifiedEdgeSummary(idx)

	if s.LabeledLLM != 1 {
		t.Errorf("LabeledLLM = %d, want 1 (cross-boundary llm-filled edge only)", s.LabeledLLM)
	}
	if s.LLMLowConfidenceEdges != 1 {
		t.Errorf("LLMLowConfidenceEdges = %d, want 1 (non-high-confidence applied llm fill)", s.LLMLowConfidenceEdges)
	}
	if s.Scored != 2 {
		t.Errorf("Scored = %d, want 2", s.Scored)
	}
}

// TestGroupEdgePaths pins the honest-edge-path contract for rolled-up BC
// advisories: the (from, to) pair comes from whichever member owns the first
// merged location; with no locations (TS edges carry none) or no owning
// member, the representative's own pair is kept — a real member edge, never
// an empty string that strips the finding's only path evidence.
func TestGroupEdgePaths(t *testing.T) {
	const (
		fromA  = "a/x.go"
		toDest = "shared/z.go"
	)
	member := func(from, to string, locs ...graph.Location) finding.Finding {
		return finding.Finding{
			Edge: finding.EdgeEvidence{
				From: finding.Endpoint{Path: from},
				To:   finding.Endpoint{Path: to},
			},
			Locations: locs,
		}
	}
	locA := graph.Location{File: fromA, Line: 3}
	locB := graph.Location{File: "b/y.go", Line: 7}
	m1 := member(fromA, toDest, locA)
	m2 := member("b/y.go", toDest, locB)

	t.Run("owner of locations[0] wins", func(t *testing.T) {
		from, to := groupEdgePaths([]finding.Finding{m2, m1}, []graph.Location{locA, locB})
		if from != fromA || to != toDest {
			t.Errorf("(from, to) = (%q, %q), want m1's edge (%q, %q)", from, to, fromA, toDest)
		}
	})

	t.Run("empty locations falls back to representative", func(t *testing.T) {
		from, to := groupEdgePaths([]finding.Finding{m1, m2}, nil)
		if from != fromA || to != toDest {
			t.Errorf("(from, to) = (%q, %q), want representative m1's edge (%q, %q)", from, to, fromA, toDest)
		}
	})

	t.Run("no member owning locations[0] falls back to representative", func(t *testing.T) {
		orphan := graph.Location{File: "c/orphan.go", Line: 1}
		from, to := groupEdgePaths([]finding.Finding{m1, m2}, []graph.Location{orphan})
		if from != fromA || to != toDest {
			t.Errorf("(from, to) = (%q, %q), want representative m1's edge (%q, %q)", from, to, fromA, toDest)
		}
	})
}
