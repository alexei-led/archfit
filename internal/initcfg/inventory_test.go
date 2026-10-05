package initcfg

import (
	"context"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

// Module names the inventory tests discover.
const (
	invModAPI      = "api"
	invModHandlers = "handlers"
	invModUtil     = "util"
	invModWebUI    = "web_ui"
	invModTSDB     = "tsdb"
	invCrateYazi   = "yazi_shared"
)

func goSource(path string, production bool) SourceFile {
	return SourceFile{Path: path, Language: langGo, Selector: filepath.Dir(path), Production: production}
}

func TestKeepModulesWithSource(t *testing.T) {
	mods := []ModuleDef{
		{Name: "chaos", Paths: []string{"pkg/chaos/**"}, Public: []string{"pkg/chaos"}},
		{Name: "mocks", Paths: []string{"mocks/**"}, Public: []string{"mocks"}},
		{Name: "gen", Paths: []string{"api/gen/**"}, Public: []string{"api/gen"}},
		{Name: "e2e", Paths: []string{"test/e2e/**"}},
		{Name: layerCmd, Paths: []string{"cmd/**"}, Public: []string{layerCmd}},
		{Name: "cmd_tool", Paths: []string{"cmd/tool/**"}, Public: []string{"cmd/tool"}},
		{Name: invModUtil, Paths: []string{"pkg/util/**"}, Public: []string{"pkg/util"}},
		{Name: invCrateYazi, Paths: []string{invCrateYazi}},
	}
	origins := []string{langGo, langGo, langGo, langGo, langGo, langGo, langGo, langRust}
	sources := []SourceFile{
		goSource("pkg/chaos/chaos.go", true),
		// mocks/ is skipped by the LOC walk: no inventory file at all.
		goSource("api/gen/api.pb.go", false),
		goSource("test/e2e/e2e_test.go", false),
		// cmd has only a test file of its own; its production code sits in
		// cmd/tool, which the more specific module owns.
		goSource("cmd/cmd_test.go", false),
		goSource("cmd/tool/main.go", true),
		// util's root package is test-only; production code is one level down,
		// so the module stays but its root public entry names no production node.
		goSource("pkg/util/util_test.go", false),
		goSource("pkg/util/strs/strs.go", true),
		// Rust files carry no selector without cargo metadata.
		{Path: "yazi-shared/src/lib.rs", Language: langRust, Production: true},
	}

	kept, keptOrigins := keepModulesWithSource(mods, origins, 7, sources)

	want := []ModuleDef{
		{Name: "chaos", Paths: []string{"pkg/chaos/**"}, Public: []string{"pkg/chaos"}},
		{Name: "cmd_tool", Paths: []string{"cmd/tool/**"}, Public: []string{"cmd/tool"}},
		{Name: invModUtil, Paths: []string{"pkg/util/**"}},
		{Name: invCrateYazi, Paths: []string{invCrateYazi}},
	}
	if !reflect.DeepEqual(kept, want) {
		t.Errorf("kept modules:\n got %+v\nwant %+v", kept, want)
	}
	if !reflect.DeepEqual(keptOrigins, []string{langGo, langGo, langGo, langRust}) {
		t.Errorf("kept origins = %v", keptOrigins)
	}
}

// TestKeepModulesWithSource_PerLanguageSelectors: ownership reads each
// language's vocabulary — a TypeScript file path, a dotted Python module.
func TestKeepModulesWithSource_PerLanguageSelectors(t *testing.T) {
	mods := []ModuleDef{
		{Name: invModAPI, Paths: []string{"src/api/**"}, Public: []string{"src/api/**"}},
		{Name: "assets", Paths: []string{"src/assets/**"}, Public: []string{"src/assets/**"}},
		{Name: "ts_mocks", Paths: []string{"src/__mocks__/**"}, Public: []string{"src/__mocks__/**"}},
		{Name: invModHandlers, Paths: []string{"app.handlers", "app.handlers.*"}},
		{Name: "tests", Paths: []string{"app.tests", "app.tests.*"}},
	}
	origins := []string{langTypeScript, langTypeScript, langTypeScript, langPython, langPython}
	sources := []SourceFile{
		{Path: "src/api/index.ts", Language: langTypeScript, Selector: "src/api/index.ts", Production: true},
		{Path: "src/__mocks__/api.ts", Language: langTypeScript, Selector: "src/__mocks__/api.ts"},
		{Path: "src/app/handlers/__init__.py", Language: langPython, Selector: "app.handlers", Production: true},
		{Path: "src/app/handlers/web.py", Language: langPython, Selector: "app.handlers.web", Production: true},
		{Path: "src/app/tests/test_web.py", Language: langPython, Selector: "app.tests.test_web"},
	}
	kept, _ := keepModulesWithSource(mods, origins, len(mods), sources)
	names := make([]string, 0, len(kept))
	for _, m := range kept {
		names = append(names, m.Name)
	}
	if strings.Join(names, ",") != "api,handlers" {
		t.Errorf("kept = %v, want [api handlers]", names)
	}
}

func TestGraphGap(t *testing.T) {
	webUI := ModuleDef{Name: invModWebUI, Paths: []string{"web/ui/**"}}
	tsdb := ModuleDef{Name: invModTSDB, Paths: []string{"tsdb/**"}}
	goOnly := []SourceFile{goSource("tsdb/db.go", true), goSource("web/ui/ui.go", true)}
	withTS := append(append([]SourceFile(nil), goOnly...),
		SourceFile{Path: "web/ui/app/src/main.tsx", Language: langTypeScript, Selector: "web/ui/app/src/main.tsx", Production: true})
	tests := []struct {
		name    string
		mods    []ModuleDef
		origins []string
		rust    bool
		sources []SourceFile
		want    string
	}{
		{name: "go only", mods: []ModuleDef{tsdb, webUI}, origins: []string{langGo, langGo}, sources: goOnly},
		{name: "go module owns typescript", mods: []ModuleDef{tsdb, webUI}, origins: []string{langGo, langGo}, sources: withTS,
			want: "Go module(s) web_ui also own typescript source the Go import graph omits"},
		{name: "python modules", mods: []ModuleDef{{Name: layerCore}}, origins: []string{langPython}, want: gapPython},
		{name: "typescript modules", mods: []ModuleDef{{Name: invModAPI}}, origins: []string{langTypeScript}, want: gapTypeScript},
		{name: "rust present", origins: nil, rust: true, want: gapRust},
		{name: "no inventory supplied", mods: []ModuleDef{tsdb, webUI}, origins: []string{langGo, langGo}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := graphGap(tt.mods, tt.origins, tt.rust, tt.sources); got != tt.want {
				t.Errorf("graphGap = %q, want %q", got, tt.want)
			}
		})
	}
}

