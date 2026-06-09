package config_test

import (
	"testing"

	"github.com/alexei-led/archfit/internal/config"
)

const (
	volHigh = "high"
	volLow  = "low"
)

func TestDeriveVolatility_BandsAndAttribution(t *testing.T) {
	modules := map[string]config.ModuleDef{
		// Go-style slash globs.
		"hot":  {Paths: []string{"internal/hot/**"}},
		"warm": {Paths: []string{"internal/warm/**"}},
		"cold": {Paths: []string{"internal/cold/**"}},
		// Python-style dotted globs (matched via file→module conversion).
		"py": {Paths: []string{"app.svc", "app.svc.*"}},
	}
	churn := map[string]int{
		"internal/hot/a.go":  9, // hot total 9 (max)
		"internal/warm/b.go": 3, // warm total 3 (1/3 of max → medium)
		"internal/cold/c.go": 1, // cold total 1 (< 1/3 → low)
		"src/app/svc/x.py":   8, // attributed to "py" via dotted form app.svc.x
	}

	vol := config.DeriveVolatility(modules, churn)
	want := map[string]string{"hot": volHigh, "warm": "medium", "cold": volLow, "py": volHigh}
	for name, w := range want {
		if vol[name] != w {
			t.Errorf("module %q volatility = %q, want %q", name, vol[name], w)
		}
	}
}

func TestDeriveVolatility_NoChurnIsNil(t *testing.T) {
	modules := map[string]config.ModuleDef{"a": {Paths: []string{"a/**"}}}
	if got := config.DeriveVolatility(modules, nil); got != nil {
		t.Errorf("expected nil for empty churn, got %v", got)
	}
}

func TestApplyVolatility_RespectsExplicitConfig(t *testing.T) {
	cfg := config.Config{Modules: map[string]config.ModuleDef{
		"auto":     {Paths: []string{"a/**"}},
		"explicit": {Paths: []string{"b/**"}, Volatility: volLow},
		"subdom":   {Paths: []string{"c/**"}, Subdomain: "core"},
	}}
	cfg.ApplyVolatility(map[string]string{"auto": volHigh, "explicit": volHigh, "subdom": volHigh})

	if cfg.Modules["auto"].Volatility != volHigh {
		t.Errorf("auto should get derived volatility, got %q", cfg.Modules["auto"].Volatility)
	}
	if cfg.Modules["explicit"].Volatility != volLow {
		t.Errorf("explicit volatility must be preserved, got %q", cfg.Modules["explicit"].Volatility)
	}
	if cfg.Modules["subdom"].Volatility != "" {
		t.Errorf("subdomain-declared module must not be overridden, got %q", cfg.Modules["subdom"].Volatility)
	}
}
