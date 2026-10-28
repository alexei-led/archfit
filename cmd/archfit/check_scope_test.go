package main

import (
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"
)

// ruleNoBImpl is the rule hookRuleSection declares.
const (
	ruleNoBImpl = "no_b_impl"
	changePass  = "pass"
)

// scopeAgent is the part of the agent result the scope tests read.
type scopeAgent struct {
	NextAction string `json:"next_action"`
	Scope      *struct {
		Mode   string `json:"mode"`
		Target string `json:"target"`
		Base   string `json:"base"`
	} `json:"scope"`
	Change struct {
		Status       string `json:"status"`
		Reason       string `json:"reason"`
		ReasonDetail string `json:"reason_detail"`
		Blockers     []struct {
			RuleIDs []string `json:"rule_ids"`
		} `json:"blockers"`
		PolicyChange *struct {
			Files             []string `json:"files"`
			UnderTargetPolicy struct {
				Status   string `json:"status"`
				Blockers []struct {
					RuleIDs []string `json:"rule_ids"`
				} `json:"blockers"`
			} `json:"under_target_policy"`
		} `json:"policy_change"`
	} `json:"change"`
}

// scopeRepo commits cfg and a.go on main, then moves to a new feature branch.
func scopeRepo(t *testing.T, cfg, a string) string {
	t.Helper()
	dir := hookRepo(t, cfg, a, "")
	gitFixture(t, dir, "branch", "-M", "main")
	gitFixture(t, dir, "checkout", "-q", "-b", "feature")
	return dir
}

// commitFiles writes files and commits them on the current branch.
func commitFiles(t *testing.T, dir string, files map[string]string) {
	t.Helper()
	for name, content := range files {
		writeFixtureFile(t, dir, name, content)
	}
	gitCommitFixture(t, dir)
}

func checkAgent(t *testing.T, dir string, extra ...string) (int, scopeAgent, string) {
	t.Helper()
	args := append([]string{cmdCheck, "-c", filepath.Join(dir, defaultConfigPath), "--format=agent"}, extra...)
	code, stdout, stderr := runArchfit(t, args...)
	var res scopeAgent
	if err := json.Unmarshal([]byte(stdout), &res); err != nil {
		t.Fatalf("decode agent result: %v\nstdout: %s\nstderr: %s", err, stdout, stderr)
	}
	return code, res, stderr
}

// TestCheckScope_PolicyDeletedOnBranch: a branch that commits a violation and
// deletes the rule that forbids it cannot grade itself. The branch's own policy
// sees nothing; the target's policy, which the gate uses, sees the violation.
func TestCheckScope_PolicyDeletedOnBranch(t *testing.T) {
	t.Parallel()
	dir := scopeRepo(t, lintCleanModules+hookRuleSection, hookCleanA)
	commitFiles(t, dir, map[string]string{filePkgAA: hookViolatingA, defaultConfigPath: lintCleanModules})

	code, res, stderr := checkAgent(t, dir)
	if res.Change.Status != claimTypeUnknown || res.Change.Reason != "policy_changed_on_branch" || res.NextAction != "ask_owner" {
		t.Fatalf("change %q reason %q next_action %q, want unknown policy_changed_on_branch ask_owner\nstderr: %s", res.Change.Status, res.Change.Reason, res.NextAction, stderr)
	}
	if len(res.Change.Blockers) != 0 {
		t.Errorf("under the branch's own policy: %d blockers, want 0 (the rule is gone there)", len(res.Change.Blockers))
	}
	pc := res.Change.PolicyChange
	if pc == nil || len(pc.Files) != 1 || pc.Files[0] != defaultConfigPath {
		t.Fatalf("policy_change = %+v, want the changed file %s", pc, defaultConfigPath)
	}
	if pc.UnderTargetPolicy.Status != hardGateFail || len(pc.UnderTargetPolicy.Blockers) != 1 || pc.UnderTargetPolicy.Blockers[0].RuleIDs[0] != ruleNoBImpl {
		t.Errorf("under the target's policy = %+v, want fail with the no_b_impl violation", pc.UnderTargetPolicy)
	}
	if res.Scope == nil || res.Scope.Mode != "merge_base" || res.Scope.Target != "main" {
		t.Errorf("scope = %+v, want merge_base against main", res.Scope)
	}
	if code != 2 {
		t.Errorf("exit = %d, want 2 for an unknown change", code)
	}
}

