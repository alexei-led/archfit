package initcfg_test

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	"github.com/alexei-led/archfit/internal/baseline"
	"github.com/alexei-led/archfit/internal/config"
	"github.com/alexei-led/archfit/internal/engine"
	goextract "github.com/alexei-led/archfit/internal/extract/golang"
	"github.com/alexei-led/archfit/internal/initcfg"
	"github.com/alexei-led/archfit/internal/metrics"
	"github.com/alexei-led/archfit/internal/model/signal"
	"github.com/alexei-led/archfit/internal/ports"
	"github.com/alexei-led/archfit/internal/rules"
	"github.com/alexei-led/archfit/internal/scope"
	"github.com/alexei-led/archfit/internal/toolrun"
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
		{name: "go", root: "gofixture", runner: toolrun.New()},
		{name: "typescript", root: "tsfixture", runner: &toolrun.RunnerMock{}},
		{name: "python", root: "pyfixture", runner: &toolrun.RunnerMock{}},
		{name: "rust", root: "rustfixture", runner: rustFixtureRunner()},
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
		})
	}
}

// TestConfigInit_GoFixture_LayerRuleBackEdge runs the full analyze gate over
// the Go fixture with the config `config init` generates for it. The fixture
// has a genuine layer back-edge (internal/model imports internal/engine —
// the innermost layer reaching into the outermost one).
//
// TODO(wave2): this asserts TODAY's behavior — the generated rule can never
// fire (V4 in reports/eval-2026-07-02-v1.1.2/00-FINDINGS.md: Render emits
// `type: forbidden_dependency` with `from_layer`/`to_layer`, but
// forbiddenDependency.Check reads `From`/`To`, which are empty). Task 2
// switches Render to emit `type: forbidden_layer_direction`; once that lands,
// flip the assertion below to expect exactly one finding for the back-edge.
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

	classifyCfg := cfg.ForClassify()
	rs, err := rules.New(cfg.ForRules())
	if err != nil {
		t.Fatalf("rules.New: %v", err)
	}
	ms := metrics.New(cfg)

	extractor := goextract.New(config.ExtractConfig{})
	base := baseline.Baseline{SchemaVersion: baseline.SchemaVersion}
	now := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	s := scope.Scope{Root: root, Mode: scope.ModeFull}

	diag, err := engine.Run(context.Background(), engine.RunInput{
		Mode:        engine.Mode{Full: true},
		Scope:       s,
		Classify:    classifyCfg,
		Staleness:   config.StalenessConfig{},
		Waivers:     config.WaiverSet{},
		Extractors:  []ports.Extractor{extractor},
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
		t.Fatalf("engine.Run: %v", err)
	}

	// TODO(wave2): should be 1 (the back-edge) once Task 2 lands — see doc
	// comment above.
	if got := len(diag.Findings); got != 0 {
		t.Errorf("layer back-edge findings = %d, want 0 (today's V4 bug — generated rule cannot fire); flip this once Task 2 lands: %+v", got, diag.Findings)
	}
}
