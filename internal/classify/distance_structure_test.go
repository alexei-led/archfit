package classify

import (
	"strings"
	"testing"

	"github.com/alexei-led/archfit/internal/config"
	"github.com/alexei-led/archfit/internal/model/coupling"
	"github.com/alexei-led/archfit/internal/model/graph"
)

const (
	modInternalFoo             = "internal/foo"
	modInternalClassify        = "internal/classify"
	modInternalMetricsBoundary = "internal/metrics/boundary"
	modCmdArchfit              = "cmd/archfit"
	distOwnerTeamX             = "team-x"
	distOwnerTeamY             = "team-y"
	distDeployUnitA            = "svc-a"
	distDeployUnitB            = "svc-b"
	distModCore                = "core"
	distModAPI                 = "api"
)

func TestDistanceCompression(t *testing.T) {
	got := DistanceCompression()
	if !got.CompressedMiddleRungs {
		t.Fatal("CompressedMiddleRungs = false, want true")
	}
	for _, rung := range []int{2, 4, 7, 9, 10} {
		if !hasInt(got.ImplementedRungs, rung) {
			t.Errorf("ImplementedRungs = %v, missing %d", got.ImplementedRungs, rung)
		}
	}
	for _, rung := range []int{3, 5, 6, 8} {
		if !hasInt(got.OmittedRungs, rung) {
			t.Errorf("OmittedRungs = %v, missing %d", got.OmittedRungs, rung)
		}
	}
	if len(got.DeterministicSplits) == 0 {
		t.Fatal("DeterministicSplits is empty")
	}
	for _, rung := range []int{3, 5, 6, 8} {
		if reason := omittedRungReason(got.OmittedRungReasons, rung); reason == "" {
			t.Errorf("OmittedRungReasons missing D=%d: %+v", rung, got.OmittedRungReasons)
		}
	}
	if reason := omittedRungReason(got.OmittedRungReasons, 8); !strings.Contains(reason, "external_systems") {
		t.Errorf("D=8 omitted reason = %q, want external_systems decision", reason)
	}
}

func omittedRungReason(reasons []DistanceOmittedRungReason, rung int) string {
	for _, r := range reasons {
		if r.Rung == rung {
			return r.Reason
		}
	}
	return ""
}

func hasInt(values []int, want int) bool {
	for _, v := range values {
		if v == want {
			return true
		}
	}
	return false
}

