package main

import (
	"encoding/json"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/alexei-led/archfit/internal/initcfg"
)

// Onboarding end-to-end: `config init` over a real tree must write a policy
// that passes `config lint` and blocks the canonical violation under `check`.

const (
	ruleIDStarterModuleCycles = "no-module-cycles"
	ruleIDStarterLayerRule    = "no-layer-back-edges"
	testLayerApplication      = "application"

	hexDomainToAdapterFile = "internal/domain/billing/charge.go"
	hexDomainToAdapterSrc  = "package billing\n\nimport \"example.com/shop/internal/adapters/postgres\"\n\n" +
		"func Persist() error { return postgres.Store{}.Ping() }\n"
)

// hexagonalFiles is a stdlib-only hexagonal Go service: domain at the centre,
// app orchestrating it, adapters implementing its ports, one entrypoint.
func hexagonalFiles() map[string]string {
	return map[string]string{
		markerGoMod: "module example.com/shop\n\ngo 1.21\n",
		"internal/domain/order/order.go": "package order\n\ntype Order struct{ ID string }\n\n" +
			"func New(id string) Order { return Order{ID: id} }\n",
		"internal/domain/billing/billing.go": "package billing\n\nimport \"example.com/shop/internal/domain/order\"\n\n" +
			"func Charge(o order.Order) int { return len(o.ID) }\n",
		"internal/app/app.go": "package app\n\nimport (\n\t\"example.com/shop/internal/domain/billing\"\n" +
			"\t\"example.com/shop/internal/domain/order\"\n)\n\n" +
			"type Repo interface{ Save(order.Order) error }\n\n" +
			"func Place(r Repo, id string) (int, error) {\n\to := order.New(id)\n\treturn billing.Charge(o), r.Save(o)\n}\n",
		"internal/adapters/postgres/postgres.go": "package postgres\n\nimport \"example.com/shop/internal/domain/order\"\n\n" +
			"type Store struct{}\n\nfunc (Store) Save(order.Order) error { return nil }\n\n" +
			"func (Store) Ping() error { return nil }\n",
		"internal/adapters/http/http.go": "package http\n\nimport (\n\t\"net/http\"\n\n\t\"example.com/shop/internal/app\"\n)\n\n" +
			"func Handler(r app.Repo) http.HandlerFunc {\n" +
			"\treturn func(w http.ResponseWriter, _ *http.Request) { _, _ = app.Place(r, \"x\") }\n}\n",
		"cmd/server/main.go": "package main\n\nimport (\n\t\"net/http\"\n\n" +
			"\thttpadapter \"example.com/shop/internal/adapters/http\"\n" +
			"\t\"example.com/shop/internal/adapters/postgres\"\n)\n\n" +
			"func main() { _ = http.ListenAndServe(\":8080\", httpadapter.Handler(postgres.Store{})) }\n",
	}
}

// writeOnboardingRepo writes files into a fresh git repository with no config.
func writeOnboardingRepo(t *testing.T, files map[string]string) string {
	t.Helper()
	dir := t.TempDir()
	for rel, content := range files {
		writeFileAt(t, dir, rel, content)
	}
	gitInitFixtureRepo(t, dir)
	return dir
}

// initAndLint runs `config init` and `config lint` over root and returns the
// written config path and its contents. Lint must pass: a generated policy
// with a dead selector or a public entry that names nothing is not a policy.
func initAndLint(t *testing.T, root string) (string, string) {
	t.Helper()
	if code, stdout, stderr := runArchfit(t, "config", "init", "--root", root); code != 0 {
		t.Fatalf("config init: exit %d\nstdout:\n%s\nstderr:\n%s", code, stdout, stderr)
	}
	cfgPath := filepath.Join(root, defaultConfigPath)
	data, err := os.ReadFile(cfgPath) //#nosec G304 -- test temp dir
	if err != nil {
		t.Fatal(err)
	}
	if code, stdout, stderr := runArchfit(t, "config", "lint", "-c", cfgPath); code != 0 {
		t.Fatalf("config lint on generated config: exit %d\nstdout:\n%s\nstderr:\n%s\nconfig:\n%s", code, stdout, stderr, data)
	}
	return cfgPath, string(data)
}

