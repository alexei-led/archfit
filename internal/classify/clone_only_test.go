package classify_test

import (
	"testing"

	"github.com/alexei-led/archfit/internal/classify"
	"github.com/alexei-led/archfit/internal/config"
	"github.com/alexei-led/archfit/internal/model/coupling"
	"github.com/alexei-led/archfit/internal/model/graph"
)

// Clone evidence file constants for the modA/modB pair, plus config volatility
// literals shared across classify test files (goconst).
const (
	cloneFileA = "services/a/dup.go"
	cloneFileB = "services/b/dup.go"
	cfgVolLow  = "low"
)

// cloneOnlyCfg returns the canonical two-module config with a modA/modB clone
// pair plus evidence, optionally mutated per test case.
func cloneOnlyCfg(mutate func(*config.ClassifyConfig)) config.ClassifyConfig {
	cfg := twoModuleConfig(nil, modABClonePair)
	cfg.CloneEvidence = map[string][]graph.Location{
		modABKey: {
			{File: cloneFileA, Line: 10},
			{File: cloneFileB, Line: 20},
		},
	}
	if mutate != nil {
		mutate(&cfg)
	}
	return cfg
}

// setModule mutates one module def in place (map values are structs).
func setModule(c *config.ClassifyConfig, name string, mutate func(*config.ModuleDef)) {
	def := c.Modules[name]
	mutate(&def)
	c.Modules[name] = def
}

// TestCloneOnlyPairs covers when a duplicated-knowledge pair is (not) detected:
// only a cross-module clone pair with NO import edge between the modules and no
// human-approved label for the pair qualifies.
func TestCloneOnlyPairs(t *testing.T) {
	t.Parallel()

	emptyGraph := graph.Build(nil)
	edgeGraph := graph.Build([]graph.Facts{{
		Edges:    []graph.Edge{{From: fileFromA, To: fileToB, Kind: graph.EdgeKindImports, StrengthHint: hintFunctional}},
		Language: graph.LangGo,
	}})

	tests := []struct {
		name string
		g    *graph.Graph
		cfg  config.ClassifyConfig
		want int
	}{
		{
			name: "no edge → pair detected",
			g:    emptyGraph,
			cfg:  cloneOnlyCfg(nil),
			want: 1,
		},
		{
			name: "edge exists → symmetric-upgrade territory, no pair",
			g:    edgeGraph,
			cfg:  cloneOnlyCfg(nil),
			want: 0,
		},
		{
			name: "approved label (canonical order) accepts the pair",
			g:    emptyGraph,
			cfg: cloneOnlyCfg(func(c *config.ClassifyConfig) {
				c.ApprovedLabels = map[string]string{modABKey: hintFunctional}
			}),
			want: 0,
		},
		{
			name: "approved label (reverse order) accepts the pair",
			g:    emptyGraph,
			cfg: cloneOnlyCfg(func(c *config.ClassifyConfig) {
				c.ApprovedLabels = map[string]string{modNameB + "\x00" + modNameA: hintFunctional}
			}),
			want: 0,
		},
		{
			name: "llm label accepts the pair",
			g:    emptyGraph,
			cfg: cloneOnlyCfg(func(c *config.ClassifyConfig) {
				c.LLMLabels = map[string]string{modABKey: hintFunctional}
			}),
			want: 0,
		},
		{
			name: "frozen pair scores balanced → no pair",
			g:    emptyGraph,
			cfg: cloneOnlyCfg(func(c *config.ClassifyConfig) {
				setModule(c, modNameA, func(d *config.ModuleDef) { d.Volatility = "frozen" })
				setModule(c, modNameB, func(d *config.ModuleDef) { d.Volatility = "frozen" })
			}),
			want: 0,
		},
		{
			name: "no clone pairs → nil",
			g:    emptyGraph,
			cfg:  twoModuleConfig(nil, nil),
			want: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := classify.CloneOnlyPairs(tt.g, tt.cfg)
			if len(got) != tt.want {
				t.Fatalf("CloneOnlyPairs returned %d pairs, want %d: %+v", len(got), tt.want, got)
			}
		})
	}
}

func TestCloneOnlyPairs_ReportOnlyDoesNotInventClassifiedEdges(t *testing.T) {
	t.Parallel()
	g := graph.Build(nil)
	cfg := cloneOnlyCfg(nil)

	pairs := classify.CloneOnlyPairs(g, cfg)
	if len(pairs) != 1 {
		t.Fatalf("CloneOnlyPairs returned %d pairs, want 1", len(pairs))
	}
	if idx := classify.Run(g, cfg); len(idx) != 0 {
		t.Fatalf("classify.Run produced %d classified graph edges for clone-only evidence, want 0: %+v", len(idx), idx)
	}
}

