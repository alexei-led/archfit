package initcfg

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

// Test-local constants to satisfy goconst across the disambiguate test file.
const (
	testAlpha   = "alpha"
	testSvc     = "svc"
	testSvcName = "internal_svc"
	testSvcCfg  = "internal/svc"
	testPathA   = "a/**"
)

// ---------------------------------------------------------------------------
// disambiguateNames unit tests
// ---------------------------------------------------------------------------

func TestDisambiguateNames_NoCollision_Unchanged(t *testing.T) {
	mods := []ModuleDef{
		{Name: testAlpha, Paths: []string{"pkg/" + testAlpha + "/**"}},
		{Name: "beta", Paths: []string{"pkg/beta/**"}},
		{Name: "gamma", Paths: []string{"pkg/gamma/**"}},
	}
	got := disambiguateNames(mods)
	wantNames := []string{testAlpha, "beta", "gamma"}
	for i, w := range wantNames {
		if got[i].Name != w {
			t.Errorf("[%d] Name = %q, want %q", i, got[i].Name, w)
		}
	}
}

func TestDisambiguateNames_Collision_SlugFromFirstPath(t *testing.T) {
	// Go module layerCore and Python module layerCore — names collide.
	mods := []ModuleDef{
		{Name: layerCore, Paths: []string{"internal/" + layerCore + "/**"}},
		{Name: layerCore, Paths: []string{"myapp." + layerCore, "myapp." + layerCore + ".*"}},
	}
	got := disambiguateNames(mods)
	if got[0].Name != "internal_core" {
		t.Errorf("[0] Name = %q, want %q", got[0].Name, "internal_core")
	}
	if got[1].Name != "myapp_core" {
		t.Errorf("[1] Name = %q, want %q", got[1].Name, "myapp_core")
	}
}

func TestDisambiguateNames_SlugCollision_NumericSuffix(t *testing.T) {
	// Two modules whose slugs both resolve to "internal_utils" — one gets _2.
	mods := []ModuleDef{
		{Name: "utils", Paths: []string{"internal/utils/**"}},
		{Name: "utils", Paths: []string{"internal/utils/**"}}, // same slug
	}
	got := disambiguateNames(mods)
	if got[0].Name != "internal_utils" {
		t.Errorf("[0] Name = %q, want %q", got[0].Name, "internal_utils")
	}
	if got[1].Name != "internal_utils_2" {
		t.Errorf("[1] Name = %q, want %q", got[1].Name, "internal_utils_2")
	}
}

func TestDisambiguateNames_SlugEqualsExistingName_NumericSuffix(t *testing.T) {
	// "pkg/alpha" slugs to "pkg_alpha", but "pkg_alpha" is already taken by
	// a non-colliding module. The slug must get a _2 suffix.
	mods := []ModuleDef{
		{Name: "pkg_" + testAlpha, Paths: []string{"vendor/pkg_" + testAlpha + "/**"}}, // non-collider, keeps its name
		{Name: testAlpha, Paths: []string{"pkg/" + testAlpha + "/**"}},                 // collider 1
		{Name: testAlpha, Paths: []string{"internal/" + testAlpha + "/**"}},            // collider 2
	}
	got := disambiguateNames(mods)

	// Non-collider must be untouched.
	if got[0].Name != "pkg_"+testAlpha {
		t.Errorf("[0] Name = %q, want %q", got[0].Name, "pkg_"+testAlpha)
	}
	// First collider slugs to "pkg_alpha" — already taken, gets _2.
	if got[1].Name != "pkg_"+testAlpha+"_2" {
		t.Errorf("[1] Name = %q, want %q", got[1].Name, "pkg_"+testAlpha+"_2")
	}
	// Second collider slugs to "internal_alpha" — unique.
	if got[2].Name != "internal_"+testAlpha {
		t.Errorf("[2] Name = %q, want %q", got[2].Name, "internal_"+testAlpha)
	}
}

