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
	reachabilityFixturePath         = "../../testdata/fixtures/reachability"
	reachabilityCoveragePlaceholder = "{{REACHABILITY_COVERAGE_BLOCK}}"
	reachabilityTemporaryOutcome    = "B-temporary"
	reachabilityNewCollectorRemedy  = "new collector required"
)

type reachabilityRemedy struct {
	OutcomeClass string
	Symbol       string
	RemedyClass  string
	Remedy       string
}

var reachabilityRemedies = map[string]reachabilityRemedy{
	report.DimensionComplexity: {
		OutcomeClass: reachabilityTemporaryOutcome,
		Symbol:       "complexityDimension",
		RemedyClass:  reachabilityNewCollectorRemedy,
		Remedy:       "Task 7 must collect complete module-graph depth, fan-in, and fan-out facts",
	},
	report.DimensionTestability: {
		OutcomeClass: reachabilityTemporaryOutcome,
		Symbol:       "testabilityDimension",
		RemedyClass:  reachabilityNewCollectorRemedy,
		Remedy:       "Tasks 8-10 must ingest, attribute, and freshness-check supplied coverage",
	},
}

// TestIntegrationReachability drives the only supported materialization path for
// this fixture. It records either valid outcome: a fully asserted healthy state,
// or a complete impossibility report whose remedies come from the Task 2 audit.
func TestIntegrationReachability(t *testing.T) {
	t.Run("baseline_transition", testIntegrationReachabilityBaselineTransition)
	t.Run("drift_lifecycle", testIntegrationReachabilityDriftLifecycle)
}

func testIntegrationReachabilityBaselineTransition(t *testing.T) {
	root, configPath := materializeFixture(t, false)

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
	switch state.Verdict {
	case report.StateHealthy:
		assertReachabilityOutcomeA(t, state, checkCode, activeDiagnostics)
	default:
		assertReachabilityOutcomeB(t, state, checkCode, activeDiagnostics)
	}

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
	env := deterministicReachabilityEnv()
	runReachabilityTool(t, root, env, "go", "test", "-coverprofile=coverage.out", "./...")
	sourceRef := strings.TrimSpace(runReachabilityTool(t, root, env, "git", "rev-parse", "HEAD"))

	sources := map[string]string{}
	for _, rel := range []string{"services/api/api.go", "services/app/app.go"} {
		data, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(rel))) //nolint:gosec // test-owned root and fixed relative paths
		if err != nil {
			t.Fatalf("read covered source %s: %v", rel, err)
		}
		sum := sha256.Sum256(data)
		sources[rel] = hex.EncodeToString(sum[:])
	}
	sidecar := struct {
		SchemaVersion int               `json:"schema_version"`
		SourceRef     string            `json:"source_ref"`
		Modules       []string          `json:"modules"`
		Sources       map[string]string `json:"sources"`
	}{SchemaVersion: 1, SourceRef: sourceRef, Modules: []string{"api", "app"}, Sources: sources}
	data, err := json.MarshalIndent(sidecar, "", "  ")
	if err != nil {
		t.Fatalf("encode coverage sidecar: %v", err)
	}
	data = append(data, '\n')
	if err := os.WriteFile(filepath.Join(root, "coverage.out.sidecar.json"), data, 0o600); err != nil {
		t.Fatalf("write coverage sidecar: %v", err)
	}
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
	code, stdout, stderr := runArchfit(t, command, "-c", configPath, flagRefresh, "--progress=none", fmtJSON)
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

func assertReachabilityOutcomeB(t *testing.T, state report.ArchitectureState, checkCode int, diagnostics []report.Finding) {
	t.Helper()
	var blockers []report.DimensionState
	for _, dimension := range state.Dimensions.All() {
		if dimension.Status != report.MeasurementMeasured {
			blockers = append(blockers, dimension)
		}
	}

	var lines []string
	for _, dimension := range blockers {
		remedy, known := reachabilityRemedies[dimension.Name]
		if !known {
			lines = append(lines, fmt.Sprintf(
				"- dimension=%s status=%s outcome_class=UNCLASSIFIED symbol=UNKNOWN remedy_class=UNKNOWN",
				dimension.Name, dimension.Status))
			continue
		}
		lines = append(lines, fmt.Sprintf(
			"- dimension=%s status=%s outcome_class=%s symbol=%s remedy_class=%q remedy=%q",
			dimension.Name, dimension.Status, remedy.OutcomeClass, remedy.Symbol, remedy.RemedyClass, remedy.Remedy))
	}
	for _, diagnostic := range diagnostics {
		lines = append(lines, fmt.Sprintf(
			"- diagnostic=%s status=%s outcome_class=UNCLASSIFIED symbol=classifyFindings remedy_class=fixture-or-product-decision-required",
			diagnostic.RuleID, diagnostic.Status))
	}
	t.Logf("reachability outcome: B\n%s", strings.Join(lines, "\n"))

	if state.Verdict != report.StateNeedsAttention || checkCode != 2 {
		t.Fatalf("Outcome B verdict=%q check_exit=%d, want needs_attention/2", state.Verdict, checkCode)
	}
	if state.Decision.HardGates != report.HardGatePass || state.Decision.ActiveBlockers != 0 {
		t.Fatalf("Outcome B is not the characterized evidence-only block: decision=%+v", state.Decision)
	}
	if len(diagnostics) != 0 {
		t.Fatalf("fixture emitted %d active diagnostics; report above names them, but the characterized fixture must have none", len(diagnostics))
	}
	if len(blockers) != len(reachabilityRemedies) {
		t.Fatalf("blocking dimensions = %d, want %d; report:\n%s", len(blockers), len(reachabilityRemedies), strings.Join(lines, "\n"))
	}
	for _, dimension := range blockers {
		if _, known := reachabilityRemedies[dimension.Name]; !known {
			t.Fatalf("unexpected blocking dimension %q; report:\n%s", dimension.Name, strings.Join(lines, "\n"))
		}
	}
	if state.Decision.UnknownDimensions != len(reachabilityRemedies) {
		t.Fatalf("unknown_dimensions = %d, want %d", state.Decision.UnknownDimensions, len(reachabilityRemedies))
	}
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
