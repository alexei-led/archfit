package main

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"testing"

	"github.com/alexei-led/archfit/internal/assessment/finding"
	"github.com/alexei-led/archfit/internal/baseline"
	"github.com/alexei-led/archfit/internal/model/report"
)

const (
	reachabilityFixturePath            = "../../testdata/fixtures/reachability"
	reachabilityCoveragePlaceholder    = "{{REACHABILITY_COVERAGE_BLOCK}}"
	reachabilityTrackedCoveredSource   = "services/api/api.go"
	reachabilityUntrackedCoveredSource = "services/api/untracked.go"
	reachabilityIgnoredCoveredSource   = "services/app/ignored.go"
	reachabilityReasonWorktreeDiffers  = "worktree_differs_from_ref"
)

type reachabilityCoverageSidecar struct {
	SchemaVersion int               `json:"schema_version"`
	SourceRef     string            `json:"source_ref"`
	Modules       []string          `json:"modules"`
	Sources       map[string]string `json:"sources"`
}

// TestIntegrationReachability drives the only supported materialization path for
// this fixture. Its terminal baseline transition requires the healthy state that
// the completed collectors make reachable.
func TestIntegrationReachability(t *testing.T) {
	t.Run("baseline_transition", testIntegrationReachabilityBaselineTransition)
	t.Run("drift_lifecycle", testIntegrationReachabilityDriftLifecycle)
	t.Run("testability_coverage", testIntegrationReachabilityTestabilityCoverage)
}

func testIntegrationReachabilityBaselineTransition(t *testing.T) {
	root, configPath := materializeFixture(t, true)

	firstCode, first := runReachabilityState(t, cmdAnalyze, configPath)
	secondCode, second := runReachabilityState(t, cmdAnalyze, configPath)
	if firstCode != 0 || secondCode != 0 {
		t.Fatalf("pre-baseline analyze exits = %d/%d, want 0/0", firstCode, secondCode)
	}
	if !bytes.Equal(first, second) {
		t.Fatalf("two identical pre-baseline analyzes differ:\n%s", firstDiffLine(string(first), string(second)))
	}
	preBaseline := decodeReachabilityState(t, first)
	assertReachabilityEnvelope(t, first)
	if preBaseline.Dimensions.Drift.Status != report.MeasurementUnmeasured {
		t.Fatalf("pre-baseline drift status = %q, want unmeasured so the baseline transition is exercised",
			preBaseline.Dimensions.Drift.Status)
	}

	baselineCode, baselineOut, baselineErr := runArchfit(t, cmdBaseline, "-c", configPath, flagRefresh)
	if baselineCode != 0 {
		t.Fatalf("baseline: exit=%d\nstdout:\n%s\nstderr:\n%s", baselineCode, baselineOut, baselineErr)
	}

	postCode, post := runReachabilityState(t, cmdAnalyze, configPath)
	repeatCode, repeat := runReachabilityState(t, cmdAnalyze, configPath)
	if postCode != 0 || repeatCode != 0 {
		t.Fatalf("post-baseline analyze exits = %d/%d, want 0/0", postCode, repeatCode)
	}
	if !bytes.Equal(post, repeat) {
		t.Fatalf("two identical post-baseline analyzes differ:\n%s", firstDiffLine(string(post), string(repeat)))
	}
	state := decodeReachabilityState(t, post)
	if state.Dimensions.Drift.Status != report.MeasurementMeasured {
		t.Fatalf("post-baseline drift status = %q, want measured; unknown=%+v delta=%+v",
			state.Dimensions.Drift.Status, state.Dimensions.Drift.Unknown, state.Dimensions.Drift.Delta)
	}
	assertReachabilityOperations(t, state.Dimensions.Operations)
	assertReachabilityComplexity(t, state.Dimensions.Complexity)
	for _, unknown := range state.Dimensions.Drift.Unknown {
		if strings.Contains(strings.ToLower(unknown.Reason), "comparable") {
			t.Errorf("post-baseline drift still reports a comparability blocker: %+v", unknown)
		}
	}

	checkCode, check := runReachabilityState(t, cmdCheck, configPath)
	if !bytes.Equal(post, check) {
		t.Fatalf("analyze and check JSON differ on the same persisted baseline:\n%s",
			firstDiffLine(string(post), string(check)))
	}
	assertReachabilityExit(t, state.Verdict, checkCode)

	activeDiagnostics := activeReachabilityDiagnostics(state.Findings)
	logReachabilityState(t, state, checkCode, activeDiagnostics)
	assertReachabilityOutcomeA(t, state, checkCode, activeDiagnostics)

	// Keep root live so a future edit cannot accidentally make the fixture path
	// escape the test-owned repository without this test noticing.
	if !strings.HasPrefix(configPath, root+string(filepath.Separator)) {
		t.Fatalf("rendered config %q is outside materialized root %q", configPath, root)
	}
}

