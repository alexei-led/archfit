package evidence

import "testing"

func TestChangedProducerContractsHaveDistinctSemantics(t *testing.T) {
	for _, tc := range []struct {
		tool string
		want string
	}{
		{"dependency-cruiser", "dependency-cruiser.v2"},
		{"ast-grep/syntax", "ast-grep/syntax.v2"},
		{"scip", "scip.v3"},
		{"scip-symbols", "scip-symbols.v2"},
		{"go/packages", "go/packages.v2"},
		{"grimp", "grimp.v3"},
		{"cargo", "cargo.v1"},
		{"cargo-modules", "cargo-modules.v2"},
		{"dotnet-msbuild", "dotnet-msbuild.v1"},
		{"roslyn", "roslyn.v1"},
	} {
		t.Run(tc.tool, func(t *testing.T) {
			semantics, external := MeasurementContract(tc.tool)
			if semantics != tc.want || !external {
				t.Fatalf("MeasurementContract(%q) = (%q, %v), want (%q, true)", tc.tool, semantics, external, tc.want)
			}
		})
	}
}
