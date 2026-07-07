package engine_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"slices"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/alexei-led/archfit/internal/baseline"
	"github.com/alexei-led/archfit/internal/config"
	"github.com/alexei-led/archfit/internal/engine"
	"github.com/alexei-led/archfit/internal/labels"
	"github.com/alexei-led/archfit/internal/metrics"
	"github.com/alexei-led/archfit/internal/model/coupling"
	"github.com/alexei-led/archfit/internal/model/diagnostic"
	"github.com/alexei-led/archfit/internal/model/finding"
	"github.com/alexei-led/archfit/internal/model/graph"
	"github.com/alexei-led/archfit/internal/model/pattern"
	"github.com/alexei-led/archfit/internal/model/signal"
	"github.com/alexei-led/archfit/internal/model/symbol"
	"github.com/alexei-led/archfit/internal/ports"
	"github.com/alexei-led/archfit/internal/rules"
	"github.com/alexei-led/archfit/internal/scope"
	archscore "github.com/alexei-led/archfit/internal/score"
)

const (
	rulePublicAPIOnly     = "public_api_only"
	ruleLabelsStale       = "labels/stale"
	pathFileA             = "pkg/a/a.go"
	pathFileANode         = "file:pkg/a/a.go"
	pathFileBInternal     = "pkg/b/internal/impl.go"
	pathFileBInternalNode = "file:pkg/b/internal/impl.go"

	// Glob constants shared with golden_test.go (same package).
	globModuleA         = "pkg/a/**"
	globModuleB         = "pkg/b/**"
	globModuleBInternal = "pkg/b/internal/**"

	headRef         = "HEAD"
	kindAdvisory    = "advisory"
	kindGate        = "gate"
	gateFail        = "fail"
	toolNameAstgrep = "ast-grep"
	toolNameScip    = "scip"

	// confidenceHigh is the string literal used in Facts and symbol graph tests.
	confidenceHigh = "high"

	subdomainCore = "core"

	// symbolFoo is the test symbol used in SymbolGraph forwarding tests.
	symbolFoo          = "pkg/a.Foo"
	symbolBar          = "pkg/b.Bar"
	pathFileB          = "pkg/b/b.go"
	strengthModel      = "model"
	strengthFunctional = "functional"
)

// cannedConfig builds a ClassifyConfig and RuleConfig for a two-module (a, b)
// architecture where pkg/a/** belongs to module "a" and pkg/b/** to module "b".
// Module b has a public path (pkg/b/api/**) and an internal path (pkg/b/internal/**).
// Different owners ensure the composite distance signal reaches cross_module_diff_owner
// for advisory test cases that require a non-trivial distance.
func cannedConfig() (config.ClassifyConfig, []rules.Rule) {
	modules := map[string]config.ModuleDef{
		"a": {
			Paths:    []string{globModuleA},
			Public:   []string{globModuleA},
			Internal: []string{},
			Owner:    "team-a",
		},
		"b": {
			Paths:    []string{globModuleB},
			Public:   []string{"pkg/b/api/**"},
			Internal: []string{globModuleBInternal},
			Owner:    "team-b",
		},
	}

	cfg := config.Config{
		Version: 1,
		Modules: modules,
		Rules: []config.RuleDef{
			{
				ID:   rulePublicAPIOnly,
				Type: rulePublicAPIOnly,
				Gate: gateFail,
			},
		},
	}

	classifyCfg := cfg.ForClassify()
	ruleCfg := cfg.ForRules()
	rs, err := rules.New(ruleCfg)
	if err != nil {
		panic("cannedConfig: " + err.Error())
	}
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
				Confidence: confidenceHigh,
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
// The edge is pkg/a → pkg/b/api (contract strength, cross_module_diff_owner distance)
// which is the XOR loose quadrant under the Balanced Coupling model — no advisory.
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
				Confidence: confidenceHigh,
				Locations:  []graph.Location{{File: pathFileA, Line: 3}},
			},
		},
	}
}

// advisoryFacts returns Facts with an imbalanced import edge that produces a BC advisory
// under the Balanced Coupling model: functional strength (high) + cross_module_diff_owner
// distance (high) = symmetric unbalanced → SeverityMedium (high+high, any volatility).
// The StrengthHint "functional" drives the strength since the target has no public/internal glob.
func advisoryFacts() graph.Facts {
	return graph.Facts{
		Language: "go",
		Nodes: []graph.Node{
			{Kind: graph.NodeKindFile, Path: pathFileA},
			{Kind: graph.NodeKindFile, Path: pathFileBInternal},
		},
		Edges: []graph.Edge{
			{
				From:         pathFileANode,
				To:           pathFileBInternalNode,
				Kind:         graph.EdgeKindImports,
				Language:     "go",
				Confidence:   confidenceHigh,
				StrengthHint: strengthFunctional,
				Locations:    []graph.Location{{File: pathFileA, Line: 7}},
			},
		},
	}
}

func TestRun_GateFinding_VerdictFail(t *testing.T) {
	ctx := context.Background()
	facts := violationFacts()

	ex := &ports.ExtractorMock{
		NameFunc: func() string { return "go" },
		ExtractFunc: func(_ context.Context, _ scope.Scope) (graph.Facts, diagnostic.Coverage, error) {
			return facts, diagnostic.Coverage{Tool: "go", Status: "ok", FilesSeen: 2, FilesApplicable: 2}, nil
		},
	}

	classifyCfg, rs := cannedConfig()
	ms := metrics.New(config.Config{Version: 1})
	base := baseline.Baseline{}
	now := time.Date(2026, 6, 7, 0, 0, 0, 0, time.UTC)

	d, err := engine.Run(ctx, engine.RunInput{
		Mode:        engine.Mode{Head: headRef},
		Scope:       scope.Scope{Root: "."},
		Classify:    classifyCfg,
		Staleness:   config.StalenessConfig{},
		Waivers:     config.WaiverSet{},
		Extractors:  []ports.Extractor{ex},
		Patterns:    ports.NopPatternProvider{},
		Resolver:    ports.NopSymbolResolver{},
		PatternCfg:  config.PatternConfig{},
		Rules:       rs,
		Metrics:     ms,
		Accepted:    base,
		BaseMetrics: base.Metrics,
		Labels:      nil,
		Signals:     signal.RunSignals{},
		Now:         now,
	})
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
		if f.RuleID == "public_api_only" && f.Kind == kindGate {
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

	ex := &ports.ExtractorMock{
		NameFunc: func() string { return "go" },
		ExtractFunc: func(_ context.Context, _ scope.Scope) (graph.Facts, diagnostic.Coverage, error) {
			return facts, diagnostic.Coverage{Tool: "go", Status: "ok", FilesSeen: 2, FilesApplicable: 2}, nil
		},
	}

	classifyCfg, rs := cannedConfig()
	ms := metrics.New(config.Config{Version: 1})
	base := baseline.Baseline{}
	now := time.Date(2026, 6, 7, 0, 0, 0, 0, time.UTC)

	d, err := engine.Run(ctx, engine.RunInput{
		Mode:        engine.Mode{Head: headRef},
		Scope:       scope.Scope{Root: "."},
		Classify:    classifyCfg,
		Staleness:   config.StalenessConfig{},
		Waivers:     config.WaiverSet{},
		Extractors:  []ports.Extractor{ex},
		Patterns:    ports.NopPatternProvider{},
		Resolver:    ports.NopSymbolResolver{},
		PatternCfg:  config.PatternConfig{},
		Rules:       rs,
		Metrics:     ms,
		Accepted:    base,
		BaseMetrics: base.Metrics,
		Labels:      nil,
		Signals:     signal.RunSignals{},
		Now:         now,
	})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}

	if d.Verdict != diagnostic.VerdictPass {
		t.Errorf("verdict=%q, want %q", d.Verdict, diagnostic.VerdictPass)
	}

	// No new gate findings.
	for _, f := range d.Findings {
		if f.Kind == kindGate && (f.Status == finding.StatusNew || f.Status == finding.StatusExpiredWaiver) {
			t.Errorf("unexpected new gate finding: %+v", f)
		}
	}

	if d.Summary.GateFindings != 0 {
		t.Errorf("summary.gate_findings=%d, want 0", d.Summary.GateFindings)
	}
}

// TestRun_PerExtractorFailureIsolation guards Fix group 0 Task 0.3
// (reproduces H2's shared root cause): engine.go's extract loop
// (internal/engine/engine.go extract, the `for _, ex := range in.Extractors`
// loop) returns the FIRST extractor error and discards every already-built
// graph.Facts from any extractor that already succeeded — one flaky
// subprocess in one language currently zeroes out every other language's
// results too. In a 2-language config where one extractor's subprocess
// fails (non-zero exit / error) and the other succeeds, the run must still
// produce output for the surviving extractor (here: the public_api_only
// gate finding derived from the Go extractor's facts), with the failed
// extractor only producing a coverage gap — not a total abort. RED today:
// Run returns the raw extractor error and an empty diagnostic.Diagnostic{},
// discarding the Go extractor's already-extracted facts.
func TestRun_PerExtractorFailureIsolation(t *testing.T) {
	ctx := context.Background()
	facts := violationFacts()

	const failedTool = "python"
	extractErr := errors.New("python extractor: ast-grep exited with status 1")

	failingEx := &ports.ExtractorMock{
		NameFunc: func() string { return failedTool },
		ExtractFunc: func(_ context.Context, _ scope.Scope) (graph.Facts, diagnostic.Coverage, error) {
			return graph.Facts{}, diagnostic.Coverage{}, extractErr
		},
	}
	healthyEx := &ports.ExtractorMock{
		NameFunc: func() string { return "go" },
		ExtractFunc: func(_ context.Context, _ scope.Scope) (graph.Facts, diagnostic.Coverage, error) {
			return facts, diagnostic.Coverage{Tool: "go", Status: diagnostic.StatusOK, FilesSeen: 2, FilesApplicable: 2}, nil
		},
	}

	classifyCfg, rs := cannedConfig()
	ms := metrics.New(config.Config{Version: 1})
	base := baseline.Baseline{}
	now := time.Date(2026, 6, 7, 0, 0, 0, 0, time.UTC)

	d, err := engine.Run(ctx, engine.RunInput{
		Mode:        engine.Mode{Head: headRef},
		Scope:       scope.Scope{Root: "."},
		Classify:    classifyCfg,
		Staleness:   config.StalenessConfig{},
		Waivers:     config.WaiverSet{},
		Extractors:  []ports.Extractor{failingEx, healthyEx},
		Patterns:    ports.NopPatternProvider{},
		Resolver:    ports.NopSymbolResolver{},
		PatternCfg:  config.PatternConfig{},
		Rules:       rs,
		Metrics:     ms,
		Accepted:    base,
		BaseMetrics: base.Metrics,
		Labels:      nil,
		Signals:     signal.RunSignals{},
		Now:         now,
	})
	if err != nil {
		t.Fatalf("Run: %v — one failed extractor must not abort the whole run; want the healthy extractor's (go) result preserved", err)
	}

	// The Go extractor's facts must still have flowed through: the
	// public_api_only gate finding derived from violationFacts() must be
	// present, exactly as it is in TestRun_GateFinding_VerdictFail with a
	// single healthy extractor.
	var gateFinding *finding.Finding
	for i := range d.Findings {
		f := &d.Findings[i]
		if f.RuleID == rulePublicAPIOnly && f.Kind == kindGate {
			gateFinding = f
			break
		}
	}
	if gateFinding == nil {
		t.Fatalf("no gate finding with rule_id=%s found — healthy extractor's facts were discarded; findings=%+v", rulePublicAPIOnly, d.Findings)
	}

	// The failed extractor must surface as a coverage gap, not silent
	// disappearance: a tool_coverage entry for it must exist and must not
	// report ok.
	var failedCov *diagnostic.Coverage
	for i := range d.ToolCoverage {
		if d.ToolCoverage[i].Tool == failedTool {
			failedCov = &d.ToolCoverage[i]
			break
		}
	}
	if failedCov == nil {
		t.Errorf("no tool_coverage entry for failed extractor %q — failure must surface, not vanish silently", failedTool)
	} else if failedCov.Status == diagnostic.StatusOK {
		t.Errorf("tool_coverage for %q reports status=ok, want a non-ok coverage gap", failedTool)
	}
}

// TestRun_AllExtractorsFail_StillFatal guards the boundary of the
// per-extractor isolation added for Task 1.1: degrading one failed
// extractor to a coverage gap must not silently turn the degenerate case —
// every extractor failing, nothing left to preserve — into a successful
// empty run. That must still error out.
func TestRun_AllExtractorsFail_StillFatal(t *testing.T) {
	ctx := context.Background()

	failingA := &ports.ExtractorMock{
		NameFunc: func() string { return "python" },
		ExtractFunc: func(_ context.Context, _ scope.Scope) (graph.Facts, diagnostic.Coverage, error) {
			return graph.Facts{}, diagnostic.Coverage{}, errors.New("python extractor: ast-grep exited with status 1")
		},
	}
	failingB := &ports.ExtractorMock{
		NameFunc: func() string { return "go" },
		ExtractFunc: func(_ context.Context, _ scope.Scope) (graph.Facts, diagnostic.Coverage, error) {
			return graph.Facts{}, diagnostic.Coverage{}, errors.New("go extractor: go list exited with status 1")
		},
	}

	classifyCfg, rs := cannedConfig()
	ms := metrics.New(config.Config{Version: 1})
	base := baseline.Baseline{}
	now := time.Date(2026, 6, 7, 0, 0, 0, 0, time.UTC)

	_, err := engine.Run(ctx, engine.RunInput{
		Mode:        engine.Mode{Head: headRef},
		Scope:       scope.Scope{Root: "."},
		Classify:    classifyCfg,
		Staleness:   config.StalenessConfig{},
		Waivers:     config.WaiverSet{},
		Extractors:  []ports.Extractor{failingA, failingB},
		Patterns:    ports.NopPatternProvider{},
		Resolver:    ports.NopSymbolResolver{},
		PatternCfg:  config.PatternConfig{},
		Rules:       rs,
		Metrics:     ms,
		Accepted:    base,
		BaseMetrics: base.Metrics,
		Labels:      nil,
		Signals:     signal.RunSignals{},
		Now:         now,
	})
	if err == nil {
		t.Fatal("Run: got nil error, want an error — every extractor failed, there is nothing to preserve")
	}
}

