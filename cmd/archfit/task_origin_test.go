package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/alexei-led/archfit/internal/config"
	"github.com/alexei-led/archfit/internal/policy"
)

// TestTaskOrigin covers canonical --base task-origin reporting and verifies
// that the classification never changes the gate exit code.
func TestTaskOrigin(t *testing.T) {
	t.Parallel()
	t.Run("effective_config", testTaskOriginEffectiveConfig)
	t.Run("check_base_json", testTaskOriginCheckBaseJSON)
}

func testTaskOriginEffectiveConfig(t *testing.T) {
	t.Parallel()
	t.Run("module map is independent of the head config", func(t *testing.T) {
		t.Parallel()
		original := config.Config{Modules: map[string]policy.ModuleDef{
			"a": {Paths: []string{"pkg/a/**"}},
		}}
		snapshot := original.WithIndependentModules()
		original.FillMissingOwners(map[string]string{"a": "head-tree-owner"})
		if def := snapshot.Modules["a"]; def.Owner != "" {
			t.Errorf("base config inherited a head-tree owner %q", def.Owner)
		}
		if def := original.Modules["a"]; def.Owner != "head-tree-owner" {
			t.Errorf("head config lost its own backfill: %+v", def)
		}
	})

	t.Run("head-tree owners do not reach the base measurement", func(t *testing.T) {
		t.Parallel()
		cfgPath := taskOriginOwnerFixtureRepo(t)
		code, stdout, stderr := runArchfit(t, cmdAnalyze, flagBase, diffBaseRef, fmtJSON, "-c", cfgPath)
		if code != 0 {
			t.Fatalf("analyze --base: exit = %d\nstdout:\n%s\nstderr:\n%s", code, stdout, stderr)
		}
		var got taskOriginJSON
		if err := json.Unmarshal([]byte(stdout), &got); err != nil {
			t.Fatalf("invalid JSON: %v\n%s", err, stdout)
		}
		if got.Comparison.Status != "non_comparable" {
			t.Errorf("comparison = %q, want non_comparable on a differing module map", got.Comparison.Status)
		}
		if !slices.ContainsFunc(got.Comparison.Reasons, func(r string) bool { return strings.Contains(r, "model_hash") }) {
			t.Errorf("comparability refusal must name model_hash, got %v", got.Comparison.Reasons)
		}
	})

	t.Run("analyzer overrides reach the base run", func(t *testing.T) {
		t.Parallel()
		cfgPath := taskOriginFixtureRepo(t, coupledModulesCfg)
		code, stdout, stderr := runArchfit(t, cmdAnalyze, flagBase, diffBaseRef, fmtJSON, "--lang", "go", "-c", cfgPath)
		if code != 0 {
			t.Fatalf("analyze --base --lang go: exit = %d\nstdout:\n%s\nstderr:\n%s", code, stdout, stderr)
		}
		var got taskOriginJSON
		if err := json.Unmarshal([]byte(stdout), &got); err != nil {
			t.Fatalf("invalid JSON: %v\n%s", err, stdout)
		}
		if got.Comparison.TaskOriginStatus == "" {
			t.Fatalf("task_origin_status missing from canonical comparison: %s", stdout)
		}
		if len(got.Comparison.TaskOriginReasons) != 0 {
			t.Errorf("task_origin_reasons = %v, want none", got.Comparison.TaskOriginReasons)
		}
	})
}

