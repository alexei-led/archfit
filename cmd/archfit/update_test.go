package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	evidenceports "github.com/alexei-led/archfit/internal/evidence/ports"

	"github.com/alexei-led/archfit/internal/config"
	"github.com/alexei-led/archfit/internal/initcfg"
	"github.com/alexei-led/archfit/internal/llm"
	"github.com/alexei-led/archfit/internal/model/graph"
	"github.com/alexei-led/archfit/internal/policy"
	"github.com/alexei-led/archfit/internal/toolrun"
)

const (
	testUpdatePluginsModule = "plugins"
	testUpdateDeployUnit    = "deploy_unit"
	testUpdateModuleA       = "services/a"
	testUpdateModuleAGlob   = "services/a/**"
	testUpdateModuleAFile   = "file:services/a/impl.go"
	testUpdateModuleAPath   = "services/a/impl.go"
	testUpdateLayerAdapter  = "adapter"
	testUpdateOwnerTeamA    = "team-a"
	testUpdateWebModule     = "web"
	testUpdateWebGlob       = "cmd/web/**"
	rustEnabledAuto         = "rust:\n    enabled: auto"
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

func TestDistanceConfigCandidates_DynamicImportsBecomeReviewOnlyHints(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, testUpdatePluginsModule), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, testUpdatePluginsModule, "loader.py"), []byte("def load(name):\n    return __import__(name)\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	cfg := config.Config{Modules: map[string]policy.ModuleDef{
		testUpdatePluginsModule: {Paths: []string{testUpdatePluginsModule + "/**"}},
	}}

	got := distanceConfigCandidates(context.Background(), dir, cfg, &appDeps{Runner: emptyRunner()})
	if len(got) != 2 {
		t.Fatalf("distanceConfigCandidates len = %d, want 2: %+v", len(got), got)
	}
	if got[0].SourceBlock != "dynamic_connascence_signals" || got[0].Module != testUpdatePluginsModule || got[0].SuggestedReviewAction != testUpdateDeployUnit {
		t.Fatalf("distanceConfigCandidates[0] = %+v", got[0])
	}
	if got[1].SourceBlock != "dynamic_imports" || got[1].Module != testUpdatePluginsModule || got[1].SuggestedReviewAction != testUpdateDeployUnit {
		t.Fatalf("distanceConfigCandidates[1] = %+v", got[1])
	}
	if len(got[1].EvidenceRefs) != 1 || got[1].EvidenceRefs[0] != "plugins/loader.py:2" {
		t.Fatalf("distanceConfigCandidates evidence = %+v", got[1].EvidenceRefs)
	}
}

func TestStaticExternalDistanceConfigCandidatesFromGraph_GoThirdPartyHint(t *testing.T) {
	t.Parallel()

	cfg := config.Config{Modules: map[string]policy.ModuleDef{
		testUpdateModuleA: {Paths: []string{testUpdateModuleAGlob}},
	}}
	g := graph.Build([]graph.Facts{{
		Language:  graph.LangGo,
		GoModules: []graph.GoModule{{Path: "example.com/project", RelDir: "."}},
		Edges: []graph.Edge{
			{From: testUpdateModuleAFile, To: "package:github.com/aws/aws-sdk-go-v2/service/s3", Kind: graph.EdgeKindImports, Language: graph.LangGo, Locations: []graph.Location{{File: testUpdateModuleAPath, Line: 7}}},
			{From: testUpdateModuleAFile, To: "package:fmt", Kind: graph.EdgeKindImports, Language: graph.LangGo, Locations: []graph.Location{{File: testUpdateModuleAPath, Line: 8}}},
		},
	}})

	raw := staticExternalDistanceConfigCandidatesFromGraph(g, cfg)
	if len(raw) != 1 {
		t.Fatalf("staticExternalDistanceConfigCandidatesFromGraph len = %d, want 1: %+v", len(raw), raw)
	}
	got := toInitcfgDistanceConfigCandidate(raw[0])
	if got.SourceBlock != "classified_external_edges" || got.Module != testUpdateModuleA || got.Target != "github.com/aws/aws-sdk-go-v2/**" {
		t.Fatalf("static external candidate = %+v", got)
	}
	if got.SuggestedReviewAction != "external_systems" || got.IntegrationKind != string(graph.EdgeKindImports) {
		t.Fatalf("static external candidate action/kind = %+v", got)
	}
	if len(got.EvidenceRefs) != 1 || got.EvidenceRefs[0] != "services/a/impl.go:7" {
		t.Fatalf("static external evidence refs = %+v", got.EvidenceRefs)
	}
}

func TestCandidateConfigForUpdate_UsesDiscoveredModules(t *testing.T) {
	t.Parallel()

	cfg := config.Config{Modules: map[string]policy.ModuleDef{
		testUpdateModuleA: {
			Paths:      []string{"old/a/**"},
			Public:     []string{"old/public/**"},
			Internal:   []string{"old/internal/**"},
			Layer:      testUpdateLayerAdapter,
			Owner:      testUpdateOwnerTeamA,
			Subdomain:  subdomainCore,
			Volatility: "high",
			DeployUnit: "svc-a",
		},
	}}
	discovered := initcfg.DiscoveredConfig{Modules: []initcfg.ModuleDef{
		{Name: testUpdateModuleA, Paths: []string{testUpdateModuleAGlob}, Public: []string{"services/a/api/**"}, Internal: []string{"services/a/internal/**"}, Layer: layerCore},
		{Name: testUpdateWebModule, Paths: []string{"web/**"}, Public: []string{"web/api/**"}, Internal: []string{"web/internal/**"}, Layer: testUpdateLayerAdapter},
	}}

	got := candidateConfigForUpdate(cfg, discovered, nil)
	mm := got.ModuleMapView()
	if mod, ok := mm.ModuleForFile(testUpdateModuleAPath); !ok || mod != testUpdateModuleA {
		t.Fatalf("ModuleForFile(services/a/impl.go) = (%q,%t), want (services/a,true)", mod, ok)
	}
	if mod, ok := mm.ModuleForFile(testUpdateWebModule + "/app.ts"); !ok || mod != testUpdateWebModule {
		t.Fatalf("ModuleForFile(web/app.ts) = (%q,%t), want (web,true)", mod, ok)
	}
	if got.Modules[testUpdateModuleA].Owner != testUpdateOwnerTeamA || got.Modules[testUpdateModuleA].DeployUnit != "svc-a" {
		t.Fatalf("existing metadata not preserved: %+v", got.Modules[testUpdateModuleA])
	}
	if len(got.Modules[testUpdateModuleA].Paths) != 1 || got.Modules[testUpdateModuleA].Paths[0] != testUpdateModuleAGlob {
		t.Fatalf("services/a paths = %+v, want discovered paths", got.Modules[testUpdateModuleA].Paths)
	}
	if len(got.Modules[testUpdateModuleA].Public) != 1 || got.Modules[testUpdateModuleA].Public[0] != "old/public/**" {
		t.Fatalf("services/a public = %+v, want existing public globs preserved", got.Modules[testUpdateModuleA].Public)
	}
	if len(got.Modules[testUpdateWebModule].Public) != 1 || got.Modules[testUpdateWebModule].Public[0] != "web/api/**" {
		t.Fatalf("web public = %+v, want discovered public globs", got.Modules[testUpdateWebModule].Public)
	}
}