// TestRender_GraphGapReason: a starter rule init cannot prove evaluable is
// written at gate: warn with the reason, not at gate: fail.
func TestRender_GraphGapReason(t *testing.T) {
	const gap = "Go module(s) web_ui also own typescript source the Go import graph omits"
	out := Render(DiscoveredConfig{
		Modules:  []ModuleDef{{Name: invModTSDB, Paths: []string{"tsdb/**"}}, {Name: invModWebUI, Paths: []string{"web/ui/**"}}},
		Edges:    []ModuleEdge{{From: invModTSDB, To: invModWebUI}, {From: invModWebUI, To: invModTSDB}},
		GraphGap: gap,
	}, nil, false)
	for _, want := range []string{
		"  # gate: warn — init saw no complete import graph.",
		"  # Why: " + gap + ".\n  # The partial graph already shows 1 module cycle(s).\n" +
			"  - id: no-module-cycles\n    type: module_cycle\n    gate: warn\n",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("rendered config missing %q:\n%s", want, out)
		}
	}
}

// TestDiscoverPy_GuessesNoLayer: sub-package names (handlers, providers,
// models, cli) do not prove a layer, so Python discovery assigns none and
// Render writes the layers how-to with the direction rule commented.
func TestDiscoverPy_GuessesNoLayer(t *testing.T) {
	root := t.TempDir()
	write := func(rel string) {
		path := filepath.Join(root, rel)
		if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, nil, 0o600); err != nil {
			t.Fatal(err)
		}
	}
	write("pyproject.toml")
	for _, sub := range []string{invModHandlers, "providers", "models", tierCLI, layerEngine} {
		write("src/app/" + sub + "/__init__.py")
	}
	write("src/app/__init__.py")

	cfg, err := Discover(t.Context(), root, nil, Presence{Python: true})
	if err != nil {
		t.Fatal(err)
	}
	if len(cfg.Modules) != 5 {
		t.Fatalf("modules = %+v, want 5", cfg.Modules)
	}
	for _, m := range cfg.Modules {
		if m.Layer != "" {
			t.Errorf("module %s layer = %q, want none", m.Name, m.Layer)
		}
	}
	if cfg.Layers != nil {
		t.Errorf("layers = %v, want none", cfg.Layers)
	}
	out := Render(cfg, nil, false)
	if strings.Contains(out, "\nlayers:\n") || !strings.Contains(out, "# layers: not inferred") ||
		!strings.Contains(out, "  # - id: no-layer-back-edges\n") {
		t.Errorf("want the commented layers how-to and no live layer rule:\n%s", out)
	}
}

// TestDiscover_DroppedModuleEdgesFoldIntoTheOwningAncestor: api/v1 holds only
// generated code, so init drops it, but the written api/** glob still owns its
// package. check sees server -> api/v1 as server -> api, so init must count
// the api <-> server cycle the same way instead of writing gate: fail.
func TestDiscover_DroppedModuleEdgesFoldIntoTheOwningAncestor(t *testing.T) {
	root := t.TempDir()
	writeGoMod(t, root)
	runner := mockRunner(`{"ImportPath":"github.com/example/myapp/api","Imports":["github.com/example/myapp/internal/server"],"Module":{"Path":"github.com/example/myapp"}}
{"ImportPath":"github.com/example/myapp/api/v1","Imports":["github.com/example/myapp/api"],"Module":{"Path":"github.com/example/myapp"}}
{"ImportPath":"github.com/example/myapp/internal/server","Imports":["github.com/example/myapp/api/v1"],"Module":{"Path":"github.com/example/myapp"}}`)
	presence := Presence{Go: true, GoMembers: []string{root}, Sources: []SourceFile{
		goSource("api/types.go", true),
		goSource("api/v1/v1.pb.go", false),
		goSource("internal/server/server.go", true),
	}}
	cfg, err := Discover(context.Background(), root, runner, presence)
	if err != nil {
		t.Fatalf("Discover: %v", err)
	}
	want := []ModuleEdge{{From: invModAPI, To: "server"}, {From: "server", To: invModAPI}}
	if !reflect.DeepEqual(cfg.Edges, want) {
		t.Errorf("edges = %+v, want %+v", cfg.Edges, want)
	}
	if out := Render(cfg, nil, false); !strings.Contains(out, "gate: warn — 1 current module cycle(s) at init") {
		t.Errorf("module_cycle gate should count the api <-> server cycle:\n%s", out)
	}
}
