package main

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/alexei-led/archfit/internal/assessment/finding"
	"github.com/alexei-led/archfit/internal/assessment/result"
	"github.com/alexei-led/archfit/internal/baseline"
	"github.com/alexei-led/archfit/internal/model/report"
)

// coupledModulesCfg declares two modules with different owners and no rules, so
// Any FAIL from `archfit check` on the fixture repo comes from the coupling gate alone.
const coupledModulesCfg = `version: 2
modules:
  a:
    paths: ["pkg/a/**"]
    owner: team-a
  b:
    paths: ["pkg/b/**"]
    internal: ["pkg/b/internal/**"]
    owner: team-b
`

// writeCoupledRepo creates a minimal Go repo with one cross-module edge
// (pkg/a → pkg/b/internal: intrusive strength across different owners — a
// measured, unbalanced coupling_balance score) and the given config body.
// Returns the config path.
func writeCoupledRepo(t *testing.T, cfgBody string) string {
	t.Helper()
	dir := t.TempDir()
	files := map[string]string{
		markerGoMod: "module example.com/test\n\ngo 1.21\n",
		filePkgAA: "package a\n\nimport \"example.com/test/pkg/b/internal/impl\"\n\n" +
			"func UseSecret() string { return impl.Secret() }\n",
		filePkgBImpl:      implSource(),
		defaultConfigPath: cfgBody,
	}
	for name, content := range files {
		path := filepath.Join(dir, name)
		if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	gitInitFixtureRepo(t, dir)
	return filepath.Join(dir, defaultConfigPath)
}

// distributedMonolithCfg declares the same two modules in different deploy
// units, so the pkg/a -> pkg/b/internal edge is intrusive at cross_deploy_unit
// distance: balance max(|10-9|, 0)+1 = 2, the critical band, which is exactly
// the distributed-monolith seam condition.
const distributedMonolithCfg = `version: 2
modules:
  a:
    paths: ["pkg/a/**"]
    owner: team-a
    deploy_unit: svc-a
  b:
    paths: ["pkg/b/**"]
    internal: ["pkg/b/internal/**"]
    owner: team-b
    deploy_unit: svc-b
`

// TestRun_Check_RejectsRetiredCouplingGateKeys pins the migration contract:
// the retired scalar knobs still decode (config update --migration-only has to
// read them) but no analysis accepts them, and the refusal names the one
// supported way out verbatim.
func TestRun_Check_RejectsRetiredCouplingGateKeys(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name string
		cfg  string
	}{
		{"schema v1", strings.Replace(coupledModulesCfg, "version: 2", "version: 1", 1)},
		{"retired min_band", coupledModulesCfg + "coupling:\n  gate:\n    min_band: strong\n"},
		{"retired max_drop", coupledModulesCfg + "coupling:\n  gate:\n    max_drop: 0\n"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			cfgPath := writeCoupledRepo(t, tc.cfg)
			for _, cmd := range []string{cmdCheck, cmdAnalyze} {
				code, _, stderr := runArchfit(t, cmd, "-c", cfgPath, flagRefresh)
				if code != 3 {
					t.Errorf("%s: exit = %d, want 3 (config error, not a reportable state)\nstderr:\n%s",
						cmd, code, stderr)
				}
				if !strings.Contains(stderr, "archfit config update --migration-only --apply") {
					t.Errorf("%s stderr does not name the migration command:\n%s", cmd, stderr)
				}
			}
		})
	}
}

// TestRun_Check_DistributedMonolithSeamIsDiagnostic pins the default posture: a
// genuine distributed-monolith seam is reported, but warn mode never fails the
// run and never emits a gate finding. Blocking needs mode: fail AND a
// comparable reference; that half is pinned in
// evaluation.TestApplySeamGate_BlocksOnlyOnNewSeamsInFailMode, because no
// comparable reference exists until baseline v2 lands.
func TestRun_Check_DistributedMonolithSeamIsDiagnostic(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name string
		cfg  string
	}{
		{"no gate block takes the warn default", distributedMonolithCfg},
		{"explicit warn mode", distributedMonolithCfg +
			"coupling:\n  gate:\n    distributed_monolith:\n      mode: warn\n      max_new_seams: 0\n"},
		{"fail mode without a comparable reference cannot claim a new seam", distributedMonolithCfg +
			"coupling:\n  gate:\n    distributed_monolith:\n      mode: fail\n      max_new_seams: 0\n"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			cfgPath := writeCoupledRepo(t, tc.cfg)

			var buf bytes.Buffer
			if code := Run([]string{cmdCheck, fmtJSON, "-c", cfgPath, flagRefresh}, &buf); code != 0 {
				t.Fatalf("check: exit = %d, want 0 — a coupling seam is diagnostic\noutput:\n%s", code, buf.String())
			}
			var diag result.Result
			if err := json.Unmarshal(buf.Bytes(), &diag); err != nil {
				t.Fatalf("unmarshal JSON output: %v", err)
			}
			if diag.Verdict == result.VerdictFail {
				t.Errorf("verdict = fail on a diagnostic seam")
			}
			for _, f := range diag.Findings {
				if f.RuleID == ruleIDCouplingGate {
					t.Errorf("diagnostic run emitted a coupling-gate finding: %+v", f)
				}
			}
		})
	}
}

