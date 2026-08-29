package config_test

import (
	"context"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"testing"

	evidenceports "github.com/alexei-led/archfit/internal/evidence/ports"
	suppliedcoverage "github.com/alexei-led/archfit/internal/extract/coverage"

	"github.com/alexei-led/archfit/internal/config"
	"github.com/alexei-led/archfit/internal/model/pattern"
	"github.com/alexei-led/archfit/internal/policy"

	"github.com/goccy/go-yaml"
)

func TestLoad_Valid(t *testing.T) {
	cfg, err := config.Load(context.Background(), "testdata/valid.yaml")
	if err != nil {
		t.Fatalf("Load: %v", err)
	}

	if cfg.Version != config.SchemaVersion {
		t.Errorf("Version = %d, want %d", cfg.Version, config.SchemaVersion)
	}
	if len(cfg.Modules) != 2 {
		t.Errorf("len(Modules) = %d, want 2", len(cfg.Modules))
	}
	if len(cfg.Layers) != 3 {
		t.Errorf("len(Layers) = %d, want 3", len(cfg.Layers))
	}
	if len(cfg.Rules) != 2 {
		t.Errorf("len(Rules) = %d, want 2", len(cfg.Rules))
	}
	if len(cfg.Exclude) != 2 {
		t.Errorf("len(Exclude) = %d, want 2", len(cfg.Exclude))
	}
	if len(cfg.Waivers) != 1 {
		t.Errorf("len(Waivers) = %d, want 1", len(cfg.Waivers))
	}

	// Verify language/analyzer modes (true→on, auto stays auto).
	if got := cfg.Languages.Go.Enabled; got != evidenceports.ModeOn {
		t.Errorf("languages.go.enabled = %q, want %q", got, evidenceports.ModeOn)
	}
	if got := cfg.Languages.TypeScript.Enabled; got != evidenceports.ModeAuto {
		t.Errorf("languages.typescript.enabled = %q, want %q", got, evidenceports.ModeAuto)
	}
	if got := cfg.Analyzers.Clones.Enabled; got != evidenceports.ModeAuto {
		t.Errorf("analyzers.clones.enabled = %q, want %q", got, evidenceports.ModeAuto)
	}

	// Verify module details.
	checkout, ok := cfg.Modules["checkout"]
	if !ok {
		t.Error("module checkout not found")
	} else {
		if checkout.Owner != "team-checkout" {
			t.Errorf("checkout.Owner = %q, want %q", checkout.Owner, "team-checkout")
		}
		if checkout.DeployUnit != "web-api" {
			t.Errorf("checkout.DeployUnit = %q, want %q", checkout.DeployUnit, "web-api")
		}
	}

	// Verify outputs.
	if !cfg.Outputs.JSON {
		t.Error("outputs.json = false, want true")
	}
	if cfg.Outputs.Markdown {
		t.Error("outputs.markdown = true, want false")
	}
}

func TestLoad_UnknownField(t *testing.T) {
	_, err := config.Load(context.Background(), "testdata/unknown_field.yaml")
	if err == nil {
		t.Fatal("Load: expected error for unknown field, got nil")
	}
	if !strings.Contains(err.Error(), "unknown") && !strings.Contains(err.Error(), "field") {
		t.Errorf("error %q does not mention unknown field", err.Error())
	}
}

func TestLoad_FromLayerRejected(t *testing.T) {
	// from_layer/to_layer were removed from RuleDef: no rule ever read them, so
	// a config carrying them looked configured while the keys were inert. Strict
	// decoding must now reject them loudly. Do not re-add the fields.
	_, err := config.Load(context.Background(), "testdata/from_layer_rejected.yaml")
	if err == nil {
		t.Fatal("Load: expected error for from_layer/to_layer keys, got nil")
	}
}

func TestLoad_MissingVersion(t *testing.T) {
	_, err := config.Load(context.Background(), "testdata/missing_version.yaml")
	if err == nil {
		t.Fatal("Load: expected error for missing version, got nil")
	}
	if !strings.Contains(err.Error(), "version") {
		t.Errorf("error %q does not mention version", err.Error())
	}
}

func TestLoad_FileNotFound(t *testing.T) {
	_, err := config.Load(context.Background(), "testdata/nonexistent.yaml")
	if err == nil {
		t.Fatal("Load: expected error for missing file, got nil")
	}
}

func TestModuleFor(t *testing.T) {
	cfg, err := config.Load(context.Background(), "testdata/valid.yaml")
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	mm := cfg.ModuleMapView()

	tests := []struct {
		path   string
		want   string
		wantOK bool
	}{
		{"services/checkout/handler.go", "checkout", true},
		{"services/checkout/deep/nested/file.go", "checkout", true},
		{"services/pricing/contracts/api.go", "pricing", true},
		{"services/pricing/internal/impl.go", "pricing", true},
		{"cmd/main.go", "", false},
		{"", "", false},
	}

	for _, tc := range tests {
		t.Run(tc.path, func(t *testing.T) {
			got, ok := mm.ModuleFor(tc.path)
			if ok != tc.wantOK {
				t.Errorf("ModuleFor(%q): ok=%v, want %v", tc.path, ok, tc.wantOK)
			}
			if got != tc.want {
				t.Errorf("ModuleFor(%q): module=%q, want %q", tc.path, got, tc.want)
			}
		})
	}
}

// TestModuleFor_PythonDottedGlobs locks the Python module-resolution invariant:
// the grimp extractor emits DOTTED module node IDs ("prefect.states"), so Python
// module globs (paths:/public:/internal:, and rule from:/to:, which share this
// matcher) MUST be written in dotted form ("prefect.**") — NOT file-path form
// ("src/prefect/**"). Slash globs silently fail to match the dotted node IDs, so
// every Python edge classifies as external → 0 scored → coupling_balance n/a.
// This was the prefect eval miss: file-path globs that never bound.
func TestModuleFor_PythonDottedGlobs(t *testing.T) {
	const node = "prefect.states" // grimp-style dotted node ID (graph.NodePath form)
	const mod = "states"

	dottedCfg := config.Config{
		Version: 1,
		Modules: map[string]policy.ModuleDef{
			mod: {Paths: []string{"prefect.states", "prefect.states.**"}},
		},
	}
	dotted := dottedCfg.ModuleMapView()
	if got, ok := dotted.ModuleFor(node); !ok || got != mod {
		t.Errorf("dotted glob: ModuleFor(%q) = (%q,%v), want (%s,true)", node, got, ok, mod)
	}

	slashCfg := config.Config{
		Version: 1,
		Modules: map[string]policy.ModuleDef{
			mod: {Paths: []string{"src/prefect/states/**", "src/prefect/states.py"}},
		},
	}
	slash := slashCfg.ModuleMapView()
	if got, ok := slash.ModuleFor(node); ok {
		t.Errorf("slash glob: ModuleFor(%q) = (%q,true), want no-match — Python globs must be dotted module form, not file paths", node, got)
	}
}

// TestModuleFor_ConsumerConsistency guards Fix group 0 Task 0.2 / Fix group 4
// Task 4.1 (reproduces and closes A2): edge-based consumers
// (internal/classify/classify.go, via pathFromID) resolve a Python edge
// endpoint's DOTTED node ID through ModuleFor — that entry point is
// unchanged. File-path-based consumers (internal/ownership CODEOWNERS
// resolution, internal/assessment/rules public_api_* attribution, internal/engine
// clone-pairing) resolve the SAME underlying source file's real
// repo-relative path through ModuleForFile, which normalizes the file into
// the language's node-key form (dotted for Python) before delegating to
// ModuleFor. For a config to work end-to-end, both entry points must resolve
// to the same module. Previously they didn't: ModuleFor alone is one generic
// glob matcher shared by both key spaces, so a config declared in dotted form
// (the CLAUDE.md-mandated Python convention, mirroring
// testdata/fixture-py/.archfit.yaml) silently failed to match the real file
// path that file-based consumers look up.
func TestModuleFor_ConsumerConsistency(t *testing.T) {
	// Mirrors testdata/fixture-py/.archfit.yaml's module "b".
	cfg := config.Config{
		Version: 1,
		Modules: map[string]policy.ModuleDef{
			"b": {Paths: []string{"fixture_py.b", "fixture_py.b.**"}},
		},
	}
	mm := cfg.ModuleMapView()

	const (
		dottedNode = "fixture_py.b.mod"    // grimp-style edge endpoint node ID
		realPath   = "fixture_py/b/mod.py" // same file's real repo-relative path
	)

	edgeModule, ok := mm.ModuleFor(dottedNode)
	if !ok {
		t.Fatalf("edge-consumer lookup: ModuleFor(%q) = (_, false), want module found", dottedNode)
	}

	fileModule, ok := mm.ModuleForFile(realPath)
	if !ok || fileModule != edgeModule {
		t.Errorf("file-consumer lookup: ModuleForFile(%q) = (%q, %v), want (%q, true) — "+
			"the same source file must resolve to the same module as its dotted edge "+
			"node ID %q did (%q)", realPath, fileModule, ok, edgeModule, dottedNode, edgeModule)
	}
}

func TestModuleMap_IsModuleRoot(t *testing.T) {
	cfg := config.Config{
		Version: 1,
		Modules: map[string]policy.ModuleDef{
			"promqltest": {Paths: []string{"promql/promqltest/**"}},
			"literal":    {Paths: []string{"cmd/tool"}}, // no wildcard: pattern is itself a literal path
		},
	}
	mm := cfg.ModuleMapView()

	tests := []struct {
		name string
		dir  string
		want bool
	}{
		{"module's own root", "promql/promqltest", true},
		{"nested subdirectory, not root", "promql/promqltest/cmd/migrate", false},
		{"literal (no-wildcard) pattern matches itself", "cmd/tool", true},
		{"unconfigured directory", "unrelated/dir", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := mm.IsModuleRoot(tt.dir); got != tt.want {
				t.Errorf("IsModuleRoot(%q) = %v, want %v", tt.dir, got, tt.want)
			}
		})
	}
}

