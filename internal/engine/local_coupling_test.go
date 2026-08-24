package engine

import (
	"testing"
	"time"

	"github.com/alexei-led/archfit/internal/baseline"
	"github.com/alexei-led/archfit/internal/model/graph"
	"github.com/alexei-led/archfit/internal/model/module"
	"github.com/alexei-led/archfit/internal/relationship/classify"
	"github.com/alexei-led/archfit/internal/relationship/coupling"
	"github.com/alexei-led/archfit/internal/view"
)

// Fixture constants shared by the local_coupling tests.
const (
	lcSubdomainCore = "core"
	lcBallOfMudFrom = "services/a/x.go"
)

// localCouplingFixture returns the module config, graph, and coupling.Index
// shared by TestBuildLocalCoupling and the same-module-edges-produce-no-
// advisory cross-check (report-only local_coupling must never leak into the
// bc/imbalanced_coupling advisory pipeline): one module ("a", subdomain core)
// with a ball-of-mud same-module edge (critical), a cohesion same-module edge
// (none), an abstained same-module edge (unknown strength), plus one
// cross-module edge into module "b" — the only edge coupling_balance or
// collectAdvisories may ever report on.
func localCouplingFixture() (view.ClassifyConfig, *graph.Graph, coupling.Index) {
	modules := map[string]module.ModuleDef{
		"a": {Paths: []string{testServicesAGlob}, Subdomain: lcSubdomainCore},
		"b": {Paths: []string{testServicesBGlob}, Subdomain: "supporting"},
	}
	cfg := view.ClassifyConfig{Modules: modules, ModuleMap: module.BuildMap(modules)}

	edges := []graph.Edge{
		// Ball of mud: model/same_module/high → balance 2, critical → offender,
		// local-complexity quadrant.
		{
			From: "file:" + lcBallOfMudFrom, To: "file:services/a/y.go",
			Kind: graph.EdgeKindImports, Language: graph.LangGo,
			StrengthHint: string(coupling.StrengthModel),
			Locations:    []graph.Location{{File: lcBallOfMudFrom, Line: 3}},
		},
		// Cohesion: intrusive/same_module/high → balance 9, none → scored, not
		// an offender, not in the complexity quadrant.
		{
			From: "file:services/a/y.go", To: "file:services/a/z.go",
			Kind: graph.EdgeKindImports, Language: graph.LangGo,
			StrengthHint: string(coupling.StrengthIntrusive),
		},
		// Unknown strength at same-module distance → abstained.
		{
			From: "file:services/a/z.go", To: "file:services/a/w.go",
			Kind: graph.EdgeKindImports, Language: graph.LangGo,
		},
		// Cross-module edge — the only one coupling_balance may count.
		{
			From: "file:" + lcBallOfMudFrom, To: "file:services/b/api.go",
			Kind: graph.EdgeKindImports, Language: graph.LangGo,
			StrengthHint: string(coupling.StrengthFunctional),
		},
	}
	g := graph.Build([]graph.Facts{{Edges: edges, Language: graph.LangGo}})
	idx := classify.Run(g, cfg)
	return cfg, g, idx
}

