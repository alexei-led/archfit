package initcfg

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/alexei-led/archfit/internal/config"
	"github.com/alexei-led/archfit/internal/toolrun"
)

// Test-local constants to satisfy goconst across the test file.
const (
	testModuleName   = "utils"
	testCmdArchfit   = "cmd/archfit"
	testCmdMyapp     = "cmd_myapp"
	testModPath      = "github.com/foo/bar"
	testModPathChild = "github.com/foo/bar/pkg/a"
	testDirPerm      = 0o750
	testCorePath     = "internal/core/**"
	testModelPath    = "internal/model/**"
	testTSCore       = "core"
	testExampleMod   = "example.com/test"
	testClassifyPath = "internal/classify/**"
	testGeneric      = "generic"
	testNonexistent  = "nonexistent"
)

// sampleGoListJSON is concatenated go list -json output for a small module.
// go list emits one JSON object per package, not an array.
const sampleGoListJSON = `{
  "ImportPath": "github.com/example/myapp/cmd/myapp",
  "Dir": "/repo/cmd/myapp",
  "Module": {"Path": "github.com/example/myapp"}
}
{
  "ImportPath": "github.com/example/myapp/internal/model/graph",
  "Dir": "/repo/internal/model/graph",
  "Module": {"Path": "github.com/example/myapp"}
}
{
  "ImportPath": "github.com/example/myapp/internal/extract/golang",
  "Dir": "/repo/internal/extract/golang",
  "Module": {"Path": "github.com/example/myapp"}
}
{
  "ImportPath": "github.com/example/myapp/internal/engine",
  "Dir": "/repo/internal/engine",
  "Module": {"Path": "github.com/example/myapp"}
}
{
  "ImportPath": "github.com/example/myapp/internal/classify",
  "Dir": "/repo/internal/classify",
  "Module": {"Path": "github.com/example/myapp"}
}
`

func mockRunner(jsonOutput string) *toolrun.RunnerMock {
	return &toolrun.RunnerMock{
		RunFunc: func(_ context.Context, cmd toolrun.ToolCmd) (toolrun.Output, error) {
			if cmd.Name == "go" {
				return toolrun.Output{Stdout: []byte(jsonOutput)}, nil
			}
			return toolrun.Output{}, nil
		},
		DetectFunc: func(_ context.Context, _ string) (toolrun.ToolInfo, bool) {
			return toolrun.ToolInfo{}, false
		},
	}
}

// ---------------------------------------------------------------------------
// Discover
// ---------------------------------------------------------------------------

func writeGoMod(t *testing.T, root string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(root, "go.mod"), []byte("module github.com/example/myapp\ngo 1.21\n"), 0o600); err != nil {
		t.Fatal(err)
	}
}

func TestDiscover_GoList_GroupsModules(t *testing.T) {
	root := t.TempDir()
	writeGoMod(t, root)
	runner := mockRunner(sampleGoListJSON)
	cfg, err := Discover(context.Background(), root, runner, ProbePresence(root))
	if err != nil {
		t.Fatalf("Discover: %v", err)
	}

	if cfg.ModulePath != "github.com/example/myapp" {
		t.Errorf("ModulePath = %q, want %q", cfg.ModulePath, "github.com/example/myapp")
	}

	// Expect modules grouped by 2-segment key.
	wantNames := map[string]bool{
		testCmdMyapp:   true,
		layerModel:     true,
		adapterExtract: true,
		layerEngine:    true,
		testClassify:   true,
	}
	for _, m := range cfg.Modules {
		delete(wantNames, m.Name)
	}
	for name := range wantNames {
		t.Errorf("expected module %q not found in discovered modules", name)
	}
}

