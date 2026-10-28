package main

import (
	"encoding/json"
	"strings"
	"testing"
)

const (
	webAppTS     = "web/app.ts"
	movedImport  = "package c\n\nimport \"example.com/test/pkg/b/impl\"\n\nvar _ = impl.Secret()\n"
	failChange   = "module_review:\n  change_gate: fail\n"
	tsDisabled   = "languages:\n  typescript:\n    enabled: false\n"
	excludeTools = "exclude:\n  - \"gen/**\"\n"
)

type uncoveredResult struct {
	NextAction string `json:"next_action"`
	Change     struct {
		Status           string `json:"status"`
		UnevaluatedRules []struct {
			RuleID string `json:"rule_id"`
			Reason string `json:"reason"`
		} `json:"unevaluated_rules"`
		Notices []struct {
			Kind  string `json:"kind"`
			Count int    `json:"count"`
		} `json:"notices"`
	} `json:"change"`
}

func checkUncovered(t *testing.T, dir string) (int, uncoveredResult) {
	t.Helper()
	code, out, stderr := runArchfit(t, cmdCheck, "-c", dir+"/"+defaultConfigPath, "--format=agent")
	var res uncoveredResult
	if err := json.Unmarshal([]byte(out), &res); err != nil {
		t.Fatalf("decode: %v\nstdout: %s\nstderr: %s", err, out, stderr)
	}
	return code, res
}

// TestCheckUncoveredChange: a forbidden import moved into code no analysis
// covers is invisible to every rule. By default the change says so in a notice;
// under module_review.change_gate: fail it is unknown, with the file and the
// glob to add, never a pass.
func TestCheckUncoveredChange(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name      string
		cfg       string
		files     map[string]string
		wantInRsn []string
	}{
		{name: "a new unowned package", cfg: lintCleanModules + hookRuleSection, files: map[string]string{unownedPkgC: movedImport},
			wantInRsn: []string{"unowned", unownedPkgC, "pkg/c"}},
		{name: "a file of a language the config turned off", cfg: tsDisabled + lintCleanModules + hookRuleSection,
			files:     map[string]string{webAppTS: "import { secret } from '../pkg/b/impl';\nexport const x = secret;\n"},
			wantInRsn: []string{"language_disabled", webAppTS}},
		{name: "a file under an exclude: glob", cfg: excludeTools + lintCleanModules + hookRuleSection, files: map[string]string{"gen/g.go": movedImport},
			wantInRsn: []string{"excluded", "gen/g.go"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			// Default posture: the change passes and says what it did not read.
			warn := scopeRepo(t, tc.cfg, hookCleanA)
			commitFiles(t, warn, tc.files)
			_, res := checkUncovered(t, warn)
			if res.Change.Status != changePass || len(res.Change.Notices) != 1 || res.Change.Notices[0].Kind != "uncovered_change" {
				t.Fatalf("default posture: change %q notices %+v, want pass with an uncovered_change notice", res.Change.Status, res.Change.Notices)
			}

			// change_gate: fail: never a pass.
			strict := scopeRepo(t, tc.cfg+failChange, hookCleanA)
			commitFiles(t, strict, tc.files)
			code, res := checkUncovered(t, strict)
			if res.Change.Status != claimTypeUnknown || res.NextAction != "ask_owner" || code != 2 {
				t.Fatalf("change_gate: fail: change %q next_action %q exit %d, want unknown ask_owner 2", res.Change.Status, res.NextAction, code)
			}
			if len(res.Change.UnevaluatedRules) != 1 || res.Change.UnevaluatedRules[0].RuleID != "map/uncovered_change" {
				t.Fatalf("unevaluated rules = %+v, want the map/uncovered_change entry", res.Change.UnevaluatedRules)
			}
			for _, want := range tc.wantInRsn {
				if !strings.Contains(res.Change.UnevaluatedRules[0].Reason, want) {
					t.Errorf("reason %q is missing %q", res.Change.UnevaluatedRules[0].Reason, want)
				}
			}
		})
	}
}

// TestCheckUncoveredChangeIgnoresOldCode: code that was already unowned on the
// target is not the change's.
func TestCheckUncoveredChangeIgnoresOldCode(t *testing.T) {
	t.Parallel()
	dir := scopeRepo(t, lintCleanModules+hookRuleSection+failChange, hookCleanA)
	commitFiles(t, dir, map[string]string{filePkgAA: hookCleanA + "\n// touched, not added\n"})
	_, res := checkUncovered(t, dir)
	if res.Change.Status != changePass || len(res.Change.Notices) != 0 {
		t.Errorf("change %q notices %+v, want a plain pass", res.Change.Status, res.Change.Notices)
	}
}

