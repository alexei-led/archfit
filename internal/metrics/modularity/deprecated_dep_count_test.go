package modularity_test

import (
	"strings"
	"testing"

	"github.com/alexei-led/archfit/internal/metrics/internal/result"
	"github.com/alexei-led/archfit/internal/metrics/modularity"
	"github.com/alexei-led/archfit/internal/model/diagnostic"
	"github.com/alexei-led/archfit/internal/model/signal"
)

const (
	kindRetract = "retract" // go.mod retract marker kind
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

func TestDeprecatedDepCountMetric_Metadata(t *testing.T) {
	m := modularity.DeprecatedDepCountMetric{}
	if m.Name() != "deprecated_dep_count" {
		t.Errorf("Name() = %q; want %q", m.Name(), "deprecated_dep_count")
	}
	if m.Version() == "" {
		t.Error("Version() is empty")
	}
}

func TestDeprecatedDepCountMetric_SingleEntryFormat(t *testing.T) {
	// Verify exact display format for a single deprecation entry.
	deps := []diagnostic.DeprecatedDep{
		{File: "go.mod", Kind: kindRetract, Subject: "github.com/foo/bar", Note: ""},
	}
	m := modularity.DeprecatedDepCountMetric{}
	got := m.Calculate(signal.CommonInput{DeprecatedDeps: deps})
	want := "1 declared deprecation marker(s): github.com/foo/bar (" + kindRetract + ")"
	if got.Display != want {
		t.Errorf("Display = %q; want %q", got.Display, want)
	}
}

func TestDeprecatedDepCountMetric_TruncationAt6(t *testing.T) {
	// Truncation: display loop breaks at i==5, so 6 entries → "+1 more".
	deps := []diagnostic.DeprecatedDep{
		{Kind: kindRetract, Subject: ccModA},
		{Kind: kindRetract, Subject: ccModB},
		{Kind: kindRetract, Subject: "pkg/c"},
		{Kind: kindRetract, Subject: "pkg/d"},
		{Kind: kindRetract, Subject: "pkg/e"},
		{Kind: kindRetract, Subject: "pkg/f"},
	}
	m := modularity.DeprecatedDepCountMetric{}
	got := m.Calculate(signal.CommonInput{DeprecatedDeps: deps})
	if !strings.Contains(got.Display, "+1 more") {
		t.Errorf("Display = %q; want it to contain %q for 6 entries", got.Display, "+1 more")
	}
}

func TestDeprecatedDepCountMetric_Some(t *testing.T) {
	deps := []diagnostic.DeprecatedDep{
		{File: "go.mod", Kind: kindRetract, Subject: "v1.0.0", Note: "bad"},
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
