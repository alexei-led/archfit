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

	"github.com/alexei-led/archfit/internal/initcfg"
	"github.com/alexei-led/archfit/internal/llm"
	"github.com/alexei-led/archfit/internal/toolrun"
)

// minimalRoot creates a temp dir with enough structure for Discover to succeed
// (just a go.mod so it finds a Go module root).
func minimalRoot(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "go.mod"), []byte("module example.com/test\n\ngo 1.21\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	return dir
}

// runInitCmd runs InitCmd.Run with the given field values and returns stdout output and error.
// The runner returns empty go list output (zero packages discovered).
func runInitCmd(t *testing.T, cmd *InitCmd) (string, error) {
	t.Helper()
	return runInitCmdWithRunner(t, cmd, func(_ context.Context, _ toolrun.ToolCmd) (toolrun.Output, error) {
		// Empty stdout — go list -json streaming decoder treats this as zero packages.
		return toolrun.Output{}, nil
	})
}

// runInitCmdWithRunner runs InitCmd.Run with a custom RunFunc for the runner mock.
func runInitCmdWithRunner(t *testing.T, cmd *InitCmd, runFn func(context.Context, toolrun.ToolCmd) (toolrun.Output, error)) (string, error) {
	t.Helper()
	var buf bytes.Buffer
	runner := &toolrun.RunnerMock{
		DetectFunc: func(_ context.Context, _ string) (toolrun.ToolInfo, bool) {
			return toolrun.ToolInfo{}, false
		},
		RunFunc: runFn,
	}
	deps := &appDeps{Runner: runner, Stdout: &buf}
	err := cmd.Run(deps)
	return buf.String(), err
}

// goListJSON returns a single go list -json entry for the given module path and import path.
func goListJSON(modPath, importPath string) []byte {
	return fmt.Appendf(nil,
		`{"ImportPath":%q,"Dir":".","Module":{"Path":%q}}`,
		importPath, modPath,
	)
}

func TestInitCmd_ApplyWithoutLLM_IsError(t *testing.T) {
	root := minimalRoot(t)
	cmd := &InitCmd{
		Root:   root,
		Output: filepath.Join(root, ".archfit.yaml"),
		Apply:  true,
		LLM:    false,
	}
	_, err := runInitCmd(t, cmd)
	if err == nil {
		t.Fatal("expected error for --apply without --llm")
	}
	var ee *exitError
	if !errors.As(err, &ee) || ee.code != 3 {
		t.Errorf("want exitError code 3, got: %v", err)
	}
}

func TestInitCmd_StdoutMode_NoFileWritten(t *testing.T) {
	root := minimalRoot(t)
	outPath := filepath.Join(root, ".archfit.yaml")
	cmd := &InitCmd{
		Root:   root,
		Output: "-",
	}
	out, err := runInitCmd(t, cmd)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(out, "version:") {
		t.Errorf("stdout output missing 'version:'; got: %q", out)
	}
	if _, statErr := os.Stat(outPath); statErr == nil {
		t.Error("file should not have been written in stdout mode")
	}
}

func TestInitCmd_NoLLM_WritesFile(t *testing.T) {
	root := minimalRoot(t)
	outPath := filepath.Join(root, ".archfit.yaml")
	cmd := &InitCmd{
		Root:   root,
		Output: outPath,
	}
	out, err := runInitCmd(t, cmd)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(out, "wrote") {
		t.Errorf("expected 'wrote' in output, got: %q", out)
	}
	data, readErr := os.ReadFile(outPath) //nolint:gosec
	if readErr != nil {
		t.Fatalf("file not written: %v", readErr)
	}
	if !strings.Contains(string(data), "version:") {
		t.Errorf("written file missing 'version:'; content: %q", string(data))
	}
}