// TestModuleRootDirs verifies the agent_tasks files[] last-resort fallback:
// every module with a Paths glob maps to its literal (wildcard-free) root
// dir, and modules with no Paths are absent — never a bare dotted/"::" id.
func TestModuleRootDirs(t *testing.T) {
	const (
		modDomain    = "domain"
		modLiteral   = "literal"
		modPyDotted  = "myapp.domain"
		modNoPaths   = "nopaths"
		literalPath  = "cmd/tool"
		pyDottedGlob = "myapp.domain.**"
	)
	modules := map[string]policy.ModuleDef{
		modDomain:   {Paths: []string{modDomain + "/**"}},
		modLiteral:  {Paths: []string{literalPath}},
		modPyDotted: {Paths: []string{pyDottedGlob}}, // Python dotted glob: no "/" wildcard prefix
		modNoPaths:  {},
	}
	got := policy.ModuleRootDirs(modules)

	want := map[string]string{
		modDomain:   modDomain,
		modLiteral:  literalPath,
		modPyDotted: "myapp.domain", // globRoot cuts at the first "*"; the trailing separator dot is trimmed so the resolver's dotted-module probe can turn it into a real path
	}
	if len(got) != len(want) {
		t.Fatalf("policy.ModuleRootDirs = %+v, want %+v", got, want)
	}
	for name, dir := range want {
		if got[name] != dir {
			t.Errorf("policy.ModuleRootDirs[%q] = %q, want %q", name, got[name], dir)
		}
	}
	if _, ok := got[modNoPaths]; ok {
		t.Error("policy.ModuleRootDirs should omit a module with no Paths")
	}
}

func TestModuleFor_Deterministic(t *testing.T) {
	// Two modules whose globs could overlap — same path must always return the
	// same (alphabetically-first) module name.
	cfg := config.Config{
		Version: 1,
		Modules: map[string]policy.ModuleDef{
			"beta":  {Paths: []string{"shared/**"}},
			"alpha": {Paths: []string{"shared/**"}},
		},
	}
	mm := cfg.ModuleMapView()

	const path = "shared/util.go"
	first, _ := mm.ModuleFor(path)
	for range 10 {
		got, _ := mm.ModuleFor(path)
		if got != first {
			t.Errorf("ModuleFor(%q) is non-deterministic: got %q then %q", path, first, got)
		}
	}
	// Alpha sorts before beta — alpha wins.
	if first != "alpha" {
		t.Errorf("ModuleFor(%q) = %q, want alpha (alphabetically first)", path, first)
	}
}

// TestModuleFor_MostSpecific guards the catch-all shadowing regression: a broad
// fallback glob (internal/**) must never shadow a specific module
// (internal/model/**), even though the catch-all sorts first alphabetically.
// Resolution is most-specific-match, so the specific stanza wins where it
// applies and the catch-all only absorbs paths nothing more specific matches.
func TestModuleFor_MostSpecific(t *testing.T) {
	const catchAll = "internal" // broad fallback stanza; repeated below
	cfg := config.Config{
		Version: 1,
		Modules: map[string]policy.ModuleDef{
			catchAll:          {Paths: []string{"internal/**"}},
			"internal/model":  {Paths: []string{"internal/model/**"}},
			"internal/engine": {Paths: []string{"internal/engine/**"}},
		},
	}
	mm := cfg.ModuleMapView()

	tests := []struct {
		path string
		want string
	}{
		{"internal/model/evidence/evidence.go", "internal/model"}, // specific beats catch-all
		{"internal/engine/run.go", "internal/engine"},             // specific beats catch-all
		{"internal/arch_test.go", catchAll},                       // only the catch-all matches
		{"internal/scope/scope.go", catchAll},                     // no specific stanza → catch-all
	}
	for _, tc := range tests {
		t.Run(tc.path, func(t *testing.T) {
			got, ok := mm.ModuleFor(tc.path)
			if !ok || got != tc.want {
				t.Errorf("ModuleFor(%q) = %q (ok=%v), want %q", tc.path, got, ok, tc.want)
			}
		})
	}
}

func TestForScope(t *testing.T) {
	cfg, err := config.Load(context.Background(), "testdata/valid.yaml")
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	sc := cfg.ForScope()
	if len(sc.Exclusions) != len(cfg.Exclude) {
		t.Errorf("ForScope().Exclusions len=%d, want %d", len(sc.Exclusions), len(cfg.Exclude))
	}
}

func TestForExtract(t *testing.T) {
	cfg, err := config.Load(context.Background(), "testdata/valid.yaml")
	if err != nil {
		t.Fatalf("Load: %v", err)
	}

	t.Run("typescript_mode_auto", func(t *testing.T) {
		ec := cfg.ForExtract("typescript")
		if ec.Mode != evidenceports.ModeAuto {
			t.Errorf("ForExtract(typescript).Mode = %q, want auto", ec.Mode)
		}
	})

	t.Run("go_mode_on", func(t *testing.T) {
		// testdata/valid.yaml sets languages.go.enabled: true → ModeOn
		ec := cfg.ForExtract("go")
		if ec.Mode != evidenceports.ModeOn {
			t.Errorf("ForExtract(go).Mode = %q, want on", ec.Mode)
		}
	})

	t.Run("paths_populated", func(t *testing.T) {
		ec := cfg.ForExtract("go")
		if len(ec.Paths) == 0 {
			t.Error("ForExtract(go).Paths is empty")
		}
	})

	t.Run("internal_globs_collected", func(t *testing.T) {
		ec := cfg.ForExtract("go")
		// pricing module has internal: ["services/pricing/internal/**"]
		if !slices.Contains(ec.Internal, "services/pricing/internal/**") {
			t.Errorf("ForExtract internal globs missing pricing internal glob; got %v", ec.Internal)
		}
	})

	t.Run("rust_mode_default_auto", func(t *testing.T) {
		// rust tool not in testdata/valid.yaml → defaults to auto
		ec := cfg.ForExtract("rust")
		if ec.Mode != evidenceports.ModeAuto {
			t.Errorf("ForExtract(rust).Mode = %q, want auto", ec.Mode)
		}
	})

	t.Run("rust_fields_populated", func(t *testing.T) {
		rc := config.Config{
			Version: 1,
			Languages: config.LanguagesConfig{Rust: config.RustLanguage{
				Manifest:       rustManifestPath,
				Features:       rustFeatures,
				IncludeDevDeps: true,
			}},
		}
		ec := rc.ForExtract("rust")
		if ec.CargoManifest != rustManifestPath {
			t.Errorf("CargoManifest = %q, want %q", ec.CargoManifest, rustManifestPath)
		}
		if !slices.Equal(ec.CargoFeatures, rustFeatures) {
			t.Errorf("CargoFeatures = %v, want %v", ec.CargoFeatures, rustFeatures)
		}
		if !ec.IncludeDevDeps {
			t.Error("IncludeDevDeps = false, want true")
		}
	})

	t.Run("rust_fields_absent_for_non_rust", func(t *testing.T) {
		// A non-rust language must not pick up the Cargo fields.
		rc := config.Config{Version: 1, Languages: config.LanguagesConfig{Rust: config.RustLanguage{Manifest: "Cargo.toml", IncludeDevDeps: true}}}
		ec := rc.ForExtract("go")
		if ec.CargoManifest != "" || ec.CargoFeatures != nil || ec.IncludeDevDeps {
			t.Errorf("non-rust lang leaked rust fields: %+v", ec)
		}
	})

	t.Run("go_module_filter_propagated", func(t *testing.T) {
		// tools.go.modules.include/exclude must surface in ForExtract("go").
		gc := config.Config{
			Version: 1,
			Languages: config.LanguagesConfig{Go: config.GoLanguage{
				Enabled: evidenceports.ModeAuto,
				Modules: config.GoModuleFilter{
					Include: []string{globSvcAll},
					Exclude: []string{"svc/legacy"},
				},
			}},
		}
		ec := gc.ForExtract("go")
		if !slices.Equal(ec.GoModuleInclude, []string{globSvcAll}) {
			t.Errorf("GoModuleInclude = %v, want [svc/**]", ec.GoModuleInclude)
		}
		if !slices.Equal(ec.GoModuleExclude, []string{"svc/legacy"}) {
			t.Errorf("GoModuleExclude = %v, want [svc/legacy]", ec.GoModuleExclude)
		}
	})

	t.Run("go_module_filter_absent_for_non_go", func(t *testing.T) {
		// tools.go.modules must not leak into other language extractors.
		gc := config.Config{
			Version: 1,
			Languages: config.LanguagesConfig{Go: config.GoLanguage{
				Modules: config.GoModuleFilter{Include: []string{globSvcAll}},
			}},
		}
		ec := gc.ForExtract("typescript")
		if ec.GoModuleInclude != nil || ec.GoModuleExclude != nil {
			t.Errorf("go module filter leaked into typescript extractor: %+v", ec)
		}
	})
}

// TestForExtract_SrcNotModuleDerived guards against a regression where
// ForExtract("typescript").Src was derived from the alphabetically-first
// module's Paths[0] — a classification glob (e.g. "addons/**"), not a real
// filesystem directory. Under --root subtree rewriting that nonsense path was
// re-prefixed and handed to dependency-cruiser, which found no files and
// aborted with a fatal TS18003 (docs/plans/completed/20260701-multilang-reliability-fixes.md
// Task 4.3). Two configs below declare the same two modules with different
// alphabetically-first names; Src must be identical across both and must
// never equal a module's Paths[0].
func TestForExtract_SrcNotModuleDerived(t *testing.T) {
	const webGlob = "packages/web/**"

	cfgAddonsFirst := config.Config{
		Version: 1,
		Modules: map[string]policy.ModuleDef{
			"addons": {Paths: []string{"addons/**"}},
			"web":    {Paths: []string{webGlob}},
		},
	}
	cfgWebFirst := config.Config{
		Version: 1,
		Modules: map[string]policy.ModuleDef{
			"web": {Paths: []string{webGlob}},
			"zzz": {Paths: []string{"zzz/**"}},
		},
	}

	srcAddonsFirst := cfgAddonsFirst.ForExtract("typescript").Src
	srcWebFirst := cfgWebFirst.ForExtract("typescript").Src

	if srcAddonsFirst != srcWebFirst {
		t.Errorf("Src depends on module glob order: %q (addons-first config) != %q (web-first config)", srcAddonsFirst, srcWebFirst)
	}
	for _, bad := range []string{"addons/**", webGlob, "zzz/**"} {
		if srcAddonsFirst == bad {
			t.Errorf("Src = %q, derived from a module Paths glob (must be a filesystem root, not a classification glob)", srcAddonsFirst)
		}
	}
	if srcAddonsFirst != "." {
		t.Errorf("Src = %q, want %q (the extractor-agnostic default; TS falls back to \"src\")", srcAddonsFirst, ".")
	}
}

