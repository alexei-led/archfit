// Package arch_test also pins the coverage names that cross the adapter and
// evidence boundary. It lives here, not under internal/assessment, because
// assessment must not import an extractor adapter even from a test.
package arch_test

import (
	"testing"

	evidenceports "github.com/alexei-led/archfit/v3/internal/evidence/ports"
	"github.com/alexei-led/archfit/v3/internal/extract/dotnet"
	goextract "github.com/alexei-led/archfit/v3/internal/extract/golang"
	"github.com/alexei-led/archfit/v3/internal/extract/registry"
	"github.com/alexei-led/archfit/v3/internal/extract/ts"
	"github.com/alexei-led/archfit/v3/internal/model/evidence"
)

// Use adapter names so a rename cannot silently change coverage classification.
var (
	toolDepCruiserName    = ts.New(nil, evidenceports.ExtractConfig{}).CoverageTool()
	toolGoPackagesName    = goextract.New(evidenceports.ExtractConfig{}).CoverageTool()
	toolDotNetMSBuildName = dotnet.New(nil, evidenceports.ExtractConfig{}, nil).CoverageTool()
	toolRoslynName        = dotnet.New(nil, evidenceports.ExtractConfig{}, nil).LastTypeGraphCoverage().Tool
)

func TestPartialFromUnresolvedSpecifiers_AdapterCoverageNames(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name string
		tool string
		want bool
	}{
		{"dependency-cruiser completes with unresolved specifiers", toolDepCruiserName, true},
		{"go/packages counts packages it could not load", toolGoPackagesName, false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got := evidence.PartialFromUnresolvedSpecifiers(evidence.Coverage{
				Tool: tc.tool, Status: evidence.StatusPartial, Unresolved: 1,
			})
			if got != tc.want {
				t.Fatalf("PartialFromUnresolvedSpecifiers(tool=%q) = %v, want %v — the adapter's coverage name and the predicate's table have drifted apart",
					tc.tool, got, tc.want)
			}
		})
	}
}

// The .NET rows keep the names the registry and extractor contract spell.
func TestDotNetCoverageNamesMatchTheRegistryAndContract(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct{ adapter, registryName string }{
		{toolDotNetMSBuildName, registry.ToolDotNetMSBuild},
		{toolRoslynName, registry.ToolRoslyn},
	} {
		if tc.adapter != tc.registryName {
			t.Errorf("adapter coverage name %q, registry %q", tc.adapter, tc.registryName)
		}
		if semantics, external := evidence.MeasurementContract(tc.adapter); semantics == "" || !external {
			t.Errorf("MeasurementContract(%q) = (%q, %v), want an external contract", tc.adapter, semantics, external)
		}
	}
}
