package initcfg

import (
	"context"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/alexei-led/archfit/internal/toolrun"
)

// TestDiscover_GoWorkMembers runs go list once per member the extractor names,
// keys packages by their root-relative path, maps cross-member imports, and
// passes GOWORK=off through when the extractor ignores the go.work.
const (
	testModLibShared = "lib_shared"
	testModSvcA      = "svc_a"
	testModDomain    = "domain"
)

func TestDiscover_GoWorkMembers(t *testing.T) {
	root := t.TempDir()
	byDir := map[string]string{
		filepath.Join(root, "lib", "shared"): `{"ImportPath":"example.com/lib/shared","Module":{"Path":"example.com/lib/shared"}}
{"ImportPath":"example.com/lib/shared/hook","Imports":["example.com/svc/a/api","fmt"],"Module":{"Path":"example.com/lib/shared"}}`,
		filepath.Join(root, "svc", "a"): `{"ImportPath":"example.com/svc/a","Imports":["example.com/lib/shared"],"Module":{"Path":"example.com/svc/a"}}
{"ImportPath":"example.com/svc/a/api","Module":{"Path":"example.com/svc/a"}}`,
	}
	var envs [][]string
	runner := &toolrun.RunnerMock{
		RunFunc: func(_ context.Context, cmd toolrun.ToolCmd) (toolrun.Output, error) {
			envs = append(envs, cmd.Env)
			return toolrun.Output{Stdout: []byte(byDir[cmd.WorkDir])}, nil
		},
	}
	members := []string{filepath.Join(root, "lib", "shared"), filepath.Join(root, "svc", "a")}
	cfg, err := Discover(context.Background(), root, runner, Presence{Go: true, GoMembers: members, GoWorkOff: true})
	if err != nil {
		t.Fatalf("Discover: %v", err)
	}

	type mod struct {
		Paths, Public, Sources []string
	}
	got := map[string]mod{}
	for _, m := range cfg.Modules {
		got[m.Name] = mod{m.Paths, m.Public, m.Sources}
	}
	want := map[string]mod{
		testModLibShared: {[]string{"lib/shared/**"}, nil, []string{"lib/shared", "lib/shared/hook"}},
		testModSvcA:      {[]string{"svc/a/**"}, nil, []string{"svc/a", "svc/a/api"}},
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("modules = %+v, want %+v", got, want)
	}
	wantEdges := []ModuleEdge{{From: testModLibShared, To: testModSvcA}, {From: testModSvcA, To: testModLibShared}}
	if !reflect.DeepEqual(cfg.Edges, wantEdges) {
		t.Errorf("edges = %+v, want %+v", cfg.Edges, wantEdges)
	}
	if !cfg.HasGo || !cfg.ImportGraphComplete || cfg.ModulePath != "example.com/lib/shared" {
		t.Errorf("HasGo=%v ImportGraphComplete=%v ModulePath=%q", cfg.HasGo, cfg.ImportGraphComplete, cfg.ModulePath)
	}
	if len(envs) != 2 || !reflect.DeepEqual(envs[0], []string{"GOWORK=off"}) {
		t.Errorf("go list env = %v, want GOWORK=off on each of 2 member runs", envs)
	}
	if out := Render(cfg, nil, false); !strings.Contains(out, "gate: warn — 1 current module cycle(s) at init") {
		t.Errorf("the cross-member cycle should be counted in the module_cycle gate note:\n%s", out)
	}
}

// TestDiscover_GoModulesDeclareNoPublicSurface: under bc_score.v7 a public:
// entry claims a published contract, so discovery never writes one for Go.
func TestDiscover_GoModulesDeclareNoPublicSurface(t *testing.T) {
	root := t.TempDir()
	writeGoMod(t, root)
	runner := mockRunner(`{"ImportPath":"github.com/example/myapp/internal/domain/order","Module":{"Path":"github.com/example/myapp"}}
{"ImportPath":"github.com/example/myapp/internal/app","Module":{"Path":"github.com/example/myapp"}}`)
	cfg, err := Discover(context.Background(), root, runner, Presence{Go: true, GoMembers: []string{root}})
	if err != nil {
		t.Fatalf("Discover: %v", err)
	}
	if len(cfg.Modules) == 0 {
		t.Fatal("no modules discovered")
	}
	for _, m := range cfg.Modules {
		if len(m.Public) != 0 {
			t.Errorf("module %s public = %v, want none", m.Name, m.Public)
		}
	}
}

