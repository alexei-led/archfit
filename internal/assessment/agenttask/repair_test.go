package agenttask_test

import (
	"strings"
	"testing"

	"github.com/alexei-led/archfit/internal/assessment/agenttask"
	"github.com/alexei-led/archfit/internal/assessment/finding"
	"github.com/alexei-led/archfit/internal/model/evidence"
	"github.com/alexei-led/archfit/internal/relationship"
)

const (
	publicAPIText      = "public API"
	targetPublicGlob   = "pkg/b/api/**"
	ruleTypeNewCross   = "new_cross_module_dependency"
	ruleTypeModuleDeps = "module_dependencies"
)

func TestRepairDoesNotSuggestForbiddenPublicRoute(t *testing.T) {
	for _, ruleType := range []string{ruleTypeForbidden, ruleTypeNewCross} {
		t.Run(ruleType, func(t *testing.T) {
			f := gateFinding("repair", ruleForbidden, finding.StatusNew)
			f.Constraint = "Remove the dependency"
			f.Alternatives = nil
			tasks := agenttask.Build([]finding.Finding{f}, map[string]string{ruleForbidden: ruleType}, nil, nil, nil, nil, agenttask.PathResolver{})
			if len(tasks) != 1 {
				t.Fatalf("tasks = %d, want 1", len(tasks))
			}
			for _, invalid := range []string{publicAPIText, "archfit baseline"} {
				if strings.Contains(tasks[0].Goal, invalid) {
					t.Errorf("goal suggests %q: %s", invalid, tasks[0].Goal)
				}
			}
		})
	}
}

// TestConstraintsListTargetPublicSurfaceOnlyWhereItIsARoute pins the
// rule-aware constraint: a forbidden dependency, an inverted layer, a cycle, or
// a new cross-module dependency stays a violation through the target's public
// API, so listing that surface sends the agent straight back into it. Rules
// whose remedy IS the public surface, or that describe the module's own
// surface, keep it.
func TestConstraintsListTargetPublicSurfaceOnlyWhereItIsARoute(t *testing.T) {
	tests := []struct {
		ruleType   string
		wantPublic bool
	}{
		{ruleTypeForbidden, false},
		{ruleTypeLayer, false},
		{ruleTypePublicAPI, true},
		{"internal_api_access", true},
		{"cycle", false},
		{ruleTypeCycle, false},
		{ruleTypeNewCross, false},
		{ruleTypeModuleDeps, false},
		{"public_api_max", true},
	}
	for _, tc := range tests {
		t.Run(tc.ruleType, func(t *testing.T) {
			f := gateFinding("repair", ruleForbidden, finding.StatusNew)
			tasks := agenttask.Build([]finding.Finding{f}, map[string]string{ruleForbidden: tc.ruleType},
				map[string][]string{"b": {targetPublicGlob}}, nil, nil, nil, agenttask.PathResolver{})
			if len(tasks) != 1 {
				t.Fatalf("tasks = %d, want 1", len(tasks))
			}
			listed := strings.Contains(strings.Join(tasks[0].Constraints, "\n"), targetPublicGlob)
			if listed != tc.wantPublic {
				t.Errorf("target public surface listed = %v, want %v; constraints = %q",
					listed, tc.wantPublic, tasks[0].Constraints)
			}
			if tasks[0].Constraints[0] != f.Constraint {
				t.Errorf("constraints[0] = %q, want the rule's own constraint %q", tasks[0].Constraints[0], f.Constraint)
			}
		})
	}
}

// TestModuleDependenciesTaskNamesThePairAndTheOwnerPath pins the allowlist
// repair contract: the goal names the denied pair and the denying lists, says
// a route through the target's public API keeps the violation, and leaves the
// allowlist change to the architecture owner. An unowned importer is named by
// its package. A module-pair task carries no declarations.
func TestModuleDependenciesTaskNamesThePairAndTheOwnerPath(t *testing.T) {
	const importer = "pkg/a/a.go"
	tests := []struct {
		name      string
		from      finding.Endpoint
		violates  string
		wantNamed string
	}{
		{"declared importer", finding.Endpoint{Module: "a"}, "depends_on,visible_to", "Remove the dependency of a on module b"},
		{"unowned importer", finding.Endpoint{Path: "tools/gen"}, "visible_to", "Remove the dependency of tools/gen (owned by no declared module) on module b"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			f := finding.NewKeyed("boundaries", "module_dependency", tc.from.Module, "b")
			f.Edge.From, f.Edge.To = tc.from, finding.Endpoint{Module: "b"}
			f.MatchedBy = map[string]string{"violates": tc.violates}
			f.Locations = []relationship.Location{{File: importer, Line: 3}}
			facts := []evidence.SyntaxFact{{File: importer, Kind: "function", Name: "Charge", Exported: true, StartLine: 3}}
			tasks := agenttask.Build([]finding.Finding{f}, map[string]string{"boundaries": ruleTypeModuleDeps},
				map[string][]string{"b": {targetPublicGlob}}, nil, facts, nil, agenttask.PathResolver{})
			if len(tasks) != 1 {
				t.Fatalf("tasks = %d, want 1", len(tasks))
			}
			task := tasks[0]
			for _, want := range []string{tc.wantNamed, "(" + tc.violates + ")", "public API", "keeps the violation", "architecture owner"} {
				if !strings.Contains(task.Goal, want) {
					t.Errorf("goal %q does not contain %q", task.Goal, want)
				}
			}
			if task.RepairKind != "code_change" || len(task.Declarations) != 0 {
				t.Errorf("repair kind = %s, declarations = %v, want code_change and none", task.RepairKind, task.Declarations)
			}
			if strings.Contains(strings.Join(task.Constraints, "\n"), targetPublicGlob) {
				t.Errorf("constraints list the target's public surface: %q", task.Constraints)
			}
		})
	}
}