func TestDisambiguateNames_StableOrdering(t *testing.T) {
	// Running disambiguateNames on the same input twice must produce identical output.
	mods := func() []ModuleDef {
		return []ModuleDef{
			{Name: testSvc, Paths: []string{"internal/" + testSvc + "/**"}},
			{Name: testSvc, Paths: []string{"pkg/" + testSvc + "/**"}},
			{Name: testSvc, Paths: []string{"cmd/" + testSvc + "/**"}},
		}
	}
	first := disambiguateNames(mods())
	second := disambiguateNames(mods())
	for i := range first {
		if first[i].Name != second[i].Name {
			t.Errorf("[%d] unstable: first=%q second=%q", i, first[i].Name, second[i].Name)
		}
	}
	// Also verify specific expected names.
	wantNames := []string{testSvcName, "pkg_" + testSvc, "cmd_" + testSvc}
	for i, w := range wantNames {
		if first[i].Name != w {
			t.Errorf("[%d] Name = %q, want %q", i, first[i].Name, w)
		}
	}
}

func TestDisambiguateNames_Empty(t *testing.T) {
	got := disambiguateNames(nil)
	if got != nil {
		t.Errorf("expected nil for nil input, got %v", got)
	}
	got = disambiguateNames([]ModuleDef{})
	if len(got) != 0 {
		t.Errorf("expected empty for empty input, got %v", got)
	}
}

// ---------------------------------------------------------------------------
// Discover integration: no collision → Render output byte-identical
// ---------------------------------------------------------------------------

// goListSingleLang is a go list output with no name collisions.
const goListSingleLang = `{
  "ImportPath": "github.com/example/myapp/internal/core",
  "Dir": "/repo/internal/core",
  "Module": {"Path": "github.com/example/myapp"}
}
{
  "ImportPath": "github.com/example/myapp/internal/model/graph",
  "Dir": "/repo/internal/model/graph",
  "Module": {"Path": "github.com/example/myapp"}
}
`

func TestDiscover_NoCollision_RenderByteIdentical(t *testing.T) {
	root := t.TempDir()
	writeGoMod(t, root)
	runner := mockRunner(goListSingleLang)

	cfg1, err := Discover(context.Background(), root, runner)
	if err != nil {
		t.Fatalf("Discover: %v", err)
	}
	cfg2, err := Discover(context.Background(), root, runner)
	if err != nil {
		t.Fatalf("Discover second run: %v", err)
	}

	out1 := Render(cfg1, nil, false)
	out2 := Render(cfg2, nil, false)
	if out1 != out2 {
		t.Errorf("Render output not byte-identical across two Discover runs\n--- run1 ---\n%s\n--- run2 ---\n%s", out1, out2)
	}

	// Verify no disambiguation changed names (no collision exists).
	for _, m := range cfg1.Modules {
		if m.Name != layerCore && m.Name != layerModel {
			t.Errorf("unexpected module name %q (collision disambiguation should not have fired)", m.Name)
		}
	}
}

// ---------------------------------------------------------------------------
// Discover integration: Go + Python collision → distinct deterministic slugs
// ---------------------------------------------------------------------------

func TestDiscover_GoAndPythonSameName_DistinctSlugs(t *testing.T) {
	// Go module internal/model and Python package "model" both yield Name="model".
	root := t.TempDir()
	writeGoMod(t, root)

	// Python project marker + top-level "model" package.
	if err := os.WriteFile(filepath.Join(root, "pyproject.toml"), []byte("[project]"), 0o600); err != nil {
		t.Fatal(err)
	}
	modelDir := filepath.Join(root, layerModel)
	if err := os.MkdirAll(modelDir, testDirPerm); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(modelDir, pyInitFile), []byte(""), 0o600); err != nil {
		t.Fatal(err)
	}

	// Go list returns internal/model, which moduleNameFromKey maps to "model".
	goList := `{
  "ImportPath": "github.com/example/myapp/internal/model/graph",
  "Dir": "/repo/internal/model/graph",
  "Module": {"Path": "github.com/example/myapp"}
}
`
	runner := mockRunner(goList)
	cfg, err := Discover(context.Background(), root, runner)
	if err != nil {
		t.Fatalf("Discover: %v", err)
	}

	names := make(map[string]int)
	for _, m := range cfg.Modules {
		names[m.Name]++
	}
	// All names must be unique after disambiguation.
	for name, n := range names {
		if n > 1 {
			t.Errorf("duplicate name %q after disambiguation (appears %d times)", name, n)
		}
	}
	// At most one module may carry the raw "model" name (the one whose slug
	// happens to still be "model" — e.g. a Python dotted path "model" whose
	// first path element has no "/" or "." to replace). The invariant is
	// uniqueness, not that the slug must differ from the original name.
	if names[layerModel] > 1 {
		t.Errorf("raw name %q appears %d times — disambiguation failed", layerModel, names[layerModel])
	}
}