// TestRun_Analyze_DiscloseSeamAbstention pins the analyze-only stderr contract:
// analyze discloses that qualifying seams exist but cannot be compared, so a
// reader is never told an unrated gate is a clean one.
func TestRun_Analyze_DiscloseSeamAbstention(t *testing.T) {
	t.Parallel()
	cfgPath := writeCoupledRepo(t, distributedMonolithCfg)

	var buf, errBuf bytes.Buffer
	if code := RunWithStderr([]string{cmdAnalyze, "-c", cfgPath, flagRefresh}, &buf, &errBuf); code != 0 {
		t.Fatalf("analyze: exit = %d, want 0\nstderr:\n%s", code, errBuf.String())
	}
	if !strings.Contains(errBuf.String(), "distributed-monolith seam") {
		t.Errorf("analyze did not disclose the qualifying seams:\n%s", errBuf.String())
	}
	if !strings.Contains(errBuf.String(), "no comparable reference") {
		t.Errorf("analyze did not disclose why no new-seam count was claimed:\n%s", errBuf.String())
	}
}

// TestRun_Baseline_NoTripReasonOnStderr pins the analyze-only half of the
// stderr contract: baseline shares the stage executor but does not consume the
// coupling gate as an exit code, so the reason line is noise there.
func TestRun_Baseline_NoTripReasonOnStderr(t *testing.T) {
	t.Parallel()
	cfgPath := writeCoupledRepo(t, distributedMonolithCfg)

	var buf, errBuf bytes.Buffer
	code := RunWithStderr([]string{cmdBaseline, "-c", cfgPath, flagRefresh}, &buf, &errBuf)
	if code != 0 {
		t.Fatalf("baseline: exit = %d, want 0\nstderr:\n%s", code, errBuf.String())
	}
	if strings.Contains(errBuf.String(), "coupling gate: ") {
		t.Errorf("baseline echoed coupling-gate reasons to stderr (analyze-only contract):\n%s", errBuf.String())
	}
}

// TestRun_NonAnalyzeCommands_NoTripReasonOnStderr extends the analyze-only
// stderr contract to every other command that shares the stage executor. None
// of them consumes the gate as an exit code, so the reason line is noise
// there; `config compare` would print it twice, unlabelled, once per side.
// `config enrich` shares the same request builder; it needs an AI provider to
// reach the stage, so its half is pinned in
// application.TestEnrichSuppressesCouplingGateReasons.
func TestRun_NonAnalyzeCommands_NoTripReasonOnStderr(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name string
		args func(cfgPath string) []string
	}{
		{"explain", func(c string) []string { return []string{cmdExplain, "0", "-c", c, flagRefresh} }},
		{"config compare", func(c string) []string { return []string{cmdConfig, cmdCompare, c, "-c", c} }},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			cfgPath := writeCoupledRepo(t, distributedMonolithCfg)
			_, _, stderr := runArchfit(t, test.args(cfgPath)...)
			if strings.Contains(stderr, "coupling gate: ") {
				t.Errorf("%s echoed coupling-gate reasons to stderr (analyze-only contract):\n%s", test.name, stderr)
			}
		})
	}
}

// TestRun_Analyze_UnmeasurableCouplingStaysSilent pins the abstain rule at the
// disclosure layer: a repo where coupling cannot be measured has no seams to
// report, so the run passes and says nothing about seams. An abstention printed
// on every unmeasurable run trains readers to ignore the line that matters.
func TestRun_Analyze_UnmeasurableCouplingStaysSilent(t *testing.T) {
	t.Parallel()
	cfgPath := writeNonGoRepo(t, "version: 2\ncoupling:\n  gate:\n    distributed_monolith:\n      mode: fail\n")

	var buf, errBuf bytes.Buffer
	code := RunWithStderr([]string{cmdCheck, "-c", cfgPath, flagRefresh}, &buf, &errBuf)
	if code != 0 {
		t.Fatalf("check on unmeasurable coupling: exit = %d, want 0\noutput:\n%s", code, buf.String())
	}
	if strings.Contains(errBuf.String(), "distributed-monolith seam") {
		t.Errorf("unmeasurable run disclosed a seam abstention:\n%s", errBuf.String())
	}
}

