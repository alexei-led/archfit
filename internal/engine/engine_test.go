package engine_test

import (
	"context"
	"testing"
	"time"

	"github.com/alexei-led/archfit/internal/baseline"
	"github.com/alexei-led/archfit/internal/config"
	"github.com/alexei-led/archfit/internal/engine"
	"github.com/alexei-led/archfit/internal/metrics"
	"github.com/alexei-led/archfit/internal/model/diagnostic"
	"github.com/alexei-led/archfit/internal/model/finding"
	"github.com/alexei-led/archfit/internal/model/graph"
	"github.com/alexei-led/archfit/internal/rules"
	"github.com/alexei-led/archfit/internal/scope"
)

const (
	rulePublicAPIOnly     = "public_api_only"
	pathFileA             = "pkg/a/a.go"
	pathFileANode         = "file:pkg/a/a.go"
	pathFileBInternal     = "pkg/b/internal/impl.go"
	pathFileBInternalNode = "file:pkg/b/internal/impl.go"

	// Glob constants shared with golden_test.go (same package).
	globModuleA         = "pkg/a/**"
	globModuleB         = "pkg/b/**"
	globModuleBInternal = "pkg/b/internal/**"

	headRef      = "HEAD"
	kindAdvisory = "advisory"
)

// cannedConfig builds a ClassifyConfig and RuleConfig for a two-module (a, b)
// architecture where pkg/a/** belongs to module "a" and pkg/b/** to module "b".
// Module b has a public path (pkg/b/api/**) and an internal path (pkg/b/internal/**).
func cannedConfig() (config.ClassifyConfig, []rules.Rule) {
	modules := map[string]config.ModuleDef{
		"a": {
			Paths:    []string{globModuleA},
			Public:   []string{globModuleA},
			Internal: []string{},
		},
		"b": {
			Paths:    []string{globModuleB},
			Public:   []string{"pkg/b/api/**"},
			Internal: []string{globModuleBInternal},
		},
	}

	cfg := config.Config{
		Version: 1,
		Modules: modules,
		Rules: []config.RuleDef{
			{
				ID:   rulePublicAPIOnly,
				Type: rulePublicAPIOnly,
				Gate: "fail",
			},
		},
	}

	classifyCfg := cfg.ForClassify()
	ruleCfg := cfg.ForRules()
	rs := rules.New(ruleCfg)
	return classifyCfg, rs
}

// violationFacts returns Facts with a uses_internal edge from pkg/a/a.go → pkg/b/internal/impl.go.
func violationFacts() graph.Facts {
	return graph.Facts{
		Language: "go",
		Nodes: []graph.Node{
			{Kind: graph.NodeKindFile, Path: pathFileA},
			{Kind: graph.NodeKindFile, Path: pathFileBInternal},
		},
		Edges: []graph.Edge{
			{
				From:       pathFileANode,
				To:         pathFileBInternalNode,
				Kind:       graph.EdgeKindUsesInternal,
				Language:   "go",
				Confidence: "high",
				Locations:  []graph.Location{{File: pathFileA, Line: 5}},
			},
		},
	}
}

const (
	pathFileBAPIService     = "pkg/b/api/service.go"
	pathFileBAPIServiceNode = "file:pkg/b/api/service.go"
)

// cleanFacts returns Facts with a normal import edge that does not violate any rule.
func cleanFacts() graph.Facts {
	return graph.Facts{
		Language: "go",
		Nodes: []graph.Node{
			{Kind: graph.NodeKindFile, Path: pathFileA},
			{Kind: graph.NodeKindFile, Path: pathFileBAPIService},
		},
		Edges: []graph.Edge{
			{
				From:       pathFileANode,
				To:         pathFileBAPIServiceNode,
				Kind:       graph.EdgeKindImports,
				Language:   "go",
				Confidence: "high",
				Locations:  []graph.Location{{File: "pkg/a/a.go", Line: 3}},
			},
		},
	}
}

