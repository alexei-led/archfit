package main

import (
	"encoding/json"
	"path/filepath"
	"testing"
)

// TestCheckSarifMarksBaselineMembership runs check --format sarif end to end
// against a captured baseline: the baselined violation is unchanged and
// accepted, a violation added after the capture is new and not suppressed. A
// run with no baseline file writes no baselineState at all.
func TestCheckSarifMarksBaselineMembership(t *testing.T) {
	t.Parallel()
	const newViolation = "pkg/a/later.go"
	dir := t.TempDir()
	for name, content := range map[string]string{
		markerGoMod: goModStub, filePkgAA: hookViolatingA, hookImplFile: implSource(), defaultConfigPath: hookRuleCfg,
	} {
		writeFixtureFile(t, dir, name, content)
	}
	gitInitFixtureRepo(t, dir)
	gitCommitFixture(t, dir)
	cfg := filepath.Join(dir, defaultConfigPath)

	code, before, stderr := runArchfit(t, cmdCheck, "-c", cfg, "--format=sarif")
	if code != 1 {
		t.Fatalf("check before baseline: exit %d\n%s", code, stderr)
	}
	for id, res := range sarifResults(t, before) {
		if _, ok := res["baselineState"]; ok {
			t.Errorf("%s carries baselineState with no baseline file", id)
		}
	}

	if code, _, stderr := runArchfit(t, cmdBaseline, "-c", cfg); code != 0 {
		t.Fatalf("baseline: exit %d\n%s", code, stderr)
	}
	writeFixtureFile(t, dir, newViolation, hookViolatingA)
	_, after, stderr = runArchfit(t, cmdCheck, "-c", cfg, "--format=sarif")
	states := map[string]int{}
	for id, res := range sarifResults(t, after) {
		state, _ := res["baselineState"].(string)
		sup, _ := res["suppressions"].([]any)
		states[state]++
		switch state {
		case "unchanged":
			if len(sup) != 1 {
				t.Errorf("%s is unchanged but not suppressed: %v", id, res)
			}
		case "new":
			if len(sup) != 0 {
				t.Errorf("%s is new but suppressed: %v", id, res)
			}
		default:
			t.Errorf("%s baselineState = %q, want new or unchanged", id, state)
		}
	}
	if states["unchanged"] == 0 || states["new"] == 0 {
		t.Errorf("baselineState counts = %v, want both unchanged and new\n%s", states, stderr)
	}
}

// sarifResults decodes a SARIF log's results keyed by the archfit fingerprint.
func sarifResults(t *testing.T, out string) map[string]map[string]any {
	t.Helper()
	var doc struct {
		Runs []struct {
			Results []map[string]any `json:"results"`
		} `json:"runs"`
	}
	if err := json.Unmarshal([]byte(out), &doc); err != nil || len(doc.Runs) != 1 {
		t.Fatalf("not a one-run SARIF log (%v):\n%s", err, out)
	}
	byID := map[string]map[string]any{}
	for _, res := range doc.Runs[0].Results {
		byID[res["fingerprints"].(map[string]any)["archfit/v1"].(string)] = res
	}
	return byID
}