func testIntegrationReachabilityDriftLifecycle(t *testing.T) {
	root, configPath := materializeFixture(t, false)

	t.Run("comparison_not_requested_without_persisted_baseline", func(t *testing.T) {
		code, raw := runReachabilityState(t, cmdAnalyze, configPath)
		if code != 0 {
			t.Fatalf("analyze without a baseline: exit=%d, want 0", code)
		}
		state := decodeReachabilityState(t, raw)
		if state.Comparison.Status != report.ComparisonNotRequested {
			t.Fatalf("root comparison status = %q, want not_requested", state.Comparison.Status)
		}
		reason := assertReachabilityUnmeasuredDrift(t, state, "no comparable architecture-state reference is stored")
		if strings.Contains(reason, "the stored baseline was written under different inputs") {
			t.Fatalf("missing-baseline reason is indistinguishable from an incomparable baseline: %q", reason)
		}
	})

	t.Run("comparison_not_requested_with_incomparable_persisted_baseline", func(t *testing.T) {
		baselineCode, baselineOut, baselineErr := runArchfit(t, cmdBaseline, "-c", configPath, flagRefresh)
		if baselineCode != 0 {
			t.Fatalf("baseline: exit=%d\nstdout:\n%s\nstderr:\n%s", baselineCode, baselineOut, baselineErr)
		}
		makeReachabilityBaselineIncomparable(t, root)

		code, raw := runReachabilityState(t, cmdAnalyze, configPath)
		if code != 0 {
			t.Fatalf("analyze with an incomparable baseline: exit=%d, want 0", code)
		}
		state := decodeReachabilityState(t, raw)
		if state.Comparison.Status != report.ComparisonNotRequested {
			t.Fatalf("root comparison status = %q, want not_requested", state.Comparison.Status)
		}
		reason := assertReachabilityUnmeasuredDrift(t, state, "the stored baseline was written under different inputs")
		if !strings.Contains(reason, "config_hash") {
			t.Fatalf("incomparable-baseline reason %q does not name the drifted config_hash", reason)
		}
		if strings.Contains(reason, "no comparable architecture-state reference is stored") {
			t.Fatalf("incomparable-baseline reason is indistinguishable from a missing baseline: %q", reason)
		}
	})
}

