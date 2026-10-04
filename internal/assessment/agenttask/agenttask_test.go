package agenttask_test

import (
	"encoding/json"
	"fmt"
	"reflect"
	"strings"
	"testing"

	"github.com/alexei-led/archfit/internal/assessment/agenttask"
	"github.com/alexei-led/archfit/internal/assessment/finding"
	"github.com/alexei-led/archfit/internal/model/evidence"
	"github.com/alexei-led/archfit/internal/relationship"
)

const (
	ruleForbidden     = "no_internal_access"
	fileFrom          = "pkg/a/a.go"
	fileTo            = "pkg/b/internal/impl.go"
	ruleTypeForbidden = "forbidden_dependency"
	ruleTypePublicAPI = "public_api_only"
	ruleTypeCycle     = "module_cycle"
	ruleTypeLayer     = "forbidden_layer_direction"
	validateCmd       = "archfit check"
	kindFunction      = "function"
	moduleDomain      = "domain"
)

func gateFinding(id, ruleID string, status finding.Status) finding.Finding {
	return finding.Finding{
		ID:     id,
		Kind:   "gate",
		RuleID: ruleID,
		Status: status,
		Edge: finding.EdgeEvidence{
			From: finding.Endpoint{Path: fileFrom, Module: "a"},
			To:   finding.Endpoint{Path: fileTo, Module: "b"},
			Kind: "uses_internal",
		},
		Locations:    []relationship.Location{{File: fileFrom, Line: 5}},
		Why:          "a uses b internals",
		Constraint:   "Use only the public API of module b",
		Alternatives: []string{"pkg/b/api"},
	}
}

func TestBuild_ActiveGateFindingsOnly(t *testing.T) {
	findings := []finding.Finding{
		gateFinding("f-new", ruleForbidden, finding.StatusNew),
		gateFinding("f-expired", ruleForbidden, finding.StatusExpiredWaiver),
		gateFinding("f-baselined", ruleForbidden, finding.StatusBaseline),
		gateFinding("f-fixed", ruleForbidden, finding.StatusFixed),
		func() finding.Finding {
			f := gateFinding("f-advisory", "bc/imbalanced_coupling", finding.StatusNew)
			f.Kind = "advisory"
			return f
		}(),
	}

	tasks := agenttask.Build(findings,
		map[string]string{ruleForbidden: ruleTypeForbidden},
		nil,
		[]string{"archfit check"},
		nil,
		nil,
		agenttask.PathResolver{},
	)

	if len(tasks) != 2 {
		t.Fatalf("tasks = %d, want 2 (new + expired only); got %+v", len(tasks), tasks)
	}
	// Sorted by FindingID: f-expired < f-new.
	if tasks[0].FindingID != "f-expired" || tasks[1].FindingID != "f-new" {
		t.Errorf("order = [%s, %s], want [f-expired, f-new]", tasks[0].FindingID, tasks[1].FindingID)
	}
}

// TestBuild_OneTaskPerFindingNotPerEdge pins the published cardinality: one
// import that breaks two rules is two findings with two IDs, so it is two
// tasks. Consumers key tasks by finding_id; merging them per edge is a
// contract change, not a dedup fix.
func TestBuild_OneTaskPerFindingNotPerEdge(t *testing.T) {
	forbidden := gateFinding("f-forbidden", ruleForbidden, finding.StatusNew)
	layered := gateFinding("f-layer", "layer-direction", finding.StatusNew)
	tasks := agenttask.Build([]finding.Finding{forbidden, layered},
		map[string]string{ruleForbidden: ruleTypeForbidden, "layer-direction": ruleTypeLayer},
		nil, nil, nil, nil, agenttask.PathResolver{})
	if len(tasks) != 2 || tasks[0].FindingID != "f-forbidden" || tasks[1].FindingID != "f-layer" {
		t.Fatalf("tasks = %+v, want one per finding ID", tasks)
	}
}

