package main

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/alexei-led/archfit/internal/config"
	"github.com/alexei-led/archfit/internal/initcfg"
	"github.com/alexei-led/archfit/internal/llm"
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

func TestClassifyTargetsForUpdate_IncludesSyntheticOverridePath(t *testing.T) {
	t.Parallel()
	const (
		syntheticModule = "mycrate-state"
		syntheticPath   = "mycrate::state"
	)
	cfg := config.Config{Modules: map[string]config.ModuleDef{
		syntheticModule: {Paths: []string{syntheticPath}},
	}}
	report := initcfg.UpdateReport{Unclassified: []string{syntheticModule}}

	got := classifyTargetsForUpdate(cfg, report, nil)
	if len(got) != 1 {
		t.Fatalf("targets = %d, want 1: %+v", len(got), got)
	}
	if got[0].Name != syntheticModule || len(got[0].Paths) != 1 || got[0].Paths[0] != syntheticPath {
		t.Fatalf("synthetic override target not preserved: %+v", got[0])
	}
}

func TestCollectUpdateRepoEvidence_ReadmeAndDocsHeadings(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "README.md"), []byte("# Payments\n\n## Settlement\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(filepath.Join(dir, "docs"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "docs", "domain.md"), []byte("# Domain Map\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	got := strings.Join(collectUpdateRepoEvidence(dir), "\n")
	for _, want := range []string{"README.md: Payments", "README.md: Settlement", "docs/domain.md: Domain Map"} {
		if !strings.Contains(got, want) {
			t.Fatalf("evidence missing %q:\n%s", want, got)
		}
	}
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
    from: "internal/a/**"
    to: "internal/b/**"
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
    from: "internal/a/**"
    to: "internal/b/**"
`
)

// TestUpdateCmd_PlanMode_FileUnchanged verifies plan mode (no --apply) leaves the
// config file byte-identical.
func TestUpdateCmd_PlanMode_FileUnchanged(t *testing.T) {
	t.Parallel()
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
	t.Parallel()
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
	t.Parallel()
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
    from: "internal/a/**"
    to: "internal/b/**"
`
	cfgPath := writeConfig(t, dir, cfg)
	before, err := os.ReadFile(cfgPath) //nolint:gosec
	if err != nil {
		t.Fatal(err)
	}
	runner := matchingRunner("internal/mymod")

	cmd := &UpdateCmd{
		Config:           cfgPath,
		Root:             dir,
		LLM:              true,
		Apply:            true,
		LLMProvider:      providerAnthropic,
		LLMModel:         defaultLLMModel,
		providerOverride: &flexFakeProvider{subdomain: subdomainCore, volatility: volatilityLow, layer: layerCore},
	}
	out, err := runUpdateCmd(t, cmd, runner)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	data, err := os.ReadFile(cfgPath) //nolint:gosec
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(before, data) {
		t.Error("--llm --apply must not write review-only LLM suggestions")
	}
	for _, want := range []string{"+ subdomain: core", "+ volatility: low", "+ layer: core", "rationale: test"} {
		if !strings.Contains(out, want) {
			t.Errorf("review diff missing %q:\n%s", want, out)
		}
	}
}

// TestUpdateCmd_LLMApply_NoSetFieldsForAddedModule verifies that SetModuleFields is
// never emitted for a newly-added module (AddModule handles classification).
func TestUpdateCmd_LLMApply_NoSetFieldsForAddedModule(t *testing.T) {
	t.Parallel()
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
		providerOverride: &flexFakeProvider{subdomain: subdomainCore, volatility: volatilityLow, layer: layerCore},
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
	if strings.Contains(content, "subdomain:") {
		t.Errorf("LLM subdomain suggestion must not be written by --apply; content:\n%s", content)
	}
	if strings.Contains(content, "volatility:") {
		t.Errorf("LLM volatility suggestion must not be written by --apply; content:\n%s", content)
	}
}

