// Regression tests for assessment decisions whose only observable effect is in
// the rendered report: advisory visibility, the warnings count, endpoint module
// labels, and the paths a rolled-up advisory reports.
package evaluation_test

import (
	"testing"

	"github.com/alexei-led/archfit/v3/internal/assessment/evaluation"
	"github.com/alexei-led/archfit/v3/internal/relationship"
)

const (
	moduleA   = "a"
	moduleB   = "b"
	ruleWarn  = "warn_rule"
	fpWarnAdv = "11111111111111111111111111111111"
	fpGate    = "22222222222222222222222222222222"
)

// TestEvaluateRolledUpAdvisoryReportsTheFirstLocationsOwner pins that a
// rolled-up advisory's from/to paths name the member owning locations[0], so
// the reported paths and the first reported location agree. Last-match wins
// made the pair point at a different member's file.
func TestEvaluateRolledUpAdvisoryReportsTheFirstLocationsOwner(t *testing.T) {
	shared := relationship.Location{File: "a/first.go", Line: 1}
	member := func(id, from, to string, locs ...relationship.Location) relationship.AdvisoryCandidate {
		return relationship.AdvisoryCandidate{
			ID: id, RuleID: ruleBC, Severity: relationship.SeverityHigh,
			From: from, To: to, FromModule: moduleA, ToModule: moduleB, EdgeKind: kindImports,
			Locations: locs,
			MatchedBy: map[string]string{keyStrength: strFunctional, keyDistance: distSameOwner, keyVolatility: volLow},
		}
	}
	// Both members carry the shared first-sorting location; only the earlier ID
	// is the representative, and its paths must win.
	got := evaluation.Evaluate(evaluation.Input{
		AdvisoryCandidates: []relationship.AdvisoryCandidate{
			member("adv-1", "a/first.go", "b/first.go", shared),
			member("adv-2", "a/second.go", "b/second.go", shared, relationship.Location{File: "a/zzz.go", Line: 9}),
		},
		IncludeAdvisories: true, Now: evaluatedAt,
	})
	rep := findByID(t, got.Findings, "adv-1")
	if len(rep.Locations) == 0 || rep.Locations[0] != shared {
		t.Fatalf("rolled-up locations = %v, want %v first", rep.Locations, shared)
	}
	if rep.Edge.From.Path != "a/first.go" || rep.Edge.To.Path != "b/first.go" {
		t.Errorf("rolled-up paths = %q -> %q, want the member owning locations[0]",
			rep.Edge.From.Path, rep.Edge.To.Path)
	}
}

func TestEvaluatePreservesAdvisorySeverity(t *testing.T) {
	t.Parallel()
	for _, severity := range []relationship.Severity{
		relationship.SeverityCritical, relationship.SeverityHigh,
		relationship.SeverityMedium, relationship.SeverityLow,
	} {
		t.Run(string(severity), func(t *testing.T) {
			t.Parallel()
			candidate := relationship.AdvisoryCandidate{
				ID: "advisory", RuleID: ruleBC, Severity: severity,
				From: pathA, To: pathB, FromModule: moduleA, ToModule: moduleB, EdgeKind: kindImports,
				MatchedBy: map[string]string{"score_band": string(severity)},
			}
			got := evaluation.Evaluate(evaluation.Input{
				Relationships: relationship.Set{Edges: []relationship.Edge{{
					FromPath: pathA, ToPath: pathB, Kind: kindImports,
					Strength: relationship.StrengthFunctional, Distance: relationship.DistanceCrossModule,
				}}},
				AdvisoryCandidates: []relationship.AdvisoryCandidate{candidate}, IncludeAdvisories: true,
			})
			if len(got.Findings) != 1 {
				t.Fatalf("findings = %+v, want one advisory", got.Findings)
			}
			f := got.Findings[0]
			if string(f.Severity) != string(severity) || f.MatchedBy["score_band"] != string(severity) {
				t.Fatalf("severity = %s, score band = %s, want %s unchanged", f.Severity, f.MatchedBy["score_band"], severity)
			}
		})
	}
}
