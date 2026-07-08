package engine

import (
	"testing"
	"time"

	"github.com/alexei-led/archfit/internal/baseline"
	"github.com/alexei-led/archfit/internal/model/clone"
	"github.com/alexei-led/archfit/internal/model/coupling"
	"github.com/alexei-led/archfit/internal/model/graph"
	"github.com/alexei-led/archfit/internal/model/module"
	"github.com/alexei-led/archfit/internal/view"
)

const (
	testCrateA        = "crate_a"
	testCrateB        = "crate_b"
	testCrateAGlob    = testCrateA + "/**"
	testCrateBGlob    = testCrateB + "/**"
	testCrateALib     = testCrateA + "/src/lib.rs"
	testCrateBLib     = testCrateB + "/src/lib.rs"
	testClonePairKey  = testCrateA + "\x00" + testCrateB
	testCargoManifest = testCrateA + "/Cargo.toml"
)

// testTwoCrateModuleMap returns a ModuleMap with two modules matching the
// crate_a/crate_b path layout shared by the tests below.
func testTwoCrateModuleMap() module.Map {
	return module.BuildMap(map[string]module.ModuleDef{
		testCrateA: {Paths: []string{testCrateAGlob}},
		testCrateB: {Paths: []string{testCrateBGlob}},
	})
}

// TestBuildClonePairSet_CarriesLocationEvidence verifies that buildClonePairSet
// no longer collapses a clone cluster to a bare module-pair boolean: it also
// returns the real duplicated-code file:line locations (both sides) keyed by
// the same canonical pair, sourced from the cluster's per-file line ranges (B6).
func TestBuildClonePairSet_CarriesLocationEvidence(t *testing.T) {
	clusters := []clone.Cluster{
		{
			Files: []string{testCrateALib, testCrateBLib},
			Lines: 12,
			Locations: []clone.LineRange{
				{StartLine: 10, EndLine: 22},
				{StartLine: 40, EndLine: 52},
			},
		},
	}

	pairs, evidence := buildClonePairSet(clusters, testTwoCrateModuleMap(), nil)

	if _, ok := pairs[testClonePairKey]; !ok {
		t.Fatalf("pair set missing key %q: %v", testClonePairKey, pairs)
	}
	locs, ok := evidence[testClonePairKey]
	if !ok {
		t.Fatalf("evidence map missing key %q: %v", testClonePairKey, evidence)
	}
	want := []graph.Location{
		{File: testCrateALib, Line: 10},
		{File: testCrateBLib, Line: 40},
	}
	if len(locs) != len(want) {
		t.Fatalf("evidence locations = %v, want %v", locs, want)
	}
	for _, w := range want {
		if !containsLocation(locs, w) {
			t.Errorf("evidence locations %v missing %v", locs, w)
		}
	}
}

func TestBuildClonePairSet_SkipsTypeScriptFamilyTestsInFallback(t *testing.T) {
	tests := []struct {
		name string
		file string
	}{
		{name: "mts spec", file: testCrateA + "/src/widget.spec.mts"},
		{name: "mjs spec", file: testCrateA + "/src/widget.spec.mjs"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			clusters := []clone.Cluster{{
				Files:     []string{tt.file, testCrateBLib},
				Lines:     8,
				Locations: []clone.LineRange{{StartLine: 5, EndLine: 12}, {StartLine: 40, EndLine: 47}},
			}}

			pairs, evidence := buildClonePairSet(clusters, testTwoCrateModuleMap(), nil)
			if len(pairs) != 0 {
				t.Fatalf("expected test clone cluster to be excluded, got pairs=%v evidence=%v", pairs, evidence)
			}
		})
	}
}

// TestCollectAdvisories_ClonebackedSymmetricFinding_CitesRealLocation is the B6
// end-to-end guard: a Symmetric-strength finding backed by a cross-module clone
// pair must cite the REAL duplicated-code location jscpd found, not only the
// edge's generic baseline provenance (e.g. a Rust crate's Cargo.toml:0).
func TestCollectAdvisories_ClonebackedSymmetricFinding_CitesRealLocation(t *testing.T) {
	baselineLoc := graph.Location{File: testCargoManifest, Line: 0}
	edge := graph.Edge{
		From:      "package:" + testCrateA,
		To:        "package:" + testCrateB,
		Kind:      graph.EdgeKindDependsOn,
		Locations: []graph.Location{baselineLoc},
	}
	g := graph.Build([]graph.Facts{{
		Nodes: []graph.Node{
			{Kind: graph.NodeKindPackage, Path: testCrateA},
			{Kind: graph.NodeKindPackage, Path: testCrateB},
		},
		Edges:    []graph.Edge{edge},
		Language: graph.LangRust,
	}})

	cloneLocs := []graph.Location{
		{File: testCrateALib, Line: 12},
		{File: testCrateBLib, Line: 40},
	}
	cloneCouplingLocs := []coupling.Location{
		{File: testCrateALib, Line: 12},
		{File: testCrateBLib, Line: 40},
	}
	key := edge.From + "\x00" + edge.To + "\x00" + string(edge.Kind)
	couplingIdx := coupling.Index{
		key: coupling.Classification{
			Strength:       coupling.StrengthSymmetric,
			Distance:       coupling.DistanceCrossModuleSameOwner,
			Volatility:     coupling.VolatilityMedium,
			Severity:       coupling.SeverityMedium,
			CloneLocations: cloneCouplingLocs,
		},
	}
	classifyCfg := view.ClassifyConfig{ModuleMap: testTwoCrateModuleMap()}
	in := RunInput{Now: time.Now(), Accepted: baseline.Baseline{}, Waivers: view.WaiverSet{}}

	findings := collectAdvisories(g, couplingIdx, classifyCfg, nil, in)

	var locs []graph.Location
	var found bool
	for _, f := range findings {
		if f.RuleID != RuleIDBCImbalanced {
			continue
		}
		locs = f.Locations
		found = true
	}
	if !found {
		t.Fatalf("no %s finding produced: %+v", RuleIDBCImbalanced, findings)
	}

	if !containsLocation(locs, baselineLoc) {
		t.Errorf("Locations lost the baseline provenance: %v", locs)
	}
	for _, want := range cloneLocs {
		if !containsLocation(locs, want) {
			t.Errorf("Locations = %v; missing real clone-derived location %v (regressed to baseline-only)", locs, want)
		}
	}
}

// TestVolatilityClause covers every Volatility value, including the frozen
// and undeclared cases the advisory prose has to distinguish from a genuinely
// unresolved (unknown) target.
func TestVolatilityClause(t *testing.T) {
	wantUnknown := "a target of unknown volatility"
	tests := []struct {
		v    coupling.Volatility
		want string
	}{
		{coupling.VolatilityHigh, "a volatile target"},
		{coupling.VolatilityMedium, "a moderately volatile target"},
		{coupling.VolatilityLow, "a low-volatility target"},
		{coupling.VolatilityFrozen, "a frozen target"},
		{coupling.VolatilityUndeclared, "a target of undeclared volatility"},
		{coupling.VolatilityUnknown, wantUnknown},
		{coupling.Volatility("bogus"), wantUnknown}, // default branch: any other unrecognized value
	}
	for _, tt := range tests {
		t.Run(string(tt.v), func(t *testing.T) {
			if got := volatilityClause(tt.v); got != tt.want {
				t.Errorf("volatilityClause(%q) = %q, want %q", tt.v, got, tt.want)
			}
		})
	}
}

func containsLocation(locs []graph.Location, want graph.Location) bool {
	for _, l := range locs {
		if l == want {
			return true
		}
	}
	return false
}