func testIntegrationReachabilityTestabilityCoverage(t *testing.T) {
	t.Run("fresh_full_module_coverage_is_measured", func(t *testing.T) {
		_, configPath := materializeFixture(t, true)
		code, raw := runReachabilityState(t, cmdAnalyze, configPath)
		if code != 0 {
			t.Fatalf("analyze with fresh coverage: exit=%d, want 0", code)
		}
		assertReachabilityTestabilityMeasured(t, decodeReachabilityState(t, raw))
	})

	t.Run("partial_coverage_keeps_five_module_denominator", func(t *testing.T) {
		root, configPath := materializeFixture(t, true)
		// These production modules are added after the coverage artifact and
		// sidecar were produced. The artifact still matches every source byte it
		// represents, but attribution must disclose the two uncovered modules.
		writeReachabilitySource(t, root, "services/missing-d/missing.go", "package missingd\n\nfunc uncoveredD() {}\n")
		writeReachabilitySource(t, root, "services/missing-e/missing.go", "package missinge\n\nfunc uncoveredE() {}\n")
		addReachabilityPartialCoverageModules(t, configPath)

		_, raw := runReachabilityState(t, cmdAnalyze, configPath)
		state := decodeReachabilityState(t, raw)
		dim := state.Dimensions.Testability
		if dim.Status != report.MeasurementPartial {
			t.Fatalf("3/5 testability status = %q, want partial; unknown=%+v", dim.Status, dim.Unknown)
		}
		metric := reachabilityDimensionMetrics(dim)["modules_with_coverage"]
		if metric.Denominator == nil || metric.Denominator.Observed != 3 || metric.Denominator.Total != 5 {
			t.Fatalf("modules_with_coverage denominator = %+v, want 3/5", metric.Denominator)
		}
		reasons := make([]string, 0, len(dim.Unknown))
		for _, unknown := range dim.Unknown {
			reasons = append(reasons, unknown.Reason)
		}
		joined := strings.Join(reasons, " ")
		for _, module := range []string{"missing-d", "missing-e"} {
			if !strings.Contains(joined, module) {
				t.Errorf("partial attribution reasons %q do not name %q", joined, module)
			}
		}
	})

	t.Run("source_ref_is_metadata_when_hashes_match", func(t *testing.T) {
		root, configPath := materializeFixture(t, true)
		mutateReachabilitySidecar(t, root, func(sidecar *reachabilityCoverageSidecar) {
			sidecar.SourceRef = strings.Repeat("f", sha256.Size*2)
		})
		_, raw := runReachabilityState(t, cmdAnalyze, configPath)
		assertReachabilityTestabilityMeasured(t, decodeReachabilityState(t, raw))
	})

	for _, tc := range []struct {
		name       string
		wantReason string
		mutate     func(t *testing.T, root string)
	}{
		{
			name: "tracked_covered_source_modified", wantReason: reachabilityReasonWorktreeDiffers,
			mutate: func(t *testing.T, root string) {
				appendReachabilitySource(t, root, reachabilityTrackedCoveredSource, "\n// tracked source changed after coverage\n")
			},
		},
		{
			name: "untracked_covered_source_modified", wantReason: reachabilityReasonWorktreeDiffers,
			mutate: func(t *testing.T, root string) {
				appendReachabilitySource(t, root, reachabilityUntrackedCoveredSource, "\n// untracked source changed after coverage\n")
			},
		},
		{
			name: "gitignored_covered_source_modified_on_git_clean_tree", wantReason: reachabilityReasonWorktreeDiffers,
			mutate: func(t *testing.T, root string) {
				excludePath := filepath.Join(root, ".git", "info", "exclude")
				if err := os.MkdirAll(filepath.Dir(excludePath), 0o750); err != nil {
					t.Fatalf("create git info directory: %v", err)
				}
				if err := os.WriteFile(excludePath, []byte(reachabilityUntrackedCoveredSource+"\n"), 0o600); err != nil {
					t.Fatalf("exclude untracked fixture source: %v", err)
				}
				appendReachabilitySource(t, root, reachabilityIgnoredCoveredSource, "\n// ignored source changed after coverage\n")
				status := strings.TrimSpace(runReachabilityTool(t, root, deterministicReachabilityEnv(), "git", "status", "--porcelain"))
				if status != "" {
					t.Fatalf("ignored-source fixture is not git-clean: %q", status)
				}
			},
		},
		{
			name: "listed_covered_source_deleted", wantReason: reachabilityReasonWorktreeDiffers,
			mutate: func(t *testing.T, root string) {
				if err := os.Remove(filepath.Join(root, filepath.FromSlash(reachabilityIgnoredCoveredSource))); err != nil {
					t.Fatalf("delete listed covered source: %v", err)
				}
			},
		},
		{
			name: "sidecar_missing", wantReason: "freshness_unverified",
			mutate: func(t *testing.T, root string) {
				if err := os.Remove(filepath.Join(root, "coverage.out.sidecar.json")); err != nil {
					t.Fatalf("remove coverage sidecar: %v", err)
				}
			},
		},
		{
			name: "sidecar_schema_unrecognized", wantReason: "freshness_unverified",
			mutate: func(t *testing.T, root string) {
				mutateReachabilitySidecar(t, root, func(sidecar *reachabilityCoverageSidecar) { sidecar.SchemaVersion = 2 })
			},
		},
		{
			name: "sidecar_source_hash_mismatch", wantReason: reachabilityReasonWorktreeDiffers,
			mutate: func(t *testing.T, root string) {
				mutateReachabilitySidecar(t, root, func(sidecar *reachabilityCoverageSidecar) {
					sidecar.Sources[reachabilityTrackedCoveredSource] = strings.Repeat("0", sha256.Size*2)
				})
			},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			root, configPath := materializeFixture(t, true)
			tc.mutate(t, root)
			_, raw := runReachabilityState(t, cmdAnalyze, configPath)
			assertReachabilityTestabilityPartial(t, decodeReachabilityState(t, raw), tc.wantReason)
		})
	}

	t.Run("warm_cache_recomputes_freshness", func(t *testing.T) {
		root, configPath := materializeFixture(t, true)
		_, firstRaw := runReachabilityStateWithCache(t, cmdAnalyze, configPath, false)
		assertReachabilityTestabilityMeasured(t, decodeReachabilityState(t, firstRaw))

		appendReachabilitySource(t, root, reachabilityTrackedCoveredSource, "\n// edit after the coverage fact cache is warm\n")
		_, secondRaw := runReachabilityStateWithCache(t, cmdAnalyze, configPath, false)
		assertReachabilityTestabilityPartial(t, decodeReachabilityState(t, secondRaw), reachabilityReasonWorktreeDiffers)
	})
}

