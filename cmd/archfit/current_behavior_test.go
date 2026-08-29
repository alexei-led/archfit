package main

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/alexei-led/archfit/internal/model/report"
)

type currentStateJSON struct {
	Verdict  string `json:"verdict"`
	Decision struct {
		HardGates string `json:"hard_gates"`
	} `json:"decision"`
	Coverage struct {
		Measured   int `json:"measured"`
		Partial    int `json:"partial"`
		Unmeasured int `json:"unmeasured"`
		Tools      []struct {
			Tool   string `json:"tool"`
			Status string `json:"status"`
		} `json:"tools"`
	} `json:"coverage"`
	Dimensions map[string]struct {
		Metrics []struct {
			Name  string  `json:"name"`
			Value float64 `json:"value"`
		} `json:"metrics"`
	} `json:"dimensions"`
}

func decodeCurrentState(t *testing.T, raw []byte) currentStateJSON {
	t.Helper()
	var state currentStateJSON
	if err := json.Unmarshal(raw, &state); err != nil {
		t.Fatalf("invalid state JSON: %v\n%s", err, raw)
	}
	return state
}

func toolStatus(state currentStateJSON, tool string) (string, bool) {
	for _, row := range state.Coverage.Tools {
		if row.Tool == tool {
			return row.Status, true
		}
	}
	return "", false
}

func dimensionMetric(state currentStateJSON, dimension, metric string) (float64, bool) {
	for _, row := range state.Dimensions[dimension].Metrics {
		if row.Name == metric {
			return row.Value, true
		}
	}
	return 0, false
}

func TestRun_Check_RequireToolsHardGate(t *testing.T) {
	t.Parallel()
	t.Run("default stays report-only", func(t *testing.T) {
		t.Parallel()
		cfgPath := writeGapRepo(t, "")
		var buf bytes.Buffer
		if code := Run([]string{cmdAnalyze, "-c", cfgPath, flagRefresh, fmtJSON}, &buf); code != 0 {
			t.Fatalf("analyze: exit = %d, want 0\n%s", code, buf.String())
		}
		state := decodeCurrentState(t, buf.Bytes())
		if state.Decision.HardGates == string(report.HardGateFail) {
			t.Fatalf("default missing-tool policy unexpectedly failed: %+v", state.Decision)
		}
	})

	t.Run("require tools blocks check", func(t *testing.T) {
		t.Parallel()
		cfgPath := writeGapRepo(t, "")
		var buf bytes.Buffer
		if code := Run([]string{cmdCheck, "-c", cfgPath, flagRefresh, "--require-tools", fmtJSON}, &buf); code != 1 {
			t.Fatalf("check --require-tools: exit = %d, want 1\n%s", code, buf.String())
		}
		state := decodeCurrentState(t, buf.Bytes())
		if state.Verdict != string(report.StateBlocked) || state.Decision.HardGates != string(report.HardGateFail) {
			t.Fatalf("state = verdict %q, hard_gates %q; want blocked/fail", state.Verdict, state.Decision.HardGates)
		}
	})

	t.Run("per-tool fail gate is scoped", func(t *testing.T) {
		t.Parallel()
		cfg := "version: 2\nlanguages:\n  go:\n    enabled: false\n    gate: fail\n"
		cfgPath := writeNonGoRepo(t, cfg)
		var buf bytes.Buffer
		if code := Run([]string{cmdCheck, "-c", cfgPath, flagRefresh, fmtJSON}, &buf); code != 1 {
			t.Fatalf("tools.go.gate fail: exit = %d, want 1\n%s", code, buf.String())
		}
		state := decodeCurrentState(t, buf.Bytes())
		if status, ok := toolStatus(state, toolGoPackages); !ok || status == "ok" {
			t.Fatalf("go/packages status = %q, present=%v; want a missing-tool status", status, ok)
		}
	})

	t.Run("require tools remains report-only under analyze", func(t *testing.T) {
		t.Parallel()
		cfgPath := writeGapRepo(t, "")
		var buf bytes.Buffer
		if code := Run([]string{cmdAnalyze, "-c", cfgPath, flagRefresh, "--require-tools", fmtJSON}, &buf); code != 0 {
			t.Fatalf("analyze --require-tools: exit = %d, want 0\n%s", code, buf.String())
		}
	})
}

