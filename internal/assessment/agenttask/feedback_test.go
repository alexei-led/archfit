package agenttask_test

import (
	"reflect"
	"testing"

	"github.com/alexei-led/archfit/v3/internal/assessment/agenttask"
	"github.com/alexei-led/archfit/v3/internal/assessment/finding"
	"github.com/alexei-led/archfit/v3/internal/assessment/rules"
	"github.com/alexei-led/archfit/v3/internal/model/evidence"
	"github.com/alexei-led/archfit/v3/internal/policy"
	"github.com/alexei-led/archfit/v3/internal/relationship"
)

const feedbackRepairKind = "code_change"

func TestPublicAPICeilingTaskSeparatesSourceReviewFromOwnerPolicy(t *testing.T) {
	const ruleID = "api-ceiling"
	ceiling := 1
	compiled, err := rules.New(policy.RuleConfig{
		Rules: []policy.RuleDef{{ID: ruleID, Type: rulePublicAPIMax, Max: &ceiling}},
		ModuleMap: policy.BuildModuleMap(map[string]policy.ModuleDef{
			moduleDomain: {Paths: []string{"pkg/a/**"}},
		}),
	})
	if err != nil {
		t.Fatal(err)
	}
	findings := compiled[0].Check(relationship.Set{}, rules.Evidence{SyntaxFacts: []evidence.SyntaxFact{
		{File: fileFrom, Name: "First", Kind: kindFunction, Exported: true},
		{File: fileFrom, Name: "Second", Kind: kindFunction, Exported: true},
	}})
	if len(findings) != 1 {
		t.Fatalf("findings = %+v, want one API ceiling finding", findings)
	}
	f := findings[0]
	if f.RuleID != ruleID || f.Kind != finding.KindGate || f.Status != finding.StatusNew || f.Severity != finding.SeverityMedium ||
		f.MatchedBy["count"] != "2" || f.MatchedBy["max"] != "1" {
		t.Fatalf("finding facts changed: %+v", f)
	}
	const wantConstraint = "Review the module's exported declarations and remove unneeded public API. Only the architecture owner may change the max threshold; do not raise it to make this check pass."
	if f.Constraint != wantConstraint {
		t.Errorf("finding constraint = %q, want %q", f.Constraint, wantConstraint)
	}
	tasks := agenttask.Build(findings, map[string]string{ruleID: rulePublicAPIMax}, nil, []string{validateCmd}, nil, agenttask.PathResolver{})
	if len(tasks) != 1 {
		t.Fatalf("tasks = %+v, want one source-review task", tasks)
	}
	task := tasks[0]
	if task.FindingID != f.ID || task.Goal != f.Why || task.RepairKind != feedbackRepairKind {
		t.Errorf("task identity/goal/repair kind changed: %+v", task)
	}
	if !reflect.DeepEqual(task.Constraints, []string{wantConstraint}) ||
		!reflect.DeepEqual(task.Files, []string{fileFrom}) ||
		!reflect.DeepEqual(task.Validation, []string{validateCmd}) {
		t.Errorf("task must carry the owner-only constraint, real source and supplied validation: %+v", task)
	}
}