func TestRun_DiagnosticShape(t *testing.T) {
	ctx := context.Background()

	ex := &ports.ExtractorMock{
		NameFunc: func() string { return "go" },
		ExtractFunc: func(_ context.Context, _ scope.Scope) (graph.Facts, diagnostic.Coverage, error) {
			return graph.Facts{Language: "go"}, diagnostic.Coverage{Tool: "go", Status: "ok"}, nil
		},
	}

	classifyCfg, rs := cannedConfig()
	ms := metrics.New(config.Config{Version: 1})
	base := baseline.Baseline{}
	now := time.Now()

	d, err := engine.Run(ctx, engine.RunInput{
		Mode:        engine.Mode{Base: "main", Head: "feature"},
		Scope:       scope.Scope{Root: "."},
		Classify:    classifyCfg,
		Staleness:   config.StalenessConfig{},
		Waivers:     config.WaiverSet{},
		Extractors:  []ports.Extractor{ex},
		Patterns:    ports.NopPatternProvider{},
		Resolver:    ports.NopSymbolResolver{},
		PatternCfg:  config.PatternConfig{},
		Rules:       rs,
		Metrics:     ms,
		Accepted:    base,
		BaseMetrics: base.Metrics,
		Labels:      nil,
		Signals:     signal.RunSignals{},
		Now:         now,
	})
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
	// Metrics should contain all registered metrics (the book + structural-canon
	// keep set: encapsulation, unbalanced_edge, cycle, coverage, blast_radius).
	if len(d.Metrics) != 5 {
		t.Errorf("len(metrics)=%d, want 5", len(d.Metrics))
	}
	// Volatility triage disclosure wiring: modules are configured (cannedConfig),
	// so the module volatility source counts must reach the summary.
	if d.ClassifiedEdges == nil || d.ClassifiedEdges.VolatilityProvenance == nil {
		t.Errorf("classified_edges.volatility_provenance is nil, want module volatility source counts")
	}
}

// TestRun_PrimaryExtractorTools_Forwarded asserts the injected primary-extractor
// tool list is copied verbatim onto the Diagnostic so the score package can read
// it instead of a hardcoded literal. Empty input leaves the field empty (score
// falls back to its default set).
func TestRun_PrimaryExtractorTools_Forwarded(t *testing.T) {
	ctx := context.Background()
	ex := &ports.ExtractorMock{
		NameFunc: func() string { return "go" },
		ExtractFunc: func(_ context.Context, _ scope.Scope) (graph.Facts, diagnostic.Coverage, error) {
			return graph.Facts{Language: "go"}, diagnostic.Coverage{Tool: "go", Status: "ok"}, nil
		},
	}
	classifyCfg, rs := cannedConfig()
	ms := metrics.New(config.Config{Version: 1})
	base := baseline.Baseline{}

	build := func(tools []string) engine.RunInput {
		return engine.RunInput{
			Mode:                  engine.Mode{},
			Scope:                 scope.Scope{Root: "."},
			Classify:              classifyCfg,
			Waivers:               config.WaiverSet{},
			Extractors:            []ports.Extractor{ex},
			Patterns:              ports.NopPatternProvider{},
			Resolver:              ports.NopSymbolResolver{},
			Rules:                 rs,
			Metrics:               ms,
			Accepted:              base,
			BaseMetrics:           base.Metrics,
			Now:                   time.Now(),
			PrimaryExtractorTools: tools,
		}
	}

	want := []string{"go/packages", "cargo"}
	d, err := engine.Run(ctx, build(want))
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if !slices.Equal(d.PrimaryExtractorTools, want) {
		t.Errorf("PrimaryExtractorTools=%v, want %v", d.PrimaryExtractorTools, want)
	}

	empty, err := engine.Run(ctx, build(nil))
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if len(empty.PrimaryExtractorTools) != 0 {
		t.Errorf("PrimaryExtractorTools=%v, want empty", empty.PrimaryExtractorTools)
	}
}

// TestRun_Advisory_FilteredWhenDisabled asserts that advisory findings do NOT appear
// in Findings and Summary.Warnings is 0 when mode.Advisory = false.
// The violation graph (intrusive edge a→b/internal) will produce a coupling advisory,
// but it must be suppressed.
func TestRun_Advisory_FilteredWhenDisabled(t *testing.T) {
	ctx := context.Background()
	ex := &ports.ExtractorMock{
		NameFunc: func() string { return "go" },
		ExtractFunc: func(_ context.Context, _ scope.Scope) (graph.Facts, diagnostic.Coverage, error) {
			return violationFacts(), diagnostic.Coverage{Tool: "go", Status: "ok"}, nil
		},
	}

	classifyCfg, rs := cannedConfig()
	ms := metrics.New(config.Config{Version: 1})
	base := baseline.Baseline{}
	now := time.Date(2026, 6, 7, 0, 0, 0, 0, time.UTC)

	d, err := engine.Run(ctx, engine.RunInput{
		Mode:        engine.Mode{Head: headRef, Advisory: false},
		Scope:       scope.Scope{Root: "."},
		Classify:    classifyCfg,
		Staleness:   config.StalenessConfig{},
		Waivers:     config.WaiverSet{},
		Extractors:  []ports.Extractor{ex},
		Patterns:    ports.NopPatternProvider{},
		Resolver:    ports.NopSymbolResolver{},
		PatternCfg:  config.PatternConfig{},
		Rules:       rs,
		Metrics:     ms,
		Accepted:    base,
		BaseMetrics: base.Metrics,
		Labels:      nil,
		Signals:     signal.RunSignals{},
		Now:         now,
	})
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
	// advisoryFacts: functional imports edge a→b/internal → high strength + high distance
	// (cross_module_diff_owner, no owners set in cannedConfig) → SeverityMedium advisory.
	ex := &ports.ExtractorMock{
		NameFunc: func() string { return "go" },
		ExtractFunc: func(_ context.Context, _ scope.Scope) (graph.Facts, diagnostic.Coverage, error) {
			return advisoryFacts(), diagnostic.Coverage{Tool: "go", Status: "ok"}, nil
		},
	}

	classifyCfg, rs := cannedConfig()
	ms := metrics.New(config.Config{Version: 1})
	base := baseline.Baseline{}
	now := time.Date(2026, 6, 7, 0, 0, 0, 0, time.UTC)

	d, err := engine.Run(ctx, engine.RunInput{
		Mode:        engine.Mode{Head: headRef, Advisory: true},
		Scope:       scope.Scope{Root: "."},
		Classify:    classifyCfg,
		Staleness:   config.StalenessConfig{},
		Waivers:     config.WaiverSet{},
		Extractors:  []ports.Extractor{ex},
		Patterns:    ports.NopPatternProvider{},
		Resolver:    ports.NopSymbolResolver{},
		PatternCfg:  config.PatternConfig{},
		Rules:       rs,
		Metrics:     ms,
		Accepted:    base,
		BaseMetrics: base.Metrics,
		Labels:      nil,
		Signals:     signal.RunSignals{},
		Now:         now,
	})
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

// TestRun_Advisory_NumericScoreFields asserts that the numeric BC score (value +
// band) is attached to advisory findings' MatchedBy, not just the scorer name.
// Regression for the "score computed then discarded" gap (Task 2).
func TestRun_Advisory_NumericScoreFields(t *testing.T) {
	ctx := context.Background()
	ex := &ports.ExtractorMock{
		NameFunc: func() string { return "go" },
		ExtractFunc: func(_ context.Context, _ scope.Scope) (graph.Facts, diagnostic.Coverage, error) {
			return advisoryFacts(), diagnostic.Coverage{Tool: "go", Status: "ok"}, nil
		},
	}

	classifyCfg, rs := cannedConfig()
	ms := metrics.New(config.Config{Version: 1})
	base := baseline.Baseline{}
	now := time.Date(2026, 6, 7, 0, 0, 0, 0, time.UTC)

	d, err := engine.Run(ctx, engine.RunInput{
		Mode:        engine.Mode{Head: headRef, Advisory: true},
		Scope:       scope.Scope{Root: "."},
		Classify:    classifyCfg,
		Staleness:   config.StalenessConfig{},
		Waivers:     config.WaiverSet{},
		Extractors:  []ports.Extractor{ex},
		Patterns:    ports.NopPatternProvider{},
		Resolver:    ports.NopSymbolResolver{},
		PatternCfg:  config.PatternConfig{},
		Rules:       rs,
		Metrics:     ms,
		Accepted:    base,
		BaseMetrics: base.Metrics,
		Labels:      nil,
		Signals:     signal.RunSignals{},
		Now:         now,
	})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}

	var adv *finding.Finding
	for i := range d.Findings {
		if d.Findings[i].RuleID == engine.RuleIDBCImbalanced {
			adv = &d.Findings[i]
			break
		}
	}
	if adv == nil {
		t.Fatalf("no bc/imbalanced_coupling advisory present; findings=%+v", d.Findings)
	}

	// score = scorer name (default is BookScorer as of bc_score.v3).
	if got := adv.MatchedBy["score"]; got != "book" {
		t.Errorf("MatchedBy[score]=%q, want scorer name %q", got, "book")
	}

	// score_value must parse to an int in [1,10] (BookScorer balance range).
	rawValue, ok := adv.MatchedBy["score_value"]
	if !ok {
		t.Fatalf("MatchedBy missing score_value; got %+v", adv.MatchedBy)
	}
	value, err := strconv.Atoi(rawValue)
	if err != nil {
		t.Fatalf("score_value %q not an integer: %v", rawValue, err)
	}
	if value < 1 || value > 10 {
		t.Errorf("score_value=%d out of range [1,10]", value)
	}

	// score_band must equal the band derived from the value (consistency).
	wantBand := string(coupling.ScoreBand(value))
	if got := adv.MatchedBy["score_band"]; got != wantBand {
		t.Errorf("MatchedBy[score_band]=%q, want %q (band for value %d)", got, wantBand, value)
	}

	// score_version pins the BC score formula version so a deliberate ordinal/
	// formula change is observable in output.
	if got := adv.MatchedBy["score_version"]; got != coupling.ScoreVersion {
		t.Errorf("MatchedBy[score_version]=%q, want %q", got, coupling.ScoreVersion)
	}

	// cheapest_move: the edge lands in module b's Internal glob, so classify
	// upgrades strength to intrusive regardless of the functional hint (S=10,
	// diff-owner D=7, undeclared V=10 → balance 4, high). Reducing strength one
	// rung (intrusive→symmetric) does not drop the band, but reducing distance
	// (diff-owner→same-owner) does — reduce_distance is the only real lever.
	if got := adv.MatchedBy["cheapest_move"]; got != "reduce_distance" {
		t.Errorf("MatchedBy[cheapest_move]=%q, want %q", got, "reduce_distance")
	}
}

// TestRun_Advisory_DistanceBasisInMatchedBy asserts that advisory findings include
// distance_basis in their MatchedBy map when the distance basis is known. This makes
// the distance signal auditable in JSON output.
func TestRun_Advisory_DistanceBasisInMatchedBy(t *testing.T) {
	ctx := context.Background()
	ex := &ports.ExtractorMock{
		NameFunc: func() string { return "go" },
		ExtractFunc: func(_ context.Context, _ scope.Scope) (graph.Facts, diagnostic.Coverage, error) {
			return advisoryFacts(), diagnostic.Coverage{Tool: "go", Status: "ok"}, nil
		},
	}

	classifyCfg, rs := cannedConfig()
	ms := metrics.New(config.Config{Version: 1})
	base := baseline.Baseline{}
	now := time.Date(2026, 6, 7, 0, 0, 0, 0, time.UTC)

	d, err := engine.Run(ctx, engine.RunInput{
		Mode:        engine.Mode{Head: headRef, Advisory: true},
		Scope:       scope.Scope{Root: "."},
		Classify:    classifyCfg,
		Staleness:   config.StalenessConfig{},
		Waivers:     config.WaiverSet{},
		Extractors:  []ports.Extractor{ex},
		Patterns:    ports.NopPatternProvider{},
		Resolver:    ports.NopSymbolResolver{},
		PatternCfg:  config.PatternConfig{},
		Rules:       rs,
		Metrics:     ms,
		Accepted:    base,
		BaseMetrics: base.Metrics,
		Labels:      nil,
		Signals:     signal.RunSignals{},
		Now:         now,
	})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}

	var adv *finding.Finding
	for i := range d.Findings {
		if d.Findings[i].RuleID == engine.RuleIDBCImbalanced {
			adv = &d.Findings[i]
			break
		}
	}
	if adv == nil {
		t.Fatalf("no bc/imbalanced_coupling advisory; findings=%+v", d.Findings)
	}

	// cannedConfig uses distinct owners (team-a / team-b) → ownership basis.
	basis, ok := adv.MatchedBy["distance_basis"]
	if !ok || basis != "ownership" {
		t.Errorf("MatchedBy[distance_basis] = %q, want \"ownership\"; MatchedBy=%+v", basis, adv.MatchedBy)
	}
}

// bcFloodFacts returns Facts with n distinct same-shape edges, all from module a
// (pkg/a) into module b's internals (pkg/b/internal) with functional strength — the
// flood pattern that Task 5 collapses into a single rollup advisory.
func bcFloodFacts(n int) graph.Facts {
	f := graph.Facts{Language: "go"}
	for i := 0; i < n; i++ {
		from := "pkg/a/a" + strconv.Itoa(i) + ".go"
		to := "pkg/b/internal/impl" + strconv.Itoa(i) + ".go"
		f.Nodes = append(f.Nodes,
			graph.Node{Kind: graph.NodeKindFile, Path: from},
			graph.Node{Kind: graph.NodeKindFile, Path: to},
		)
		f.Edges = append(f.Edges, graph.Edge{
			From:         "file:" + from,
			To:           "file:" + to,
			Kind:         graph.EdgeKindImports,
			Language:     "go",
			Confidence:   confidenceHigh,
			StrengthHint: strengthFunctional,
			Locations:    []graph.Location{{File: from, Line: 7}},
		})
	}
	return f
}