// TestDiscover_GoList_GuessesNoLayers pins that Go discovery assigns no layer
// from directory names. The retired table was keyed on archfit's own package
// names (model, extract, engine), so any other repo got domain, application and
// adapters in one layer and a direction rule that could never fire.
func TestDiscover_GoList_GuessesNoLayers(t *testing.T) {
	root := t.TempDir()
	writeGoMod(t, root)
	runner := mockRunner(sampleGoListJSON)
	cfg, err := Discover(context.Background(), root, runner, ProbePresence(root))
	if err != nil {
		t.Fatalf("Discover: %v", err)
	}
	if len(cfg.Layers) != 0 {
		t.Errorf("Layers = %v, want none", cfg.Layers)
	}
	for _, m := range cfg.Modules {
		if m.Layer != "" {
			t.Errorf("module %q: layer = %q, want none", m.Name, m.Layer)
		}
	}
}

func TestDiscover_GoListError_ReturnsError(t *testing.T) {
	root := t.TempDir()
	writeGoMod(t, root)
	runner := &toolrun.RunnerMock{
		RunFunc: func(_ context.Context, _ toolrun.ToolCmd) (toolrun.Output, error) {
			return toolrun.Output{ExitCode: 1, Stderr: []byte("no go files")}, nil
		},
		DetectFunc: func(_ context.Context, _ string) (toolrun.ToolInfo, bool) {
			return toolrun.ToolInfo{}, false
		},
	}
	_, err := Discover(context.Background(), root, runner, ProbePresence(root))
	if err == nil {
		t.Fatal("expected error for non-zero exit, got nil")
	}
}

func TestDiscover_MalformedJSON_ReturnsError(t *testing.T) {
	root := t.TempDir()
	writeGoMod(t, root)
	runner := mockRunner(`{not valid json}`)
	_, err := Discover(context.Background(), root, runner, ProbePresence(root))
	if err == nil {
		t.Fatal("expected error for malformed JSON, got nil")
	}
}

// ---------------------------------------------------------------------------
// DiscoverTS
// ---------------------------------------------------------------------------

func TestDiscoverTS_NoPackageJSON_ReturnsNil(t *testing.T) {
	root := t.TempDir()
	mods, err := DiscoverTS(root)
	if err != nil {
		t.Fatalf("DiscoverTS: %v", err)
	}
	if len(mods) != 0 {
		t.Errorf("expected no modules, got %v", mods)
	}
}

func TestDiscoverTS_WithSrcSubdirs(t *testing.T) {
	root := t.TempDir()
	// Write package.json marker.
	if err := os.WriteFile(filepath.Join(root, "package.json"), []byte(`{}`), 0o600); err != nil {
		t.Fatal(err)
	}
	// Create src/core and src/api subdirectories.
	for _, sub := range []string{"src/core", "src/api"} {
		if err := os.MkdirAll(filepath.Join(root, sub), testDirPerm); err != nil {
			t.Fatal(err)
		}
	}

	mods, err := DiscoverTS(root)
	if err != nil {
		t.Fatalf("DiscoverTS: %v", err)
	}

	names := make(map[string]bool, len(mods))
	for _, m := range mods {
		names[m.Name] = true
	}
	for _, want := range []string{testTSCore, "api"} {
		if !names[want] {
			t.Errorf("expected module %q, got modules: %v", want, mods)
		}
	}
}

func TestDiscoverTS_WithLibSubdirs(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "package.json"), []byte(`{}`), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(root, "lib/"+testModuleName), testDirPerm); err != nil {
		t.Fatal(err)
	}

	mods, err := DiscoverTS(root)
	if err != nil {
		t.Fatalf("DiscoverTS: %v", err)
	}
	if len(mods) != 1 || mods[0].Name != testModuleName {
		t.Errorf("expected module %q, got %v", testModuleName, mods)
	}
	// Path should reference lib/utils.
	if !strings.Contains(mods[0].Paths[0], "lib/"+testModuleName) {
		t.Errorf("expected path containing lib/%s, got %q", testModuleName, mods[0].Paths[0])
	}
}

