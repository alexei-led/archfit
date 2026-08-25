// Package arch_test also pins the coverage names that cross the adapter and
// decision boundary. It lives here, not under internal/assessment, because
// assessment must not import an extractor adapter even from a test.
package arch_test

import (
	"testing"

	"github.com/alexei-led/archfit/internal/assessment/decision"
	"github.com/alexei-led/archfit/internal/assessment/result"
	evidenceports "github.com/alexei-led/archfit/internal/evidence/ports"
	goextract "github.com/alexei-led/archfit/internal/extract/golang"
	"github.com/alexei-led/archfit/internal/extract/py"
	"github.com/alexei-led/archfit/internal/extract/ts"
)

// PartialFromUnresolvedSpecifiers keys on these two names alone to separate a
// COMPLETED run with unresolved import specifiers from a run that did not
// finish, so a rename in ts.go or py.go would make both --base origin
// classification and `config compare` inert on TypeScript and Python — silently,
// because every other status keeps working. Sourcing them from the adapters
// turns that rename into a test failure. The decision package itself must not
// import an adapter; a test file may (the ring rule and TestArchImports both
// scan production imports only).
var (
	toolDepCruiserName = ts.New(nil, evidenceports.ExtractConfig{}).CoverageTool()
	toolGrimpName      = py.New(nil, evidenceports.ExtractConfig{}).CoverageTool()
	toolGoPackagesName = goextract.New(evidenceports.ExtractConfig{}).CoverageTool()
)

// TestPartialFromUnresolvedSpecifiers_AdapterCoverageNames drives the real
// predicate with the real adapter names, so it fails on a rename on either side
// instead of pinning the decision package against its own copy of the literal.
// go/packages is the negative case: it sets Unresolved too, counting packages
// whose load did not complete, which must never grade comparable.
func TestPartialFromUnresolvedSpecifiers_AdapterCoverageNames(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name string
		tool string
		want bool
	}{
		{"dependency-cruiser completes with unresolved specifiers", toolDepCruiserName, true},
		{"grimp completes with unresolved specifiers", toolGrimpName, true},
		{"go/packages counts packages it could not load", toolGoPackagesName, false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got := decision.PartialFromUnresolvedSpecifiers(result.Coverage{
				Tool: tc.tool, Status: result.StatusPartial, Unresolved: 1,
			})
			if got != tc.want {
				t.Fatalf("PartialFromUnresolvedSpecifiers(tool=%q) = %v, want %v — the adapter's coverage name and the predicate's table have drifted apart",
					tc.tool, got, tc.want)
			}
		})
	}
}