// TestRun_Advisory_GroupedRollups asserts that N same-shape BC advisory edges between
// the same module pair collapse into ONE rollup advisory carrying group_count=N and
// sorted representative member IDs, with all member locations merged and the result
// deterministic across runs (Task 5).
func TestRun_Advisory_GroupedRollups(t *testing.T) {
	ctx := context.Background()
	const edges = 5
	ex := &ports.ExtractorMock{
		NameFunc: func() string { return "go" },
		ExtractFunc: func(_ context.Context, _ scope.Scope) (graph.Facts, diagnostic.Coverage, error) {
			return bcFloodFacts(edges), diagnostic.Coverage{Tool: "go", Status: "ok"}, nil
		},
	}
	classifyCfg, rs := cannedConfig()
	ms := metrics.New(config.Config{Version: 1})
	base := baseline.Baseline{}
	now := time.Date(2026, 6, 7, 0, 0, 0, 0, time.UTC)

	run := func() diagnostic.Diagnostic {
		t.Helper()
		d, err := engine.Run(ctx, engine.RunInput{
			Mode:        engine.Mode{Head: headRef, Advisory: true},
			Scope:       scope.Scope{Root: "."},
			Classify:    classifyCfg,
			Staleness:   config.StalenessConfig{},
			Waivers:     config.WaiverSet{},
			Extractors:  []ports.Extractor{ex},
			Patterns:    ports.NopPatternProvider{},
			Resolver:    ports.NopSymbolResolver{},
			PatternCfg:  config.PatternConfig{},
			Rules:       rs,
			Metrics:     ms,
			Accepted:    base,
			BaseMetrics: base.Metrics,
			Labels:      nil,
			Signals:     signal.RunSignals{},
			Now:         now,
		})
		if err != nil {
			t.Fatalf("Run: %v", err)
		}
		return d
	}

	d := run()

	var bc []finding.Finding
	for _, f := range d.Findings {
		if f.RuleID == engine.RuleIDBCImbalanced {
			bc = append(bc, f)
		}
	}
	if len(bc) != 1 {
		t.Fatalf("want 1 grouped BC advisory for %d same-shape edges, got %d: %+v", edges, len(bc), bc)
	}
	rollup := bc[0]

	if got := rollup.MatchedBy["group_count"]; got != strconv.Itoa(edges) {
		t.Errorf("group_count=%q, want %d", got, edges)
	}
	members := strings.Split(rollup.MatchedBy["group_members"], ",")
	if len(members) != edges { // edges < bcAdvisoryRollupCap, so all are listed
		t.Fatalf("group_members has %d ids, want %d: %v", len(members), edges, members)
	}
	for i := 1; i < len(members); i++ {
		if members[i-1] >= members[i] {
			t.Errorf("group_members not sorted ascending (non-deterministic): %v", members)
		}
	}
	// Every member's file:line evidence is preserved on the rollup.
	if len(rollup.Locations) != edges {
		t.Errorf("rollup locations=%d, want %d (one per merged edge)", len(rollup.Locations), edges)
	}
	// Warnings count the rollup, consistent with the single advisory in Findings.
	if d.Summary.Warnings != 1 {
		t.Errorf("warnings=%d, want 1 (one rollup)", d.Summary.Warnings)
	}

	// Determinism: a second run encodes byte-identically.
	first, err := json.Marshal(d)
	if err != nil {
		t.Fatalf("marshal first: %v", err)
	}
	second, err := json.Marshal(run())
	if err != nil {
		t.Fatalf("marshal second: %v", err)
	}
	if !bytes.Equal(first, second) {
		t.Errorf("grouped advisory output not deterministic across runs")
	}
}

// TestRun_Advisory_GroupedRollup_EdgePathHonesty asserts that a rolled-up BC
// advisory's edge.from.path/edge.to.path name a real underlying member edge
// that is also present in the rollup's own locations[] — never an arbitrary
// hash-ID-ordered representative that can point at a completely different
// file than locations[0] (Task 2, wave3-output-truthfulness).
func TestRun_Advisory_GroupedRollup_EdgePathHonesty(t *testing.T) {
	ctx := context.Background()
	const edges = 5
	ex := &ports.ExtractorMock{
		NameFunc: func() string { return "go" },
		ExtractFunc: func(_ context.Context, _ scope.Scope) (graph.Facts, diagnostic.Coverage, error) {
			return bcFloodFacts(edges), diagnostic.Coverage{Tool: "go", Status: "ok"}, nil
		},
	}
	classifyCfg, rs := cannedConfig()
	ms := metrics.New(config.Config{Version: 1})
	base := baseline.Baseline{}
	now := time.Date(2026, 6, 7, 0, 0, 0, 0, time.UTC)

	d, err := engine.Run(ctx, engine.RunInput{
		Mode:        engine.Mode{Head: headRef, Advisory: true},
		Scope:       scope.Scope{Root: "."},
		Classify:    classifyCfg,
		Staleness:   config.StalenessConfig{},
		Waivers:     config.WaiverSet{},
		Extractors:  []ports.Extractor{ex},
		Patterns:    ports.NopPatternProvider{},
		Resolver:    ports.NopSymbolResolver{},
		PatternCfg:  config.PatternConfig{},
		Rules:       rs,
		Metrics:     ms,
		Accepted:    base,
		BaseMetrics: base.Metrics,
		Labels:      nil,
		Signals:     signal.RunSignals{},
		Now:         now,
	})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}

	var rollup finding.Finding
	found := false
	for _, f := range d.Findings {
		if f.RuleID == engine.RuleIDBCImbalanced {
			rollup = f
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("no grouped BC advisory found")
	}
	if got := rollup.MatchedBy["group_count"]; got != strconv.Itoa(edges) {
		t.Fatalf("group_count=%q, want %d", got, edges)
	}
	if len(rollup.Locations) == 0 {
		t.Fatalf("rollup has no locations")
	}

	// bcFloodFacts wires pkg/a/aN.go -> pkg/b/internal/implN.go; locations are
	// sorted by (File, Line), so "pkg/a/a0.go" sorts first among a0..a4.
	wantFrom := "pkg/a/a0.go"
	wantTo := "pkg/b/internal/impl0.go"
	if rollup.Locations[0].File != wantFrom {
		t.Fatalf("test setup: locations[0].File=%q, want %q", rollup.Locations[0].File, wantFrom)
	}
	if rollup.Edge.From.Path != wantFrom {
		t.Errorf("edge.from.path=%q, want %q (locations[0].File)", rollup.Edge.From.Path, wantFrom)
	}
	if rollup.Edge.To.Path != wantTo {
		t.Errorf("edge.to.path=%q, want %q (the real target of edge.from.path, not an unrelated member)", rollup.Edge.To.Path, wantTo)
	}
	foundInLocs := false
	for _, l := range rollup.Locations {
		if l.File == rollup.Edge.From.Path {
			foundInLocs = true
			break
		}
	}
	if !foundInLocs {
		t.Errorf("edge.from.path=%q not present in locations[] %v", rollup.Edge.From.Path, rollup.Locations)
	}
}

// TestRun_Advisory_VerdictUnchanged asserts that advisory findings do NOT change
// a fail verdict: a gate violation still fails even when advisories are present.
func TestRun_Advisory_VerdictUnchanged(t *testing.T) {
	ctx := context.Background()
	ex := &ports.ExtractorMock{
		NameFunc: func() string { return "go" },
		ExtractFunc: func(_ context.Context, _ scope.Scope) (graph.Facts, diagnostic.Coverage, error) {
			return violationFacts(), diagnostic.Coverage{Tool: "go", Status: "ok"}, nil
		},
	}

	classifyCfg, rs := cannedConfig()
	ms := metrics.New(config.Config{Version: 1})
	base := baseline.Baseline{}
	now := time.Date(2026, 6, 7, 0, 0, 0, 0, time.UTC)

	d, err := engine.Run(ctx, engine.RunInput{
		Mode:        engine.Mode{Head: headRef, Advisory: true},
		Scope:       scope.Scope{Root: "."},
		Classify:    classifyCfg,
		Staleness:   config.StalenessConfig{},
		Waivers:     config.WaiverSet{},
		Extractors:  []ports.Extractor{ex},
		Patterns:    ports.NopPatternProvider{},
		Resolver:    ports.NopSymbolResolver{},
		PatternCfg:  config.PatternConfig{},
		Rules:       rs,
		Metrics:     ms,
		Accepted:    base,
		BaseMetrics: base.Metrics,
		Labels:      nil,
		Signals:     signal.RunSignals{},
		Now:         now,
	})
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

// TestRun_PatternProvider_MatchesPropagated asserts that pattern matches returned
// by the PatternProvider are converted and passed as Evidence.PatternMatches to rules.
// We verify this by checking that Find was called and coverage record is present.
func TestRun_PatternProvider_MatchesPropagated(t *testing.T) {
	ctx := context.Background()

	ex := &ports.ExtractorMock{
		NameFunc: func() string { return "go" },
		ExtractFunc: func(_ context.Context, _ scope.Scope) (graph.Facts, diagnostic.Coverage, error) {
			return cleanFacts(), diagnostic.Coverage{Tool: "go", Status: "ok"}, nil
		},
	}

	// PatternProvider returns one known match.
	findCalled := false
	pp := &ports.PatternProviderMock{
		NameFunc: func() string { return toolNameAstgrep },
		FindFunc: func(_ context.Context, _ scope.Scope, _ config.PatternConfig) ([]pattern.Match, diagnostic.Coverage, error) {
			findCalled = true
			return []pattern.Match{
				{File: pathFileA, Pattern: "unsafe-cast", Text: "unsafe.Pointer(x)", Line: 10, Column: 0},
			}, diagnostic.Coverage{Tool: toolNameAstgrep, Status: "ok", FilesSeen: 1}, nil
		},
	}

	classifyCfg, rs := cannedConfig()
	ms := metrics.New(config.Config{Version: 1})
	base := baseline.Baseline{}
	now := time.Date(2026, 6, 7, 0, 0, 0, 0, time.UTC)

	d, err := engine.Run(ctx, engine.RunInput{
		Mode:        engine.Mode{Head: headRef},
		Scope:       scope.Scope{Root: "."},
		Classify:    classifyCfg,
		Staleness:   config.StalenessConfig{},
		Waivers:     config.WaiverSet{},
		Extractors:  []ports.Extractor{ex},
		Patterns:    pp,
		Resolver:    ports.NopSymbolResolver{},
		PatternCfg:  config.PatternConfig{},
		Rules:       rs,
		Metrics:     ms,
		Accepted:    base,
		BaseMetrics: base.Metrics,
		Labels:      nil,
		Signals:     signal.RunSignals{},
		Now:         now,
	})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}

	// PatternProvider.Find must have been called.
	if !findCalled {
		t.Error("PatternProvider.Find was not called")
	}

	// Coverage from PatternProvider must appear in ToolCoverage.
	var found bool
	for _, cov := range d.ToolCoverage {
		if cov.Tool == toolNameAstgrep {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("ast-grep coverage record missing from ToolCoverage; got %+v", d.ToolCoverage)
	}
}

// TestRun_PatternProvider_DoesNotAffectVerdict asserts that pattern matches from
// the PatternProvider do not alter the gate verdict when no gate rule fires.
func TestRun_PatternProvider_DoesNotAffectVerdict(t *testing.T) {
	ctx := context.Background()

	ex := &ports.ExtractorMock{
		NameFunc: func() string { return "go" },
		ExtractFunc: func(_ context.Context, _ scope.Scope) (graph.Facts, diagnostic.Coverage, error) {
			return cleanFacts(), diagnostic.Coverage{Tool: "go", Status: "ok"}, nil
		},
	}

	// PatternProvider returns matches — but no rule uses them to produce gate findings.
	pp := &ports.PatternProviderMock{
		NameFunc: func() string { return toolNameAstgrep },
		FindFunc: func(_ context.Context, _ scope.Scope, _ config.PatternConfig) ([]pattern.Match, diagnostic.Coverage, error) {
			return []pattern.Match{
				{File: pathFileA, Pattern: "reflect-unexported", Text: "reflect.ValueOf(x).Field(0)", Line: 5, Column: 4},
				{File: pathFileBAPIService, Pattern: "reflect-unexported", Text: "reflect.ValueOf(y).Field(1)", Line: 12, Column: 0},
			}, diagnostic.Coverage{Tool: toolNameAstgrep, Status: "ok", FilesSeen: 2}, nil
		},
	}

	classifyCfg, rs := cannedConfig()
	ms := metrics.New(config.Config{Version: 1})
	base := baseline.Baseline{}
	now := time.Date(2026, 6, 7, 0, 0, 0, 0, time.UTC)

	d, err := engine.Run(ctx, engine.RunInput{
		Mode:        engine.Mode{Head: headRef},
		Scope:       scope.Scope{Root: "."},
		Classify:    classifyCfg,
		Staleness:   config.StalenessConfig{},
		Waivers:     config.WaiverSet{},
		Extractors:  []ports.Extractor{ex},
		Patterns:    pp,
		Resolver:    ports.NopSymbolResolver{},
		PatternCfg:  config.PatternConfig{},
		Rules:       rs,
		Metrics:     ms,
		Accepted:    base,
		BaseMetrics: base.Metrics,
		Labels:      nil,
		Signals:     signal.RunSignals{},
		Now:         now,
	})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}

	// Verdict must remain pass — pattern matches alone must not gate.
	if d.Verdict != diagnostic.VerdictPass {
		t.Errorf("verdict=%q, want pass; pattern matches must not affect verdict", d.Verdict)
	}
	if d.Summary.GateFindings != 0 {
		t.Errorf("summary.gate_findings=%d, want 0", d.Summary.GateFindings)
	}
}

// ---------------------------------------------------------------------------
// SymbolGraph forwarding tests
// ---------------------------------------------------------------------------

// spyMetric is a test-only metrics.Metric that captures the MetricInput it receives.
type spyMetric struct {
	captured *signal.CollectedSignals
}

func (s *spyMetric) Name() string    { return "spy" }
func (s *spyMetric) Version() string { return "spy.v1" }
func (s *spyMetric) Calculate(in signal.CollectedSignals) diagnostic.MetricResult {
	*s.captured = in
	return diagnostic.MetricResult{Name: s.Name(), Version: s.Version(), Band: "n/a"}
}