func TestDiscoverTS_WorkspaceArrayForm(t *testing.T) {
	root := t.TempDir()
	pkg := `{"workspaces": ["packages/*", "apps/core"]}`
	if err := os.WriteFile(filepath.Join(root, "package.json"), []byte(pkg), 0o600); err != nil {
		t.Fatal(err)
	}
	for _, d := range []string{"packages/router", "packages/ui", "apps/core"} {
		if err := os.MkdirAll(filepath.Join(root, d), testDirPerm); err != nil {
			t.Fatal(err)
		}
	}

	mods, err := DiscoverTS(root)
	if err != nil {
		t.Fatalf("DiscoverTS: %v", err)
	}

	names := make(map[string]bool, len(mods))
	for _, m := range mods {
		names[m.Name] = true
	}
	for _, want := range []string{"router", "ui", "core"} {
		if !names[want] {
			t.Errorf("expected module %q; got %v", want, mods)
		}
	}
	// paths must use forward slashes
	for _, m := range mods {
		if strings.Contains(m.Paths[0], "\\") {
			t.Errorf("path has backslash: %q", m.Paths[0])
		}
	}
}

func TestDiscoverTS_WorkspaceObjectForm(t *testing.T) {
	root := t.TempDir()
	pkg := `{"workspaces": {"packages": ["code/addons/*", "code/core"]}}`
	if err := os.WriteFile(filepath.Join(root, "package.json"), []byte(pkg), 0o600); err != nil {
		t.Fatal(err)
	}
	for _, d := range []string{"code/addons/a11y", "code/addons/docs", "code/core"} {
		if err := os.MkdirAll(filepath.Join(root, d), testDirPerm); err != nil {
			t.Fatal(err)
		}
	}

	mods, err := DiscoverTS(root)
	if err != nil {
		t.Fatalf("DiscoverTS: %v", err)
	}

	names := make(map[string]bool, len(mods))
	for _, m := range mods {
		names[m.Name] = true
	}
	for _, want := range []string{"a11y", "docs", "core"} {
		if !names[want] {
			t.Errorf("expected module %q; got %v", want, mods)
		}
	}
}

func TestDiscoverTS_WorkspaceFallsBackToSrcWhenNoMatch(t *testing.T) {
	root := t.TempDir()
	// workspaces field present but the glob matches nothing
	pkg := `{"workspaces": ["packages/*"]}`
	if err := os.WriteFile(filepath.Join(root, "package.json"), []byte(pkg), 0o600); err != nil {
		t.Fatal(err)
	}
	// Only a src/ subdir exists — should fall back
	if err := os.MkdirAll(filepath.Join(root, "src/core"), testDirPerm); err != nil {
		t.Fatal(err)
	}

	mods, err := DiscoverTS(root)
	if err != nil {
		t.Fatalf("DiscoverTS: %v", err)
	}
	names := make(map[string]bool)
	for _, m := range mods {
		names[m.Name] = true
	}
	if !names[testTSCore] {
		t.Errorf("expected fallback module %q from src/; got %v", testTSCore, mods)
	}
}

// ---------------------------------------------------------------------------
// DiscoverPy
// ---------------------------------------------------------------------------

func TestDiscoverPy_NoMarker_ReturnsNil(t *testing.T) {
	root := t.TempDir()
	mods, err := DiscoverPy(root)
	if err != nil {
		t.Fatalf("DiscoverPy: %v", err)
	}
	if len(mods) != 0 {
		t.Errorf("expected no modules, got %v", mods)
	}
}

func TestDiscoverPy_WithPyprojectToml(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "pyproject.toml"), []byte(`[project]`), 0o600); err != nil {
		t.Fatal(err)
	}
	// Create two Python packages.
	for _, pkg := range []string{"myapp", testModuleName} {
		dir := filepath.Join(root, pkg)
		if err := os.MkdirAll(dir, testDirPerm); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir, "__init__.py"), []byte(""), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	// Non-package dir without __init__.py — should be ignored.
	if err := os.MkdirAll(filepath.Join(root, "docs"), testDirPerm); err != nil {
		t.Fatal(err)
	}

	mods, err := DiscoverPy(root)
	if err != nil {
		t.Fatalf("DiscoverPy: %v", err)
	}

	names := make(map[string]bool, len(mods))
	for _, m := range mods {
		names[m.Name] = true
	}
	for _, want := range []string{"myapp", testModuleName} {
		if !names[want] {
			t.Errorf("expected module %q, not found in %v", want, mods)
		}
	}
	if names["docs"] {
		t.Error("docs dir (no __init__.py) should not be a module")
	}
}

