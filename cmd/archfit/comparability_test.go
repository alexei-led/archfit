package main

import (
	"bytes"
	"os"
	"strings"
	"testing"
)

// gateReferenceReasons runs analyze and returns the stored-reference reasons.
func gateReferenceReasons(t *testing.T, cfgPath string) (reasons, drift []string, baselinePresent bool) {
	t.Helper()
	state := erosionState(t, cfgPath)
	if state.GateReference == nil {
		t.Fatal("gate_reference is missing")
	}
	if state.GateReference.BaselinePresent != nil {
		baselinePresent = *state.GateReference.BaselinePresent
	}
	return state.GateReference.Reasons, state.GateReference.Drift, baselinePresent
}

// TestRun_ComparabilityIgnoresGovernanceEdits runs the real commands: capture a
// baseline, edit the config, analyze. A comment or a rule gate is governance
// and must not appear among the reasons the reference is not comparable; a
// classification leaf must, with its drift class.
func TestRun_ComparabilityIgnoresGovernanceEdits(t *testing.T) {
	t.Parallel()
	cfgPath := writeCoupledRepo(t, distributedMonolithCfg)
	var buf bytes.Buffer
	if code := Run([]string{cmdBaseline, "-c", cfgPath}, &buf); code != 0 {
		t.Fatalf("baseline: exit = %d\n%s", code, buf.String())
	}
	original, err := os.ReadFile(cfgPath) //nolint:gosec // path derives from t.TempDir()
	if err != nil {
		t.Fatal(err)
	}

	_, _, present := gateReferenceReasons(t, cfgPath)
	if !present {
		t.Fatal("gate_reference.baseline_present = false after a baseline was captured")
	}

	edits := []struct {
		name         string
		body         string
		wantClassify bool
	}{
		{"comment", "# reviewed by the platform team\n" + string(original), false},
		{"min_severity", string(original) + "coupling:\n  min_severity: high\n", false},
		{"volatility cascade", string(original) + "coupling:\n  volatility_cascade: true\n", true},
	}
	for _, edit := range edits {
		if err := os.WriteFile(cfgPath, []byte(edit.body), 0o600); err != nil {
			t.Fatal(err)
		}
		reasons, drift, _ := gateReferenceReasons(t, cfgPath)
		joined := strings.Join(reasons, "; ")
		if strings.Contains(joined, "config_hash") {
			t.Errorf("%s: reasons name the raw config_hash: %s", edit.name, joined)
		}
		named := strings.Contains(joined, "classification_hash")
		if named != edit.wantClassify {
			t.Errorf("%s: classification_hash named = %v, want %v (reasons: %s)", edit.name, named, edit.wantClassify, joined)
		}
		hasClass := false
		for _, class := range drift {
			hasClass = hasClass || class == "classification_hash"
		}
		if hasClass != edit.wantClassify {
			t.Errorf("%s: drift = %v, want classification_hash = %v", edit.name, drift, edit.wantClassify)
		}
	}
}

func TestRun_GateReferenceSaysWhenThereIsNoBaselineFile(t *testing.T) {
	t.Parallel()
	cfgPath := writeCoupledRepo(t, distributedMonolithCfg)
	_, _, present := gateReferenceReasons(t, cfgPath)
	if present {
		t.Error("gate_reference.baseline_present = true with no baseline file")
	}
}
