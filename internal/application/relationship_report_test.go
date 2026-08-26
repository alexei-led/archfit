package application

import (
	"strings"
	"testing"

	"github.com/alexei-led/archfit/internal/assessment/result"
	modevidence "github.com/alexei-led/archfit/internal/model/evidence"
	"github.com/alexei-led/archfit/internal/policy"
)

const (
	distanceGlobA = "a/**"
	distanceGlobB = "b/**"

	wantOwnerModelNoOwner               = "no_owner_signal"
	wantOwnerModelSingleOwnerDegenerate = "single_owner_degenerate"
	wantOwnerModelMultiOwner            = "multi_owner"
)

func distancePolicy(modules map[string]policy.ModuleDef, external map[string]policy.ExternalSystemDef) policy.PolicySnapshot {
	topology := policy.TopologyView{Modules: modules, ModuleMap: policy.BuildModuleMap(modules), ExternalSystems: external}
	return policy.New(topology, policy.RelationshipPolicy{}, policy.AssessmentPolicy{}, policy.GatePolicy{}, nil, nil)
}

func TestBuildDistanceContextSingleOwnerDegenerate(t *testing.T) {
	d := result.New()
	d.ClassifiedEdges = &result.ClassifiedEdgeSummary{
		ByDistanceBasis: map[string]int{"code_structure": 3, "deploy_unit": 1},
	}
	snapshot := distancePolicy(map[string]policy.ModuleDef{
		"a": {Paths: []string{distanceGlobA}, Owner: "team"},
		"b": {Paths: []string{distanceGlobB}, Owner: "team"},
	}, nil)

	got := buildDistanceContext(d, snapshot, 1)
	if got.OwnerModel != wantOwnerModelSingleOwnerDegenerate {
		t.Fatalf("owner_model = %q, want %q", got.OwnerModel, wantOwnerModelSingleOwnerDegenerate)
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

func TestBuildDistanceContextNoOwnerSignal(t *testing.T) {
	snapshot := distancePolicy(map[string]policy.ModuleDef{
		"a": {Paths: []string{distanceGlobA}},
		"b": {Paths: []string{distanceGlobB}},
	}, nil)

	got := buildDistanceContext(result.New(), snapshot, 0)
	if got.OwnerModel != wantOwnerModelNoOwner {
		t.Fatalf("owner_model = %q, want %q", got.OwnerModel, wantOwnerModelNoOwner)
	}
	if !strings.Contains(got.Interpretation, "ownership is absent or unresolved") {
		t.Fatalf("interpretation = %q, want absent-ownership explanation", got.Interpretation)
	}
	if strings.Contains(got.Interpretation, "can still raise distance") {
		t.Fatalf("interpretation should omit the evidence suffix with no evidence: %q", got.Interpretation)
	}
}

func TestBuildDistanceContextNoOwnerWithExternalEvidence(t *testing.T) {
	snapshot := distancePolicy(
		map[string]policy.ModuleDef{"a": {Paths: []string{distanceGlobA}}},
		map[string]policy.ExternalSystemDef{"stripe": {Targets: []string{"stripe.**"}}},
	)

	got := buildDistanceContext(result.New(), snapshot, 0)
	if got.OwnerModel != wantOwnerModelNoOwner {
		t.Fatalf("owner_model = %q, want %q", got.OwnerModel, wantOwnerModelNoOwner)
	}
	if got.DeclaredExternalSystems != 1 {
		t.Fatalf("declared_external_systems = %d, want 1", got.DeclaredExternalSystems)
	}
	if !strings.Contains(got.Interpretation, "can still raise distance") {
		t.Fatalf("interpretation should include the evidence suffix: %q", got.Interpretation)
	}
}

func TestBuildDistanceContextRuntimeAsyncEvidence(t *testing.T) {
	d := result.New()
	d.RuntimeAsyncEdges = []modevidence.RuntimeAsyncEdge{
		{FromModule: "a", Target: "rabbitmq", IntegrationKind: "message_queue", Count: 2},
		{FromModule: "b", Target: "pubsub", IntegrationKind: "event_bus", Count: 1},
		{FromModule: "c", Target: "celery", IntegrationKind: "async_task", Count: 1},
		{FromModule: "a", Target: "kafka", IntegrationKind: "message_queue", Count: 1},
	}
	snapshot := distancePolicy(map[string]policy.ModuleDef{
		"a": {Paths: []string{distanceGlobA}},
		"b": {Paths: []string{distanceGlobB}},
	}, nil)

	got := buildDistanceContext(d, snapshot, 0)
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

func TestBuildDistanceContextMultiOwner(t *testing.T) {
	const (
		ownerTeamA = "team-a"
		ownerTeamB = "team-b"
	)
	snapshot := distancePolicy(map[string]policy.ModuleDef{
		"a": {Paths: []string{distanceGlobA}, Owner: ownerTeamA},
		"b": {Paths: []string{distanceGlobB}, Owner: ownerTeamB},
	}, nil)

	got := buildDistanceContext(result.New(), snapshot, 0)
	if got.OwnerModel != wantOwnerModelMultiOwner {
		t.Fatalf("owner_model = %q, want %q", got.OwnerModel, wantOwnerModelMultiOwner)
	}
	if !strings.Contains(got.Interpretation, "different-owner") {
		t.Fatalf("interpretation = %q, want different-owner explanation", got.Interpretation)
	}
}
