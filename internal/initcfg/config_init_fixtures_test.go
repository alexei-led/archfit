package initcfg_test

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	apppipeline "github.com/alexei-led/archfit/internal/analysispipeline"
	"github.com/alexei-led/archfit/internal/assessment/finding"
	"github.com/alexei-led/archfit/internal/assessment/metrics"
	"github.com/alexei-led/archfit/internal/assessment/result"
	"github.com/alexei-led/archfit/internal/assessment/rules"
	signal "github.com/alexei-led/archfit/internal/assessment/signals"
	"github.com/alexei-led/archfit/internal/baseline"
	"github.com/alexei-led/archfit/internal/config"
	evidenceports "github.com/alexei-led/archfit/internal/evidence/ports"
	goextract "github.com/alexei-led/archfit/internal/extract/golang"
	"github.com/alexei-led/archfit/internal/initcfg"
	"github.com/alexei-led/archfit/internal/model/graph"
	"github.com/alexei-led/archfit/internal/model/pattern"
	"github.com/alexei-led/archfit/internal/policy"
	"github.com/alexei-led/archfit/internal/relationship"
	"github.com/alexei-led/archfit/internal/scope"
	"github.com/alexei-led/archfit/internal/toolrun"
)

// Fixture root directory names, shared across every per-language table test
// in this file (TestConfigInit_PerLanguage, TestPublicAPIOnly_Task1Fixtures,
// TestForbiddenLayerDirection_Task1Fixtures).
const (
	fixtureRootGo   = "gofixture"
	fixtureRootTS   = "tsfixture"
	fixtureRootPy   = "pyfixture"
	fixtureRootRust = "rustfixture"
	langTypeScript  = "typescript"
	langPython      = "python"
	langRust        = "rust"
)

// initFixtureRoot returns the absolute path to internal/initcfg/testdata/<name>,
// one minimal per-language project just large enough for `config init` to infer
// modules from.
func initFixtureRoot(t *testing.T, name string) string {
	t.Helper()
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller failed")
	}
	root := filepath.Join(filepath.Dir(file), "testdata", name)
	abs, err := filepath.Abs(root)
	if err != nil {
		t.Fatalf("abs path: %v", err)
	}
	return abs
}

// rustFixtureRunner reports cargo present and replies to `cargo metadata` with
// a single first-party crate named "rustfixture" — no inter-crate edges.
func rustFixtureRunner() *toolrun.RunnerMock {
	const metadataJSON = `{
  "packages": [{"id": "rustfixture 0.1.0", "name": "rustfixture", "dependencies": []}],
  "workspace_members": ["rustfixture 0.1.0"]
}`
	return &toolrun.RunnerMock{
		DetectFunc: func(_ context.Context, name string) (toolrun.ToolInfo, bool) {
			return toolrun.ToolInfo{}, name == "cargo"
		},
		RunFunc: func(_ context.Context, _ toolrun.ToolCmd) (toolrun.Output, error) {
			return toolrun.Output{Stdout: []byte(metadataJSON)}, nil
		},
	}
}

// loadRendered writes Render's output to a temp .archfit.yaml and strict-parses
// it through config.Load, exactly as `archfit config init` output must.
func loadRendered(t *testing.T, rendered string) config.Config {
	t.Helper()
	path := filepath.Join(t.TempDir(), ".archfit.yaml")
	if err := os.WriteFile(path, []byte(rendered), 0o600); err != nil {
		t.Fatalf("write rendered config: %v", err)
	}
	cfg, err := config.Load(context.Background(), path)
	if err != nil {
		t.Fatalf("generated config does not parse under strict config:\n%v\n\nrendered:\n%s", err, rendered)
	}
	return cfg
}