func TestDiscoverPy_WithSetupPy(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "setup.py"), []byte(""), 0o600); err != nil {
		t.Fatal(err)
	}
	dir := filepath.Join(root, "mylib")
	if err := os.MkdirAll(dir, testDirPerm); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "__init__.py"), []byte(""), 0o600); err != nil {
		t.Fatal(err)
	}

	mods, err := DiscoverPy(root)
	if err != nil {
		t.Fatalf("DiscoverPy: %v", err)
	}
	if len(mods) != 1 || mods[0].Name != "mylib" {
		t.Errorf("expected module 'mylib', got %v", mods)
	}
}

// ---------------------------------------------------------------------------
// Render
// ---------------------------------------------------------------------------

func TestRender_ContainsRequiredSections(t *testing.T) {
	cfg := DiscoveredConfig{
		ModulePath: "github.com/example/myapp",
		Modules: []ModuleDef{
			{Name: layerEngine, Paths: []string{"internal/engine/..."}, Layer: layerEngine},
			{Name: layerModel, Paths: []string{"internal/model/..."}, Layer: layerModel},
		},
		Layers: []string{layerModel, layerEngine},
	}
	out := Render(cfg, nil, false)

	checks := []struct {
		desc string
		want string
	}{
		{"version", "version: 2"},
		{"header comment", "# Generated by archfit config init"},
		{"layers section", "layers:"},
		{"model layer", "- " + layerModel},
		{"engine layer", "- " + layerEngine},
		{"modules section", "modules:"},
		{"engine module", layerEngine + ":"},
		{"model module", layerModel + ":"},
		{"rules section", "rules:"},
		{"module_cycle type", "type: module_cycle"},
		{"forbidden_layer_direction type", "type: forbidden_layer_direction"},
		{"gate warn", "gate: warn"},
	}
	for _, c := range checks {
		if !strings.Contains(out, c.want) {
			t.Errorf("Render output missing %s (want %q)\nfull output:\n%s", c.desc, c.want, out)
		}
	}
	if strings.Contains(out, "complexity: { enabled:") {
		t.Errorf("Render output suggests removed analyzers.complexity key:\n%s", out)
	}
}

func TestRender_NoModules_StillValid(t *testing.T) {
	cfg := DiscoveredConfig{ModulePath: "github.com/example/empty"}
	out := Render(cfg, nil, false)
	if !strings.Contains(out, "version: 2") {
		t.Errorf("expected version: 2 in output, got:\n%s", out)
	}
	if !strings.Contains(out, "rules:") {
		t.Errorf("expected rules: section in output, got:\n%s", out)
	}
}