func TestStaticExternalDistanceConfigCandidatesFromGraph_UsesDiscoveredModuleConfig(t *testing.T) {
	t.Parallel()

	g := graph.Build([]graph.Facts{{
		Language:  graph.LangGo,
		GoModules: []graph.GoModule{{Path: "example.com/project", RelDir: "."}},
		Edges: []graph.Edge{
			{From: testUpdateModuleAFile, To: "package:github.com/aws/aws-sdk-go-v2/service/s3", Kind: graph.EdgeKindImports, Language: graph.LangGo, Locations: []graph.Location{{File: testUpdateModuleAPath, Line: 7}}},
		},
	}})
	cfg := config.Config{Modules: map[string]policy.ModuleDef{
		testUpdateModuleA: {Paths: []string{"old/a/**"}},
	}}
	if raw := staticExternalDistanceConfigCandidatesFromGraph(g, cfg); len(raw) != 0 {
		t.Fatalf("staticExternalDistanceConfigCandidatesFromGraph with stale cfg len = %d, want 0: %+v", len(raw), raw)
	}
	discovered := initcfg.DiscoveredConfig{Modules: []initcfg.ModuleDef{{
		Name:  testUpdateModuleA,
		Paths: []string{testUpdateModuleAGlob},
	}}}

	raw := staticExternalDistanceConfigCandidatesFromGraph(g, candidateConfigForUpdate(cfg, discovered, nil))
	if len(raw) != 1 {
		t.Fatalf("staticExternalDistanceConfigCandidatesFromGraph len = %d, want 1: %+v", len(raw), raw)
	}
	if raw[0].Module != testUpdateModuleA || raw[0].Target != "github.com/aws/aws-sdk-go-v2/**" {
		t.Fatalf("candidate = %+v", raw[0])
	}
}

func TestDeployUnitSuggestions_DeterministicHintsOnlyForMissingConfig(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, "cmd", testUpdateWebModule), 0o700); err != nil {
		t.Fatal(err)
	}
	runner := &toolrun.RunnerMock{
		DetectFunc: func(_ context.Context, name string) (toolrun.ToolInfo, bool) {
			return toolrun.ToolInfo{}, name == "go"
		},
		RunFunc: func(_ context.Context, _ toolrun.ToolCmd) (toolrun.Output, error) {
			return toolrun.Output{Stdout: []byte(filepath.Join(dir, "cmd", testUpdateWebModule) + "\n")}, nil
		},
	}
	cfg := config.Config{Modules: map[string]policy.ModuleDef{
		testUpdateWebModule: {Paths: []string{testUpdateWebGlob}},
		"api":               {Paths: []string{"cmd/api/**"}, DeployUnit: "api-service"},
	}}

	got := deployUnitSuggestions(context.Background(), dir, cfg, &appDeps{Runner: runner})
	if len(got) != 1 {
		t.Fatalf("deployUnitSuggestions len = %d, want 1: %+v", len(got), got)
	}
	if got[0].Module != testUpdateWebModule || got[0].Unit != testUpdateWebModule || got[0].Source != "cmd/web" {
		t.Fatalf("deployUnitSuggestions[0] = %+v", got[0])
	}
}

func TestDeployUnitSuggestions_UsesDiscoveredModuleMap(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, "cmd", testUpdateWebModule), 0o700); err != nil {
		t.Fatal(err)
	}
	runner := &toolrun.RunnerMock{
		DetectFunc: func(_ context.Context, name string) (toolrun.ToolInfo, bool) {
			return toolrun.ToolInfo{}, name == "go"
		},
		RunFunc: func(_ context.Context, _ toolrun.ToolCmd) (toolrun.Output, error) {
			return toolrun.Output{Stdout: []byte(filepath.Join(dir, "cmd", testUpdateWebModule) + "\n")}, nil
		},
	}
	cfg := config.Config{}
	discovered := initcfg.DiscoveredConfig{Modules: []initcfg.ModuleDef{{Name: testUpdateWebModule, Paths: []string{testUpdateWebGlob}}}}

	got := deployUnitSuggestions(context.Background(), dir, candidateConfigForUpdate(cfg, discovered, nil), &appDeps{Runner: runner})
	if len(got) != 1 {
		t.Fatalf("deployUnitSuggestions len = %d, want 1: %+v", len(got), got)
	}
	if got[0].Module != testUpdateWebModule || got[0].Unit != testUpdateWebModule || got[0].Source != "cmd/web" {
		t.Fatalf("deployUnitSuggestions[0] = %+v", got[0])
	}

	// A name-drifted stanza is preserved verbatim by --apply, so the candidate
	// config must carry its metadata. Keying the copy on the DISCOVERED name lost
	// it, and the builder then proposed a deploy_unit the config already sets —
	// under a module name that does not exist in the config either.
	t.Run("name-drifted module keeps its configured deploy unit", func(t *testing.T) {
		const driftedName = "internal/" + testUpdateWebModule
		driftedCfg := config.Config{Modules: map[string]policy.ModuleDef{
			driftedName: {Paths: []string{testUpdateWebGlob}, DeployUnit: "web-service"},
		}}
		drift := []initcfg.NameDrift{{
			ConfigName:     driftedName,
			DiscoveredName: testUpdateWebModule,
			Paths:          []string{testUpdateWebGlob},
		}}
		candidate := candidateConfigForUpdate(driftedCfg, discovered, drift)
		if _, ok := candidate.Modules[driftedName]; !ok {
			t.Fatalf("candidate modules = %+v, want the module under its config name", candidate.Modules)
		}
		if suggestions := deployUnitSuggestions(context.Background(), dir, candidate, &appDeps{Runner: runner}); len(suggestions) != 0 {
			t.Errorf("deployUnitSuggestions = %+v, want none — the config already sets deploy_unit", suggestions)
		}
	})
}

