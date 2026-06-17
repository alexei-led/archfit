package main

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/alexei-led/archfit/internal/toolrun"
)

// runUpdateCmd runs UpdateCmd.Run with a fake runner and returns stdout output and error.
func runUpdateCmd(t *testing.T, cmd *UpdateCmd, runner toolrun.Runner) (string, error) {
	t.Helper()
	if runner == nil {
		runner = emptyRunner()
	}
	var buf bytes.Buffer
	deps := &appDeps{Runner: runner, Stdout: &buf}
	err := cmd.Run(deps)
	return buf.String(), err
}

// emptyRunner returns a RunnerMock that emits no packages from go list.
func emptyRunner() *toolrun.RunnerMock {
	return &toolrun.RunnerMock{
		DetectFunc: func(_ context.Context, _ string) (toolrun.ToolInfo, bool) {
			return toolrun.ToolInfo{}, false
		},
		RunFunc: func(_ context.Context, _ toolrun.ToolCmd) (toolrun.Output, error) {
			return toolrun.Output{}, nil
		},
	}
}

// matchingRunner returns a runner that emits exactly one package entry so that
// discovery returns a module at "internal/<sub>" with the given module path.
// pkgRel is relative to modPath, e.g. "internal/mymod".
func matchingRunner(pkgRel string) *toolrun.RunnerMock {
	const modPath = "example.com/test"
	return &toolrun.RunnerMock{
		DetectFunc: func(_ context.Context, _ string) (toolrun.ToolInfo, bool) {
			return toolrun.ToolInfo{}, false
		},
		RunFunc: func(_ context.Context, _ toolrun.ToolCmd) (toolrun.Output, error) {
			// ndjson stream — one object per package, no array wrapper.
			entry := fmt.Sprintf(
				`{"ImportPath":%q,"Dir":"/ignored","Module":{"Path":%q}}`,
				modPath+"/"+pkgRel, modPath,
			)
			return toolrun.Output{Stdout: []byte(entry)}, nil
		},
	}
}

