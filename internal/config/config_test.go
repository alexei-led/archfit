package config_test

import (
	"os"
	"slices"
	"testing"

	"github.com/alexei-led/archfit/v3/internal/config"
	"github.com/alexei-led/archfit/v3/internal/config/policyfile"
	evidenceports "github.com/alexei-led/archfit/v3/internal/evidence/ports"
	"github.com/alexei-led/archfit/v3/internal/policy"
	"github.com/alexei-led/archfit/v3/internal/policy/resolved"
)

const (
	moduleCheckout   = "checkout"
	layerApplication = "application"
	modulePricing    = "pricing"
	lintPath         = "a/**"
	lintOwner        = "owner"
	lintVol          = "subdomain/volatility"
	lintTeam         = "team-a"
	layerAdapter     = "adapter"
	layerCore        = "core"
	globSvcAll       = "svc/**"
	rustManifestPath = "crates/app/Cargo.toml"
)

var rustFeatures = []string{"a", "b"}

func sampleConfig() config.Config {
	cfg := config.Default()
	cfg.Languages.Go.Enabled = config.ModeOn
	cfg.Modules = map[string]policy.ModuleDef{
		moduleCheckout: {Paths: []string{"services/checkout/**"}, Layer: layerApplication, Owner: "team-checkout"},
		modulePricing:  {Paths: []string{"services/pricing/**"}, Public: []string{"services/pricing/contracts/**"}, Internal: []string{"services/pricing/internal/**"}, Layer: "domain", Owner: "team-pricing"},
	}
	cfg.Layers = []string{"domain", layerApplication, "infrastructure"}
	cfg.Exclude = []string{"**/generated/**", "**/*.pb.go"}
	enabled := true
	cfg.Metrics.Entries = map[string]policy.MetricEntry{"encapsulation": {Enabled: &enabled, Gate: "warn"}}
	return cfg
}

