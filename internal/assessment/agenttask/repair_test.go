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

// TestTaskCarriesTheRuleRationale pins the one agenttask change for rule
// rationale: the rationale follows the rule's constraint, the alternatives
// follow it, and the target's public surface stays out for a rule that
// forbids the route. A module-selector finding names its modules in the goal.
func TestTaskCarriesTheRuleRationale(t *testing.T) {
	f := finding.NewKeyed("domain_no_http", "module_dependency", "module:"+seamTo, "path:net/http")
	f.Edge.From, f.Edge.To = finding.Endpoint{Module: seamTo}, finding.Endpoint{Path: "net/http"}
	f.Constraint = "Remove the dependency (see docs/adr/003.md)"
	f.Rationale = "Domain code stays free of I/O"
	f.Alternatives = []string{"Depend on a port in the application layer"}
	tasks := agenttask.Build([]finding.Finding{f}, map[string]string{"domain_no_http": ruleTypeForbidden},
		map[string][]string{seamTo: {targetPublicGlob}}, nil, nil, nil, agenttask.PathResolver{})
	if len(tasks) != 1 {
		t.Fatalf("tasks = %d, want 1", len(tasks))
	}
	want := []string{
		"Remove the dependency (see docs/adr/003.md)",
		"rationale: Domain code stays free of I/O",
		"allowed alternative: Depend on a port in the application layer",
	}
	if got := tasks[0].Constraints; strings.Join(got, "\n") != strings.Join(want, "\n") {
		t.Errorf("constraints = %q, want %q", got, want)
	}
	if !strings.Contains(tasks[0].Goal, "from module "+seamTo+" on net/http") {
		t.Errorf("goal = %q, want it to name module billing and net/http", tasks[0].Goal)
	}
}

// TestModuleSelectorTaskCarriesNoDeclarations pins that a module-selector
// forbidden_dependency task, a module-pair task with up to fifty import sites,
// carries no declarations, like the other module-pair tasks; a path-mode
// forbidden_dependency task keeps them.
func TestModuleSelectorTaskCarriesNoDeclarations(t *testing.T) {
	const importer = "pkg/a/a.go"
	facts := []evidence.SyntaxFact{{File: importer, Kind: "function", Name: "Charge", Exported: true, StartLine: 3}}
	for _, tc := range []struct {
		name  string
		kind  string
		wantN int
	}{
		{"module selector", "module_dependency", 0},
		{"path glob", "imports", 1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f := gateFinding("sel", ruleForbidden, finding.StatusNew)
			f.Edge.Kind = tc.kind
			f.Locations = []relationship.Location{{File: importer, Line: 3}}
			tasks := agenttask.Build([]finding.Finding{f}, map[string]string{ruleForbidden: ruleTypeForbidden}, nil, nil, facts, nil, agenttask.PathResolver{})
			if len(tasks) != 1 || len(tasks[0].Declarations) != tc.wantN {
				t.Errorf("tasks = %+v, want %d declarations", tasks, tc.wantN)
			}
		})
	}
}

// TestUncoveredSourceTaskAsksTheOwner pins the map completeness repair: which
// module owns a directory is a config decision, so the task asks the owner and
// still points at the directory's files.
func TestUncoveredSourceTaskAsksTheOwner(t *testing.T) {
	const file = "tools/gen/main.go"
	f := finding.Finding{
		ID: "uncovered", Kind: finding.KindGate, RuleID: finding.RuleIDMapUncoveredPath, Status: finding.StatusNew,
		MatchedBy: map[string]string{"subject": "tools/gen"},
		Locations: []relationship.Location{{File: file}},
	}
	resolver := agenttask.NewPathResolver(map[string]struct{}{file: {}}, nil, nil, nil)
	tasks := agenttask.Build([]finding.Finding{f}, nil, nil, nil, nil, nil, resolver)
	if len(tasks) != 1 {
		t.Fatalf("tasks = %d, want 1", len(tasks))
	}
	task := tasks[0]
	if task.RepairKind != "needs_owner_decision" {
		t.Errorf("repair_kind = %q, want needs_owner_decision", task.RepairKind)
	}
	if !strings.Contains(task.Goal, "tools/gen") || !strings.Contains(task.Goal, "architecture owner") {
		t.Errorf("goal = %q, want it to name the directory and the owner", task.Goal)
	}
	if len(task.Files) != 1 || task.Files[0] != file {
		t.Errorf("files = %v, want [%s]", task.Files, file)
	}
}
