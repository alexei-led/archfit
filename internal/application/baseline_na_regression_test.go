package application

import (
	"context"
	"testing"

	"github.com/alexei-led/archfit/v3/internal/model/report"
)

const (
	baselineTestEncapsulation = "encapsulation"
	baselineTestCycle         = "cycle"
)

func TestBaselineCaptureOmitsIndeterminateMetric(t *testing.T) {
	writer := &baselineTestWriter{}
	service := baselineTestService("2099-01-01", writer)
	assessed, err := service.Stages.Execute(context.Background(), AnalysisRequest{EmptyBaseline: true})
	if err != nil {
		t.Fatalf("assess fixture: %v", err)
	}
	found := false
	for _, m := range assessed.Diagnostic.Metrics {
		if m.Name != baselineTestEncapsulation {
			continue
		}
		found = true
		if m.Band != string(report.ScoreBandNA) || m.Delta != nil {
			t.Fatalf("fixture must produce indeterminate encapsulation: %+v", m)
		}
		t.Logf("before capture: %s band=%q value=%g version=%s delta=%v", m.Name, m.Band, m.Value, m.Version, m.Delta)
	}
	if !found {
		t.Fatal("fixture did not produce encapsulation metric")
	}
	if _, err := service.Execute(context.Background(), BaselineRequest{Path: baselineTestPath}); err != nil {
		t.Fatalf("capture: %v", err)
	}
	if got, persisted := writer.snapshot.Metrics[baselineTestEncapsulation]; persisted {
		t.Fatalf("n/a encapsulation persisted as numeric baseline: value=%g version=%s; want absent, not placeholder zero", got.Value, got.Version)
	}
}

func TestBaselineCaptureRetainsMeasuredZero(t *testing.T) {
	writer := &baselineTestWriter{}
	service := baselineTestService("2099-01-01", writer)
	assessed, err := service.Stages.Execute(context.Background(), AnalysisRequest{EmptyBaseline: true})
	if err != nil {
		t.Fatalf("assess fixture: %v", err)
	}
	version := ""
	for _, m := range assessed.Diagnostic.Metrics {
		if m.Name != baselineTestCycle {
			continue
		}
		if m.Band == string(report.ScoreBandNA) || m.Value != 0 {
			t.Fatalf("fixture must produce a measured zero cycle count: %+v", m)
		}
		version = m.Version
		t.Logf("before capture: %s band=%q value=%g version=%s", m.Name, m.Band, m.Value, m.Version)
	}
	if version == "" {
		t.Fatal("fixture did not produce a measured cycle metric")
	}
	if _, err := service.Execute(context.Background(), BaselineRequest{Path: baselineTestPath}); err != nil {
		t.Fatalf("capture: %v", err)
	}
	got, persisted := writer.snapshot.Metrics[baselineTestCycle]
	if !persisted || got.Value != 0 || got.Version != version {
		t.Fatalf("measured zero lost: persisted=%v value=%g version=%s, want value0 version%s", persisted, got.Value, got.Version, version)
	}
	t.Logf("after capture: measured cycle zero retained at %s", got.Version)
}
