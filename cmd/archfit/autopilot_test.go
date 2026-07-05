package main

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/alexei-led/archfit/internal/llm"
	"github.com/alexei-led/archfit/internal/toolrun"
)

// autopilotProvider answers both the classify and the owner-draft calls by
// reading module names from the user prompt and branching on the system prompt.
type autopilotProvider struct{}

func (autopilotProvider) Name() string { return "test/autopilot" }
func (autopilotProvider) Complete(_ context.Context, req llm.Request) (llm.Response, error) {
	owner := strings.Contains(req.System, "responsible owner")
	ref := firstEvidenceRefForTest(req.User)
	var parts []string
	for _, line := range strings.Split(req.User, "\n") {
		line = strings.TrimSpace(line)
		if !strings.HasPrefix(line, "- module: ") {
			continue
		}
		name := strings.TrimSpace(strings.TrimPrefix(line, "- module: "))
		if owner {
			parts = append(parts, fmt.Sprintf(`{"module":%q,"value":"@team-%s","rationale":"t cites %s","evidence_refs":[%q],"basis":"semantic_judgment"}`, name, name, ref, ref))
		} else {
			parts = append(parts, fmt.Sprintf(
				`{"module":%q,"subdomain":"core","volatility":"low","layer":"core","role":"core","name":"","rationale":"t cites %s","evidence_refs":[%q],"basis":"semantic_judgment"}`, name, ref, ref))
		}
	}
	if len(parts) == 0 {
		return llm.Response{Text: "[]"}, nil
	}
	return llm.Response{Text: "[" + strings.Join(parts, ",") + "]"}, nil
}

// TestInit_LLMDraft_OwnerCommentWritten verifies the owner-draft pass that was
// folded from the former `autopilot` command into `config init --llm`: when
// Apply is false, the owner suggestion must appear as a commented annotation.
// The subdomain/volatility side is covered by TestInitCmd_LLM_CommentedSuggestions.
func TestInit_LLMDraft_OwnerCommentWritten(t *testing.T) {
	t.Parallel()
	root := minimalRoot(t)
	if err := os.MkdirAll(filepath.Join(root, "internal", "mymod"), 0o750); err != nil {
		t.Fatal(err)
	}
	outPath := filepath.Join(root, defaultConfigPath)
	const modPath = "example.com/test"

	cmd := &InitCmd{
		Root:             root,
		Output:           outPath,
		LLM:              true,
		Apply:            false,
		LLMProvider:      providerAnthropic,
		LLMModel:         defaultLLMModel,
		providerOverride: autopilotProvider{},
	}
	_, err := runInitCmdWithRunner(t, cmd, func(_ context.Context, _ toolrun.ToolCmd) (toolrun.Output, error) {
		return toolrun.Output{Stdout: goListJSON(modPath, modPath+"/internal/mymod")}, nil
	})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}

	data, err := os.ReadFile(outPath) //nolint:gosec // test temp path
	if err != nil {
		t.Fatalf("config not written: %v", err)
	}
	got := string(data)
	// Owner draft pass must write a commented annotation (plan mode = never live).
	if !strings.Contains(got, "# owner:") {
		t.Errorf("init --llm plan mode missing commented owner suggestion:\n%s", got)
	}
	// Verify no uncommented live owner field leaked through.
	for _, line := range strings.Split(got, "\n") {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "owner:") {
			t.Errorf("plan-mode leaked a live owner field %q:\n%s", trimmed, got)
		}
	}
}
