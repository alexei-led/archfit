package main

import (
	"strings"
	"testing"

	"github.com/alexei-led/archfit/internal/config"
	"github.com/alexei-led/archfit/internal/model/diagnostic"
)

func TestBuildDistanceContext_SingleOwnerDegenerate(t *testing.T) {
	d := diagnostic.New()
	d.ClassifiedEdges = &diagnostic.ClassifiedEdgeSummary{
		ByDistanceBasis: map[string]int{"code_structure": 3, "deploy_unit": 1},
	}
	cfg := config.Config{Modules: map[string]config.ModuleDef{
		"a": {Paths: []string{"a/**"}, Owner: "team"},
		"b": {Paths: []string{"b/**"}, Owner: "team"},
	}}

	got := buildDistanceContext(d, cfg, 1)
	if got.OwnerModel != ownerModelSingleOwnerDegenerate {
		t.Fatalf("owner_model = %q, want %q", got.OwnerModel, ownerModelSingleOwnerDegenerate)
	}
	if got.DistanceBasis["code_structure"] != 3 || got.DistanceBasis["deploy_unit"] != 1 {
		t.Fatalf("distance_basis = %+v", got.DistanceBasis)
	}
	if got.DeployUnitDetectedModules != 1 {
		t.Fatalf("deploy_unit_detected_modules = %d, want 1", got.DeployUnitDetectedModules)
	}
	if !strings.Contains(got.Interpretation, "low socio-technical distance") {
		t.Fatalf("interpretation does not explain low-distance one-owner repos: %q", got.Interpretation)
	}
}

func TestBuildDistanceContext_MultiOwner(t *testing.T) {
	cfg := config.Config{Modules: map[string]config.ModuleDef{
		"a": {Paths: []string{"a/**"}, Owner: "team-a"},
		"b": {Paths: []string{"b/**"}, Owner: "team-b"},
	}}

	got := buildDistanceContext(diagnostic.New(), cfg, 0)
	if got.OwnerModel != ownerModelMultiOwner {
		t.Fatalf("owner_model = %q, want %q", got.OwnerModel, ownerModelMultiOwner)
	}
	if !strings.Contains(got.Interpretation, "different-owner") {
		t.Fatalf("interpretation = %q, want different-owner explanation", got.Interpretation)
	}
}
