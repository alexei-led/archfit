package evaluation_test

import (
	"testing"
	"time"

	"github.com/alexei-led/archfit/internal/assessment/evaluation"
	"github.com/alexei-led/archfit/internal/assessment/finding"
	"github.com/alexei-led/archfit/internal/assessment/result"
	signal "github.com/alexei-led/archfit/internal/assessment/signals"
	"github.com/alexei-led/archfit/internal/policy"
	"github.com/alexei-led/archfit/internal/relationship"
)

type findingPopulationMetric struct{}

func (findingPopulationMetric) Name() string    { return "finding_population" }
func (findingPopulationMetric) Version() string { return "v1" }
func (findingPopulationMetric) Calculate(in signal.CollectedSignals) result.MetricResult {
	return result.MetricResult{Name: "finding_population", Value: float64(len(in.Common.Findings))}
}

func TestEvaluateAcceptedAdvisoryLifecycle(t *testing.T) {
	const nativeID = "native-advisory"
	for _, tc := range []struct {
		name       string
		warnLive   bool
		nativeLive bool
	}{
		{name: "both present", warnLive: true, nativeLive: true},
		{name: "warn rule removed", nativeLive: true},
		{name: "native advisory removed", warnLive: true},
		{name: "both removed"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			in := evaluation.Input{
				Accepted: acceptedSet{
					{Fingerprint: fpWarnAdv, RuleID: ruleWarn, Kind: finding.KindAdvisory},
					{Fingerprint: nativeID, RuleID: ruleBC, Kind: finding.KindAdvisory},
				},
				Metrics:           evaluation.MetricsetOf(findingPopulationMetric{}),
				IncludeAdvisories: true, Delta: true, Now: evaluatedAt,
			}
			if tc.warnLive {
				in.Rules = evaluation.RulesetOf(stubRule{id: ruleWarn, findings: []finding.Finding{warnRuleFinding(fpWarnAdv)}})
			}
			if tc.nativeLive {
				in.AdvisoryCandidates = []relationship.AdvisoryCandidate{{ID: nativeID, RuleID: ruleBC, Severity: relationship.SeverityHigh}}
			}
			got := evaluation.Evaluate(in)
			if len(got.Findings) != 2 {
				t.Errorf("findings = %+v, want exactly one finding per accepted ID", got.Findings)
			}
			active := 0
			for id, live := range map[string]bool{fpWarnAdv: tc.warnLive, nativeID: tc.nativeLive} {
				want := finding.StatusFixed
				if live {
					want = finding.StatusBaseline
					active++
				}
				count := 0
				for _, f := range got.Findings {
					if f.ID == id {
						count++
						if f.Status != want || f.Kind != finding.KindAdvisory {
							t.Errorf("finding %s = %s/%s, want advisory/%s", id, f.Kind, f.Status, want)
						}
					}
				}
				if count != 1 {
					t.Errorf("finding %s emitted %d times, want once", id, count)
				}
			}
			if got.Warnings != active || got.GateFindings != 0 {
				t.Errorf("warnings/gate findings = %d/%d, want %d/0", got.Warnings, got.GateFindings, active)
			}
			if got.Delta == nil || len(got.Delta.Existing) != active || len(got.Delta.Resolved) != 2-active {
				t.Errorf("delta = %+v, want %d existing and %d resolved", got.Delta, active, 2-active)
			}
			wantMetric := 0.0
			if tc.warnLive {
				wantMetric = 1
			}
			if got.Metrics[0].Value != wantMetric {
				t.Errorf("metric finding population = %g, want rule findings only: %g", got.Metrics[0].Value, wantMetric)
			}
			in.IncludeAdvisories = false
			hidden := evaluation.Evaluate(in)
			if len(hidden.Findings) != 0 || hidden.Warnings != 0 || hidden.Delta != nil {
				t.Errorf("hidden advisories leaked: %+v", hidden)
			}
			if hidden.Verdict != got.Verdict || hidden.Metrics[0].Value != got.Metrics[0].Value {
				t.Errorf("advisory visibility changed verdict or metrics: visible=%+v hidden=%+v", got, hidden)
			}
		})
	}
}

func TestEvaluateAdvisoryWaiverExpiry(t *testing.T) {
	for _, tc := range []struct {
		name   string
		expiry time.Time
		want   finding.Status
	}{
		{name: "valid through expiry day", expiry: evaluatedAt, want: finding.StatusWaived},
		{name: "expired", expiry: evaluatedAt.Add(-24 * time.Hour), want: finding.StatusExpiredWaiver},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got := evaluation.Evaluate(evaluation.Input{
				Rules:              evaluation.RulesetOf(stubRule{id: ruleWarn, findings: []finding.Finding{warnRuleFinding("waived-warn-advisory")}}),
				AdvisoryCandidates: []relationship.AdvisoryCandidate{{ID: "native-advisory", RuleID: ruleBC, Severity: relationship.SeverityHigh}},
				Policy:             policy.AssessmentPolicy{Waivers: policy.WaiverSet{Waivers: []policy.WaiverDef{{Expires: tc.expiry.Format("2006-01-02")}}}},
				Accepted:           acceptedSet{}, IncludeAdvisories: true, Now: evaluatedAt,
			})
			if len(got.Findings) != 2 {
				t.Fatalf("findings = %+v, want both advisory sources", got.Findings)
			}
			for _, f := range got.Findings {
				if f.Status != tc.want {
					t.Errorf("%s status = %s, want %s", f.ID, f.Status, tc.want)
				}
			}
			if got.GateFindings != 0 || got.Verdict != result.VerdictWarn {
				t.Errorf("advisory waiver changed gate behavior: %+v", got)
			}
		})
	}
}

func TestEvaluateRemovedCouplingAdvisoriesKeepEachAcceptedID(t *testing.T) {
	accepted := acceptedSet{
		{Fingerprint: "removed-bc-a", RuleID: ruleBC, Kind: finding.KindAdvisory},
		{Fingerprint: "removed-bc-b", RuleID: ruleBC, Kind: finding.KindAdvisory},
	}
	got := evaluation.Evaluate(evaluation.Input{
		Accepted: accepted, IncludeAdvisories: true, Delta: true, Now: evaluatedAt,
	})
	if len(got.Findings) != len(accepted) {
		t.Errorf("findings = %+v, want one fixed finding per accepted ID", got.Findings)
	}
	for _, entry := range accepted {
		count := 0
		for _, f := range got.Findings {
			if f.ID == entry.Fingerprint {
				count++
				if f.Status != finding.StatusFixed || f.Kind != finding.KindAdvisory {
					t.Errorf("finding %s = %s/%s, want advisory/fixed", f.ID, f.Kind, f.Status)
				}
			}
		}
		if count != 1 {
			t.Errorf("accepted ID %s emitted %d times, want once", entry.Fingerprint, count)
		}
	}
	if got.Delta == nil || len(got.Delta.Resolved) != len(accepted) {
		t.Errorf("delta = %+v, want both accepted IDs resolved", got.Delta)
	}
	if got.Warnings != 0 || got.GateFindings != 0 || got.Verdict != result.VerdictPass {
		t.Errorf("removed advisories affect decision: %+v", got)
	}
}
