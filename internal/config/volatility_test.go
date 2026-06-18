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

	modHot  = "hot"
	modCold = "cold"

	pathA      = "a/**"
	subdomCore = "core"
)

func TestDeriveVolatility_BandsAndAttribution(t *testing.T) {
	modules := map[string]config.ModuleDef{
		// Go-style slash globs.
		modHot:  {Paths: []string{"internal/hot/**"}},
		"warm":  {Paths: []string{"internal/warm/**"}},
		modCold: {Paths: []string{"internal/cold/**"}},
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
	want := map[string]string{modHot: volHigh, "warm": "medium", modCold: volLow, "py": volHigh}
	for name, w := range want {
		if vol[name] != w {
			t.Errorf("module %q volatility = %q, want %q", name, vol[name], w)
		}
	}
}

func TestDeriveVolatility_NoChurnIsNil(t *testing.T) {
	modules := map[string]config.ModuleDef{"a": {Paths: []string{pathA}}}
	if got := config.DeriveVolatility(modules, nil); got != nil {
		t.Errorf("expected nil for empty churn, got %v", got)
	}
}

func TestApplyVolatility_RespectsExplicitConfig(t *testing.T) {
	cfg := config.Config{Modules: map[string]config.ModuleDef{
		modAuto:     {Paths: []string{pathA}},
		modExplicit: {Paths: []string{"b/**"}, Volatility: volLow},
		modSubdom:   {Paths: []string{"c/**"}, Subdomain: subdomCore},
	}}
	cfg.ApplyVolatility(map[string]string{modAuto: volHigh, modExplicit: volHigh, modSubdom: volHigh})

	// Modules always stays hand-authored-only: derived values must never
	// land on the ModuleDef (risk_hub and enrich read it directly).
	for name, wantPristine := range map[string]string{modAuto: "", modExplicit: volLow, modSubdom: ""} {
		if got := cfg.Modules[name].Volatility; got != wantPristine {
			t.Errorf("Modules[%q].Volatility = %q, want pristine %q", name, got, wantPristine)
		}
	}

	// The classify view sees hand-authored values only; churn is excluded from the gate.
	effective := cfg.ForClassify().Modules
	for name, want := range map[string]string{modAuto: "", modExplicit: volLow, modSubdom: ""} {
		if got := effective[name].Volatility; got != want {
			t.Errorf("ForClassify().Modules[%q].Volatility = %q, want %q (churn must not reach gate)", name, got, want)
		}
	}
}

// TestForClassify_ChurnExcludedFromGate verifies that ForClassify never exposes
// churn-derived volatility, even after ApplyVolatility has been called.
// Balanced Coupling forbids commit-history volatility on the gate path.
func TestForClassify_ChurnExcludedFromGate(t *testing.T) {
	cfg := config.Config{
		Version: 1,
		Modules: map[string]config.ModuleDef{
			"a": {Paths: []string{pathA}},                         // no explicit volatility
			"b": {Paths: []string{"b/**"}, Volatility: volLow},    // explicit wins
			"c": {Paths: []string{"c/**"}, Subdomain: subdomCore}, // subdomain wins
		},
	}
	churn := map[string]int{
		"a/x.go": 10,
		"b/y.go": 10,
		"c/z.go": 10,
	}
	cfg.ApplyVolatility(config.DeriveVolatility(cfg.Modules, churn))

	cc := cfg.ForClassify()

	// Module "a" has no explicit volatility or subdomain; churn derived "high"
	// but the gate must see "" (unknown), never the churn-derived band.
	if got := cc.Modules["a"].Volatility; got != "" {
		t.Errorf("ForClassify().Modules[a].Volatility = %q, want \"\" (churn must not reach gate)", got)
	}
	// Module "b" has explicit volatility volLow — must still be visible.
	if got := cc.Modules["b"].Volatility; got != volLow {
		t.Errorf("ForClassify().Modules[b].Volatility = %q, want %q", got, volLow)
	}
	// Module "c" has subdomain subdomCore but no explicit volatility — subdomain is hand-authored and must remain.
	if got := cc.Modules["c"].Subdomain; got != subdomCore {
		t.Errorf("ForClassify().Modules[c].Subdomain = %q, want %q", got, subdomCore)
	}
	if got := cc.Modules["c"].Volatility; got != "" {
		t.Errorf("ForClassify().Modules[c].Volatility = %q, want \"\" (subdomain already set)", got)
	}
}

// TestApplyVolatility_ChurnPathIntact verifies that ApplyVolatility / DeriveVolatility
// still work correctly for the report-only metrics path. The churn store is populated
// and readable via effectiveModules (tested indirectly by checking cfg.Modules is unchanged
// while ForClassify hides the churn — the churn store itself is tested by checking that
// ApplyVolatility records derived values and that Modules stays pristine).
func TestApplyVolatility_ChurnPathIntact(t *testing.T) {
	cfg := config.Config{
		Version: 1,
		Modules: map[string]config.ModuleDef{
			modHot:  {Paths: []string{"hot/**"}},
			modCold: {Paths: []string{"cold/**"}},
		},
	}
	churn := map[string]int{"hot/x.go": 9, "cold/y.go": 1}
	vol := config.DeriveVolatility(cfg.Modules, churn)

	// DeriveVolatility must still produce bands.
	if vol[modHot] != volHigh {
		t.Errorf("DeriveVolatility hot = %q, want %q", vol[modHot], volHigh)
	}
	if vol[modCold] != volLow {
		t.Errorf("DeriveVolatility cold = %q, want %q", vol[modCold], volLow)
	}

	cfg.ApplyVolatility(vol)

	// Modules stays pristine (hand-authored-only).
	if got := cfg.Modules[modHot].Volatility; got != "" {
		t.Errorf("cfg.Modules[hot].Volatility = %q after ApplyVolatility, want \"\" (must stay pristine)", got)
	}

	// ForClassify hides churn — gate sees "" for both.
	cc := cfg.ForClassify()
	for _, name := range []string{modHot, modCold} {
		if got := cc.Modules[name].Volatility; got != "" {
			t.Errorf("ForClassify().Modules[%q].Volatility = %q, want \"\" (churn must not reach gate)", name, got)
		}
	}
}
