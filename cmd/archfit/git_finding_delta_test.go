package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/alexei-led/archfit/internal/assessment/result"
	"github.com/alexei-led/archfit/internal/config"
	"github.com/alexei-led/archfit/internal/policy"
)

// TestGitFindingDelta covers the `--base` git-origin block: how tasks are placed
// (introduced / pre-existing / unknown), which analyzer evidence is comparable,
// which families are active for a config, and the end-to-end `check --base
// --json` contract at every gate exit code.
//
// One exported test function by design — cmd/archfit sits at its public_api_max
// ceiling, so new coverage arrives as subtests, never as new exported names.
func TestGitFindingDelta(t *testing.T) {
	t.Parallel()
	t.Run("effective_config", testGitDeltaEffectiveConfig)
	t.Run("check_base_json", testGitDeltaCheckBaseJSON)
}

// testGitDeltaEffectiveConfig covers the base sub-run's config contract: it gets
// the caller's effective config (flag overrides included) through an independent
// module map, so the head pipeline's owner and deploy-unit backfill cannot leak
// head-tree evidence into the base measurement.
func testGitDeltaEffectiveConfig(t *testing.T) {
	t.Parallel()
	t.Run("module map is independent of the head config", func(t *testing.T) {
		t.Parallel()
		original := config.Config{Modules: map[string]policy.ModuleDef{
			"a": {Paths: []string{"pkg/a/**"}},
		}}
		snapshot := original.WithIndependentModules()
		// Stand in for the pipeline's owner backfill, which writes through the map.
		original.FillMissingOwners(map[string]string{"a": "head-tree-owner"})
		if def := snapshot.Modules["a"]; def.Owner != "" {
			t.Errorf("base config inherited a head-tree owner %q", def.Owner)
		}
		if def := original.Modules["a"]; def.Owner != "head-tree-owner" {
			t.Errorf("head config lost its own backfill: %+v", def)
		}
	})

	// The call site, not the helper: analyze.go must snapshot the module map
	// BEFORE the head pipeline backfills owners into it. Replace
	// `WithIndependentModules(cfg)` with `cfg` there and the base run inherits
	// the head tree's per-module owners, classifies the shared edge at
	// cross_module_different_owner it never observed, and reports a critical
	// finding that makes a genuinely pre-existing seam look pre-existing for the
	// wrong reason — or, as here, hides that the seam is unchanged.
	t.Run("head-tree owners do not reach the base measurement", func(t *testing.T) {
		t.Parallel()
		cfgPath := gitDeltaOwnerFixtureRepo(t)
		code, stdout, stderr := runArchfit(t, cmdAnalyze, flagBase, diffBaseRef, fmtLegacyJSON, "-c", cfgPath)
		if code != 0 {
			t.Fatalf("analyze --base: exit = %d\nstdout:\n%s\nstderr:\n%s", code, stdout, stderr)
		}
		var got gitDeltaJSON
		if err := json.Unmarshal([]byte(stdout), &got); err != nil {
			t.Fatalf("invalid JSON: %v\n%s", err, stdout)
		}
		if got.GitFindingDelta == nil {
			t.Fatalf("--base --json must emit git_finding_delta\n%s", stdout)
		}
		// The head tree splits ownership, so the shared edge crosses owners; the
		// base tree has one owner, so the same edge is a same-owner sibling. The
		// two module maps therefore differ, and model_hash says so. A base run
		// that inherited the head module map would produce an EQUAL model_hash
		// and a comparable verdict — so the named model_hash mismatch IS the
		// isolation evidence.
		if got.StateComparison != "non_comparable" {
			t.Errorf("the base run measured head-tree owners: comparison = %q, want non_comparable on a differing module map", got.StateComparison)
		}
		if !slices.ContainsFunc(got.StateReasons, func(r string) bool { return strings.Contains(r, "model_hash") }) {
			t.Errorf("the comparability refusal must name model_hash, got %v", got.StateReasons)
		}
		// A non-comparable run publishes no numerical delta: an ownership split
		// moves the score without the code moving.
		if got.ScoreDelta != nil {
			t.Errorf("score_delta published beside a non_comparable comparison: %+v", got.ScoreDelta)
		}
		d := got.GitFindingDelta
		if len(d.ComparisonReasons) != 0 {
			t.Fatalf("fixture regression: both sides compile, want no comparison_reasons, got %v", d.ComparisonReasons)
		}
	})

	t.Run("analyzer overrides still reach the base run", func(t *testing.T) {
		t.Parallel()
		cfgPath := gitDeltaFixtureRepo(t, coupledModulesCfg)
		code, stdout, stderr := runArchfit(t, cmdAnalyze, flagBase, diffBaseRef, fmtLegacyJSON, "--lang", "go", "-c", cfgPath)
		if code != 0 {
			t.Fatalf("analyze --base --lang go: exit = %d\nstdout:\n%s\nstderr:\n%s", code, stdout, stderr)
		}
		var got gitDeltaJSON
		if err := json.Unmarshal([]byte(stdout), &got); err != nil {
			t.Fatalf("invalid JSON: %v\n%s", err, stdout)
		}
		if got.GitFindingDelta == nil {
			t.Fatalf("--base with a --lang override must still emit git_finding_delta\n%s", stdout)
		}
		// Both sides ran the same forced analyzer set, so nothing is unavailable.
		if len(got.GitFindingDelta.ComparisonReasons) != 0 {
			t.Errorf("comparison_reasons = %v, want none when both sides force the same analyzers",
				got.GitFindingDelta.ComparisonReasons)
		}
	})
}

