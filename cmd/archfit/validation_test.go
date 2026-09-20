package main

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestAgentValidationRetainsAnalysisFlags(t *testing.T) {
	cfgPath := writeViolatingRepo(t)
	var stdout, stderr bytes.Buffer
	code := RunWithStderr([]string{cmdCheck, "-c", cfgPath, "--lang", "go", requireToolsFlag, flagRefresh, flagJSON}, &stdout, &stderr)
	if code != 1 {
		t.Fatalf("exit = %d, want blocked; stderr=%s", code, &stderr)
	}
	var doc struct {
		AgentTasks []struct {
			Validation []string `json:"validation"`
			RepairKind string   `json:"repair_kind"`
		} `json:"agent_tasks"`
	}
	if err := json.Unmarshal(stdout.Bytes(), &doc); err != nil {
		t.Fatal(err)
	}
	if len(doc.AgentTasks) == 0 || len(doc.AgentTasks[0].Validation) != 1 {
		t.Fatalf("missing repair validation: %s", &stdout)
	}
	command := doc.AgentTasks[0].Validation[0]
	for _, flag := range []string{"--lang go", requireToolsFlag} {
		if !strings.Contains(command, flag) {
			t.Errorf("validation %q lost %q", command, flag)
		}
	}
	args := strings.Fields(command)
	var replayOut, replayErr bytes.Buffer
	if got := RunWithStderr(args[1:], &replayOut, &replayErr); got != code {
		t.Fatalf("replay exit=%d, original=%d; stderr=%s", got, code, &replayErr)
	}
	writeFileAt(t, filepath.Dir(cfgPath), filePkgAA, "package a\n\nfunc UseSecret() string { return \"local behavior\" }\n")
	replayOut.Reset()
	replayErr.Reset()
	if got := RunWithStderr(args[1:], &replayOut, &replayErr); got != 0 && got != 2 {
		t.Fatalf("repaired dependency failed validation: exit=%d; stderr=%s", got, &replayErr)
	}
	if _, err := os.Stat(filepath.Join(filepath.Dir(cfgPath), defaultBaselinePath)); !os.IsNotExist(err) {
		t.Fatalf("code repair changed accepted debt: %v", err)
	}
}