// TestConfigInit_PerLanguage runs `config init`'s Discover+Render over one
// minimal fixture per supported language and asserts the two invariants every
// generated config must satisfy: it strict-parses, and every emitted rule
// type is recognized by internal/rules (see rules.New).
func TestConfigInit_PerLanguage(t *testing.T) {
	tests := []struct {
		name   string
		root   string
		runner toolrun.Runner
	}{
		{name: "go", root: fixtureRootGo, runner: toolrun.New()},
		{name: langTypeScript, root: fixtureRootTS, runner: &toolrun.RunnerMock{}},
		{name: langPython, root: fixtureRootPy, runner: &toolrun.RunnerMock{}},
		{name: langRust, root: fixtureRootRust, runner: rustFixtureRunner()},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			root := initFixtureRoot(t, tt.root)
			discovered, err := initcfg.Discover(context.Background(), root, tt.runner)
			if err != nil {
				t.Fatalf("Discover: %v", err)
			}

			rendered := initcfg.Render(discovered, nil, false)
			cfg := loadRendered(t, rendered)

			if _, err := rules.New(cfg.ForRules()); err != nil {
				t.Errorf("generated rule type not recognized by internal/rules: %v\n\nrendered:\n%s", err, rendered)
			}
			if tt.name == langRust {
				if cfg.Languages.Rust.Enabled != evidenceports.ModeAuto {
					t.Errorf("generated Rust mode = %q, want auto\n\nrendered:\n%s", cfg.Languages.Rust.Enabled, rendered)
				}
				if !cfg.CargoModulesEnabled() || !cfg.ScipEnabled() {
					t.Errorf("generated Rust config should enable cargo_modules and scip\n\nrendered:\n%s", rendered)
				}
			}
		})
	}
}

// pathIn derives a concrete descendant path inside a module's glob pattern,
// e.g. "src/core/**" -> "src/core/x", "pyfixture.core.*" -> "pyfixture.core.x".
// Python modules use dotted globs (e.g. "pyfixture.core.*"), Go/TS use slash
// path globs (e.g. "src/core/**") — pick "." or "/" as the descendant
// separator to match.
func pathIn(glob string) string {
	sep := "/"
	if !strings.Contains(glob, "/") {
		sep = "."
	}
	base := strings.TrimSuffix(strings.TrimSuffix(glob, "**"), "*")
	base = strings.TrimSuffix(base, sep)
	return base + sep + "x"
}

// fixtureGraph builds a two-node relationship.Set with a single uses_internal
// edge from "module:"+from to "module:"+to (test-only adapter).
func fixtureGraph(from, to string) relationship.Set {
	nodes := []graph.Node{
		{Kind: graph.NodeKindModule, Path: from},
		{Kind: graph.NodeKindModule, Path: to},
	}
	edges := []graph.Edge{
		{From: "module:" + from, To: "module:" + to, Kind: graph.EdgeKindUsesInternal},
	}
	g := graph.Build([]graph.Facts{{Nodes: nodes, Edges: edges, Language: "go"}})
	set := relationship.Set{
		Nodes: make([]relationship.Node, 0, len(g.Nodes())),
		Edges: make([]relationship.Edge, 0, len(g.Edges())),
	}
	for _, n := range g.Nodes() {
		set.Nodes = append(set.Nodes, relationship.Node{ID: n.ID(), Path: n.Path, Kind: string(n.Kind), Language: n.Language})
	}
	for _, e := range g.Edges() {
		set.Edges = append(set.Edges, relationship.Edge{
			FromID: e.From, ToID: e.To,
			FromPath: graph.NodePath(e.From), ToPath: graph.NodePath(e.To),
			Kind: string(e.Kind), Language: e.Language,
		})
	}
	return set
}