func addReachabilityPartialCoverageModules(t *testing.T, configPath string) {
	t.Helper()
	data, err := os.ReadFile(configPath) //nolint:gosec // test-owned config path
	if err != nil {
		t.Fatalf("read reachability config: %v", err)
	}
	const modules = `modules:
  coverage-extra:
    paths: ["services/api/untracked.go"]
  missing-d:
    paths: ["services/missing-d/**"]
  missing-e:
    paths: ["services/missing-e/**"]
`
	rendered := strings.Replace(string(data), "\nmodules:\n", "\n"+modules, 1)
	if rendered == string(data) {
		t.Fatal("reachability config has no modules block to extend")
	}
	// #nosec G703 -- configPath is the fixed config filename under a test-owned temporary root.
	if err := os.WriteFile(configPath, []byte(rendered), 0o600); err != nil {
		t.Fatalf("extend reachability config modules: %v", err)
	}
}

func appendReachabilitySource(t *testing.T, root, rel, suffix string) {
	t.Helper()
	path := filepath.Join(root, filepath.FromSlash(rel))
	data, err := os.ReadFile(path) //nolint:gosec // test-owned root and controlled relative path
	if err != nil {
		t.Fatalf("read source %s: %v", rel, err)
	}
	// #nosec G703 -- path is contained by the test-owned temporary fixture root.
	if err := os.WriteFile(path, append(data, suffix...), 0o600); err != nil {
		t.Fatalf("modify source %s: %v", rel, err)
	}
}

func assertReachabilityTestabilityMeasured(t *testing.T, state report.ArchitectureState) {
	t.Helper()
	dim := state.Dimensions.Testability
	if dim.Status != report.MeasurementMeasured {
		t.Fatalf("testability status = %q, want measured; unknown=%+v metrics=%+v", dim.Status, dim.Unknown, dim.Metrics)
	}
	if dim.Coverage.Observed != 2 || dim.Coverage.Total != 2 {
		t.Fatalf("testability module coverage = %d/%d, want 2/2", dim.Coverage.Observed, dim.Coverage.Total)
	}
	metrics := reachabilityDimensionMetrics(dim)
	if metrics["modules_with_coverage"].Denominator == nil || metrics["modules_with_coverage"].Denominator.Total != 2 {
		t.Fatalf("modules_with_coverage = %+v, want declared-module denominator 2", metrics["modules_with_coverage"])
	}
	if metrics["unresolved_coverage_paths"].Value != 0 {
		t.Fatalf("unresolved_coverage_paths = %v, want 0", metrics["unresolved_coverage_paths"].Value)
	}
	if metrics["test_files"].Value != 0 || metrics["coverage_ratio"].Value != 0 {
		t.Fatalf("zero-test fixture static/coverage metrics = test_files %v coverage_ratio %v, want 0/0",
			metrics["test_files"].Value, metrics["coverage_ratio"].Value)
	}
	if !strings.Contains(strings.Join(metrics["coverage_ratio"].Provenance, " "), "matched") {
		t.Fatalf("measured coverage provenance does not carry matched freshness: %+v", metrics["coverage_ratio"].Provenance)
	}
}

