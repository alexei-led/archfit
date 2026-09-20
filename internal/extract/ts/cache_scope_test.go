package ts_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	evidenceports "github.com/alexei-led/archfit/internal/evidence/ports"
	"github.com/alexei-led/archfit/internal/extract/ts"
	"github.com/alexei-led/archfit/internal/factcache"
	"github.com/alexei-led/archfit/internal/scope"
)

func TestFactCache_DependencySymlinksAndResolutionInventory(t *testing.T) {
	root := writeTSFixture(t, `import "dependency"`, nil)
	dependency := t.TempDir()
	if err := os.WriteFile(filepath.Join(dependency, "package.json"), []byte(`{"name":"dependency","main":"index.js"}`), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(filepath.Join(root, "node_modules"), 0o750); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(dependency, filepath.Join(root, "node_modules", "dependency")); err != nil {
		t.Fatal(err)
	}
	f, err := os.Create(filepath.Join(dependency, "index.js")) //nolint:gosec // isolated test fixture
	if err != nil {
		t.Fatal(err)
	}
	if err := f.Truncate(1 << 30); err != nil {
		t.Fatal(err)
	}
	if err := f.Close(); err != nil {
		t.Fatal(err)
	}
	calls := 0
	ex := ts.New(cacheFixtureRunner(`{"modules":[{"source":"a.ts","dependencies":[{"module":"dependency","resolved":"node_modules/dependency/index.js"}]}]}`, &calls), evidenceports.ExtractConfig{Mode: evidenceports.ModeAuto, Src: "."})
	ex.Cache = factcache.NewStore(t.TempDir())
	extract := func() {
		t.Helper()
		if _, _, err := ex.Extract(context.Background(), scope.Scope{Root: root}); err != nil {
			t.Fatal(err)
		}
	}
	extract()
	extract()
	if calls != 1 {
		t.Fatalf("stable symlink dependency inventory must cache without reading source bytes: calls=%d", calls)
	}
	if err := os.WriteFile(filepath.Join(dependency, "new.js"), []byte("export {}"), 0o600); err != nil {
		t.Fatal(err)
	}
	extract()
	if calls != 2 {
		t.Fatalf("new resolution candidate must invalidate: calls=%d", calls)
	}
	if err := os.WriteFile(filepath.Join(dependency, "package.json"), []byte(`{"name":"dependency","main":"new.js"}`), 0o600); err != nil {
		t.Fatal(err)
	}
	extract()
	if calls != 3 {
		t.Fatalf("symlinked resolver metadata edit must invalidate: calls=%d", calls)
	}
}

func TestFactCache_DynamicDepcruiseConfigRunsFresh(t *testing.T) {
	root := writeTSFixture(t, "export const a = 1", map[string]string{
		".dependency-cruiser.cjs": `module.exports = require("./custom-settings.json")`,
		"custom-settings.json":    `{"options":{}}`,
	})
	calls := 0
	ex := ts.New(cacheFixtureRunner(`{"modules":[{"source":"a.ts","dependencies":[]}]}`, &calls), evidenceports.ExtractConfig{Mode: evidenceports.ModeAuto, Src: "."})
	ex.Cache = factcache.NewStore(t.TempDir())
	for range 2 {
		if _, _, err := ex.Extract(context.Background(), scope.Scope{Root: root}); err != nil {
			t.Fatal(err)
		}
	}
	if calls != 2 {
		t.Fatalf("unknown dynamic configuration inputs must bypass cache: calls=%d", calls)
	}
}