func TestModuleFor(t *testing.T) {
	cfg := sampleConfig()
	mm := cfg.ModuleMapView()

	tests := []struct {
		path   string
		want   string
		wantOK bool
	}{
		{"services/checkout/handler.go", moduleCheckout, true},
		{"services/checkout/deep/nested/file.go", moduleCheckout, true},
		{"services/pricing/contracts/api.go", modulePricing, true},
		{"services/pricing/internal/impl.go", modulePricing, true},
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

func TestModuleFor_PythonDottedGlobs(t *testing.T) {
	const node = "prefect.states" // grimp-style dotted node ID (graph.NodePath form)
	const mod = "states"

	dottedCfg := config.Config{
		Modules: map[string]policy.ModuleDef{
			mod: {Paths: []string{"prefect.states", "prefect.states.**"}},
		},
	}
	dotted := dottedCfg.ModuleMapView()
	if got, ok := dotted.ModuleFor(node); !ok || got != mod {
		t.Errorf("dotted glob: ModuleFor(%q) = (%q,%v), want (%s,true)", node, got, ok, mod)
	}

	slashCfg := config.Config{
		Modules: map[string]policy.ModuleDef{
			mod: {Paths: []string{"src/prefect/states/**", "src/prefect/states.py"}},
		},
	}
	slash := slashCfg.ModuleMapView()
	if got, ok := slash.ModuleFor(node); ok {
		t.Errorf("slash glob: ModuleFor(%q) = (%q,true), want no-match — Python globs must be dotted module form, not file paths", node, got)
	}
}

func TestModuleFor_ConsumerConsistency(t *testing.T) {
	// Mirrors testdata/fixture-py/.archfit.yaml's module "b".
	cfg := config.Config{
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

func TestModuleFor_MostSpecific(t *testing.T) {
	const catchAll = "internal" // broad fallback stanza; repeated below
	cfg := config.Config{
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
	cfg := sampleConfig()
	sc := cfg.ForScope()
	if len(sc.Exclusions) != len(cfg.Exclude) {
		t.Errorf("ForScope().Exclusions len=%d, want %d", len(sc.Exclusions), len(cfg.Exclude))
	}
}

func TestForExtract(t *testing.T) {
	cfg := sampleConfig()

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
		rc := config.Config{Languages: config.LanguagesConfig{Rust: config.RustLanguage{Manifest: "Cargo.toml", IncludeDevDeps: true}}}
		ec := rc.ForExtract("go")
		if ec.CargoManifest != "" || ec.CargoFeatures != nil || ec.IncludeDevDeps {
			t.Errorf("non-rust lang leaked rust fields: %+v", ec)
		}
	})

	t.Run("go_module_filter_propagated", func(t *testing.T) {
		// tools.go.modules.include/exclude must surface in ForExtract("go").
		gc := config.Config{
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

func TestForExtract_SrcNotModuleDerived(t *testing.T) {
	const webGlob = "packages/web/**"

	cfgAddonsFirst := config.Config{
		Modules: map[string]policy.ModuleDef{
			"addons": {Paths: []string{"addons/**"}},
			"web":    {Paths: []string{webGlob}},
		},
	}
	cfgWebFirst := config.Config{
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
}

func TestRoslynEnabledDefaultsOn(t *testing.T) {
	for _, tc := range []struct {
		body string
		want bool
	}{
		{"version: 3\nowners: [owner]\nmodules: {app: {paths: ['**']}}\n", true},
		{"version: 3\nowners: [owner]\nmodules: {app: {paths: ['**']}}\n" + "analyzers:\n  roslyn:\n    enabled: auto\n", true},
		{"version: 3\nowners: [owner]\nmodules: {app: {paths: ['**']}}\n" + "analyzers:\n  roslyn:\n    enabled: true\n", true},
		{"version: 3\nowners: [owner]\nmodules: {app: {paths: ['**']}}\n" + "analyzers:\n  roslyn:\n    enabled: false\n", false},
	} {
		cfg, err := config.ReportConfig([]byte(tc.body))
		if err != nil {
			t.Fatalf("Load %q: %v", tc.body, err)
		}
		if got := cfg.RoslynEnabled(); got != tc.want {
			t.Errorf("RoslynEnabled for %q = %v, want %v", tc.body, got, tc.want)
		}
	}
}

func TestSetToolModeReachesDotNetAndRoslyn(t *testing.T) {
	cfg := config.Default()
	if got := cfg.ToolMode(config.LangDotNet); got != evidenceports.ModeAuto {
		t.Fatalf("Default dotnet mode = %q, want auto", got)
	}
	for _, id := range []string{config.LangDotNet, config.ToolRoslyn} {
		cfg.SetToolMode(id, evidenceports.ModeOff)
		if got := cfg.ToolMode(id); got != evidenceports.ModeOff {
			t.Errorf("ToolMode(%q) after SetToolMode(off) = %q", id, got)
		}
	}
}

func TestForClassify(t *testing.T) {
	cfg := sampleConfig()
	cc := cfg.ForClassify()
	if len(cc.Modules) != len(cfg.Modules) {
		t.Errorf("ForClassify().Modules len=%d, want %d", len(cc.Modules), len(cfg.Modules))
	}
	if len(cc.Layers) != len(cfg.Layers) {
		t.Errorf("ForClassify().Layers len=%d, want %d", len(cc.Layers), len(cfg.Layers))
	}
}

func TestForMetric(t *testing.T) {
	cfg := sampleConfig()

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

func selfPolicy(t *testing.T) resolved.Policy {
	t.Helper()
	data, err := os.ReadFile("../../.archfit.yaml")
	if err != nil {
		t.Fatalf("read self-policy: %v", err)
	}
	p, err := policyfile.Resolve(data)
	if err != nil {
		t.Fatalf("resolve self-policy: %v", err)
	}
	return p
}

func TestLoad_SelfConfig(t *testing.T) {
	p := selfPolicy(t)
	if len(p.Modules) == 0 || len(p.Layers) == 0 {
		t.Fatal("self-policy must declare modules and layers")
	}
}

func TestSelfConfig_CapabilityModuleMap(t *testing.T) {
	p := selfPolicy(t)
	for _, tc := range []struct{ file, module, layer string }{
		{"internal/model/evidence/evidence.go", "evidence-contracts", "model"},
		{"internal/scope/scope.go", "analysis-scope", "support"},
		{"internal/syntax/fileclass.go", "evidence-analysis", layerCore},
		{"internal/extract/golang/cache.go", "evidence-adapters", layerAdapter},
		{"internal/history/git/added_lines.go", "persistence-adapters", layerAdapter},
		{"internal/ownership/ownership.go", "persistence-adapters", layerAdapter},
		{"internal/output/reporttext/reporttext.go", "report-adapters", layerAdapter},
		{"internal/model/report/document.go", "report-contract", "model"},
		{"internal/policy/module.go", "architecture-policy", layerCore},
		{"internal/relationship/relationship.go", "relationship-analysis", layerCore},
		{"internal/assessment/evaluation/evaluation.go", "assessment-repair", layerCore},
		{"internal/application/analysis.go", "analysis-application", layerApplication},
		{"internal/evidence/acquisition/service.go", "evidence-acquisition", layerAdapter},
		{"cmd/archfit/main.go", "cli-composition", "cmd"},
	} {
		t.Run(tc.file, func(t *testing.T) {
			name, ok := p.ModuleOf(tc.file)
			if !ok || name != tc.module {
				t.Fatalf("owner of %s = %q, %v; want %q", tc.file, name, ok, tc.module)
			}
			if got := p.Modules[name].Layer; got != tc.layer {
				t.Errorf("layer of %s = %q, want %q", name, got, tc.layer)
			}
		})
	}
}

func TestSelfConfig_RoleLayerConformance(t *testing.T) {
	p := selfPolicy(t)
	used := make(map[string]bool)
	for name, module := range p.Modules {
		if !slices.Contains(p.Layers, module.Layer) {
			t.Errorf("module %s has undeclared layer %q", name, module.Layer)
		}
		used[module.Layer] = true
	}
	for _, layer := range p.Layers {
		if !used[layer] {
			t.Errorf("layer %q has no modules", layer)
		}
	}
}

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