// declareLayersInConfig performs the edit the generated layers how-to asks for:
// list the layers, set layer: on each module, and uncomment the direction rule.
func declareLayersInConfig(t *testing.T, cfgPath, generated string, layers []string, moduleLayer map[string]string) {
	t.Helper()
	const commented = "  # - id: no-layer-back-edges\n  #   type: forbidden_layer_direction\n  #   gate: fail\n"
	if !strings.Contains(generated, commented) {
		t.Fatalf("generated config has no commented layer rule:\n%s", generated)
	}
	out := strings.Replace(generated, commented, strings.ReplaceAll(commented, "# ", ""), 1)
	out = strings.Replace(out, "\nmodules:\n", "\nlayers:\n  - "+strings.Join(layers, "\n  - ")+"\n\nmodules:\n", 1)
	for module, layer := range moduleLayer {
		header := "\n  " + module + ":\n    paths:\n"
		if !strings.Contains(out, header) {
			t.Fatalf("module %q not in generated config:\n%s", module, out)
		}
		out = strings.Replace(out, header, "\n  "+module+":\n    layer: "+layer+"\n    paths:\n", 1)
	}
	if err := os.WriteFile(cfgPath, []byte(out), 0o600); err != nil {
		t.Fatal(err)
	}
}

// TestOnboarding_Hexagonal_ModuleCycleBlocksDomainToAdapters: the generated
// policy passes lint, does not block the clean service, and blocks a domain
// package importing an adapter — adapters already import the domain, so the
// import closes a module cycle the starter module_cycle rule (gate: fail,
// because init saw a clean complete Go graph) reports.
func TestOnboarding_Hexagonal_ModuleCycleBlocksDomainToAdapters(t *testing.T) {
	t.Parallel()
	root := writeOnboardingRepo(t, hexagonalFiles())
	cfgPath, generated := initAndLint(t, root)
	for _, want := range []string{
		"  go:\n    enabled: true\n",
		"  - id: " + ruleIDStarterModuleCycles + "\n    type: module_cycle\n    gate: fail\n",
		"# layers: not inferred",
	} {
		if !strings.Contains(generated, want) {
			t.Errorf("generated config missing %q:\n%s", want, generated)
		}
	}
	for _, bogus := range []string{"\n      - \"internal/domain\"\n", "\n      - \"internal/adapters\"\n", "layer: core"} {
		if strings.Contains(generated, bogus) {
			t.Errorf("generated config carries %q (a grouping dir is no package; core is a guess):\n%s", bogus, generated)
		}
	}

	if code, state := checkStateOf(t, cfgPath); code == 1 {
		t.Fatalf("check on the clean service: exit 1, findings %+v", state.Findings)
	}

	writeFileAt(t, root, hexDomainToAdapterFile, hexDomainToAdapterSrc)
	code, state := checkStateOf(t, cfgPath)
	if code != 1 {
		t.Fatalf("check after domain→adapters import: exit %d, want 1; findings %+v", code, state.Findings)
	}
	if len(findingsOf(state, ruleIDStarterModuleCycles)) == 0 {
		t.Errorf("no %s finding for the domain↔adapters cycle: %+v", ruleIDStarterModuleCycles, state.Findings)
	}
}

// TestOnboarding_Hexagonal_DeclaredLayersBlockDomainToAdapters: init guesses
// no layer, and once the owner declares the layers its how-to describes, the
// generated direction rule passes the clean service and blocks the canonical
// violation.
func TestOnboarding_Hexagonal_DeclaredLayersBlockDomainToAdapters(t *testing.T) {
	t.Parallel()
	root := writeOnboardingRepo(t, hexagonalFiles())
	cfgPath, generated := initAndLint(t, root)
	declareLayersInConfig(t, cfgPath, generated,
		[]string{testLayerDomain, testLayerApplication, testUpdateLayerAdapter, "entrypoint"},
		map[string]string{"domain": testLayerDomain, "app": testLayerApplication, "adapters": testUpdateLayerAdapter, "cmd_server": "entrypoint"})
	if code, stdout, stderr := runArchfit(t, "config", "lint", "-c", cfgPath); code != 0 {
		t.Fatalf("config lint after declaring layers: exit %d\nstdout:\n%s\nstderr:\n%s", code, stdout, stderr)
	}
	if code, state := checkStateOf(t, cfgPath); code == 1 {
		t.Fatalf("check on the clean layered service: exit 1, findings %+v", state.Findings)
	}

	writeFileAt(t, root, hexDomainToAdapterFile, hexDomainToAdapterSrc)
	code, state := checkStateOf(t, cfgPath)
	if code != 1 {
		t.Fatalf("check after domain→adapters import: exit %d, want 1; findings %+v", code, state.Findings)
	}
	if len(findingsOf(state, ruleIDStarterLayerRule)) == 0 {
		t.Errorf("no %s finding for domain importing an adapter: %+v", ruleIDStarterLayerRule, state.Findings)
	}
}