// TestRun_SymbolGraph_ForwardedToMetricInput verifies that a populated symbol.Graph
// returned by the SymbolResolver is threaded through to MetricInput.SymbolGraph.
func TestRun_SymbolGraph_ForwardedToMetricInput(t *testing.T) {
	ctx := context.Background()

	wantGraph := symbol.Graph{
		Module: map[string]string{symbolFoo: "a"},
		FanIn:  map[string]int{symbolFoo: 3},
		Refs:   map[string]map[string]struct{}{symbolFoo: {symbolBar: {}}},
	}

	sr := &ports.SymbolResolverMock{
		NameFunc: func() string { return toolNameScip },
		ResolveFunc: func(_ context.Context, _, toPath string) (string, string) {
			return toPath, confidenceHigh
		},
		StrengthsFunc: func(_ context.Context, _ scope.Scope) (map[string]string, diagnostic.Coverage, error) {
			return nil, diagnostic.Coverage{Tool: toolNameScip, Status: "absent"}, nil
		},
		SymbolsFunc: func(_ context.Context, _ scope.Scope) (symbol.Graph, diagnostic.Coverage, error) {
			return wantGraph, diagnostic.Coverage{Tool: toolNameScip, Status: "ok", FilesSeen: 1, FilesApplicable: 1}, nil
		},
	}

	ex := &ports.ExtractorMock{
		NameFunc: func() string { return "go" },
		ExtractFunc: func(_ context.Context, _ scope.Scope) (graph.Facts, diagnostic.Coverage, error) {
			return cleanFacts(), diagnostic.Coverage{Tool: "go", Status: "ok"}, nil
		},
	}

	var captured signal.CollectedSignals
	spy := &spyMetric{captured: &captured}

	classifyCfg, rs := cannedConfig()
	now := time.Date(2026, 6, 7, 0, 0, 0, 0, time.UTC)

	_, err := engine.Run(ctx, engine.RunInput{
		Mode:        engine.Mode{Head: headRef},
		Scope:       scope.Scope{Root: "."},
		Classify:    classifyCfg,
		Staleness:   config.StalenessConfig{},
		Waivers:     config.WaiverSet{},
		Extractors:  []ports.Extractor{ex},
		Patterns:    ports.NopPatternProvider{},
		Resolver:    sr,
		PatternCfg:  config.PatternConfig{},
		Rules:       rs,
		Metrics:     []metrics.Metric{spy},
		Accepted:    baseline.Baseline{},
		BaseMetrics: nil,
		Labels:      nil,
		Signals:     signal.RunSignals{},
		Now:         now,
	})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}

	if captured.Symbol.Graph.Empty() {
		t.Fatal("MetricInput.SymbolGraph is empty, want populated graph")
	}
	if got := captured.Symbol.Graph.Module["pkg/a.Foo"]; got != "a" {
		t.Errorf("SymbolGraph.Module[pkg/a.Foo]=%q, want %q", got, "a")
	}
	if got := captured.Symbol.Graph.FanIn["pkg/a.Foo"]; got != 3 {
		t.Errorf("SymbolGraph.FanIn[pkg/a.Foo]=%d, want 3", got)
	}
}

// TestRun_SymbolGraph_EmptyWhenNopResolver verifies that NopSymbolResolver results
// in an empty SymbolGraph reaching MetricInput (no SCIP configured).
func TestRun_SymbolGraph_EmptyWhenNopResolver(t *testing.T) {
	ctx := context.Background()

	ex := &ports.ExtractorMock{
		NameFunc: func() string { return "go" },
		ExtractFunc: func(_ context.Context, _ scope.Scope) (graph.Facts, diagnostic.Coverage, error) {
			return cleanFacts(), diagnostic.Coverage{Tool: "go", Status: "ok"}, nil
		},
	}

	var captured signal.CollectedSignals
	spy := &spyMetric{captured: &captured}

	classifyCfg, rs := cannedConfig()
	now := time.Date(2026, 6, 7, 0, 0, 0, 0, time.UTC)

	_, err := engine.Run(ctx, engine.RunInput{
		Mode:        engine.Mode{Head: headRef},
		Scope:       scope.Scope{Root: "."},
		Classify:    classifyCfg,
		Staleness:   config.StalenessConfig{},
		Waivers:     config.WaiverSet{},
		Extractors:  []ports.Extractor{ex},
		Patterns:    ports.NopPatternProvider{},
		Resolver:    ports.NopSymbolResolver{},
		PatternCfg:  config.PatternConfig{},
		Rules:       rs,
		Metrics:     []metrics.Metric{spy},
		Accepted:    baseline.Baseline{},
		BaseMetrics: nil,
		Labels:      nil,
		Signals:     signal.RunSignals{},
		Now:         now,
	})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}

	if !captured.Symbol.Graph.Empty() {
		t.Errorf("MetricInput.SymbolGraph is non-empty with NopSymbolResolver, want empty")
	}
}

// TestRun_FileFacts_AttachedFromSymbolGraph verifies that the engine assembles
// the neutral structural-facts block from the symbol graph + change history and
// attaches it to the diagnostic without affecting the verdict.
func TestRun_FileFacts_AttachedFromSymbolGraph(t *testing.T) {
	ctx := context.Background()

	symGraph := symbol.Graph{
		Module: map[string]string{symbolFoo: "a", symbolBar: "b"},
		Path:   map[string]string{symbolFoo: pathFileA, symbolBar: pathFileB},
		Refs:   map[string]map[string]struct{}{symbolFoo: {symbolBar: {}}},
	}

	sr := &ports.SymbolResolverMock{
		NameFunc: func() string { return toolNameScip },
		ResolveFunc: func(_ context.Context, _, toPath string) (string, string) {
			return toPath, confidenceHigh
		},
		StrengthsFunc: func(_ context.Context, _ scope.Scope) (map[string]string, diagnostic.Coverage, error) {
			return nil, diagnostic.Coverage{Tool: toolNameScip, Status: "absent"}, nil
		},
		SymbolsFunc: func(_ context.Context, _ scope.Scope) (symbol.Graph, diagnostic.Coverage, error) {
			return symGraph, diagnostic.Coverage{Tool: toolNameScip, Status: "ok"}, nil
		},
	}

	ex := &ports.ExtractorMock{
		NameFunc: func() string { return "go" },
		ExtractFunc: func(_ context.Context, _ scope.Scope) (graph.Facts, diagnostic.Coverage, error) {
			return cleanFacts(), diagnostic.Coverage{Tool: "go", Status: "ok"}, nil
		},
	}

	classifyCfg, rs := cannedConfig()
	now := time.Date(2026, 6, 7, 0, 0, 0, 0, time.UTC)
	change := signal.RunSignals{
		Size: signal.SizeSignals{FileLOC: map[string]int{pathFileA: 100, pathFileB: 40}},
	}

	d, err := engine.Run(ctx, engine.RunInput{
		Mode:        engine.Mode{Head: headRef},
		Scope:       scope.Scope{Root: "."},
		Classify:    classifyCfg,
		Staleness:   config.StalenessConfig{},
		Waivers:     config.WaiverSet{},
		Extractors:  []ports.Extractor{ex},
		Patterns:    ports.NopPatternProvider{},
		Resolver:    sr,
		PatternCfg:  config.PatternConfig{},
		Rules:       rs,
		Metrics:     nil,
		Accepted:    baseline.Baseline{},
		BaseMetrics: nil,
		Labels:      nil,
		Signals:     change,
		Now:         now,
	})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}

	if len(d.FileFacts) != 2 {
		t.Fatalf("FileFacts count = %d, want 2 (modules a, b)", len(d.FileFacts))
	}
	a := d.FileFacts[0]
	if a.Module != "a" || a.OutboundDestinations != 1 || a.LOC != 100 {
		t.Errorf("facts[0] = %+v, want module a, outbound 1, LOC 100", a)
	}
	b := d.FileFacts[1]
	if b.Module != "b" || b.InboundModuleFanIn != 1 || b.LOC != 40 {
		t.Errorf("facts[1] = %+v, want module b, inbound 1, LOC 40", b)
	}
}

// TestRun_FileFacts_EmptyWhenNopResolver verifies the facts block is empty
// (non-nil) when no symbol graph is collected.
func TestRun_FileFacts_EmptyWhenNopResolver(t *testing.T) {
	ctx := context.Background()

	ex := &ports.ExtractorMock{
		NameFunc: func() string { return "go" },
		ExtractFunc: func(_ context.Context, _ scope.Scope) (graph.Facts, diagnostic.Coverage, error) {
			return cleanFacts(), diagnostic.Coverage{Tool: "go", Status: "ok"}, nil
		},
	}

	classifyCfg, rs := cannedConfig()
	now := time.Date(2026, 6, 7, 0, 0, 0, 0, time.UTC)

	d, err := engine.Run(ctx, engine.RunInput{
		Mode:        engine.Mode{Head: headRef},
		Scope:       scope.Scope{Root: "."},
		Classify:    classifyCfg,
		Staleness:   config.StalenessConfig{},
		Waivers:     config.WaiverSet{},
		Extractors:  []ports.Extractor{ex},
		Patterns:    ports.NopPatternProvider{},
		Resolver:    ports.NopSymbolResolver{},
		PatternCfg:  config.PatternConfig{},
		Rules:       rs,
		Metrics:     nil,
		Accepted:    baseline.Baseline{},
		BaseMetrics: nil,
		Labels:      nil,
		Signals:     signal.RunSignals{},
		Now:         now,
	})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}

	if d.FileFacts == nil {
		t.Fatal("FileFacts is nil, want empty slice")
	}
	if len(d.FileFacts) != 0 {
		t.Errorf("FileFacts count = %d, want 0 with NopSymbolResolver", len(d.FileFacts))
	}
}

// TestRun_NewCrossModuleDependency_BaselineSemantics verifies the "new"
// semantics of new_cross_module_dependency: every cross-module edge fires at
// the rule stage, but a baselined fingerprint is assigned StatusBaseline and
// does not gate; without a baseline the same edge fails the verdict.
func TestRun_NewCrossModuleDependency_BaselineSemantics(t *testing.T) {
	ctx := context.Background()

	const crossModRuleID = "review_new_deps"
	cfg := config.Config{
		Version: 1,
		Modules: map[string]config.ModuleDef{
			"a": {Paths: []string{globModuleA}},
			"b": {Paths: []string{globModuleB}},
		},
		Rules: []config.RuleDef{{ID: crossModRuleID, Type: "new_cross_module_dependency", Gate: gateFail}},
	}
	classifyCfg := cfg.ForClassify()
	rs, err := rules.New(cfg.ForRules())
	if err != nil {
		t.Fatalf("rules.New: %v", err)
	}

	ex := &ports.ExtractorMock{
		NameFunc: func() string { return "go" },
		ExtractFunc: func(_ context.Context, _ scope.Scope) (graph.Facts, diagnostic.Coverage, error) {
			return cleanFacts(), diagnostic.Coverage{Tool: "go", Status: "ok"}, nil
		},
	}
	now := time.Date(2026, 6, 11, 0, 0, 0, 0, time.UTC)

	run := func(base baseline.Baseline) diagnostic.Diagnostic {
		t.Helper()
		d, err := engine.Run(ctx, engine.RunInput{
			Mode:        engine.Mode{Head: headRef},
			Scope:       scope.Scope{Root: "."},
			Classify:    classifyCfg,
			Staleness:   config.StalenessConfig{},
			Waivers:     config.WaiverSet{},
			Extractors:  []ports.Extractor{ex},
			Patterns:    ports.NopPatternProvider{},
			Resolver:    ports.NopSymbolResolver{},
			PatternCfg:  config.PatternConfig{},
			Rules:       rs,
			Metrics:     nil,
			Accepted:    base,
			BaseMetrics: base.Metrics,
			Labels:      nil,
			Signals:     signal.RunSignals{},
			Now:         now,
		})
		if err != nil {
			t.Fatalf("Run: %v", err)
		}
		return d
	}

	// Without a baseline: the cross-module edge is a new gate finding → fail.
	noBase := run(baseline.Baseline{})
	if noBase.Verdict != diagnostic.VerdictFail {
		t.Fatalf("no baseline: verdict = %q, want fail", noBase.Verdict)
	}
	var fp string
	for _, f := range noBase.Findings {
		if f.RuleID == crossModRuleID && f.Status == finding.StatusNew {
			fp = f.ID
		}
	}
	if fp == "" {
		t.Fatalf("no new %s finding without baseline; findings=%+v", crossModRuleID, noBase.Findings)
	}

	// With the fingerprint baselined: same edge is StatusBaseline → pass,
	// and the finding is still present (NOT suppressed — fixed-detection
	// depends on it remaining in the current set).
	based := run(baseline.Baseline{Accepted: []baseline.AcceptedFinding{
		{Fingerprint: fp, RuleID: crossModRuleID, Kind: kindGate},
	}})
	if based.Verdict != diagnostic.VerdictPass {
		t.Errorf("baselined: verdict = %q, want pass", based.Verdict)
	}
	found := false
	for _, f := range based.Findings {
		if f.ID == fp {
			found = true
			if f.Status != finding.StatusBaseline {
				t.Errorf("baselined finding status = %q, want %q", f.Status, finding.StatusBaseline)
			}
		}
	}
	if !found {
		t.Error("baselined finding missing from output — rule must keep emitting it")
	}
}