func TestRender_LayeredRules_FromEdges(t *testing.T) {
	// Fixture: model←core (natural: core imports model) plus a back-edge
	// model→core (model imports core — a layer inversion).
	cfg := DiscoveredConfig{
		ModulePath: testExampleMod,
		Modules: []ModuleDef{
			{Name: layerModel, Paths: []string{testModelPath}, Layer: layerModel},
			{Name: layerCore, Paths: []string{testCorePath}, Layer: layerCore},
		},
		Layers: []string{layerModel, layerCore},
		// core imports model (natural), model imports core (back-edge / inversion).
		Edges: []ModuleEdge{
			{From: layerCore, To: layerModel},
			{From: layerModel, To: layerCore},
		},
	}
	out := Render(cfg, nil, false)

	// Must have >1 layer.
	if !strings.Contains(out, "- "+layerModel) || !strings.Contains(out, "- "+layerCore) {
		t.Fatalf("expected both layers in output:\n%s", out)
	}

	// Must contain at least one forbidden_layer_direction rule. from_layer/to_layer
	// are not emitted — forbiddenLayerDirection.Check derives layer ordering from
	// cfg.Layers and endpoint layers from the module map, never from a per-rule
	// from_layer/to_layer (see internal/rules/rules_dependency.go).
	if !strings.Contains(out, "type: forbidden_layer_direction") {
		t.Errorf("no forbidden_layer_direction rule in output:\n%s", out)
	}
	if strings.Contains(out, "from_layer:") || strings.Contains(out, "to_layer:") {
		t.Errorf("from_layer/to_layer should not be emitted (checker never reads them):\n%s", out)
	}
	if !strings.Contains(out, "gate: warn") {
		t.Errorf("gate: warn missing in output:\n%s", out)
	}

	// Exactly ONE rule: forbiddenLayerDirection.Check is global (each instance
	// re-detects every back-edge), so a second rule would duplicate findings.
	if !strings.Contains(out, "id: no-layer-back-edges") {
		t.Errorf("expected the single no-layer-back-edges rule:\n%s", out)
	}
	if n := strings.Count(out, "type: forbidden_layer_direction"); n != 1 {
		t.Errorf("got %d forbidden_layer_direction rules, want exactly 1:\n%s", n, out)
	}
}

func TestRender_LayeredRules_RoundTripsConfigLoad(t *testing.T) {
	cfg := DiscoveredConfig{
		ModulePath:          testExampleMod,
		HasGo:               true,
		ImportGraphComplete: true,
		Layers:              []string{layerModel, layerCore, layerAdapter, layerCmd},
		Modules: []ModuleDef{
			{Name: layerModel, Paths: []string{testModelPath}, Layer: layerModel},
			{Name: layerCore, Paths: []string{testCorePath}, Layer: layerCore},
			{Name: layerAdapter, Paths: []string{"internal/adapter/**"}, Layer: layerAdapter},
			{Name: testCmdMyapp, Paths: []string{"cmd/myapp/**"}, Layer: layerCmd},
		},
		// Natural: core→model, adapter→core, cmd→adapter.
		Edges: []ModuleEdge{
			{From: layerCore, To: layerModel},
			{From: layerAdapter, To: layerCore},
			{From: "cmd_myapp", To: layerAdapter},
		},
	}
	rendered := Render(cfg, nil, false)

	path := filepath.Join(t.TempDir(), ".archfit.yaml")
	if err := os.WriteFile(path, []byte(rendered), 0o600); err != nil {
		t.Fatal(err)
	}
	loaded, err := config.Load(context.Background(), path)
	if err != nil {
		t.Fatalf("config.Load rejected layered init YAML: %v\n---\n%s", err, rendered)
	}
	// Regression for the 3+-layer duplicate-findings bug: the checker is global,
	// so init must emit exactly one forbidden_layer_direction rule even when
	// four layers are discovered — N rules would report every violation N times.
	got := map[string]string{}
	for _, r := range loaded.Rules {
		if _, dup := got[r.Type]; dup {
			t.Errorf("rule type %q emitted twice: %+v", r.Type, loaded.Rules)
		}
		got[r.Type] = r.Gate
	}
	want := map[string]string{"module_cycle": gateFail, "forbidden_layer_direction": gateFail}
	if len(got) != len(want) || got["module_cycle"] != gateFail || got["forbidden_layer_direction"] != gateFail {
		t.Errorf("rules (type → gate) = %v, want %v\n%s", got, want, rendered)
	}
}