func TestCodeStructureDistance(t *testing.T) {
	tests := []struct {
		name string
		from string
		to   string
		lang string
		want coupling.Distance
	}{
		// Empty inputs (language irrelevant — returns before segmenting).
		{name: "empty from", from: "", to: modInternalFoo, lang: graph.LangGo, want: coupling.DistanceUnknown},
		{name: "empty to", from: modInternalFoo, to: "", lang: graph.LangGo, want: coupling.DistanceUnknown},
		{name: "both empty", from: "", to: "", lang: graph.LangGo, want: coupling.DistanceUnknown},

		// Two flat single-segment names carry no tree signal; return the honest
		// floor (SameOwner) rather than fabricating DiffOwner from absent data.
		// codeStructureDistance is only reached when ownership is degenerate/absent,
		// so the modules already share — or have unknown — team context (P1 fix).
		{name: "flat a vs b", from: "a", to: "b", lang: graph.LangGo, want: coupling.DistanceCrossModuleSameOwner},
		{name: "flat alpha vs beta", from: "alpha", to: "beta", lang: graph.LangGo, want: coupling.DistanceCrossModuleSameOwner},

		// Siblings (same parent, different last segment).
		{name: "sibling metrics packages", from: modInternalMetricsBoundary, to: "internal/metrics/modularity", lang: graph.LangGo, want: coupling.DistanceCrossModuleSameOwner},
		{name: "sibling top-level", from: modCmdArchfit, to: "cmd/other", lang: graph.LangGo, want: coupling.DistanceCrossModuleSameOwner},
		{name: "sibling two-deep", from: "pkg/a", to: "pkg/b", lang: graph.LangGo, want: coupling.DistanceCrossModuleSameOwner},

		// Parent-child (shorter path is a prefix of the longer).
		{name: "parent-child internal/metrics", from: "internal/metrics", to: modInternalMetricsBoundary, lang: graph.LangGo, want: coupling.DistanceCrossModuleSameOwner},

		// Different subtrees under the same root.
		{name: "different subtrees under internal", from: modInternalClassify, to: modInternalMetricsBoundary, lang: graph.LangGo, want: coupling.DistanceCrossModuleDiffOwner},
		{name: "different subtrees deep", from: "internal/extract/go", to: "internal/metrics/modularity", lang: graph.LangGo, want: coupling.DistanceCrossModuleDiffOwner},

		// Different top-level roots (no common prefix segments).
		{name: "cmd vs internal", from: modCmdArchfit, to: "internal/extract/py", lang: graph.LangGo, want: coupling.DistanceCrossModuleDiffOwner},
		{name: "services vs infra", from: "services/payments", to: "infra/db", lang: graph.LangGo, want: coupling.DistanceCrossModuleDiffOwner},
		{name: "single vs multi-segment", from: "a", to: modInternalFoo, lang: graph.LangGo, want: coupling.DistanceCrossModuleDiffOwner},

		// Plan examples: siblings closer than distant trees.
		{name: "plan: metrics siblings are closer", from: "metrics/boundary", to: "metrics/modularity", lang: graph.LangGo, want: coupling.DistanceCrossModuleSameOwner},
		{name: "plan: cmd vs extract is farther", from: modCmdArchfit, to: "extract/py", lang: graph.LangGo, want: coupling.DistanceCrossModuleDiffOwner},

		// Dotted Python module names: split on "." so dotted siblings are not all
		// collapsed to single segments (the bug that drove the BC advisory flood).
		{name: "py dotted siblings", from: "pkg.foo", to: "pkg.bar", lang: graph.LangPython, want: coupling.DistanceCrossModuleSameOwner},
		{name: "py dotted deep siblings", from: "pkg.metrics.boundary", to: "pkg.metrics.modularity", lang: graph.LangPython, want: coupling.DistanceCrossModuleSameOwner},
		{name: "py dotted parent-child", from: "pkg.metrics", to: "pkg.metrics.boundary", lang: graph.LangPython, want: coupling.DistanceCrossModuleSameOwner},
		{name: "py dotted distant trees", from: "pkg.api.routes", to: "pkg.store.db", lang: graph.LangPython, want: coupling.DistanceCrossModuleDiffOwner},
		{name: "py dotted distant deep trees", from: "a.b.c.d", to: "a.x.y.z", lang: graph.LangPython, want: coupling.DistanceCrossModuleDiffOwner},
		// Flat Python names: same rationale as Go flat names — SameOwner floor.
		{name: "py flat single names", from: "core", to: "api", lang: graph.LangPython, want: coupling.DistanceCrossModuleSameOwner},

		// TS module names are slash-separated, so two files in one dir keep their
		// two-segment shape and stay siblings — never split on the ".ts" suffix.
		{name: "ts dotted filenames stay siblings", from: "src/utils.ts", to: "src/helpers.ts", lang: graph.LangTypeScript, want: coupling.DistanceCrossModuleSameOwner},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := codeStructureDistance(tc.from, tc.to, tc.lang)
			if got != tc.want {
				t.Errorf("codeStructureDistance(%q, %q, %q) = %q, want %q", tc.from, tc.to, tc.lang, got, tc.want)
			}
		})
	}
}