// TestRun_PinnedLabels verifies the labels integration end-to-end in the
// engine: an approved fresh label refines edge strength; a stale label is
// ignored and surfaces as a labels/stale advisory.
func TestRun_PinnedLabels(t *testing.T) {
	ctx := context.Background()

	// Modules with paths only — no public/internal globs, so strength is
	// glob-undecided and the label decides.
	cfg := config.Config{
		Version: 1,
		Modules: map[string]config.ModuleDef{
			"a": {Paths: []string{globModuleA}},
			"b": {Paths: []string{globModuleB}},
		},
	}
	ex := &ports.ExtractorMock{
		NameFunc: func() string { return "go" },
		ExtractFunc: func(_ context.Context, _ scope.Scope) (graph.Facts, diagnostic.Coverage, error) {
			return cleanFacts(), diagnostic.Coverage{Tool: "go", Status: "ok"}, nil
		},
	}
	now := time.Date(2026, 6, 11, 0, 0, 0, 0, time.UTC)
	// The engine hashes "fromPath\x00toPath\x00kind" per edge of the pair.
	freshHash := labels.HashItems([]string{pathFileA + "\x00" + pathFileBAPIService + "\x00imports"})

	pinnedRules, err := rules.New(cfg.ForRules())
	if err != nil {
		t.Fatalf("rules.New: %v", err)
	}

	run := func(lbls []labels.Label) (diagnostic.Diagnostic, signal.CollectedSignals) {
		t.Helper()
		var captured signal.CollectedSignals
		spy := &spyMetric{captured: &captured}
		d, err := engine.Run(ctx, engine.RunInput{
			Mode:        engine.Mode{Head: headRef, Advisory: true},
			Scope:       scope.Scope{Root: "."},
			Classify:    cfg.ForClassify(),
			Staleness:   config.StalenessConfig{},
			Waivers:     config.WaiverSet{},
			Extractors:  []ports.Extractor{ex},
			Patterns:    ports.NopPatternProvider{},
			Resolver:    ports.NopSymbolResolver{},
			PatternCfg:  config.PatternConfig{},
			Rules:       pinnedRules,
			Metrics:     []metrics.Metric{spy},
			Accepted:    baseline.Baseline{},
			BaseMetrics: nil,
			Labels:      lbls,
			Signals:     signal.RunSignals{},
			Now:         now,
		})
		if err != nil {
			t.Fatalf("Run: %v", err)
		}
		return d, captured
	}

	edgeStrength := func(in signal.CollectedSignals) string {
		for key, cl := range in.Common.Classifications {
			if strings.Contains(key, pathFileBAPIService) {
				return string(cl.Strength)
			}
		}
		t.Fatal("edge classification not found")
		return ""
	}

	t.Run("approved fresh label refines strength", func(t *testing.T) {
		d, in := run([]labels.Label{{
			From: "a", To: "b", Strength: strengthModel,
			EvidenceHash: freshHash, Status: labels.StatusApproved,
		}})
		if got := edgeStrength(in); got != strengthModel {
			t.Errorf("strength = %q, want model (pinned)", got)
		}
		for _, f := range d.Findings {
			if f.RuleID == ruleLabelsStale {
				t.Errorf("unexpected stale advisory for fresh label: %+v", f)
			}
		}
	})

	t.Run("stale label ignored with advisory", func(t *testing.T) {
		d, in := run([]labels.Label{{
			From: "a", To: "b", Strength: strengthModel,
			EvidenceHash: "deadbeef", Status: labels.StatusApproved,
		}})
		if got := edgeStrength(in); got == strengthModel {
			t.Error("stale label must not be applied")
		}
		found := false
		for _, f := range d.Findings {
			if f.RuleID == ruleLabelsStale && f.Edge.From.Module == "a" && f.Edge.To.Module == "b" {
				found = true
			}
		}
		if !found {
			t.Errorf("labels/stale advisory missing; findings = %+v", d.Findings)
		}
	})

	t.Run("draft label inert", func(t *testing.T) {
		_, in := run([]labels.Label{{
			From: "a", To: "b", Strength: strengthModel,
			EvidenceHash: freshHash, Status: labels.StatusDraft,
		}})
		if got := edgeStrength(in); got == strengthModel {
			t.Error("draft label must never be consumed by the gate")
		}
	})
}

// TestRun_LLMLabels_FillDeterminismAndBucket is the Wave 7 Task 2 guard: with a
// committed llm-provenance label the full pipeline is byte-identical across two
// runs (the LLM ran at enrich time, never here), the filled edge is attributed
// in classified_edges.labeled_llm, and removing the label returns the edge to
// the abstained bucket — no residue of the fill.
func TestRun_LLMLabels_FillDeterminismAndBucket(t *testing.T) {
	ctx := context.Background()

	// Modules with paths only and a hint-less edge (cleanFacts): every static
	// source abstains, so the llm label is the only strength source.
	cfg := config.Config{
		Version: 1,
		Modules: map[string]config.ModuleDef{
			"a": {Paths: []string{globModuleA}},
			"b": {Paths: []string{globModuleB}},
		},
	}
	ex := &ports.ExtractorMock{
		NameFunc: func() string { return "go" },
		ExtractFunc: func(_ context.Context, _ scope.Scope) (graph.Facts, diagnostic.Coverage, error) {
			return cleanFacts(), diagnostic.Coverage{Tool: "go", Status: "ok"}, nil
		},
	}
	rs, err := rules.New(cfg.ForRules())
	if err != nil {
		t.Fatalf("rules.New: %v", err)
	}
	now := time.Date(2026, 7, 4, 0, 0, 0, 0, time.UTC)
	freshHash := labels.HashItems([]string{pathFileA + "\x00" + pathFileBAPIService + "\x00imports"})

	run := func(lbls []labels.Label) (diagnostic.Diagnostic, []byte) {
		t.Helper()
		d, runErr := engine.Run(ctx, engine.RunInput{
			Mode:        engine.Mode{Full: true, Advisory: true},
			Scope:       scope.Scope{Root: "."},
			Classify:    cfg.ForClassify(),
			Staleness:   config.StalenessConfig{},
			Waivers:     config.WaiverSet{},
			Extractors:  []ports.Extractor{ex},
			Patterns:    ports.NopPatternProvider{},
			Resolver:    ports.NopSymbolResolver{},
			PatternCfg:  config.PatternConfig{},
			Rules:       rs,
			Metrics:     metrics.New(config.Config{Version: 1}),
			Accepted:    baseline.Baseline{},
			BaseMetrics: nil,
			Labels:      lbls,
			Signals:     signal.RunSignals{},
			Now:         now,
		})
		if runErr != nil {
			t.Fatalf("Run: %v", runErr)
		}
		raw, mErr := json.Marshal(d)
		if mErr != nil {
			t.Fatalf("marshal: %v", mErr)
		}
		return d, raw
	}

	llmLabel := []labels.Label{{
		From: "a", To: "b", Strength: strengthModel,
		EvidenceHash: freshHash, Status: labels.StatusApproved,
		Provenance: labels.ProvenanceLLM, Confidence: labels.ConfidenceMedium,
	}}

	labeled, firstRaw := run(llmLabel)
	_, secondRaw := run(llmLabel)
	if !bytes.Equal(firstRaw, secondRaw) {
		t.Error("two full runs with a committed llm label differ — gate determinism broken")
	}

	ce := labeled.ClassifiedEdges
	if ce == nil {
		t.Fatal("classified_edges missing")
	}
	if ce.LabeledLLM != 1 {
		t.Errorf("labeled_llm = %d, want 1 (fill attributed to the semantic layer)", ce.LabeledLLM)
	}
	if ce.Abstained != 0 {
		t.Errorf("abstained = %d, want 0 (llm label filled the only unknown cell)", ce.Abstained)
	}
	if ce.Scored < 1 {
		t.Errorf("scored = %d, want >= 1", ce.Scored)
	}

	// Labels removed → the abstain returns.
	bare, _ := run(nil)
	if bare.ClassifiedEdges == nil {
		t.Fatal("classified_edges missing on bare run")
	}
	if bare.ClassifiedEdges.LabeledLLM != 0 {
		t.Errorf("labeled_llm = %d, want 0 without labels", bare.ClassifiedEdges.LabeledLLM)
	}
	if bare.ClassifiedEdges.Abstained != 1 {
		t.Errorf("abstained = %d, want 1 (abstain returns when the label is deleted)", bare.ClassifiedEdges.Abstained)
	}
}

// langPyTest / kindLazy are factored out so the dynamic-import test stays under
// goconst's repeated-literal threshold.
const (
	langPyTest    = "python"
	kindLazy      = "lazy_import"
	kindFuncDecl  = "function_declaration"
	pathPyA       = "pkg/a/x.py"
	moduleScripts = "scripts"

	// Runtime async test constants.
	kindMQ        = "message_queue"
	kindEventBus  = "event_bus"
	kindAsyncTask = "async_task"
	libAmqp       = "amqp"
	libKafka      = "kafka"
	libCelery     = "celery"
	confMedium    = "medium"
)

// dynImportRun executes the engine over cleanFacts with the given dynamic-import
// sites and returns the assembled Diagnostic. The graph is identical regardless
// of the sites — the sites only feed the report-only DynamicImports block.
func dynImportRun(t *testing.T, sites []diagnostic.DynamicImportSite) diagnostic.Diagnostic {
	t.Helper()
	classifyCfg, rs := cannedConfig() // pkg/a/** -> "a", pkg/b/** -> "b"
	ex := &ports.ExtractorMock{
		NameFunc: func() string { return "go" },
		ExtractFunc: func(_ context.Context, _ scope.Scope) (graph.Facts, diagnostic.Coverage, error) {
			return cleanFacts(), diagnostic.Coverage{Tool: "go", Status: "ok"}, nil
		},
	}
	base := baseline.Baseline{}
	in := engine.RunInput{
		Mode: engine.Mode{Head: headRef}, Scope: scope.Scope{Root: "."},
		Classify: classifyCfg, Staleness: config.StalenessConfig{}, Waivers: config.WaiverSet{},
		Extractors: []ports.Extractor{ex}, Patterns: ports.NopPatternProvider{}, Resolver: ports.NopSymbolResolver{},
		PatternCfg: config.PatternConfig{}, Rules: rs, Metrics: metrics.New(config.Config{Version: 1}),
		Accepted: base, BaseMetrics: base.Metrics,
		Signals: signal.RunSignals{DynamicImports: signal.DynamicImportSignals{Sites: sites}},
		Now:     time.Date(2026, 6, 7, 0, 0, 0, 0, time.UTC),
	}
	d, err := engine.Run(context.Background(), in)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	return d
}

// TestRun_DynamicImports_GroupedPerModule asserts the report-only dynamic-import
// sites are rolled up per module (module-map key, or file directory when unmapped),
// counted in full, and sampled with the cap applied — deterministically.
func TestRun_DynamicImports_GroupedPerModule(t *testing.T) {
	var sites []diagnostic.DynamicImportSite
	for i := 1; i <= 7; i++ { // 7 sites in module "a" -> count 7, sample capped at 5
		sites = append(sites, diagnostic.DynamicImportSite{File: pathPyA, Line: i, Kind: kindLazy, Language: langPyTest})
	}
	sites = append(sites,
		diagnostic.DynamicImportSite{File: "pkg/b/y.ts", Line: 2, Kind: "require", Language: "typescript"},
		diagnostic.DynamicImportSite{File: "scripts/tool.py", Line: 9, Kind: "importlib", Language: langPyTest},
	)

	d := dynImportRun(t, sites)

	// Modules sorted by name: "a", "b", "scripts" (last is the unmapped dir fallback).
	want := []struct {
		module string
		count  int
		sample int
	}{
		{"a", 7, 5},
		{"b", 1, 1},
		{moduleScripts, 1, 1},
	}
	if len(d.DynamicImports) != len(want) {
		t.Fatalf("dynamic_imports groups = %d, want %d (%+v)", len(d.DynamicImports), len(want), d.DynamicImports)
	}
	for i, w := range want {
		got := d.DynamicImports[i]
		if got.Module != w.module {
			t.Errorf("group[%d].Module = %q, want %q", i, got.Module, w.module)
		}
		if got.Count != w.count {
			t.Errorf("group[%d].Count = %d, want %d", i, got.Count, w.count)
		}
		if len(got.Sites) != w.sample {
			t.Errorf("group[%d] sample sites = %d, want %d", i, len(got.Sites), w.sample)
		}
	}
}

// TestRun_DynamicImports_StaticGraphUnchanged proves the dynamic-import signal is
// purely additive: a run with sites and a run without them produce a byte-identical
// diagnostic once the DynamicImports block is removed. Nothing graph-, metric-, or
// verdict-derived may change.
func TestRun_DynamicImports_StaticGraphUnchanged(t *testing.T) {
	withSites := dynImportRun(t, []diagnostic.DynamicImportSite{
		{File: pathPyA, Line: 4, Kind: kindLazy, Language: langPyTest},
	})
	withoutSites := dynImportRun(t, nil)

	if len(withSites.DynamicImports) == 0 {
		t.Fatal("expected dynamic imports to be populated in the with-sites run")
	}
	if len(withoutSites.DynamicImports) != 0 {
		t.Errorf("expected empty dynamic imports in the no-sites run, got %+v", withoutSites.DynamicImports)
	}

	withSites.DynamicImports = nil
	withoutSites.DynamicImports = nil
	a, err := json.Marshal(withSites)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	b, err := json.Marshal(withoutSites)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if !bytes.Equal(a, b) {
		t.Errorf("dynamic-import signal altered the rest of the diagnostic:\n with: %s\n without: %s", a, b)
	}
}

// TestRun_DynamicImports_Deterministic asserts a double run with identical sites
// yields byte-identical diagnostics (no map-order leakage in the rollup).
func TestRun_DynamicImports_Deterministic(t *testing.T) {
	sites := []diagnostic.DynamicImportSite{
		{File: "pkg/b/y.ts", Line: 1, Kind: "dynamic_import", Language: "typescript"},
		{File: pathPyA, Line: 2, Kind: kindLazy, Language: langPyTest},
		{File: "scripts/z.py", Line: 3, Kind: "importlib", Language: langPyTest},
	}
	first, err := json.Marshal(dynImportRun(t, sites).DynamicImports)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	second, err := json.Marshal(dynImportRun(t, sites).DynamicImports)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if !bytes.Equal(first, second) {
		t.Errorf("non-deterministic dynamic_imports:\n first:  %s\n second: %s", first, second)
	}
}