// TestPublicAPIOnly_Task1Fixtures documents public_api_only's (V5) behavior on
// the Wave 2 Task 1 per-language fixtures, using each fixture's real
// Discover-derived module paths:
//
//   - Discover never populates ModuleDef.Internal for any language (config
//     init has no "internal:" inference), so on a freshly generated config,
//     dependency-cruiser/grimp/cargo never tag an edge EdgeKindUsesInternal
//     and public_api_only is structurally inert — no spurious findings.
//   - If a user later hand-adds "internal:" globs (the documented pattern in
//     configuration-reference.md), the V5 module-map fix still does the right
//     thing on these fixtures' real module shapes: same-module access is not
//     flagged, genuine cross-module access still is.
func TestPublicAPIOnly_Task1Fixtures(t *testing.T) {
	tests := []struct {
		name   string
		root   string
		runner toolrun.Runner
	}{
		// go is the one language whose extractor tags EdgeKindUsesInternal
		// natively, so the V5 same-module fix must hold on its real module shapes.
		{name: "go", root: fixtureRootGo, runner: toolrun.New()},
		{name: langTypeScript, root: fixtureRootTS, runner: &toolrun.RunnerMock{}},
		{name: langPython, root: fixtureRootPy, runner: &toolrun.RunnerMock{}},
		{name: langRust, root: fixtureRootRust, runner: rustFixtureRunner()},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			root := initFixtureRoot(t, tt.root)
			discovered, err := initcfg.Discover(context.Background(), root, tt.runner)
			if err != nil {
				t.Fatalf("Discover: %v", err)
			}
			for _, m := range discovered.Modules {
				if len(m.Internal) != 0 {
					t.Fatalf("module %q: Discover set Internal=%v, want empty (config init never infers internal: globs)", m.Name, m.Internal)
				}
			}
			if len(discovered.Modules) < 2 {
				// rustfixture is a single crate: no second module to build a
				// cross-module edge against. The Internal-empty assertion above
				// already covers the inertness claim for this fixture.
				return
			}

			modules := make(map[string]policy.ModuleDef, len(discovered.Modules))
			for _, m := range discovered.Modules {
				modules[m.Name] = policy.ModuleDef{Paths: m.Paths}
			}
			cfg := config.Config{
				Version: 1,
				Modules: modules,
				Rules: []policy.RuleDef{
					{ID: "no-internal-access", Type: "public_api_only"},
				},
			}
			rs, err := rules.New(cfg.ForRules())
			if err != nil {
				t.Fatalf("rules.New: %v", err)
			}

			// Build a same-module and a cross-module uses_internal edge from the
			// fixture's own module glob patterns.
			apiPath, corePath := pathIn(discovered.Modules[0].Paths[0]), pathIn(discovered.Modules[1].Paths[0])
			nested := func(base string) string {
				sep := "/"
				if !strings.Contains(base, "/") {
					sep = "."
				}
				return base + sep + "inner"
			}

			g := fixtureGraph(corePath, nested(corePath))
			if findings := rs[0].Check(g, rules.Evidence{}); len(findings) != 0 {
				t.Errorf("same-module uses_internal edge: got %d findings, want 0: %+v", len(findings), findings)
			}

			g = fixtureGraph(apiPath, nested(corePath))
			if findings := rs[0].Check(g, rules.Evidence{}); len(findings) != 1 {
				t.Errorf("cross-module uses_internal edge: got %d findings, want 1: %+v", len(findings), findings)
			}
		})
	}
}

// TestForbiddenLayerDirection_Task1Fixtures is the forbidden_layer_direction
// mirror of TestPublicAPIOnly_Task1Fixtures: it proves the rule actually FIRES
// on each language fixture's real Discover-derived layers and module globs,
// not just that a generated config parses and the rule type is recognized
// (that weaker proof already exists in TestConfigInit_PerLanguage).
func TestForbiddenLayerDirection_Task1Fixtures(t *testing.T) {
	tests := []struct {
		name   string
		root   string
		runner toolrun.Runner
	}{
		{name: "go", root: fixtureRootGo, runner: toolrun.New()},
		{name: langTypeScript, root: fixtureRootTS, runner: &toolrun.RunnerMock{}},
		{name: langPython, root: fixtureRootPy, runner: &toolrun.RunnerMock{}},
		{name: langRust, root: fixtureRootRust, runner: rustFixtureRunner()},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			root := initFixtureRoot(t, tt.root)
			discovered, err := initcfg.Discover(context.Background(), root, tt.runner)
			if err != nil {
				t.Fatalf("Discover: %v", err)
			}
			if len(discovered.Layers) < 2 {
				// This fixture's real Discover-derived layer assignment collapses
				// every module onto a single layer: discoverSubdirs (TS) hardcodes
				// Layer=layerCore regardless of directory name, and rustfixture is
				// a single crate with no inter-crate edges to topo-assign distinct
				// layers from (see discover_ts.go, discover_rust.go). There is no
				// forbidden direction to construct without inventing a layer split
				// Discover never produces on these fixtures — mirrors
				// TestPublicAPIOnly_Task1Fixtures's rustfixture early-return.
				return
			}

			modules := make(map[string]policy.ModuleDef, len(discovered.Modules))
			for _, m := range discovered.Modules {
				modules[m.Name] = policy.ModuleDef{Paths: m.Paths, Layer: m.Layer}
			}
			cfg := config.Config{
				Version: 1,
				Layers:  discovered.Layers,
				Modules: modules,
				Rules: []policy.RuleDef{
					{ID: "no-back-edge", Type: "forbidden_layer_direction"},
				},
			}
			rs, err := rules.New(cfg.ForRules())
			if err != nil {
				t.Fatalf("rules.New: %v", err)
			}

			// The forbidden direction is innermost layer (rank 0, Layers[0]) importing
			// the outermost layer (highest rank, Layers[last]) — see
			// forbiddenLayerDirection.Check's fromRank < toRank comment.
			innerLayer := discovered.Layers[0]
			outerLayer := discovered.Layers[len(discovered.Layers)-1]
			var innerPath, outerPath string
			for _, m := range discovered.Modules {
				if m.Layer == innerLayer && innerPath == "" {
					innerPath = pathIn(m.Paths[0])
				}
				if m.Layer == outerLayer && outerPath == "" {
					outerPath = pathIn(m.Paths[0])
				}
			}
			if innerPath == "" || outerPath == "" {
				t.Fatalf("could not find modules for layers %q/%q in %+v", innerLayer, outerLayer, discovered.Modules)
			}

			g := fixtureGraph(innerPath, outerPath)
			if findings := rs[0].Check(g, rules.Evidence{}); len(findings) != 1 {
				t.Errorf("back-edge %s(%s) -> %s(%s): got %d findings, want 1: %+v", innerPath, innerLayer, outerPath, outerLayer, len(findings), findings)
			}

			// Sanity: the allowed direction (outer imports inner) must not fire.
			g = fixtureGraph(outerPath, innerPath)
			if findings := rs[0].Check(g, rules.Evidence{}); len(findings) != 0 {
				t.Errorf("allowed direction %s(%s) -> %s(%s): got %d findings, want 0: %+v", outerPath, outerLayer, innerPath, innerLayer, len(findings), findings)
			}
		})
	}
}