// TestBuildLocalCoupling verifies the Wave 5 local-complexity block: scored
// same-module edges land in local_coupling with quadrant share, mean balance,
// and worst offenders — and stay OUT of coupling_balance's denominator
// (buildClassifiedEdgeSummary keeps counting them as SameModule only).
func TestBuildLocalCoupling(t *testing.T) {
	cfg, g, idx := localCouplingFixture()
	mm := cfg.ModuleMap

	// coupling_balance denominator: same-module edges excluded.
	s := buildClassifiedEdgeSummary(idx)
	if s.SameModule != 3 {
		t.Errorf("summary.SameModule = %d, want 3", s.SameModule)
	}
	if s.Scored != 1 {
		t.Errorf("summary.Scored = %d, want 1 (cross-module only — same-module edges must not enter coupling_balance)", s.Scored)
	}

	lc := buildLocalCoupling(g, idx, mm)
	if len(lc) != 1 {
		t.Fatalf("local_coupling modules = %d, want 1 (%+v)", len(lc), lc)
	}
	m := lc[0]
	if m.Module != "a" {
		t.Errorf("Module = %q, want a", m.Module)
	}
	if m.ScoredEdges != 2 {
		t.Errorf("ScoredEdges = %d, want 2", m.ScoredEdges)
	}
	if m.AbstainedEdges != 1 {
		t.Errorf("AbstainedEdges = %d, want 1", m.AbstainedEdges)
	}
	if m.ComplexityEdges != 1 {
		t.Errorf("ComplexityEdges = %d, want 1", m.ComplexityEdges)
	}
	if m.ComplexitySharePct != 50 {
		t.Errorf("ComplexitySharePct = %d, want 50", m.ComplexitySharePct)
	}
	if m.MeanBalance != 5.5 { // (2 + 9) / 2
		t.Errorf("MeanBalance = %v, want 5.5", m.MeanBalance)
	}
	if len(m.WorstOffenders) != 1 {
		t.Fatalf("WorstOffenders = %d, want 1 (%+v)", len(m.WorstOffenders), m.WorstOffenders)
	}
	off := m.WorstOffenders[0]
	if off.From != lcBallOfMudFrom || off.To != "services/a/y.go" {
		t.Errorf("offender edge = %s → %s, want %s → services/a/y.go", off.From, off.To, lcBallOfMudFrom)
	}
	if off.Strength != string(coupling.StrengthModel) || off.Balance != 2 || off.Band != string(coupling.SeverityCritical) {
		t.Errorf("offender = %+v, want strength model balance 2 band critical", off)
	}
	if off.File != lcBallOfMudFrom || off.Line != 3 {
		t.Errorf("offender location = %s:%d, want %s:3", off.File, off.Line, lcBallOfMudFrom)
	}
}

// TestBuildLocalCoupling_FourLanguages verifies a same-module edge per language
// lands in local_coupling and none of them enter coupling_balance's
// denominator: Go package-internal import (file paths), TS relative import
// within the module glob (file paths), Python dotted intra-package edge
// (grimp-style dotted node IDs), and Rust intra-crate "::" module edge
// (cargo-modules-style package nodes).
func TestBuildLocalCoupling_FourLanguages(t *testing.T) {
	modules := map[string]module.ModuleDef{
		"gomod":   {Paths: []string{"services/gomod/**"}, Subdomain: lcSubdomainCore},
		"tsmod":   {Paths: []string{"src/ui/**"}, Subdomain: lcSubdomainCore},
		"pymod":   {Paths: []string{"pkg.**"}, Subdomain: lcSubdomainCore},
		"rustmod": {Paths: []string{"mycrate", "mycrate::**"}, Subdomain: lcSubdomainCore},
	}
	cfg := view.ClassifyConfig{Modules: modules}
	model := string(coupling.StrengthModel)

	facts := []graph.Facts{
		{Language: graph.LangGo, Edges: []graph.Edge{{
			From: "file:services/gomod/a.go", To: "file:services/gomod/b.go",
			Kind: graph.EdgeKindImports, Language: graph.LangGo, StrengthHint: model,
		}}},
		{Language: graph.LangTypeScript, Edges: []graph.Edge{{
			From: "file:src/ui/button.ts", To: "file:src/ui/util.ts",
			Kind: graph.EdgeKindImports, Language: graph.LangTypeScript, StrengthHint: model,
		}}},
		{Language: graph.LangPython, Edges: []graph.Edge{{
			From: "module:pkg.a", To: "module:pkg.b",
			Kind: graph.EdgeKindImports, Language: graph.LangPython, StrengthHint: model,
		}}},
		{Language: graph.LangRust, Edges: []graph.Edge{{
			From: "package:mycrate::a", To: "package:mycrate::b",
			Kind: graph.EdgeKindUsesInternal, Language: graph.LangRust, StrengthHint: model,
		}}},
	}
	g := graph.Build(facts)
	idx := classify.Run(g, cfg)
	mm := module.BuildMap(modules)

	s := buildClassifiedEdgeSummary(idx)
	if s.SameModule != 4 {
		t.Errorf("summary.SameModule = %d, want 4 — each language's edge must classify same_module", s.SameModule)
	}
	if s.Scored != 0 {
		t.Errorf("summary.Scored = %d, want 0 — no same-module edge may enter coupling_balance", s.Scored)
	}

	lc := buildLocalCoupling(g, idx, mm)
	want := []string{"gomod", "pymod", "rustmod", "tsmod"}
	if len(lc) != len(want) {
		t.Fatalf("local_coupling modules = %d, want %d (%+v)", len(lc), len(want), lc)
	}
	for i, name := range want {
		m := lc[i]
		if m.Module != name {
			t.Errorf("module[%d] = %q, want %q (sorted)", i, m.Module, name)
			continue
		}
		if m.ScoredEdges != 1 || m.ComplexityEdges != 1 {
			t.Errorf("%s: ScoredEdges=%d ComplexityEdges=%d, want 1/1", name, m.ScoredEdges, m.ComplexityEdges)
		}
	}
}