// TestRun_GoWorkspace_ModuleMapRebuild verifies that secondary consumers
// (buildDynamicImports, buildRuntimeAsync, diagnostic module-label resolution)
// see the auto-registered Go workspace members after ModuleMap is rebuilt
// post-augmentation (Finding A fix). Before the fix, the stale ModuleMap had
// no entries for auto-registered members and sites would fall back to the
// file's directory (e.g. "svc"), not the module path (e.g. "example.com/svc").
func TestRun_GoWorkspace_ModuleMapRebuild(t *testing.T) {
	// Two-member workspace: "example.com/svc" at svc/, "example.com/lib" at lib/.
	// No modules are declared in config — AugmentGoWorkspaceModules must register them.
	const (
		modPathSvc = "example.com/svc"
		modPathLib = "example.com/lib"
		fileSvc    = "svc/main.go"
	)

	workspaceFacts := graph.Facts{
		Language: "go",
		Nodes: []graph.Node{
			{Kind: graph.NodeKindFile, Path: fileSvc},
			{Kind: graph.NodeKindFile, Path: "lib/util.go"},
		},
		Edges: []graph.Edge{
			{
				From: "file:" + fileSvc, To: "file:lib/util.go",
				Kind: graph.EdgeKindImports, Language: "go", Confidence: confidenceHigh,
			},
		},
		GoModules: []graph.GoModule{
			{Path: modPathSvc, RelDir: "svc"},
			{Path: modPathLib, RelDir: "lib"},
		},
	}

	emptyCfg := config.Config{Version: 1}
	classifyCfg := emptyCfg.ForClassify()
	rs, err := rules.New(emptyCfg.ForRules())
	if err != nil {
		t.Fatalf("rules.New: %v", err)
	}

	ex := &ports.ExtractorMock{
		NameFunc: func() string { return "go" },
		ExtractFunc: func(_ context.Context, _ scope.Scope) (graph.Facts, diagnostic.Coverage, error) {
			return workspaceFacts, diagnostic.Coverage{Tool: "go", Status: "ok"}, nil
		},
	}
	base := baseline.Baseline{}
	in := engine.RunInput{
		Mode: engine.Mode{Head: headRef}, Scope: scope.Scope{Root: "."},
		Classify: classifyCfg, Staleness: config.StalenessConfig{}, Waivers: config.WaiverSet{},
		Extractors: []ports.Extractor{ex}, Patterns: ports.NopPatternProvider{}, Resolver: ports.NopSymbolResolver{},
		PatternCfg: config.PatternConfig{}, Rules: rs, Metrics: metrics.New(config.Config{Version: 1}),
		Accepted: base, BaseMetrics: base.Metrics,
		Signals: signal.RunSignals{
			DynamicImports: signal.DynamicImportSignals{
				Sites: []diagnostic.DynamicImportSite{
					{File: fileSvc, Line: 1, Kind: kindLazy, Language: langPyTest},
				},
			},
		},
		Now: time.Date(2026, 6, 27, 0, 0, 0, 0, time.UTC),
	}
	d, runErr := engine.Run(context.Background(), in)
	if runErr != nil {
		t.Fatalf("Run: %v", runErr)
	}

	if len(d.DynamicImports) == 0 {
		t.Fatal("expected DynamicImports to be populated")
	}
	got := d.DynamicImports[0].Module
	if got != modPathSvc {
		// Before the fix the stale ModuleMap returned "" and the fallback was "svc".
		t.Errorf("DynamicImports[0].Module = %q, want %q (rebuilt ModuleMap must resolve auto-registered workspace member)", got, modPathSvc)
	}
}

func TestRun_GoWorkspace_StalePinnedLabelUsesAugmentedModuleMap(t *testing.T) {
	const (
		modPathSvc = "example.com/svc"
		modPathLib = "example.com/lib"
		fileSvc    = "svc/main.go"
		fileLib    = "lib/util.go"
	)
	workspaceFacts := graph.Facts{
		Language: "go",
		Nodes: []graph.Node{
			{Kind: graph.NodeKindFile, Path: fileSvc},
			{Kind: graph.NodeKindFile, Path: fileLib},
		},
		Edges: []graph.Edge{
			{
				From: "file:" + fileSvc, To: "file:" + fileLib,
				Kind: graph.EdgeKindImports, Language: "go", Confidence: confidenceHigh, StrengthHint: strengthFunctional,
			},
		},
		GoModules: []graph.GoModule{
			{Path: modPathSvc, RelDir: "svc"},
			{Path: modPathLib, RelDir: "lib"},
		},
	}
	ex := &ports.ExtractorMock{
		NameFunc: func() string { return "go" },
		ExtractFunc: func(_ context.Context, _ scope.Scope) (graph.Facts, diagnostic.Coverage, error) {
			return workspaceFacts, diagnostic.Coverage{Tool: "go", Status: "ok"}, nil
		},
	}
	cfg := config.Config{Version: 1}
	rs, err := rules.New(cfg.ForRules())
	if err != nil {
		t.Fatalf("rules.New: %v", err)
	}
	var captured signal.CollectedSignals
	spy := &spyMetric{captured: &captured}
	d, err := engine.Run(context.Background(), engine.RunInput{
		Mode:       engine.Mode{Head: headRef, Advisory: true},
		Scope:      scope.Scope{Root: "."},
		Classify:   cfg.ForClassify(),
		Staleness:  config.StalenessConfig{},
		Waivers:    config.WaiverSet{},
		Extractors: []ports.Extractor{ex},
		Patterns:   ports.NopPatternProvider{},
		Resolver:   ports.NopSymbolResolver{},
		PatternCfg: config.PatternConfig{},
		Rules:      rs,
		Metrics:    []metrics.Metric{spy},
		Accepted:   baseline.Baseline{},
		Labels: []labels.Label{{
			From: modPathSvc, To: modPathLib, Strength: strengthModel,
			EvidenceHash: "deadbeef", Status: labels.StatusApproved,
		}},
		Now: time.Date(2026, 7, 4, 0, 0, 0, 0, time.UTC),
	})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}

	key := "file:" + fileSvc + "\x00" + "file:" + fileLib + "\x00" + string(graph.EdgeKindImports)
	if got := string(captured.Common.Classifications[key].Strength); got != strengthFunctional {
		t.Errorf("stale synthetic-module label strength = %q, want existing hint %q", got, strengthFunctional)
	}
	found := false
	for _, f := range d.Findings {
		if f.RuleID == ruleLabelsStale && f.Edge.From.Module == modPathSvc && f.Edge.To.Module == modPathLib {
			found = true
		}
	}
	if !found {
		t.Fatalf("labels/stale advisory missing for synthetic module pair; findings = %+v", d.Findings)
	}
}

// runtimeAsyncRun executes the engine over cleanFacts with the given runtime-async
// sites and returns the assembled Diagnostic. The graph is identical regardless of
// the sites — the sites only feed the report-only RuntimeAsync and
// RuntimeAsyncEdges blocks; they do not annotate graph edges or affect classify,
// score, or verdict.
func runtimeAsyncRun(t *testing.T, sites []diagnostic.RuntimeAsyncSite, confidence string) diagnostic.Diagnostic {
	t.Helper()
	classifyCfg, rs := cannedConfig()
	ex := &ports.ExtractorMock{
		NameFunc: func() string { return "go" },
		ExtractFunc: func(_ context.Context, _ scope.Scope) (graph.Facts, diagnostic.Coverage, error) {
			return cleanFacts(), diagnostic.Coverage{Tool: "go", Status: "ok"}, nil
		},
	}
	base := baseline.Baseline{}
	in := engine.RunInput{
		Mode: engine.Mode{Head: headRef}, Scope: scope.Scope{Root: "."},
		Classify: classifyCfg, Staleness: config.StalenessConfig{}, Waivers: config.WaiverSet{},
		Extractors: []ports.Extractor{ex}, Patterns: ports.NopPatternProvider{}, Resolver: ports.NopSymbolResolver{},
		PatternCfg: config.PatternConfig{}, Rules: rs, Metrics: metrics.New(config.Config{Version: 1}),
		Accepted: base, BaseMetrics: base.Metrics,
		Signals: signal.RunSignals{RuntimeAsync: signal.RuntimeAsyncSignals{Sites: sites, Confidence: confidence}},
		Now:     time.Date(2026, 6, 22, 0, 0, 0, 0, time.UTC),
	}
	d, err := engine.Run(context.Background(), in)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	return d
}

// TestRun_RuntimeAsync_GroupedPerModule asserts async sites are rolled up per module
// (module-map key, or file directory when unmapped), counted, and sorted.
func TestRun_RuntimeAsync_GroupedPerModule(t *testing.T) {
	sites := []diagnostic.RuntimeAsyncSite{
		{File: pathFileA, Line: 1, Library: libAmqp, IntegrationKind: kindMQ, Language: "go"},
		{File: pathFileA, Line: 2, Library: libAmqp, IntegrationKind: kindMQ, Language: "go"},
		{File: "pkg/b/b.go", Line: 5, Library: libKafka, IntegrationKind: kindEventBus, Language: "go"},
		{File: "scripts/worker.py", Line: 3, Library: libCelery, IntegrationKind: kindAsyncTask, Language: langPyTest},
	}
	d := runtimeAsyncRun(t, sites, confMedium)

	// Modules sorted: "a" (2 sites, message_queue), "b" (1 site, event_bus), "scripts" (unmapped dir)
	want := []struct {
		module string
		kind   string
		count  int
		conf   string
	}{
		{"a", kindMQ, 2, confMedium},
		{"b", kindEventBus, 1, confMedium},
		{moduleScripts, kindAsyncTask, 1, confMedium},
	}
	if len(d.RuntimeAsync) != len(want) {
		t.Fatalf("runtime_async groups = %d, want %d (%+v)", len(d.RuntimeAsync), len(want), d.RuntimeAsync)
	}
	for i, w := range want {
		got := d.RuntimeAsync[i]
		if got.Module != w.module {
			t.Errorf("group[%d].Module = %q, want %q", i, got.Module, w.module)
		}
		if got.IntegrationKind != w.kind {
			t.Errorf("group[%d].IntegrationKind = %q, want %q", i, got.IntegrationKind, w.kind)
		}
		if got.Count != w.count {
			t.Errorf("group[%d].Count = %d, want %d", i, got.Count, w.count)
		}
		if got.Confidence != w.conf {
			t.Errorf("group[%d].Confidence = %q, want %q", i, got.Confidence, w.conf)
		}
	}

	wantEdges := []struct {
		from   string
		target string
		kind   string
		count  int
	}{
		{"a", libAmqp, kindMQ, 2},
		{"b", libKafka, kindEventBus, 1},
		{moduleScripts, libCelery, kindAsyncTask, 1},
	}
	if len(d.RuntimeAsyncEdges) != len(wantEdges) {
		t.Fatalf("runtime_async_edges = %d, want %d (%+v)", len(d.RuntimeAsyncEdges), len(wantEdges), d.RuntimeAsyncEdges)
	}
	for i, w := range wantEdges {
		got := d.RuntimeAsyncEdges[i]
		if got.FromModule != w.from || got.Target != w.target || got.IntegrationKind != w.kind || got.Count != w.count {
			t.Errorf("edge[%d] = %+v, want from=%q target=%q kind=%q count=%d", i, got, w.from, w.target, w.kind, w.count)
		}
		if got.Confidence != confMedium {
			t.Errorf("edge[%d].Confidence = %q, want %q", i, got.Confidence, confMedium)
		}
		if len(got.Sites) == 0 || len(got.Sites) > 5 {
			t.Errorf("edge[%d].Sites len = %d, want 1..5", i, len(got.Sites))
		}
	}
}

// TestRun_RuntimeAsync_EmptySites asserts that an empty site list produces no
// runtime_async block in the diagnostic (omitempty).
func TestRun_RuntimeAsync_EmptySites(t *testing.T) {
	d := runtimeAsyncRun(t, nil, "low")
	if len(d.RuntimeAsync) != 0 {
		t.Errorf("expected empty runtime_async, got %+v", d.RuntimeAsync)
	}
	if len(d.RuntimeAsyncEdges) != 0 {
		t.Errorf("expected empty runtime_async_edges, got %+v", d.RuntimeAsyncEdges)
	}
}

// TestRun_RuntimeAsync_StaticGraphUnchanged proves the runtime-async signal is
// purely additive: a run with sites and a run without them produce a byte-identical
// diagnostic once the RuntimeAsync block is nulled out. Verdict, metrics, findings
// must be unchanged — this is report-only signal.
func TestRun_RuntimeAsync_StaticGraphUnchanged(t *testing.T) {
	withSites := runtimeAsyncRun(t, []diagnostic.RuntimeAsyncSite{
		{File: pathFileA, Line: 1, Library: libAmqp, IntegrationKind: kindMQ, Language: "go"},
	}, confMedium)
	withoutSites := runtimeAsyncRun(t, nil, "low")

	if len(withSites.RuntimeAsync) == 0 {
		t.Fatal("expected runtime_async to be populated in the with-sites run")
	}
	if len(withSites.RuntimeAsyncEdges) == 0 {
		t.Fatal("expected runtime_async_edges to be populated in the with-sites run")
	}
	if len(withoutSites.RuntimeAsync) != 0 {
		t.Errorf("expected empty runtime_async in the no-sites run, got %+v", withoutSites.RuntimeAsync)
	}
	if len(withoutSites.RuntimeAsyncEdges) != 0 {
		t.Errorf("expected empty runtime_async_edges in the no-sites run, got %+v", withoutSites.RuntimeAsyncEdges)
	}

	withSites.RuntimeAsync = nil
	withSites.RuntimeAsyncEdges = nil
	withoutSites.RuntimeAsync = nil
	withoutSites.RuntimeAsyncEdges = nil
	a, err := json.Marshal(withSites)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	b, err := json.Marshal(withoutSites)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if !bytes.Equal(a, b) {
		t.Errorf("runtime-async signal altered the rest of the diagnostic:\n with: %s\n without: %s", a, b)
	}
}

