package engine

import (
	"reflect"
	"testing"

	"github.com/alexei-led/archfit/internal/model/coupling"
	"github.com/alexei-led/archfit/internal/model/finding"
	"github.com/alexei-led/archfit/internal/model/graph"
)

const (
	matchedGroupCount = "group_count"
	billingFile       = "internal/billing/a.go"
	ledgerFile        = "internal/ledger/b.go"
)

func TestBuildAdvisoryTasks_GroupedBCAdvisories(t *testing.T) {
	findings := []finding.Finding{
		{
			ID:       "gate-1",
			Kind:     finding.KindGate,
			RuleID:   RuleIDBCImbalanced,
			Status:   finding.StatusNew,
			Severity: finding.SeverityHigh,
			MatchedBy: map[string]string{
				matchedGroupCount: "3",
			},
		},
		{
			ID:       "single-1",
			Kind:     finding.KindAdvisory,
			RuleID:   RuleIDBCImbalanced,
			Status:   finding.StatusNew,
			Severity: finding.SeverityMedium,
			MatchedBy: map[string]string{
				matchedGroupCount: "1",
			},
		},
		{
			ID:       "rollup-1",
			Kind:     finding.KindAdvisory,
			RuleID:   RuleIDBCImbalanced,
			Status:   finding.StatusNew,
			Severity: finding.SeverityHigh,
			Edge: finding.EdgeEvidence{
				From: finding.Endpoint{Module: "billing", Path: billingFile},
				To:   finding.Endpoint{Module: "ledger", Path: ledgerFile},
				Kind: "imports",
			},
			MatchedBy: map[string]string{
				matchedGroupCount: "3",
				"group_members":   "id1,id2,id3",
				"cheapest_move":   "reduce_distance",
				"score_value":     "8",
				matchedStrength:   string(coupling.StrengthIntrusive),
				matchedDistance:   "shared_owner",
				matchedVolatility: string(coupling.VolatilityHigh),
			},
			Locations: []graph.Location{
				{File: ledgerFile, Line: 4},
				{File: billingFile, Line: 2},
				{File: billingFile, Line: 3},
			},
			Constraint: "Prefer the public ledger API.",
		},
	}

	tasks := BuildAdvisoryTasks(findings, []string{"archfit analyze --gate --full"})
	if len(tasks) != 1 {
		t.Fatalf("BuildAdvisoryTasks() = %d tasks, want 1: %+v", len(tasks), tasks)
	}
	task := tasks[0]
	if task.FindingID != "rollup-1" || task.RuleID != RuleIDBCImbalanced {
		t.Fatalf("task identity = %+v, want rollup-1/%s", task, RuleIDBCImbalanced)
	}
	if task.GroupCount != 3 || task.Severity != finding.SeverityHigh || task.Status != finding.StatusNew {
		t.Errorf("task rollup metadata = count %d severity %q status %q", task.GroupCount, task.Severity, task.Status)
	}
	if !reflect.DeepEqual(task.GroupMembers, []string{"id1", "id2", "id3"}) {
		t.Errorf("GroupMembers = %v", task.GroupMembers)
	}
	if task.CheapestMove != "reduce_distance" || task.ScoreValue != 8 {
		t.Errorf("move/score = %q/%d", task.CheapestMove, task.ScoreValue)
	}
	wantFiles := []string{billingFile, ledgerFile}
	if !reflect.DeepEqual(task.TopFiles, wantFiles) {
		t.Errorf("TopFiles = %v, want %v", task.TopFiles, wantFiles)
	}
	wantValidation := []string{"archfit analyze --gate --full"}
	if !reflect.DeepEqual(task.Validation, wantValidation) {
		t.Errorf("Validation = %v, want %v", task.Validation, wantValidation)
	}
	for _, want := range []string{
		"keep agent_tasks[] reserved for active gate findings",
		"preserve or improve coupling shape: strength=" + string(coupling.StrengthIntrusive) + ", distance=shared_owner, volatility=" + string(coupling.VolatilityHigh),
		"prefer cheapest_move: reduce_distance",
		"Prefer the public ledger API.",
	} {
		if !containsString(task.Constraints, want) {
			t.Errorf("constraints missing %q: %v", want, task.Constraints)
		}
	}
	if task.Goal == "" {
		t.Error("Goal is empty")
	}
}

func containsString(items []string, want string) bool {
	for _, item := range items {
		if item == want {
			return true
		}
	}
	return false
}