func TestDefaultIncludesRust(t *testing.T) {
	cfg := config.Default()
	if got := cfg.Languages.Rust.Enabled; got != evidenceports.ModeAuto {
		t.Errorf("Default rust mode = %q, want auto", got)
	}
	if got := cfg.ForClassify().DuplicatedKnowledgePolicy; got != policy.DuplicatedKnowledgePolicyScore {
		t.Errorf("Default duplicated knowledge policy = %q, want %q", got, policy.DuplicatedKnowledgePolicyScore)
	}
}

// Test fixtures factored out to keep goconst quiet about repeated literals.
const (
	rustManifestPath            = "crates/core/Cargo.toml"
	globSvcAll                  = "svc/**" // tools.go.modules include glob used across subtests
	errBlastRadiusInformational = "metrics.blast_radius is informational and never gates"
)

var rustFeatures = []string{"serde", "tokio"}

func TestLoadRustFields(t *testing.T) {
	yaml := "version: 2\n" +
		"languages:\n" +
		"  rust:\n" +
		"    manifest: " + rustManifestPath + "\n" +
		"    features: [serde, tokio]\n" +
		"    include_dev_deps: true\n"
	path := filepath.Join(t.TempDir(), "rust.yaml")
	if err := os.WriteFile(path, []byte(yaml), 0o600); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	cfg, err := config.Load(context.Background(), path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	ec := cfg.ForExtract("rust")
	if ec.CargoManifest != rustManifestPath {
		t.Errorf("CargoManifest = %q, want %q", ec.CargoManifest, rustManifestPath)
	}
	if !slices.Equal(ec.CargoFeatures, rustFeatures) {
		t.Errorf("CargoFeatures = %v, want %v", ec.CargoFeatures, rustFeatures)
	}
	if !ec.IncludeDevDeps {
		t.Error("IncludeDevDeps = false, want true")
	}
}

func TestForClassify(t *testing.T) {
	cfg, err := config.Load(context.Background(), "testdata/valid.yaml")
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	cc := cfg.ForClassify()
	if len(cc.Modules) != len(cfg.Modules) {
		t.Errorf("ForClassify().Modules len=%d, want %d", len(cc.Modules), len(cfg.Modules))
	}
	if len(cc.Layers) != len(cfg.Layers) {
		t.Errorf("ForClassify().Layers len=%d, want %d", len(cc.Layers), len(cfg.Layers))
	}
}

// TestWithExplicitOwners verifies the test seam: a Config literal (bypassing Load)
// carries no explicit owners until marked, and WithExplicitOwners threads through
// ForClassify so classify can treat marked ownership as authoritative.
func TestWithExplicitOwners(t *testing.T) {
	cfg := config.Config{
		Version: 1,
		Modules: map[string]policy.ModuleDef{
			"a": {Owner: "x"},
			"b": {Owner: "x"},
		},
	}
	if eo := cfg.ForClassify().ExplicitOwners; len(eo) != 0 {
		t.Errorf("unmarked literal ExplicitOwners = %v, want empty", eo)
	}
	got := cfg.WithExplicitOwners("a").ForClassify().ExplicitOwners
	if !got["a"] {
		t.Error(`ExplicitOwners["a"] = false, want true`)
	}
	if got["b"] {
		t.Error(`ExplicitOwners["b"] = true, want false (not marked)`)
	}
}

// TestLoad_PopulatesExplicitOwners verifies the production path: Load records a
// module as having an explicit owner iff its YAML `owner:` is non-empty, BEFORE
// any resolver fill.
func TestLoad_PopulatesExplicitOwners(t *testing.T) {
	path := filepath.Join(t.TempDir(), ".archfit.yaml")
	yaml := "version: 2\n" +
		"modules:\n" +
		"  owned:\n    paths: [\"owned/**\"]\n    owner: team-x\n" +
		"  bare:\n    paths: [\"bare/**\"]\n"
	if err := os.WriteFile(path, []byte(yaml), 0o600); err != nil {
		t.Fatalf("write config: %v", err)
	}
	cfg, err := config.Load(context.Background(), path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	eo := cfg.ForClassify().ExplicitOwners
	if !eo["owned"] {
		t.Error(`ExplicitOwners["owned"] = false, want true (has YAML owner)`)
	}
	if eo["bare"] {
		t.Error(`ExplicitOwners["bare"] = true, want false (no owner)`)
	}
}

func TestForRules(t *testing.T) {
	cfg, err := config.Load(context.Background(), "testdata/valid.yaml")
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	rc := cfg.ForRules()
	if len(rc.Rules) != len(cfg.Rules) {
		t.Errorf("ForRules().Rules len=%d, want %d", len(rc.Rules), len(cfg.Rules))
	}
}

func TestFunctionLOCThresholdConfig(t *testing.T) {
	t.Run("absent uses the diagnostic default", func(t *testing.T) {
		cfg := config.Config{Version: config.SchemaVersion}
		if got := cfg.Metrics.FunctionLOCThresholdValue(); got != policy.DefaultFunctionLOCThreshold {
			t.Fatalf("function LOC threshold = %d, want default %d", got, policy.DefaultFunctionLOCThreshold)
		}
		if got := cfg.PolicySnapshot().Assessment.FunctionLOCThreshold; got != policy.DefaultFunctionLOCThreshold {
			t.Fatalf("projected threshold = %d, want default %d", got, policy.DefaultFunctionLOCThreshold)
		}
	})

	t.Run("scalar coexists with object metric entries", func(t *testing.T) {
		cfg, err := loadConfigInline(t, "version: 2\nmetrics:\n  function_loc_threshold: 75\n  cycle:\n    enabled: true\n    gate: fail\n    max_new: 0\n")
		if err != nil {
			t.Fatal(err)
		}
		if got := cfg.Metrics.FunctionLOCThresholdValue(); got != 75 {
			t.Fatalf("function LOC threshold = %d, want 75", got)
		}
		if cycle := cfg.ForMetric("cycle"); cycle.Enabled == nil || !*cycle.Enabled || cycle.Gate != "fail" {
			t.Fatalf("cycle metric entry = %+v, want existing object form preserved", cycle)
		}
		if _, reservedEnteredGates := cfg.PolicySnapshot().Gates.Metrics["function_loc_threshold"]; reservedEnteredGates {
			t.Fatal("function_loc_threshold entered metric gates")
		}

		encoded, err := yaml.Marshal(cfg)
		if err != nil {
			t.Fatalf("marshal config: %v", err)
		}
		roundTrip, err := loadConfigInline(t, string(encoded))
		if err != nil {
			t.Fatalf("round-trip config: %v\n%s", err, encoded)
		}
		if roundTrip.Metrics.FunctionLOCThresholdValue() != 75 || roundTrip.ForMetric("cycle").Gate != "fail" {
			t.Fatalf("round-trip metrics = %+v", roundTrip.Metrics)
		}
	})

	for _, tc := range []struct {
		name  string
		value string
		want  string
	}{
		{name: "zero", value: "0", want: "positive integer"},
		{name: "negative", value: "-1", want: "positive integer"},
		{name: "fraction", value: "1.5", want: "must be an integer"},
		{name: "object", value: "{ threshold: 60 }", want: "must be an integer"},
	} {
		t.Run("rejects "+tc.name, func(t *testing.T) {
			_, err := loadConfigInline(t, "version: 2\nmetrics:\n  function_loc_threshold: "+tc.value+"\n")
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("error = %v, want %q", err, tc.want)
			}
		})
	}
}

func TestForMetric(t *testing.T) {
	cfg, err := config.Load(context.Background(), "testdata/valid.yaml")
	if err != nil {
		t.Fatalf("Load: %v", err)
	}

	t.Run("known_metric", func(t *testing.T) {
		mc := cfg.ForMetric("encapsulation")
		if mc.Enabled == nil || !*mc.Enabled {
			t.Error("ForMetric(encapsulation).Enabled != true, want explicit true")
		}
		if mc.Gate != "warn" {
			t.Errorf("ForMetric(encapsulation).Gate = %q, want warn", mc.Gate)
		}
	})

	t.Run("unknown_metric_zero_value", func(t *testing.T) {
		mc := cfg.ForMetric("nonexistent")
		if mc.Enabled != nil {
			t.Errorf("ForMetric(nonexistent).Enabled = %v, want nil (absent)", *mc.Enabled)
		}
	})
}

func TestForStaleness_GateOffDisables(t *testing.T) {
	const gateOff, gateWarn, gateFail = "off", "warn", "fail"
	tests := []struct {
		name        string
		mapReview   config.ModuleReviewConfig
		wantEnabled bool
	}{
		{"gate off disables", config.ModuleReviewConfig{Gate: gateOff}, false},
		{"gate off overrides stale_after", config.ModuleReviewConfig{Gate: gateOff, StaleAfter: "720h"}, false},
		{"gate warn enables", config.ModuleReviewConfig{Gate: gateWarn}, true},
		{"gate fail enables", config.ModuleReviewConfig{Gate: gateFail}, true},
		{"stale_after enables", config.ModuleReviewConfig{StaleAfter: "720h"}, true},
		{"nothing set stays disabled", config.ModuleReviewConfig{}, false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			cfg := config.Config{ModuleReview: tc.mapReview}
			if got := cfg.ForStaleness().Enabled; got != tc.wantEnabled {
				t.Errorf("ForStaleness().Enabled = %v, want %v", got, tc.wantEnabled)
			}
		})
	}
}

func TestForStatus(t *testing.T) {
	cfg, err := config.Load(context.Background(), "testdata/valid.yaml")
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	es := cfg.ForWaivers()
	if len(es.Waivers) != len(cfg.Waivers) {
		t.Errorf("ForWaivers().Waivers len=%d, want %d", len(es.Waivers), len(cfg.Waivers))
	}
}

