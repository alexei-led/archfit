package initcfg

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/alexei-led/archfit/internal/toolrun"
)

// Test-local constants to satisfy goconst across the test file.
const (
	testModuleName   = "utils"
	testCmdArchfit   = "cmd/archfit"
	testModPath      = "github.com/foo/bar"
	testModPathChild = "github.com/foo/bar/pkg/a"
	testDirPerm      = 0o750
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

func TestDiscover_GoList_GroupsModules(t *testing.T) {
	runner := mockRunner(sampleGoListJSON)
	cfg, err := Discover(context.Background(), "/repo", runner)
	if err != nil {
		t.Fatalf("Discover: %v", err)
	}

	if cfg.ModulePath != "github.com/example/myapp" {
		t.Errorf("ModulePath = %q, want %q", cfg.ModulePath, "github.com/example/myapp")
	}

	// Expect modules grouped by 2-segment key.
	wantNames := map[string]bool{
		"cmd_myapp":    true,
		layerModel:     true,
		adapterExtract: true,
		layerEngine:    true,
		"classify":     true,
	}
	for _, m := range cfg.Modules {
		delete(wantNames, m.Name)
	}
	for name := range wantNames {
		t.Errorf("expected module %q not found in discovered modules", name)
	}
}

func TestDiscover_GoList_LayerInference(t *testing.T) {
	runner := mockRunner(sampleGoListJSON)
	cfg, err := Discover(context.Background(), "/repo", runner)
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
		{"cmd_myapp", layerCmd},
		{layerModel, layerModel},
		{adapterExtract, layerAdapter},
		{layerEngine, layerEngine},
		{"classify", layerCore},
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
	runner := mockRunner(sampleGoListJSON)
	cfg, err := Discover(context.Background(), "/repo", runner)
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
	runner := &toolrun.RunnerMock{
		RunFunc: func(_ context.Context, _ toolrun.ToolCmd) (toolrun.Output, error) {
			return toolrun.Output{ExitCode: 1, Stderr: []byte("no go files")}, nil
		},
		DetectFunc: func(_ context.Context, _ string) (toolrun.ToolInfo, bool) {
			return toolrun.ToolInfo{}, false
		},
	}
	_, err := Discover(context.Background(), "/repo", runner)
	if err == nil {
		t.Fatal("expected error for non-zero exit, got nil")
	}
}

func TestDiscover_MalformedJSON_ReturnsError(t *testing.T) {
	runner := mockRunner(`{not valid json}`)
	_, err := Discover(context.Background(), "/repo", runner)
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
	for _, want := range []string{"core", "api"} {
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
	out := Render(cfg)

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
		{"forbidden_dependency type", "type: forbidden_dependency"},
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
	out := Render(cfg)
	if !strings.Contains(out, "version: 1") {
		t.Errorf("expected version: 1 in output, got:\n%s", out)
	}
	if !strings.Contains(out, "rules:") {
		t.Errorf("expected rules: section in output, got:\n%s", out)
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