// TestCheckScope_ViolationWithoutPolicyEdit: the same branch without the policy
// edit fails with its one violation.
func TestCheckScope_ViolationWithoutPolicyEdit(t *testing.T) {
	t.Parallel()
	dir := scopeRepo(t, lintCleanModules+hookRuleSection, hookCleanA)
	commitFiles(t, dir, map[string]string{filePkgAA: hookViolatingA})

	code, res, stderr := checkAgent(t, dir)
	if res.Change.Status != hardGateFail || len(res.Change.Blockers) != 1 || res.Change.Reason != "" || res.Change.PolicyChange != nil {
		t.Fatalf("change %+v, want fail with one blocker and no policy change\nstderr: %s", res.Change, stderr)
	}
	if code != 1 {
		t.Errorf("exit = %d, want 1", code)
	}
}

// TestCheckScope_OwnerAddsARule: a branch that only adds a rule is unknown, and
// the new rule's findings are listed under the branch's policy.
func TestCheckScope_OwnerAddsARule(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name          string
		mainA         string
		wantBlockers  int
		wantAttention string
	}{
		// The violation is introduced beside the rule: a blocker under the
		// branch's policy.
		{name: "rule and violation arrive together", mainA: hookCleanA, wantBlockers: 1},
		// The violation was already on main: the new rule flags old code, which
		// is repository attention, not this change's blocker.
		{name: "rule flags code that was already there", mainA: hookViolatingA, wantAttention: "existing_violation"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			dir := scopeRepo(t, lintCleanModules, tc.mainA)
			files := map[string]string{defaultConfigPath: lintCleanModules + hookRuleSection}
			if tc.mainA == hookCleanA {
				files[filePkgAA] = hookViolatingA
			}
			commitFiles(t, dir, files)

			_, stdout, stderr := runArchfit(t, cmdCheck, "-c", filepath.Join(dir, defaultConfigPath), "--format=agent")
			var res struct {
				scopeAgent
				Repository struct {
					Attention []struct {
						Kind    string   `json:"kind"`
						RuleIDs []string `json:"rule_ids"`
					} `json:"attention"`
				} `json:"repository"`
			}
			if err := json.Unmarshal([]byte(stdout), &res); err != nil {
				t.Fatalf("decode: %v\n%s\n%s", err, stdout, stderr)
			}
			if res.Change.Status != claimTypeUnknown || res.Change.Reason != "policy_changed_on_branch" {
				t.Fatalf("change %q reason %q, want unknown policy_changed_on_branch\nstderr: %s", res.Change.Status, res.Change.Reason, stderr)
			}
			if len(res.Change.Blockers) != tc.wantBlockers {
				t.Errorf("blockers under the branch's policy = %d, want %d", len(res.Change.Blockers), tc.wantBlockers)
			}
			if tc.wantBlockers == 1 && res.Change.Blockers[0].RuleIDs[0] != ruleNoBImpl {
				t.Errorf("blocker rules = %v, want the new rule", res.Change.Blockers[0].RuleIDs)
			}
			if tc.wantAttention != "" {
				found := false
				for _, a := range res.Repository.Attention {
					found = found || (a.Kind == tc.wantAttention && len(a.RuleIDs) == 1 && a.RuleIDs[0] == ruleNoBImpl)
				}
				if !found {
					t.Errorf("attention = %+v, want %s naming no_b_impl", res.Repository.Attention, tc.wantAttention)
				}
			}
			if pc := res.Change.PolicyChange; pc == nil || pc.UnderTargetPolicy.Status != changePass || len(pc.UnderTargetPolicy.Blockers) != 0 {
				t.Errorf("under the target's policy = %+v, want pass: the target has no such rule", pc)
			}
		})
	}
}