// gitDeltaFixtureRepo builds a two-commit Go repo: the base commit holds only
// pkg/b, the head commit adds a pkg/a → pkg/b importer. The head run therefore
// carries a cross-module edge the base ref does not, and BOTH sides compile, so
// go/packages reports ok on both and the evidence is genuinely comparable.
func gitDeltaFixtureRepo(t *testing.T, cfgBody string) string {
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

// gitDeltaOwnerFixtureRepo builds a two-commit Go repo whose CODE stays put and
// whose OWNERSHIP moves: pkg/a → pkg/b exists in both commits, but the base
// commit gives the whole tree one owner while the head commit splits it in two.
// Neither module declares an owner, so each side must resolve its own from its
// own CODEOWNERS — which is exactly what the head pipeline's owner backfill
// would destroy if the base run shared its module map.
func gitDeltaOwnerFixtureRepo(t *testing.T) string {
	t.Helper()
	// min_severity keeps the same-owner form of the edge below the advisory
	// floor, so the two trees measure differently only because their owners
	// differ — which is exactly what a leaked head-tree module map would hide.
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

// gitDeltaJSON is the minimal decoder for the block under test.
type gitDeltaJSON struct {
	GitFindingDelta *struct {
		BaseRef           string   `json:"base_ref"`
		ComparisonStatus  string   `json:"comparison_status"`
		Introduced        []string `json:"introduced_finding_ids"`
		PreExisting       []string `json:"pre_existing_finding_ids"`
		UnknownOrigin     []string `json:"unknown_origin_finding_ids"`
		ComparisonReasons []string `json:"comparison_reasons"`
	} `json:"git_finding_delta"`
	AgentTasks []struct {
		FindingID string `json:"finding_id"`
		RuleID    string `json:"rule_id"`
	} `json:"agent_tasks"`
	ScoreDelta *struct {
		OverallDelta int `json:"overall_delta"`
	} `json:"score_delta"`
	// StateComparison is the run's own comparability verdict, disclosed beside
	// the legacy envelope so a consumer can tell an absent delta from a
	// non-comparable one.
	StateComparison string   `json:"comparison_status"`
	StateReasons    []string `json:"comparison_reasons"`
}

func testGitDeltaCheckBaseJSON(t *testing.T) {
	t.Parallel()
	const warnRule = `rules:
  - id: no-a-to-b
    type: forbidden_dependency
    gate: warn
    from: "pkg/a/**"
    to: "pkg/b/**"
`
	const failRule = `rules:
  - id: no-a-to-b
    type: forbidden_dependency
    gate: fail
    from: "pkg/a/**"
    to: "pkg/b/**"
`
	tests := []struct {
		name string
		// cfgBody selects the gate outcome; wantCode is the exit code the run
		// must produce with AND without --base (the delta is report-only).
		cfgBody string
		// wantIntroduced is the number of current repair tasks the base ref does
		// not carry. The head commit adds the only violating import, so a
		// blocking rule must attribute its task to this change.
		wantIntroduced int
		// extraArgs are appended to BOTH the --base run and the plain run, so
		// the report-only assertion still compares like with like.
		extraArgs []string
		wantCode  int
	}{
		// No rule to violate, so nothing blocks. The run is still
		// needs_attention (exit 2): v1 reports complexity, testability, and
		// operations partial by contract, and a partial dimension is never
		// reported as healthy.
		{name: "clean gate exits 2", cfgBody: coupledModulesCfg, wantCode: 2},
		{name: "blocking rule exits 1", cfgBody: coupledModulesCfg + failRule, wantIntroduced: 1, wantCode: 1},
		{name: "warning rule exits 2", cfgBody: coupledModulesCfg + warnRule, wantCode: 2},
		// The synthetic coupling-gate task's unknown-origin placement is pinned
		// in decision.TestGitFindingDelta instead: schema v2's seam gate blocks
		// only against a comparable reference, which no single CLI run can
		// produce, so the finding is unreachable from here.
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			cfgPath := gitDeltaFixtureRepo(t, tc.cfgBody)
			baseArgs := append([]string{cmdCheck, flagBase, diffBaseRef, fmtLegacyJSON, "-c", cfgPath}, tc.extraArgs...)
			code, stdout, stderr := runArchfit(t, baseArgs...)
			if code != tc.wantCode {
				t.Fatalf("check --base --json: exit = %d, want %d\nstdout:\n%s\nstderr:\n%s", code, tc.wantCode, stdout, stderr)
			}
			// Report-only: the same run without --base reaches the same verdict.
			plainArgs := append([]string{cmdCheck, fmtLegacyJSON, "-c", cfgPath}, tc.extraArgs...)
			if plain, _, _ := runArchfit(t, plainArgs...); plain != code {
				t.Errorf("--base changed the exit code: %d with, %d without", code, plain)
			}
			var got gitDeltaJSON
			if err := json.Unmarshal([]byte(stdout), &got); err != nil {
				t.Fatalf("invalid JSON: %v\n%s", err, stdout)
			}
			if got.GitFindingDelta == nil {
				t.Fatalf("--base --json must emit git_finding_delta\n%s", stdout)
			}
			d := got.GitFindingDelta
			if d.BaseRef != diffBaseRef {
				t.Errorf("base_ref = %q, want %q", d.BaseRef, diffBaseRef)
			}
			if d.Introduced == nil || d.PreExisting == nil || d.UnknownOrigin == nil || d.ComparisonReasons == nil {
				t.Errorf("every git_finding_delta list must be a non-null array: %s", stdout)
			}
			if len(d.ComparisonReasons) != 0 {
				t.Errorf("both fixture sides compile; want no comparison_reasons, got %v", d.ComparisonReasons)
			}
			if len(d.Introduced) != tc.wantIntroduced {
				t.Errorf("introduced_finding_ids = %v, want %d entr(y|ies)", d.Introduced, tc.wantIntroduced)
			}
			// Every current repair task lands in exactly one origin bucket.
			total := len(d.Introduced) + len(d.PreExisting) + len(d.UnknownOrigin)
			if total != len(got.AgentTasks) {
				t.Errorf("origin buckets hold %d ids for %d agent_tasks", total, len(got.AgentTasks))
			}
			if len(d.UnknownOrigin) == 0 && d.ComparisonStatus != result.GitComparisonComparable {
				t.Errorf("comparison_status = %q with no unknown-origin task", d.ComparisonStatus)
			}
			if len(d.UnknownOrigin) > 0 && d.ComparisonStatus != result.GitComparisonUnknown {
				t.Errorf("comparison_status = %q with %d unknown-origin tasks", d.ComparisonStatus, len(d.UnknownOrigin))
			}
			// Isolation: the base worktree is deleted before output is read, so
			// no base-side path may appear anywhere in the head report. Asserted
			// on the path SEGMENTS, not on a parent the test recomputes — see
			// baseWorktreeSegments.
			assertNoBaseWorktreeLeak(t, stdout)
		})
	}
}
