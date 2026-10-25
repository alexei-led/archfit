package evaluation_test

import (
	"fmt"
	"slices"
	"strings"
	"testing"

	"github.com/alexei-led/archfit/v3/internal/assessment/decision"
	"github.com/alexei-led/archfit/v3/internal/assessment/evaluation"
	"github.com/alexei-led/archfit/v3/internal/assessment/finding"
	"github.com/alexei-led/archfit/v3/internal/assessment/result"
	"github.com/alexei-led/archfit/v3/internal/model/evidence"
	"github.com/alexei-led/archfit/v3/internal/relationship"
)

func TestOriginExplainsGroupedMemberEvidence(t *testing.T) {
	candidates := make([]relationship.AdvisoryCandidate, 10)
	for i := range candidates {
		candidates[i] = relationship.AdvisoryCandidate{
			ID: fmt.Sprintf("member-%02d", i), RuleID: ruleBC, Severity: relationship.SeverityHigh,
			From: fmt.Sprintf("a/%02d.py", i), To: "b/private.py", FromModule: "a", ToModule: "b", EdgeKind: kindImports,
			Locations: []relationship.Location{{File: fmt.Sprintf("a/%02d.py", i), Line: 1}},
			Why:       "coupling evidence",
			MatchedBy: map[string]string{keyStrength: string(relationship.StrengthIntrusive), keyDistance: distSameOwner, keyVolatility: "high"},
		}
	}
	tests := []struct {
		name       string
		baseCount  int
		baseStatus string
		origin     finding.Origin
		wantBasis  []string
	}{
		{"identical full membership", 10, evidence.StatusOK, finding.OriginPreExisting,
			[]string{"10 of 10", "observed in base"}},
		{"new private member beyond display cap", 9, evidence.StatusOK, finding.OriginIntroduced,
			[]string{"9 of 10", "member-09", "does not imply every member", "not observed in base"}},
		{"all members unmatched and output capped", 0, evidence.StatusOK, finding.OriginIntroduced,
			[]string{"0 of 10", "10 not observed in base", "showing 8 IDs"}},
		{"missing base evidence is not introduction", 9, evidence.StatusPartial, finding.OriginUnknown,
			[]string{"9 of 10", "member-09", "cannot establish introduction"}},
		{"observed matches survive incomplete evidence", 10, evidence.StatusPartial, finding.OriginPreExisting,
			[]string{"10 of 10", "observed in base"}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			assessed := evaluation.Evaluate(evaluation.Input{AdvisoryCandidates: candidates, Accepted: acceptedSet{}, IncludeAdvisories: true, Now: evaluatedAt})
			diag := result.Result{Findings: assessed.Findings, Verdict: assessed.Verdict}
			base := evaluation.Evaluate(evaluation.Input{AdvisoryCandidates: candidates[:tc.baseCount], Accepted: acceptedSet{}, IncludeAdvisories: true, Now: evaluatedAt})
			verdict := diag.Verdict
			evaluation.AttachOrigins(&diag, evaluation.OriginInput{
				BaseRef: "base", BaseFindings: decision.BaseFindings(base.Findings),
				PrimaryTools: []string{assessGrimp},
				HeadCoverage: []evidence.Coverage{{Tool: assessGrimp, Status: evidence.StatusOK}},
				BaseCoverage: []evidence.Coverage{{Tool: assessGrimp, Status: tc.baseStatus}},
			})
			if len(diag.Findings) != 1 {
				t.Fatalf("findings = %d, want one group", len(diag.Findings))
			}
			f := diag.Findings[0]
			if f.Origin != tc.origin || f.Status != finding.StatusNew || diag.Verdict != verdict {
				t.Fatalf("origin=%s status=%s verdict=%s, want %s, new, %s", f.Origin, f.Status, diag.Verdict, tc.origin, verdict)
			}
			if len(f.Members) != 10 || len(strings.Split(f.MatchedBy["group_members"], ",")) != 8 {
				t.Fatal("fixture must compare ten members behind an eight-member display")
			}
			for _, want := range tc.wantBasis {
				if !strings.Contains(f.MatchedBy["origin_basis"], want) {
					t.Errorf("origin explanation must contain %q: matched_by=%q why=%q", want, f.MatchedBy["origin_basis"], f.Why)
				}
			}
			if !strings.Contains(f.Why, tc.wantBasis[0]) {
				t.Errorf("human summary lost member count: %s", f.Why)
			}
			introduced := []string{}
			if tc.origin == finding.OriginIntroduced {
				introduced = []string{f.ID}
			}
			if !slices.Equal(diag.Comparison.IntroducedFindingIDs, introduced) || len(diag.Comparison.ResolvedFindingIDs) != 0 {
				t.Errorf("comparison introduced=%v resolved=%v", diag.Comparison.IntroducedFindingIDs, diag.Comparison.ResolvedFindingIDs)
			}
		})
	}
}