// TestClassifyDistance_Precedence exercises the precedence chain: deploy boundary
// is absolute; explicit ownership beats the code-structure fallback (so flat
// names with an explicit same owner stay SameOwner instead of collapsing to the
// flat-name DiffOwner default); and ownerless flat names fall through to
// code structure (DiffOwner).
func TestClassifyDistance_Precedence(t *testing.T) {
	fromPath, toPath := distModCore+"/x.go", distModAPI+"/y.go"
	mods := func(fromOwner, toOwner, fromUnit, toUnit string) map[string]config.ModuleDef {
		return map[string]config.ModuleDef{
			distModCore: {Paths: []string{distModCore + "/**"}, Owner: fromOwner, DeployUnit: fromUnit},
			distModAPI:  {Paths: []string{distModAPI + "/**"}, Owner: toOwner, DeployUnit: toUnit},
		}
	}
	both := map[string]bool{distModCore: true, distModAPI: true}

	tests := []struct {
		name      string
		modules   map[string]config.ModuleDef
		explicit  map[string]bool
		want      coupling.Distance
		wantBasis coupling.DistanceBasis
	}{
		{
			// Ownerless flat names: reach code structure; no tree signal → SameOwner
			// floor (honest: no evidence of different teams). P1 fix.
			name:      "ownerless flat names → code structure (SameOwner)",
			modules:   mods("", "", "", ""),
			want:      coupling.DistanceCrossModuleSameOwner,
			wantBasis: coupling.DistanceBasisStructure,
		},
		{
			// Both modules have the same explicit owner (team-x/team-x): the
			// explicit-owner sub-map is degenerate (1 distinct owner) → Step 2
			// falls through to code structure. Flat names "core" and "api" carry
			// no tree signal → SameOwner floor (P1 fix). The old DiffOwner here
			// produced false tight-coupling on every single-team flat-named repo.
			name:      "explicit same owner (degenerate) → falls through to code structure (SameOwner)",
			modules:   mods(distOwnerTeamX, distOwnerTeamX, "", ""),
			explicit:  both,
			want:      coupling.DistanceCrossModuleSameOwner,
			wantBasis: coupling.DistanceBasisStructure,
		},
		{
			// Two distinct explicit owners: sub-map is NOT degenerate → ownership applies.
			name:      "explicit different owners → DiffOwner (ownership basis)",
			modules:   mods(distOwnerTeamX, distOwnerTeamY, "", ""),
			explicit:  both,
			want:      coupling.DistanceCrossModuleDiffOwner,
			wantBasis: coupling.DistanceBasisOwnership,
		},
		{
			// One-sided explicit owner with same owner on both sides: explicit sub-map
			// has 1 distinct owner (team-x) → degenerate → falls through to code structure.
			// Flat names carry no tree signal → SameOwner floor (P1 fix).
			name:      "one-sided explicit owner, same (degenerate) → code structure (SameOwner)",
			modules:   mods(distOwnerTeamX, distOwnerTeamX, "", ""),
			explicit:  map[string]bool{distModCore: true},
			want:      coupling.DistanceCrossModuleSameOwner,
			wantBasis: coupling.DistanceBasisStructure,
		},
		{
			// One-sided explicit owner; the other endpoint is ownerless. The explicit
			// sub-map contains only {distModCore: "team-x"} → 1 distinct owner →
			// degenerate → falls through to code structure. Flat names → SameOwner (P1 fix).
			name:      "one-sided explicit owner, other ownerless (degenerate) → code structure (SameOwner)",
			modules:   mods(distOwnerTeamX, "", "", ""),
			explicit:  map[string]bool{distModCore: true},
			want:      coupling.DistanceCrossModuleSameOwner,
			wantBasis: coupling.DistanceBasisStructure,
		},
		{
			name:      "differing deploy units are absolute → CrossDeployUnit",
			modules:   mods(distOwnerTeamX, distOwnerTeamX, distDeployUnitA, distDeployUnitB),
			explicit:  both,
			want:      coupling.DistanceCrossDeployUnit,
			wantBasis: coupling.DistanceBasisDeployUnit,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			mi := buildModuleIndex(tc.modules)
			explicitOwnerMap := make(map[string]string, len(tc.explicit))
			for mod := range tc.explicit {
				explicitOwnerMap[mod] = tc.modules[mod].Owner
			}
			degExplicit := isDegenerateOwnerMap(explicitOwnerMap)
			fullOwnerMap := make(map[string]string, len(tc.modules))
			for name, def := range tc.modules {
				fullOwnerMap[name] = def.Owner
			}
			degOwners := isDegenerateOwnerMap(fullOwnerMap)
			got, gotBasis := classifyDistance(fromPath, toPath, graph.LangGo, mi, tc.modules, tc.explicit, degExplicit, degOwners)
			if got != tc.want {
				t.Errorf("classifyDistance = %q, want %q", got, tc.want)
			}
			if gotBasis != tc.wantBasis {
				t.Errorf("DistanceBasis = %q, want %q", gotBasis, tc.wantBasis)
			}
		})
	}
}

