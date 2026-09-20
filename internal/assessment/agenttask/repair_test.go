package agenttask_test

import (
	"strings"
	"testing"

	"github.com/alexei-led/archfit/internal/assessment/agenttask"
	"github.com/alexei-led/archfit/internal/assessment/finding"
)

const publicAPIText = "public API"

func TestRepairDoesNotSuggestForbiddenPublicRoute(t *testing.T) {
	for _, ruleType := range []string{ruleTypeForbidden, "new_cross_module_dependency"} {
		t.Run(ruleType, func(t *testing.T) {
			f := gateFinding("repair", ruleForbidden, finding.StatusNew)
			f.Constraint = "Remove the dependency"
			f.Alternatives = nil
			tasks := agenttask.Build([]finding.Finding{f}, map[string]string{ruleForbidden: ruleType}, nil, nil, nil, agenttask.PathResolver{})
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