func TestToolMode_UnmarshalYAML(t *testing.T) {
	// Inline config struct to test ToolMode decoding directly via Load.
	tests := []struct {
		name    string
		yaml    string
		wantErr bool
		want    evidenceports.ToolMode
	}{
		{"bool_true", "version: 2\nanalyzers:\n  clones:\n    enabled: true\n", false, evidenceports.ModeOn},
		{"bool_false", "version: 2\nanalyzers:\n  clones:\n    enabled: false\n", false, evidenceports.ModeOff},
		{"string_auto", "version: 2\nanalyzers:\n  clones:\n    enabled: auto\n", false, evidenceports.ModeAuto},
		// on/off are no longer accepted — canonical vocabulary is true|false|auto.
		{"bare_on_rejected", "version: 2\nanalyzers:\n  clones:\n    enabled: on\n", true, ""},
		{"quoted_on_rejected", "version: 2\nanalyzers:\n  clones:\n    enabled: \"on\"\n", true, ""},
		{"quoted_off_rejected", "version: 2\nanalyzers:\n  clones:\n    enabled: \"off\"\n", true, ""},
		{"invalid", "version: 2\nanalyzers:\n  clones:\n    enabled: maybe\n", true, ""},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			// Write to a temp file to reuse config.Load.
			tmp := t.TempDir() + "/cfg.yaml"
			if err := writeFile(tmp, tc.yaml); err != nil {
				t.Fatalf("write temp: %v", err)
			}
			cfg, err := config.Load(context.Background(), tmp)
			if tc.wantErr {
				if err == nil {
					t.Error("expected error, got nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("Load: %v", err)
			}
			got := cfg.Analyzers.Clones.Enabled
			if got != tc.want {
				t.Errorf("Enabled = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestLoad_Patterns(t *testing.T) {
	cfg, err := config.Load(context.Background(), "testdata/patterns.yaml")
	if err != nil {
		t.Fatalf("Load: %v", err)
	}

	// Verify the rule with patterns parsed correctly.
	if len(cfg.Rules) != 2 {
		t.Fatalf("len(Rules) = %d, want 2", len(cfg.Rules))
	}
	rule := cfg.Rules[0]
	if rule.ID != "no_unsafe_ptr" {
		t.Errorf("Rules[0].ID = %q, want no_unsafe_ptr", rule.ID)
	}
	if len(rule.Patterns) != 2 {
		t.Fatalf("Rules[0].Patterns len=%d, want 2", len(rule.Patterns))
	}

	p0 := rule.Patterns[0]
	if p0.ID != "unsafe_cast" {
		t.Errorf("Patterns[0].ID = %q, want unsafe_cast", p0.ID)
	}
	if p0.Lang != "go" {
		t.Errorf("Patterns[0].Lang = %q, want go", p0.Lang)
	}
	if p0.Rule != "unsafe.Pointer($X)" {
		t.Errorf("Patterns[0].Rule = %q, want unsafe.Pointer($X)", p0.Rule)
	}

	p1 := rule.Patterns[1]
	if p1.ID != "reflect_unexported" {
		t.Errorf("Patterns[1].ID = %q, want reflect_unexported", p1.ID)
	}

	// Rule without patterns should have nil/empty Patterns slice.
	if len(cfg.Rules[1].Patterns) != 0 {
		t.Errorf("Rules[1].Patterns len=%d, want 0", len(cfg.Rules[1].Patterns))
	}
}

func TestForPatterns(t *testing.T) {
	tests := []struct {
		name    string
		rules   []policy.RuleDef
		wantLen int
		wantIDs []string
	}{
		{
			name:    "no_rules",
			rules:   nil,
			wantLen: 0,
		},
		{
			name: "rules_without_patterns",
			rules: []policy.RuleDef{
				{ID: "r1", Type: "forbidden_dependency"},
				{ID: "r2", Type: "public_api_only"},
			},
			wantLen: 0,
		},
		{
			name: "one_rule_with_patterns",
			rules: []policy.RuleDef{
				{
					ID:   "r1",
					Type: "forbidden_dependency",
					Patterns: []pattern.Def{
						{ID: "p1", Lang: "go", Rule: "unsafe.Pointer($X)"},
						{ID: "p2", Lang: "go", Rule: "reflect.ValueOf($X)"},
					},
				},
			},
			wantLen: 2,
			wantIDs: []string{"p1", "p2"},
		},
		{
			name: "multiple_rules_with_patterns",
			rules: []policy.RuleDef{
				{
					ID: "r1",
					Patterns: []pattern.Def{
						{ID: "p1", Lang: "go", Rule: "foo($X)"},
					},
				},
				{ID: "r2"}, // no patterns
				{
					ID: "r3",
					Patterns: []pattern.Def{
						{ID: "p2", Lang: langTypeScript, Rule: "bar($X)"},
						{ID: "p3", Lang: langTypeScript, Rule: "baz($X)"},
					},
				},
			},
			wantLen: 3,
			wantIDs: []string{"p1", "p2", "p3"},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			cfg := config.Config{
				Version: 1,
				Rules:   tc.rules,
			}
			got := cfg.ForPatterns()
			if len(got) != tc.wantLen {
				t.Errorf("ForPatterns() len=%d, want %d", len(got), tc.wantLen)
			}
			for i, wantID := range tc.wantIDs {
				if i >= len(got) {
					t.Errorf("ForPatterns()[%d] missing, want ID=%q", i, wantID)
					continue
				}
				if got[i].ID != wantID {
					t.Errorf("ForPatterns()[%d].ID = %q, want %q", i, got[i].ID, wantID)
				}
			}
		})
	}
}

// loadInline writes body to a temp config file and loads it, returning the error.
func loadInline(t *testing.T, body string) error {
	t.Helper()
	_, err := loadConfigInline(t, body)
	return err
}

func loadConfigInline(t *testing.T, body string) (config.Config, error) {
	t.Helper()
	dir := t.TempDir()
	p := filepath.Join(dir, ".archfit.yaml")
	if err := os.WriteFile(p, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	return config.Load(context.Background(), p)
}

func TestLoad_SuppliedCoverage(t *testing.T) {
	t.Run("absent block defaults disabled", func(t *testing.T) {
		cfg, err := loadConfigInline(t, "version: 2\n")
		if err != nil {
			t.Fatal(err)
		}
		if cfg.Coverage.Enabled || cfg.SuppliedCoverageOptions().Enabled {
			t.Fatal("absent coverage block must default disabled")
		}
	})

	t.Run("explicit false decodes disabled", func(t *testing.T) {
		cfg, err := loadConfigInline(t, "version: 2\ncoverage:\n  enabled: false\n  gate: warn\n  sources:\n    - path: coverage.out\n      format: go-coverprofile\n      sidecar_path: evidence/coverage.sidecar.json\n")
		if err != nil {
			t.Fatal(err)
		}
		got := cfg.SuppliedCoverageOptions()
		if got.Enabled || got.Gate != string(config.GateWarn) || len(got.Sources) != 1 {
			t.Fatalf("coverage projection = %+v", got)
		}
		if got.Sources[0].Path != "coverage.out" || got.Sources[0].Format != "go-coverprofile" || got.Sources[0].SidecarPath != "evidence/coverage.sidecar.json" {
			t.Fatalf("coverage source projection = %+v", got.Sources[0])
		}
	})

	t.Run("omitted format projects auto", func(t *testing.T) {
		cfg, err := loadConfigInline(t, "version: 2\ncoverage:\n  enabled: true\n  sources:\n    - path: coverage.out\n")
		if err != nil {
			t.Fatal(err)
		}
		if got := cfg.SuppliedCoverageOptions().Sources[0].Format; got != "auto" {
			t.Fatalf("format = %q, want auto", got)
		}
	})

	for _, tc := range []struct {
		name string
		body string
		want string
	}{
		{name: "enabled without sources", body: "version: 2\ncoverage:\n  enabled: true\n", want: "coverage.sources requires at least one source"},
		{name: "invalid gate", body: "version: 2\ncoverage:\n  gate: block\n", want: "coverage.gate"},
		{name: "missing path", body: "version: 2\ncoverage:\n  sources:\n    - format: lcov\n", want: "coverage.sources[0].path is required"},
		{name: "invalid format", body: "version: 2\ncoverage:\n  sources:\n    - path: coverage.out\n      format: cobertura\n", want: "is not one of"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if err := loadInline(t, tc.body); err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("Load error = %v, want substring %q", err, tc.want)
			}
		})
	}
}

func TestLoad_SuppliedCoverageLimits(t *testing.T) {
	t.Run("omitted limits project bounded defaults", func(t *testing.T) {
		cfg, err := loadConfigInline(t, "version: 2\ncoverage:\n  enabled: true\n  sources:\n    - path: coverage.out\n")
		if err != nil {
			t.Fatal(err)
		}
		if cfg.Coverage.Sources[0].MaxBytes != nil || cfg.Coverage.Sources[0].MaxFacts != nil {
			t.Fatalf("omitted decoded limits = %+v, want nil pointers", cfg.Coverage.Sources[0])
		}
		got := cfg.SuppliedCoverageOptions().Sources[0]
		if got.MaxBytes != suppliedcoverage.DefaultMaxBytes || got.MaxFacts != suppliedcoverage.DefaultMaxFacts {
			t.Fatalf("projected defaults = %d bytes/%d facts, want %d/%d", got.MaxBytes, got.MaxFacts, suppliedcoverage.DefaultMaxBytes, suppliedcoverage.DefaultMaxFacts)
		}
	})

	t.Run("configured positive limits survive projection", func(t *testing.T) {
		cfg, err := loadConfigInline(t, "version: 2\ncoverage:\n  enabled: true\n  sources:\n    - path: coverage.out\n      max_bytes: 12345\n      max_facts: 678\n")
		if err != nil {
			t.Fatal(err)
		}
		got := cfg.SuppliedCoverageOptions().Sources[0]
		if got.MaxBytes != 12345 || got.MaxFacts != 678 {
			t.Fatalf("projected configured limits = %d/%d, want 12345/678", got.MaxBytes, got.MaxFacts)
		}
	})

	for _, tc := range []struct {
		name  string
		field string
		value int
	}{
		{name: "zero bytes", field: "max_bytes", value: 0},
		{name: "negative bytes", field: "max_bytes", value: -1},
		{name: "zero facts", field: "max_facts", value: 0},
		{name: "negative facts", field: "max_facts", value: -1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			body := "version: 2\ncoverage:\n  enabled: true\n  sources:\n    - path: coverage.out\n      " + tc.field + ": " + strconv.Itoa(tc.value) + "\n"
			if err := loadInline(t, body); err == nil || !strings.Contains(err.Error(), tc.field+" must be positive") {
				t.Fatalf("Load error = %v, want positive %s validation", err, tc.field)
			}
		})
	}
}

func TestLoad_ExternalSystems(t *testing.T) {
	t.Run("valid entry decodes and projects into classify.Config", func(t *testing.T) {
		p := filepath.Join(t.TempDir(), ".archfit.yaml")
		body := "version: 2\nexternal_systems:\n  aws:\n    targets: [\"github.com/aws/aws-sdk-go-v2/**\"]\n    volatility: medium\n  payment-gateway:\n    targets: [\"node_modules/@stripe/**\", \"stripe\"]\n"
		if err := os.WriteFile(p, []byte(body), 0o600); err != nil {
			t.Fatal(err)
		}
		cfg, err := config.Load(context.Background(), p)
		if err != nil {
			t.Fatalf("Load: %v", err)
		}
		aws := cfg.ExternalSystems["aws"]
		if len(aws.Targets) != 1 || aws.Volatility != "medium" {
			t.Errorf("aws = %+v, want 1 target + medium volatility", aws)
		}
		if pg := cfg.ExternalSystems["payment-gateway"]; len(pg.Targets) != 2 || pg.Volatility != "" {
			t.Errorf("payment-gateway = %+v, want 2 targets + unset volatility (defaults to low in classify)", pg)
		}
		if got := cfg.ForClassify().ExternalSystems; len(got) != 2 {
			t.Errorf("ForClassify().ExternalSystems len = %d, want 2", len(got))
		}
	})

	t.Run("entry without targets is rejected", func(t *testing.T) {
		err := loadInline(t, "version: 2\nexternal_systems:\n  aws:\n    volatility: low\n")
		if err == nil || !strings.Contains(err.Error(), "external_systems.aws requires at least one targets glob") {
			t.Errorf("got %v, want 'requires at least one targets glob' error", err)
		}
	})

	t.Run("case-variant volatility is accepted", func(t *testing.T) {
		err := loadInline(t, "version: 2\nexternal_systems:\n  aws:\n    targets: [\"github.com/aws/**\"]\n    volatility: High\n")
		if err != nil {
			t.Errorf("got %v, want High accepted (classify matches case-insensitively)", err)
		}
	})

	t.Run("invalid volatility is rejected", func(t *testing.T) {
		err := loadInline(t, "version: 2\nexternal_systems:\n  aws:\n    targets: [\"github.com/aws/**\"]\n    volatility: sometimes\n")
		if err == nil || !strings.Contains(err.Error(), `external_systems.aws.volatility "sometimes"`) {
			t.Errorf("got %v, want volatility enum error", err)
		}
	})

	t.Run("empty target glob is rejected", func(t *testing.T) {
		err := loadInline(t, "version: 2\nexternal_systems:\n  aws:\n    targets: [\"\"]\n")
		if err == nil || !strings.Contains(err.Error(), "external_systems.aws.targets[0] must not be empty") {
			t.Errorf("got %v, want empty-target error", err)
		}
	})

	t.Run("malformed glob is rejected", func(t *testing.T) {
		err := loadInline(t, "version: 2\nexternal_systems:\n  aws:\n    targets: [\"github.com/[aws/**\"]\n")
		if err == nil || !strings.Contains(err.Error(), "is not a valid glob pattern") {
			t.Errorf("got %v, want invalid-glob error", err)
		}
	})
}

func TestLoad_UnknownMetricKey_IsError(t *testing.T) {
	err := loadInline(t, "version: 2\nmetrics:\n  bogus:\n    enabled: true\n")
	if err == nil || !strings.Contains(err.Error(), "metrics.bogus is not a known metric") {
		t.Errorf("unknown metric: got %v, want 'not a known metric' error", err)
	}
}

func TestLoad_RemovedMetricKey_IsActionableError(t *testing.T) {
	for _, key := range []string{"risk_hub", "functional_candidates"} {
		err := loadInline(t, "version: 2\nmetrics:\n  "+key+":\n    enabled: true\n")
		if err == nil || !strings.Contains(err.Error(), "removed in v1.0") {
			t.Errorf("removed metric %q: got %v, want 'removed in v1.0'", key, err)
		}
	}
}

func TestLoad_DeprecatedToolsKey_IsActionableError(t *testing.T) {
	err := loadInline(t, "version: 2\ntools:\n  scip:\n    enabled: true\n")
	if err == nil || !strings.Contains(err.Error(), "renamed to `analyzers:`") {
		t.Errorf("tools key: got %v, want 'renamed to analyzers:' hint", err)
	}
}

func TestLoad_ExistingConfigUnchanged(t *testing.T) {
	// Existing configs without patterns: must still load cleanly.
	cfg, err := config.Load(context.Background(), "testdata/valid.yaml")
	if err != nil {
		t.Fatalf("Load valid.yaml: %v", err)
	}
	for _, r := range cfg.Rules {
		if len(r.Patterns) != 0 {
			t.Errorf("rule %q: Patterns len=%d, want 0 (existing config has no patterns)", r.ID, len(r.Patterns))
		}
	}
	// ForPatterns on a config with no patterns returns empty, not nil.
	pc := cfg.ForPatterns()
	if len(pc) != 0 {
		t.Errorf("ForPatterns() len=%d on no-patterns config, want 0", len(pc))
	}
}

// TestLoad_NewToolsAndMetrics verifies that new tool entries and the three
// Tranche-1 metric entries parse correctly (checkbox: load with new keys).
func TestLoad_NewToolsAndMetrics(t *testing.T) {
	cfg, err := config.Load(context.Background(), "testdata/new_tools_metrics.yaml")
	if err != nil {
		t.Fatalf("Load: %v", err)
	}

	// analyzers.clones.enabled: true
	if got := cfg.Analyzers.Clones.Enabled; got != evidenceports.ModeOn {
		t.Errorf("analyzers.clones.enabled = %q, want on", got)
	}
	if !cfg.ClonesEnabled() {
		t.Error("ClonesEnabled() = false, want true when enabled")
	}

	// metrics.blast_radius: enabled false
	rh := cfg.ForMetric("blast_radius")
	if rh.Enabled == nil || *rh.Enabled {
		t.Error("ForMetric(blast_radius).Enabled != false, want explicit false")
	}

	// metrics.encapsulation: enabled true, gate warn
	af := cfg.ForMetric("encapsulation")
	if af.Enabled == nil || !*af.Enabled {
		t.Error("ForMetric(encapsulation).Enabled != true, want explicit true")
	}
	if af.Gate != "warn" {
		t.Errorf("ForMetric(encapsulation).Gate = %q, want warn", af.Gate)
	}

	// metrics.coverage: enabled false
	fc := cfg.ForMetric("coverage")
	if fc.Enabled == nil || *fc.Enabled {
		t.Error("ForMetric(coverage).Enabled != false, want explicit false")
	}
}

// TestNewToolsDefaultOff verifies that absent clones entries default to
// disabled (zero ToolMode is not ModeOn), matching the opt-in contract.
func TestNewToolsDefaultOff(t *testing.T) {
	cfg := config.Config{Version: 1}
	if cfg.ClonesEnabled() {
		t.Error("ClonesEnabled() = true when absent, want false")
	}
}

// TestNewMetricsDefaultZero verifies that absent Tranche-1 metric entries return
// a zero MetricEntry (Enabled=nil, Gate=""), consistent with ForMetric contract.
func TestNewMetricsDefaultZero(t *testing.T) {
	cfg := config.Config{Version: 1}
	for _, name := range []string{"blast_radius", "encapsulation", "coverage"} {
		mc := cfg.ForMetric(name)
		if mc.Enabled != nil {
			t.Errorf("ForMetric(%q).Enabled = %v on empty config, want nil (absent)", name, *mc.Enabled)
		}
		if mc.Gate != "" {
			t.Errorf("ForMetric(%q).Gate = %q on empty config, want empty", name, mc.Gate)
		}
	}
}

// TestNewToolInvalidMode verifies that an invalid mode value for an analyzer key is rejected.
func TestNewToolInvalidMode(t *testing.T) {
	yaml := "version: 2\nanalyzers:\n  clones:\n    enabled: maybe\n"
	tmp := t.TempDir() + "/cfg.yaml"
	if err := writeFile(tmp, yaml); err != nil {
		t.Fatalf("write temp: %v", err)
	}
	_, err := config.Load(context.Background(), tmp)
	if err == nil {
		t.Error("Load: expected error for invalid clones mode, got nil")
	}
}

// TestLoad_SelfConfig verifies that the project's own .archfit.yaml — the realistic
// config we run locally and in CI — loads cleanly and is well-formed. It deliberately
// does NOT pin opt-in toggles (scip, cargo_modules, …): those are operational choices,
// not invariants, and pinning them broke this test on every legitimate config change.
// Toggle-accessor behavior is covered against synthetic configs by TestForMetric,
// TestNewToolsDefaultOff, and the testdata fixture tests above.
func TestLoad_SelfConfig(t *testing.T) {
	// Go tests run with cwd = package dir (internal/config); repo root is two levels up.
	cfg, err := config.Load(context.Background(), "../../.archfit.yaml")
	if err != nil {
		t.Fatalf("Load self-config: %v", err)
	}
	if len(cfg.Modules) == 0 {
		t.Error("self-config: no modules parsed")
	}
	if len(cfg.Layers) == 0 {
		t.Error("self-config: no layers parsed")
	}
}

func TestSelfConfig_CapabilityModuleMap(t *testing.T) {
	cfg, err := config.Load(context.Background(), "../../.archfit.yaml")
	if err != nil {
		t.Fatalf("Load self-config: %v", err)
	}

	tests := []struct {
		name      string
		layer     string
		role      policy.Role
		wantPaths []string
	}{
		{
			name: "evidence-contracts", layer: layerModel, role: policy.RoleSharedModel,
			wantPaths: []string{"internal/model/evidence/**", "internal/evidence", "internal/evidence/*.go"},
		},
		{
			name: "analysis-scope", layer: layerSupport, role: policy.RoleSharedModel,
			wantPaths: []string{"internal/scope/**"},
		},
		{
			name: "evidence-analysis", layer: layerCore, role: policy.RoleCore,
			wantPaths: []string{"internal/syntax/**"},
		},
		{
			name: "evidence-adapters", layer: layerAdapter, role: policy.RoleAdapter,
			wantPaths: []string{"internal/extract/**", "internal/toolrun/**", "internal/evidence/ports/**"},
		},
		{
			name: "persistence-adapters", layer: layerAdapter, role: policy.RoleAdapter,
			wantPaths: []string{"internal/factcache/**", "internal/history/**", "internal/ownership/**", "internal/baseline/**", "internal/labels/**"},
		},
		{
			name: "provider-adapters", layer: layerAdapter, role: policy.RoleAdapter,
			wantPaths: []string{"internal/llm/**"},
		},
		{
			name: "report-adapters", layer: layerAdapter, role: policy.RoleAdapter,
			wantPaths: []string{"internal/output/**"},
		},
		{
			name: "architecture-policy", layer: layerCore, role: policy.RoleCore,
			wantPaths: []string{"internal/policy/**"},
		},
		{
			name: "relationship-analysis", layer: layerCore, role: policy.RoleCore,
			wantPaths: []string{"internal/relationship/**"},
		},
		{
			name: "assessment-repair", layer: layerCore, role: policy.RoleCore,
			wantPaths: []string{"internal/assessment/**"},
		},
		{
			name: "analysis-application", layer: "application",
			wantPaths: []string{"internal/application/**"},
		},
		{
			name: "report-contract", layer: layerModel, role: policy.RoleSharedModel,
			wantPaths: []string{"internal/model/report/**"},
		},
		{
			name: "evidence-acquisition", layer: layerAdapter, role: policy.RoleAdapter,
			wantPaths: []string{"internal/evidence/acquisition/**"},
		},
		{
			name: "cli-composition", layer: "cmd", role: policy.RoleCompositionRoot,
			wantPaths: []string{"cmd/archfit/**"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			def, ok := cfg.Modules[tt.name]
			if !ok {
				t.Fatalf("capability module %q not found in self-config", tt.name)
			}
			if def.Layer != tt.layer {
				t.Errorf("module %q: layer = %q, want %q", tt.name, def.Layer, tt.layer)
			}
			if def.Role != tt.role {
				t.Errorf("module %q: role = %q, want %q", tt.name, def.Role, tt.role)
			}
			for _, path := range tt.wantPaths {
				if !slices.Contains(def.Paths, path) {
					t.Errorf("module %q: paths %v do not include %q", tt.name, def.Paths, path)
				}
			}
		})
	}
}

func TestSelfConfig_RoleLayerConformance(t *testing.T) {
	cfg, err := config.Load(context.Background(), "../../.archfit.yaml")
	if err != nil {
		t.Fatalf("Load self-config: %v", err)
	}

	names := make([]string, 0, len(cfg.Modules))
	for name := range cfg.Modules {
		names = append(names, name)
	}
	slices.Sort(names)

	for _, name := range names {
		def := cfg.Modules[name]
		switch def.Role {
		case "":
		case policy.RoleAdapter:
			if def.Layer != layerAdapter {
				t.Errorf("module %q: role=adapter but layer=%q (want adapter)", name, def.Layer)
			}
		case policy.RoleCompositionRoot, policy.RoleTest:
			if def.Layer != "cmd" {
				t.Errorf("module %q: role=%s but layer=%q (want cmd)", name, def.Role, def.Layer)
			}
		case policy.RoleCore:
			if def.Layer != layerCore {
				t.Errorf("module %q: role=core but layer=%q (want core)", name, def.Layer)
			}
		case policy.RoleSharedModel:
			if def.Layer != "model" && def.Layer != "support" {
				t.Errorf("module %q: role=shared_model but layer=%q (want model or support)", name, def.Layer)
			}
		default:
			t.Errorf("module %q: unknown role %q", name, def.Role)
		}
	}
}

// TestFillMissingOwners verifies that FillMissingOwners merges resolved ownership
// correctly: config owner wins, resolver fills gaps, absent entries are unchanged.
func TestFillMissingOwners(t *testing.T) {
	const (
		resolvedOwnerA = "@team-alpha"
		resolvedOwnerB = "@team-beta"
		configOwnerX   = "@team-x"
		pathPkgA       = "pkg/a/**"
		pathPkgB       = "pkg/b/**"
	)

	tests := []struct {
		name     string
		modules  map[string]policy.ModuleDef
		resolved map[string]string
		want     map[string]string // module name → expected Owner after call
	}{
		{
			name: "fills modules with no owner",
			modules: map[string]policy.ModuleDef{
				"a": {Paths: []string{pathPkgA}},
				"b": {Paths: []string{pathPkgB}},
			},
			resolved: map[string]string{
				"a": resolvedOwnerA,
				"b": resolvedOwnerB,
			},
			want: map[string]string{
				"a": resolvedOwnerA,
				"b": resolvedOwnerB,
			},
		},
		{
			name: "config owner wins over resolver",
			modules: map[string]policy.ModuleDef{
				"a": {Paths: []string{pathPkgA}, Owner: configOwnerX},
				"b": {Paths: []string{pathPkgB}},
			},
			resolved: map[string]string{
				"a": resolvedOwnerA, // must not overwrite configOwnerX
				"b": resolvedOwnerB,
			},
			want: map[string]string{
				"a": configOwnerX,   // config wins
				"b": resolvedOwnerB, // resolver fills gap
			},
		},
		{
			name: "module absent from resolved stays unchanged",
			modules: map[string]policy.ModuleDef{
				"a": {Paths: []string{pathPkgA}},
				"b": {Paths: []string{pathPkgB}},
			},
			resolved: map[string]string{
				"a": resolvedOwnerA,
				// "b" absent from resolved
			},
			want: map[string]string{
				"a": resolvedOwnerA,
				"b": "", // unchanged — no owner from either source
			},
		},
		{
			name: "empty resolved map — no change",
			modules: map[string]policy.ModuleDef{
				"a": {Paths: []string{pathPkgA}},
			},
			resolved: map[string]string{},
			want: map[string]string{
				"a": "",
			},
		},
		{
			name: "empty resolved owner string — no change",
			modules: map[string]policy.ModuleDef{
				"a": {Paths: []string{pathPkgA}},
			},
			resolved: map[string]string{
				"a": "", // empty string — must not be written
			},
			want: map[string]string{
				"a": "",
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			cfg := config.Config{
				Version: 1,
				Modules: tc.modules,
			}
			cfg.FillMissingOwners(tc.resolved)
			for mod, wantOwner := range tc.want {
				def, ok := cfg.Modules[mod]
				if !ok {
					t.Errorf("module %q not found after FillMissingOwners", mod)
					continue
				}
				if def.Owner != wantOwner {
					t.Errorf("module %q: Owner = %q, want %q", mod, def.Owner, wantOwner)
				}
			}
		})
	}
}

// TestLoad_ValidateEnums checks that bad coupling.min_severity and gate values
// are rejected at load with a descriptive error, not silently accepted (which would
// disable the check the field was meant to configure), while valid values load clean.
func TestLoad_ValidateEnums(t *testing.T) {
	tests := []struct {
		name    string
		yaml    string
		wantErr string // substring the error must mention; "" = expect success
	}{
		{
			name:    "valid bc severity",
			yaml:    "version: 2\ncoupling:\n  min_severity: critical\n",
			wantErr: "",
		},
		{
			name:    "empty bc severity is allowed",
			yaml:    yamlV1,
			wantErr: "",
		},
		{
			name:    "invalid bc severity",
			yaml:    "version: 2\ncoupling:\n  min_severity: severe\n",
			wantErr: "coupling.min_severity",
		},
		{
			name:    "duplicated knowledge score policy loads clean",
			yaml:    "version: 2\ncoupling:\n  duplicated_knowledge: score\n",
			wantErr: "",
		},
		{
			name:    "duplicated knowledge advisory policy loads clean",
			yaml:    "version: 2\ncoupling:\n  duplicated_knowledge: advisory\n",
			wantErr: "",
		},
		{
			name:    "invalid duplicated knowledge policy rejected",
			yaml:    "version: 2\ncoupling:\n  duplicated_knowledge: maybe\n",
			wantErr: "coupling.duplicated_knowledge",
		},
		{
			name:    "coupling.gate with distributed_monolith warn loads clean",
			yaml:    "version: 2\ncoupling:\n  gate:\n    distributed_monolith:\n      mode: warn\n",
			wantErr: "",
		},
		{
			name:    "coupling.gate with distributed_monolith fail and a tolerance loads clean",
			yaml:    "version: 2\ncoupling:\n  gate:\n    distributed_monolith:\n      mode: fail\n      max_new_seams: 2\n",
			wantErr: "",
		},
		{
			name:    "coupling.gate with an empty distributed_monolith block takes the warn default",
			yaml:    "version: 2\ncoupling:\n  gate:\n    distributed_monolith: {}\n",
			wantErr: "",
		},
		{
			name:    "empty coupling.gate block rejected",
			yaml:    "version: 2\ncoupling:\n  gate: {}\n",
			wantErr: "coupling.gate requires distributed_monolith",
		},
		{
			name:    "invalid distributed_monolith mode rejected",
			yaml:    "version: 2\ncoupling:\n  gate:\n    distributed_monolith:\n      mode: block\n",
			wantErr: "distributed_monolith.mode",
		},
		{
			name:    "negative max_new_seams rejected",
			yaml:    "version: 2\ncoupling:\n  gate:\n    distributed_monolith:\n      max_new_seams: -1\n",
			wantErr: "max_new_seams must be >= 0",
		},
		{
			name:    "config schema v1 rejected before retired fields decode",
			yaml:    "version: 1\ncoupling:\n  gate:\n    min_band: serviceable\n    max_drop: 5\n",
			wantErr: "Archfit skill manual migration reference (references/migration.md)",
		},
		{
			name:    "retired scalar gate in v2 points to manual migration",
			yaml:    "version: 2\ncoupling:\n  gate:\n    min_band: serviceable\n",
			wantErr: "references/migration.md",
		},
		{
			name:    "config schema newer than this binary rejected",
			yaml:    "version: 3\n",
			wantErr: "newer than this binary understands",
		},
		{
			name:    "unknown coupling.gate key rejected at decode",
			yaml:    "version: 2\ncoupling:\n  gate:\n    band_floor: mixed\n",
			wantErr: "band_floor",
		},
		{
			name:    "metrics.coupling_balance points at coupling.gate",
			yaml:    "version: 2\nmetrics:\n  coupling_balance:\n    enabled: true\n",
			wantErr: "coupling.gate",
		},
		{
			name:    "valid rule gates",
			yaml:    "version: 2\nrules:\n  - id: r1\n    type: cycle\n    gate: fail\n  - id: r2\n    type: cycle\n    gate: warn\n",
			wantErr: "",
		},
		{
			name:    "empty rule gate is allowed",
			yaml:    "version: 2\nrules:\n  - id: r1\n    type: cycle\n",
			wantErr: "",
		},
		{
			name:    "invalid rule gate names the rule id",
			yaml:    "version: 2\nrules:\n  - id: nocycle\n    type: cycle\n    gate: block\n",
			wantErr: "rules[nocycle]",
		},
		{
			name:    "missing rule id rejected",
			yaml:    "version: 2\nrules:\n  - type: cycle\n    gate: block\n",
			wantErr: "rules[#0].id is required",
		},
		{
			name:    "empty rule id rejected",
			yaml:    "version: 2\nrules:\n  - id: \"\"\n    type: cycle\n",
			wantErr: "rules[#0].id is required",
		},
		{
			name:    "pattern entry missing rule rejected",
			yaml:    "version: 2\nrules:\n  - id: r1\n    type: cycle\n    patterns:\n      - id: p1\n        lang: go\n",
			wantErr: "rules[r1].patterns[0]",
		},
		{
			name:    "pattern entry missing lang rejected",
			yaml:    "version: 2\nrules:\n  - id: r1\n    type: cycle\n    patterns:\n      - id: p1\n        rule: unsafe.Pointer($X)\n",
			wantErr: "rules[r1].patterns[0]",
		},
		{
			name:    "complete pattern entry loads clean",
			yaml:    "version: 2\nrules:\n  - id: r1\n    type: cycle\n    patterns:\n      - id: p1\n        lang: go\n        rule: unsafe.Pointer($X)\n",
			wantErr: "",
		},
		{
			name:    "invalid metric gate names the metric",
			yaml:    "version: 2\nmetrics:\n  cycle:\n    enabled: true\n    gate: nope\n",
			wantErr: "metrics.cycle",
		},
		{
			name:    "metric knobs matching the metric kind load clean",
			yaml:    "version: 2\nmetrics:\n  cycle:\n    enabled: true\n    gate: fail\n    max_new: 2\n  encapsulation:\n    enabled: true\n    gate: warn\n    min_delta: 0.05\n",
			wantErr: "",
		},
		{
			name:    "negative min_delta rejected",
			yaml:    "version: 2\nmetrics:\n  encapsulation:\n    enabled: true\n    min_delta: -0.1\n",
			wantErr: "metrics.encapsulation.min_delta must be >= 0",
		},
		{
			name:    "negative max_new rejected",
			yaml:    "version: 2\nmetrics:\n  cycle:\n    enabled: true\n    max_new: -1\n",
			wantErr: "metrics.cycle.max_new must be >= 0",
		},
		{
			name:    "max_new on a ratio metric rejected",
			yaml:    "version: 2\nmetrics:\n  encapsulation:\n    enabled: true\n    max_new: 1\n",
			wantErr: "metrics.encapsulation.max_new applies only to count metrics",
		},
		{
			name:    "min_delta on a count metric rejected",
			yaml:    "version: 2\nmetrics:\n  cycle:\n    enabled: true\n    min_delta: 0.1\n",
			wantErr: "metrics.cycle.min_delta applies only to ratio metrics",
		},
		{
			name:    "zero max_new on a ratio metric still rejected",
			yaml:    "version: 2\nmetrics:\n  encapsulation:\n    enabled: true\n    max_new: 0\n",
			wantErr: "metrics.encapsulation.max_new applies only to count metrics",
		},
		{
			name:    "zero min_delta on a count metric still rejected",
			yaml:    "version: 2\nmetrics:\n  cycle:\n    enabled: true\n    min_delta: 0\n",
			wantErr: "metrics.cycle.min_delta applies only to ratio metrics",
		},
		{
			name:    "gate on informational blast_radius rejected",
			yaml:    "version: 2\nmetrics:\n  blast_radius:\n    enabled: true\n    gate: warn\n",
			wantErr: errBlastRadiusInformational,
		},
		{
			name:    "zero threshold on informational blast_radius rejected",
			yaml:    "version: 2\nmetrics:\n  blast_radius:\n    enabled: true\n    max_new: 0\n",
			wantErr: errBlastRadiusInformational,
		},
		{
			name:    "min_delta on informational blast_radius rejected",
			yaml:    "version: 2\nmetrics:\n  blast_radius:\n    enabled: true\n    min_delta: 0.1\n",
			wantErr: errBlastRadiusInformational,
		},
		{
			name:    "enabled toggle on blast_radius is allowed",
			yaml:    "version: 2\nmetrics:\n  blast_radius:\n    enabled: false\n",
			wantErr: "",
		},
		{
			name:    "removed max_new_high field rejected at decode",
			yaml:    "version: 2\nmetrics:\n  unbalanced_edge:\n    enabled: true\n    max_new_high: 0\n",
			wantErr: "max_new_high",
		},
		{
			name:    "invalid module_review gate",
			yaml:    "version: 2\nmodule_review:\n  gate: maybe\n",
			wantErr: "module_review",
		},
		{
			name:    "valid language gate",
			yaml:    "version: 2\nlanguages:\n  go:\n    enabled: auto\n    gate: fail\n",
			wantErr: "",
		},
		{
			name:    "empty language gate is allowed",
			yaml:    "version: 2\nlanguages:\n  go:\n    enabled: auto\n",
			wantErr: "",
		},
		{
			name:    "invalid language gate names the language",
			yaml:    "version: 2\nlanguages:\n  go:\n    enabled: auto\n    gate: block\n",
			wantErr: "languages.go",
		},
		{
			name:    "off is a valid gate",
			yaml:    "version: 2\nmodule_review:\n  gate: off\n",
			wantErr: "",
		},
		{
			name:    "invalid stale_after duration",
			yaml:    "version: 2\nmodule_review:\n  stale_after: \"180 days\"\n",
			wantErr: "module_review.stale_after",
		},
		{
			name:    "valid stale_after duration",
			yaml:    "version: 2\nmodule_review:\n  stale_after: 720h\n",
			wantErr: "",
		},
		{
			name:    "invalid analyzer timeout rejected",
			yaml:    "version: 2\nanalyzers:\n  scip:\n    timeout: \"5min\"\n",
			wantErr: "analyzers.scip.timeout",
		},
		{
			name:    "valid analyzer timeout accepted",
			yaml:    "version: 2\nanalyzers:\n  scip:\n    timeout: \"5m\"\n",
			wantErr: "",
		},
		{
			name:    "valid module role",
			yaml:    "version: 2\nmodules:\n  cmd:\n    paths: [\"cmd/**\"]\n    role: composition_root\n",
			wantErr: "",
		},
		{
			name:    "empty module role is allowed",
			yaml:    "version: 2\nmodules:\n  cmd:\n    paths: [\"cmd/**\"]\n",
			wantErr: "",
		},
		{
			name:    "invalid module role names the module",
			yaml:    "version: 2\nmodules:\n  cmd:\n    paths: [\"cmd/**\"]\n    role: wiring\n",
			wantErr: "modules.cmd.role",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			tmp := filepath.Join(t.TempDir(), "cfg.yaml")
			if err := writeFile(tmp, tc.yaml); err != nil {
				t.Fatalf("write temp: %v", err)
			}
			_, err := config.Load(context.Background(), tmp)
			if tc.wantErr == "" {
				if err != nil {
					t.Fatalf("Load: unexpected error: %v", err)
				}
				return
			}
			if err == nil {
				t.Fatalf("Load: expected error mentioning %q, got nil", tc.wantErr)
			}
			if !strings.Contains(err.Error(), tc.wantErr) {
				t.Errorf("error %q does not mention %q", err.Error(), tc.wantErr)
			}
		})
	}
}

func TestForClassify_DuplicatedKnowledgePolicy(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		yaml string
		want policy.DuplicatedKnowledgePolicy
	}{
		{
			name: "omitted defaults to score",
			yaml: yamlV1,
			want: policy.DuplicatedKnowledgePolicyScore,
		},
		{
			name: "score preserved",
			yaml: "version: 2\ncoupling:\n  duplicated_knowledge: score\n",
			want: policy.DuplicatedKnowledgePolicyScore,
		},
		{
			name: "advisory preserved",
			yaml: "version: 2\ncoupling:\n  duplicated_knowledge: advisory\n",
			want: policy.DuplicatedKnowledgePolicyAdvisory,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			path := filepath.Join(t.TempDir(), "cfg.yaml")
			if err := writeFile(path, tt.yaml); err != nil {
				t.Fatalf("write temp: %v", err)
			}
			cfg, err := config.Load(context.Background(), path)
			if err != nil {
				t.Fatalf("Load: %v", err)
			}
			if got := cfg.ForClassify().DuplicatedKnowledgePolicy; got != tt.want {
				t.Errorf("ForClassify().DuplicatedKnowledgePolicy = %q, want %q", got, tt.want)
			}
		})
	}
}

