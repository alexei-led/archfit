package engine_test

import (
	"bytes"
	"context"
	"encoding/json"
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

	headRef         = "HEAD"
	kindAdvisory    = "advisory"
	kindGate        = "gate"
	gateFail        = "fail"
	toolNameAstgrep = "ast-grep"
	toolNameScip    = "scip"

	// confidenceHigh is the string literal used in Facts and symbol graph tests.
	confidenceHigh = "high"

	// symbolFoo is the test symbol used in SymbolGraph forwarding tests.
	symbolFoo     = "pkg/a.Foo"
	symbolBar     = "pkg/b.Bar"
	pathFileB     = "pkg/b/b.go"
	strengthModel = "model"
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
				Confidence: "high",
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
				StrengthHint: "functional",
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
		Exceptions:  config.ExceptionSet{},
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
		Exceptions:  config.ExceptionSet{},
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
		if f.Kind == kindGate && (f.Status == finding.StatusNew || f.Status == finding.StatusExpiredExcept) {
			t.Errorf("unexpected new gate finding: %+v", f)
		}
	}

	if d.Summary.GateFindings != 0 {
		t.Errorf("summary.gate_findings=%d, want 0", d.Summary.GateFindings)
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
		Exceptions:  config.ExceptionSet{},
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
	// Metrics should contain all registered metrics.
	if len(d.Metrics) != 19 {
		t.Errorf("len(metrics)=%d, want 19", len(d.Metrics))
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
		Exceptions:  config.ExceptionSet{},
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
		Exceptions:  config.ExceptionSet{},
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
		Exceptions:  config.ExceptionSet{},
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
		if d.Findings[i].RuleID == "bc/imbalanced_coupling" {
			adv = &d.Findings[i]
			break
		}
	}
	if adv == nil {
		t.Fatalf("no bc/imbalanced_coupling advisory present; findings=%+v", d.Findings)
	}

	// score = scorer name (default is the multiplicative scorer).
	if got := adv.MatchedBy["score"]; got != "multiplicative" {
		t.Errorf("MatchedBy[score]=%q, want scorer name %q", got, "multiplicative")
	}

	// score_value must parse to an int in [0,10].
	rawValue, ok := adv.MatchedBy["score_value"]
	if !ok {
		t.Fatalf("MatchedBy missing score_value; got %+v", adv.MatchedBy)
	}
	value, err := strconv.Atoi(rawValue)
	if err != nil {
		t.Fatalf("score_value %q not an integer: %v", rawValue, err)
	}
	if value < 0 || value > 10 {
		t.Errorf("score_value=%d out of range [0,10]", value)
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
			StrengthHint: "functional",
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
			Exceptions:  config.ExceptionSet{},
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
		if f.RuleID == "bc/imbalanced_coupling" {
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
		Exceptions:  config.ExceptionSet{},
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
		Exceptions:  config.ExceptionSet{},
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
		Exceptions:  config.ExceptionSet{},
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
		Exceptions:  config.ExceptionSet{},
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
		Exceptions:  config.ExceptionSet{},
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
		Size:    signal.SizeSignals{FileLOC: map[string]int{pathFileA: 100, pathFileB: 40}},
		History: signal.HistorySignals{CoChange: map[[2]string]int{{pathFileA, pathFileB}: 5}},
	}

	d, err := engine.Run(ctx, engine.RunInput{
		Mode:        engine.Mode{Head: headRef},
		Scope:       scope.Scope{Root: "."},
		Classify:    classifyCfg,
		Staleness:   config.StalenessConfig{},
		Exceptions:  config.ExceptionSet{},
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
	if len(a.CoChangePartners) != 1 || a.CoChangePartners[0] != pathFileB {
		t.Errorf("facts[0].CoChangePartners = %v, want [pkg/b/b.go]", a.CoChangePartners)
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
		Exceptions:  config.ExceptionSet{},
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
	rs := rules.New(cfg.ForRules())

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
			Exceptions:  config.ExceptionSet{},
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

	run := func(lbls []labels.Label) (diagnostic.Diagnostic, signal.CollectedSignals) {
		t.Helper()
		var captured signal.CollectedSignals
		spy := &spyMetric{captured: &captured}
		d, err := engine.Run(ctx, engine.RunInput{
			Mode:        engine.Mode{Head: headRef, Advisory: true},
			Scope:       scope.Scope{Root: "."},
			Classify:    cfg.ForClassify(),
			Staleness:   config.StalenessConfig{},
			Exceptions:  config.ExceptionSet{},
			Extractors:  []ports.Extractor{ex},
			Patterns:    ports.NopPatternProvider{},
			Resolver:    ports.NopSymbolResolver{},
			PatternCfg:  config.PatternConfig{},
			Rules:       rules.New(cfg.ForRules()),
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
			if f.RuleID == "labels/stale" {
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
			if f.RuleID == "labels/stale" && f.Edge.From.Module == "a" && f.Edge.To.Module == "b" {
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

// langPyTest / kindLazy are factored out so the dynamic-import test stays under
// goconst's repeated-literal threshold.
const (
	langPyTest = "python"
	kindLazy   = "lazy_import"
	pathPyA    = "pkg/a/x.py"
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
		Classify: classifyCfg, Staleness: config.StalenessConfig{}, Exceptions: config.ExceptionSet{},
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
		{"scripts", 1, 1},
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