// TestDiscover_PresenceDrivesLanguageModes: presence comes from the registry,
// so a language whose extractor applies is enabled even when discovery found
// no module (a package.json with no src/ or lib/, a flat Python project), and
// an absent language is written auto, never false. Go discovery does not run
// without a Go member.
func TestDiscover_PresenceDrivesLanguageModes(t *testing.T) {
	root := t.TempDir()
	for name, body := range map[string]string{"package.json": "{}", "pyproject.toml": "[project]\n", "app.py": ""} {
		if err := os.WriteFile(filepath.Join(root, name), []byte(body), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	runner := &toolrun.RunnerMock{RunFunc: func(_ context.Context, cmd toolrun.ToolCmd) (toolrun.Output, error) {
		t.Errorf("unexpected subprocess %s %v", cmd.Name, cmd.Args)
		return toolrun.Output{}, nil
	}}
	cfg, err := Discover(context.Background(), root, runner, ProbePresence(root))
	if err != nil {
		t.Fatalf("Discover: %v", err)
	}
	if len(cfg.Modules) != 0 {
		t.Fatalf("modules = %+v, want none", cfg.Modules)
	}
	out := Render(cfg, nil, false)
	for _, want := range []string{
		"  go:\n    enabled: auto\n",
		"  python:\n    enabled: true\n",
		"  typescript:\n    enabled: true\n",
		"  rust:\n    enabled: auto\n",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("missing %q:\n%s", want, out)
		}
	}
	if strings.Contains(out, "enabled: false") {
		t.Errorf("init must never write enabled: false:\n%s", out)
	}
}

// TestDiscover_GoWorkBrokenMemberStillDiscovered: one package that cannot
// load (an import no module provides) must not abort discovery for the whole
// workspace — analysis tolerates it, so onboarding must too.
func TestDiscover_GoWorkBrokenMemberStillDiscovered(t *testing.T) {
	root := t.TempDir()
	for rel, body := range map[string]string{
		"go.work":           "go 1.21\n\nuse (\n\t./svc/a\n\t./svc/b\n)\n",
		"svc/a/go.mod":      "module example.com/svc/a\n\ngo 1.21\n",
		"svc/a/a.go":        "package a\n\nfunc A() int { return 1 }\n",
		"svc/b/go.mod":      "module example.com/svc/b\n\ngo 1.21\n",
		"svc/b/main.go":     "package main\n\nimport \"example.com/nowhere/pkg\"\n\nfunc main() { println(pkg.X) }\n",
		"svc/b/util/u.go":   "package util\n\nimport \"example.com/svc/a\"\n\nfunc U() int { return a.A() }\n",
		"svc/b/broken/b.go": "package broken\n\nfunc X( {\n",
	} {
		full := filepath.Join(root, filepath.FromSlash(rel))
		if err := os.MkdirAll(filepath.Dir(full), 0o750); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(full, []byte(body), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	cfg, err := Discover(context.Background(), root, toolrun.New(), ProbePresence(root))
	if err != nil {
		t.Fatalf("Discover: %v", err)
	}
	names := make([]string, 0, len(cfg.Modules))
	for _, m := range cfg.Modules {
		names = append(names, m.Name)
	}
	if !reflect.DeepEqual(names, []string{testModSvcA, "svc_b"}) {
		t.Errorf("modules = %v, want [svc_a svc_b]", names)
	}
	if want := []ModuleEdge{{From: "svc_b", To: testModSvcA}}; !reflect.DeepEqual(cfg.Edges, want) {
		t.Errorf("edges = %+v, want %+v", cfg.Edges, want)
	}
}
