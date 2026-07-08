package main

import (
	"strings"
	"testing"

	"github.com/alexei-led/archfit/internal/config"
	"github.com/alexei-led/archfit/internal/model/diagnostic"
	"github.com/alexei-led/archfit/internal/model/module"
)

const (
	globA = "a/**"
	globB = "b/**"
)

func TestBuildDistanceContext_SingleOwnerDegenerate(t *testing.T) {
	d := diagnostic.New()
	d.ClassifiedEdges = &diagnostic.ClassifiedEdgeSummary{
		ByDistanceBasis: map[string]int{"code_structure": 3, "deploy_unit": 1},
	}
	cfg := config.Config{Modules: map[string]module.ModuleDef{
		"a": {Paths: []string{globA}, Owner: "team"},
		"b": {Paths: []string{globB}, Owner: "team"},
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

func TestBuildDistanceContext_NoOwnerSignal(t *testing.T) {
	cfg := config.Config{Modules: map[string]module.ModuleDef{
		"a": {Paths: []string{globA}},
		"b": {Paths: []string{globB}},
	}}

	got := buildDistanceContext(diagnostic.New(), cfg, 0)
	if got.OwnerModel != ownerModelNoOwner {
		t.Fatalf("owner_model = %q, want %q", got.OwnerModel, ownerModelNoOwner)
	}
	if !strings.Contains(got.Interpretation, "ownership is absent or unresolved") {
		t.Fatalf("interpretation = %q, want absent-ownership explanation", got.Interpretation)
	}
	// No deploy-unit or declared-external evidence → no "can still raise distance" suffix.
	if strings.Contains(got.Interpretation, "can still raise distance") {
		t.Fatalf("interpretation should omit the evidence suffix with no evidence: %q", got.Interpretation)
	}
}

func TestBuildDistanceContext_NoOwnerWithExternalEvidence(t *testing.T) {
	cfg := config.Config{
		Modules:         map[string]module.ModuleDef{"a": {Paths: []string{globA}}},
		ExternalSystems: map[string]config.ExternalSystemDef{"stripe": {Targets: []string{"stripe.**"}}},
	}

	got := buildDistanceContext(diagnostic.New(), cfg, 0)
	if got.OwnerModel != ownerModelNoOwner {
		t.Fatalf("owner_model = %q, want %q", got.OwnerModel, ownerModelNoOwner)
	}
	if got.DeclaredExternalSystems != 1 {
		t.Fatalf("declared_external_systems = %d, want 1", got.DeclaredExternalSystems)
	}
	// Declared external systems → suffix disclosing distance can still be raised.
	if !strings.Contains(got.Interpretation, "can still raise distance") {
		t.Fatalf("interpretation should include the evidence suffix: %q", got.Interpretation)
	}
}

func TestBuildDistanceContext_RuntimeAsyncEvidence(t *testing.T) {
	d := diagnostic.New()
	d.RuntimeAsyncEdges = []diagnostic.RuntimeAsyncEdge{
		{FromModule: "a", Target: "rabbitmq", IntegrationKind: "message_queue", Count: 2},
		{FromModule: "b", Target: "pubsub", IntegrationKind: "event_bus", Count: 1},
		{FromModule: "c", Target: "celery", IntegrationKind: "async_task", Count: 1},
		{FromModule: "a", Target: "kafka", IntegrationKind: "message_queue", Count: 1},
	}
	cfg := config.Config{Modules: map[string]module.ModuleDef{
		"a": {Paths: []string{globA}},
		"b": {Paths: []string{globB}},
	}}

	got := buildDistanceContext(d, cfg, 0)
	if got.RuntimeAsyncRelations != 4 {
		t.Fatalf("runtime_async_relations = %d, want 4", got.RuntimeAsyncRelations)
	}
	if got.RuntimeAsyncKinds["message_queue"] != 2 || got.RuntimeAsyncKinds["event_bus"] != 1 || got.RuntimeAsyncKinds["async_task"] != 1 {
		t.Fatalf("runtime_async_kinds = %+v", got.RuntimeAsyncKinds)
	}
	if !strings.Contains(got.RuntimeInterpretation, "reduce lifecycle coupling") {
		t.Fatalf("runtime_interpretation = %q", got.RuntimeInterpretation)
	}
}

func TestBuildDistanceContext_MultiOwner(t *testing.T) {
	const (
		ownerTeamA = "team-a"
		ownerTeamB = "team-b"
	)
	cfg := config.Config{Modules: map[string]module.ModuleDef{
		"a": {Paths: []string{globA}, Owner: ownerTeamA},
		"b": {Paths: []string{globB}, Owner: ownerTeamB},
	}}

	got := buildDistanceContext(diagnostic.New(), cfg, 0)
	if got.OwnerModel != ownerModelMultiOwner {
		t.Fatalf("owner_model = %q, want %q", got.OwnerModel, ownerModelMultiOwner)
	}
	if !strings.Contains(got.Interpretation, "different-owner") {
		t.Fatalf("interpretation = %q, want different-owner explanation", got.Interpretation)
	}
}
