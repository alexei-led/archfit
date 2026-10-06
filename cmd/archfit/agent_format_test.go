package main

import (
	"encoding/json"
	"slices"
	"strings"
	"testing"
)

// agentResult is the part of archfit.agent-result.v1 these tests read.
type agentResult struct {
	SchemaVersion string `json:"schema_version"`
	Verdict       string `json:"verdict"`
	NextAction    string `json:"next_action"`
	Repairs       []struct {
		FindingIDs []string `json:"finding_ids"`
		RuleIDs    []string `json:"rule_ids"`
		Edit       []string `json:"edit"`
	} `json:"repairs"`
	WorsenedMetrics []struct {
		Name string `json:"name"`
	} `json:"worsened_metrics"`
	Omitted struct {
		Repairs int `json:"repairs"`
	} `json:"omitted"`
	Truncated bool   `json:"truncated"`
	Validate  string `json:"validate"`
}

func decodeAgentResult(t *testing.T, raw string) agentResult {
	t.Helper()
	var r agentResult
	if err := json.Unmarshal([]byte(raw), &r); err != nil {
		t.Fatalf("decode agent result: %v\n%s", err, raw)
	}
	return r
}

// TestFormatMatrix_AgentDigestCarriesTheState is the agent format's half of the
// parity rule. The digest is exempt from layout parity, as SARIF is, but not
// from facts: it reports the same verdict as --format json, every active gate
// finding of the state reaches a repair, and check exits with the same code.
func TestFormatMatrix_AgentDigestCarriesTheState(t *testing.T) {
	t.Parallel()
	requireHealthyExtraction(t)
	cfgPath := writeViolatingRepo(t)

	var state struct {
		Verdict  string `json:"verdict"`
		Findings []struct {
			ID     string `json:"id"`
			Kind   string `json:"kind"`
			Status string `json:"status"`
		} `json:"findings"`
	}
	jsonCode, stateJSON, _ := runArchfit(t, cmdCheck, "-c", cfgPath, "--progress=none", "--format="+formatJSON)
	if err := json.Unmarshal([]byte(stateJSON), &state); err != nil {
		t.Fatalf("decode state: %v", err)
	}
	agentCode, raw, stderr := runArchfit(t, cmdCheck, "-c", cfgPath, "--progress=none", "--format="+formatAgent)
	if agentCode != jsonCode {
		t.Errorf("check --format agent exit = %d, --format json exit = %d: the format changed the verdict\nstderr:\n%s", agentCode, jsonCode, stderr)
	}
	got := decodeAgentResult(t, raw)

	if got.SchemaVersion != "archfit.agent-result.v1" || got.Verdict != state.Verdict {
		t.Errorf("schema %q verdict %q, want archfit.agent-result.v1 and %q", got.SchemaVersion, got.Verdict, state.Verdict)
	}
	if got.NextAction != "repair" {
		t.Errorf("next_action = %q, want repair for an in-scope forbidden dependency", got.NextAction)
	}
	var inRepairs []string
	for _, r := range got.Repairs {
		inRepairs = append(inRepairs, r.FindingIDs...)
	}
	active := 0
	for _, f := range state.Findings {
		if f.Kind != findingKindGate || (f.Status != findingStatusNew && f.Status != "expired_waiver") {
			continue
		}
		active++
		if !slices.Contains(inRepairs, f.ID) && got.Omitted.Repairs == 0 {
			t.Errorf("active gate finding %s is in no repair and nothing was omitted", f.ID)
		}
	}
	if active == 0 {
		t.Fatal("the fixture produced no active gate finding: the completeness check is vacuous")
	}
	if !strings.HasSuffix(got.Validate, " --format agent") || !strings.Contains(got.Validate, "archfit check -c ") {
		t.Errorf("validate = %q, want the check command replayed with --format agent", got.Validate)
	}
}

// TestFormatMatrix_AgentDigestNamesTheRatchet pins the agent channel of a
// metric ratchet: the block produces no finding and no task, so the digest
// names the worsened metric and still tells the agent to repair.
func TestFormatMatrix_AgentDigestNamesTheRatchet(t *testing.T) {
	t.Parallel()
	code, raw, stderr := runArchfit(t, cmdCheck, "-c", writeMetricRegressionRepo(t), "--progress=none", "--format="+formatAgent)
	if code != 1 {
		t.Fatalf("check --format agent: exit = %d, want 1 (ratchet block)\nstderr:\n%s", code, stderr)
	}
	got := decodeAgentResult(t, raw)
	if got.NextAction != "repair" || len(got.Repairs) != 0 {
		t.Errorf("next_action = %q with %d repairs, want repair with none", got.NextAction, len(got.Repairs))
	}
	if len(got.WorsenedMetrics) != 1 || got.WorsenedMetrics[0].Name != "coverage" {
		t.Errorf("worsened_metrics = %+v, want [coverage]", got.WorsenedMetrics)
	}
}