// TestBuildLocalCoupling_ContractCountsAsComplexity verifies the other half of
// coupling.LocalComplexity's disjunct: a same-module Contract-strength edge
// (not just Model) lands in the local-complexity quadrant, while a
// same-module Functional-strength edge — scored, but neither Contract nor
// Model — does not.
func TestBuildLocalCoupling_ContractCountsAsComplexity(t *testing.T) {
	modules := map[string]module.ModuleDef{
		"a": {Paths: []string{"pkg/q/**"}, Subdomain: lcSubdomainCore},
	}
	cfg := view.ClassifyConfig{Modules: modules}
	edges := []graph.Edge{
		{ // contract/same_module/high → local-complexity quadrant
			From: "file:pkg/q/x.go", To: "file:pkg/q/y.go",
			Kind: graph.EdgeKindImports, Language: graph.LangGo,
			StrengthHint: string(coupling.StrengthContract),
		},
		{ // functional/same_module/high → scored, NOT local-complexity
			From: "file:pkg/q/y.go", To: "file:pkg/q/z.go",
			Kind: graph.EdgeKindImports, Language: graph.LangGo,
			StrengthHint: string(coupling.StrengthFunctional),
		},
	}
	g := graph.Build([]graph.Facts{{Edges: edges, Language: graph.LangGo}})
	idx := classify.Run(g, cfg)
	mm := module.BuildMap(modules)

	lc := buildLocalCoupling(g, idx, mm)
	if len(lc) != 1 {
		t.Fatalf("local_coupling modules = %d, want 1 (%+v)", len(lc), lc)
	}
	m := lc[0]
	if m.ScoredEdges != 2 {
		t.Errorf("ScoredEdges = %d, want 2", m.ScoredEdges)
	}
	if m.ComplexityEdges != 1 {
		t.Errorf("ComplexityEdges = %d, want 1 (only the contract-strength edge; functional does not count)", m.ComplexityEdges)
	}
}

