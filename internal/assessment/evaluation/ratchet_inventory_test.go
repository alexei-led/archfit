package evaluation

import (
	"reflect"
	"strings"
	"testing"

	"github.com/alexei-led/archfit/v3/internal/assessment/result"
	"github.com/alexei-led/archfit/v3/internal/assessment/state"
	"github.com/alexei-led/archfit/v3/internal/policy"
)

const (
	ratchetCoverageMetric  = "coverage"
	ratchetCycleVersion    = "cycle.v1"
	ratchetCoverageVersion = "coverage.v1"
)

func TestMetricRatchetInventorySeparatesMeasurementFromGate(t *testing.T) {
	snapshot := result.MetricSnapshot{
		ruleTypeCycle:         {Value: 0, Version: ratchetCycleVersion},
		ratchetCoverageMetric: {Value: 1, Version: ratchetCoverageVersion},
	}
	metrics := []result.MetricResult{
		{Name: ruleTypeCycle, Value: 1, Version: ratchetCycleVersion, Delta: new(1.0), Direction: result.DirectionHigherIsWorse},
		{Name: ratchetCoverageMetric, Value: 0.8, Version: ratchetCoverageVersion, Delta: new(0.8 - 1.0), Direction: result.DirectionHigherIsBetter},
	}
	for _, tc := range []struct {
		name       string
		reference  RatchetReference
		metrics    []result.MetricResult
		wantStatus state.MetricRatchetStatus
		wantReason string
		wantCount  int
	}{
		{"comparable", RatchetReference{Present: true, Comparable: true, Metrics: snapshot}, metrics, state.MetricRatchetEvaluated, "", 2},
		{"profile drift", RatchetReference{Present: true, Metrics: snapshot, Drift: []string{"measurement_profile"}}, metrics, state.MetricRatchetUnmeasured, "measurement_profile", 2},
		{"no baseline", RatchetReference{}, metrics, "", "", 0},
		{"missing results", RatchetReference{Present: true, Comparable: true, Metrics: snapshot}, nil, state.MetricRatchetUnmeasured, "not reported", 2},
		{"missing delta", RatchetReference{Present: true, Comparable: true, Metrics: snapshot},
			[]result.MetricResult{{Name: ruleTypeCycle, Version: ratchetCycleVersion}, {Name: ratchetCoverageMetric, Version: ratchetCoverageVersion}},
			state.MetricRatchetUnmeasured, "no comparable delta", 2},
		{"metric version drift", RatchetReference{Present: true, Comparable: true, Metrics: snapshot},
			[]result.MetricResult{{Name: ruleTypeCycle, Version: "cycle.v2"}, {Name: ratchetCoverageMetric, Version: "coverage.v2"}},
			state.MetricRatchetUnmeasured, "metric version", 2},
	} {
		t.Run(tc.name, func(t *testing.T) {
			diagnostic := result.New()
			diagnostic.Metrics = tc.metrics
			got := buildState(&diagnostic, stateInput{Ratchets: tc.reference})
			if got.MetricRatchets == nil || len(got.MetricRatchets) != tc.wantCount {
				t.Fatalf("inventory=%+v, want %d explicit checks", got.MetricRatchets, tc.wantCount)
			}
			for _, check := range got.MetricRatchets {
				if check.Status != tc.wantStatus || !strings.Contains(check.Reason, tc.wantReason) {
					t.Fatalf("check=%+v", check)
				}
				if check.Status == state.MetricRatchetUnmeasured {
					if check.Before != nil || check.After != nil || check.Delta != nil {
						t.Fatalf("unmeasured comparison invented numbers: %+v", check)
					}
				} else if check.Before == nil || check.After == nil || check.Delta == nil {
					t.Fatalf("evaluated comparison lost numbers: %+v", check)
				}
			}
			wantGate := state.HardGatePass
			if tc.reference.Present && !tc.reference.Comparable {
				wantGate = state.HardGateUnmeasured
			}
			if got.Decision.HardGates != wantGate || len(got.Blockers) != 0 {
				t.Fatal("reporting inventory created another gate decision")
			}
		})
	}
	got := RatchetReference{Present: true, Comparable: true, Metrics: snapshot}.checks(metrics, nil)
	if got[0].Metric != ratchetCoverageMetric || got[0].Threshold != "min_delta 0" || *got[0].Before != 1 || *got[0].After != 0.8 {
		t.Fatalf("ratio evidence=%+v", got[0])
	}
	if got[1].Metric != ruleTypeCycle || got[1].Threshold != "max_new 0" || *got[1].Before != 0 || *got[1].Delta != 1 {
		t.Fatalf("count evidence=%+v", got[1])
	}
}

func TestMetricRatchetInventoryUsesTheRequiredRatchetApplicability(t *testing.T) {
	snapshot := result.MetricSnapshot{
		"blast_radius": {Value: 3}, ruleTypeCycle: {Value: 0}, ratchetCoverageMetric: {Value: 1},
		"unbalanced_edge": {Value: 0}, "encapsulation": {Value: 1},
	}
	cfg := map[string]policy.MetricConfig{
		ruleTypeCycle: {Gate: "warn"}, ratchetCoverageMetric: {Gate: "off"}, "encapsulation": {Enabled: new(false)},
	}
	ref := RatchetReference{Present: true, Metrics: snapshot}
	names := make([]string, 0, len(snapshot))
	for _, check := range ref.checks(nil, cfg) {
		names = append(names, check.Metric)
	}
	if !reflect.DeepEqual(names, []string{"unbalanced_edge"}) {
		t.Fatalf("checks=%v, want only the applicable blocking ratchet", names)
	}
}
