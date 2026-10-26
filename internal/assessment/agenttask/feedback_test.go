package agenttask_test

import (
	"reflect"
	"strings"
	"testing"

	"github.com/alexei-led/archfit/v3/internal/assessment/agenttask"
	"github.com/alexei-led/archfit/v3/internal/assessment/finding"
	"github.com/alexei-led/archfit/v3/internal/assessment/rules"
	"github.com/alexei-led/archfit/v3/internal/model/evidence"
	"github.com/alexei-led/archfit/v3/internal/policy"
	"github.com/alexei-led/archfit/v3/internal/relationship"
)

const feedbackRepairKind = "code_change"

func TestMetricRatchetTaskDoesNotInferPROriginFromAcceptedReference(t *testing.T) {
	for _, origin := range []finding.Origin{"", finding.OriginUnknown, finding.OriginIntroduced, finding.OriginPreExisting} {
		t.Run("origin="+string(origin), func(t *testing.T) {
			f := finding.NewKeyed(finding.RuleIDMetricPrefix+"cycle_count", "metric_ratchet", "cycle_count")
			f.Origin = origin
			f.MatchedBy = map[string]string{"metric": "cycle_count", "before": "0", "after": "2", "delta": "2", "threshold": "max_new 0"}
			f.Constraint = "do not accept this by re-running `archfit baseline`: restore the metric, or ask the owner to review and re-baseline"
			validation := []string{"archfit check -c .archfit.yaml --base HEAD"}
			tasks := agenttask.Build([]finding.Finding{f}, nil, nil, validation, nil, nil, agenttask.PathResolver{})
			if len(tasks) != 1 {
				t.Fatalf("tasks = %+v, want one metric task", tasks)
			}
			task := tasks[0]
			for _, want := range []string{
				"Restore the cycle_count metric to its accepted value 0; it is now 2.",
				"Investigate the difference from the accepted baseline",
				"this comparison alone does not establish which PR introduced it",
				"Ask the architecture owner to review the new value before accepting it",
			} {
				if !strings.Contains(task.Goal, want) {
					t.Errorf("goal = %q, want %q", task.Goal, want)
				}
			}
			if strings.Contains(task.Goal, "in this change") {
				t.Errorf("goal infers PR causality from an accepted-reference delta: %q", task.Goal)
			}
			if task.FindingID != f.ID || task.RuleID != f.RuleID || task.RepairKind != feedbackRepairKind ||
				!reflect.DeepEqual(task.Constraints, []string{f.Constraint}) ||
				!reflect.DeepEqual(task.Validation, validation) || len(task.Files) != 0 {
				t.Errorf("non-copy task facts changed: %+v", task)
			}
		})
	}
}

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
	tasks := agenttask.Build(findings, map[string]string{ruleID: rulePublicAPIMax},
		nil, []string{validateCmd}, nil, nil, agenttask.PathResolver{})
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