func TestRun_GateFinding_VerdictFail(t *testing.T) {
	ctx := context.Background()
	facts := violationFacts()

	ex := &engine.ExtractorMock{
		NameFunc: func() string { return "go" },
		ExtractFunc: func(_ context.Context, _ scope.Scope) (graph.Facts, diagnostic.Coverage, error) {
			return facts, diagnostic.Coverage{Tool: "go", Status: "ok", FilesSeen: 2, FilesApplicable: 2}, nil
		},
	}

	classifyCfg, rs := cannedConfig()
	ms := metrics.New(config.Config{Version: 1})
	base := baseline.Baseline{}
	now := time.Date(2026, 6, 7, 0, 0, 0, 0, time.UTC)

	d, err := engine.Run(
		ctx,
		engine.Mode{Head: headRef},
		scope.Scope{Root: "."},
		classifyCfg,
		config.StalenessConfig{},
		config.ExceptionSet{},
		[]engine.Extractor{ex},
		engine.NopPatternProvider{},
		engine.NopSymbolResolver{},
		rs,
		ms,
		base,
		now,
	)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}

	if d.Verdict != diagnostic.VerdictFail {
		t.Errorf("verdict=%q, want %q", d.Verdict, diagnostic.VerdictFail)
	}

	// There should be exactly one gate finding for the public_api_only rule.
	var gateFinding *finding.Finding
	for i := range d.Findings {
		f := &d.Findings[i]
		if f.RuleID == "public_api_only" && f.Kind == "gate" {
			gateFinding = f
			break
		}
	}
	if gateFinding == nil {
		t.Fatalf("no gate finding with rule_id=public_api_only found; findings=%+v", d.Findings)
	}
	if gateFinding.Status != finding.StatusNew {
		t.Errorf("finding status=%q, want %q", gateFinding.Status, finding.StatusNew)
	}

	// Edge evidence must be populated.
	if gateFinding.Edge.From.Path == "" {
		t.Errorf("finding.Edge.From.Path is empty")
	}
	if gateFinding.Edge.To.Path == "" {
		t.Errorf("finding.Edge.To.Path is empty")
	}

	// Summary must reflect the gate finding.
	if d.Summary.GateFindings < 1 {
		t.Errorf("summary.gate_findings=%d, want >= 1", d.Summary.GateFindings)
	}

	// agent_tasks must be a typed empty slice (not nil).
	if d.AgentTasks == nil {
		t.Errorf("agent_tasks is nil, want typed empty slice")
	}
}

func TestRun_CleanGraph_VerdictPass(t *testing.T) {
	ctx := context.Background()
	facts := cleanFacts()

	ex := &engine.ExtractorMock{
		NameFunc: func() string { return "go" },
		ExtractFunc: func(_ context.Context, _ scope.Scope) (graph.Facts, diagnostic.Coverage, error) {
			return facts, diagnostic.Coverage{Tool: "go", Status: "ok", FilesSeen: 2, FilesApplicable: 2}, nil
		},
	}

	classifyCfg, rs := cannedConfig()
	ms := metrics.New(config.Config{Version: 1})
	base := baseline.Baseline{}
	now := time.Date(2026, 6, 7, 0, 0, 0, 0, time.UTC)

	d, err := engine.Run(
		ctx,
		engine.Mode{Head: headRef},
		scope.Scope{Root: "."},
		classifyCfg,
		config.StalenessConfig{},
		config.ExceptionSet{},
		[]engine.Extractor{ex},
		engine.NopPatternProvider{},
		engine.NopSymbolResolver{},
		rs,
		ms,
		base,
		now,
	)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}

	if d.Verdict != diagnostic.VerdictPass {
		t.Errorf("verdict=%q, want %q", d.Verdict, diagnostic.VerdictPass)
	}

	// No new gate findings.
	for _, f := range d.Findings {
		if f.Kind == "gate" && (f.Status == finding.StatusNew || f.Status == finding.StatusExpiredExcept) {
			t.Errorf("unexpected new gate finding: %+v", f)
		}
	}

	if d.Summary.GateFindings != 0 {
		t.Errorf("summary.gate_findings=%d, want 0", d.Summary.GateFindings)
	}
}

