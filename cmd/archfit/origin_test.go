package main

import (
	"encoding/json"
	"maps"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/alexei-led/archfit/v3/internal/config"
	"github.com/alexei-led/archfit/v3/internal/policy"
)

// TestOrigin covers canonical --base origin reporting and verifies that the
// classification never changes the gate exit code.
func TestOrigin(t *testing.T) {
	t.Parallel()
	t.Run("effective_config", testOriginEffectiveConfig)
	t.Run("check_base_json", testOriginCheckBaseJSON)
	t.Run("go_only_findings", testOriginGoOnlyFindings)
}

func testOriginEffectiveConfig(t *testing.T) {
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
		if got.Comparison.OriginStatus == "" {
			t.Fatalf("origin_status missing from canonical comparison: %s", stdout)
		}
		if len(got.Comparison.OriginReasons) != 0 {
			t.Errorf("origin_reasons = %v, want none", got.Comparison.OriginReasons)
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
		Status               string    `json:"status"`
		Reasons              []string  `json:"reasons"`
		OriginStatus         string    `json:"origin_status"`
		OriginReasons        []string  `json:"origin_reasons"`
		IntroducedFindingIDs *[]string `json:"introduced_finding_ids"`
		ResolvedFindingIDs   *[]string `json:"resolved_finding_ids"`
	} `json:"comparison"`
	Findings []struct {
		ID     string `json:"id"`
		RuleID string `json:"rule_id"`
		Status string `json:"status"`
		Origin string `json:"origin"`
		Edge   struct {
			From struct {
				Path string `json:"path"`
			} `json:"from"`
		} `json:"edge"`
	} `json:"findings"`
	AgentTasks []struct {
		FindingID string `json:"finding_id"`
		RuleID    string `json:"rule_id"`
		Origin    string `json:"origin"`
	} `json:"agent_tasks"`
}

func testOriginCheckBaseJSON(t *testing.T) {
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
			if got.Comparison.OriginStatus == "" {
				t.Fatalf("origin_status missing: %s", stdout)
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

// testOriginGoOnlyFindings pins the one classifier end to end on a Go-only
// tree, where every non-Go analyzer is not applicable: one edge pre-dates the
// base ref, one is added, one is removed. A measurement-profile change that
// drops not-applicable producers must keep all three answers.
func testOriginGoOnlyFindings(t *testing.T) {
	t.Parallel()
	const cfg = coupledModulesCfg + `rules:
  - id: no-x-to-b
    type: forbidden_dependency
    gate: fail
    from: "pkg/{a,c,d}/**"
    to: "pkg/b/**"
`
	const importer = "import \"example.com/test/pkg/b/api\"\n\nfunc Use() string { return api.Secret() }\n"
	dir := t.TempDir()
	writeFileAt(t, dir, markerGoMod, "module example.com/test\n\ngo 1.21\n")
	writeFileAt(t, dir, "pkg/b/api/api.go", "package api\n\nfunc Secret() string { return \"s\" }\n")
	writeFileAt(t, dir, "pkg/a/a.go", "package a\n\n"+importer)
	writeFileAt(t, dir, "pkg/d/d.go", "package d\n\n"+importer)
	writeFileAt(t, dir, defaultConfigPath, cfg)
	gitInitFixtureRepo(t, dir)
	gitCommitAll(t, dir, "base: a and d import b")
	if err := os.Remove(filepath.Join(dir, "pkg/d/d.go")); err != nil {
		t.Fatal(err)
	}
	writeFileAt(t, dir, "pkg/d/d.go", "package d\n")
	writeFileAt(t, dir, "pkg/c/c.go", "package c\n\n"+importer)
	gitCommitAll(t, dir, "head: c imports b, d no longer does")
	cfgPath := filepath.Join(dir, defaultConfigPath)

	code, stdout, stderr := runArchfit(t, cmdCheck, flagBase, diffBaseRef, fmtJSON, "-c", cfgPath)
	if code != 1 {
		t.Fatalf("check --base: exit = %d, want 1\nstdout:\n%s\nstderr:\n%s", code, stdout, stderr)
	}
	var got taskOriginJSON
	if err := json.Unmarshal([]byte(stdout), &got); err != nil {
		t.Fatalf("invalid JSON: %v\n%s", err, stdout)
	}
	if got.Comparison.OriginStatus != "comparable" {
		t.Fatalf("origin_status = %q, want comparable (reasons %v)", got.Comparison.OriginStatus, got.Comparison.OriginReasons)
	}
	byFrom := map[string]string{}
	idByFrom := map[string]string{}
	for _, f := range got.Findings {
		if f.RuleID == "no-x-to-b" {
			byFrom[f.Edge.From.Path] = f.Origin
			idByFrom[f.Edge.From.Path] = f.ID
		}
	}
	want := map[string]string{"pkg/a/a.go": "pre_existing", "pkg/c/c.go": "introduced"}
	if !maps.Equal(byFrom, want) {
		t.Errorf("finding origins by importer = %v, want %v", byFrom, want)
	}
	if ids := got.Comparison.IntroducedFindingIDs; ids == nil || !slices.Equal(*ids, []string{idByFrom["pkg/c/c.go"]}) {
		t.Errorf("introduced_finding_ids = %v, want [%s]", ids, idByFrom["pkg/c/c.go"])
	}
	if ids := got.Comparison.ResolvedFindingIDs; ids == nil || len(*ids) != 1 {
		t.Errorf("resolved_finding_ids = %v, want the removed pkg/d edge", ids)
	}
	for _, task := range got.AgentTasks {
		for from, id := range idByFrom {
			if task.FindingID == id && task.Origin != byFrom[from] {
				t.Errorf("task %s origin = %q, finding origin = %q: one classifier, one answer", id, task.Origin, byFrom[from])
			}
		}
	}

	code, text, _ := runArchfit(t, cmdCheck, flagBase, diffBaseRef, "-c", cfgPath)
	if code != 1 || !strings.Contains(text, "origin: introduced") || !strings.Contains(text, "introduced: 1  ·  resolved: 1") {
		t.Errorf("text brief must show blocker origins and the origin counts (exit %d):\n%s", code, text)
	}

	_, plain, _ := runArchfit(t, cmdCheck, fmtJSON, "-c", cfgPath)
	if strings.Contains(plain, `"origin`) || strings.Contains(plain, "introduced_finding_ids") {
		t.Error("a run without --base must carry no origin key")
	}
}