func TestRun_Check_OldBaselineErrorIsActionable(t *testing.T) {
	t.Parallel()
	cfgPath := writeViolatingRepo(t)
	baselinePath := filepath.Join(filepath.Dir(cfgPath), defaultBaselinePath)
	body := `{"schema_version":"archfit.baseline.v1","accepted":[]}`
	if err := os.WriteFile(baselinePath, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	code, _, stderr := runArchfit(t, cmdCheck, "-c", cfgPath, fmtJSON)
	if code != 3 {
		t.Fatalf("old baseline: exit=%d, want 3; stderr=%s", code, stderr)
	}
	if strings.Contains(stderr, "baseline: baseline:") || !strings.Contains(stderr, "regenerate with `archfit baseline`") {
		t.Fatalf("old baseline error is not actionable: %s", stderr)
	}
}

func TestRun_Analyze_MarkdownAlias(t *testing.T) {
	t.Parallel()
	cfgPath := writeViolatingRepo(t)
	code, alias, aliasErr := runArchfit(t, cmdAnalyze, "-c", cfgPath, flagRefresh, "--format=md")
	if code != 0 {
		t.Fatalf("--format=md: exit=%d stderr=%s", code, aliasErr)
	}
	code, canonical, canonicalErr := runArchfit(t, cmdAnalyze, "-c", cfgPath, "--format=markdown")
	if code != 0 {
		t.Fatalf("--format=markdown: exit=%d stderr=%s", code, canonicalErr)
	}
	if alias != canonical {
		t.Fatalf("md alias differs from markdown:\n%s", firstDiffLine(canonical, alias))
	}
}

func TestRun_Analyze_ScipDisabledCoverageRow(t *testing.T) {
	t.Parallel()
	cfgPath := writeViolatingRepo(t)
	var buf bytes.Buffer
	if code := Run([]string{cmdAnalyze, "-c", cfgPath, flagRefresh, fmtJSON}, &buf); code == 3 {
		t.Fatalf("analyze exited 3\n%s", buf.String())
	}
	state := decodeCurrentState(t, buf.Bytes())
	if status, ok := toolStatus(state, toolScip); !ok || status != "disabled" {
		t.Fatalf("scip status = %q, present=%v; want disabled", status, ok)
	}
}

func TestRun_Analyze_DeployUnitCoverageRowIsDiagnosticOnly(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	files := map[string]string{
		markerGoMod:       "module example.com/deploycov\n\ngo 1.21\n",
		"cmd/api/main.go": goMainSrc,
		defaultConfigPath: "version: 2\nmodules:\n  cmd/api:\n    paths: [\"cmd/api/**\"]\n",
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
	var buf bytes.Buffer
	if code := Run([]string{cmdAnalyze, "-c", filepath.Join(dir, defaultConfigPath), flagRefresh, fmtJSON}, &buf); code == 3 {
		t.Fatalf("analyze exited 3\n%s", buf.String())
	}
	state := decodeCurrentState(t, buf.Bytes())
	if status, ok := toolStatus(state, toolDeployUnit); !ok || status != "ok" {
		t.Fatalf("deploy-unit status = %q, present=%v; want ok", status, ok)
	}
	if got := state.Coverage.Measured + state.Coverage.Partial + state.Coverage.Unmeasured; got != report.DimensionCount {
		t.Fatalf("dimension coverage total = %d, want %d", got, report.DimensionCount)
	}
}

func TestRun_Check_FileClassConfigWiredToPipeline(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	files := map[string]string{
		markerGoMod:                   "module example.com/fctest\n\ngo 1.21\n",
		"main.go":                     goMainSrc,
		"codegen/mycodegen_output.go": "package codegen\n\nfunc Generated() {}\n",
		defaultConfigPath:             "version: 2\nfile_class:\n  generated_globs:\n    - \"codegen/**\"\n",
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
	var buf bytes.Buffer
	if code := Run([]string{cmdAnalyze, "-c", filepath.Join(dir, defaultConfigPath), flagRefresh, fmtJSON}, &buf); code == 3 {
		t.Fatalf("analyze exited 3\n%s", buf.String())
	}
	state := decodeCurrentState(t, buf.Bytes())
	if status, ok := toolStatus(state, toolLoc); !ok || status != "ok" {
		t.Fatalf("loc status = %q, present=%v; want ok", status, ok)
	}
	if files, ok := dimensionMetric(state, report.DimensionComplexity, "production_files"); !ok || files != 1 {
		t.Fatalf("production_files = %v, present=%v; generated file must be excluded", files, ok)
	}
}

func TestRun_Check_Root_ScopesLocCount(t *testing.T) {
	t.Parallel()
	repoDir := t.TempDir()
	files := map[string]string{
		markerGoMod:      "module example.com/mono\n\ngo 1.21\n",
		"other/other.go": "package other\n",
		"sub/go.mod":     "module example.com/sub\n\ngo 1.21\n",
		"sub/service.go": "package sub\n",
	}
	for name, content := range files {
		path := filepath.Join(repoDir, name)
		if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	gitInitFixtureRepo(t, repoDir)
	cfgDir := t.TempDir()
	cfgPath := filepath.Join(cfgDir, defaultConfigPath)
	if err := os.WriteFile(cfgPath, []byte("version: 2\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	filesSeen := func(root string) float64 {
		var buf bytes.Buffer
		if code := Run([]string{cmdAnalyze, flagRoot, root, "-c", cfgPath, flagRefresh, fmtJSON}, &buf); code == 3 {
			t.Fatalf("analyze --root %s: exit 3\n%s", root, buf.String())
		}
		state := decodeCurrentState(t, buf.Bytes())
		value, ok := dimensionMetric(state, report.DimensionComplexity, "production_files")
		if !ok {
			t.Fatalf("production_files missing for --root %s", root)
		}
		return value
	}
	if got := filesSeen(filepath.Join(repoDir, "sub")); got != 1 {
		t.Errorf("subtree production_files = %v, want 1", got)
	}
	if got := filesSeen(repoDir); got < 2 {
		t.Errorf("repository production_files = %v, want at least 2", got)
	}
}

func TestRun_Check_Root_OutputWarningUsesRoot(t *testing.T) {
	t.Parallel()
	repoDir := t.TempDir()
	subDir := filepath.Join(repoDir, "sub")
	if err := os.MkdirAll(subDir, 0o750); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(repoDir, markerGoMod), []byte("module example.com/m\n\ngo 1.21\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	cfgPath := filepath.Join(subDir, defaultConfigPath)
	if err := os.WriteFile(cfgPath, []byte("version: 2\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	gitInitFixtureRepo(t, repoDir)
	code, stdout, stderr := runArchfit(t, cmdAnalyze, flagRoot, subDir, "-c", cfgPath, flagRefresh, "--format=markdown")
	if code == 3 {
		t.Fatalf("unexpected exit 3: stdout=%s stderr=%s", stdout, stderr)
	}
	if strings.Contains(stdout+stderr, "output written inside analyzed root") {
		t.Fatalf("spurious output-inside-root warning: stdout=%s stderr=%s", stdout, stderr)
	}
}

func TestRun_Check_Root_CaseVariantSubtree_OwnerSourceCodeowners(t *testing.T) {
	t.Parallel()
	parent := t.TempDir()
	if !isCaseInsensitiveFS(t, parent) {
		t.Skip("case-sensitive filesystem")
	}
	repoDir := filepath.Join(parent, "Repo")
	subDir := filepath.Join(repoDir, "services", "api")
	if err := os.MkdirAll(subDir, 0o750); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(subDir, "handler.go"), []byte("package api\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(repoDir, ".github"), 0o750); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(repoDir, ".github/CODEOWNERS"), []byte("/services/api @api-team\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	gitInitFixtureRepo(t, repoDir)
	cfgPath := filepath.Join(repoDir, defaultConfigPath)
	if err := os.WriteFile(cfgPath, []byte("version: 2\nmodules:\n  api:\n    paths: [\"**\"]\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	caseVariantRoot := filepath.Join(parent, "repo", "services", "api")
	code, stdout, stderr := runArchfit(t, cmdAnalyze, flagRoot, caseVariantRoot, "-c", cfgPath, flagRefresh, "--format=markdown")
	if code == 3 {
		t.Fatalf("unexpected exit 3: stdout=%s stderr=%s", stdout, stderr)
	}
	if !strings.Contains(stdout, "`owner_source`: codeowners") {
		t.Fatalf("case-variant root corrupted CODEOWNERS resolution:\n%s", stdout)
	}
}

func TestRun_Check_OwnerResolutionSignals(t *testing.T) {
	t.Parallel()
	t.Run("CODEOWNERS no match is disclosed", func(t *testing.T) {
		t.Parallel()
		repoDir := t.TempDir()
		if err := os.MkdirAll(filepath.Join(repoDir, "src"), 0o750); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(repoDir, "src/main.go"), []byte("package main\n"), 0o600); err != nil {
			t.Fatal(err)
		}
		if err := os.MkdirAll(filepath.Join(repoDir, ".github"), 0o750); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(repoDir, ".github/CODEOWNERS"), []byte("docs/ @docs-team\n"), 0o600); err != nil {
			t.Fatal(err)
		}
		gitInitFixtureRepo(t, repoDir)
		cfgPath := filepath.Join(repoDir, defaultConfigPath)
		if err := os.WriteFile(cfgPath, []byte("version: 2\nmodules:\n  app:\n    paths: [\"src/**\"]\n"), 0o600); err != nil {
			t.Fatal(err)
		}
		code, stdout, stderr := runArchfit(t, cmdAnalyze, "-c", cfgPath, flagRefresh, "--format=markdown")
		if code == 3 {
			t.Fatalf("unexpected exit 3: stdout=%s stderr=%s", stdout, stderr)
		}
		if !strings.Contains(stdout, "`owner_source`: codeowners_no_match") || !strings.Contains(stdout, "CODEOWNERS") {
			t.Fatalf("owner degradation missing from Markdown:\n%s", stdout)
		}
	})

	t.Run("clean none source has no degradation warning", func(t *testing.T) {
		t.Parallel()
		repoDir := t.TempDir()
		if err := os.MkdirAll(filepath.Join(repoDir, "src"), 0o750); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(repoDir, "src/main.go"), []byte("package main\n"), 0o600); err != nil {
			t.Fatal(err)
		}
		gitInitFixtureRepo(t, repoDir)
		cfgPath := filepath.Join(repoDir, defaultConfigPath)
		if err := os.WriteFile(cfgPath, []byte("version: 2\nmodules:\n  app:\n    paths: [\"src/**\"]\n"), 0o600); err != nil {
			t.Fatal(err)
		}
		code, stdout, stderr := runArchfit(t, cmdAnalyze, "-c", cfgPath, flagRefresh, "--format=markdown")
		if code == 3 {
			t.Fatalf("unexpected exit 3: stdout=%s stderr=%s", stdout, stderr)
		}
		if !strings.Contains(stdout, "`owner_source`: none") || strings.Contains(stdout+stderr, "owner resolution") {
			t.Fatalf("unexpected owner degradation: stdout=%s stderr=%s", stdout, stderr)
		}
	})
}
