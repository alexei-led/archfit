package rules_test

import (
	"testing"

	"github.com/alexei-led/archfit/v3/internal/assessment/rules"
	"github.com/alexei-led/archfit/v3/internal/relationship"
)

// langDotNet is the graph language tag of the .NET extractor.
const langDotNet = "dotnet"

// hintEdge is a dependency edge carrying an extractor strength hint, the shape
// the classifier hands the internal-access rules.
func hintEdge(from, to, kind, strength string) relationship.Edge {
	return relationship.Edge{
		FromID: from, ToID: to,
		FromPath: relationship.NodePath(from), ToPath: relationship.NodePath(to),
		Kind: kind, Language: langDotNet, Strength: relationship.Strength(strength),
	}
}

// TestInternalAccessRules_FallBackToTheExtractorHint pins the documented surface
// resolution of both internal-access rules: declared public:/internal: globs
// decide first, and only a target no declaration covers falls back to the
// extractor evidence. An intrusive strength is a use of a member the target
// keeps out of its surface — a C# internal member reached through
// InternalsVisibleTo, a Python private name, a private symbol a SCIP index
// resolves — so it is the same fact as a uses_internal edge, in every language.
func TestInternalAccessRules_FallBackToTheExtractorHint(t *testing.T) {
	const (
		importer         = "file:internal/shipping/domain/shipment.cs"
		undeclaredTarget = "package:vendor/lib/thing"
		publicTarget     = "package:internal/billing/api"
		tsImporter       = "file:packages/web/src/app.ts"
		tsInternalFile   = "file:packages/core/src/internal/cache.ts"
	)
	tests := []struct {
		name     string
		edge     relationship.Edge
		want     int
		wantGlob string
		wantSig  string
	}{
		{
			name: "intrusive hint with no declared surface fires",
			edge: hintEdge(importer, undeclaredTarget, edgeKindImports, "intrusive"),
			want: 1, wantSig: "intrusive",
		},
		{
			name: "declared internal glob fires and names itself",
			edge: hintEdge(importer, nodeBillingDomPk, edgeKindImports, "intrusive"),
			want: 1, wantGlob: globBillingAll,
		},
		{
			name: "declared public glob wins over the intrusive hint",
			edge: hintEdge(importer, publicTarget, edgeKindImports, "intrusive"),
			want: 0,
		},
		{
			name: "non-intrusive hint on an undeclared target does not fire",
			edge: hintEdge(importer, undeclaredTarget, edgeKindImports, "functional"),
			want: 0,
		},
		{
			name: "uses_internal kind on an undeclared target still fires",
			edge: hintEdge(importer, undeclaredTarget, edgeKindUsesInternal, ""),
			want: 1, wantSig: "uses_internal",
		},
		{
			name: "a non-intrusive edge into a declared internal file still fires",
			edge: relationship.Edge{
				FromID: tsImporter, ToID: tsInternalFile,
				FromPath: "packages/web/src/app.ts", ToPath: "packages/core/src/internal/cache.ts",
				Kind: edgeKindImports, Language: langTypeScript,
			},
			want: 1, wantGlob: globTSInternal,
		},
		{
			name: "a plain go import into an undeclared target does not fire",
			edge: relationship.Edge{
				FromID: "file:internal/shipping/api/api.go", ToID: "package:internal/shipping/domain",
				FromPath: "internal/shipping/api/api.go", ToPath: "internal/shipping/domain",
				Kind: edgeKindImports, Language: langGo,
			},
			want: 0,
		},
		{
			name: "an intrusive hint inside one module does not fire",
			edge: hintEdge("file:internal/billing/api/api.go", "file:internal/billing/domain/store.cs", edgeKindImports, "intrusive"),
			want: 0,
		},
	}
	for _, ruleType := range []string{typePublicAPIOnly, typeInternalAPIAccess} {
		r := surfaceRule(t, ruleType)
		for _, tc := range tests {
			t.Run(ruleType+"/"+tc.name, func(t *testing.T) {
				findings := r.Check(relationship.Set{Edges: []relationship.Edge{tc.edge}}, rules.Evidence{})
				if len(findings) != tc.want {
					t.Fatalf("got %d findings, want %d: %+v", len(findings), tc.want, findings)
				}
				if tc.want == 0 {
					return
				}
				f := findings[0]
				if got := f.MatchedBy["internal_glob"]; got != tc.wantGlob {
					t.Errorf("matched_by.internal_glob = %q, want %q", got, tc.wantGlob)
				}
				if got := f.MatchedBy["internal_signal"]; got != tc.wantSig {
					t.Errorf("matched_by.internal_signal = %q, want %q", got, tc.wantSig)
				}
			})
		}
	}
}