func TestInitCmd_LLM_CommentedSuggestions(t *testing.T) {
	root := minimalRoot(t)
	if err := os.MkdirAll(filepath.Join(root, "internal", "mymod"), 0o750); err != nil {
		t.Fatal(err)
	}
	outPath := filepath.Join(root, ".archfit.yaml")

	const modPath = "example.com/test"
	cmd := &InitCmd{
		Root:             root,
		Output:           outPath,
		LLM:              true,
		Apply:            false,
		LLMProvider:      providerAnthropic,
		LLMModel:         defaultLLMModel,
		providerOverride: &flexFakeProvider{subdomain: subdomainCore, volatility: volatilityLow, layer: testLayerDomain},
	}
	_, err := runInitCmdWithRunner(t, cmd, func(_ context.Context, _ toolrun.ToolCmd) (toolrun.Output, error) {
		return toolrun.Output{Stdout: goListJSON(modPath, modPath+"/internal/mymod")}, nil
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	data, readErr := os.ReadFile(outPath) //nolint:gosec
	if readErr != nil {
		t.Fatalf("file not written: %v", readErr)
	}
	// In plan mode (apply=false), LLM suggestions must be commented out.
	if !strings.Contains(string(data), "# subdomain:") {
		t.Errorf("plan mode output missing '# subdomain:' annotation; content: %q", string(data))
	}
	if !strings.Contains(string(data), "# volatility:") {
		t.Errorf("plan mode output missing '# volatility:' annotation; content: %q", string(data))
	}
}

func TestInitCmd_LLMApply_LiveFields(t *testing.T) {
	root := minimalRoot(t)
	if err := os.MkdirAll(filepath.Join(root, "internal", "mymod"), 0o750); err != nil {
		t.Fatal(err)
	}
	outPath := filepath.Join(root, ".archfit.yaml")

	const modPath = "example.com/test"
	cmd := &InitCmd{
		Root:             root,
		Output:           outPath,
		LLM:              true,
		Apply:            true,
		LLMProvider:      providerAnthropic,
		LLMModel:         defaultLLMModel,
		providerOverride: &flexFakeProvider{subdomain: subdomainCore, volatility: volatilityLow, layer: testLayerDomain},
	}
	_, err := runInitCmdWithRunner(t, cmd, func(_ context.Context, _ toolrun.ToolCmd) (toolrun.Output, error) {
		return toolrun.Output{Stdout: goListJSON(modPath, modPath+"/internal/mymod")}, nil
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	data, readErr := os.ReadFile(outPath) //nolint:gosec
	if readErr != nil {
		t.Fatalf("file not written: %v", readErr)
	}
	if !strings.Contains(string(data), "subdomain:") {
		t.Errorf("apply mode output missing live 'subdomain:' field; content: %q", string(data))
	}
	if !strings.Contains(string(data), "volatility:") {
		t.Errorf("apply mode output missing live 'volatility:' field; content: %q", string(data))
	}
}

func TestInitCmd_LLMStdout_NoFileWritten(t *testing.T) {
	root := minimalRoot(t)
	outPath := filepath.Join(root, ".archfit.yaml")

	cmd := &InitCmd{
		Root:             root,
		Output:           "-",
		LLM:              true,
		Apply:            false,
		LLMProvider:      providerAnthropic,
		LLMModel:         defaultLLMModel,
		providerOverride: &flexFakeProvider{subdomain: subdomainCore, volatility: volatilityLow, layer: testLayerDomain},
	}
	out, err := runInitCmd(t, cmd)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(out, "version:") {
		t.Errorf("stdout missing 'version:'; got: %q", out)
	}
	if _, statErr := os.Stat(outPath); statErr == nil {
		t.Error("-o - with --llm must not write file")
	}
}

func TestInitCmd_InvalidExistingConfig_LLMRunsFromFlags(t *testing.T) {
	root := minimalRoot(t)
	outPath := filepath.Join(root, ".archfit.yaml")
	// Write a broken config at the output path.
	if err := os.WriteFile(outPath, []byte("this: is: not: valid: yaml: [[["), 0o600); err != nil {
		t.Fatal(err)
	}

	cmd := &InitCmd{
		Root:             root,
		Output:           outPath,
		LLM:              true,
		Apply:            false,
		LLMProvider:      providerAnthropic,
		LLMModel:         defaultLLMModel,
		providerOverride: &flexFakeProvider{subdomain: subdomainCore, volatility: volatilityLow, layer: testLayerDomain},
	}
	// Should NOT error — tolerate broken existing config and run from flags.
	_, err := runInitCmd(t, cmd)
	if err != nil {
		t.Fatalf("unexpected error with broken existing config: %v", err)
	}
}

// flexFakeProvider generates responses by extracting module names from the user prompt.
type flexFakeProvider struct {
	subdomain  string
	volatility string
	layer      string
}

func (p *flexFakeProvider) Name() string { return "test/flex" }
func (p *flexFakeProvider) Complete(_ context.Context, req llm.Request) (llm.Response, error) {
	// Extract module names from lines like "- module: <name>" in the user prompt.
	var entries []string
	for _, line := range strings.Split(req.User, "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "- module: ") {
			name := strings.TrimSpace(strings.TrimPrefix(line, "- module: "))
			entries = append(entries, fmt.Sprintf(
				`{"module":%q,"subdomain":%q,"volatility":%q,"layer":%q,"name":"","rationale":"test"}`,
				name, p.subdomain, p.volatility, p.layer,
			))
		}
	}
	if len(entries) == 0 {
		return llm.Response{Text: "[]"}, nil
	}
	return llm.Response{Text: "[" + strings.Join(entries, ",") + "]"}, nil
}

// Ensure flexFakeProvider satisfies llm.Provider at compile time.
var _ llm.Provider = (*flexFakeProvider)(nil)

// Ensure initcfg import is used (ClassifyTarget referenced here for compile-time check).
var _ = initcfg.ClassifyTarget{}