// writeConfig writes content to <dir>/.archfit.yaml and returns the path.
func writeConfig(t *testing.T, dir, content string) string {
	t.Helper()
	path := filepath.Join(dir, ".archfit.yaml")
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

const (
	// minimalConfigNoModules is a valid config with no modules section.
	// Structurally in sync with empty discovery (Added=[], Removed=[], Drift=[]).
	minimalConfigNoModules = `version: 1
layers:
  - core
  - adapter
rules:
  - id: no-bad-deps
    type: forbidden_dependency
    gate: warn
`
	// configWithRemovedModule has a module that empty discovery will mark as Removed.
	configWithRemovedModule = `version: 1
layers:
  - core
  - adapter
# a comment that must survive
modules:
  oldmod:
    paths:
      - "internal/oldmod/**"
    layer: core
# another comment
rules:
  - id: no-bad-deps
    type: forbidden_dependency
    gate: warn
`

	// layerCore is the "core" layer string used in layer assertions. It equals
	// subdomainCore in value but represents a layer, not a subdomain.
	// Reuse subdomainCore to satisfy goconst (same string "core").
	layerCoreStr = subdomainCore
)

// TestUpdateCmd_PlanMode_FileUnchanged verifies plan mode (no --apply) leaves the
// config file byte-identical.
func TestUpdateCmd_PlanMode_FileUnchanged(t *testing.T) {
	dir := minimalRoot(t)
	cfgPath := writeConfig(t, dir, configWithRemovedModule)
	before, err := os.ReadFile(cfgPath) //nolint:gosec
	if err != nil {
		t.Fatal(err)
	}

	cmd := &UpdateCmd{
		Config: cfgPath,
		Root:   dir,
	}
	out, err := runUpdateCmd(t, cmd, emptyRunner())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(out, "REMOVED") {
		t.Errorf("plan output should mention REMOVED; got: %q", out)
	}

	after, err := os.ReadFile(cfgPath) //nolint:gosec
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(before, after) {
		t.Error("plan mode must leave config file byte-unchanged")
	}
}

// TestUpdateCmd_Apply_CommentsRemoved verifies --apply: comments a removed module
// and preserves rules and comments.
func TestUpdateCmd_Apply_CommentsRemoved(t *testing.T) {
	dir := minimalRoot(t)
	cfgPath := writeConfig(t, dir, configWithRemovedModule)

	cmd := &UpdateCmd{
		Config: cfgPath,
		Root:   dir,
		Apply:  true,
	}
	out, err := runUpdateCmd(t, cmd, emptyRunner())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(out, "wrote") {
		t.Errorf("apply output should mention 'wrote'; got: %q", out)
	}

	data, err := os.ReadFile(cfgPath) //nolint:gosec
	if err != nil {
		t.Fatal(err)
	}
	content := string(data)

	// Removed module should be commented out.
	if !strings.Contains(content, "archfit: removed module") {
		t.Error("removed module should have archfit marker comment")
	}
	// Rules must be preserved.
	if !strings.Contains(content, "no-bad-deps") {
		t.Error("rules must be preserved after apply")
	}
	if !strings.Contains(content, "forbidden_dependency") {
		t.Error("rules content should be preserved after apply")
	}
	// Backup must exist.
	bakPath := cfgPath + ".bak"
	if _, statErr := os.Stat(bakPath); statErr != nil {
		t.Error("backup (.archfit.yaml.bak) should exist after apply")
	}
}

// TestUpdateCmd_LLMApply_WritesOnlyAbsentFields verifies that --llm --apply on a
// structurally in-sync config writes only absent fields.
//
// Setup: go.mod + module "mymod" at internal/mymod matched by discovery.
// Config has mymod with subdomain+volatility but no layer.
// LLM suggests layer=core (in layers). Expected: only layer is added.
func TestUpdateCmd_LLMApply_WritesOnlyAbsentFields(t *testing.T) {
	dir := minimalRoot(t)
	cfg := `version: 1
layers:
  - core
  - adapter
modules:
  mymod:
    paths:
      - "internal/mymod/**"
    subdomain: supporting
    volatility: medium
rules:
  - id: no-bad-deps
    type: forbidden_dependency
    gate: warn
`
	cfgPath := writeConfig(t, dir, cfg)
	runner := matchingRunner("internal/mymod")

	cmd := &UpdateCmd{
		Config:           cfgPath,
		Root:             dir,
		LLM:              true,
		Apply:            true,
		LLMProvider:      providerAnthropic,
		LLMModel:         defaultLLMModel,
		providerOverride: &flexFakeProvider{subdomain: subdomainCore, volatility: volatilityLow, layer: layerCoreStr},
	}
	_, err := runUpdateCmd(t, cmd, runner)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	data, err := os.ReadFile(cfgPath) //nolint:gosec
	if err != nil {
		t.Fatal(err)
	}
	content := string(data)

	// layer should now be written.
	if !strings.Contains(content, "layer: core") {
		t.Errorf("expected 'layer: core' to be written; content:\n%s", content)
	}
	// Existing fields must not be duplicated.
	if strings.Count(content, "subdomain:") > 1 {
		t.Error("subdomain should not be duplicated")
	}
	if strings.Count(content, "volatility:") > 1 {
		t.Error("volatility should not be duplicated")
	}
}

// TestUpdateCmd_LLMApply_NoSetFieldsForAddedModule verifies that SetModuleFields is
// never emitted for a newly-added module (AddModule handles classification).
func TestUpdateCmd_LLMApply_NoSetFieldsForAddedModule(t *testing.T) {
	dir := minimalRoot(t)
	// Config has no modules; discovery will find one (it will be Added).
	cfgPath := writeConfig(t, dir, minimalConfigNoModules)
	runner := matchingRunner("internal/newmod")

	cmd := &UpdateCmd{
		Config:           cfgPath,
		Root:             dir,
		LLM:              true,
		Apply:            true,
		LLMProvider:      providerAnthropic,
		LLMModel:         defaultLLMModel,
		providerOverride: &flexFakeProvider{subdomain: subdomainCore, volatility: volatilityLow, layer: layerCoreStr},
	}
	_, err := runUpdateCmd(t, cmd, runner)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	data, err := os.ReadFile(cfgPath) //nolint:gosec
	if err != nil {
		t.Fatal(err)
	}
	content := string(data)

	// The new module should appear once (from AddModule).
	count := strings.Count(content, "newmod:")
	if count != 1 {
		t.Errorf("newmod should appear exactly once, got %d; content:\n%s", count, content)
	}
	// No duplicate field blocks (SetModuleFields on AddModule would cause duplicates).
	if strings.Count(content, "subdomain:") > 1 {
		t.Errorf("subdomain should appear at most once; content:\n%s", content)
	}
}

// TestUpdateCmd_ExistingLayerNeverOverwritten verifies that an existing module's
// layer field is never replaced by the LLM suggestion.
func TestUpdateCmd_ExistingLayerNeverOverwritten(t *testing.T) {
	dir := minimalRoot(t)
	cfg := `version: 1
layers:
  - core
  - adapter
modules:
  mymod:
    paths:
      - "internal/mymod/**"
    layer: adapter
rules:
  - id: no-bad-deps
    type: forbidden_dependency
    gate: warn
`
	cfgPath := writeConfig(t, dir, cfg)
	runner := matchingRunner("internal/mymod")

	// LLM suggests layer=core, but mymod already has layer=adapter.
	cmd := &UpdateCmd{
		Config:           cfgPath,
		Root:             dir,
		LLM:              true,
		Apply:            true,
		LLMProvider:      providerAnthropic,
		LLMModel:         defaultLLMModel,
		providerOverride: &flexFakeProvider{subdomain: subdomainCore, volatility: volatilityLow, layer: layerCoreStr},
	}
	_, err := runUpdateCmd(t, cmd, runner)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	data, err := os.ReadFile(cfgPath) //nolint:gosec
	if err != nil {
		t.Fatal(err)
	}
	content := string(data)

	// Original layer must survive unchanged.
	if !strings.Contains(content, "layer: adapter") {
		t.Errorf("original layer 'adapter' must be preserved; content:\n%s", content)
	}
	if strings.Count(content, "layer:") > 1 {
		t.Errorf("layer field must not be duplicated; content:\n%s", content)
	}
}

// TestUpdateCmd_MissingLayerFilledWhenValid verifies that a module with
// subdomain+volatility but no layer gets the layer written when the LLM
// returns a value that is in the allowed set.
func TestUpdateCmd_MissingLayerFilledWhenValid(t *testing.T) {
	dir := minimalRoot(t)
	cfg := `version: 1
layers:
  - core
  - adapter
modules:
  mymod:
    paths:
      - "internal/mymod/**"
    subdomain: supporting
    volatility: low
rules:
  - id: no-bad-deps
    type: forbidden_dependency
    gate: warn
`
	cfgPath := writeConfig(t, dir, cfg)
	runner := matchingRunner("internal/mymod")

	cmd := &UpdateCmd{
		Config:           cfgPath,
		Root:             dir,
		LLM:              true,
		Apply:            true,
		LLMProvider:      providerAnthropic,
		LLMModel:         defaultLLMModel,
		providerOverride: &flexFakeProvider{subdomain: subdomainCore, volatility: volatilityLow, layer: "adapter"},
	}
	_, err := runUpdateCmd(t, cmd, runner)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	data, err := os.ReadFile(cfgPath) //nolint:gosec
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), "layer: adapter") {
		t.Errorf("expected 'layer: adapter' to be written; content:\n%s", string(data))
	}
}