// TestLoad_ToolGate verifies the languages.<x>.gate field parses into the typed
// GateMode and that an omitted gate is the empty value (callers default it to warn).
func TestLoad_ToolGate(t *testing.T) {
	tmp := filepath.Join(t.TempDir(), "cfg.yaml")
	yaml := "version: 2\n" +
		"languages:\n" +
		"  go:\n" +
		"    enabled: auto\n" +
		"    gate: fail\n" +
		"  typescript:\n" +
		"    enabled: auto\n"
	if err := writeFile(tmp, yaml); err != nil {
		t.Fatalf("write temp: %v", err)
	}
	cfg, err := config.Load(context.Background(), tmp)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if got := cfg.ToolGate("go"); got != config.GateFail {
		t.Errorf("languages.go.gate = %q, want %q", got, config.GateFail)
	}
	if got := cfg.ToolGate("typescript"); got != "" {
		t.Errorf("languages.typescript.gate = %q, want \"\" (unset)", got)
	}
}

// Config-quality lint test constants (kept out of literals for goconst).
const (
	lintPath  = "a/**"
	lintOwner = "owner"
	lintVol   = "subdomain/volatility"
	lintTeam  = "team-a"

	// Layer name constants used in self-config conformance tests.
	layerAdapter = "adapter"
	layerCore    = "core"
	layerEngine  = "engine"
	layerModel   = "model"
	layerSupport = "support"

	// langTypeScript and yamlV1 appear in many tests; kept as constants to
	// satisfy the goconst linter.
	langTypeScript = "typescript"
	yamlV1         = "version: 2\n"
)

