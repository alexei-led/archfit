package evaluation_test

import (
	"strings"
	"testing"

	"github.com/alexei-led/archfit/internal/assessment/evaluation"
	"github.com/alexei-led/archfit/internal/assessment/finding"
	"github.com/alexei-led/archfit/internal/assessment/result"
	"github.com/alexei-led/archfit/internal/policy"
)

const metricCoverageName = "coverage"

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
