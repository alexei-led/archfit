package config_test

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/alexei-led/archfit/internal/config"
)

// TestLoad_ExampleTemplates verifies that every starter template shipped in
// examples/ parses and validates cleanly. These templates are documentation we
// hand to users as a copy-paste starting point, so a malformed or
// schema-violating one is a real defect: it would fail on the user's first run.
// Loading also exercises enum validation (Task 7), so a bad gate/severity value
// in a template is caught here.
func TestLoad_ExampleTemplates(t *testing.T) {
	// cwd is internal/config; repo root is two levels up.
	names := []string{
		"go-monolith",
		"python-package",
		"ts-monorepo",
		"ddd",
		"microservices",
	}
	for _, name := range names {
		t.Run(name, func(t *testing.T) {
			path := filepath.Join("..", "..", "examples", name+".archfit.yaml")
			cfg, err := config.Load(context.Background(), path)
			if err != nil {
				t.Fatalf("Load(%s): %v", path, err)
			}
			if len(cfg.Modules) == 0 {
				t.Errorf("%s: no modules parsed", name)
			}
			if len(cfg.Rules) == 0 {
				t.Errorf("%s: no rules parsed", name)
			}
			// Templates are fully specified by design: every module declares an
			// owner and a subdomain (or explicit volatility) so distance and
			// volatility classify cleanly and Config.Lint() stays quiet.
			if warns := cfg.Lint(); len(warns) != 0 {
				t.Errorf("%s: Lint() returned %d warnings, want 0: %v", name, len(warns), warns)
			}
		})
	}
}