func TestLint(t *testing.T) {
	tests := []struct {
		name string
		mod  policy.ModuleDef
		want []string // expected Missing tokens; nil = no warning for this module
	}{
		{
			name: "fully specified",
			mod:  policy.ModuleDef{Paths: []string{lintPath}, Owner: lintTeam, Subdomain: layerCore, Volatility: "high"},
			want: nil,
		},
		{
			name: "missing owner only",
			mod:  policy.ModuleDef{Paths: []string{lintPath}, Subdomain: layerCore},
			want: []string{lintOwner},
		},
		{
			name: "missing subdomain and volatility only",
			mod:  policy.ModuleDef{Paths: []string{lintPath}, Owner: lintTeam},
			want: []string{lintVol},
		},
		{
			name: "missing all three",
			mod:  policy.ModuleDef{Paths: []string{lintPath}},
			want: []string{lintOwner, lintVol},
		},
		{
			name: "subdomain alone resolves volatility",
			mod:  policy.ModuleDef{Paths: []string{lintPath}, Owner: lintTeam, Subdomain: "generic"},
			want: nil,
		},
		{
			name: "volatility alone resolves volatility",
			mod:  policy.ModuleDef{Paths: []string{lintPath}, Owner: lintTeam, Volatility: "low"},
			want: nil,
		},
		{
			name: "pathless module is not linted",
			mod:  policy.ModuleDef{Owner: ""}, // no paths → classifies nothing
			want: nil,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			cfg := config.Config{Modules: map[string]policy.ModuleDef{"m": tc.mod}}
			got := cfg.Lint()
			if tc.want == nil {
				if len(got) != 0 {
					t.Fatalf("Lint() = %v, want no warnings", got)
				}
				return
			}
			if len(got) != 1 {
				t.Fatalf("Lint() = %v, want exactly one warning", got)
			}
			if got[0].Module != "m" {
				t.Errorf("module = %q, want %q", got[0].Module, "m")
			}
			if !slices.Equal(got[0].Missing, tc.want) {
				t.Errorf("Missing = %v, want %v (order is fixed)", got[0].Missing, tc.want)
			}
		})
	}
}