func TestRun_ReportOnlyLocalAndRuntimeFactsDoNotChangeScoreOrVerdict(t *testing.T) {
	modules := map[string]config.ModuleDef{
		"a": {Paths: []string{"pkg/a/**"}, Owner: "team-a", Subdomain: subdomainCore},
		"b": {Paths: []string{"pkg/b/**"}, Owner: "team-b", Subdomain: subdomainCore},
	}
	cfg := config.Config{Version: 1, Modules: modules}
	cross := graph.Edge{
		From: "file:pkg/a/a.go", To: "file:pkg/b/b.go",
		Kind: graph.EdgeKindImports, Language: graph.LangGo,
		StrengthHint: string(coupling.StrengthFunctional),
	}
	local := graph.Edge{
		From: "file:pkg/a/local_a.go", To: "file:pkg/a/local_b.go",
		Kind: graph.EdgeKindImports, Language: graph.LangGo,
		StrengthHint: string(coupling.StrengthModel),
	}
	run := func(facts graph.Facts, signals signal.RunSignals) diagnostic.Diagnostic {
		t.Helper()
		ex := &ports.ExtractorMock{
			NameFunc: func() string { return "go" },
			ExtractFunc: func(_ context.Context, _ scope.Scope) (graph.Facts, diagnostic.Coverage, error) {
				return facts, diagnostic.Coverage{Tool: "go", Status: diagnostic.StatusOK}, nil
			},
		}
		base := baseline.Baseline{}
		d, err := engine.Run(context.Background(), engine.RunInput{
			Mode:        engine.Mode{Head: headRef, Advisory: true},
			Scope:       scope.Scope{Root: "."},
			Classify:    cfg.ForClassify(),
			Extractors:  []ports.Extractor{ex},
			Patterns:    ports.NopPatternProvider{},
			Resolver:    ports.NopSymbolResolver{},
			Rules:       nil,
			Metrics:     nil,
			Accepted:    base,
			BaseMetrics: base.Metrics,
			Signals:     signals,
			Now:         time.Date(2026, 7, 7, 0, 0, 0, 0, time.UTC),
		})
		if err != nil {
			t.Fatalf("Run: %v", err)
		}
		return d
	}

	base := run(graph.Facts{Language: graph.LangGo, Edges: []graph.Edge{cross}}, signal.RunSignals{})
	withReportOnly := run(
		graph.Facts{Language: graph.LangGo, Edges: []graph.Edge{cross, local}},
		signal.RunSignals{RuntimeAsync: signal.RuntimeAsyncSignals{
			Confidence: confMedium,
			Sites:      []diagnostic.RuntimeAsyncSite{{File: "pkg/a/a.go", Line: 12, Library: libAmqp, IntegrationKind: kindMQ, Language: "go"}},
		}},
	)

	if withReportOnly.ClassifiedEdges.SameModule == 0 {
		t.Fatal("expected same-module edge to be counted as report-only local coupling")
	}
	if len(withReportOnly.LocalCoupling) == 0 {
		t.Fatal("expected local_coupling report block")
	}
	if len(withReportOnly.RuntimeAsync) == 0 || len(withReportOnly.RuntimeAsyncEdges) == 0 {
		t.Fatal("expected runtime_async report blocks")
	}

	baseScore := archscore.Synthesize(base)
	reportOnlyScore := archscore.Synthesize(withReportOnly)
	if baseScore.Overall != reportOnlyScore.Overall || baseScore.OverallBand != reportOnlyScore.OverallBand {
		t.Errorf("report-only facts changed score: base=%+v with=%+v", baseScore, reportOnlyScore)
	}
	if base.Verdict != withReportOnly.Verdict {
		t.Errorf("report-only facts changed verdict: base=%q with=%q", base.Verdict, withReportOnly.Verdict)
	}
}

// TestRun_RuntimeAsync_Deterministic asserts a double run with identical sites
// yields byte-identical diagnostics (no map-order leakage in the rollup).
func TestRun_RuntimeAsync_Deterministic(t *testing.T) {
	sites := []diagnostic.RuntimeAsyncSite{
		{File: "pkg/b/b.go", Line: 1, Library: libKafka, IntegrationKind: kindEventBus, Language: "go"},
		{File: pathFileA, Line: 2, Library: libAmqp, IntegrationKind: kindMQ, Language: "go"},
		{File: "scripts/z.py", Line: 3, Library: libCelery, IntegrationKind: kindAsyncTask, Language: langPyTest},
	}
	firstDiag := runtimeAsyncRun(t, sites, confMedium)
	secondDiag := runtimeAsyncRun(t, sites, confMedium)
	first, err := json.Marshal(struct {
		Modules []diagnostic.RuntimeAsyncModule
		Edges   []diagnostic.RuntimeAsyncEdge
	}{firstDiag.RuntimeAsync, firstDiag.RuntimeAsyncEdges})
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	second, err := json.Marshal(struct {
		Modules []diagnostic.RuntimeAsyncModule
		Edges   []diagnostic.RuntimeAsyncEdge
	}{secondDiag.RuntimeAsync, secondDiag.RuntimeAsyncEdges})
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if !bytes.Equal(first, second) {
		t.Errorf("non-deterministic runtime_async:\n first:  %s\n second: %s", first, second)
	}
}

// bookExampleFacts builds Facts with a single import edge using the given StrengthHint.
// fromFile and toFile are repo-relative file paths; the edge is tagged EdgeKindImports.
func bookExampleFacts(fromFile, toFile, strengthHint string) graph.Facts {
	fromNode := "file:" + fromFile
	toNode := "file:" + toFile
	return graph.Facts{
		Language: "go",
		Nodes: []graph.Node{
			{Kind: graph.NodeKindFile, Path: fromFile},
			{Kind: graph.NodeKindFile, Path: toFile},
		},
		Edges: []graph.Edge{
			{
				From:         fromNode,
				To:           toNode,
				Kind:         graph.EdgeKindImports,
				Language:     "go",
				Confidence:   confidenceHigh,
				StrengthHint: strengthHint,
				Locations:    []graph.Location{{File: fromFile, Line: 1}},
			},
		},
	}
}

// bookExampleConfig builds a two-module ClassifyConfig for Ch10 engine tests.
//
// Module "src" owns fromFile paths, module "dst" owns toFile paths.
// sameDeployUnit controls whether cross_deploy_unit distance fires:
//
//	false → src gets deploy_unit="svc-src", dst gets deploy_unit="svc-dst" → D=9
//	true  → both get deploy_unit="svc-shared" → distance stays at cross_module_diff_owner or same_module
//
// sameOwner=true sets both modules to "team-shared" → cross_module_same_owner (D=4) when in the same deploy unit.
// targetSubdomain controls the dst module's domain volatility ("core"→V=10, "supporting"→V=3, "generic"→V=3).
func bookExampleConfig(fromGlob, toGlob string, sameDeployUnit, sameOwner bool, targetSubdomain string) (config.ClassifyConfig, []rules.Rule) {
	srcUnit := "svc-src"
	dstUnit := "svc-dst"
	if sameDeployUnit {
		srcUnit = "svc-shared"
		dstUnit = "svc-shared"
	}
	srcOwner := "team-src"
	dstOwner := "team-dst"
	if sameOwner {
		srcOwner = "team-shared"
		dstOwner = "team-shared"
	}

	// No Public/Internal globs: classifyStrength returns StrengthUnknown so the
	// edge's StrengthHint is used verbatim (last-resort fallback in classify.go).
	//
	// A third sentinel module with a distinct owner ("team-other") ensures the
	// owner map is never degenerate (≥2 distinct owners), which allows the explicit-
	// owner distance signal to fire for the src→dst edge. Without it, both modules
	// sharing one owner triggers degenerateExplicit suppression and falls through to
	// code-structure distance, which cannot produce cross_module_same_owner.
	modules := map[string]config.ModuleDef{
		"src": {
			Paths:      []string{fromGlob},
			Owner:      srcOwner,
			DeployUnit: srcUnit,
		},
		"dst": {
			Paths:      []string{toGlob},
			Owner:      dstOwner,
			DeployUnit: dstUnit,
			Subdomain:  targetSubdomain,
		},
		"sentinel": {
			Paths:      []string{"services/sentinel/**"},
			Owner:      "team-other",
			DeployUnit: srcUnit,
		},
	}

	cfg := config.Config{Version: 1, Modules: modules}
	rs, err := rules.New(cfg.ForRules())
	if err != nil {
		panic("ch10Config: " + err.Error())
	}
	return cfg.ForClassify(), rs
}

// TestRun_BookExamples_Ch10 is an engine-level regression test anchoring the full
// pipeline (facts → classify → score → advisory) to Khononov Ch10 book examples.
// It verifies that synthetic graphs with known StrengthHint / distance / volatility
// produce the balance values computed by the BookScorer formula:
//
//	modularity = |S - D|
//	balance    = max(modularity, 10 - V) + 1
//	same_module guard: if D == same_module → balance = 10 (no advisory)
func TestRun_BookExamples_Ch10(t *testing.T) {
	// File and glob constants local to this test suite to avoid collision with
	// the global cannedConfig globs (pkg/a/** / pkg/b/**).
	const (
		fromFile = "services/src/main.go"
		fromGlob = "services/src/**"
		toFile   = "services/dst/impl.go"
		toGlob   = "services/dst/**"
		toFileSM = "services/src/impl.go" // same module as fromFile for the same_module case
	)

	type tc struct {
		name            string
		strengthHint    string // StrengthHint on the edge
		sameDeployUnit  bool   // false → cross_deploy_unit (D=9), true → same-deploy-unit distance
		sameOwner       bool   // true → cross_module_same_owner (D=4); false → cross_module_diff_owner (D=7)
		targetSubdomain string // dst module subdomain ("core"→high V=10, "supporting"→low V=3, "generic"→low V=3)
		usesSameModule  bool   // true → fromFile and toFile are in the same module (same_module guard)
		wantBalance     int    // expected balance score_value
		wantBand        string // expected score_band
		wantAdvisory    bool   // false → no bc/imbalanced_coupling finding expected
		comment         string
	}

	cases := []tc{
		{
			// Ch10: "Distributed Monolith" — symmetric strength (S=9), cross-deploy distance (D=9),
			// core domain (V=10). mod=|9-9|=0, volR=10-10=0, balance=max(0,0)+1=1.
			name:            "distributed_monolith",
			strengthHint:    "symmetric",
			sameDeployUnit:  false,
			targetSubdomain: subdomainCore,
			wantBalance:     1,
			wantBand:        "critical",
			wantAdvisory:    true,
			comment:         "S=9 D=9 V=10 → mod=0 volR=0 balance=1 (critical)",
		},
		{
			// Ch10: "Ball of Mud" — model strength (S=3, low), cross_module_same_owner distance (D=4, low),
			// core domain (V=10, high). low+low → advisory (over-decoupled volatile seam).
			// mod=|3-4|=1, volR=10-10=0, balance=max(1,0)+1=2 (critical).
			// Note: "model over distance" (S=3 low, D=9 high) is XOR loose quadrant → SeverityNone by design.
			name:            "ball_of_mud",
			strengthHint:    "model",
			sameDeployUnit:  true,
			sameOwner:       true,
			targetSubdomain: subdomainCore,
			wantBalance:     2,
			wantBand:        "critical",
			wantAdvisory:    true,
			comment:         "S=3 D=4 V=10 → mod=1 volR=0 balance=2 (critical)",
		},
		{
			// Ch10: "Frozen Legacy" (approximation) — intrusive strength (S=10), cross-deploy distance (D=9),
			// generic domain (V=3, archfit minimum; book uses V=1 "frozen" which doesn't exist here).
			// mod=|10-9|=1, volR=10-3=7, balance=max(1,7)+1=8.
			// Book result with V=1: volR=9, balance=10 (none). This approximation gives balance=8 (low).
			name:            "frozen_legacy_approx",
			strengthHint:    "intrusive",
			sameDeployUnit:  false,
			targetSubdomain: "generic",
			wantBalance:     8,
			wantBand:        "low",
			wantAdvisory:    true,
			comment:         "S=10 D=9 V=3(low/approx) → mod=1 volR=7 balance=8 (low); book V=1→balance=10",
		},
		{
			// Ch10: "Transactional Cohesion" — functional strength (S=8), same_module (D=2, guard fires).
			// same_module guard → balance=10 → no advisory emitted.
			name:           "transactional_cohesion",
			strengthHint:   strengthFunctional,
			usesSameModule: true,
			wantBalance:    10,
			wantBand:       "none",
			wantAdvisory:   false,
			comment:        "S=8 D=same_module → guard → balance=10 (none), no advisory",
		},
	}

	ctx := context.Background()
	now := time.Date(2026, 6, 23, 0, 0, 0, 0, time.UTC)

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var facts graph.Facts
			var classifyCfg config.ClassifyConfig
			var rs []rules.Rule

			if tc.usesSameModule {
				// Both files in the same module (services/src/**) → same_module distance guard.
				facts = bookExampleFacts(fromFile, toFileSM, tc.strengthHint)
				classifyCfg, rs = bookExampleConfig(fromGlob, fromGlob, true, false, tc.targetSubdomain)
			} else {
				facts = bookExampleFacts(fromFile, toFile, tc.strengthHint)
				classifyCfg, rs = bookExampleConfig(fromGlob, toGlob, tc.sameDeployUnit, tc.sameOwner, tc.targetSubdomain)
			}

			ex := &ports.ExtractorMock{
				NameFunc: func() string { return "go" },
				ExtractFunc: func(_ context.Context, _ scope.Scope) (graph.Facts, diagnostic.Coverage, error) {
					return facts, diagnostic.Coverage{Tool: "go", Status: "ok"}, nil
				},
			}

			ms := metrics.New(config.Config{Version: 1})
			base := baseline.Baseline{}

			d, err := engine.Run(ctx, engine.RunInput{
				Mode:        engine.Mode{Head: headRef, Advisory: true},
				Scope:       scope.Scope{Root: "."},
				Classify:    classifyCfg,
				Staleness:   config.StalenessConfig{},
				Waivers:     config.WaiverSet{},
				Extractors:  []ports.Extractor{ex},
				Patterns:    ports.NopPatternProvider{},
				Resolver:    ports.NopSymbolResolver{},
				PatternCfg:  config.PatternConfig{},
				Rules:       rs,
				Metrics:     ms,
				Accepted:    base,
				BaseMetrics: base.Metrics,
				Labels:      nil,
				Signals:     signal.RunSignals{},
				Now:         now,
			})
			if err != nil {
				t.Fatalf("Run: %v", err)
			}

			// Find the bc/imbalanced_coupling advisory (there may be at most one for a single edge).
			var adv *finding.Finding
			for i := range d.Findings {
				if d.Findings[i].RuleID == engine.RuleIDBCImbalanced && d.Findings[i].Kind == kindAdvisory {
					adv = &d.Findings[i]
					break
				}
			}

			if !tc.wantAdvisory {
				if adv != nil {
					t.Errorf("%s: expected no bc/imbalanced_coupling advisory (same_module guard), got score_value=%s score_band=%s",
						tc.comment, adv.MatchedBy["score_value"], adv.MatchedBy["score_band"])
				}
				return
			}

			if adv == nil {
				t.Fatalf("%s: no bc/imbalanced_coupling advisory; findings=%+v", tc.comment, d.Findings)
			}

			rawValue, ok := adv.MatchedBy["score_value"]
			if !ok {
				t.Fatalf("%s: MatchedBy missing score_value; got %+v", tc.comment, adv.MatchedBy)
			}
			gotValue, err := strconv.Atoi(rawValue)
			if err != nil {
				t.Fatalf("%s: score_value %q not an integer: %v", tc.comment, rawValue, err)
			}
			if gotValue != tc.wantBalance {
				t.Errorf("%s: score_value=%d, want %d", tc.comment, gotValue, tc.wantBalance)
			}

			gotBand := adv.MatchedBy["score_band"]
			if gotBand != tc.wantBand {
				t.Errorf("%s: score_band=%q, want %q", tc.comment, gotBand, tc.wantBand)
			}
		})
	}
}