func assertReachabilityTestabilityPartial(t *testing.T, state report.ArchitectureState, wantReason string) {
	t.Helper()
	dim := state.Dimensions.Testability
	if dim.Status != report.MeasurementPartial {
		t.Fatalf("testability status = %q, want partial; unknown=%+v metrics=%+v", dim.Status, dim.Unknown, dim.Metrics)
	}
	reasons := make([]string, 0, len(dim.Unknown))
	for _, unknown := range dim.Unknown {
		reasons = append(reasons, unknown.Reason)
	}
	if joined := strings.Join(reasons, " "); !strings.Contains(joined, wantReason) {
		t.Fatalf("testability unknown reasons %q do not contain %q", joined, wantReason)
	}
	metrics := reachabilityDimensionMetrics(dim)
	if _, ok := metrics["coverage_ratio"]; !ok {
		t.Fatalf("partial freshness discarded the observed ratio: %+v", dim.Metrics)
	}
}

func reachabilityDimensionMetrics(dim report.DimensionState) map[string]report.MetricValue {
	metrics := make(map[string]report.MetricValue, len(dim.Metrics))
	for _, metric := range dim.Metrics {
		metrics[metric.Name] = metric
	}
	return metrics
}

func assertReachabilityUnmeasuredDrift(t *testing.T, state report.ArchitectureState, wantReason string) string {
	t.Helper()
	drift := state.Dimensions.Drift
	if drift.Status != report.MeasurementUnmeasured {
		t.Fatalf("drift status = %q, want unmeasured; unknown=%+v delta=%+v", drift.Status, drift.Unknown, drift.Delta)
	}
	if drift.Delta == nil {
		t.Fatal("unmeasured drift carries no comparison delta")
	}
	if drift.Delta.Status != report.ComparisonNonComparable {
		t.Fatalf("drift delta status = %q, want non_comparable", drift.Delta.Status)
	}
	deltaReason := strings.Join(drift.Delta.Reasons, " ")
	if !strings.Contains(deltaReason, wantReason) {
		t.Fatalf("drift delta reason %q does not contain %q", deltaReason, wantReason)
	}
	unknownReasons := make([]string, 0, len(drift.Unknown))
	for _, unknown := range drift.Unknown {
		unknownReasons = append(unknownReasons, unknown.Reason)
	}
	if joined := strings.Join(unknownReasons, " "); !strings.Contains(joined, wantReason) {
		t.Fatalf("drift unknown reasons %q do not contain %q", joined, wantReason)
	}
	return deltaReason
}

func makeReachabilityBaselineIncomparable(t *testing.T, root string) {
	t.Helper()
	path := filepath.Join(root, defaultBaselinePath)
	stored, err := baseline.Load(t.Context(), path)
	if err != nil {
		t.Fatalf("load captured baseline: %v", err)
	}
	if stored.State == nil {
		t.Fatal("captured baseline has no architecture-state snapshot")
	}
	driftedHash := strings.Repeat("0", 64)
	if stored.State.ConfigHash == driftedHash {
		driftedHash = strings.Repeat("1", 64)
	}
	stored.State.ConfigHash = driftedHash
	if err := baseline.Save(t.Context(), path, stored); err != nil {
		t.Fatalf("write incomparable baseline: %v", err)
	}
}

// materializeFixture copies committed source into a fresh repository, creates
// exactly one deterministic commit, optionally emits supplied coverage plus its
// sidecar, and finally renders .archfit.yaml. No test runs the committed fixture
// directory directly, because it has neither its own git history nor baseline.
func materializeFixture(t *testing.T, withCoverage bool) (root, configPath string) {
	t.Helper()

	_, thisFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller failed")
	}
	fixture, err := filepath.Abs(filepath.Join(filepath.Dir(thisFile), reachabilityFixturePath))
	if err != nil {
		t.Fatalf("resolve reachability fixture: %v", err)
	}
	root = t.TempDir()
	if err := copyFixtureIntoDir(fixture, root); err != nil {
		t.Fatalf("copy reachability fixture: %v", err)
	}
	if err := os.WriteFile(filepath.Join(root, ".gitignore"), []byte(".archfit.yaml\n.archfit-baseline.json\ncoverage.out*\nservices/app/ignored.go\n"), 0o600); err != nil {
		t.Fatalf("write fixture gitignore: %v", err)
	}

	initReachabilityGitRepo(t, root)
	if withCoverage {
		writeReachabilityCoverage(t, root)
	}
	configPath = renderReachabilityConfig(t, root, withCoverage)
	return root, configPath
}