// goWorkFiles is a go.work monorepo with no root go.mod: two services and a
// shared library, each its own module.
func goWorkFiles() map[string]string {
	return map[string]string{
		"go.work":            "go 1.21\n\nuse (\n\t./lib/shared\n\t./svc/a\n\t./svc/b\n)\n",
		"lib/shared/go.mod":  "module example.com/lib/shared\n\ngo 1.21\n",
		"lib/shared/util.go": "package shared\n\nfunc Name() string { return \"shared\" }\n",
		"svc/a/go.mod":       "module example.com/svc/a\n\ngo 1.21\n",
		"svc/a/api/api.go":   "package api\n\nfunc Version() string { return \"1\" }\n",
		"svc/a/main.go": "package main\n\nimport \"example.com/lib/shared\"\n\n" +
			"func main() { println(shared.Name()) }\n",
		"svc/b/go.mod": "module example.com/svc/b\n\ngo 1.21\n",
		"svc/b/main.go": "package main\n\nimport \"example.com/lib/shared\"\n\n" +
			"func main() { println(shared.Name()) }\n",
	}
}

// TestOnboarding_GoWorkMonorepo_EnablesGoAndBlocksCycle: presence comes from
// the Go extractor's own member discovery, so a go.work-only monorepo is
// written with Go enabled and one module per member — not `enabled: false`
// with zero modules — and a cross-member module cycle blocks check.
func TestOnboarding_GoWorkMonorepo_EnablesGoAndBlocksCycle(t *testing.T) {
	t.Parallel()
	root := writeOnboardingRepo(t, goWorkFiles())
	cfgPath, generated := initAndLint(t, root)
	for _, want := range []string{
		"  go:\n    enabled: true\n",
		"  lib_shared:\n    paths:\n      - \"lib/shared/**\"\n",
		"  svc_a:\n    paths:\n      - \"svc/a/**\"\n",
		"  svc_b:\n    paths:\n      - \"svc/b/**\"\n",
		"type: module_cycle\n    gate: fail\n",
	} {
		if !strings.Contains(generated, want) {
			t.Errorf("generated config missing %q:\n%s", want, generated)
		}
	}
	if code, state := checkStateOf(t, cfgPath); code == 1 {
		t.Fatalf("check on the clean monorepo: exit 1, findings %+v", state.Findings)
	}

	// shared now reaches back into service a, which already imports shared.
	writeFileAt(t, root, "lib/shared/hook/hook.go", "package hook\n\nimport \"example.com/svc/a/api\"\n\n"+
		"func Version() string { return api.Version() }\n")
	code, state := checkStateOf(t, cfgPath)
	if code != 1 {
		t.Fatalf("check after the cross-member cycle: exit %d, want 1; findings %+v", code, state.Findings)
	}
	if len(findingsOf(state, ruleIDStarterModuleCycles)) == 0 {
		t.Errorf("no %s finding for svc_a ↔ lib_shared: %+v", ruleIDStarterModuleCycles, state.Findings)
	}
}

// curatedHexagonalConfig groups the hexagonal service more finely (one module
// per domain context and per adapter) and more coarsely (every entrypoint in
// one module) than discovery's two-segment directories.
const curatedHexagonalConfig = `version: 2
layers:
  - domain
  - application
  - adapter
  - entrypoint
modules:
  domain-order:
    paths: ["internal/domain/order/**"]
    layer: domain
    owner: team-orders
    subdomain: core
  domain-billing:
    paths: ["internal/domain/billing/**"]
    layer: domain
    owner: team-billing
    subdomain: core
  app:
    paths: ["internal/app/**"]
    layer: application
    owner: team-orders
    subdomain: supporting
  postgres:
    paths: ["internal/adapters/postgres/**"]
    layer: adapter
    owner: team-platform
    subdomain: generic
  web:
    paths: ["internal/adapters/http/**"]
    layer: adapter
    owner: team-platform
    subdomain: generic
  entrypoints:
    paths: ["cmd/**"]
    layer: entrypoint
    owner: team-platform
    subdomain: generic
    deploy_unit: server
rules:
  - id: no-module-cycles
    type: module_cycle
    gate: fail
  - id: no-layer-back-edges
    type: forbidden_layer_direction
    gate: fail
`