// TestRender_StarterRuleGates pins the adaptive starter gate: fail only when
// the init-time graph covers every module and shows no violation, warn with
// the current count when it shows violations, warn when no complete graph
// was available.
func TestRender_StarterRuleGates(t *testing.T) {
	layered := []ModuleDef{
		{Name: layerCore, Paths: []string{testCorePath}, Layer: layerCore},
		{Name: layerAdapter, Paths: []string{"internal/adapter/**"}, Layer: layerAdapter},
	}
	tests := []struct {
		name          string
		edges         []ModuleEdge
		graphComplete bool
		wantCycle     string
		wantLayer     string
		wantText      string
	}{
		{name: "clean complete graph", edges: []ModuleEdge{{From: layerAdapter, To: layerCore}},
			graphComplete: true, wantCycle: gateFail, wantLayer: gateFail, wantText: "none at init"},
		{name: "cycle", edges: []ModuleEdge{{From: layerAdapter, To: layerCore}, {From: layerCore, To: layerAdapter}},
			graphComplete: true, wantCycle: gateWarn, wantLayer: gateWarn, wantText: "1 current module cycle(s)"},
		{name: "back-edge only", edges: []ModuleEdge{{From: layerCore, To: layerAdapter}},
			graphComplete: true, wantCycle: gateFail, wantLayer: gateWarn, wantText: "1 current layer back-edge(s)"},
		{name: "no complete graph", graphComplete: false,
			wantCycle: gateWarn, wantLayer: gateWarn, wantText: "no complete import graph"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			out := Render(DiscoveredConfig{
				Modules: layered, Layers: []string{layerCore, layerAdapter},
				Edges: tt.edges, ImportGraphComplete: tt.graphComplete,
			}, nil, false)
			loaded := loadRenderedConfig(t, out)
			gates := map[string]string{}
			for _, r := range loaded.Rules {
				gates[r.Type] = r.Gate
			}
			if gates["module_cycle"] != tt.wantCycle || gates["forbidden_layer_direction"] != tt.wantLayer {
				t.Errorf("gates = %v, want module_cycle=%s forbidden_layer_direction=%s\n%s",
					gates, tt.wantCycle, tt.wantLayer, out)
			}
			if !strings.Contains(out, tt.wantText) {
				t.Errorf("missing gate note %q:\n%s", tt.wantText, out)
			}
		})
	}
}

// TestRender_NoInferredLayers_CommentedHowTo pins the no-guess path: with
// fewer than two layers, no live layers: list and no live direction rule
// (it could never fire), but a commented stanza and rule the owner uncomments.
func TestRender_NoInferredLayers_CommentedHowTo(t *testing.T) {
	out := Render(DiscoveredConfig{
		Modules: []ModuleDef{{Name: layerCore, Paths: []string{"internal/core/**"}}},
	}, nil, false)
	loaded := loadRenderedConfig(t, out)
	if len(loaded.Layers) != 0 {
		t.Errorf("layers = %v, want none", loaded.Layers)
	}
	if len(loaded.Rules) != 1 || loaded.Rules[0].Type != "module_cycle" {
		t.Errorf("live rules = %+v, want only module_cycle", loaded.Rules)
	}
	for _, want := range []string{
		"# layers: not inferred",
		"# layers:\n#   - domain\n",
		"  # - id: no-layer-back-edges\n  #   type: forbidden_layer_direction\n  #   gate: fail\n",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("missing %q:\n%s", want, out)
		}
	}
	for i, line := range strings.Split(out, "\n") {
		if len(line) > 100 {
			t.Errorf("line %d is %d bytes, want <= 100: %q", i+1, len(line), line)
		}
	}
}

func loadRenderedConfig(t *testing.T, rendered string) config.Config {
	t.Helper()
	path := filepath.Join(t.TempDir(), ".archfit.yaml")
	if err := os.WriteFile(path, []byte(rendered), 0o600); err != nil {
		t.Fatal(err)
	}
	loaded, err := config.Load(context.Background(), path)
	if err != nil {
		t.Fatalf("config.Load: %v\n---\n%s", err, rendered)
	}
	return loaded
}

// ---------------------------------------------------------------------------
// Internal helpers
// ---------------------------------------------------------------------------