func initReachabilityGitRepo(t *testing.T, root string) {
	t.Helper()
	env := deterministicReachabilityEnv()
	runReachabilityTool(t, root, env, "git", "-c", "init.defaultBranch=main", "-c", "init.templateDir=", "init", "-q")
	runReachabilityTool(t, root, env, "git", "-c", "core.autocrlf=false", "add", "-A")
	runReachabilityTool(t, root, env, "git", "-c", "commit.gpgSign=false", "-c", "core.hooksPath=/dev/null",
		"commit", "-q", "-m", "reachability fixture")
	count := strings.TrimSpace(runReachabilityTool(t, root, env, "git", "rev-list", "--count", "HEAD"))
	if count != "1" {
		t.Fatalf("fixture commit count = %q, want exactly 1", count)
	}
}

func deterministicReachabilityEnv() []string {
	blocked := map[string]struct{}{
		"GIT_AUTHOR_NAME": {}, "GIT_AUTHOR_EMAIL": {}, "GIT_AUTHOR_DATE": {},
		"GIT_COMMITTER_NAME": {}, "GIT_COMMITTER_EMAIL": {}, "GIT_COMMITTER_DATE": {},
		"GIT_CONFIG_GLOBAL": {}, "GIT_CONFIG_NOSYSTEM": {}, "GOWORK": {}, "GOFLAGS": {},
		"GOPROXY": {}, "GOSUMDB": {}, "LC_ALL": {}, "TZ": {},
	}
	env := scrubGitFixtureEnv(os.Environ())
	filtered := make([]string, 0, len(env)+14)
	for _, entry := range env {
		name, _, found := strings.Cut(entry, "=")
		if found {
			if _, remove := blocked[name]; remove {
				continue
			}
		}
		filtered = append(filtered, entry)
	}
	return append(filtered,
		"GIT_AUTHOR_NAME=archfit fixture",
		"GIT_AUTHOR_EMAIL=fixture@example.invalid",
		"GIT_AUTHOR_DATE=2000-01-01T00:00:00Z",
		"GIT_COMMITTER_NAME=archfit fixture",
		"GIT_COMMITTER_EMAIL=fixture@example.invalid",
		"GIT_COMMITTER_DATE=2000-01-01T00:00:00Z",
		"GIT_CONFIG_GLOBAL=/dev/null",
		"GIT_CONFIG_NOSYSTEM=1",
		"GOWORK=off",
		"GOFLAGS=",
		"GOPROXY=off",
		"GOSUMDB=off",
		"LC_ALL=C",
		"TZ=UTC",
	)
}

func runReachabilityTool(t *testing.T, dir string, env []string, name string, args ...string) string {
	t.Helper()
	cmd := exec.Command(name, args...) //nolint:gosec // fixed test tools and controlled fixture arguments
	cmd.Dir = dir
	cmd.Env = env
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("%s %s: %v\n%s", name, strings.Join(args, " "), err, out)
	}
	return string(out)
}

func writeReachabilityCoverage(t *testing.T, root string) {
	t.Helper()
	writeReachabilitySource(t, root, reachabilityUntrackedCoveredSource, "package api\n\nfunc untrackedCoveragePoint() string { return \"untracked\" }\n")
	writeReachabilitySource(t, root, reachabilityIgnoredCoveredSource, "package app\n\nfunc ignoredCoveragePoint() string { return \"ignored\" }\n")

	env := deterministicReachabilityEnv()
	runReachabilityTool(t, root, env, "go", "test", "-coverprofile=coverage.out", "./...")
	sourceRef := strings.TrimSpace(runReachabilityTool(t, root, env, "git", "rev-parse", "HEAD"))

	sources := map[string]string{}
	for _, rel := range []string{
		reachabilityTrackedCoveredSource,
		reachabilityUntrackedCoveredSource,
		"services/app/app.go",
		reachabilityIgnoredCoveredSource,
	} {
		data, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(rel))) //nolint:gosec // test-owned root and fixed relative paths
		if err != nil {
			t.Fatalf("read covered source %s: %v", rel, err)
		}
		sum := sha256.Sum256(data)
		sources[rel] = hex.EncodeToString(sum[:])
	}
	writeReachabilitySidecar(t, root, reachabilityCoverageSidecar{
		SchemaVersion: 1, SourceRef: sourceRef, Modules: []string{"api", "app"}, Sources: sources,
	})
}

func writeReachabilitySource(t *testing.T, root, rel, contents string) {
	t.Helper()
	path := filepath.Join(root, filepath.FromSlash(rel))
	if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
		t.Fatalf("create source directory for %s: %v", rel, err)
	}
	if err := os.WriteFile(path, []byte(contents), 0o600); err != nil {
		t.Fatalf("write covered source %s: %v", rel, err)
	}
}