// TestOnboarding_ConfigUpdate_CuratedMapGetsNoCatchAlls: discovery's
// internal/domain/**, internal/adapters/** and cmd/server/** are owned
// entirely by the curated modules, so update proposes no catch-all stanza and
// reports no curated module as unmatched. Before ownership matching it added
// three stanzas that owned no file under most-specific matching.
func TestOnboarding_ConfigUpdate_CuratedMapGetsNoCatchAlls(t *testing.T) {
	t.Parallel()
	files := hexagonalFiles()
	files[defaultConfigPath] = curatedHexagonalConfig
	root := writeOnboardingRepo(t, files)
	cfgPath := filepath.Join(root, defaultConfigPath)

	code, stdout, stderr := runArchfit(t, "config", "update", "-c", cfgPath, "--json")
	if code != 0 {
		t.Fatalf("config update --json: exit %d\nstdout:\n%s\nstderr:\n%s", code, stdout, stderr)
	}
	var review initcfg.ConfigReview
	if err := json.Unmarshal([]byte(stdout), &review); err != nil {
		t.Fatalf("decode review: %v\n%s", err, stdout)
	}
	if len(review.Structure.AddedModules) != 0 {
		t.Errorf("added_modules = %v, want none (curated modules own every discovered package)", review.Structure.AddedModules)
	}
	if len(review.Structure.RemovedModules) != 0 || len(review.UncheckedModules) != 0 {
		t.Errorf("removed = %v, unchecked = %+v, want none (every curated module owns discovered code)",
			review.Structure.RemovedModules, review.UncheckedModules)
	}
	if review.Status != initcfg.ReviewStatusNoKnownIssues {
		t.Errorf("status = %q, want %q\n%s", review.Status, initcfg.ReviewStatusNoKnownIssues, stdout)
	}
}

// TestOnboarding_GeneratedConfigPassesLint_PerLanguage: the lint guarantee
// holds for every language init discovers, not only Go. Each init fixture
// gains trap trees discovery can see but the source inventory does not hold
// as production code; the generated config keeps the real modules, drops the
// traps, guesses no layer from a directory name, and passes config lint.
func TestOnboarding_GeneratedConfigPassesLint_PerLanguage(t *testing.T) {
	t.Parallel()
	cases := []struct {
		fixture string
		traps   map[string]string
		modules []string
		absent  []string
		wants   []string
	}{
		{
			fixture: "gofixture",
			traps: map[string]string{
				"mocks/engine.go":          "// Code generated by mockery; DO NOT EDIT.\n\npackage mocks\n",
				"internal/fake/fake.go":    "// Code generated by counterfeiter. DO NOT EDIT.\n\npackage fake\n",
				"internal/e2e/e2e_test.go": "package e2e\n",
			},
			modules: []string{fixtureModEngine, fixtureModModel},
			absent:  []string{"mocks/**", "internal/fake/**", "internal/e2e/**"},
		},
		{
			fixture: "tsfixture",
			traps: map[string]string{
				"src/assets/logo.svg":   "<svg/>\n",
				"src/__mocks__/api.ts":  "export const api = {};\n",
				"src/__tests__/core.ts": "export {};\n",
			},
			modules: []string{fixtureModAPI, fixtureModCore},
			absent:  []string{"src/assets/**", "src/__mocks__/**", "src/__tests__/**"},
		},
		{
			fixture: "pyfixture",
			traps: map[string]string{
				"src/pyfixture/tests/__init__.py":  "",
				"src/pyfixture/tests/test_core.py": "def test_core():\n    pass\n",
			},
			modules: []string{fixtureModAPI, fixtureModCore},
			absent:  []string{"pyfixture.tests"},
		},
		{
			fixture: fixtureRust,
			modules: []string{fixtureRust},
		},
		{
			// No shared fixture: Go packages and a root package.json over the
			// same src/ directories. Go and TypeScript discovery both propose
			// src/web/**; the TypeScript duplicate owns no file and is dropped
			// instead of tying (ambiguous_ownership) with the Go module.
			fixture: "go+typescript",
			traps: map[string]string{
				markerGoMod:      "module example.com/tie\n\ngo 1.21\n",
				"package.json":   "{\"name\":\"tie\",\"version\":\"0.0.0\"}\n",
				"src/web/web.go": "package web\n\nfunc Serve() {}\n",
				"src/web/app.ts": "export const app = 1;\n",
				"src/api/api.go": "package api\n\nfunc Version() string { return \"1\" }\n",
			},
			modules: []string{"src_web", "src_api"},
			absent:  []string{"src/web/**\"\n    public:\n      - \"src/web/**"},
			wants: []string{"  # Why: Go module(s) src_web also own typescript source the Go import graph omits.\n" +
				"  - id: " + ruleIDStarterModuleCycles + "\n    type: module_cycle\n    gate: warn\n"},
		},
	}
	for _, tc := range cases {
		t.Run(tc.fixture, func(t *testing.T) {
			t.Parallel()
			if tc.fixture == fixtureRust {
				if _, err := exec.LookPath("cargo"); err != nil {
					t.Skip("cargo not installed: Rust discovery proposes no crate module")
				}
			}
			files := map[string]string{}
			if !strings.Contains(tc.fixture, "+") {
				files = readFixtureTree(t, filepath.Join("..", "..", "internal", "initcfg", "testdata", tc.fixture))
			}
			for rel, content := range tc.traps {
				files[rel] = content
			}
			root := writeOnboardingRepo(t, files)
			_, generated := initAndLint(t, root)
			for _, name := range tc.modules {
				if !strings.Contains(generated, "\n  "+name+":\n    paths:\n") {
					t.Errorf("generated config has no module %q:\n%s", name, generated)
				}
			}
			for _, trap := range tc.absent {
				if strings.Contains(generated, "\""+trap+"\"") {
					t.Errorf("generated config declares %q, which holds no production source:\n%s", trap, generated)
				}
			}
			for _, want := range tc.wants {
				if !strings.Contains(generated, want) {
					t.Errorf("generated config missing %q:\n%s", want, generated)
				}
			}
			if strings.Contains(generated, "\nlayers:\n") || strings.Contains(generated, "    layer: ") {
				t.Errorf("generated config guesses layers from directory names:\n%s", generated)
			}
		})
	}
}