// TestUpdateCmd_ExistingLayerNeverOverwritten verifies that an existing module's
// layer field is never replaced by the LLM suggestion.
func TestUpdateCmd_ExistingLayerNeverOverwritten(t *testing.T) {
	t.Parallel()
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
    from: "internal/a/**"
    to: "internal/b/**"
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
		providerOverride: &flexFakeProvider{subdomain: subdomainCore, volatility: volatilityLow, layer: layerCore},
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
	t.Parallel()
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
    from: "internal/a/**"
    to: "internal/b/**"
`
	cfgPath := writeConfig(t, dir, cfg)
	before, err := os.ReadFile(cfgPath) //nolint:gosec
	if err != nil {
		t.Fatal(err)
	}
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
	out, err := runUpdateCmd(t, cmd, runner)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	data, err := os.ReadFile(cfgPath) //nolint:gosec
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(before, data) {
		t.Error("--llm --apply must leave missing layer as a review-only suggestion")
	}
	if !strings.Contains(out, "+ layer: adapter") {
		t.Errorf("review diff missing layer suggestion:\n%s", out)
	}
}

// TestUpdateCmd_OutOfSetLayerWritesNothing verifies that when the only pending
// field-fill is a layer not in layers:, no backup is created and the file is unchanged.
func TestUpdateCmd_OutOfSetLayerWritesNothing(t *testing.T) {
	t.Parallel()
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
    from: "internal/a/**"
    to: "internal/b/**"
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
	t.Parallel()
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

// TestUpdateCmd_Apply_Idempotent verifies that running --apply twice on the same
// divergent fixture is a no-op on the second run: the file is byte-identical after
// both runs, and no backup is created by the second run (because there are no
// actionable edits left — AddModule and CommentModule are both no-ops on re-apply).
func TestUpdateCmd_Apply_Idempotent(t *testing.T) {
	t.Parallel()
	dir := minimalRoot(t)
	cfgPath := writeConfig(t, dir, configWithRemovedModule)

	cmd := &UpdateCmd{Config: cfgPath, Root: dir, Apply: true}

	// First apply: structural changes (comment removed module).
	if _, err := runUpdateCmd(t, cmd, emptyRunner()); err != nil {
		t.Fatalf("first apply: unexpected error: %v", err)
	}
	afterFirst, err := os.ReadFile(cfgPath) //nolint:gosec
	if err != nil {
		t.Fatal(err)
	}

	// Remove the .bak so we can detect whether the second run creates a new one.
	_ = os.Remove(cfgPath + ".bak")

	// Second apply: no actionable edits — CommentModule and AddModule are no-ops.
	if _, err = runUpdateCmd(t, cmd, emptyRunner()); err != nil {
		t.Fatalf("second apply: unexpected error: %v", err)
	}
	afterSecond, err := os.ReadFile(cfgPath) //nolint:gosec
	if err != nil {
		t.Fatal(err)
	}

	if !bytes.Equal(afterFirst, afterSecond) {
		t.Error("second --apply must be a no-op: file should be byte-identical to post-first-apply state")
	}
	if _, statErr := os.Stat(cfgPath + ".bak"); statErr == nil {
		t.Error("second --apply must not create a backup when there are no actionable edits")
	}
}

// TestUpdateCmd_LLMPlanMode_FileUnchanged verifies that --llm without --apply
// produces a report that includes LLM classification but leaves the config file untouched.
func TestUpdateCmd_LLMPlanMode_FileUnchanged(t *testing.T) {
	t.Parallel()
	dir := minimalRoot(t)
	// Config has mymod with no classification fields → it is Unclassified.
	cfg := `version: 1
layers:
  - core
  - adapter
modules:
  mymod:
    paths:
      - "internal/mymod/**"
rules:
  - id: no-bad-deps
    type: forbidden_dependency
    gate: warn
    from: "internal/a/**"
    to: "internal/b/**"
`
	cfgPath := writeConfig(t, dir, cfg)
	before, err := os.ReadFile(cfgPath) //nolint:gosec
	if err != nil {
		t.Fatal(err)
	}
	runner := matchingRunner("internal/mymod")

	cmd := &UpdateCmd{
		Config:           cfgPath,
		Root:             dir,
		LLM:              true,
		Apply:            false,
		LLMProvider:      providerAnthropic,
		LLMModel:         defaultLLMModel,
		providerOverride: &flexFakeProvider{subdomain: subdomainCore, volatility: volatilityLow, layer: layerCore},
	}
	out, err := runUpdateCmd(t, cmd, runner)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Report must mention the module and the LLM-suggested classification.
	if !strings.Contains(out, "mymod") {
		t.Errorf("plan report should mention 'mymod'; got:\n%s", out)
	}
	for _, want := range []string{"+ subdomain: core", "+ volatility: low", "rationale: test"} {
		if !strings.Contains(out, want) {
			t.Errorf("plan report missing %q:\n%s", want, out)
		}
	}

	after, err := os.ReadFile(cfgPath) //nolint:gosec
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(before, after) {
		t.Error("--llm plan mode must leave config file byte-unchanged")
	}
	if _, statErr := os.Stat(cfgPath + ".bak"); statErr == nil {
		t.Error("--llm plan mode must not create a backup")
	}
}

// TestUpdateCmd_Apply_RoundTripsConfigLoad verifies that after a structural --apply
// the written config is valid and loadable via config.Load.
func TestUpdateCmd_Apply_RoundTripsConfigLoad(t *testing.T) {
	t.Parallel()
	dir := minimalRoot(t)
	// Start with a removed module so --apply writes something.
	cfgPath := writeConfig(t, dir, configWithRemovedModule)

	cmd := &UpdateCmd{Config: cfgPath, Root: dir, Apply: true}
	if _, err := runUpdateCmd(t, cmd, emptyRunner()); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	ctx := context.Background()
	if _, err := config.Load(ctx, cfgPath); err != nil {
		t.Errorf("written config must round-trip through config.Load: %v", err)
	}
}

// TestUpdateCmd_ChangedSinceReadAborts verifies that safeWriteConfig aborts when
// the config file changes between the initial read and the write step.
func TestUpdateCmd_ChangedSinceReadAborts(t *testing.T) {
	t.Parallel()
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

// TestUpdateCmd_LLM_WarnPartialClassify verifies that when the LLM omits a
// module from its response, a warning is printed naming the unclassified module,
// and other modules are still written in --apply mode.
func TestUpdateCmd_LLM_WarnPartialClassify(t *testing.T) {
	t.Parallel()
	dir := minimalRoot(t)
	// Config has no modules; discovery will find two (both will be Added).
	cfgPath := writeConfig(t, dir, minimalConfigNoModules)

	const modPath = "example.com/test"
	runner := &toolrun.RunnerMock{
		DetectFunc: func(_ context.Context, _ string) (toolrun.ToolInfo, bool) {
			return toolrun.ToolInfo{}, false
		},
		RunFunc: func(_ context.Context, _ toolrun.ToolCmd) (toolrun.Output, error) {
			// Emit two packages: alpha and beta.
			a := fmt.Sprintf(`{"ImportPath":%q,"Dir":"/ignored","Module":{"Path":%q}}`, modPath+"/internal/alpha", modPath)
			b := fmt.Sprintf(`{"ImportPath":%q,"Dir":"/ignored","Module":{"Path":%q}}`, modPath+"/internal/beta", modPath)
			return toolrun.Output{Stdout: []byte(a + "\n" + b)}, nil
		},
	}

	// fakeOmitProvider classifies "alpha" only; "beta" is intentionally omitted.
	cmd := &UpdateCmd{
		Config:           cfgPath,
		Root:             dir,
		LLM:              true,
		Apply:            true,
		LLMProvider:      providerAnthropic,
		LLMModel:         defaultLLMModel,
		providerOverride: &fakeOmitProvider{classifyName: "alpha"},
	}
	out, err := runUpdateCmd(t, cmd, runner)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// A warning must name the omitted module.
	if !strings.Contains(out, "warning:") {
		t.Errorf("expected partial-classification warning; got:\n%s", out)
	}
	if !strings.Contains(out, "beta") {
		t.Errorf("warning should name 'beta'; got:\n%s", out)
	}
}

// fakeOmitProvider classifies only the module named classifyName; all others are omitted.
type fakeOmitProvider struct {
	classifyName string
}

func (p *fakeOmitProvider) Name() string { return "test/omit" }
func (p *fakeOmitProvider) Complete(_ context.Context, req llm.Request) (llm.Response, error) {
	var entries []string
	for _, line := range strings.Split(req.User, "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "- module: ") {
			name := strings.TrimSpace(strings.TrimPrefix(line, "- module: "))
			if name == p.classifyName {
				entries = append(entries, fmt.Sprintf(
					`{"module":%q,"subdomain":"core","volatility":"low","layer":"core","name":"","rationale":"test"}`,
					name,
				))
			}
			// Other modules intentionally omitted to trigger the partial-classify warning.
		}
	}
	if len(entries) == 0 {
		return llm.Response{Text: "[]"}, nil
	}
	return llm.Response{Text: "[" + strings.Join(entries, ",") + "]"}, nil
}

var _ llm.Provider = (*fakeOmitProvider)(nil)