// TestUpdateCmd_OutOfSetLayerWritesNothing verifies that when the only pending
// field-fill is a layer not in layers:, no backup is created and the file is unchanged.
func TestUpdateCmd_OutOfSetLayerWritesNothing(t *testing.T) {
	dir := minimalRoot(t)
	// mymod has subdomain+volatility but no layer; LLM will suggest "infra" NOT in layers.
	cfg := `version: 1
layers:
  - core
  - adapter
modules:
  mymod:
    paths:
      - "internal/mymod/**"
    subdomain: supporting
    volatility: low
rules:
  - id: no-bad-deps
    type: forbidden_dependency
    gate: warn
`
	cfgPath := writeConfig(t, dir, cfg)
	before, err := os.ReadFile(cfgPath) //nolint:gosec
	if err != nil {
		t.Fatal(err)
	}

	runner := matchingRunner("internal/mymod")

	// flexFakeProvider returns layer="infra" (NOT in layers: [core, adapter]).
	// subdomain and volatility are already present → their fills are skipped.
	// Layer fill fails the in-set check → no actionable edits → nothing written.
	cmd := &UpdateCmd{
		Config:           cfgPath,
		Root:             dir,
		LLM:              true,
		Apply:            true,
		LLMProvider:      providerAnthropic,
		LLMModel:         defaultLLMModel,
		providerOverride: &flexFakeProvider{subdomain: subdomainCore, volatility: volatilityLow, layer: "infra"},
	}
	_, err = runUpdateCmd(t, cmd, runner)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	after, err := os.ReadFile(cfgPath) //nolint:gosec
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(before, after) {
		t.Error("file must be unchanged when no actionable edits exist")
	}

	bakPath := cfgPath + ".bak"
	if _, statErr := os.Stat(bakPath); statErr == nil {
		t.Error("backup must not be created when nothing is written")
	}
}

