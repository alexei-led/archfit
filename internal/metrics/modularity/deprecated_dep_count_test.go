package modularity_test

import (
	"testing"

	"github.com/alexei-led/archfit/internal/metrics/internal/result"
	"github.com/alexei-led/archfit/internal/metrics/modularity"
	"github.com/alexei-led/archfit/internal/model/diagnostic"
	"github.com/alexei-led/archfit/internal/model/signal"
)

func TestDeprecatedDepCountMetric_Zero(t *testing.T) {
	m := modularity.DeprecatedDepCountMetric{}
	got := m.Calculate(signal.CommonInput{})

	if got.Name != "deprecated_dep_count" {
		t.Errorf("name: %q", got.Name)
	}
	if got.Value != 0 {
		t.Errorf("value: got %v, want 0", got.Value)
	}
	if got.Band != result.BandInformational {
		t.Errorf("band: got %q, want %q", got.Band, result.BandInformational)
	}
	if got.Display != "0 declared deprecation markers" {
		t.Errorf("display: %q", got.Display)
	}
}

func TestDeprecatedDepCountMetric_Some(t *testing.T) {
	deps := []diagnostic.DeprecatedDep{
		{File: "go.mod", Kind: "retract", Subject: "v1.0.0", Note: "bad"},
		{File: "package.json", Kind: "deprecated", Subject: "old-pkg", Note: "use new-pkg"},
	}
	m := modularity.DeprecatedDepCountMetric{}
	got := m.Calculate(signal.CommonInput{DeprecatedDeps: deps})

	if got.Value != 2 {
		t.Errorf("value: got %v, want 2", got.Value)
	}
	if got.Band != result.BandInformational {
		t.Errorf("band: got %q, want %q", got.Band, result.BandInformational)
	}
	if got.Display == "0 declared deprecation markers" {
		t.Errorf("display should be non-zero summary, got: %q", got.Display)
	}
}