func writeReachabilitySidecar(t *testing.T, root string, sidecar reachabilityCoverageSidecar) {
	t.Helper()
	data, err := json.MarshalIndent(sidecar, "", "  ")
	if err != nil {
		t.Fatalf("encode coverage sidecar: %v", err)
	}
	data = append(data, '\n')
	if err := os.WriteFile(filepath.Join(root, "coverage.out.sidecar.json"), data, 0o600); err != nil {
		t.Fatalf("write coverage sidecar: %v", err)
	}
}

func mutateReachabilitySidecar(t *testing.T, root string, mutate func(*reachabilityCoverageSidecar)) {
	t.Helper()
	path := filepath.Join(root, "coverage.out.sidecar.json")
	data, err := os.ReadFile(path) //nolint:gosec // test-owned root and fixed path
	if err != nil {
		t.Fatalf("read coverage sidecar: %v", err)
	}
	var sidecar reachabilityCoverageSidecar
	if err := json.Unmarshal(data, &sidecar); err != nil {
		t.Fatalf("decode coverage sidecar: %v", err)
	}
	mutate(&sidecar)
	writeReachabilitySidecar(t, root, sidecar)
}

func renderReachabilityConfig(t *testing.T, root string, withCoverage bool) string {
	t.Helper()
	templatePath := filepath.Join(root, ".archfit.yaml.tmpl")
	template, err := os.ReadFile(templatePath) //nolint:gosec // test-owned root
	if err != nil {
		t.Fatalf("read config template: %v", err)
	}
	if count := strings.Count(string(template), reachabilityCoveragePlaceholder); count != 1 {
		t.Fatalf("config template placeholder count = %d, want 1", count)
	}
	coverage := ""
	if withCoverage {
		coverage = `coverage:
  enabled: true
  gate: warn
  sources:
    - path: coverage.out
      format: go-coverprofile
      sidecar_path: coverage.out.sidecar.json
`
	}
	rendered := strings.Replace(string(template), reachabilityCoveragePlaceholder, coverage, 1)
	configPath := filepath.Join(root, defaultConfigPath)
	// #nosec G703 -- root is created by t.TempDir and the filename is fixed.
	if err := os.WriteFile(configPath, []byte(rendered), 0o600); err != nil {
		t.Fatalf("render config: %v", err)
	}
	return configPath
}

func runReachabilityState(t *testing.T, command, configPath string) (int, []byte) {
	t.Helper()
	return runReachabilityStateWithCache(t, command, configPath, true)
}

func runReachabilityStateWithCache(t *testing.T, command, configPath string, refresh bool) (int, []byte) {
	t.Helper()
	args := []string{command, "-c", configPath}
	if refresh {
		args = append(args, flagRefresh)
	}
	args = append(args, "--progress=none", fmtJSON)
	code, stdout, stderr := runArchfit(t, args...)
	if code == 3 {
		t.Fatalf("%s failed: exit=%d\nstdout:\n%s\nstderr:\n%s", command, code, stdout, stderr)
	}
	return code, []byte(stdout)
}

func decodeReachabilityState(t *testing.T, data []byte) report.ArchitectureState {
	t.Helper()
	var state report.ArchitectureState
	if err := json.Unmarshal(data, &state); err != nil {
		t.Fatalf("decode reachability state: %v\n%s", err, data)
	}
	if state.SchemaVersion != report.StateSchemaVersion {
		t.Fatalf("schema_version = %q, want %q", state.SchemaVersion, report.StateSchemaVersion)
	}
	return state
}

func assertReachabilityEnvelope(t *testing.T, data []byte) {
	t.Helper()
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(data, &raw); err != nil {
		t.Fatalf("decode root envelope: %v", err)
	}
	if _, wrapped := raw["architecture_state"]; wrapped {
		t.Fatal("JSON unexpectedly wraps the state in architecture_state; the observed contract is the document root")
	}
	want := []string{
		"agent_tasks", "comparison", "coverage", "decision", "dimensions", "findings",
		"measurement", "schema_version", "seams", "verdict",
	}
	got := make([]string, 0, len(raw))
	for key := range raw {
		got = append(got, key)
	}
	sort.Strings(got)
	if strings.Join(got, "\n") != strings.Join(want, "\n") {
		t.Fatalf("JSON root keys = %v, want %v", got, want)
	}
}