func TestLint_DeterministicModuleOrder(t *testing.T) {
	// Map iteration is random; Lint must return modules in sorted name order.
	bare := policy.ModuleDef{Paths: []string{"x/**"}}
	cfg := config.Config{Modules: map[string]policy.ModuleDef{
		"mod-z": bare,
		"mod-a": bare,
		"mod-m": bare,
		"ok":    {Paths: []string{"o/**"}, Owner: "t", Volatility: "low"}, // no warning
	}}

	first := cfg.Lint()
	gotModules := make([]string, len(first))
	for i, w := range first {
		gotModules[i] = w.Module
	}
	wantModules := []string{"mod-a", "mod-m", "mod-z"}
	if !slices.Equal(gotModules, wantModules) {
		t.Fatalf("module order = %v, want %v", gotModules, wantModules)
	}

	// Stable across repeated calls (no map-order leakage).
	for range 20 {
		again := cfg.Lint()
		for i := range again {
			if again[i].Module != first[i].Module || !slices.Equal(again[i].Missing, first[i].Missing) {
				t.Fatalf("Lint() not deterministic: %v vs %v", again, first)
			}
		}
	}
}

func TestLintWarning_String(t *testing.T) {
	w := config.LintWarning{Module: "billing", Missing: []string{lintOwner, lintVol}}
	got := w.String()
	want := `module "billing" omits owner, subdomain/volatility`
	if got != want {
		t.Errorf("String() = %q, want %q", got, want)
	}
}