func TestClassifyDistance_SingleOwnerHierarchicalRepoUsesStructure(t *testing.T) {
	modules := map[string]config.ModuleDef{
		modInternalClassify: {Paths: []string{modInternalClassify + "/**"}, Owner: distOwnerTeamX},
		modCmdArchfit:       {Paths: []string{modCmdArchfit + "/**"}, Owner: distOwnerTeamX},
	}
	mi := buildModuleIndex(modules)
	explicit := map[string]bool{modInternalClassify: true, modCmdArchfit: true}
	degExplicit, degOwners := ownerDegeneracy(config.ClassifyConfig{Modules: modules, ExplicitOwners: explicit})

	got, gotBasis := classifyDistance(modInternalClassify+"/x.go", modCmdArchfit+"/main.go", graph.LangGo, mi, modules, explicit, degExplicit, degOwners)
	if got != coupling.DistanceCrossModuleDiffOwner {
		t.Fatalf("distance = %q, want %q from code-structure distance in a single-owner repo", got, coupling.DistanceCrossModuleDiffOwner)
	}
	if gotBasis != coupling.DistanceBasisStructure {
		t.Fatalf("basis = %q, want %q", gotBasis, coupling.DistanceBasisStructure)
	}
}

func TestOwnershipDistance(t *testing.T) {
	tests := []struct {
		name      string
		fromOwner string
		toOwner   string
		want      coupling.Distance
	}{
		{name: "both empty — no signal", fromOwner: "", toOwner: "", want: coupling.DistanceSameModule},
		{name: "same owner", fromOwner: distOwnerTeamX, toOwner: distOwnerTeamX, want: coupling.DistanceCrossModuleSameOwner},
		{name: "different owners", fromOwner: distOwnerTeamX, toOwner: distOwnerTeamY, want: coupling.DistanceCrossModuleDiffOwner},
		{name: "from empty to non-empty", fromOwner: "", toOwner: distOwnerTeamY, want: coupling.DistanceCrossModuleDiffOwner},
		{name: "from non-empty to empty", fromOwner: distOwnerTeamX, toOwner: "", want: coupling.DistanceCrossModuleDiffOwner},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := ownershipDistance(tc.fromOwner, tc.toOwner)
			if got != tc.want {
				t.Errorf("ownershipDistance(%q, %q) = %q, want %q", tc.fromOwner, tc.toOwner, got, tc.want)
			}
		})
	}
}

func TestDeployDistance(t *testing.T) {
	tests := []struct {
		name     string
		fromUnit string
		toUnit   string
		want     coupling.Distance
	}{
		{name: "both empty — no signal", fromUnit: "", toUnit: "", want: coupling.DistanceSameModule},
		{name: "same unit", fromUnit: distDeployUnitA, toUnit: distDeployUnitA, want: coupling.DistanceSameModule},
		{name: "different units", fromUnit: distDeployUnitA, toUnit: "svc-b", want: coupling.DistanceCrossDeployUnit},
		{name: "from empty", fromUnit: "", toUnit: "svc-b", want: coupling.DistanceSameModule},
		{name: "to empty", fromUnit: distDeployUnitA, toUnit: "", want: coupling.DistanceSameModule},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := deployDistance(tc.fromUnit, tc.toUnit)
			if got != tc.want {
				t.Errorf("deployDistance(%q, %q) = %q, want %q", tc.fromUnit, tc.toUnit, got, tc.want)
			}
		})
	}
}

