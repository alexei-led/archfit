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
)

// cannedConfig builds a ClassifyConfig and RuleConfig for a two-module (a, b)
// architecture where pkg/a/** belongs to module "a" and pkg/b/** to module "b".
// Module b has a public path (pkg/b/api/**) and an internal path (pkg/b/internal/**).
func cannedConfig() (config.ClassifyConfig, []rules.Rule) {
	modules := map[string]config.ModuleDef{
		"a": {
			Paths:    []string{"pkg/a/**"},
			Public:   []string{"pkg/a/**"},
			Internal: []string{},
		},
		"b": {
			Paths:    []string{"pkg/b/**"},
			Public:   []string{"pkg/b/api/**"},
			Internal: []string{"pkg/b/internal/**"},
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
		engine.Mode{Head: "HEAD"},
		scope.Scope{Root: "."},
		classifyCfg,
		config.ExceptionSet{},
		[]engine.Extractor{ex},
		rs,
		ms,
		nil, // no renderers in tests
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
		engine.Mode{Head: "HEAD"},
		scope.Scope{Root: "."},
		classifyCfg,
		config.ExceptionSet{},
		[]engine.Extractor{ex},
		rs,
		ms,
		nil,
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
		config.ExceptionSet{},
		[]engine.Extractor{ex},
		rs,
		ms,
		nil,
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