// TestBuildLocalCoupling_OffenderCap verifies the WorstOffenders cap: a module
// with more than localCouplingOffenderCap scored, non-none same-module edges
// keeps only the cap's worth of worst (lowest-balance) offenders, sorted by
// balance then From then To.
func TestBuildLocalCoupling_OffenderCap(t *testing.T) {
	modules := map[string]module.ModuleDef{
		"a": {Paths: []string{"pkg/w/**"}, Subdomain: lcSubdomainCore},
	}
	cfg := view.ClassifyConfig{Modules: modules}
	// Six same-module Model-strength edges: all balance 2 (critical), tied —
	// From/To decide the cap's cut. n6 sorts last and must be dropped.
	edges := []graph.Edge{
		{From: "file:pkg/w/n1.go", To: "file:pkg/w/m1.go", Kind: graph.EdgeKindImports, Language: graph.LangGo, StrengthHint: string(coupling.StrengthModel)},
		{From: "file:pkg/w/n2.go", To: "file:pkg/w/m2.go", Kind: graph.EdgeKindImports, Language: graph.LangGo, StrengthHint: string(coupling.StrengthModel)},
		{From: "file:pkg/w/n3.go", To: "file:pkg/w/m3.go", Kind: graph.EdgeKindImports, Language: graph.LangGo, StrengthHint: string(coupling.StrengthModel)},
		{From: "file:pkg/w/n4.go", To: "file:pkg/w/m4.go", Kind: graph.EdgeKindImports, Language: graph.LangGo, StrengthHint: string(coupling.StrengthModel)},
		{From: "file:pkg/w/n5.go", To: "file:pkg/w/m5.go", Kind: graph.EdgeKindImports, Language: graph.LangGo, StrengthHint: string(coupling.StrengthModel)},
		{From: "file:pkg/w/n6.go", To: "file:pkg/w/m6.go", Kind: graph.EdgeKindImports, Language: graph.LangGo, StrengthHint: string(coupling.StrengthModel)},
	}
	g := graph.Build([]graph.Facts{{Edges: edges, Language: graph.LangGo}})
	idx := classify.Run(g, cfg)
	mm := module.BuildMap(modules)

	lc := buildLocalCoupling(g, idx, mm)
	if len(lc) != 1 {
		t.Fatalf("local_coupling modules = %d, want 1 (%+v)", len(lc), lc)
	}
	off := lc[0].WorstOffenders
	if len(off) != localCouplingOffenderCap {
		t.Fatalf("WorstOffenders = %d, want cap %d", len(off), localCouplingOffenderCap)
	}
	wantFrom := []string{"pkg/w/n1.go", "pkg/w/n2.go", "pkg/w/n3.go", "pkg/w/n4.go", "pkg/w/n5.go"}
	for i, want := range wantFrom {
		if off[i].From != want {
			t.Errorf("offender[%d].From = %q, want %q (tied balance; sorted by From, n6 dropped by the cap)", i, off[i].From, want)
		}
	}
}

// TestBuildLocalCoupling_SameModuleEdgesProduceNoAdvisory is the report-only
// cross-check: using the identical coupling.Index/graph/config that
// TestBuildLocalCoupling verifies against local_coupling, collectAdvisories
// must emit NO bc/imbalanced_coupling finding for a same-module edge — they
// keep SeverityNone (classify.go), regardless of how critical their book
// balance score is. Only the fixture's one cross-module edge may surface.
func TestBuildLocalCoupling_SameModuleEdgesProduceNoAdvisory(t *testing.T) {
	cfg, g, idx := localCouplingFixture()

	fnds := collectAdvisories(g, idx, cfg, nil, RunInput{Now: time.Now(), Accepted: baseline.Baseline{}, Waivers: view.WaiverSet{}})
	bc := findingsByRule(fnds, RuleIDBCImbalanced)
	if len(bc) != 1 {
		t.Fatalf("bc/imbalanced_coupling findings = %d, want 1 (only the cross-module edge; same-module edges must stay SeverityNone): %+v", len(bc), bc)
	}
	if bc[0].Edge.From.Path != lcBallOfMudFrom || bc[0].Edge.To.Path != "services/b/api.go" {
		t.Errorf("advisory edge = %s → %s, want the cross-module edge %s → services/b/api.go",
			bc[0].Edge.From.Path, bc[0].Edge.To.Path, lcBallOfMudFrom)
	}
}