// TestClassifyDistance_MultiOwnerPrecomputed verifies that classifyDistance
// returns identical results when the degenerate flags are pre-computed once
// (as Run does) versus computed inline. This covers the O(E·M) fix: the
// maps are built once before the edge loop and passed in, not rebuilt per edge.
func TestClassifyDistance_MultiOwnerPrecomputed(t *testing.T) {
	// Two modules with two distinct explicit owners — non-degenerate.
	modules := map[string]config.ModuleDef{
		distModCore: {Paths: []string{distModCore + "/**"}, Owner: distOwnerTeamX},
		distModAPI:  {Paths: []string{distModAPI + "/**"}, Owner: distOwnerTeamY},
	}
	explicit := map[string]bool{distModCore: true, distModAPI: true}

	mi := buildModuleIndex(modules)

	// Pre-compute as Run() does.
	explicitOwnerMap := make(map[string]string, len(explicit))
	for mod := range explicit {
		explicitOwnerMap[mod] = modules[mod].Owner
	}
	degExplicit := isDegenerateOwnerMap(explicitOwnerMap)

	fullOwnerMap := make(map[string]string, len(modules))
	for name, def := range modules {
		fullOwnerMap[name] = def.Owner
	}
	degOwners := isDegenerateOwnerMap(fullOwnerMap)

	// Two distinct owners → explicit map is NOT degenerate → ownership basis.
	if degExplicit {
		t.Fatal("explicit owner map with two distinct owners must not be degenerate")
	}

	fromPath := distModCore + "/x.go"
	toPath := distModAPI + "/y.go"

	got, gotBasis := classifyDistance(fromPath, toPath, "go", mi, modules, explicit, degExplicit, degOwners)
	if got != coupling.DistanceCrossModuleDiffOwner {
		t.Errorf("distance = %q, want cross_module_diff_owner", got)
	}
	if gotBasis != coupling.DistanceBasisOwnership {
		t.Errorf("basis = %q, want ownership", gotBasis)
	}

	// Calling again with the same pre-computed flags must return the same result
	// (verifies the flags are stateless / not mutated between calls).
	got2, gotBasis2 := classifyDistance(fromPath, toPath, "go", mi, modules, explicit, degExplicit, degOwners)
	if got2 != got || gotBasis2 != gotBasis {
		t.Errorf("second call returned different result: dist=%q basis=%q", got2, gotBasis2)
	}

	// Same-owner pair in the same config → degenerate (1 distinct owner) →
	// falls through to code structure. Flat names → SameOwner floor (P1 fix).
	modulesSame := map[string]config.ModuleDef{
		distModCore: {Paths: []string{distModCore + "/**"}, Owner: distOwnerTeamX},
		distModAPI:  {Paths: []string{distModAPI + "/**"}, Owner: distOwnerTeamX},
	}
	explicitSameMap := map[string]string{distModCore: distOwnerTeamX, distModAPI: distOwnerTeamX}
	degSame := isDegenerateOwnerMap(explicitSameMap)
	// Single owner → degenerate → falls through to code structure.
	if !degSame {
		t.Fatal("single-owner explicit map must be degenerate")
	}
	miSame := buildModuleIndex(modulesSame)
	fullSame := map[string]string{distModCore: distOwnerTeamX, distModAPI: distOwnerTeamX}
	degFullSame := isDegenerateOwnerMap(fullSame)
	gotSame, gotBasisSame := classifyDistance(fromPath, toPath, "go", miSame, modulesSame, explicit, degSame, degFullSame)
	// Flat names "core" and "api" with degenerate owner → code structure → SameOwner (P1 fix).
	// Previously returned DiffOwner, causing false tight-coupling on single-team flat-named repos.
	if gotSame != coupling.DistanceCrossModuleSameOwner {
		t.Errorf("degenerate same-owner: distance = %q, want cross_module_same_owner", gotSame)
	}
	if gotBasisSame != coupling.DistanceBasisStructure {
		t.Errorf("degenerate same-owner: basis = %q, want structure", gotBasisSame)
	}
}