// TestCheckUncoveredChangeEdges covers the ways added code used to slip past
// change_gate: fail: an untracked file on a fresh branch (the target-branch
// mode has no base), and a file in a directory the source walk skips.
func TestCheckUncoveredChangeEdges(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name  string
		setup func(t *testing.T) string
		want  string
	}{
		{name: "an untracked file on a branch with no commits yet", want: unownedPkgC, setup: func(t *testing.T) string {
			dir := scopeRepo(t, lintCleanModules+hookRuleSection+failChange, hookCleanA)
			writeFixtureFile(t, dir, unownedPkgC, movedImport)
			return dir
		}},
		{name: "an untracked file on the target branch itself", want: unownedPkgC, setup: func(t *testing.T) string {
			dir := scopeRepo(t, lintCleanModules+hookRuleSection+failChange, hookCleanA)
			gitFixture(t, dir, "checkout", "-q", branchMain)
			writeFixtureFile(t, dir, unownedPkgC, movedImport)
			return dir
		}},
		// target/ is skipped by the source walk but loaded by go/packages: the
		// file is unowned. A dot directory is read by nothing: unanalysed.
		{name: "a file in a directory the source walk skips", want: "pkg/target/t.go", setup: func(t *testing.T) string {
			dir := scopeRepo(t, lintCleanModules+hookRuleSection+failChange, hookCleanA)
			commitFiles(t, dir, map[string]string{"pkg/target/t.go": "package target\n\nimport \"example.com/test/pkg/b/impl\"\n\nvar _ = impl.Secret()\n"})
			return dir
		}},
		{name: "a file in a dot directory", want: ".tools/x.go", setup: func(t *testing.T) string {
			dir := scopeRepo(t, lintCleanModules+hookRuleSection+failChange, hookCleanA)
			commitFiles(t, dir, map[string]string{".tools/x.go": "package tools\n\nimport \"example.com/test/pkg/b/impl\"\n\nvar _ = impl.Secret()\n"})
			return dir
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			code, res := checkUncovered(t, tc.setup(t))
			if res.Change.Status != claimTypeUnknown || code != 2 {
				t.Fatalf("change %q exit %d, want unknown 2: unread added code must not pass", res.Change.Status, code)
			}
			if len(res.Change.UnevaluatedRules) != 1 || !strings.Contains(res.Change.UnevaluatedRules[0].Reason, tc.want) {
				t.Errorf("unevaluated = %+v, want it to name %s", res.Change.UnevaluatedRules, tc.want)
			}
		})
	}
}

// TestCheckUncoveredChangeUnderRoot: with --root a subtree, an untracked file
// in that subtree is still added.
func TestCheckUncoveredChangeUnderRoot(t *testing.T) {
	t.Parallel()
	repo := t.TempDir()
	sub := repo + "/svc"
	for name, content := range map[string]string{
		markerGoMod:       goModStub,
		filePkgAA:         hookCleanA,
		hookImplFile:      implSource(),
		defaultConfigPath: lintCleanModules + hookRuleSection + failChange,
	} {
		writeFixtureFile(t, sub, name, content)
	}
	gitInitFixtureRepo(t, repo)
	gitCommitFixture(t, repo)
	gitFixture(t, repo, "branch", "-M", branchMain)
	gitFixture(t, repo, "checkout", "-q", "-b", "feature")
	writeFixtureFile(t, sub, unownedPkgC, movedImport)

	code, out, stderr := runArchfit(t, cmdCheck, "-c", sub+"/"+defaultConfigPath, "--root", sub, "--format=agent")
	var res uncoveredResult
	if err := json.Unmarshal([]byte(out), &res); err != nil {
		t.Fatalf("decode: %v\nstdout: %s\nstderr: %s", err, out, stderr)
	}
	if res.Change.Status != claimTypeUnknown || code != 2 || len(res.Change.UnevaluatedRules) != 1 ||
		!strings.Contains(res.Change.UnevaluatedRules[0].Reason, unownedPkgC) {
		t.Fatalf("change %q exit %d unevaluated %+v, want unknown naming %s", res.Change.Status, code, res.Change.UnevaluatedRules, unownedPkgC)
	}
}
