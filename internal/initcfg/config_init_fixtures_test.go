package initcfg_test

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/alexei-led/archfit/v3/internal/config"
	evidenceports "github.com/alexei-led/archfit/v3/internal/evidence/ports"
	"github.com/alexei-led/archfit/v3/internal/initcfg"
	"github.com/alexei-led/archfit/v3/internal/toolrun"
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
			discovered, err := initcfg.Discover(context.Background(), root, tt.runner, initcfg.ProbePresence(root))
			if err != nil {
				t.Fatalf("Discover: %v", err)
			}

			rendered := initcfg.Render(discovered, nil, false)
			cfg := loadRendered(t, rendered)
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