func TestIsDegenerateOwnerMap(t *testing.T) {
	tests := []struct {
		name   string
		owners map[string]string
		want   bool
	}{
		// Empty map (no configured owners) — degenerate: nothing to suppress.
		{
			name:   "empty map",
			owners: map[string]string{},
			want:   true,
		},
		// All modules have no owner — degenerate.
		{
			name:   "all empty owners",
			owners: map[string]string{"a": "", "b": "", "c": ""},
			want:   true,
		},
		// Single distinct owner shared by every module — degenerate (git-author fallback).
		{
			name:   "single owner for all modules",
			owners: map[string]string{"a": distOwnerTeamX, "b": distOwnerTeamX, "c": distOwnerTeamX},
			want:   true,
		},
		// Mix of empty and a single non-empty owner — degenerate.
		{
			name:   "some empty some same owner",
			owners: map[string]string{"a": distOwnerTeamX, "b": "", "c": distOwnerTeamX},
			want:   true,
		},
		// Two distinct non-empty owners — real multi-team repo, not degenerate.
		{
			name:   "two distinct owners",
			owners: map[string]string{"a": distOwnerTeamX, "b": distOwnerTeamY},
			want:   false,
		},
		// Three distinct owners — real multi-team repo.
		{
			name:   "three distinct owners",
			owners: map[string]string{"a": distOwnerTeamX, "b": distOwnerTeamY, "c": "team-z"},
			want:   false,
		},
		// Two owners plus an empty entry — still multi-owner.
		{
			name:   "two owners plus empty",
			owners: map[string]string{"a": distOwnerTeamX, "b": distOwnerTeamY, "c": ""},
			want:   false,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := isDegenerateOwnerMap(tc.owners)
			if got != tc.want {
				t.Errorf("isDegenerateOwnerMap(%v) = %v, want %v", tc.owners, got, tc.want)
			}
		})
	}
}

func TestMaxDistance(t *testing.T) {
	tests := []struct {
		name   string
		inputs []coupling.Distance
		want   coupling.Distance
	}{
		{
			name:   "all same_module",
			inputs: []coupling.Distance{coupling.DistanceSameModule, coupling.DistanceSameModule},
			want:   coupling.DistanceSameModule,
		},
		{
			name:   "deploy beats owner",
			inputs: []coupling.Distance{coupling.DistanceCrossModuleSameOwner, coupling.DistanceCrossDeployUnit},
			want:   coupling.DistanceCrossDeployUnit,
		},
		{
			name:   "diff_owner beats same_owner",
			inputs: []coupling.Distance{coupling.DistanceCrossModuleSameOwner, coupling.DistanceCrossModuleDiffOwner},
			want:   coupling.DistanceCrossModuleDiffOwner,
		},
		{
			name: "three signals: struct=diff_owner, owner=same_owner, deploy=cross_deploy",
			inputs: []coupling.Distance{
				coupling.DistanceCrossModuleDiffOwner,
				coupling.DistanceCrossModuleSameOwner,
				coupling.DistanceCrossDeployUnit,
			},
			want: coupling.DistanceCrossDeployUnit,
		},
		{
			// diff_owner is concrete evidence; unknown is absence of signal — diff_owner wins.
			name:   "diff_owner beats unknown",
			inputs: []coupling.Distance{coupling.DistanceUnknown, coupling.DistanceCrossModuleDiffOwner},
			want:   coupling.DistanceCrossModuleDiffOwner,
		},
		{
			name:   "unknown beats same_owner",
			inputs: []coupling.Distance{coupling.DistanceCrossModuleSameOwner, coupling.DistanceUnknown},
			want:   coupling.DistanceUnknown,
		},
		{
			// Locks the distanceLevelOrdinal comment: DistanceExternal ranks
			// highest so a future combination cannot silently demote it.
			name: "external beats cross_deploy and diff_owner",
			inputs: []coupling.Distance{
				coupling.DistanceCrossDeployUnit,
				coupling.DistanceExternal,
				coupling.DistanceCrossModuleDiffOwner,
			},
			want: coupling.DistanceExternal,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := maxDistance(tc.inputs...)
			if got != tc.want {
				t.Errorf("maxDistance(%v) = %q, want %q", tc.inputs, got, tc.want)
			}
		})
	}
}
