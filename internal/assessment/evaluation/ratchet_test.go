package evaluation_test

import (
	"strings"
	"testing"

	"github.com/alexei-led/archfit/v3/internal/assessment/evaluation"
	"github.com/alexei-led/archfit/v3/internal/assessment/finding"
	"github.com/alexei-led/archfit/v3/internal/assessment/result"
	"github.com/alexei-led/archfit/v3/internal/policy"
)

const (
	metricCoverageName = "coverage"
	metricBlastRadius  = "blast_radius"
)

func TestRatchetFindingCarriesBeforeAfterAndAWaiverStableID(t *testing.T) {
	run := func(delta float64, waivers policy.WaiverSet) finding.Finding {
		t.Helper()
		got := evaluation.Evaluate(evaluation.Input{
			Metrics:  evaluation.MetricsetOf(stubMetric{res: metricValue(&delta, result.DirectionHigherIsWorse)}),
			Gates:    map[string]policy.MetricConfig{},
			Accepted: acceptedSet{}, Now: evaluatedAt,
			Policy: policy.AssessmentPolicy{Waivers: waivers},
		})
		for _, f := range got.Findings {
			if f.RuleID == "metric/"+metricName {
				return f
			}
		}
		t.Fatalf("no ratchet finding for delta %v: %+v", delta, got.Findings)
		return finding.Finding{}
	}
	first, second := run(1, policy.WaiverSet{}), run(3, policy.WaiverSet{})
	if first.ID != second.ID {
		t.Errorf("ratchet ID moved with the value (%s vs %s): a waiver could not keep matching", first.ID, second.ID)
	}
	if first.Kind != finding.KindGate || first.Status != finding.StatusNew {
		t.Errorf("finding = kind %q status %q, want a new gate", first.Kind, first.Status)
	}
	for key, want := range map[string]string{"metric": metricName, "delta": "1", "threshold": "max_new 0"} {
		if first.MatchedBy[key] != want {
			t.Errorf("matched_by[%s] = %q, want %q", key, first.MatchedBy[key], want)
		}
	}
	if !strings.Contains(first.Why, "worsened from") || !strings.Contains(first.Constraint, "archfit baseline") {
		t.Errorf("why/constraint do not carry the values and the do-not-baseline rule: %q / %q", first.Why, first.Constraint)
	}
}

func TestRatchetWaiverMatchesWithoutFromOrTo(t *testing.T) {
	delta := 2.0
	run := func(waivers policy.WaiverSet) finding.Status {
		t.Helper()
		got := evaluation.Evaluate(evaluation.Input{
			Metrics:  evaluation.MetricsetOf(stubMetric{res: metricValue(&delta, result.DirectionHigherIsWorse)}),
			Gates:    map[string]policy.MetricConfig{},
			Accepted: acceptedSet{}, Now: evaluatedAt,
			Policy: policy.AssessmentPolicy{Waivers: waivers},
		})
		for _, f := range got.Findings {
			if f.RuleID == "metric/"+metricName {
				return f.Status
			}
		}
		t.Fatalf("no ratchet finding: %+v", got.Findings)
		return ""
	}
	waiver := policy.WaiverSet{Waivers: []policy.WaiverDef{{Rule: "metric/" + metricName, Reason: "accepted", ApprovedBy: "@owner", Expires: "2099-01-01"}}}
	if got := run(waiver); got != finding.StatusWaived {
		t.Errorf("waived ratchet status = %q, want waived", got)
	}
	expired := policy.WaiverSet{Waivers: []policy.WaiverDef{{Rule: "metric/" + metricName, Reason: "accepted", ApprovedBy: "@owner", Expires: "2020-01-01"}}}
	if got := run(expired); got != finding.StatusExpiredWaiver {
		t.Errorf("ratchet with an expired waiver = %q, want expired_waiver", got)
	}
	if got := run(policy.WaiverSet{}); got != finding.StatusNew {
		t.Errorf("unwaived ratchet status = %q, want new", got)
	}
}

func TestRatchetDeltaHidesSubtractionNoise(t *testing.T) {
	delta := 0.7 - 0.9
	got := evaluation.RatchetFindings([]result.MetricResult{metricValue(&delta, result.DirectionHigherIsBetter)}, map[string]policy.MetricConfig{})
	if len(got) != 1 || got[0].MatchedBy["delta"] != "-0.2" {
		t.Errorf("ratchet findings = %+v, want one with delta -0.2", got)
	}
}

func TestRatchetReferenceUnmeasuredOnlyWhenItCannotDecide(t *testing.T) {
	snapshot := result.MetricSnapshot{metricCycle: {Value: 1}, metricCoverageName: {Value: 0.5}}
	tests := []struct {
		name string
		ref  evaluation.RatchetReference
		cfg  map[string]policy.MetricConfig
		want string
	}{
		{"no baseline file", evaluation.RatchetReference{Metrics: snapshot}, nil, ""},
		{"comparable reference decides", evaluation.RatchetReference{Present: true, Comparable: true, Metrics: snapshot}, nil, ""},
		{"non-comparable reference", evaluation.RatchetReference{Present: true, Metrics: snapshot, Drift: []string{"rubric_version"}}, nil, "reference not comparable (drift: rubric_version): 2 metric ratchets"},
		{"warn and off ratchets are not required", evaluation.RatchetReference{Present: true, Metrics: snapshot, Drift: []string{"model_hash"}},
			map[string]policy.MetricConfig{metricCycle: {Gate: string(policy.GateWarn)}}, "drift: model_hash): 1 metric ratchets"},
		{"nothing blocking in the snapshot", evaluation.RatchetReference{Present: true, Metrics: snapshot},
			map[string]policy.MetricConfig{metricCycle: {Gate: string(policy.GateOff)}, metricCoverageName: {Gate: string(policy.GateWarn)}}, ""},
		{"empty snapshot", evaluation.RatchetReference{Present: true}, nil, ""},
		{"blast_radius cannot ratchet", evaluation.RatchetReference{Present: true, Metrics: result.MetricSnapshot{metricBlastRadius: {Value: 3}}}, nil, ""},
		{"a switched-off metric is not counted", evaluation.RatchetReference{Present: true, Metrics: snapshot, Drift: []string{"model_hash"}},
			map[string]policy.MetricConfig{metricCycle: {Enabled: new(false)}}, "1 metric ratchets"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			reason, ok := tc.ref.Unevaluated(tc.cfg)
			if (tc.want != "") != ok || !strings.Contains(reason, tc.want) {
				t.Errorf("Unevaluated = %q, %v; want containing %q", reason, ok, tc.want)
			}
		})
	}
}