// TestUpdateCmd_BackupCreatedOnApply verifies a backup file is created on a
// real structural apply.
func TestUpdateCmd_BackupCreatedOnApply(t *testing.T) {
	dir := minimalRoot(t)
	cfgPath := writeConfig(t, dir, configWithRemovedModule)

	cmd := &UpdateCmd{
		Config: cfgPath,
		Root:   dir,
		Apply:  true,
	}
	_, err := runUpdateCmd(t, cmd, emptyRunner())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if _, statErr := os.Stat(cfgPath + ".bak"); statErr != nil {
		t.Error("backup (.archfit.yaml.bak) should exist after apply")
	}
}

// TestUpdateCmd_ChangedSinceReadAborts verifies that safeWriteConfig aborts when
// the config file changes between the initial read and the write step.
func TestUpdateCmd_ChangedSinceReadAborts(t *testing.T) {
	dir := minimalRoot(t)
	cfgPath := writeConfig(t, dir, configWithRemovedModule)

	original, err := os.ReadFile(cfgPath) //nolint:gosec
	if err != nil {
		t.Fatal(err)
	}

	// Simulate a concurrent write by modifying the file on disk.
	modified := make([]byte, len(original)+len("\n# concurrent change\n"))
	copy(modified, original)
	copy(modified[len(original):], "\n# concurrent change\n")
	if err := os.WriteFile(cfgPath, modified, 0o600); err != nil { //nolint:gosec
		t.Fatal(err)
	}

	// safeWriteConfig re-reads the file and compares against original.
	// Since the file now differs, it must return a "changed since read" error.
	var buf bytes.Buffer
	deps := &appDeps{Runner: emptyRunner(), Stdout: &buf}
	ctx := context.Background()
	err = safeWriteConfig(ctx, deps, cfgPath, original, original)
	if err == nil {
		t.Fatal("expected error when file changed since read")
	}
	if !strings.Contains(err.Error(), "changed since read") {
		t.Errorf("error should mention 'changed since read'; got: %v", err)
	}
}
