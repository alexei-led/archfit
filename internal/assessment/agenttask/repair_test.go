package agenttask_test

import (
	"strings"
	"testing"

	"github.com/alexei-led/archfit/internal/assessment/agenttask"
	"github.com/alexei-led/archfit/internal/assessment/finding"
)

const (
	publicAPIText    = "public API"
	targetPublicGlob = "pkg/b/api/**"
	ruleTypeNewCross = "new_cross_module_dependency"
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