func assertReachabilityComplexity(t *testing.T, complexity report.DimensionState) {
	t.Helper()
	if complexity.Status != report.MeasurementMeasured {
		t.Fatalf("complexity status = %q, want measured from the complete fixture module graph; unknown=%+v", complexity.Status, complexity.Unknown)
	}
	metrics := make(map[string]float64, len(complexity.Metrics))
	for _, metric := range complexity.Metrics {
		metrics[metric.Name] = metric.Value
	}
	for name, want := range map[string]float64{
		"max_dependency_chain": 1, "module_fan_in_p90": 1, "module_fan_out_p90": 1,
	} {
		if got, found := metrics[name]; !found || got != want {
			t.Errorf("complexity metric %q = %v (found=%t), want %v", name, got, found, want)
		}
	}
}

func assertReachabilityOperations(t *testing.T, operations report.DimensionState) {
	t.Helper()
	if operations.Status != report.MeasurementMeasured {
		t.Fatalf("operations status = %q, want measured from fixture Dockerfiles and CODEOWNERS; unknown=%+v", operations.Status, operations.Unknown)
	}
	metrics := make(map[string]float64, len(operations.Metrics))
	for _, metric := range operations.Metrics {
		metrics[metric.Name] = metric.Value
	}
	for name, want := range map[string]float64{
		"owners_from_codeowners":                2,
		"owners_from_git_author_fallback":       0,
		"corroborated_deploy_units":             2,
		"modules_with_corroborated_deploy_unit": 2,
	} {
		if got, found := metrics[name]; !found || got != want {
			t.Errorf("operations metric %q = %v (found=%t), want %v", name, got, found, want)
		}
	}
}

func assertReachabilityExit(t *testing.T, verdict report.StateVerdict, code int) {
	t.Helper()
	want := map[report.StateVerdict]int{
		report.StateHealthy:        0,
		report.StateNeedsAttention: 2,
		report.StateBlocked:        1,
	}[verdict]
	if code != want {
		t.Fatalf("check exit = %d for verdict %q, want %d", code, verdict, want)
	}
}

func activeReachabilityDiagnostics(findings []report.Finding) []report.Finding {
	var diagnostics []report.Finding
	for _, f := range findings {
		active := f.Status == string(finding.StatusNew) || f.Status == string(finding.StatusExpiredWaiver)
		if active && f.Kind != string(finding.KindGate) {
			diagnostics = append(diagnostics, f)
		}
	}
	return diagnostics
}

func assertReachabilityOutcomeA(t *testing.T, state report.ArchitectureState, checkCode int, diagnostics []report.Finding) {
	t.Helper()
	if state.Verdict != report.StateHealthy {
		t.Fatalf("terminal reachability verdict = %q, want healthy", state.Verdict)
	}
	if checkCode != 0 || state.Decision.UnknownDimensions != 0 ||
		state.Decision.HardGates != report.HardGatePass || len(diagnostics) != 0 {
		t.Fatalf("Outcome A incomplete: check_exit=%d unknown_dimensions=%d hard_gates=%s active_diagnostics=%d",
			checkCode, state.Decision.UnknownDimensions, state.Decision.HardGates, len(diagnostics))
	}
	if state.Decision.ActiveBlockers != 0 {
		t.Fatalf("Outcome A active_blockers = %d, want 0", state.Decision.ActiveBlockers)
	}
	t.Log("reachability outcome: A; healthy reached with check exit 0, hard gates passing, no unknown dimensions, and no active diagnostics")
}

func logReachabilityState(t *testing.T, state report.ArchitectureState, checkCode int, diagnostics []report.Finding) {
	t.Helper()
	all := state.Dimensions.All()
	dimensions := make([]string, 0, len(all))
	for _, dimension := range all {
		dimensions = append(dimensions, fmt.Sprintf("%s=%s", dimension.Name, dimension.Status))
	}
	t.Logf("verdict=%s check_exit=%d hard_gates=%s unknown_dimensions=%d active_diagnostics=%d envelope=document-root",
		state.Verdict, checkCode, state.Decision.HardGates, state.Decision.UnknownDimensions, len(diagnostics))
	t.Logf("dimension statuses: %s", strings.Join(dimensions, " "))
}