func TestGroupKey(t *testing.T) {
	tests := []struct {
		in   string
		want string
	}{
		{".", "."},
		{"", "."},
		{testCmdArchfit, testCmdArchfit},
		{"internal/" + adapterExtract + "/golang", "internal/" + adapterExtract},
		{"internal/" + layerModel + "/graph", "internal/" + layerModel},
		{"cmd", "cmd"},
	}
	for _, tt := range tests {
		got := groupKey(tt.in)
		if got != tt.want {
			t.Errorf("groupKey(%q) = %q, want %q", tt.in, got, tt.want)
		}
	}
}

func TestStripPrefix(t *testing.T) {
	tests := []struct {
		importPath string
		modPath    string
		want       string
	}{
		{testModPathChild, testModPath, "pkg/a"},
		{testModPath, testModPath, ""},
		{"fmt", "", "fmt"},
	}
	for _, tt := range tests {
		got := stripPrefix(tt.importPath, tt.modPath)
		if got != tt.want {
			t.Errorf("stripPrefix(%q, %q) = %q, want %q", tt.importPath, tt.modPath, got, tt.want)
		}
	}
}

func TestModuleNameFromKey(t *testing.T) {
	tests := []struct {
		key  string
		want string
	}{
		{"internal/" + adapterExtract, adapterExtract},
		{"internal/" + layerModel, layerModel},
		{testCmdArchfit, "cmd_archfit"},
		{"pkg/foo", "pkg_foo"},
	}
	for _, tt := range tests {
		got := moduleNameFromKey(tt.key)
		if got != tt.want {
			t.Errorf("moduleNameFromKey(%q) = %q, want %q", tt.key, got, tt.want)
		}
	}
}

// TestRender_RoundTripsThroughConfigLoad is the fitness guard for the implicit
// YAML contract between initcfg and config: everything Render writes must
// survive config.Load unchanged. When policy.ModuleDef gains a field that
// init should generate, this test is where the divergence surfaces.
func TestRender_RoundTripsThroughConfigLoad(t *testing.T) {
	rendered := Render(DiscoveredConfig{
		ModulePath: testExampleMod,
		HasGo:      true,
		Layers:     []string{layerModel, layerCore, "adapter", layerCmd},
		Modules: []ModuleDef{
			{
				Name:     layerCore,
				Paths:    []string{testCorePath},
				Public:   []string{"internal/core"},
				Internal: []string{"internal/core/private/**"},
				Layer:    layerCore,
			},
			{
				Name:  "adapters",
				Paths: []string{"internal/adapters/**"},
				Layer: "adapter",
			},
		},
	}, nil, false)

	path := filepath.Join(t.TempDir(), ".archfit.yaml")
	if err := os.WriteFile(path, []byte(rendered), 0o600); err != nil {
		t.Fatal(err)
	}
	cfg, err := config.Load(context.Background(), path)
	if err != nil {
		t.Fatalf("config.Load rejected init-generated YAML: %v\n---\n%s", err, rendered)
	}

	if len(cfg.Layers) != 4 || cfg.Layers[0] != layerModel || cfg.Layers[3] != layerCmd {
		t.Errorf("layers = %v, want [model core adapter cmd]", cfg.Layers)
	}
	core, ok := cfg.Modules[layerCore]
	if !ok {
		t.Fatalf("module %q missing after round-trip; modules = %v", layerCore, cfg.Modules)
	}
	if len(core.Paths) != 1 || core.Paths[0] != "internal/core/**" {
		t.Errorf("core.Paths = %v", core.Paths)
	}
	if len(core.Public) != 1 || core.Public[0] != "internal/core" {
		t.Errorf("core.Public = %v", core.Public)
	}
	if len(core.Internal) != 1 || core.Internal[0] != "internal/core/private/**" {
		t.Errorf("core.Internal = %v", core.Internal)
	}
	if core.Layer != layerCore {
		t.Errorf("core.Layer = %q, want core", core.Layer)
	}
	if _, ok := cfg.Modules["adapters"]; !ok {
		t.Errorf("module %q missing after round-trip", "adapters")
	}
	if len(cfg.Rules) == 0 {
		t.Error("init-generated config carries no rules — starter rules lost in round-trip")
	}
}