func TestRun_DiagnosticShape(t *testing.T) {
	ctx := context.Background()

	ex := &engine.ExtractorMock{
		NameFunc: func() string { return "go" },
		ExtractFunc: func(_ context.Context, _ scope.Scope) (graph.Facts, diagnostic.Coverage, error) {
			return graph.Facts{Language: "go"}, diagnostic.Coverage{Tool: "go", Status: "ok"}, nil
		},
	}

	classifyCfg, rs := cannedConfig()
	ms := metrics.New(config.Config{Version: 1})
	base := baseline.Baseline{}
	now := time.Now()

	d, err := engine.Run(
		ctx,
		engine.Mode{Base: "main", Head: "feature"},
		scope.Scope{Root: "."},
		classifyCfg,
		config.StalenessConfig{},
		config.ExceptionSet{},
		[]engine.Extractor{ex},
		engine.NopPatternProvider{},
		engine.NopSymbolResolver{},
		rs,
		ms,
		base,
		now,
	)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}

	if d.SchemaVersion != diagnostic.SchemaVersion {
		t.Errorf("schema_version=%q, want %q", d.SchemaVersion, diagnostic.SchemaVersion)
	}
	if d.Base != "main" {
		t.Errorf("base=%q, want %q", d.Base, "main")
	}
	if d.Head != "feature" {
		t.Errorf("head=%q, want %q", d.Head, "feature")
	}
	if d.AgentTasks == nil {
		t.Errorf("agent_tasks is nil, want typed empty slice")
	}
	if d.Findings == nil {
		t.Errorf("findings is nil, want typed empty slice")
	}
	if d.Metrics == nil {
		t.Errorf("metrics is nil, want typed empty slice")
	}
	if d.ToolCoverage == nil {
		t.Errorf("tool_coverage is nil, want typed empty slice")
	}
	// Metrics should contain all Phase 1 metrics.
	if len(d.Metrics) != 4 {
		t.Errorf("len(metrics)=%d, want 4", len(d.Metrics))
	}
}

// TestRun_Advisory_FilteredWhenDisabled asserts that advisory findings do NOT appear
// in Findings and Summary.Warnings is 0 when mode.Advisory = false.
// The violation graph (intrusive edge a→b/internal) will produce a coupling advisory,
// but it must be suppressed.
func TestRun_Advisory_FilteredWhenDisabled(t *testing.T) {
	ctx := context.Background()
	ex := &engine.ExtractorMock{
		NameFunc: func() string { return "go" },
		ExtractFunc: func(_ context.Context, _ scope.Scope) (graph.Facts, diagnostic.Coverage, error) {
			return violationFacts(), diagnostic.Coverage{Tool: "go", Status: "ok"}, nil
		},
	}

	classifyCfg, rs := cannedConfig()
	ms := metrics.New(config.Config{Version: 1})
	base := baseline.Baseline{}
	now := time.Date(2026, 6, 7, 0, 0, 0, 0, time.UTC)

	d, err := engine.Run(
		ctx,
		engine.Mode{Head: headRef, Advisory: false},
		scope.Scope{Root: "."},
		classifyCfg,
		config.StalenessConfig{},
		config.ExceptionSet{},
		[]engine.Extractor{ex},
		engine.NopPatternProvider{},
		engine.NopSymbolResolver{},
		rs,
		ms,
		base,
		now,
	)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}

	for _, f := range d.Findings {
		if f.Kind == kindAdvisory {
			t.Errorf("advisory finding present when mode.Advisory=false: %+v", f)
		}
	}
	if d.Summary.Warnings != 0 {
		t.Errorf("summary.warnings=%d, want 0 when advisory disabled", d.Summary.Warnings)
	}
}

