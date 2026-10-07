package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/alexei-led/archfit/internal/model/report"
)

const flagReanchor = "--reanchor"

// TestBaselineReanchor pins `archfit baseline --reanchor` end to end: it
// carries accepted debt forward, accepts no new finding, lists every
// difference, and refuses to run without a stored file.
func TestBaselineReanchor(t *testing.T) {
	t.Parallel()
	const cfg = coupledModulesCfg + `rules:
  - id: no-x-to-b
    type: forbidden_dependency
    gate: fail
    from: "pkg/{a,c}/**"
    to: "pkg/b/**"
`
	const importer = "import \"example.com/test/pkg/b/api\"\n\nfunc Use() string { return api.Secret() }\n"
	dir := t.TempDir()
	writeFileAt(t, dir, markerGoMod, "module example.com/test\n\ngo 1.21\n")
	writeFileAt(t, dir, "pkg/b/api/api.go", "package api\n\nfunc Secret() string { return \"s\" }\n")
	writeFileAt(t, dir, "pkg/a/a.go", "package a\n\n"+importer)
	writeFileAt(t, dir, defaultConfigPath, cfg)
	gitInitFixtureRepo(t, dir)
	cfgPath := filepath.Join(dir, defaultConfigPath)
	baselinePath := filepath.Join(dir, defaultBaselinePath)

	if code, _, stderr := runArchfit(t, cmdBaseline, flagReanchor, "-c", cfgPath); code != 3 || !strings.Contains(stderr, "first capture") {
		t.Fatalf("re-anchor without a stored baseline: exit = %d, want 3 naming the first capture\n%s", code, stderr)
	}
	if code, _, _ := runArchfit(t, cmdBaseline, "--from", baselinePath, "-c", cfgPath); code != 3 {
		t.Fatalf("--from without --reanchor: exit = %d, want 3", code)
	}
	if code, _, stderr := runArchfit(t, cmdBaseline, "-c", cfgPath); code != 0 {
		t.Fatalf("first capture: exit = %d\n%s", code, stderr)
	}
	if code, _, _ := runArchfit(t, cmdBaseline, flagReanchor, "--no-advisories", "-c", cfgPath); code != 3 {
		t.Fatalf("--reanchor with --no-advisories: exit = %d, want 3 (it would drop every accepted advisory)", code)
	}

	assertReanchorNamesDrift(t, cfgPath, baselinePath)

	stored := filepath.Join(t.TempDir(), "stored.json")
	data, err := os.ReadFile(baselinePath) //nolint:gosec // path derives from t.TempDir()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(stored, data, 0o600); err != nil { //nolint:gosec // stored derives from t.TempDir()
		t.Fatal(err)
	}

	writeFileAt(t, dir, "pkg/c/c.go", "package c\n\n"+importer)
	code, stdout, stderr := runArchfit(t, cmdBaseline, flagReanchor, "--from", stored, "-c", cfgPath)
	if code != 0 {
		t.Fatalf("re-anchor: exit = %d\nstdout:\n%s\nstderr:\n%s", code, stdout, stderr)
	}
	if !strings.Contains(stdout, "left 1 current finding(s) unaccepted") || !strings.Contains(stdout, "not accepted: no-x-to-b") ||
		!strings.Contains(stdout, "- -> b pkg/c/c.go") {
		t.Errorf("re-anchor must list the new finding for review:\n%s", stdout)
	}
	first, err := os.ReadFile(baselinePath) //nolint:gosec // path derives from t.TempDir()
	if err != nil {
		t.Fatal(err)
	}

	code, state := checkState(t, cfgPath)
	if code != 1 {
		t.Fatalf("check after re-anchor: exit = %d, want 1 (the new finding still blocks)", code)
	}
	statusByFrom := map[string]string{}
	for _, f := range state.Findings {
		if f.RuleID == "no-x-to-b" {
			statusByFrom[f.Edge.From.Path] = f.Status
		}
	}
	if statusByFrom["pkg/a/a.go"] != report.FindingStatusBaseline || statusByFrom["pkg/c/c.go"] != report.FindingStatusNew {
		t.Errorf("statuses after re-anchor = %v, want old debt baseline and the new finding new", statusByFrom)
	}

	if code, _, _ := runArchfit(t, cmdBaseline, flagReanchor, "-c", cfgPath); code != 0 {
		t.Fatalf("second re-anchor: exit = %d", code)
	}
	second, err := os.ReadFile(baselinePath) //nolint:gosec // path derives from t.TempDir()
	if err != nil {
		t.Fatal(err)
	}
	if string(first) != string(second) {
		t.Errorf("re-anchoring onto its own output changed the file\nfirst:\n%s\nsecond:\n%s", first, second)
	}

	if err := os.Remove(filepath.Join(dir, "pkg/a/a.go")); err != nil {
		t.Fatal(err)
	}
	writeFileAt(t, dir, "pkg/a/a.go", "package a\n")
	code, stdout, _ = runArchfit(t, cmdBaseline, flagReanchor, "-c", cfgPath)
	if code != 0 || !strings.Contains(stdout, "kept 0 accepted entries") || !strings.Contains(stdout, "dropped: no-x-to-b") {
		t.Errorf("fixed debt must be dropped and listed (exit %d):\n%s", code, stdout)
	}
}

// checkState runs check --format json and decodes the state it prints.
func checkState(t *testing.T, cfgPath string) (int, report.ArchitectureState) {
	t.Helper()
	code, out, _ := runArchfit(t, cmdCheck, fmtJSON, "-c", cfgPath)
	var state report.ArchitectureState
	if err := json.Unmarshal([]byte(out), &state); err != nil {
		t.Fatalf("check output is not a state document: %v\n%s", err, out)
	}
	return code, state
}

// assertReanchorNamesDrift drifts the stored reference, then requires the
// re-anchor to name the same drift the gate reference reports and to leave a
// comparable reference behind.
func assertReanchorNamesDrift(t *testing.T, cfgPath, baselinePath string) {
	t.Helper()
	data, err := os.ReadFile(baselinePath) //nolint:gosec // path derives from t.TempDir()
	if err != nil {
		t.Fatal(err)
	}
	var raw map[string]any
	if err := json.Unmarshal(data, &raw); err != nil {
		t.Fatal(err)
	}
	raw["state"].(map[string]any)["classification_hash"] = "drifted"
	drifted, err := json.Marshal(raw)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(baselinePath, drifted, 0o600); err != nil {
		t.Fatal(err)
	}
	_, before := checkState(t, cfgPath)
	if before.GateReference == nil || before.GateReference.Status != report.ComparisonNonComparable {
		t.Fatalf("fixture regression: a drifted stored reference must not compare: %+v", before.GateReference)
	}
	code, stdout, stderr := runArchfit(t, cmdBaseline, flagReanchor, "-c", cfgPath)
	if code != 0 {
		t.Fatalf("re-anchor of a drifted reference: exit = %d\n%s", code, stderr)
	}
	for _, reason := range before.GateReference.Reasons {
		if !strings.Contains(stdout, "drift: "+reason) {
			t.Errorf("re-anchor output lacks the gate reference's drift %q:\n%s", reason, stdout)
		}
	}
	if _, after := checkState(t, cfgPath); after.GateReference == nil || after.GateReference.Status != report.ComparisonComparable {
		t.Fatalf("after a re-anchor the reference must compare: %+v", after.GateReference)
	}
}