func taskOriginFixtureRepo(t *testing.T, cfgBody string) string {
	t.Helper()
	dir := t.TempDir()
	write := func(name, content string) {
		path := filepath.Join(dir, name)
		if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	write(markerGoMod, "module example.com/test\n\ngo 1.21\n")
	write("pkg/b/api/api.go", "package api\n\nfunc Secret() string { return \"s\" }\n")
	write(defaultConfigPath, cfgBody)
	gitInitFixtureRepo(t, dir)
	gitCommitAll(t, dir, "base: pkg/b only")
	write("pkg/a/a.go", "package a\n\nimport \"example.com/test/pkg/b/api\"\n\n"+
		"func UseSecret() string { return api.Secret() }\n")
	gitCommitAll(t, dir, "head: add the cross-module importer")
	return filepath.Join(dir, defaultConfigPath)
}

func taskOriginOwnerFixtureRepo(t *testing.T) string {
	t.Helper()
	const ownerSplitCfg = `version: 2
modules:
  a:
    paths: ["pkg/a/**"]
  b:
    paths: ["pkg/b/**"]
coupling:
  min_severity: high
`
	dir := t.TempDir()
	write := func(name, content string) {
		path := filepath.Join(dir, name)
		if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	write(markerGoMod, "module example.com/test\n\ngo 1.21\n")
	write("pkg/b/api/api.go", "package api\n\nfunc Secret() string { return \"s\" }\n")
	write("pkg/a/a.go", "package a\n\nimport \"example.com/test/pkg/b/api\"\n\n"+
		"func UseSecret() string { return api.Secret() }\n")
	write(defaultConfigPath, ownerSplitCfg)
	write(".github/CODEOWNERS", "* @team-one\n")
	gitInitFixtureRepo(t, dir)
	gitCommitAll(t, dir, "base: one owner for the whole tree")
	write(".github/CODEOWNERS", "/pkg/a/ @team-a\n/pkg/b/ @team-b\n")
	gitCommitAll(t, dir, "head: split ownership in two")
	return filepath.Join(dir, defaultConfigPath)
}

type taskOriginJSON struct {
	Comparison struct {
		Status            string   `json:"status"`
		Reasons           []string `json:"reasons"`
		TaskOriginStatus  string   `json:"task_origin_status"`
		TaskOriginReasons []string `json:"task_origin_reasons"`
	} `json:"comparison"`
	AgentTasks []struct {
		FindingID string `json:"finding_id"`
		RuleID    string `json:"rule_id"`
		Origin    string `json:"origin"`
	} `json:"agent_tasks"`
}

func testTaskOriginCheckBaseJSON(t *testing.T) {
	t.Parallel()
	const failRule = `rules:
  - id: no-a-to-b
    type: forbidden_dependency
    gate: fail
    from: "pkg/a/**"
    to: "pkg/b/**"
`
	tests := []struct {
		name           string
		cfgBody        string
		wantCode       int
		wantIntroduced int
	}{
		{name: "clean gate exits 2", cfgBody: coupledModulesCfg, wantCode: 2},
		{name: "blocking rule exits 1", cfgBody: coupledModulesCfg + failRule, wantCode: 1, wantIntroduced: 1},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			cfgPath := taskOriginFixtureRepo(t, tc.cfgBody)
			code, stdout, stderr := runArchfit(t, cmdCheck, flagBase, diffBaseRef, fmtJSON, "-c", cfgPath)
			if code != tc.wantCode {
				t.Fatalf("check --base --json: exit = %d, want %d\nstdout:\n%s\nstderr:\n%s", code, tc.wantCode, stdout, stderr)
			}
			if plain, _, _ := runArchfit(t, cmdCheck, fmtJSON, "-c", cfgPath); plain != code {
				t.Errorf("--base changed exit code: %d with, %d without", code, plain)
			}
			var got taskOriginJSON
			if err := json.Unmarshal([]byte(stdout), &got); err != nil {
				t.Fatalf("invalid JSON: %v\n%s", err, stdout)
			}
			if got.Comparison.TaskOriginStatus == "" {
				t.Fatalf("task_origin_status missing: %s", stdout)
			}
			introduced := 0
			for _, task := range got.AgentTasks {
				if task.Origin == "" {
					t.Errorf("task %s has no origin", task.FindingID)
				}
				if task.Origin == "introduced" {
					introduced++
				}
			}
			if introduced != tc.wantIntroduced {
				t.Errorf("introduced tasks = %d, want %d", introduced, tc.wantIntroduced)
			}
			assertNoBaseWorktreeLeak(t, stdout)
		})
	}
}