func TestRun_SyntaxFacts_Populated(t *testing.T) {
	ctx := context.Background()
	facts := cleanFacts()

	ex := &ports.ExtractorMock{
		NameFunc: func() string { return "go" },
		ExtractFunc: func(_ context.Context, _ scope.Scope) (graph.Facts, diagnostic.Coverage, error) {
			return facts, diagnostic.Coverage{Tool: "go", Status: "ok", FilesSeen: 2, FilesApplicable: 2}, nil
		},
	}

	synMock := &ports.SyntaxProviderMock{
		NameFunc: func() string { return toolNameAstgrep },
		SyntaxFunc: func(_ context.Context, _ scope.Scope, _ []string) ([]diagnostic.SyntaxFact, diagnostic.Coverage, error) {
			return []diagnostic.SyntaxFact{
				{File: pathFileA, Language: "go", Kind: kindFuncDecl, Name: "FooHandler", StartLine: 10},
			}, diagnostic.Coverage{Tool: toolNameAstgrep, Status: "ok", FilesSeen: 1, FilesApplicable: 1}, nil
		},
	}

	classifyCfg, rs := cannedConfig()
	ms := metrics.New(config.Config{Version: 1})
	base := baseline.Baseline{}
	now := time.Date(2026, 6, 24, 0, 0, 0, 0, time.UTC)

	d, err := engine.Run(ctx, engine.RunInput{
		Mode:        engine.Mode{Head: headRef},
		Scope:       scope.Scope{Root: "."},
		Classify:    classifyCfg,
		Staleness:   config.StalenessConfig{},
		Waivers:     config.WaiverSet{},
		Extractors:  []ports.Extractor{ex},
		Patterns:    ports.NopPatternProvider{},
		Resolver:    ports.NopSymbolResolver{},
		Syntax:      synMock,
		SyntaxCfg:   config.SyntaxConfig{Enabled: true, Languages: []string{"go"}},
		PatternCfg:  config.PatternConfig{},
		Rules:       rs,
		Metrics:     ms,
		Accepted:    base,
		BaseMetrics: base.Metrics,
		Labels:      nil,
		Signals:     signal.RunSignals{},
		Now:         now,
	})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}

	if len(d.SyntaxFacts) == 0 {
		t.Fatal("SyntaxFacts should be populated when syntax is enabled")
	}
	// Coverage must include the syntax coverage entry.
	var foundSynCov bool
	for _, c := range d.ToolCoverage {
		if c.Tool == "ast-grep" && c.FilesSeen == 1 {
			foundSynCov = true
			break
		}
	}
	if !foundSynCov {
		t.Error("syntax coverage not found in ToolCoverage")
	}
	// Facts are off-gate — verdict stays pass.
	if d.Verdict != diagnostic.VerdictPass {
		t.Errorf("verdict = %v, want pass", d.Verdict)
	}
	if len(synMock.SyntaxCalls()) == 0 {
		t.Error("SyntaxProvider.Syntax was never called")
	}
}

// TestRun_SyntaxFacts_ModuleBackfill asserts that after syntax facts are produced
// the engine populates SyntaxFact.Module from the ModuleMap for files that match
// a declared module, and leaves Module empty for files outside declared modules.
// Uses index-based range to avoid the range-copy trap.
func TestRun_SyntaxFacts_ModuleBackfill(t *testing.T) {
	ctx := context.Background()
	facts := cleanFacts()

	ex := &ports.ExtractorMock{
		NameFunc: func() string { return "go" },
		ExtractFunc: func(_ context.Context, _ scope.Scope) (graph.Facts, diagnostic.Coverage, error) {
			return facts, diagnostic.Coverage{Tool: "go", Status: "ok"}, nil
		},
	}

	// pathFileA = "pkg/a/a.go" is in module "a" per cannedConfig.
	// outsideFile is not covered by any module glob.
	const outsideFile = "scripts/gen.go"
	synMock := &ports.SyntaxProviderMock{
		NameFunc: func() string { return toolNameAstgrep },
		SyntaxFunc: func(_ context.Context, _ scope.Scope, _ []string) ([]diagnostic.SyntaxFact, diagnostic.Coverage, error) {
			return []diagnostic.SyntaxFact{
				{File: pathFileA, Language: "go", Kind: kindFuncDecl, Name: "Handler", StartLine: 1},
				{File: outsideFile, Language: "go", Kind: kindFuncDecl, Name: "Gen", StartLine: 1},
			}, diagnostic.Coverage{Tool: toolNameAstgrep, Status: "ok"}, nil
		},
	}

	classifyCfg, rs := cannedConfig()
	ms := metrics.New(config.Config{Version: 1})
	base := baseline.Baseline{}
	now := time.Date(2026, 6, 25, 0, 0, 0, 0, time.UTC)

	d, err := engine.Run(ctx, engine.RunInput{
		Mode:        engine.Mode{Head: headRef},
		Scope:       scope.Scope{Root: "."},
		Classify:    classifyCfg,
		Staleness:   config.StalenessConfig{},
		Waivers:     config.WaiverSet{},
		Extractors:  []ports.Extractor{ex},
		Patterns:    ports.NopPatternProvider{},
		Resolver:    ports.NopSymbolResolver{},
		Syntax:      synMock,
		SyntaxCfg:   config.SyntaxConfig{Enabled: true, Languages: []string{"go"}},
		PatternCfg:  config.PatternConfig{},
		Rules:       rs,
		Metrics:     ms,
		Accepted:    base,
		BaseMetrics: base.Metrics,
		Labels:      nil,
		Signals:     signal.RunSignals{},
		Now:         now,
	})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}

	if len(d.SyntaxFacts) != 2 {
		t.Fatalf("SyntaxFacts len = %d; want 2", len(d.SyntaxFacts))
	}

	// Find facts by file — order may vary after DeriveRoles+Sort.
	modByFile := make(map[string]string, len(d.SyntaxFacts))
	for _, sf := range d.SyntaxFacts {
		modByFile[sf.File] = sf.Module
	}

	if got := modByFile[pathFileA]; got != "a" {
		t.Errorf("SyntaxFact[%s].Module = %q; want %q (in-module backfill)", pathFileA, got, "a")
	}
	if got := modByFile[outsideFile]; got != "" {
		t.Errorf("SyntaxFact[%s].Module = %q; want empty (outside declared modules)", outsideFile, got)
	}
}

func TestRun_SyntaxEnabled_NilProvider_ReturnsError(t *testing.T) {
	ctx := context.Background()
	facts := cleanFacts()
	ex := &ports.ExtractorMock{
		NameFunc: func() string { return "go" },
		ExtractFunc: func(_ context.Context, _ scope.Scope) (graph.Facts, diagnostic.Coverage, error) {
			return facts, diagnostic.Coverage{Tool: "go", Status: "ok"}, nil
		},
	}
	classifyCfg, rs := cannedConfig()
	ms := metrics.New(config.Config{Version: 1})
	base := baseline.Baseline{}
	now := time.Date(2026, 6, 24, 0, 0, 0, 0, time.UTC)

	_, err := engine.Run(ctx, engine.RunInput{
		Mode:       engine.Mode{Head: headRef},
		Scope:      scope.Scope{Root: "."},
		Classify:   classifyCfg,
		Extractors: []ports.Extractor{ex},
		Patterns:   ports.NopPatternProvider{},
		Resolver:   ports.NopSymbolResolver{},
		Syntax:     nil, // deliberately nil
		SyntaxCfg:  config.SyntaxConfig{Enabled: true, Languages: []string{"go"}},
		Rules:      rs,
		Metrics:    ms,
		Accepted:   base,
		Now:        now,
	})
	if err == nil {
		t.Fatal("want error when Enabled=true and Syntax=nil, got nil")
	}
}

func TestRun_SyntaxFacts_DisabledNoCallNoCoverage(t *testing.T) {
	ctx := context.Background()
	facts := cleanFacts()

	ex := &ports.ExtractorMock{
		NameFunc: func() string { return "go" },
		ExtractFunc: func(_ context.Context, _ scope.Scope) (graph.Facts, diagnostic.Coverage, error) {
			return facts, diagnostic.Coverage{Tool: "go", Status: "ok", FilesSeen: 2, FilesApplicable: 2}, nil
		},
	}

	synMock := &ports.SyntaxProviderMock{
		NameFunc: func() string { return "ast-grep" },
		SyntaxFunc: func(_ context.Context, _ scope.Scope, _ []string) ([]diagnostic.SyntaxFact, diagnostic.Coverage, error) {
			return nil, diagnostic.Coverage{}, nil
		},
	}

	classifyCfg, rs := cannedConfig()
	ms := metrics.New(config.Config{Version: 1})
	base := baseline.Baseline{}
	now := time.Date(2026, 6, 24, 0, 0, 0, 0, time.UTC)

	d, err := engine.Run(ctx, engine.RunInput{
		Mode:        engine.Mode{Head: headRef},
		Scope:       scope.Scope{Root: "."},
		Classify:    classifyCfg,
		Staleness:   config.StalenessConfig{},
		Waivers:     config.WaiverSet{},
		Extractors:  []ports.Extractor{ex},
		Patterns:    ports.NopPatternProvider{},
		Resolver:    ports.NopSymbolResolver{},
		Syntax:      synMock,
		SyntaxCfg:   config.SyntaxConfig{Enabled: false},
		PatternCfg:  config.PatternConfig{},
		Rules:       rs,
		Metrics:     ms,
		Accepted:    base,
		BaseMetrics: base.Metrics,
		Labels:      nil,
		Signals:     signal.RunSignals{},
		Now:         now,
	})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}

	if len(d.SyntaxFacts) != 0 {
		t.Errorf("SyntaxFacts should be empty when syntax disabled, got %d", len(d.SyntaxFacts))
	}
	if len(synMock.SyntaxCalls()) != 0 {
		t.Error("SyntaxProvider should not be called when disabled")
	}
	// NopPatternProvider contributes exactly one ast-grep absent entry.
	// The syntax path must not add a second one when disabled.
	var astGrepCount int
	for _, cov := range d.ToolCoverage {
		if cov.Tool == toolNameAstgrep {
			astGrepCount++
		}
	}
	if astGrepCount != 1 {
		t.Errorf("want exactly 1 ast-grep coverage entry (from NopPatternProvider), got %d; ToolCoverage=%+v", astGrepCount, d.ToolCoverage)
	}
}

// TestRun_WarnRule_ProducesVerdictWarn verifies that a rule with gate:warn
// produces VerdictWarn (not VerdictFail), zero gate findings, and — when
// advisory mode is on — exactly one advisory finding (not two, which would
// indicate double-emission from the stage-8 partition bug).
func TestRun_WarnRule_ProducesVerdictWarn(t *testing.T) {
	ctx := context.Background()

	const warnRuleID = "warn-no-internal"

	cfg := config.Config{
		Version: 1,
		Modules: map[string]config.ModuleDef{
			"a": {Paths: []string{globModuleA}},
			"b": {Paths: []string{globModuleB}},
		},
		Rules: []config.RuleDef{
			{
				ID:   warnRuleID,
				Type: rulePublicAPIOnly,
				Gate: "warn",
			},
		},
	}

	rs, err := rules.New(cfg.ForRules())
	if err != nil {
		t.Fatalf("rules.New: %v", err)
	}

	makeEx := func() ports.Extractor {
		return &ports.ExtractorMock{
			NameFunc: func() string { return "go" },
			ExtractFunc: func(_ context.Context, _ scope.Scope) (graph.Facts, diagnostic.Coverage, error) {
				return violationFacts(), diagnostic.Coverage{Tool: "go", Status: "ok", FilesSeen: 2, FilesApplicable: 2}, nil
			},
		}
	}

	ms := metrics.New(config.Config{Version: 1})
	base := baseline.Baseline{}
	now := time.Date(2026, 6, 24, 0, 0, 0, 0, time.UTC)

	runWith := func(advisory bool) diagnostic.Diagnostic {
		t.Helper()
		d, runErr := engine.Run(ctx, engine.RunInput{
			Mode:        engine.Mode{Head: headRef, Advisory: advisory},
			Scope:       scope.Scope{Root: "."},
			Classify:    cfg.ForClassify(),
			Staleness:   config.StalenessConfig{},
			Waivers:     config.WaiverSet{},
			Extractors:  []ports.Extractor{makeEx()},
			Patterns:    ports.NopPatternProvider{},
			Resolver:    ports.NopSymbolResolver{},
			PatternCfg:  config.PatternConfig{},
			Rules:       rs,
			Metrics:     ms,
			Accepted:    base,
			BaseMetrics: base.Metrics,
			Labels:      nil,
			Signals:     signal.RunSignals{},
			Now:         now,
		})
		if runErr != nil {
			t.Fatalf("Run: %v", runErr)
		}
		return d
	}

	// Advisory mode OFF: verdict must be warn; no gate findings.
	d := runWith(false)

	if d.Verdict != diagnostic.VerdictWarn {
		t.Errorf("verdict=%q, want %q", d.Verdict, diagnostic.VerdictWarn)
	}
	for _, f := range d.Findings {
		if f.Kind == kindGate && (f.Status == finding.StatusNew || f.Status == finding.StatusExpiredWaiver) {
			t.Errorf("unexpected gate finding for warn rule: %+v", f)
		}
	}
	if d.Summary.GateFindings != 0 {
		t.Errorf("summary.gate_findings=%d, want 0 for warn rule", d.Summary.GateFindings)
	}

	// Advisory mode ON: the warn finding must appear exactly once with Kind=advisory.
	// Two occurrences would indicate double-emission from a missing stage-8 partition.
	dAdv := runWith(true)

	var warnCount int
	for _, f := range dAdv.Findings {
		if f.RuleID == warnRuleID {
			warnCount++
			if f.Kind != kindAdvisory {
				t.Errorf("warn finding Kind=%q, want %q", f.Kind, kindAdvisory)
			}
		}
	}
	if warnCount != 1 {
		t.Fatalf("got %d warn findings, want exactly 1 (double-emission guard)", warnCount)
	}
	if dAdv.Summary.Warnings != 1 {
		t.Errorf("summary.warnings=%d, want 1", dAdv.Summary.Warnings)
	}
}
