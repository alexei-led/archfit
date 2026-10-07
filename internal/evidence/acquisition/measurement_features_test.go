package acquisition

import (
	"context"
	"testing"

	"github.com/alexei-led/archfit/internal/extract/registry"
	"github.com/alexei-led/archfit/internal/model/evidence"
	"github.com/alexei-led/archfit/internal/scope"
	"github.com/alexei-led/archfit/internal/toolrun"
)

func TestMeasurementGoArchitectureFeaturesChangeIdentity(t *testing.T) {
	t.Setenv("GOARCH", "amd64")
	t.Setenv("GOAMD64", "v1")
	s := &Service{Runner: toolrun.New()}
	sc := scope.Scope{Root: t.TempDir()}
	rows := []evidence.Coverage{{Tool: registry.ToolGoPackages, Version: "go1.26.1", Status: evidence.StatusOK}}
	a := s.measurementProfile(context.Background(), sc, rows, nil, nil, nil)
	t.Setenv("GOAMD64", "v2")
	b := s.measurementProfile(context.Background(), sc, rows, nil, nil, nil)
	if len(a.Unknowns) != 0 || len(b.Unknowns) != 0 {
		t.Fatalf("effective Go environment unavailable: %v %v", a.Unknowns, b.Unknowns)
	}
	if a.SettingsHash == b.SettingsHash {
		t.Fatal("GOAMD64 changed build constraints without changing measurement identity")
	}
}

func TestMeasurementPartialBasisRequiresCompleteInputs(t *testing.T) {
	for _, tc := range []struct {
		row  evidence.Coverage
		want evidence.MeasurementPartialBasis
	}{
		{evidence.Coverage{Tool: registry.ToolGrimp, Status: evidence.StatusPartial, Unresolved: 1}, evidence.PartialUnresolvedSpecifiers},
		{evidence.Coverage{Tool: registry.ToolDepCruiser, Status: evidence.StatusPartial, Unresolved: 1}, evidence.PartialUnresolvedSpecifiers},
		{evidence.Coverage{Tool: registry.ToolGoPackages, Status: evidence.StatusPartial, UnresolvedPrecisionOnly: 2}, evidence.PartialDegradedPrecision},
		{evidence.Coverage{Tool: registry.ToolGoPackages, Status: evidence.StatusPartial, UnresolvedPrecisionOnly: 2, UnresolvedInputsMissing: 1}, ""},
		{evidence.Coverage{Tool: registry.ToolGrimp, Status: evidence.StatusPartial, Unresolved: 2, UnresolvedInputsMissing: 1}, ""},
		{evidence.Coverage{Tool: registry.ToolGrimp, Status: evidence.StatusPartial}, ""},
		{evidence.Coverage{Tool: registry.ToolGrimp, Status: evidence.StatusTimedOut, Unresolved: 1}, ""},
	} {
		if got := measurementPartialBasis(tc.row); got != tc.want {
			t.Fatalf("row=%+v basis=%s want=%s", tc.row, got, tc.want)
		}
	}
}
