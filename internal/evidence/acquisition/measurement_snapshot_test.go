package acquisition

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/alexei-led/archfit/internal/extract/registry"
	"github.com/alexei-led/archfit/internal/model/evidence"
	"github.com/alexei-led/archfit/internal/scope"
)

func TestMeasurementProfileUsesConsumedDynamicConfiguration(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, ".dependency-cruiser.cjs"), []byte("module.exports = {}"), 0o600); err != nil {
		t.Fatal(err)
	}
	s := &Service{}
	row := evidence.Coverage{Tool: registry.ToolDepCruiser, Version: "17.4.3", Status: evidence.StatusOK, MeasurementSettingsHash: "consumed-options"}
	p := s.measurementProfile(context.Background(), scope.Scope{Root: root}, []evidence.Coverage{row}, nil)
	if len(p.Unknowns) != 0 {
		t.Fatalf("consumed dynamic config ignored: %v", p.Unknowns)
	}
	row.MeasurementSettingsHash = ""
	unknown := s.measurementProfile(context.Background(), scope.Scope{Root: root}, []evidence.Coverage{row}, nil)
	if len(unknown.Unknowns) == 0 {
		t.Fatal("unresolved dynamic config was accepted without an execution snapshot")
	}
}