// runRenderedAnalyze strict-loads rendered as .archfit.yaml and runs the full
// analyze pipeline (rules + metrics) over root's Go sources. Mode.Advisory is
// always on so gate:warn rule findings (Kind=advisory) surface in
// diag.Findings, mirroring how `archfit analyze` renders warn-gated rules.
func runRenderedAnalyze(t *testing.T, root, rendered string) result.Diagnostic {
	t.Helper()
	cfg := loadRendered(t, rendered)

	classifyCfg := cfg.ForClassify()
	rs, err := rules.New(cfg.ForRules())
	if err != nil {
		t.Fatalf("rules.New: %v", err)
	}
	ms := metrics.New(cfg.Metrics)

	extractor := goextract.New(evidenceports.ExtractConfig{})
	base := baseline.Baseline{SchemaVersion: baseline.SchemaVersion}
	now := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	s := scope.Scope{Root: root, Mode: scope.ModeFull}

	diag, err := apppipeline.RunStages(context.Background(), apppipeline.StageInput{
		Mode:  apppipeline.Mode{Full: true, Advisory: true},
		Scope: s,
		Policy: policy.New(
			policy.TopologyView{Modules: classifyCfg.Modules, Layers: classifyCfg.Layers, ModuleMap: classifyCfg.ModuleMap, ExternalSystems: classifyCfg.ExternalSystems, ExplicitOwners: classifyCfg.ExplicitOwners},
			policy.RelationshipPolicy{MinimumSeverity: classifyCfg.BCAdvisoryMinSeverity, VolatilityCascadeEnabled: classifyCfg.VolatilityCascadeEnabled, DuplicatedKnowledge: classifyCfg.DuplicatedKnowledgePolicy},
			policy.AssessmentPolicy{}, policy.GatePolicy{}, nil, nil),
		Extractors:  []evidenceports.Extractor{extractor},
		Patterns:    evidenceports.NopPatternProvider{},
		Resolver:    evidenceports.NopSymbolResolver{},
		PatternCfg:  pattern.Config{},
		Rules:       rs,
		Metrics:     ms,
		Accepted:    base,
		BaseMetrics: base.Metrics,
		Signals:     signal.RunSignals{},
		Now:         now,
	})
	if err != nil {
		t.Fatalf("apppipeline.RunStages: %v", err)
	}
	return diag
}

// TestConfigInit_GoFixture_LayerRuleBackEdge runs the full analyze gate over
// the Go fixture with the config `config init` generates for it. The fixture
// has a genuine layer back-edge (internal/model imports internal/engine —
// the innermost layer reaching into the outermost one).
//
// V4 (docs/archived/reports/eval-2026-07-02-v1.1.2/00-FINDINGS.md) was that Render emitted
// `type: forbidden_dependency` with `from_layer`/`to_layer`, but
// forbiddenDependency.Check reads `From`/`To`, which were always empty — the
// generated rule could never fire. Render now emits
// `type: forbidden_layer_direction`, which derives layer ordering from
// cfg.Layers and the module map, so the back-edge below is caught.
func TestConfigInit_GoFixture_LayerRuleBackEdge(t *testing.T) {
	root := initFixtureRoot(t, "gofixture")
	discovered, err := initcfg.Discover(context.Background(), root, toolrun.New())
	if err != nil {
		t.Fatalf("Discover: %v", err)
	}
	if len(discovered.Layers) < 2 {
		t.Fatalf("fixture must discover >=2 layers (back-edge needs a layer pair), got %v", discovered.Layers)
	}

	rendered := initcfg.Render(discovered, nil, false)
	cfg := loadRendered(t, rendered)
	if len(cfg.Rules) == 0 {
		t.Fatalf("generated config has no rules:\n%s", rendered)
	}

	diag := runRenderedAnalyze(t, root, rendered)

	wantRuleID := cfg.Rules[0].ID // the single generated forbidden_layer_direction rule
	var layerFindings []finding.Finding
	for _, f := range diag.Findings {
		if f.RuleID == wantRuleID {
			layerFindings = append(layerFindings, f)
		}
	}
	if got := len(layerFindings); got != 1 {
		t.Fatalf("findings for rule %q = %d, want 1 (the back-edge): %+v", wantRuleID, got, diag.Findings)
	}
	if got := layerFindings[0].Kind; got != finding.KindAdvisory {
		t.Errorf("finding kind = %q, want %q (rule is gate: warn)", got, finding.KindAdvisory)
	}
}