// TestOnboarding_InitOnRepoSubtree_KeepsModules: --root names a subtree of a
// larger git repository whose path holds a default-excluded segment
// (testdata/). Exclusions match below the scan root, so the inventory keeps
// the subtree's source and init keeps its modules.
func TestOnboarding_InitOnRepoSubtree_KeepsModules(t *testing.T) {
	t.Parallel()
	root, err := filepath.Abs(filepath.Join("..", "..", "internal", "initcfg", "testdata", "gofixture"))
	if err != nil {
		t.Fatal(err)
	}
	cfgPath := filepath.Join(t.TempDir(), defaultConfigPath)
	if code, stdout, stderr := runArchfit(t, "config", "init", "--root", root, "-o", cfgPath); code != 0 {
		t.Fatalf("config init: exit %d\nstdout:\n%s\nstderr:\n%s", code, stdout, stderr)
	}
	data, err := os.ReadFile(cfgPath) //#nosec G304 -- test temp dir
	if err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{fixtureModEngine, fixtureModModel} {
		if !strings.Contains(string(data), "\n  "+name+":\n    paths:\n") {
			t.Errorf("generated config has no module %q:\n%s", name, data)
		}
	}
	if code, stdout, stderr := runArchfit(t, "config", "lint", "-c", cfgPath, "--root", root); code != 0 {
		t.Fatalf("config lint: exit %d\nstdout:\n%s\nstderr:\n%s\nconfig:\n%s", code, stdout, stderr, data)
	}
}

// Module names the shared init fixtures discover.
const (
	fixtureModAPI    = "api"
	fixtureModCore   = "core"
	fixtureModEngine = "engine"
	fixtureModModel  = "model"
	fixtureRust      = "rustfixture"
)

