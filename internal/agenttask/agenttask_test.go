package agenttask_test

import (
	"reflect"
	"strings"
	"testing"

	"github.com/alexei-led/archfit/internal/agenttask"
	"github.com/alexei-led/archfit/internal/model/finding"
	"github.com/alexei-led/archfit/internal/model/graph"
)

const (
	ruleForbidden = "no_internal_access"
	fileFrom      = "pkg/a/a.go"
	fileTo        = "pkg/b/internal/impl.go"
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
		Locations:    []graph.Location{{File: fileFrom, Line: 5}},
		Why:          "a uses b internals",
		Constraint:   "Use only the public API of module b",
		Alternatives: []string{"pkg/b/api"},
	}
}

func TestBuild_ActiveGateFindingsOnly(t *testing.T) {
	findings := []finding.Finding{
		gateFinding("f-new", ruleForbidden, finding.StatusNew),
		gateFinding("f-expired", ruleForbidden, finding.StatusExpiredExcept),
		gateFinding("f-baselined", ruleForbidden, finding.StatusBaseline),
		gateFinding("f-fixed", ruleForbidden, finding.StatusFixed),
		func() finding.Finding {
			f := gateFinding("f-advisory", "bc/imbalanced_coupling", finding.StatusNew)
			f.Kind = "advisory"
			return f
		}(),
	}

	tasks := agenttask.Build(findings,
		map[string]string{ruleForbidden: "forbidden_dependency"},
		nil,
		[]string{"archfit check --full"},
	)

	if len(tasks) != 2 {
		t.Fatalf("tasks = %d, want 2 (new + expired only); got %+v", len(tasks), tasks)
	}
	// Sorted by FindingID: f-expired < f-new.
	if tasks[0].FindingID != "f-expired" || tasks[1].FindingID != "f-new" {
		t.Errorf("order = [%s, %s], want [f-expired, f-new]", tasks[0].FindingID, tasks[1].FindingID)
	}
}

func TestBuild_TaskShape(t *testing.T) {
	tasks := agenttask.Build(
		[]finding.Finding{gateFinding("f1", ruleForbidden, finding.StatusNew)},
		map[string]string{ruleForbidden: "public_api_only"},
		map[string][]string{"b": {"pkg/b/api/**"}},
		[]string{"archfit check -c .archfit.yaml --full"},
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
	if len(task.Validation) != 1 || task.Validation[0] != "archfit check -c .archfit.yaml --full" {
		t.Errorf("validation = %v", task.Validation)
	}
}

func TestBuild_GoalTemplates(t *testing.T) {
	tests := []struct {
		ruleType string
		want     string
	}{
		{"forbidden_dependency", "Remove the forbidden dependency"},
		{"public_api_only", "public API"},
		{"internal_api_access", "public API"},
		{"forbidden_layer_direction", "inner layers must not import outer layers"},
		{"new_cross_module_dependency", "archfit baseline"},
		{"cycle", "Break the import cycle"},
		{"someone_elses_rule", "a uses b internals"}, // unknown type → Why fallback
	}
	for _, tc := range tests {
		t.Run(tc.ruleType, func(t *testing.T) {
			tasks := agenttask.Build(
				[]finding.Finding{gateFinding("f1", ruleForbidden, finding.StatusNew)},
				map[string]string{ruleForbidden: tc.ruleType},
				nil, nil,
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
	if got := agenttask.Build(nil, nil, nil, nil); got == nil || len(got) != 0 {
		t.Errorf("nil findings → %v, want empty non-nil slice", got)
	}

	findings := []finding.Finding{
		gateFinding("z", ruleForbidden, finding.StatusNew),
		gateFinding("a", ruleForbidden, finding.StatusNew),
	}
	first := agenttask.Build(findings, nil, nil, []string{"archfit check"})
	second := agenttask.Build(findings, nil, nil, []string{"archfit check"})
	if !reflect.DeepEqual(first, second) {
		t.Error("two builds differ — must be deterministic")
	}
	if first[0].FindingID != "a" || first[1].FindingID != "z" {
		t.Errorf("not sorted by FindingID: %s, %s", first[0].FindingID, first[1].FindingID)
	}
}