func TestBuild_TaskShape(t *testing.T) {
	tasks := agenttask.Build(
		[]finding.Finding{gateFinding("f1", ruleForbidden, finding.StatusNew)},
		map[string]string{ruleForbidden: ruleTypePublicAPI},
		map[string][]string{"b": {"pkg/b/api/**"}},
		[]string{"archfit check -c .archfit.yaml"},
		nil,
		nil,
		agenttask.PathResolver{},
	)
	if len(tasks) != 1 {
		t.Fatalf("tasks = %d, want 1", len(tasks))
	}
	task := tasks[0]

	if !strings.Contains(task.Goal, fileFrom) || !strings.Contains(task.Goal, "public API") {
		t.Errorf("goal = %q, want endpoints + public-API instruction", task.Goal)
	}
	wantFiles := []string{fileFrom, fileTo}
	if !reflect.DeepEqual(task.Files, wantFiles) {
		t.Errorf("files = %v, want %v", task.Files, wantFiles)
	}
	if len(task.Constraints) != 3 {
		t.Fatalf("constraints = %v, want 3 (constraint + alternative + public surface)", task.Constraints)
	}
	if task.Constraints[0] != "Use only the public API of module b" {
		t.Errorf("constraints[0] = %q", task.Constraints[0])
	}
	if !strings.Contains(task.Constraints[2], "pkg/b/api/**") {
		t.Errorf("constraints[2] = %q, want public surface globs", task.Constraints[2])
	}
	if len(task.Validation) != 1 || task.Validation[0] != "archfit check -c .archfit.yaml" {
		t.Errorf("validation = %v", task.Validation)
	}
}

func TestBuild_GoalTemplates(t *testing.T) {
	tests := []struct {
		ruleType string
		want     string
	}{
		{ruleTypeForbidden, "Remove the forbidden dependency"},
		{ruleTypePublicAPI, publicAPIText},
		{"internal_api_access", publicAPIText},
		{ruleTypeLayer, "inner layers must not import outer layers"},
		{ruleTypeNewCross, "architecture-owner decision"},
		{"cycle", "Break the import cycle"},
		{"someone_elses_rule", "a uses b internals"}, // unknown type → Why fallback
	}
	for _, tc := range tests {
		t.Run(tc.ruleType, func(t *testing.T) {
			tasks := agenttask.Build(
				[]finding.Finding{gateFinding("f1", ruleForbidden, finding.StatusNew)},
				map[string]string{ruleForbidden: tc.ruleType},
				nil, nil, nil, nil,
				agenttask.PathResolver{},
			)
			if len(tasks) != 1 {
				t.Fatalf("tasks = %d, want 1", len(tasks))
			}
			if !strings.Contains(tasks[0].Goal, tc.want) {
				t.Errorf("goal for %s = %q, want substring %q", tc.ruleType, tasks[0].Goal, tc.want)
			}
		})
	}
}

func TestBuild_EmptyAndDeterministic(t *testing.T) {
	if got := agenttask.Build(nil, nil, nil, nil, nil, nil, agenttask.PathResolver{}); got == nil || len(got) != 0 {
		t.Errorf("nil findings → %v, want empty non-nil slice", got)
	}

	findings := []finding.Finding{
		gateFinding("z", ruleForbidden, finding.StatusNew),
		gateFinding("a", ruleForbidden, finding.StatusNew),
	}
	first := agenttask.Build(findings, nil, nil, []string{validateCmd}, nil, nil, agenttask.PathResolver{})
	second := agenttask.Build(findings, nil, nil, []string{validateCmd}, nil, nil, agenttask.PathResolver{})
	if !reflect.DeepEqual(first, second) {
		t.Error("two builds differ — must be deterministic")
	}
	if first[0].FindingID != "a" || first[1].FindingID != "z" {
		t.Errorf("not sorted by FindingID: %s, %s", first[0].FindingID, first[1].FindingID)
	}
}

