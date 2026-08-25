package main

import (
	"bytes"
	"encoding/json"
	"slices"
	"strings"
	"testing"
)

const (
	characterizationGateFindingID     = "2b94da420ec7ed905e95b7565c44b316"
	characterizationAdvisoryFindingID = "908bc68647ca3f180713dd7d90f3c774"
	characterizationBCRuleID          = "bc/imbalanced_coupling"
	characterizationScoreDimension    = "coupling_balance"
)

type characterizationDiagnostic struct {
	Verdict string `json:"verdict"`
	Summary struct {
		GateFindings int `json:"gate_findings"`
		Warnings     int `json:"warnings"`
	} `json:"summary"`
	Findings []struct {
		ID       string `json:"id"`
		Kind     string `json:"kind"`
		RuleID   string `json:"rule_id"`
		Status   string `json:"status"`
		Severity string `json:"severity"`
	} `json:"findings"`
	ToolCoverage []struct {
		Tool   string `json:"tool"`
		Status string `json:"status"`
	} `json:"tool_coverage"`
}

func runCharacterizationFormat(t *testing.T, command, configPath, format string) (int, []byte) {
	t.Helper()
	var stdout, stderr bytes.Buffer
	code := RunWithStderr(
		[]string{command, "-c", configPath, flagRefresh, "--progress=none", "--format=" + format},
		&stdout,
		&stderr,
	)
	if code == 3 {
		t.Fatalf("%s --format=%s failed: exit=%d\nstdout:\n%s\nstderr:\n%s", command, format, code, stdout.String(), stderr.String())
	}
	return code, stdout.Bytes()
}

func TestAnalyzeCheckCharacterization(t *testing.T) {
	configPath := writeViolatingRepo(t)

	checkCode, firstJSON := runCharacterizationFormat(t, cmdCheck, configPath, formatJSON)
	if checkCode != 1 {
		t.Fatalf("check exit = %d, want 1", checkCode)
	}
	secondCode, secondJSON := runCharacterizationFormat(t, cmdCheck, configPath, formatJSON)
	if secondCode != checkCode {
		t.Fatalf("second check exit = %d, want %d", secondCode, checkCode)
	}
	if !bytes.Equal(firstJSON, secondJSON) {
		t.Fatal("identical check runs produced different JSON")
	}
	assertCharacterizationJSON(t, firstJSON)

	analyzeCode, analyzeJSON := runCharacterizationFormat(t, cmdAnalyze, configPath, formatJSON)
	if analyzeCode != 0 {
		t.Fatalf("analyze exit = %d, want 0", analyzeCode)
	}
	if !bytes.Equal(analyzeJSON, firstJSON) {
		t.Fatal("analyze and check JSON differ for the same report-only-independent findings")
	}

	assertCharacterizationMarkdown(t, configPath)
	assertCharacterizationSARIF(t, configPath)
	assertCharacterizationScorecard(t, configPath)
}

func assertCharacterizationJSON(t *testing.T, output []byte) {
	t.Helper()
	var diagnostic characterizationDiagnostic
	if err := json.Unmarshal(output, &diagnostic); err != nil {
		t.Fatalf("decode check JSON: %v", err)
	}
	if diagnostic.Verdict != "fail" {
		t.Errorf("verdict = %q, want fail", diagnostic.Verdict)
	}
	if diagnostic.Summary.GateFindings != 1 || diagnostic.Summary.Warnings != 1 {
		t.Errorf("summary = gates:%d warnings:%d, want gates:1 warnings:1", diagnostic.Summary.GateFindings, diagnostic.Summary.Warnings)
	}
	wantFindings := []struct {
		id, kind, rule, status, severity string
	}{
		{characterizationGateFindingID, "gate", ruleNoInternalAcc, "new", volatilityHigh},
		{characterizationAdvisoryFindingID, "advisory", characterizationBCRuleID, "new", volatilityHigh},
	}
	if len(diagnostic.Findings) != len(wantFindings) {
		t.Fatalf("findings = %d, want %d: %+v", len(diagnostic.Findings), len(wantFindings), diagnostic.Findings)
	}
	for i, want := range wantFindings {
		got := diagnostic.Findings[i]
		if got.ID != want.id || got.Kind != want.kind || got.RuleID != want.rule || got.Status != want.status || got.Severity != want.severity {
			t.Errorf("finding[%d] = %+v, want id=%s kind=%s rule=%s status=%s severity=%s", i, got, want.id, want.kind, want.rule, want.status, want.severity)
		}
	}
	coverageTools := make([]string, 0, len(diagnostic.ToolCoverage))
	for _, coverage := range diagnostic.ToolCoverage {
		coverageTools = append(coverageTools, coverage.Tool)
	}
	if !slices.Contains(coverageTools, toolGoPackages) {
		t.Errorf("tool_coverage = %v, want %s", coverageTools, toolGoPackages)
	}
}

func assertCharacterizationMarkdown(t *testing.T, configPath string) {
	t.Helper()
	code, output := runCharacterizationFormat(t, cmdCheck, configPath, formatMarkdown)
	if code != 1 {
		t.Fatalf("markdown check exit = %d, want 1", code)
	}
	for _, want := range []string{"FAIL", ruleNoInternalAcc, characterizationBCRuleID} {
		if !strings.Contains(string(output), want) {
			t.Errorf("markdown output missing %q", want)
		}
	}
}

func assertCharacterizationSARIF(t *testing.T, configPath string) {
	t.Helper()
	code, output := runCharacterizationFormat(t, cmdCheck, configPath, formatSarif)
	if code != 1 {
		t.Fatalf("SARIF check exit = %d, want 1", code)
	}
	var document struct {
		Version string `json:"version"`
		Runs    []struct {
			Results []struct {
				Fingerprints map[string]string `json:"fingerprints"`
			} `json:"results"`
		} `json:"runs"`
	}
	if err := json.Unmarshal(output, &document); err != nil {
		t.Fatalf("decode SARIF: %v", err)
	}
	if document.Version != "2.1.0" || len(document.Runs) != 1 {
		t.Fatalf("SARIF shape = version:%q runs:%d", document.Version, len(document.Runs))
	}
	gotFingerprints := make([]string, 0, len(document.Runs[0].Results))
	for _, result := range document.Runs[0].Results {
		gotFingerprints = append(gotFingerprints, result.Fingerprints["archfit/v1"])
	}
	wantFingerprints := []string{characterizationGateFindingID, characterizationAdvisoryFindingID}
	if !slices.Equal(gotFingerprints, wantFingerprints) {
		t.Errorf("SARIF fingerprints = %v, want %v", gotFingerprints, wantFingerprints)
	}
}

func assertCharacterizationScorecard(t *testing.T, configPath string) {
	t.Helper()
	code, output := runCharacterizationFormat(t, cmdCheck, configPath, formatScorecard)
	if code != 1 {
		t.Fatalf("scorecard check exit = %d, want 1", code)
	}
	for _, want := range []string{"# archfit scorecard", characterizationScoreDimension, "**Overall:**"} {
		if !strings.Contains(string(output), want) {
			t.Errorf("scorecard output missing %q", want)
		}
	}
}
