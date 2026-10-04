package main

import (
	"encoding/json"
	"os"
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
// holds for every language init discovers, not only Go — including the
// Python fixture, whose sub-packages carry inferred layers and a live
// direction rule.
func TestOnboarding_GeneratedConfigPassesLint_PerLanguage(t *testing.T) {
	t.Parallel()
	for _, fixture := range []string{"tsfixture", "pyfixture", "rustfixture"} {
		t.Run(fixture, func(t *testing.T) {
			t.Parallel()
			root, err := filepath.Abs(filepath.Join("..", "..", "internal", "initcfg", "testdata", fixture))
			if err != nil {
				t.Fatal(err)
			}
			cfgPath := filepath.Join(t.TempDir(), defaultConfigPath)
			if code, stdout, stderr := runArchfit(t, "config", "init", "--root", root, "-o", cfgPath); code != 0 {
				t.Fatalf("config init: exit %d\nstdout:\n%s\nstderr:\n%s", code, stdout, stderr)
			}
			if code, stdout, stderr := runArchfit(t, "config", "lint", "-c", cfgPath, "--root", root); code != 0 {
				data, _ := os.ReadFile(cfgPath) //#nosec G304 -- test temp dir
				t.Fatalf("config lint: exit %d\nstdout:\n%s\nstderr:\n%s\nconfig:\n%s", code, stdout, stderr, data)
			}
		})
	}
}
