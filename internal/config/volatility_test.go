package config_test

import (
	"testing"

	"github.com/alexei-led/archfit/internal/config"
)

const (
	volHigh = "high"
	volLow  = "low"

	modAuto     = "auto"
	modExplicit = "explicit"
	modSubdom   = "subdom"
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
		modAuto:     {Paths: []string{"a/**"}},
		modExplicit: {Paths: []string{"b/**"}, Volatility: volLow},
		modSubdom:   {Paths: []string{"c/**"}, Subdomain: "core"},
	}}
	cfg.ApplyVolatility(map[string]string{modAuto: volHigh, modExplicit: volHigh, modSubdom: volHigh})

	// Modules always stays hand-authored-only: derived values must never
	// land on the ModuleDef (risk_hub and enrich read it directly).
	for name, wantPristine := range map[string]string{modAuto: "", modExplicit: volLow, modSubdom: ""} {
		if got := cfg.Modules[name].Volatility; got != wantPristine {
			t.Errorf("Modules[%q].Volatility = %q, want pristine %q", name, got, wantPristine)
		}
	}

	// The classify view sees the effective values: derived fills the gap,
	// explicit volatility and subdomain-declared modules stay untouched.
	effective := cfg.ForClassify().Modules
	for name, want := range map[string]string{modAuto: volHigh, modExplicit: volLow, modSubdom: ""} {
		if got := effective[name].Volatility; got != want {
			t.Errorf("ForClassify().Modules[%q].Volatility = %q, want %q", name, got, want)
		}
	}
}