// TestCloneOnlyPairs_Classification verifies the scored classification: symmetric
// strength at the module-pair distance with worst-of-pair volatility, run through
// the standard book formula — never a hardcoded severity.
func TestCloneOnlyPairs_Classification(t *testing.T) {
	t.Parallel()
	g := graph.Build(nil)

	t.Run("flat names, undeclared volatility → medium", func(t *testing.T) {
		t.Parallel()
		pairs := classify.CloneOnlyPairs(g, cloneOnlyCfg(nil))
		if len(pairs) != 1 {
			t.Fatalf("pairs = %d, want 1", len(pairs))
		}
		p := pairs[0]
		if p.FromModule != modNameA || p.ToModule != modNameB {
			t.Errorf("pair modules = %s→%s, want %s→%s", p.FromModule, p.ToModule, modNameA, modNameB)
		}
		if p.FromPath != cloneFileA || p.ToPath != cloneFileB {
			t.Errorf("pair paths = %s→%s, want %s→%s", p.FromPath, p.ToPath, cloneFileA, cloneFileB)
		}
		if len(p.Locations) != 2 {
			t.Errorf("Locations = %d, want 2 (both sides)", len(p.Locations))
		}
		cl := p.Classification
		if cl.Strength != coupling.StrengthSymmetric {
			t.Errorf("Strength = %s, want symmetric", cl.Strength)
		}
		if cl.Distance != coupling.DistanceCrossModuleSameOwner {
			t.Errorf("Distance = %s, want cross_module_same_owner (flat names, no owners)", cl.Distance)
		}
		if cl.Volatility != coupling.VolatilityUndeclared {
			t.Errorf("Volatility = %s, want undeclared", cl.Volatility)
		}
		// Book formula: S=9, D=4, V=10 → max(|9-4|, 10-10)+1 = 6 → medium.
		if cl.Score.Value != 6 {
			t.Errorf("Score.Value = %d, want 6", cl.Score.Value)
		}
		if cl.Severity != coupling.SeverityMedium {
			t.Errorf("Severity = %s, want medium", cl.Severity)
		}
	})

	t.Run("worst-of-pair volatility drives the score", func(t *testing.T) {
		t.Parallel()
		cfg := cloneOnlyCfg(func(c *config.ClassifyConfig) {
			setModule(c, modNameA, func(d *config.ModuleDef) { d.Volatility = cfgVolLow })
			setModule(c, modNameB, func(d *config.ModuleDef) { d.Volatility = extVolHigh })
		})
		pairs := classify.CloneOnlyPairs(g, cfg)
		if len(pairs) != 1 {
			t.Fatalf("pairs = %d, want 1", len(pairs))
		}
		if got := pairs[0].Classification.Volatility; got != coupling.VolatilityHigh {
			t.Errorf("Volatility = %s, want high (worst of low/high pair)", got)
		}
	})

	t.Run("distinct explicit owners → diff-owner distance, high severity", func(t *testing.T) {
		t.Parallel()
		cfg := cloneOnlyCfg(func(c *config.ClassifyConfig) {
			setModule(c, modNameA, func(d *config.ModuleDef) { d.Owner = "team-a" })
			setModule(c, modNameB, func(d *config.ModuleDef) { d.Owner = "team-b" })
			c.ExplicitOwners = map[string]bool{modNameA: true, modNameB: true}
		})
		pairs := classify.CloneOnlyPairs(g, cfg)
		if len(pairs) != 1 {
			t.Fatalf("pairs = %d, want 1", len(pairs))
		}
		cl := pairs[0].Classification
		if cl.Distance != coupling.DistanceCrossModuleDiffOwner {
			t.Errorf("Distance = %s, want cross_module_different_owner", cl.Distance)
		}
		// Book formula: S=9, D=7, V=10 → max(|9-7|, 10-10)+1 = 3 → high.
		if cl.Score.Value != 3 {
			t.Errorf("Score.Value = %d, want 3", cl.Score.Value)
		}
		if cl.Severity != coupling.SeverityHigh {
			t.Errorf("Severity = %s, want high", cl.Severity)
		}
	})
}

// TestCloneOnlyPairs_NoEvidence verifies a clone pair with NO CloneEvidence
// entry at all still classifies without panicking: c.CloneEvidence[key] on a
// missing key returns a nil slice (Go map zero value, not a panic), so
// pairRepresentativePaths resolves both paths to "" — honest absence, not a
// fabricated location.
func TestCloneOnlyPairs_NoEvidence(t *testing.T) {
	t.Parallel()
	g := graph.Build(nil)
	cfg := twoModuleConfig(nil, modABClonePair) // CloneEvidence left unset (nil map)

	pairs := classify.CloneOnlyPairs(g, cfg)
	if len(pairs) != 1 {
		t.Fatalf("pairs = %d, want 1 (a clone pair with no evidence still classifies)", len(pairs))
	}
	p := pairs[0]
	if p.FromPath != "" || p.ToPath != "" {
		t.Errorf("FromPath/ToPath = %q/%q, want both empty (no clone evidence)", p.FromPath, p.ToPath)
	}
}
