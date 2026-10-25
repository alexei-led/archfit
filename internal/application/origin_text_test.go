package application

import (
	"strings"
	"testing"

	"github.com/alexei-led/archfit/v3/internal/assessment/decision"
	"github.com/alexei-led/archfit/v3/internal/assessment/evaluation"
	"github.com/alexei-led/archfit/v3/internal/assessment/finding"
	"github.com/alexei-led/archfit/v3/internal/assessment/result"
	"github.com/alexei-led/archfit/v3/internal/assessment/score"
	"github.com/alexei-led/archfit/v3/internal/model/evidence"
)

func TestProjectedOriginSummarySurvivesTextBounds(t *testing.T) {
	const oldMemberID = "origin-old-member"
	for _, status := range []string{evidence.StatusOK, evidence.StatusPartial} {
		t.Run(status, func(t *testing.T) {
			r := result.New()
			r.Findings = []finding.Finding{{
				ID: oldMemberID, RuleID: finding.RuleIDBCImbalanced, Kind: finding.KindAdvisory, Status: finding.StatusNew,
				Members: []string{oldMemberID, "new"}, Why: strings.Repeat("original coupling explanation ", 30),
			}}
			evaluation.AttachOrigins(&r, evaluation.OriginInput{
				BaseRef: "base", BaseFindings: []decision.BaseFinding{{ID: oldMemberID, Members: []string{oldMemberID}}},
				PrimaryTools: []string{textToolGrimp},
				HeadCoverage: []evidence.Coverage{{Tool: textToolGrimp, Status: evidence.StatusOK}},
				BaseCoverage: []evidence.Coverage{{Tool: textToolGrimp, Status: status}},
			})
			doc := ProjectReport(r, score.Scorecard{})
			why := doc.State.Findings[0].Why
			want := "does not imply every member or displayed site is new"
			if status == evidence.StatusPartial {
				want = "cannot establish introduction"
			}
			for _, text := range []string{"1 of 2", want, "original coupling explanation"} {
				if !strings.Contains(why, text) {
					t.Errorf("projected why lost %q: %s", text, why)
				}
			}
		})
	}
}