// TestBuild_DeclarationsEnrichedWhenSyntaxPresent verifies that when SyntaxFacts
// are provided, each task's Declarations field contains the facts for its files.
func TestBuild_DeclarationsEnrichedWhenSyntaxPresent(t *testing.T) {
	sf := []evidence.SyntaxFact{
		{File: fileFrom, Kind: kindFunction, Name: "CallB", Exported: true, StartLine: 3},
		{File: fileTo, Kind: kindFunction, Name: "internalImpl", Exported: false, StartLine: 10},
		{File: "pkg/other/other.go", Kind: kindFunction, Name: "Unrelated", Exported: true, StartLine: 1},
	}

	tasks := agenttask.Build(
		[]finding.Finding{gateFinding("f1", ruleForbidden, finding.StatusNew)},
		map[string]string{ruleForbidden: "forbidden_dependency"},
		nil,
		[]string{validateCmd},
		sf,
		nil,
		agenttask.PathResolver{},
	)
	if len(tasks) != 1 {
		t.Fatalf("tasks = %d, want 1", len(tasks))
	}
	decls := tasks[0].Declarations
	if len(decls) != 2 {
		t.Fatalf("declarations = %d, want 2 (fileFrom + fileTo facts only); got %+v", len(decls), decls)
	}
	// fileFrom appears before fileTo (sorted), so CallB comes first.
	if decls[0].Name != "CallB" || decls[0].File != fileFrom {
		t.Errorf("decls[0] = %+v, want CallB in %s", decls[0], fileFrom)
	}
	if decls[1].Name != "internalImpl" || decls[1].File != fileTo {
		t.Errorf("decls[1] = %+v, want internalImpl in %s", decls[1], fileTo)
	}
	// file:line is preserved.
	if decls[0].StartLine != 3 {
		t.Errorf("decls[0] line = %d, want 3", decls[0].StartLine)
	}
}

// TestBuild_DeclarationsAbsentWhenSyntaxEmpty verifies that when no SyntaxFacts
// are provided, the Declarations field is nil and the JSON output is byte-for-byte
// identical to the pre-enrichment shape (no extra key, no empty array).
func TestBuild_DeclarationsAbsentWhenSyntaxEmpty(t *testing.T) {
	tasks := agenttask.Build(
		[]finding.Finding{gateFinding("f1", ruleForbidden, finding.StatusNew)},
		map[string]string{ruleForbidden: "forbidden_dependency"},
		nil,
		[]string{validateCmd},
		nil,
		nil,
		agenttask.PathResolver{},
	)
	if len(tasks) != 1 {
		t.Fatalf("tasks = %d, want 1", len(tasks))
	}
	if tasks[0].Declarations != nil {
		t.Errorf("Declarations = %v, want nil when SyntaxFacts absent", tasks[0].Declarations)
	}

	// Marshal and confirm the "declarations" key is absent from JSON.
	b, err := json.Marshal(tasks[0])
	if err != nil {
		t.Fatalf("marshal error: %v", err)
	}
	if strings.Contains(string(b), `"declarations"`) {
		t.Errorf("JSON contains 'declarations' key but should be absent; got: %s", b)
	}
}

// TestBuild_ModuleCycleGoalBoundsTheMemberList pins that a large cycle's goal
// names its size instead of every member: goal text is capped downstream.
func TestBuild_ModuleCycleGoalBoundsTheMemberList(t *testing.T) {
	const ruleModuleCycle = "no_module_cycles"
	members := make([]string, 60)
	for i := range members {
		members[i] = fmt.Sprintf("capability-module-%02d", i)
	}
	f := finding.Finding{
		ID: "c1", Kind: finding.KindGate, RuleID: ruleModuleCycle, Status: finding.StatusNew,
		Edge: finding.EdgeEvidence{
			From: finding.Endpoint{Module: members[0]},
			To:   finding.Endpoint{Module: members[1]},
			Kind: "module_dependency",
		},
		MatchedBy: map[string]string{"cycle_modules": strings.Join(members, ", "), "cycle_size": "60"},
	}
	tasks := agenttask.Build([]finding.Finding{f},
		map[string]string{ruleModuleCycle: ruleTypeCycle}, nil, []string{validateCmd},
		nil, nil, agenttask.PathResolver{})
	if len(tasks) != 1 {
		t.Fatalf("tasks = %d, want 1", len(tasks))
	}
	goal := tasks[0].Goal
	if !strings.Contains(goal, "(60 modules, listed in matched_by.cycle_modules)") {
		t.Errorf("goal %q does not summarise the 60-module cycle", goal)
	}
	if strings.Contains(goal, members[59]) || len(goal) > 600 {
		t.Errorf("goal is not bounded: %d bytes", len(goal))
	}
}

