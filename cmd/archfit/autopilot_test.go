package main

import (
	"bytes"
	"context"
	"errors"
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
	var parts []string
	for _, line := range strings.Split(req.User, "\n") {
		line = strings.TrimSpace(line)
		if !strings.HasPrefix(line, "- module: ") {
			continue
		}
		name := strings.TrimSpace(strings.TrimPrefix(line, "- module: "))
		if owner {
			parts = append(parts, fmt.Sprintf(`{"module":%q,"value":"@team-%s","rationale":"t"}`, name, name))
		} else {
			parts = append(parts, fmt.Sprintf(
				`{"module":%q,"subdomain":"core","volatility":"low","layer":"core","role":"core","name":"","rationale":"t"}`, name))
		}
	}
	if len(parts) == 0 {
		return llm.Response{Text: "[]"}, nil
	}
	return llm.Response{Text: "[" + strings.Join(parts, ",") + "]"}, nil
}

func runAutopilot(t *testing.T, cmd *AutopilotCmd) (string, error) {
	t.Helper()
	var buf bytes.Buffer
	runner := &toolrun.RunnerMock{
		DetectFunc: func(_ context.Context, _ string) (toolrun.ToolInfo, bool) { return toolrun.ToolInfo{}, false },
		RunFunc: func(_ context.Context, _ toolrun.ToolCmd) (toolrun.Output, error) {
			return toolrun.Output{Stdout: goListJSON("example.com/test", "example.com/test/internal/mymod")}, nil
		},
	}
	err := cmd.Run(&appDeps{Runner: runner, Stdout: &buf})
	return buf.String(), err
}

func TestAutopilot_WritesDraftAppliesNothing(t *testing.T) {
	root := minimalRoot(t)
	if err := os.MkdirAll(filepath.Join(root, "internal", "mymod"), 0o750); err != nil {
		t.Fatal(err)
	}
	outPath := filepath.Join(root, defaultAutopilotPath)
	livePath := filepath.Join(root, defaultConfigPath)

	cmd := &AutopilotCmd{
		Root:             root,
		Config:           livePath,
		Output:           outPath,
		LLMProvider:      providerAnthropic,
		LLMModel:         defaultLLMModel,
		providerOverride: autopilotProvider{},
	}
	out, err := runAutopilot(t, cmd)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if !strings.Contains(out, "draft written") {
		t.Errorf("expected draft message, got: %s", out)
	}

	// The draft file exists and carries commented (plan-mode) suggestions, never live fields.
	data, err := os.ReadFile(outPath) //nolint:gosec // test temp path
	if err != nil {
		t.Fatalf("draft not written: %v", err)
	}
	draft := string(data)
	if !strings.Contains(draft, "# subdomain:") {
		t.Errorf("draft missing commented subdomain suggestion:\n%s", draft)
	}
	if !strings.Contains(draft, "# owner:") {
		t.Errorf("draft missing commented owner suggestion:\n%s", draft)
	}

	// Applies nothing: the live config must not have been created.
	if _, statErr := os.Stat(livePath); statErr == nil {
		t.Error("autopilot must not write .archfit.yaml")
	}
}

func TestAutopilot_RefusesToWriteLiveConfig(t *testing.T) {
	root := minimalRoot(t)
	cmd := &AutopilotCmd{
		Root:             root,
		Output:           filepath.Join(root, defaultConfigPath), // pointed at the live config
		providerOverride: autopilotProvider{},
	}
	_, err := runAutopilot(t, cmd)
	if err == nil {
		t.Fatal("expected refusal when --output is .archfit.yaml")
	}
	var ee *exitError
	if !errors.As(err, &ee) || ee.code != 3 {
		t.Errorf("want exitError code 3, got: %v", err)
	}
}