// ---------------------------------------------------------------------------
// analyzers.syntax — SyntaxEnabled + ForSyntax
// ---------------------------------------------------------------------------

func TestSyntaxEnabled(t *testing.T) {
	cases := []struct {
		name string
		mode string
		want bool
	}{
		{"true enables", "true", true},
		{"false disables", "false", false},
		{"auto disables (opt-in only)", "auto", false},
		{"absent disables", "", false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var yamlBody string
			if tc.mode == "" {
				yamlBody = "version: 2\n"
			} else {
				yamlBody = "version: 2\nanalyzers:\n  syntax:\n    enabled: " + tc.mode + "\n"
			}
			dir := t.TempDir()
			path := filepath.Join(dir, ".archfit.yaml")
			if err := writeFile(path, yamlBody); err != nil {
				t.Fatalf("writeFile: %v", err)
			}
			cfg, err := config.Load(context.Background(), path)
			if err != nil {
				t.Fatalf("Load: %v", err)
			}
			if got := cfg.SyntaxEnabled(); got != tc.want {
				t.Errorf("SyntaxEnabled() = %v, want %v (mode=%q)", got, tc.want, tc.mode)
			}
		})
	}
}

func TestForSyntax_Mode(t *testing.T) {
	cases := []struct {
		name    string
		yaml    string
		enabled bool
	}{
		{
			name:    "enabled when true",
			yaml:    "version: 2\nanalyzers:\n  syntax:\n    enabled: true\n",
			enabled: true,
		},
		{
			name:    "disabled when auto",
			yaml:    "version: 2\nanalyzers:\n  syntax:\n    enabled: auto\n",
			enabled: false,
		},
		{
			name:    "disabled when false",
			yaml:    "version: 2\nanalyzers:\n  syntax:\n    enabled: false\n",
			enabled: false,
		},
		{
			name:    "disabled when absent",
			yaml:    "version: 2\n",
			enabled: false,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			path := filepath.Join(dir, ".archfit.yaml")
			if err := writeFile(path, tc.yaml); err != nil {
				t.Fatalf("writeFile: %v", err)
			}
			cfg, err := config.Load(context.Background(), path)
			if err != nil {
				t.Fatalf("Load: %v", err)
			}
			sc := cfg.ForSyntax()
			if sc.Enabled != tc.enabled {
				t.Errorf("ForSyntax().Enabled = %v, want %v", sc.Enabled, tc.enabled)
			}
		})
	}
}

func TestForSyntax_Languages(t *testing.T) {
	allFour := []string{"go", "typescript", "python", "rust"}

	t.Run("all four when no tools configured", func(t *testing.T) {
		dir := t.TempDir()
		path := filepath.Join(dir, ".archfit.yaml")
		if err := writeFile(path, "version: 2\n"); err != nil {
			t.Fatalf("writeFile: %v", err)
		}
		cfg, _ := config.Load(context.Background(), path)
		sc := cfg.ForSyntax()
		if !slices.Equal(sc.Languages, allFour) {
			t.Errorf("Languages = %v, want %v", sc.Languages, allFour)
		}
	})

	t.Run("excludes language set to off", func(t *testing.T) {
		yaml := "version: 2\nlanguages:\n  rust:\n    enabled: false\n"
		dir := t.TempDir()
		path := filepath.Join(dir, ".archfit.yaml")
		if err := writeFile(path, yaml); err != nil {
			t.Fatalf("writeFile: %v", err)
		}
		cfg, _ := config.Load(context.Background(), path)
		sc := cfg.ForSyntax()
		want := []string{"go", "typescript", "python"}
		if !slices.Equal(sc.Languages, want) {
			t.Errorf("Languages = %v, want %v", sc.Languages, want)
		}
	})

	t.Run("includes language set to auto", func(t *testing.T) {
		yaml := "version: 2\nlanguages:\n  python:\n    enabled: auto\n"
		dir := t.TempDir()
		path := filepath.Join(dir, ".archfit.yaml")
		if err := writeFile(path, yaml); err != nil {
			t.Fatalf("writeFile: %v", err)
		}
		cfg, _ := config.Load(context.Background(), path)
		sc := cfg.ForSyntax()
		if !slices.Equal(sc.Languages, allFour) {
			t.Errorf("Languages = %v, want %v (auto should be included)", sc.Languages, allFour)
		}
	})

	t.Run("all off yields empty languages", func(t *testing.T) {
		yaml := "version: 2\nlanguages:\n  go:\n    enabled: false\n  typescript:\n    enabled: false\n  python:\n    enabled: false\n  rust:\n    enabled: false\n"
		dir := t.TempDir()
		path := filepath.Join(dir, ".archfit.yaml")
		if err := writeFile(path, yaml); err != nil {
			t.Fatalf("writeFile: %v", err)
		}
		cfg, _ := config.Load(context.Background(), path)
		sc := cfg.ForSyntax()
		if len(sc.Languages) != 0 {
			t.Errorf("Languages = %v, want empty", sc.Languages)
		}
	})
}

// writeFile is a test helper that writes content to path.
func writeFile(path, content string) error {
	return os.WriteFile(path, []byte(content), 0o600)
}
