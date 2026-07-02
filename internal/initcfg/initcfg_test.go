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
	cfg, err := Discover(context.Background(), root, runner)
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

func TestDiscover_GoList_LayerInference(t *testing.T) {
	root := t.TempDir()
	writeGoMod(t, root)
	runner := mockRunner(sampleGoListJSON)
	cfg, err := Discover(context.Background(), root, runner)
	if err != nil {
		t.Fatalf("Discover: %v", err)
	}

	layerByName := make(map[string]string, len(cfg.Modules))
	for _, m := range cfg.Modules {
		layerByName[m.Name] = m.Layer
	}

	tests := []struct {
		name      string
		wantLayer string
	}{
		{testCmdMyapp, layerCmd},
		{layerModel, layerModel},
		{adapterExtract, layerAdapter},
		{layerEngine, layerEngine},
		{testClassify, layerCore},
	}
	for _, tt := range tests {
		got, ok := layerByName[tt.name]
		if !ok {
			t.Errorf("module %q not found", tt.name)
			continue
		}
		if got != tt.wantLayer {
			t.Errorf("module %q: layer = %q, want %q", tt.name, got, tt.wantLayer)
		}
	}
}

func TestDiscover_GoList_LayerOrder(t *testing.T) {
	root := t.TempDir()
	writeGoMod(t, root)
	runner := mockRunner(sampleGoListJSON)
	cfg, err := Discover(context.Background(), root, runner)
	if err != nil {
		t.Fatalf("Discover: %v", err)
	}
	// Layers must follow canonical order: model < core < adapter < engine < cmd
	order := map[string]int{
		layerModel: 0, layerCore: 1, layerAdapter: 2, layerEngine: 3, layerCmd: 4,
	}
	prev := -1
	for _, l := range cfg.Layers {
		idx, ok := order[l]
		if !ok {
			t.Errorf("unexpected layer %q", l)
			continue
		}
		if idx <= prev {
			t.Errorf("layers out of order: %v", cfg.Layers)
			break
		}
		prev = idx
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
	_, err := Discover(context.Background(), root, runner)
	if err == nil {
		t.Fatal("expected error for non-zero exit, got nil")
	}
}

func TestDiscover_MalformedJSON_ReturnsError(t *testing.T) {
	root := t.TempDir()
	writeGoMod(t, root)
	runner := mockRunner(`{not valid json}`)
	_, err := Discover(context.Background(), root, runner)
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
		{"version", "version: 1"},
		{"TODO comment", "# Generated by archfit init"},
		{"layers section", "layers:"},
		{"model layer", "- " + layerModel},
		{"engine layer", "- " + layerEngine},
		{"modules section", "modules:"},
		{"engine module", layerEngine + ":"},
		{"model module", layerModel + ":"},
		{"rules section", "rules:"},
		{"forbidden_layer_direction type", "type: forbidden_layer_direction"},
		{"gate warn", "gate: warn"},
	}
	for _, c := range checks {
		if !strings.Contains(out, c.want) {
			t.Errorf("Render output missing %s (want %q)\nfull output:\n%s", c.desc, c.want, out)
		}
	}
}

func TestRender_NoModules_StillValid(t *testing.T) {
	cfg := DiscoveredConfig{ModulePath: "github.com/example/empty"}
	out := Render(cfg, nil, false)
	if !strings.Contains(out, "version: 1") {
		t.Errorf("expected version: 1 in output, got:\n%s", out)
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

	// The rule ID must name the back-edge direction: model→core inversion.
	if !strings.Contains(out, "id: no-"+layerModel+"-imports-"+layerCore) {
		t.Errorf("expected rule flagging %s importing %s:\n%s", layerModel, layerCore, out)
	}
}

func TestRender_LayeredRules_RoundTripsConfigLoad(t *testing.T) {
	cfg := DiscoveredConfig{
		ModulePath: testExampleMod,
		HasGo:      true,
		Layers:     []string{layerModel, layerCore, layerAdapter, layerCmd},
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
	if len(loaded.Rules) == 0 {
		t.Error("no rules after round-trip")
	}
	for _, r := range loaded.Rules {
		if r.Type != "forbidden_layer_direction" {
			t.Errorf("unexpected rule type %q", r.Type)
		}
		// from_layer/to_layer are deliberately not emitted (see Render): the checker
		// derives layer ordering from cfg.Layers and the module map, not per-rule.
		if r.FromLayer != "" || r.ToLayer != "" {
			t.Errorf("rule %q should not have from_layer/to_layer set: %+v", r.ID, r)
		}
	}
}

func TestRender_NoEdges_FallbackComment(t *testing.T) {
	// No edges → generic placeholder + comment about no gates.
	cfg := DiscoveredConfig{
		ModulePath: testExampleMod,
		Modules: []ModuleDef{
			{Name: layerCore, Paths: []string{"internal/core/**"}, Layer: layerCore},
		},
		Layers: []string{layerCore},
	}
	out := Render(cfg, nil, false)
	if !strings.Contains(out, "only metrics") {
		t.Errorf("expected fallback comment about no gates when no edges:\n%s", out)
	}
	if !strings.Contains(out, "type: forbidden_layer_direction") {
		t.Errorf("expected generic forbidden_layer_direction rule:\n%s", out)
	}
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
// survive config.Load unchanged. When config.ModuleDef gains a field that
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