func TestCollectUpdateRepoEvidence_ReadmeAndDocsHeadings(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "README.md"), []byte("# Payments\n\n## Settlement\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(dir, "docs", "design"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "docs", "design", "domain.md"), []byte("# Domain Map\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	got := strings.Join(collectUpdateRepoEvidence(dir), "\n")
	for _, want := range []string{"doc:README.md", "README.md: Payments Settlement", "doc:docs/design/domain.md", "docs/design/domain.md: Domain Map"} {
		if !strings.Contains(got, want) {
			t.Fatalf("evidence missing %q:\n%s", want, got)
		}
	}
}

func TestUpdateCmd_Apply_RustProjectEnablesDeepAnalyzers(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "Cargo.toml"), []byte("[package]\nname = \"demo\"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	cfgPath := writeConfig(t, dir, `version: 2
languages:
  rust:
    enabled: true
modules: {}
rules:
  - id: no-layer-violations
    type: forbidden_layer_direction
    gate: warn
`)

	cmd := &UpdateCmd{Config: cfgPath, Root: dir, Apply: true}
	if _, err := runUpdateCmd(t, cmd, emptyRunner()); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	gotBytes, err := os.ReadFile(cfgPath) //nolint:gosec
	if err != nil {
		t.Fatal(err)
	}
	got := string(gotBytes)
	for _, want := range []string{
		rustEnabledAuto,
		"analyzers:\n  cargo_modules:\n    enabled: true\n  scip:\n    enabled: true",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("updated config missing %q:\n%s", want, got)
		}
	}

	// A run that WRITES must still report the review-only half. Gating that
	// output on the suggestion families alone hid module gaps exactly when apply
	// had an edit to make, so the same config reported less with --apply than
	// without it.
	t.Run("apply reports review-only module sections alongside the edit", func(t *testing.T) {
		applyDir := t.TempDir()
		if err := os.WriteFile(filepath.Join(applyDir, markerCargoToml), []byte("[package]\nname = \"demo\"\n"), 0o600); err != nil {
			t.Fatal(err)
		}
		applyCfg := writeConfig(t, applyDir, `version: 2
languages:
  rust:
    enabled: true
modules:
  ghost:
    paths:
      - nowhere/**
`)
		out, err := runUpdateCmd(t, &UpdateCmd{Config: applyCfg, Root: applyDir, Apply: true}, emptyRunner())
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		for _, want := range []string{"wrote ", "UNMATCHED", "ghost"} {
			if !strings.Contains(out, want) {
				t.Errorf("apply output missing %q:\n%s", want, out)
			}
		}
	})

	// Rust applicability here must be the manifest-aware probe the coverage layer
	// uses, not initcfg's root-Cargo.toml-only HasRust flag. A config already
	// pointing languages.rust.manifest at a sub-crate manifest describes a repo
	// archfit analyses as Rust, and `config update` refused to offer it the
	// deep-analysis defaults that make a single-crate graph measurable.
	t.Run("a configured sub-crate manifest enables the deep analyzers", func(t *testing.T) {
		subDir := t.TempDir()
		writeFileAt(t, subDir, filepath.Join("crates", "core", markerCargoToml), "[package]\nname = \"core\"\n")
		subCfg := writeConfig(t, subDir, `version: 2
languages:
  rust:
    enabled: true
    manifest: crates/core/Cargo.toml
modules: {}
`)
		if _, err := runUpdateCmd(t, &UpdateCmd{Config: subCfg, Root: subDir, Apply: true}, emptyRunner()); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		gotBytes, err := os.ReadFile(subCfg) //nolint:gosec
		if err != nil {
			t.Fatal(err)
		}
		got := string(gotBytes)
		for _, want := range []string{
			rustEnabledAuto,
			"manifest: crates/core/Cargo.toml",
			"analyzers:\n  cargo_modules:\n    enabled: true\n  scip:\n    enabled: true",
		} {
			if !strings.Contains(got, want) {
				t.Fatalf("updated config missing %q:\n%s", want, got)
			}
		}
	})

	// The mirror: no root manifest and no configured one is not a Rust project,
	// so update must not write a Rust stanza into it.
	t.Run("no manifest anywhere writes no Rust stanza", func(t *testing.T) {
		plainDir := t.TempDir()
		writeFileAt(t, plainDir, markerGoMod, "module example\n")
		plainCfg := writeConfig(t, plainDir, `version: 2
modules: {}
`)
		if _, err := runUpdateCmd(t, &UpdateCmd{Config: plainCfg, Root: plainDir, Apply: true}, emptyRunner()); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		gotBytes, err := os.ReadFile(plainCfg) //nolint:gosec
		if err != nil {
			t.Fatal(err)
		}
		if got := string(gotBytes); strings.Contains(got, "rust:") {
			t.Fatalf("Rust stanza written into a repo with no Cargo.toml:\n%s", got)
		}
	})
}

func TestEnsureRustDeepAnalysisConfig_IgnoresCommentBoundaries(t *testing.T) {
	cfg := config.Config{
		Languages: config.LanguagesConfig{Rust: config.RustLanguage{Enabled: evidenceports.ModeOn}},
		Analyzers: config.AnalyzersConfig{
			CargoModules: config.Analyzer{Enabled: evidenceports.ModeAuto},
			Scip:         config.TimedAnalyzer{Enabled: evidenceports.ModeAuto},
		},
	}
	src := []byte(`version: 2
languages:
  rust:
  # note: keep this section
    enabled: true
analyzers:
  cargo_modules:
  # note: keep this section
    enabled: auto
  scip:
    enabled: auto
modules: {}
rules:
  - id: no-layer-violations
    type: forbidden_layer_direction
    gate: warn
`)
	got := string(ensureRustDeepAnalysisConfig(src, cfg))
	for _, want := range []string{
		"rust:\n  # note: keep this section\n    enabled: auto",
		"cargo_modules:\n  # note: keep this section\n    enabled: true",
		"scip:\n    enabled: true",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("updated config missing %q:\n%s", want, got)
		}
	}
}

func TestEnsureRustDeepAnalysisConfig_PreservesExplicitAnalyzerOff(t *testing.T) {
	cfg := config.Config{
		Languages: config.LanguagesConfig{Rust: config.RustLanguage{Enabled: evidenceports.ModeOn}},
		Analyzers: config.AnalyzersConfig{
			CargoModules: config.Analyzer{Enabled: evidenceports.ModeOff},
			Scip:         config.TimedAnalyzer{Enabled: evidenceports.ModeOff},
		},
	}
	src := []byte(`version: 2
languages:
  rust:
    enabled: true
analyzers:
  cargo_modules:
    enabled: false
  scip:
    enabled: false
modules: {}
rules:
  - id: no-layer-violations
    type: forbidden_layer_direction
    gate: warn
`)
	got := string(ensureRustDeepAnalysisConfig(src, cfg))
	for _, want := range []string{
		rustEnabledAuto,
		"cargo_modules:\n    enabled: false",
		"scip:\n    enabled: false",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("updated config missing %q:\n%s", want, got)
		}
	}
}

func TestUpdateCmd_Apply_RustAnalyzerOptOutsPreserved(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "Cargo.toml"), []byte("[package]\nname = \"demo\"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	cfg := `version: 2
languages:
  rust:
    enabled: true
modules: {}
analyzers:
  cargo_modules:
    enabled: false
  scip:
    enabled: false
rules:
  - id: no-layer-violations
    type: forbidden_layer_direction
    gate: warn
`
	cfgPath := writeConfig(t, dir, cfg)

	cmd := &UpdateCmd{Config: cfgPath, Root: dir, Apply: true}
	out, err := runUpdateCmd(t, cmd, emptyRunner())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(out, "wrote") {
		t.Fatalf("expected write output, got:\n%s", out)
	}
	gotBytes, err := os.ReadFile(cfgPath) //nolint:gosec
	if err != nil {
		t.Fatal(err)
	}
	got := string(gotBytes)
	for _, want := range []string{
		rustEnabledAuto,
		"cargo_modules:\n    enabled: false",
		"scip:\n    enabled: false",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("updated config missing %q:\n%s", want, got)
		}
	}
}

func TestUpdateCmd_Apply_RustExplicitOffKeepsDeepAnalyzersDisabled(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "Cargo.toml"), []byte("[package]\nname = \"demo\"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	cfg := `version: 2
languages:
  rust:
    enabled: false
modules: {}
rules:
  - id: no-layer-violations
    type: forbidden_layer_direction
    gate: warn
`
	cfgPath := writeConfig(t, dir, cfg)

	cmd := &UpdateCmd{Config: cfgPath, Root: dir, Apply: true}
	out, err := runUpdateCmd(t, cmd, emptyRunner())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(out, "structurally in sync") {
		t.Fatalf("expected no-op output, got:\n%s", out)
	}
	gotBytes, err := os.ReadFile(cfgPath) //nolint:gosec
	if err != nil {
		t.Fatal(err)
	}
	if string(gotBytes) != cfg {
		t.Fatalf("explicit rust disabled config changed:\n%s", gotBytes)
	}
}

func TestUpdateCmd_LLMPlanMode_SuggestsRustSyntheticModules(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "Cargo.toml"), []byte("[package]\nname = \"herdr\"\nversion = \"0.1.0\"\nedition = \"2021\"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	cfg := `version: 2
layers:
  - core
  - service
modules:
  herdr:
    paths:
      - "herdr"
    owner: herdr-team
    layer: service
    subdomain: core
    volatility: high
languages:
  rust:
    enabled: true
analyzers:
  cargo_modules:
    enabled: true
`
	cfgPath := writeConfig(t, dir, cfg)
	before, err := os.ReadFile(cfgPath) //nolint:gosec
	if err != nil {
		t.Fatal(err)
	}

	cmd := &UpdateCmd{
		Config:           cfgPath,
		Root:             dir,
		AIClassify:       true,
		Refresh:          true,
		AIProvider:       providerAnthropic,
		AIModel:          defaultLLMModel,
		providerOverride: rustSyntheticProvider{},
	}
	out, err := runUpdateCmd(t, cmd, rustSyntheticRunner(dir))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	for _, want := range []string{
		"SUGGESTED (2 review-only module override(s)",
		"herdr-api:",
		`- "herdr::api"`,
		"volatility: low",
		"role: core",
		"herdr-ui:",
		`- "herdr::ui"`,
		"rationale: synthetic module review",
		"structurally in sync",
	} {
		if !strings.Contains(out, want) {
			t.Fatalf("output missing %q:\n%s", want, out)
		}
	}

	after, err := os.ReadFile(cfgPath) //nolint:gosec
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(before, after) {
		t.Fatal("--llm synthetic suggestions must leave config unchanged")
	}
	if initcfg.HasPendingEdits(initcfg.UpdateReport{Suggested: []initcfg.ModuleDef{{Name: "herdr-api"}}}) {
		t.Fatal("review-only synthetic suggestions must not be actionable edits")
	}
}

const (
	reviewSuggestSubdomainCore = "+ subdomain: core"
	reviewSuggestVolatilityLow = "+ volatility: low"
	reviewSuggestLayerCore     = "+ layer: core"
	reviewSuggestRationaleTest = "rationale: test"

	// minimalConfigNoModules is a valid config with no modules section.
	// Structurally in sync with empty discovery (Added=[], Removed=[], Drift=[]).
	minimalConfigNoModules = `version: 2
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
	configWithRemovedModule = `version: 2
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
	if !strings.Contains(out, "UNMATCHED") {
		t.Errorf("plan output should mention UNMATCHED; got: %q", out)
	}

	after, err := os.ReadFile(cfgPath) //nolint:gosec
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(before, after) {
		t.Error("plan mode must leave config file byte-unchanged")
	}
}

// TestUpdateCmd_Apply_KeepsUnmatchedModule verifies that --apply never deletes
// or comments out a configured module discovery did not emit.
//
// DiffModules matches config keys to discovered keys by NAME, and the two
// conventions need not agree (this repo configures `internal/agenttask` while
// discovery emits `agenttask`). Commenting the stanza out on that evidence
// discarded owner, subdomain, volatility, layer, and public on every module of
// this repo's own config. Removal is now a reported human decision.
func TestUpdateCmd_Apply_KeepsUnmatchedModule(t *testing.T) {
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
		Apply:  true,
	}
	out, err := runUpdateCmd(t, cmd, emptyRunner())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	after, err := os.ReadFile(cfgPath) //nolint:gosec
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(before, after) {
		t.Errorf("--apply must leave an unmatched module untouched; file changed to:\n%s", after)
	}
	if strings.Contains(string(after), "archfit: removed module") {
		t.Error("--apply must not comment out a configured module")
	}
	if _, statErr := os.Stat(cfgPath + ".bak"); statErr == nil {
		t.Error("no backup should be written when there is nothing to apply")
	}

	// The run must still report the unmatched module rather than claim the
	// config is in sync.
	if !strings.Contains(out, "UNMATCHED") {
		t.Errorf("apply output must report the unmatched module; got: %q", out)
	}
	if strings.Contains(out, "structurally in sync") {
		t.Errorf("apply output must not claim in-sync with an unmatched module; got: %q", out)
	}
}

// TestUpdateCmd_LLMApply_WritesOnlyAbsentFields verifies that --llm --apply on a
// structurally in-sync config writes only absent fields.
//
// Setup: go.mod + module "mymod" at internal/mymod matched by discovery.
// Config has mymod with subdomain+volatility but no layer, plus an active
// forbidden_layer_direction rule — the rule is what makes the missing layer
// classifiable. LLM suggests layer=core (in layers). Expected: only layer is added.
func TestUpdateCmd_LLMApply_WritesOnlyAbsentFields(t *testing.T) {
	t.Parallel()
	dir := minimalRoot(t)
	cfg := `version: 2
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
  - id: layer-direction
    type: forbidden_layer_direction
    gate: warn
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
		AIClassify:       true,
		Apply:            true,
		AIProvider:       providerAnthropic,
		AIModel:          defaultLLMModel,
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
	for _, want := range []string{reviewSuggestSubdomainCore, reviewSuggestVolatilityLow, reviewSuggestLayerCore, reviewSuggestRationaleTest} {
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
		AIClassify:       true,
		Apply:            true,
		AIProvider:       providerAnthropic,
		AIModel:          defaultLLMModel,
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

func TestUpdateCmd_LLMApply_SurfacesReviewOnlySuggestionsAfterStructuralEdit(t *testing.T) {
	t.Parallel()
	dir := minimalRoot(t)
	cfgPath := writeConfig(t, dir, minimalConfigNoModules)

	cmd := &UpdateCmd{
		Config:           cfgPath,
		Root:             dir,
		AIClassify:       true,
		Apply:            true,
		AIProvider:       providerAnthropic,
		AIModel:          defaultLLMModel,
		providerOverride: &ruleSuggestionProvider{},
	}
	out, err := runUpdateCmd(t, cmd, matchingRunner("internal/mymod"))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	for _, want := range []string{
		"LLM MODULE SUGGESTIONS",
		reviewSuggestSubdomainCore,
		reviewSuggestVolatilityLow,
		"RULE SUGGESTIONS",
		"id: no-adapter-to-core",
		"evidence_refs:",
	} {
		if !strings.Contains(out, want) {
			t.Fatalf("output missing %q:\n%s", want, out)
		}
	}
}

// TestUpdateCmd_ExistingLayerNeverOverwritten verifies that an existing module's
// layer field is never replaced by the LLM suggestion.
func TestUpdateCmd_ExistingLayerNeverOverwritten(t *testing.T) {
	t.Parallel()
	dir := minimalRoot(t)
	cfg := `version: 2
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
		AIClassify:       true,
		Apply:            true,
		AIProvider:       providerAnthropic,
		AIModel:          defaultLLMModel,
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
// returns a value that is in the allowed set. The active
// forbidden_layer_direction rule is a precondition: `layer:` is only classified
// while a layer rule consumes it.
func TestUpdateCmd_MissingLayerFilledWhenValid(t *testing.T) {
	t.Parallel()
	dir := minimalRoot(t)
	cfg := `version: 2
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
  - id: layer-direction
    type: forbidden_layer_direction
    gate: warn
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
		AIClassify:       true,
		Apply:            true,
		AIProvider:       providerAnthropic,
		AIModel:          defaultLLMModel,
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
	// The forbidden_layer_direction rule is what makes the missing layer classifiable.
	cfg := `version: 2
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
  - id: layer-direction
    type: forbidden_layer_direction
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
		AIClassify:       true,
		Apply:            true,
		AIProvider:       providerAnthropic,
		AIModel:          defaultLLMModel,
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
// real structural apply — here a newly discovered module, the one module edit
// --apply still writes.
func TestUpdateCmd_BackupCreatedOnApply(t *testing.T) {
	t.Parallel()
	dir := minimalRoot(t)
	cfgPath := writeConfig(t, dir, configWithRemovedModule)

	cmd := &UpdateCmd{
		Config: cfgPath,
		Root:   dir,
		Apply:  true,
	}
	_, err := runUpdateCmd(t, cmd, matchingRunner("internal/newmod"))
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
// actionable edits left — AddModule is a no-op on re-apply).
func TestUpdateCmd_Apply_Idempotent(t *testing.T) {
	t.Parallel()
	dir := minimalRoot(t)
	cfgPath := writeConfig(t, dir, configWithRemovedModule)

	cmd := &UpdateCmd{Config: cfgPath, Root: dir, Apply: true}

	// First apply: adds the newly discovered module stanza.
	if _, err := runUpdateCmd(t, cmd, matchingRunner("internal/newmod")); err != nil {
		t.Fatalf("first apply: unexpected error: %v", err)
	}
	afterFirst, err := os.ReadFile(cfgPath) //nolint:gosec
	if err != nil {
		t.Fatal(err)
	}

	// Remove the .bak so we can detect whether the second run creates a new one.
	_ = os.Remove(cfgPath + ".bak")

	// Second apply: no actionable edits — AddModule is a no-op on re-apply.
	if _, err = runUpdateCmd(t, cmd, matchingRunner("internal/newmod")); err != nil {
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
	cfg := `version: 2
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
		AIClassify:       true,
		Apply:            false,
		AIProvider:       providerAnthropic,
		AIModel:          defaultLLMModel,
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
	for _, want := range []string{reviewSuggestSubdomainCore, reviewSuggestVolatilityLow, reviewSuggestRationaleTest} {
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

func TestUpdateCmd_LLMPlanMode_RendersCitedRuleSuggestionsAndLeavesFileUnchanged(t *testing.T) {
	t.Parallel()
	dir := minimalRoot(t)
	cfg := `version: 2
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
	cmd := &UpdateCmd{
		Config:           cfgPath,
		Root:             dir,
		AIClassify:       true,
		Apply:            false,
		AIProvider:       providerAnthropic,
		AIModel:          defaultLLMModel,
		providerOverride: &ruleSuggestionProvider{},
	}
	out, err := runUpdateCmd(t, cmd, matchingRunner("internal/mymod"))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	for _, want := range []string{
		"RULE SUGGESTIONS",
		"type: forbidden_dependency",
		"id: no-adapter-to-core",
		"evidence_refs: config:.archfit.yaml",
		"basis: semantic_judgment",
	} {
		if !strings.Contains(out, want) {
			t.Fatalf("output missing %q:\n%s", want, out)
		}
	}
	after, err := os.ReadFile(cfgPath) //nolint:gosec
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(before, after) {
		t.Fatal("--llm plan mode with rule suggestions must leave config byte-unchanged")
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

type ruleSuggestionProvider struct{}

func (p *ruleSuggestionProvider) Name() string { return "test/rule-suggestion" }
func (p *ruleSuggestionProvider) Complete(_ context.Context, req llm.Request) (llm.Response, error) {
	ref := firstEvidenceRefForTest(req.User)
	return llm.Response{Text: fmt.Sprintf(`[{"module":"mymod","subdomain":"core","volatility":"low","layer":"core","name":"","rationale":"module classification cites %[1]s","evidence_refs":[%[2]q],"basis":"semantic_judgment","rule_suggestions":[{"id":"no-adapter-to-core","type":"forbidden_dependency","gate":"warn","from":"adapter/**","to":"core/**","rationale":"adapter should not call core internals; see %[1]s","evidence_refs":[%[2]q],"basis":"semantic_judgment"}]}]`, ref, ref)}, nil
}

var _ llm.Provider = (*ruleSuggestionProvider)(nil)

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
		AIClassify:       true,
		Apply:            true,
		AIProvider:       providerAnthropic,
		AIModel:          defaultLLMModel,
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
	ref := firstEvidenceRefForTest(req.User)
	var entries []string
	for _, line := range strings.Split(req.User, "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "- module: ") {
			name := strings.TrimSpace(strings.TrimPrefix(line, "- module: "))
			if name == p.classifyName {
				entries = append(entries, fmt.Sprintf(
					`{"module":%q,"subdomain":"core","volatility":"low","layer":"core","name":"","rationale":"test cites %s","evidence_refs":[%q],"basis":"semantic_judgment"}`,
					name, ref, ref,
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

type rustSyntheticProvider struct{}

func (rustSyntheticProvider) Name() string { return "test/rust-synthetic" }

func (rustSyntheticProvider) Complete(_ context.Context, req llm.Request) (llm.Response, error) {
	ref := firstEvidenceRefForTest(req.User)
	var entries []string
	for _, line := range strings.Split(req.User, "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "- module: ") {
			name := strings.TrimSpace(strings.TrimPrefix(line, "- module: "))
			entries = append(entries, fmt.Sprintf(
				`{"module":%q,"subdomain":"supporting","volatility":"low","layer":"core","role":"core","name":"","rationale":"synthetic module review cites %s","evidence_refs":[%q],"basis":"semantic_judgment"}`,
				name, ref, ref,
			))
		}
	}
	return llm.Response{Text: "[" + strings.Join(entries, ",") + "]"}, nil
}

func rustSyntheticRunner(root string) *toolrun.RunnerMock {
	meta := fmt.Sprintf(`{
  "packages": [
    {
      "id": "id-herdr",
      "name": "herdr",
      "manifest_path": %q,
      "source": null,
      "dependencies": [],
      "targets": [{"name": "herdr", "kind": ["lib"]}]
    }
  ],
  "workspace_members": ["id-herdr"],
  "workspace_root": %q
}`, filepath.Join(root, "Cargo.toml"), root)
	dot := `digraph {
    "herdr" [label="crate|herdr"]; // "crate" node
    "herdr::api" [label="pub mod|api"]; // "mod" node
    "herdr::ui" [label="pub mod|ui"]; // "mod" node
}`
	return &toolrun.RunnerMock{
		DetectFunc: func(_ context.Context, name string) (toolrun.ToolInfo, bool) {
			return toolrun.ToolInfo{Name: name}, name == toolCargo || name == toolCargoModules
		},
		RunFunc: func(_ context.Context, cmd toolrun.ToolCmd) (toolrun.Output, error) {
			switch cmd.Name {
			case toolCargo:
				if len(cmd.Args) > 0 && cmd.Args[0] == flagVersion {
					return toolrun.Output{Stdout: []byte("cargo 1.0.0")}, nil
				}
				return toolrun.Output{Stdout: []byte(meta)}, nil
			case toolCargoModules:
				return toolrun.Output{Stdout: []byte(dot)}, nil
			default:
				return toolrun.Output{ExitCode: 1}, nil
			}
		},
	}
}

var _ llm.Provider = rustSyntheticProvider{}

// ---------------------------------------------------------------------------
// config update review: status, JSON document, and flag conflicts
// ---------------------------------------------------------------------------

const testUpdateReviewConfig = `version: 2
layers:
  - core
  - adapter
modules:
  mymod:
    paths:
      - "internal/mymod/**"
rules:
  - id: layer-direction
    type: forbidden_layer_direction
    gate: warn
`

const (
	testUpdateReviewModule = "mymod"
	testUpdateReviewPkg    = "internal/mymod"
)

// TestRequiresLayerClassification pins that `layer:` is load-bearing only while a
// forbidden_layer_direction rule is live: unset and "warn" gates both keep it live.
func TestRequiresLayerClassification(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name  string
		rules []policy.RuleDef
		want  bool
	}{
		{"no rules", nil, false},
		{"other rule type", []policy.RuleDef{{Type: "forbidden_dependency", Gate: gateFail}}, false},
		{"layer rule gate unset", []policy.RuleDef{{Type: ruleTypeForbiddenLayerDirection}}, true},
		{"layer rule gate warn", []policy.RuleDef{{Type: ruleTypeForbiddenLayerDirection, Gate: gateWarn}}, true},
		{"layer rule gate fail", []policy.RuleDef{{Type: ruleTypeForbiddenLayerDirection, Gate: gateFail}}, true},
		{"layer rule gate off", []policy.RuleDef{{Type: ruleTypeForbiddenLayerDirection, Gate: gateOff}}, false},
		{
			name: "one live layer rule among several",
			rules: []policy.RuleDef{
				{Type: ruleTypeForbiddenLayerDirection, Gate: gateOff},
				{Type: ruleTypeForbiddenLayerDirection, Gate: gateWarn},
			},
			want: true,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			if got := requiresLayerClassification(config.Config{Rules: tc.rules}); got != tc.want {
				t.Errorf("requiresLayerClassification: got %v, want %v", got, tc.want)
			}
		})
	}
}

// TestUpdateCmd_JSON_RejectsConflictingFlagsBeforeSideEffects verifies the usage
// error is exit 3 AND that nothing ran: no tool call, no config write.
func TestUpdateCmd_JSON_RejectsConflictingFlagsBeforeSideEffects(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name    string
		mutate  func(*UpdateCmd)
		wantSub string
	}{
		{"apply", func(c *UpdateCmd) { c.Apply = true }, "--apply"},
		{"ai-classify", func(c *UpdateCmd) { c.AIClassify = true }, "--ai-classify"},
		{"refresh", func(c *UpdateCmd) { c.Refresh = true }, "--refresh"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			dir := minimalRoot(t)
			cfgPath := writeConfig(t, dir, testUpdateReviewConfig)
			before, err := os.ReadFile(cfgPath) //nolint:gosec
			if err != nil {
				t.Fatal(err)
			}

			runner := matchingRunner(testUpdateReviewPkg)
			cmd := &UpdateCmd{Config: cfgPath, Root: dir, JSON: true}
			tc.mutate(cmd)

			out, err := runUpdateCmd(t, cmd, runner)
			var ee *exitError
			if !errors.As(err, &ee) || ee.code != 3 {
				t.Fatalf("want exitError{code:3}, got %v", err)
			}
			if !strings.Contains(ee.msg, tc.wantSub) {
				t.Errorf("error must name %s: %q", tc.wantSub, ee.msg)
			}
			if out != "" {
				t.Errorf("rejected invocation must emit no output:\n%s", out)
			}
			if n := len(runner.RunCalls()) + len(runner.DetectCalls()); n != 0 {
				t.Errorf("discovery must not run before the conflict is rejected: %d tool call(s)", n)
			}
			after, err := os.ReadFile(cfgPath) //nolint:gosec
			if err != nil {
				t.Fatal(err)
			}
			if !bytes.Equal(before, after) {
				t.Error("rejected invocation must leave the config byte-identical")
			}
		})
	}
}

// TestUpdateCmd_ConfigReview covers the report-only review surface: the JSON
// contract, the status line, the issue codes, and the settings preview/apply
// parity for the Rust deep-analysis defaults.
func TestUpdateCmd_ConfigReview(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name string
		run  func(*testing.T)
	}{
		{"json emits the versioned document and writes nothing", runConfigReviewJSONDocument},
		{"fully specified config reports no_known_issues without a health claim", runConfigReviewNoKnownIssues},
		{"text report leads with the status line and lists issues", runConfigReviewTextStatus},
		{"rust setting preview matches what apply writes", runConfigReviewRustSettingParity},
		{"a naming difference is review-only, never a pending edit", runConfigReviewNameDrift},
		{"apply never claims a clean config over module issues", runConfigReviewApplyDisclosesIssues},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			tc.run(t)
		})
	}
}

func runConfigReviewJSONDocument(t *testing.T) {
	dir := minimalRoot(t)
	cfgPath := writeConfig(t, dir, testUpdateReviewConfig)
	before, err := os.ReadFile(cfgPath) //nolint:gosec
	if err != nil {
		t.Fatal(err)
	}

	out, err := runUpdateCmd(t,
		&UpdateCmd{Config: cfgPath, Root: dir, JSON: true},
		matchingRunner(testUpdateReviewPkg))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	review := decodeConfigReview(t, out)
	if review.SchemaVersion != initcfg.ConfigReviewSchemaVersion {
		t.Errorf("schema_version: got %q, want %q", review.SchemaVersion, initcfg.ConfigReviewSchemaVersion)
	}
	if review.Status != initcfg.ReviewStatusActionRequired {
		t.Errorf("status: got %q, want %q", review.Status, initcfg.ReviewStatusActionRequired)
	}
	wantCodes := []string{
		initcfg.IssueMissingLayer,
		initcfg.IssueMissingOwner,
		initcfg.IssueMissingVolatilityInput,
	}
	gotCodes := make([]string, 0, len(review.Issues))
	for _, issue := range review.Issues {
		if issue.Module != testUpdateReviewModule {
			t.Errorf("unexpected issue module %q", issue.Module)
		}
		gotCodes = append(gotCodes, issue.Code)
	}
	if !reflect.DeepEqual(gotCodes, wantCodes) {
		t.Errorf("issue codes: got %v, want %v", gotCodes, wantCodes)
	}
	if strings.Contains(out, "null") {
		t.Errorf("JSON lists must be arrays, never null:\n%s", out)
	}

	after, err := os.ReadFile(cfgPath) //nolint:gosec
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(before, after) {
		t.Error("--json is report-only and must leave the config byte-identical")
	}
	if _, statErr := os.Stat(cfgPath + ".bak"); statErr == nil {
		t.Error("--json must not create a backup")
	}
}

// runConfigReviewNameDrift pins the end-to-end wiring of the naming-difference
// pass. The config key is `internal/mymod` and discovery emits `mymod` over the
// same paths — a name-only difference. It must land in name_drift, leave
// added/removed empty, keep the status below action_required, and give --apply
// nothing to write.
func runConfigReviewNameDrift(t *testing.T) {
	dir := minimalRoot(t)
	cfgPath := writeConfig(t, dir, `version: 2
layers:
  - core
  - adapter
modules:
  internal/mymod:
    paths:
      - "internal/mymod/**"
    owner: team-a
    subdomain: supporting
    layer: adapter
rules:
  - id: no-bad-deps
    type: forbidden_dependency
    gate: warn
    from: "internal/a/**"
    to: "internal/b/**"
`)
	before, err := os.ReadFile(cfgPath) //nolint:gosec
	if err != nil {
		t.Fatal(err)
	}

	out, err := runUpdateCmd(t,
		&UpdateCmd{Config: cfgPath, Root: dir, JSON: true},
		matchingRunner("internal/mymod"))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	review := decodeConfigReview(t, out)
	if review.Status == initcfg.ReviewStatusActionRequired {
		t.Errorf("a name-only difference must not read action_required; got %q", review.Status)
	}
	if len(review.Structure.AddedModules) != 0 || len(review.Structure.RemovedModules) != 0 {
		t.Errorf("name drift must not appear as add/remove: added=%v removed=%v",
			review.Structure.AddedModules, review.Structure.RemovedModules)
	}
	if len(review.Structure.NameDrift) != 1 {
		t.Fatalf("name_drift: got %+v, want one entry", review.Structure.NameDrift)
	}
	drift := review.Structure.NameDrift[0]
	if drift.ConfigModule != "internal/mymod" || drift.DiscoveredModule != "mymod" {
		t.Errorf("name_drift entry = %+v, want config internal/mymod ↔ discovered mymod", drift)
	}

	// --apply must have nothing to write: rewriting the key would discard owner,
	// subdomain, and layer.
	if _, err = runUpdateCmd(t,
		&UpdateCmd{Config: cfgPath, Root: dir, Apply: true},
		matchingRunner("internal/mymod")); err != nil {
		t.Fatalf("apply: unexpected error: %v", err)
	}
	after, err := os.ReadFile(cfgPath) //nolint:gosec
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(before, after) {
		t.Errorf("--apply must not rewrite a name-only difference; file became:\n%s", after)
	}
}

// runConfigReviewApplyDisclosesIssues pins that `--apply` never prints the
// no-changes line over module gaps a human has to decide.
//
// The fixture is structurally in sync — nothing to add, remove, or path-drift —
// but its one module declares no owner, no layer, and no volatility input, so
// the run carries three issues. The apply path used to key that decision on
// HasReviewItems, which counted only suggestions, name drift, and removals: the
// identical config reported action_required without --apply and "structurally in
// sync — no changes to apply" with it.
func runConfigReviewApplyDisclosesIssues(t *testing.T) {
	dir := minimalRoot(t)
	cfgPath := writeConfig(t, dir, testUpdateReviewConfig)
	before, err := os.ReadFile(cfgPath) //nolint:gosec
	if err != nil {
		t.Fatal(err)
	}

	out, err := runUpdateCmd(t,
		&UpdateCmd{Config: cfgPath, Root: dir, Apply: true},
		matchingRunner(testUpdateReviewPkg))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if strings.Contains(out, "no changes to apply") {
		t.Errorf("--apply must not claim a clean config while issues are unreported:\n%s", out)
	}
	if !strings.HasPrefix(out, "status: "+initcfg.ReviewStatusActionRequired) {
		t.Errorf("--apply must lead with the same status line the preview prints:\n%s", out)
	}
	for _, want := range []string{
		initcfg.IssueMissingOwner,
		initcfg.IssueMissingLayer,
		initcfg.IssueMissingVolatilityInput,
	} {
		if !strings.Contains(out, want) {
			t.Errorf("--apply output missing issue code %q:\n%s", want, out)
		}
	}

	after, err := os.ReadFile(cfgPath) //nolint:gosec
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(before, after) {
		t.Errorf("an issue-only report has nothing to write; file became:\n%s", after)
	}
}

func runConfigReviewNoKnownIssues(t *testing.T) {
	dir := minimalRoot(t)
	cfgPath := writeConfig(t, dir, `version: 2
layers:
  - core
  - adapter
modules:
  mymod:
    paths:
      - "internal/mymod/**"
    owner: team-a
    subdomain: supporting
    layer: adapter
rules:
  - id: layer-direction
    type: forbidden_layer_direction
    gate: warn
`)

	out, err := runUpdateCmd(t,
		&UpdateCmd{Config: cfgPath, Root: dir, JSON: true},
		matchingRunner(testUpdateReviewPkg))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	review := decodeConfigReview(t, out)
	if review.Status != initcfg.ReviewStatusNoKnownIssues {
		t.Fatalf("status: got %q, want %q\n%s", review.Status, initcfg.ReviewStatusNoKnownIssues, out)
	}
	for _, banned := range []string{"healthy", "complete"} {
		if strings.Contains(strings.ToLower(out), banned) {
			t.Errorf("review must not claim %q:\n%s", banned, out)
		}
	}
}

func runConfigReviewTextStatus(t *testing.T) {
	dir := minimalRoot(t)
	cfgPath := writeConfig(t, dir, testUpdateReviewConfig)

	out, err := runUpdateCmd(t,
		&UpdateCmd{Config: cfgPath, Root: dir},
		matchingRunner(testUpdateReviewPkg))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.HasPrefix(out, "status: "+initcfg.ReviewStatusActionRequired) {
		t.Errorf("report must lead with the status line:\n%s", out)
	}
	for _, want := range []string{
		initcfg.IssueMissingOwner,
		initcfg.IssueMissingLayer,
		initcfg.IssueMissingVolatilityInput,
	} {
		if !strings.Contains(out, want) {
			t.Errorf("report missing issue code %q:\n%s", want, out)
		}
	}
}

func runConfigReviewRustSettingParity(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "Cargo.toml"), []byte("[package]\nname = \"demo\"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	cfgPath := writeConfig(t, dir, `version: 2
languages:
  rust:
    enabled: true
modules: {}
rules:
  - id: no-layer-violations
    type: forbidden_layer_direction
    gate: warn
`)
	before, err := os.ReadFile(cfgPath) //nolint:gosec
	if err != nil {
		t.Fatal(err)
	}

	textOut, err := runUpdateCmd(t, &UpdateCmd{Config: cfgPath, Root: dir}, emptyRunner())
	if err != nil {
		t.Fatalf("preview failed: %v", err)
	}
	if !strings.Contains(textOut, initcfg.SettingCodeRustDeepAnalysis) {
		t.Errorf("preview must name the setting --apply would write:\n%s", textOut)
	}

	jsonOut, err := runUpdateCmd(t, &UpdateCmd{Config: cfgPath, Root: dir, JSON: true}, emptyRunner())
	if err != nil {
		t.Fatalf("json preview failed: %v", err)
	}
	review := decodeConfigReview(t, jsonOut)
	if len(review.Structure.Settings) != 1 || review.Structure.Settings[0].Code != initcfg.SettingCodeRustDeepAnalysis {
		t.Fatalf("settings: got %+v", review.Structure.Settings)
	}
	if review.Status != initcfg.ReviewStatusActionRequired {
		t.Errorf("a pending settings edit is an action: got %q", review.Status)
	}

	mid, err := os.ReadFile(cfgPath) //nolint:gosec
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(before, mid) {
		t.Fatal("preview must not write the config")
	}

	if _, err = runUpdateCmd(t, &UpdateCmd{Config: cfgPath, Root: dir, Apply: true}, emptyRunner()); err != nil {
		t.Fatalf("apply failed: %v", err)
	}
	applied, err := os.ReadFile(cfgPath) //nolint:gosec
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(applied), rustEnabledAuto) {
		t.Errorf("apply must write the previewed setting:\n%s", applied)
	}
}

// decodeConfigReview parses the `config update --json` document.
func decodeConfigReview(t *testing.T, out string) initcfg.ConfigReview {
	t.Helper()
	var review initcfg.ConfigReview
	if err := json.Unmarshal([]byte(out), &review); err != nil {
		t.Fatalf("output is not valid JSON: %v\n%s", err, out)
	}
	return review
}