// TestBuild_ModuleCycleTaskAsksToRemoveOneDirection pins the module_cycle
// repair contract: the goal names the cycle's members and the direction to drop,
// files come from the import sites (the finding has no endpoint paths), and the
// target's public surface is never offered — importing it through its public
// API still closes the cycle.
func TestBuild_ModuleCycleTaskAsksToRemoveOneDirection(t *testing.T) {
	const ruleModuleCycle = "no_module_cycles"
	f := finding.Finding{
		ID: "c1", Kind: finding.KindGate, RuleID: ruleModuleCycle, Status: finding.StatusNew,
		Edge: finding.EdgeEvidence{
			From: finding.Endpoint{Module: "billing"},
			To:   finding.Endpoint{Module: "shipping"},
			Kind: "module_dependency",
		},
		MatchedBy:  map[string]string{"cycle_modules": "billing, shipping"},
		Locations:  []relationship.Location{{File: "billing/app/notify.go", Line: 3}},
		Constraint: "Remove one direction of the module cycle",
	}
	tasks := agenttask.Build([]finding.Finding{f},
		map[string]string{ruleModuleCycle: ruleTypeCycle},
		map[string][]string{"shipping": {"shipping/api"}},
		[]string{validateCmd},
		nil,
		nil,
		agenttask.PathResolver{},
	)
	if len(tasks) != 1 {
		t.Fatalf("tasks = %d, want 1", len(tasks))
	}
	task := tasks[0]
	for _, want := range []string{"billing, shipping", "billing -> shipping", "from shipping back to billing"} {
		if !strings.Contains(task.Goal, want) {
			t.Errorf("goal %q does not mention %q", task.Goal, want)
		}
	}
	for _, c := range task.Constraints {
		if strings.Contains(c, "public surface") {
			t.Errorf("constraints = %v, want no public-surface route for a cycle", task.Constraints)
		}
	}
	if !reflect.DeepEqual(task.Files, []string{"billing/app/notify.go"}) {
		t.Errorf("files = %v, want the import site", task.Files)
	}
	if task.RepairKind != "code_change" {
		t.Errorf("repair kind = %q, want code_change", task.RepairKind)
	}
}

// TestBuild_ForbiddenPatternTaskNamesThePatternAndFile pins the pattern repair
// contract: the goal names the file and the pattern ID (never matched source),
// files come from the match sites, and no public-surface route is offered — the
// finding has no target module.
func TestBuild_ForbiddenPatternTaskNamesThePatternAndFile(t *testing.T) {
	const (
		rulePattern = "domain_no_clock"
		file        = "internal/domain/service.go"
	)
	f := finding.Finding{
		ID: "p1", Kind: finding.KindGate, RuleID: rulePattern, Status: finding.StatusNew,
		Edge:       finding.EdgeEvidence{From: finding.Endpoint{Module: moduleDomain, Path: file}, Kind: "pattern_match"},
		MatchedBy:  map[string]string{"pattern": "clock", "file": file},
		Locations:  []relationship.Location{{File: file, Line: 12}},
		Constraint: "Remove the construct matching pattern \"clock\"",
	}
	tasks := agenttask.Build([]finding.Finding{f},
		map[string]string{rulePattern: "forbidden_pattern"},
		map[string][]string{moduleDomain: {"internal/domain"}},
		[]string{validateCmd},
		nil,
		nil,
		agenttask.PathResolver{},
	)
	if len(tasks) != 1 {
		t.Fatalf("tasks = %d, want 1", len(tasks))
	}
	task := tasks[0]
	if !strings.Contains(task.Goal, file) || !strings.Contains(task.Goal, `"clock"`) {
		t.Errorf("goal %q does not name the file and the pattern", task.Goal)
	}
	if !reflect.DeepEqual(task.Files, []string{file}) {
		t.Errorf("files = %v, want the match site", task.Files)
	}
	for _, c := range task.Constraints {
		if strings.Contains(c, "public surface") {
			t.Errorf("constraints = %v, want no public-surface route", task.Constraints)
		}
	}
}