// TestRun_Advisory_PresentWhenEnabled asserts that advisory findings DO appear
// when mode.Advisory = true, verdict stays pass with a clean graph, and
// Summary.Warnings equals the advisory count.
func TestRun_Advisory_PresentWhenEnabled(t *testing.T) {
	ctx := context.Background()
	// cleanFacts: imports edge a→b/api (contract, cross-module) → imbalanced (low severity).
	ex := &engine.ExtractorMock{
		NameFunc: func() string { return "go" },
		ExtractFunc: func(_ context.Context, _ scope.Scope) (graph.Facts, diagnostic.Coverage, error) {
			return cleanFacts(), diagnostic.Coverage{Tool: "go", Status: "ok"}, nil
		},
	}

	classifyCfg, rs := cannedConfig()
	ms := metrics.New(config.Config{Version: 1})
	base := baseline.Baseline{}
	now := time.Date(2026, 6, 7, 0, 0, 0, 0, time.UTC)

	d, err := engine.Run(
		ctx,
		engine.Mode{Head: headRef, Advisory: true},
		scope.Scope{Root: "."},
		classifyCfg,
		config.StalenessConfig{},
		config.ExceptionSet{},
		[]engine.Extractor{ex},
		engine.NopPatternProvider{},
		engine.NopSymbolResolver{},
		rs,
		ms,
		base,
		now,
	)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}

	// Verdict must be pass — advisory findings never gate.
	if d.Verdict != diagnostic.VerdictPass {
		t.Errorf("verdict=%q, want pass with advisory findings present", d.Verdict)
	}

	// At least one advisory finding must be present.
	var advisoryCount int
	for _, f := range d.Findings {
		if f.Kind == kindAdvisory {
			advisoryCount++
			if f.RuleID == "" {
				t.Errorf("advisory finding has empty rule_id: %+v", f)
			}
		}
	}
	if advisoryCount == 0 {
		t.Errorf("no advisory findings present when mode.Advisory=true; findings=%+v", d.Findings)
	}

	// Summary.Warnings must match advisory count.
	if d.Summary.Warnings != advisoryCount {
		t.Errorf("summary.warnings=%d, want %d (advisory count)", d.Summary.Warnings, advisoryCount)
	}
}

// TestRun_Advisory_VerdictUnchanged asserts that advisory findings do NOT change
// a fail verdict: a gate violation still fails even when advisories are present.
func TestRun_Advisory_VerdictUnchanged(t *testing.T) {
	ctx := context.Background()
	ex := &engine.ExtractorMock{
		NameFunc: func() string { return "go" },
		ExtractFunc: func(_ context.Context, _ scope.Scope) (graph.Facts, diagnostic.Coverage, error) {
			return violationFacts(), diagnostic.Coverage{Tool: "go", Status: "ok"}, nil
		},
	}

	classifyCfg, rs := cannedConfig()
	ms := metrics.New(config.Config{Version: 1})
	base := baseline.Baseline{}
	now := time.Date(2026, 6, 7, 0, 0, 0, 0, time.UTC)

	d, err := engine.Run(
		ctx,
		engine.Mode{Head: headRef, Advisory: true},
		scope.Scope{Root: "."},
		classifyCfg,
		config.StalenessConfig{},
		config.ExceptionSet{},
		[]engine.Extractor{ex},
		engine.NopPatternProvider{},
		engine.NopSymbolResolver{},
		rs,
		ms,
		base,
		now,
	)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}

	// Gate violation must still fail.
	if d.Verdict != diagnostic.VerdictFail {
		t.Errorf("verdict=%q, want fail (gate violation present)", d.Verdict)
	}

	// Advisory findings are present in Findings.
	var advisoryCount int
	for _, f := range d.Findings {
		if f.Kind == kindAdvisory {
			advisoryCount++
		}
	}
	if advisoryCount == 0 {
		t.Errorf("no advisory findings when mode.Advisory=true")
	}

	// Summary.Warnings matches advisory count.
	if d.Summary.Warnings != advisoryCount {
		t.Errorf("summary.warnings=%d, want %d", d.Summary.Warnings, advisoryCount)
	}
}
