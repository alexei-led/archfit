package classify

import (
	"testing"

	"github.com/alexei-led/archfit/internal/config"
	"github.com/alexei-led/archfit/internal/model/coupling"
)

const (
	modInternalFoo             = "internal/foo"
	modInternalMetricsBoundary = "internal/metrics/boundary"
	modCmdArchfit              = "cmd/archfit"
	distOwnerTeamX             = "team-x"
	distOwnerTeamY             = "team-y"
	distDeployUnitA            = "svc-a"
	distDeployUnitB            = "svc-b"
	distModCore                = "core"
	distModAPI                 = "api"
)

func TestCodeStructureDistance(t *testing.T) {
	tests := []struct {
		name string
		from string
		to   string
		want coupling.Distance
	}{
		// Empty inputs.
		{name: "empty from", from: "", to: modInternalFoo, want: coupling.DistanceUnknown},
		{name: "empty to", from: modInternalFoo, to: "", want: coupling.DistanceUnknown},
		{name: "both empty", from: "", to: "", want: coupling.DistanceUnknown},

		// Two flat single-segment names share no tree structure → DiffOwner.
		// (Explicit same-owner config is resolved upstream in classifyDistance.)
		{name: "flat a vs b", from: "a", to: "b", want: coupling.DistanceCrossModuleDiffOwner},
		{name: "flat alpha vs beta", from: "alpha", to: "beta", want: coupling.DistanceCrossModuleDiffOwner},

		// Siblings (same parent, different last segment).
		{name: "sibling metrics packages", from: modInternalMetricsBoundary, to: "internal/metrics/modularity", want: coupling.DistanceCrossModuleSameOwner},
		{name: "sibling top-level", from: modCmdArchfit, to: "cmd/other", want: coupling.DistanceCrossModuleSameOwner},
		{name: "sibling two-deep", from: "pkg/a", to: "pkg/b", want: coupling.DistanceCrossModuleSameOwner},

		// Parent-child (shorter path is a prefix of the longer).
		{name: "parent-child internal/metrics", from: "internal/metrics", to: modInternalMetricsBoundary, want: coupling.DistanceCrossModuleSameOwner},

		// Different subtrees under the same root.
		{name: "different subtrees under internal", from: "internal/classify", to: modInternalMetricsBoundary, want: coupling.DistanceCrossModuleDiffOwner},
		{name: "different subtrees deep", from: "internal/extract/go", to: "internal/metrics/modularity", want: coupling.DistanceCrossModuleDiffOwner},

		// Different top-level roots (no common prefix segments).
		{name: "cmd vs internal", from: modCmdArchfit, to: "internal/extract/py", want: coupling.DistanceCrossModuleDiffOwner},
		{name: "services vs infra", from: "services/payments", to: "infra/db", want: coupling.DistanceCrossModuleDiffOwner},
		{name: "single vs multi-segment", from: "a", to: modInternalFoo, want: coupling.DistanceCrossModuleDiffOwner},

		// Plan examples: siblings closer than distant trees.
		{name: "plan: metrics siblings are closer", from: "metrics/boundary", to: "metrics/modularity", want: coupling.DistanceCrossModuleSameOwner},
		{name: "plan: cmd vs extract is farther", from: modCmdArchfit, to: "extract/py", want: coupling.DistanceCrossModuleDiffOwner},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := codeStructureDistance(tc.from, tc.to)
			if got != tc.want {
				t.Errorf("codeStructureDistance(%q, %q) = %q, want %q", tc.from, tc.to, got, tc.want)
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
		name     string
		modules  map[string]config.ModuleDef
		explicit map[string]bool
		want     coupling.Distance
	}{
		{
			name:    "ownerless flat names → code structure (DiffOwner)",
			modules: mods("", "", "", ""),
			want:    coupling.DistanceCrossModuleDiffOwner,
		},
		{
			name:     "explicit same owner overrides flat-name default → SameOwner",
			modules:  mods(distOwnerTeamX, distOwnerTeamX, "", ""),
			explicit: both,
			want:     coupling.DistanceCrossModuleSameOwner,
		},
		{
			name:     "explicit different owners → DiffOwner",
			modules:  mods(distOwnerTeamX, distOwnerTeamY, "", ""),
			explicit: both,
			want:     coupling.DistanceCrossModuleDiffOwner,
		},
		{
			name:     "one-sided explicit owner, same → SameOwner",
			modules:  mods(distOwnerTeamX, distOwnerTeamX, "", ""),
			explicit: map[string]bool{distModCore: true},
			want:     coupling.DistanceCrossModuleSameOwner,
		},
		{
			name:     "differing deploy units are absolute → CrossDeployUnit",
			modules:  mods(distOwnerTeamX, distOwnerTeamX, distDeployUnitA, distDeployUnitB),
			explicit: both,
			want:     coupling.DistanceCrossDeployUnit,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			mi := buildModuleIndex(tc.modules)
			got := classifyDistance(fromPath, toPath, mi, tc.modules, tc.explicit)
			if got != tc.want {
				t.Errorf("classifyDistance = %q, want %q", got, tc.want)
			}
		})
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
