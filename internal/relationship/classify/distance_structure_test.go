package classify

import (
	"testing"

	"github.com/alexei-led/archfit/internal/policy"
	"github.com/alexei-led/archfit/internal/relationship/coupling"
)

const (
	modInternalFoo               = "internal/foo"
	modInternalClassify          = "internal/classify"
	modInternalMetrics           = "internal/metrics"
	modInternalMetricsBoundary   = "internal/metrics/boundary"
	modInternalMetricsModularity = "internal/metrics/modularity"
	modCmdArchfit                = "cmd/archfit"
	modPyMetricsBoundary         = "pkg.metrics.boundary"
	modPyMetricsModularity       = "pkg.metrics.modularity"
	distOwnerTeamX               = "team-x"
	distOwnerTeamY               = "team-y"
	distDeployUnitA              = "svc-a"
	distDeployUnitB              = "svc-b"
	distModCore                  = "core"
	distModAPI                   = "api"
)

func TestDistanceCompression(t *testing.T) {
	got := DistanceCompression()
	for _, rung := range []int{2, 9, 10} {
		if !hasInt(got.ImplementedRungs, rung) {
			t.Errorf("ImplementedRungs = %v, missing %d", got.ImplementedRungs, rung)
		}
	}
	for _, rung := range []int{4, 6, 7} {
		if !hasInt(got.OmittedRungs, rung) {
			t.Errorf("OmittedRungs = %v, missing %d", got.OmittedRungs, rung)
		}
		if got.OmittedRungReasons == nil || omittedRungReason(got.OmittedRungReasons, rung) == "" {
			t.Errorf("rung %d has no omitted-rung reason", rung)
		}
	}
	for _, rung := range got.ImplementedRungs {
		if hasInt(got.OmittedRungs, rung) {
			t.Errorf("rung %d is both implemented and omitted", rung)
		}
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

func TestHierarchySpan(t *testing.T) {
	tree := BuildContainment(map[string]policy.ModuleDef{
		"sales":   {Paths: []string{"sales/**"}},
		"cart":    {Paths: []string{"sales/cart/**"}},
		"invoice": {Paths: []string{"sales/invoice/**"}},
		"stock":   {Paths: []string{"stock/**"}},
	})
	tests := []struct {
		from, to       string
		wantCrossings  int
		wantSharedRoot int
	}{
		{"cart", "invoice", 2, 1},
		{"cart", "stock", 3, 0},
		{"sales", "cart", 1, 1},
	}
	for _, tt := range tests {
		got := HierarchySpan(tree, tt.from, tt.to)
		if got.BoundaryCrossings != tt.wantCrossings || got.SharedAncestor != tt.wantSharedRoot {
			t.Errorf("HierarchySpan(%s, %s) = %+v, want crossings %d shared %d", tt.from, tt.to, got, tt.wantCrossings, tt.wantSharedRoot)
		}
	}
}

// Every module boundary names itself by what else changes across it, in a fixed
// order: deploy unit, then owner, then the bare module boundary.
func TestClassifyDistance_BoundaryTokens(t *testing.T) {
	tests := []struct {
		name      string
		from, to  policy.ModuleDef
		fromPath  string
		toPath    string
		wantDist  coupling.Distance
		wantBasis coupling.DistanceBasis
	}{
		{"same module", policy.ModuleDef{Paths: []string{"a/**"}}, policy.ModuleDef{Paths: []string{"b/**"}}, "a/x.go", "a/y.go", coupling.DistanceSameModule, coupling.DistanceBasisUnknown},
		{"unresolved target", policy.ModuleDef{Paths: []string{"a/**"}}, policy.ModuleDef{Paths: []string{"b/**"}}, "a/x.go", "zzz/y.go", coupling.DistanceUnknown, coupling.DistanceBasisUnknown},
		{"no owner, no deploy unit", policy.ModuleDef{Paths: []string{"a/**"}}, policy.ModuleDef{Paths: []string{"b/**"}}, "a/x.go", "b/y.go", coupling.DistanceCrossModule, coupling.DistanceBasisModule},
		{"same owner", policy.ModuleDef{Paths: []string{"a/**"}, Owner: distOwnerTeamX}, policy.ModuleDef{Paths: []string{"b/**"}, Owner: distOwnerTeamX}, "a/x.go", "b/y.go", coupling.DistanceCrossModule, coupling.DistanceBasisModule},
		{"different owners", policy.ModuleDef{Paths: []string{"a/**"}, Owner: distOwnerTeamX}, policy.ModuleDef{Paths: []string{"b/**"}, Owner: distOwnerTeamY}, "a/x.go", "b/y.go", coupling.DistanceCrossModuleDiffOwner, coupling.DistanceBasisOwnership},
		{"one owner empty is no owner change", policy.ModuleDef{Paths: []string{"a/**"}, Owner: distOwnerTeamX}, policy.ModuleDef{Paths: []string{"b/**"}}, "a/x.go", "b/y.go", coupling.DistanceCrossModule, coupling.DistanceBasisModule},
		{"different deploy units", policy.ModuleDef{Paths: []string{"a/**"}, DeployUnit: distDeployUnitA}, policy.ModuleDef{Paths: []string{"b/**"}, DeployUnit: distDeployUnitB}, "a/x.go", "b/y.go", coupling.DistanceCrossDeployUnit, coupling.DistanceBasisDeployUnit},
		{"same deploy unit", policy.ModuleDef{Paths: []string{"a/**"}, DeployUnit: distDeployUnitA}, policy.ModuleDef{Paths: []string{"b/**"}, DeployUnit: distDeployUnitA}, "a/x.go", "b/y.go", coupling.DistanceCrossModule, coupling.DistanceBasisModule},
		{"deploy unit beats owner", policy.ModuleDef{Paths: []string{"a/**"}, Owner: distOwnerTeamX, DeployUnit: distDeployUnitA}, policy.ModuleDef{Paths: []string{"b/**"}, Owner: distOwnerTeamY, DeployUnit: distDeployUnitB}, "a/x.go", "b/y.go", coupling.DistanceCrossDeployUnit, coupling.DistanceBasisDeployUnit},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			modules := map[string]policy.ModuleDef{"a": tt.from, "b": tt.to}
			gotDist, gotBasis := classifyDistance(tt.fromPath, tt.toPath, buildModuleIndex(modules), modules)
			if gotDist != tt.wantDist || gotBasis != tt.wantBasis {
				t.Errorf("classifyDistance = (%s, %q), want (%s, %q)", gotDist, gotBasis, tt.wantDist, tt.wantBasis)
			}
		})
	}
}

// Module key spelling never sets distance: a flat pair and a nested pair get
// the same token.
func TestClassifyDistance_KeySpellingIsIrrelevant(t *testing.T) {
	flat := map[string]policy.ModuleDef{"core": {Paths: []string{"core/**"}}, "api": {Paths: []string{"api/**"}}}
	nested := map[string]policy.ModuleDef{"internal/core": {Paths: []string{"x/core/**"}}, "internal/deep/api": {Paths: []string{"y/api/**"}}}
	d1, _ := classifyDistance("core/a.go", "api/b.go", buildModuleIndex(flat), flat)
	d2, _ := classifyDistance("x/core/a.go", "y/api/b.go", buildModuleIndex(nested), nested)
	if d1 != d2 {
		t.Errorf("flat = %s, nested = %s, want equal", d1, d2)
	}
}