// TestRun_Baseline_WritesStateSnapshot verifies that `archfit baseline` writes
// the schema-v2 architecture-state reference — the four comparison fingerprints
// travelling with the facts they qualify — and no repository scalar.
func TestRun_Baseline_WritesStateSnapshot(t *testing.T) {
	t.Parallel()
	cfgPath := writeCoupledRepo(t, coupledModulesCfg)

	var buf bytes.Buffer
	if code := Run([]string{cmdBaseline, "-c", cfgPath, flagRefresh}, &buf); code != 0 {
		t.Fatalf("baseline: exit = %d\noutput:\n%s", code, buf.String())
	}

	path := filepath.Join(filepath.Dir(cfgPath), defaultBaselinePath)
	b, err := baseline.Load(context.Background(), path)
	if err != nil {
		t.Fatal(err)
	}
	if b.SchemaVersion != baseline.SchemaVersion {
		t.Fatalf("schema_version = %q, want %q", b.SchemaVersion, baseline.SchemaVersion)
	}
	if b.Score != nil {
		t.Errorf("schema v2 must not store a repository scalar, got %+v", b.Score)
	}
	if b.State == nil {
		t.Fatal("baseline written without an architecture-state reference")
	}
	if b.State.ConfigHash == "" || b.State.ModelHash == "" {
		t.Errorf("state reference missing fingerprints: %+v", b.State)
	}
	if b.State.RubricVersion != report.ScoreVersion {
		t.Errorf("rubric_version = %q, want %q", b.State.RubricVersion, report.ScoreVersion)
	}
	if len(b.State.Dimensions) != report.DimensionCount {
		t.Errorf("stored dimensions = %d, want %d", len(b.State.Dimensions), report.DimensionCount)
	}
}

// TestRun_Baseline_IsIdempotent pins the capture contract: the file `archfit
// baseline` writes is a function of the tree and the config alone.
//
// It was not. The capture read the baseline it was about to overwrite, and
// Balanced-Coupling advisories roll up per (module pair, strength, distance,
// volatility, STATUS) — so accepting a group's representative split the group
// on the next run, exposed its siblings as new representatives, and wrote a
// different file every time. Two captures over an unchanged tree never settled.
func TestRun_Baseline_IsIdempotent(t *testing.T) {
	t.Parallel()
	cfgPath := writeCoupledRepo(t, distributedMonolithCfg)
	path := filepath.Join(filepath.Dir(cfgPath), defaultBaselinePath)

	capture := func() []byte {
		t.Helper()
		var buf bytes.Buffer
		if code := Run([]string{cmdBaseline, "-c", cfgPath}, &buf); code != 0 {
			t.Fatalf("baseline: exit = %d\noutput:\n%s", code, buf.String())
		}
		data, err := os.ReadFile(path) //nolint:gosec // path derives from t.TempDir()
		if err != nil {
			t.Fatal(err)
		}
		return data
	}

	first := capture()
	second := capture()
	if !bytes.Equal(first, second) {
		t.Errorf("second capture over an unchanged tree differs from the first\nfirst:\n%s\nsecond:\n%s", first, second)
	}
}

// TestRun_Baseline_KeepsNativeAdvisoryKind guards the finding-lifecycle
// contract: `archfit baseline` must persist BC findings with their native
// advisory kind. A stored "gate" kind orphans the entry — status.Assign matches
// the stored kind against the pass kind, so the edge would surface as a phantom
// "fixed" gate finding and never resolve on the advisory side.
func TestRun_Baseline_KeepsNativeAdvisoryKind(t *testing.T) {
	t.Parallel()
	cfgPath := writeCoupledRepo(t, distributedMonolithCfg)

	var buf bytes.Buffer
	if code := Run([]string{cmdBaseline, "-c", cfgPath, flagRefresh}, &buf); code != 0 {
		t.Fatalf("baseline: exit = %d\noutput:\n%s", code, buf.String())
	}
	b, err := baseline.Load(context.Background(), filepath.Join(filepath.Dir(cfgPath), defaultBaselinePath))
	if err != nil {
		t.Fatal(err)
	}
	sawBC := false
	for _, a := range b.Accepted {
		if a.RuleID != ruleIDBCImbalanced {
			continue
		}
		sawBC = true
		if a.Kind != finding.KindAdvisory {
			t.Errorf("baselined BC finding %s kind = %q, want %q", a.Fingerprint, a.Kind, finding.KindAdvisory)
		}
	}
	if !sawBC {
		t.Fatal("fixture regression: baseline persisted no BC advisory")
	}
}
