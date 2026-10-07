package main

import (
	"bytes"
	"encoding/json"
	"path/filepath"
	"testing"
)

const (
	ruleMapUncovered   = "map/uncovered_path"
	mapCompletenessCfg = hookModules + "module_review:\n  gate: "
	unownedPkgC        = "pkg/c/c.go"
	unownedPkgD        = "pkg/d/d.go"
	moduleReviewFail   = "fail"
	moduleReviewWarn   = "warn"
	gateNew            = "gate/new"
	advisoryNew        = "advisory/new"
)

type mapCompletenessReport struct {
	Findings []struct {
		ID        string            `json:"id"`
		Kind      string            `json:"kind"`
		RuleID    string            `json:"rule_id"`
		Status    string            `json:"status"`
		MatchedBy map[string]string `json:"matched_by"`
	} `json:"findings"`
	AgentTasks []struct {
		RuleID     string   `json:"rule_id"`
		RepairKind string   `json:"repair_kind"`
		Files      []string `json:"files"`
	} `json:"agent_tasks"`
}

// mapCompletenessRepo is the two-module fixture plus pkg/c, a production
// package no declared module owns, and a test-only package that never counts.
func mapCompletenessRepo(t *testing.T, gate string) string {
	t.Helper()
	dir := t.TempDir()
	for name, content := range map[string]string{
		markerGoMod:       goModStub,
		filePkgAA:         hookCleanA,
		hookImplFile:      implSource(),
		unownedPkgC:       "package c\n\nfunc C() {}\n",
		"pkg/e/e_test.go": "package e\n",
		defaultConfigPath: mapCompletenessCfg + gate + "\n",
	} {
		writeFixtureFile(t, dir, name, content)
	}
	gitInitFixtureRepo(t, dir)
	return dir
}

func checkMapCompleteness(t *testing.T, dir string) (int, mapCompletenessReport) {
	t.Helper()
	var buf bytes.Buffer
	code := Run([]string{cmdCheck, fmtJSON, "-c", filepath.Join(dir, defaultConfigPath), flagRefresh}, &buf)
	var rep mapCompletenessReport
	if err := json.Unmarshal(buf.Bytes(), &rep); err != nil {
		t.Fatalf("check exit %d, unmarshal: %v\n%s", code, err, buf.String())
	}
	return code, rep
}

func uncoveredSubjects(rep mapCompletenessReport) map[string]string {
	out := map[string]string{}
	for _, f := range rep.Findings {
		if f.RuleID == ruleMapUncovered {
			out[f.MatchedBy["subject"]] = f.Kind + "/" + f.Status
		}
	}
	return out
}

// TestRun_Check_ModuleReviewGate pins map completeness end to end: production
// source no declared module owns is a blocker under module_review.gate: fail
// and a diagnostic under warn, and its repair task asks the owner.
func TestRun_Check_ModuleReviewGate(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		gate     string
		wantExit int
		wantKind string
	}{
		{gate: moduleReviewFail, wantExit: 1, wantKind: gateNew},
		{gate: moduleReviewWarn, wantExit: 2, wantKind: advisoryNew},
	} {
		t.Run(tc.gate, func(t *testing.T) {
			t.Parallel()
			code, rep := checkMapCompleteness(t, mapCompletenessRepo(t, tc.gate))
			if code != tc.wantExit {
				t.Fatalf("exit = %d, want %d", code, tc.wantExit)
			}
			got := uncoveredSubjects(rep)
			if len(got) != 1 || got["pkg/c"] != tc.wantKind {
				t.Fatalf("uncovered = %v, want only pkg/c as %s", got, tc.wantKind)
			}
			if tc.gate != moduleReviewFail {
				return
			}
			for _, task := range rep.AgentTasks {
				if task.RuleID == ruleMapUncovered && task.RepairKind == "needs_owner_decision" &&
					len(task.Files) == 1 && task.Files[0] == unownedPkgC {
					return
				}
			}
			t.Errorf("agent_tasks = %+v, want an owner-decision task naming %s", rep.AgentTasks, unownedPkgC)
		})
	}
}

// TestRun_Check_ModuleReviewGateBlocksOnlyANewPackage is the roadmap's done
// criterion: after a baseline accepts today's unowned package, check passes the
// gate, and a new package outside every module fails it.
func TestRun_Check_ModuleReviewGateBlocksOnlyANewPackage(t *testing.T) {
	t.Parallel()
	dir := mapCompletenessRepo(t, moduleReviewFail)
	var buf bytes.Buffer
	if code := Run([]string{cmdBaseline, "-c", filepath.Join(dir, defaultConfigPath), flagRefresh}, &buf); code != 0 {
		t.Fatalf("baseline exit = %d\n%s", code, buf.String())
	}
	if code, rep := checkMapCompleteness(t, dir); code == 1 {
		t.Fatalf("check after baseline exit = 1, want accepted debt not to block; uncovered = %v", uncoveredSubjects(rep))
	}
	writeFixtureFile(t, dir, unownedPkgD, "package d\n\nfunc D() {}\n")
	code, rep := checkMapCompleteness(t, dir)
	if code != 1 {
		t.Fatalf("check with a new unowned package exit = %d, want 1", code)
	}
	got := uncoveredSubjects(rep)
	if got["pkg/d"] != gateNew || got["pkg/c"] == gateNew {
		t.Errorf("uncovered = %v, want pkg/d new and pkg/c accepted", got)
	}
}