// TestConfigInit_GoFixture_LayerRuleBackEdge_PromotedToFail simulates the
// shipped TODO comment ("review and promote gate: warn to gate: fail"): once
// promoted, the same back-edge must block the verdict, not just advise.
func TestConfigInit_GoFixture_LayerRuleBackEdge_PromotedToFail(t *testing.T) {
	root := initFixtureRoot(t, "gofixture")
	discovered, err := initcfg.Discover(context.Background(), root, toolrun.New())
	if err != nil {
		t.Fatalf("Discover: %v", err)
	}
	rendered := initcfg.Render(discovered, nil, false)
	promoted := strings.Replace(rendered, "    gate: warn\n", "    gate: fail\n", 1)
	if promoted == rendered {
		t.Fatalf("expected exactly one rule-level gate: warn to promote:\n%s", rendered)
	}

	diag := runRenderedAnalyze(t, root, promoted)
	if diag.Verdict != result.VerdictFail {
		t.Errorf("verdict = %q, want %q (back-edge under gate: fail): %+v", diag.Verdict, result.VerdictFail, diag.Findings)
	}
}

// TestConfigInit_GoFixture_NoBackEdge_GatePasses is the mirror of
// TestConfigInit_GoFixture_LayerRuleBackEdge: the same two-layer shape, but
// the dependency runs the allowed direction (outer imports inner). The
// generated forbidden_layer_direction rule must not fire.
func TestConfigInit_GoFixture_NoBackEdge_GatePasses(t *testing.T) {
	root := t.TempDir()
	writeFixtureFile(t, root, "go.mod", "module example.com/gofixtureok\n\ngo 1.21\n")
	writeFixtureFile(t, root, "internal/model/model.go", `// Package model is the innermost layer; it has no dependency on engine.
package model

// Describe is model's public surface.
func Describe() string { return "model" }
`)
	writeFixtureFile(t, root, "internal/engine/engine.go", `// Package engine is the outermost layer. It imports model — outer
// depending on inner is the allowed direction, not a back-edge.
package engine

import "example.com/gofixtureok/internal/model"

// Run reaches into the model layer.
func Run() string { return model.Describe() }
`)

	discovered, err := initcfg.Discover(context.Background(), root, toolrun.New())
	if err != nil {
		t.Fatalf("Discover: %v", err)
	}
	if len(discovered.Layers) < 2 {
		t.Fatalf("fixture must discover >=2 layers, got %v", discovered.Layers)
	}
	rendered := initcfg.Render(discovered, nil, false)

	diag := runRenderedAnalyze(t, root, rendered)
	if diag.Verdict != result.VerdictPass {
		t.Errorf("verdict = %q, want %q (no back-edge): %+v", diag.Verdict, result.VerdictPass, diag.Findings)
	}
	for _, f := range diag.Findings {
		if strings.HasPrefix(f.RuleID, "no-") {
			t.Errorf("unexpected layer-rule finding with no back-edge: %+v", f)
		}
	}
}

// writeFixtureFile writes content to root/relPath, creating parent dirs.
func writeFixtureFile(t *testing.T, root, relPath, content string) {
	t.Helper()
	full := filepath.Join(root, relPath)
	if err := os.MkdirAll(filepath.Dir(full), 0o750); err != nil {
		t.Fatalf("mkdir %s: %v", filepath.Dir(full), err)
	}
	if err := os.WriteFile(full, []byte(content), 0o600); err != nil {
		t.Fatalf("write %s: %v", full, err)
	}
}