// TestCheckScope_UnresolvedTarget: a clone that holds no target branch cannot
// say what the change is. It is unknown with a reason, never a silent HEAD.
func TestCheckScope_UnresolvedTarget(t *testing.T) {
	t.Parallel()
	dir := scopeRepo(t, lintCleanModules+hookRuleSection, hookCleanA)
	commitFiles(t, dir, map[string]string{filePkgAA: hookCleanA + "\n// edit\n"})
	clone := t.TempDir()
	gitFixture(t, clone, "clone", "-q", "--depth", "1", "--branch", "feature", "file://"+dir, ".")
	gitFixture(t, clone, "remote", "set-head", "origin", "-d")

	code, res, stderr := checkAgent(t, clone)
	if res.Change.Status != claimTypeUnknown || res.Change.Reason != "scope_unresolved" || res.NextAction != "restore_evidence" {
		t.Fatalf("change %q reason %q next_action %q, want unknown scope_unresolved restore_evidence\nstderr: %s", res.Change.Status, res.Change.Reason, res.NextAction, stderr)
	}
	if !strings.Contains(res.Change.ReasonDetail, "no target branch") {
		t.Errorf("reason_detail = %q, want it to say no target branch", res.Change.ReasonDetail)
	}
	if code != 2 {
		t.Errorf("exit = %d, want 2", code)
	}
}

// TestCheckScope_OnTheTargetBranch: on the target branch the whole tree is the
// change, so a committed violation fails as it did before --base auto existed.
func TestCheckScope_OnTheTargetBranch(t *testing.T) {
	t.Parallel()
	dir := scopeRepo(t, lintCleanModules+hookRuleSection, hookViolatingA)
	gitFixture(t, dir, "checkout", "-q", "main")

	code, res, stderr := checkAgent(t, dir)
	if res.Change.Status != hardGateFail || res.Scope == nil || res.Scope.Mode != "target_branch" || res.Scope.Base != "" {
		t.Fatalf("change %q scope %+v, want fail in target_branch mode with no base\nstderr: %s", res.Change.Status, res.Scope, stderr)
	}
	if code != 1 {
		t.Errorf("exit = %d, want 1", code)
	}
}

// TestCheckScope_ExplicitBase keeps working: --base <ref> is the base, and the
// policy guard still compares with the target.
func TestCheckScope_ExplicitBase(t *testing.T) {
	t.Parallel()
	dir := scopeRepo(t, lintCleanModules+hookRuleSection, hookCleanA)
	commitFiles(t, dir, map[string]string{filePkgAA: hookViolatingA})

	_, res, stderr := checkAgent(t, dir, "--base", "main")
	if res.Change.Status != hardGateFail || res.Scope == nil || res.Scope.Mode != "explicit" || res.Scope.Base != "main" {
		t.Fatalf("change %q scope %+v, want fail in explicit mode against main\nstderr: %s", res.Change.Status, res.Scope, stderr)
	}
}

// TestCheckScope_TargetMovedThePolicy: a branch that never touched the policy
// is not accused of changing it when the target moved it after the fork. It is
// stale: update the branch.
func TestCheckScope_TargetMovedThePolicy(t *testing.T) {
	t.Parallel()
	dir := scopeRepo(t, lintCleanModules+hookRuleSection, hookCleanA)
	commitFiles(t, dir, map[string]string{filePkgAA: hookCleanA + "\n// work on the branch\n"})
	gitFixture(t, dir, "checkout", "-q", "main")
	commitFiles(t, dir, map[string]string{defaultConfigPath: lintCleanModules + hookRuleSection + "\n# tightened on main\n"})
	gitFixture(t, dir, "checkout", "-q", "feature")

	code, res, stderr := checkAgent(t, dir)
	if res.Change.Status != claimTypeUnknown || res.Change.Reason != "policy_stale_on_branch" || res.NextAction != "restore_evidence" {
		t.Fatalf("change %q reason %q next_action %q, want unknown policy_stale_on_branch restore_evidence\nstderr: %s", res.Change.Status, res.Change.Reason, res.NextAction, stderr)
	}
	if !strings.Contains(res.Change.ReasonDetail, "merge or rebase main") {
		t.Errorf("reason_detail = %q, want it to say how to catch up", res.Change.ReasonDetail)
	}
	if code != 2 {
		t.Errorf("exit = %d, want 2", code)
	}
}