// readFixtureTree reads every file under dir into a rel-path→content map.
func readFixtureTree(t *testing.T, dir string) map[string]string {
	t.Helper()
	files := map[string]string{}
	fsys := os.DirFS(dir)
	err := fs.WalkDir(fsys, ".", func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		data, err := fs.ReadFile(fsys, path)
		if err != nil {
			return err
		}
		files[path] = string(data)
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return files
}

// mocksAndGeneratedFiles is a Go service shaped like pumba: production
// packages, a mockery mocks/ package that imports one of them, a package of
// generated code outside mocks/, a test-only package, and a package under a
// default-excluded reports/ tree. go list sees all of them as packages; the
// source inventory check and config lint read holds only the production ones.
func mocksAndGeneratedFiles() map[string]string {
	return map[string]string{
		markerGoMod:              "module example.com/chaos\n\ngo 1.21\n",
		"pkg/chaos/chaos.go":     "package chaos\n\ntype Command interface{ Run() error }\n",
		"pkg/runtime/runtime.go": "package runtime\n\nimport \"example.com/chaos/pkg/chaos\"\n\nfunc Run(c chaos.Command) error { return c.Run() }\n",
		"cmd/main.go":            "package main\n\nimport \"example.com/chaos/pkg/runtime\"\n\nfunc main() { _ = runtime.Run(nil) }\n",
		"mocks/Command.go": "// Code generated by mockery; DO NOT EDIT.\n\npackage mocks\n\n" +
			"import \"example.com/chaos/pkg/chaos\"\n\ntype Command struct{ chaos.Command }\n",
		"api/gen/api.go":           "// Code generated by protoc-gen-go. DO NOT EDIT.\n\npackage gen\n\ntype Request struct{ ID string }\n",
		"test/e2e/e2e_test.go":     "package e2e\n\nimport \"testing\"\n\nfunc TestE2E(t *testing.T) {}\n",
		"reports/render/render.go": "package render\n\nfunc HTML() string { return \"\" }\n",
	}
}

// TestOnboarding_MocksAndGeneratedTrees_GeneratedConfigEvaluates: init writes
// no module for a tree the source inventory does not hold as production code
// (mocks/, generated, test-only, default-excluded), so the generated config
// passes lint and check evaluates the starter rule instead of listing it
// unevaluated forever. On pumba the mocks/ module failed lint
// (public_matches_nothing) and held no-module-cycles unevaluated.
func TestOnboarding_MocksAndGeneratedTrees_GeneratedConfigEvaluates(t *testing.T) {
	t.Parallel()
	root := writeOnboardingRepo(t, mocksAndGeneratedFiles())
	cfgPath, generated := initAndLint(t, root)
	for _, want := range []string{
		"  pkg_chaos:\n    paths:\n      - \"pkg/chaos/**\"\n",
		"  pkg_runtime:\n    paths:\n      - \"pkg/runtime/**\"\n",
		"  cmd:\n    paths:\n      - \"cmd/**\"\n",
		"  - id: " + ruleIDStarterModuleCycles + "\n    type: module_cycle\n    gate: fail\n",
	} {
		if !strings.Contains(generated, want) {
			t.Errorf("generated config missing %q:\n%s", want, generated)
		}
	}
	if strings.Contains(generated, "public:") {
		t.Errorf("generated Go config declares a public surface; the owner declares those:\n%s", generated)
	}
	for _, trap := range []string{"mocks", "api/gen", "test/e2e", "reports/render"} {
		if strings.Contains(generated, "\""+trap+"/**\"") {
			t.Errorf("generated config declares a module over %s/, which holds no production source:\n%s", trap, generated)
		}
	}

	code, state := checkStateOf(t, cfgPath)
	if code == 1 {
		t.Fatalf("check on the clean service: exit 1, findings %+v", state.Findings)
	}
	if len(state.Decision.UnevaluatedRequiredRules) != 0 {
		t.Errorf("unevaluated required rules = %+v, want none: every starter rule must be evaluable", state.Decision.UnevaluatedRequiredRules)
	}
}

// TestOnboarding_ConfigUpdate_ProposesNoModuleForMocks: config update reads
// the same inventory, so it never proposes the trees init leaves out.
func TestOnboarding_ConfigUpdate_ProposesNoModuleForMocks(t *testing.T) {
	t.Parallel()
	files := mocksAndGeneratedFiles()
	files[defaultConfigPath] = "version: 2\nmodules:\n  chaos:\n    paths: [\"pkg/chaos/**\"]\n"
	root := writeOnboardingRepo(t, files)
	cfgPath := filepath.Join(root, defaultConfigPath)

	code, stdout, stderr := runArchfit(t, "config", "update", "-c", cfgPath, "--json")
	if code != 0 {
		t.Fatalf("config update --json: exit %d\nstdout:\n%s\nstderr:\n%s", code, stdout, stderr)
	}
	var review initcfg.ConfigReview
	if err := json.Unmarshal([]byte(stdout), &review); err != nil {
		t.Fatalf("decode review: %v\n%s", err, stdout)
	}
	if got := strings.Join(review.Structure.AddedModules, ","); got != "cmd,pkg_runtime" {
		t.Errorf("added modules = %q, want cmd,pkg_runtime (no mocks, generated, test-only or excluded tree)\n%s", got, stdout)
	}
}
