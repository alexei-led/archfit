package analysis_test

import (
	"testing"

	"github.com/alexei-led/archfit/v3/internal/policy"
	"github.com/alexei-led/archfit/v3/internal/relationship"
	"github.com/alexei-led/archfit/v3/internal/relationship/analysis"
)

func TestAnalyzeReadsVolatilityFromPolicy(t *testing.T) {
	for _, tc := range []struct {
		name, subdomain, explicit string
		want                      relationship.Volatility
	}{
		{"core maps to high", subdomainCore, "", relationship.VolatilityHigh},
		{"supporting maps to low", subdomainSupporting, "", relationship.VolatilityLow},
		{"generic maps to low", "generic", "", relationship.VolatilityLow},
		{"explicit overrides subdomain", subdomainCore, "medium", relationship.VolatilityMedium},
		{"undeclared", "", "", relationship.VolatilityUndeclared},
	} {
		t.Run(tc.name, func(t *testing.T) {
			modules := map[string]policy.ModuleDef{
				moduleA: {Paths: []string{globA}, Volatility: volHigh},
				moduleB: {Paths: []string{globB}, Subdomain: tc.subdomain, Volatility: tc.explicit},
			}
			got := analysis.Analyze(analysis.Input{
				Graph:  graphWith(string(relationship.StrengthModel)),
				Policy: relationshipPolicy(modules),
			})
			edge := onlyEdge(t, got)
			if edge.Strength != relationship.StrengthModel || edge.Volatility != tc.want {
				t.Fatalf("classification = %s/%s, want model/%s", edge.Strength, edge.Volatility, tc.want)
			}
			if len(got.Assessment.Seams) != 1 || got.Assessment.Seams[0].Volatility != tc.want {
				t.Fatalf("policy volatility lost on seam: %+v", got.Assessment.Seams)
			}
		})
	}
}
