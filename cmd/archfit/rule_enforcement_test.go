package main

import (
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"

	"github.com/alexei-led/archfit/v3/internal/model/report"
)

// Fixture repos for the rule-enforcement stream: each reproduces one shape the
// rule types used to get wrong, and runs the real `check` over it.

const (
	fixtureModBilling  = "billing"
	fixtureModShipping = "shipping"

	ruleIDAPIOnly        = "api_only"
	ruleIDInternalAccess = "internal_access"

	fileShipmentGo = "internal/shipping/domain/shipment.go"
	pkgBillingDom  = "internal/billing/domain"

	fixtureShopGoMod = "module example.com/shop\n\ngo 1.21\n"

	fileShippingAPIGo  = "internal/shipping/api/api.go"
	srcShipAPI         = "package api\n\nfunc Ship() int { return 2 }\n"
	ruleIDModuleCycles = "no_module_cycles"
)

// writeRuleFixtureRepo writes files into a fresh git repository and returns the
// path of its .archfit.yaml.
func writeRuleFixtureRepo(t *testing.T, files map[string]string) string {
	t.Helper()
	dir := t.TempDir()
	for rel, content := range files {
		writeFileAt(t, dir, rel, content)
	}
	gitInitFixtureRepo(t, dir)
	return filepath.Join(dir, defaultConfigPath)
}

// checkStateOf runs `archfit check --format json` and decodes the state.
func checkStateOf(t *testing.T, cfgPath string) (int, report.ArchitectureState) {
	t.Helper()
	code, stdout, stderr := runArchfit(t, cmdCheck, "-c", cfgPath, "--format="+formatJSON)
	if code == 3 {
		t.Fatalf("check: exit 3\nstdout:\n%s\nstderr:\n%s", stdout, stderr)
	}
	var state report.ArchitectureState
	if err := json.Unmarshal([]byte(stdout), &state); err != nil {
		t.Fatalf("decode state: %v\nstdout:\n%s\nstderr:\n%s", err, stdout, stderr)
	}
	return code, state
}

func findingsOf(state report.ArchitectureState, ruleID string) []report.Finding {
	var out []report.Finding
	for _, f := range state.Findings {
		if f.RuleID == ruleID {
			out = append(out, f)
		}
	}
	return out
}

// declaredSurfaceRules gates both internal-access rule types.
const declaredSurfaceRules = `rules:
  - id: api_only
    type: public_api_only
    gate: fail
  - id: internal_access
    type: internal_api_access
    gate: fail
`

// TestRun_Check_DeclaredInternalBypassFiresWithRootGoMod pins the root-go.mod
// half of the declared-surface fix. With go.mod at the scan root, the import
// path into a top-level internal/ tree is stripped to internal/..., so the Go
// extractor's `/internal/` segment never marks it, and a bypass of billing's
// declared surface used to pass both rules silently. The bypass now fires at its
// import line; the import through billing's public package does not.
func TestRun_Check_DeclaredInternalBypassFiresWithRootGoMod(t *testing.T) {
	t.Parallel()
	cfgPath := writeRuleFixtureRepo(t, map[string]string{
		markerGoMod: fixtureShopGoMod,
		"internal/billing/api/api.go": "package api\n\nimport \"example.com/shop/internal/billing/domain\"\n\n" +
			"func Charge() int { return domain.Total() }\n",
		"internal/billing/domain/domain.go": "package domain\n\nfunc Total() int { return 1 }\n",
		fileShipmentGo: "package domain\n\nimport \"example.com/shop/internal/billing/domain\"\n\n" +
			"func Cost() int { return domain.Total() }\n",
		fileShippingAPIGo: "package api\n\nimport billapi \"example.com/shop/internal/billing/api\"\n\n" +
			"func Quote() int { return billapi.Charge() }\n",
		defaultConfigPath: `version: 2
modules:
  billing:
    paths: ["internal/billing/**"]
    public: ["internal/billing/api"]
    internal: ["internal/billing/**"]
    owner: team-b
    subdomain: core
  shipping:
    paths: ["internal/shipping/**"]
    public: ["internal/shipping/api"]
    internal: ["internal/shipping/**"]
    owner: team-c
    subdomain: core
` + declaredSurfaceRules,
	})

	code, state := checkStateOf(t, cfgPath)
	if code != 1 {
		t.Fatalf("check exit = %d, want 1 (declared-internal bypass blocks)", code)
	}
	for _, ruleID := range []string{ruleIDAPIOnly, ruleIDInternalAccess} {
		got := findingsOf(state, ruleID)
		if len(got) != 1 {
			t.Fatalf("%s findings = %+v, want exactly the shipment.go bypass", ruleID, got)
		}
		f := got[0]
		if f.Edge.From.Path != fileShipmentGo || f.Edge.To.Path != pkgBillingDom {
			t.Errorf("%s edge = %+v, want %s -> %s", ruleID, f.Edge, fileShipmentGo, pkgBillingDom)
		}
		if len(f.Locations) != 1 || f.Locations[0].File != fileShipmentGo || f.Locations[0].Line != 3 {
			t.Errorf("%s locations = %+v, want %s:3", ruleID, f.Locations, fileShipmentGo)
		}
		if f.Kind != "gate" || f.Status != "new" {
			t.Errorf("%s kind/status = %s/%s, want gate/new", ruleID, f.Kind, f.Status)
		}
	}
}

// TestRun_Check_PublicImportsDoNotFireWithNestedGoMod pins the nested-go.mod
// half, run from the repository root as a user would. go.mod sits in svc/, so
// every `svc/internal/...` import carries the extractor's uses_internal kind,
// and both rules used to block every cross-module import, including the ones
// through the declared public packages. A declared public: glob now wins.
func TestRun_Check_PublicImportsDoNotFireWithNestedGoMod(t *testing.T) {
	t.Parallel()
	cfgPath := writeRuleFixtureRepo(t, map[string]string{
		"svc/go.mod": "module example.com/svc\n\ngo 1.21\n",
		"svc/cmd/server/main.go": "package main\n\nimport (\n" +
			"\tbillapi \"example.com/svc/internal/billing/api\"\n" +
			"\tshipapi \"example.com/svc/internal/shipping/api\"\n)\n\n" +
			"func main() {\n\t_ = billapi.Charge()\n\t_ = shipapi.Ship()\n}\n",
		"svc/internal/billing/api/api.go": "package api\n\nimport \"example.com/svc/internal/billing/domain\"\n\n" +
			"func Charge() int { return domain.Total() }\n",
		"svc/internal/billing/domain/domain.go": "package domain\n\nfunc Total() int { return 1 }\n",
		"svc/internal/shipping/api/api.go":      srcShipAPI,
		defaultConfigPath: `version: 2
modules:
  server:
    paths: ["svc/cmd/**"]
    owner: team-a
    subdomain: generic
  billing:
    paths: ["svc/internal/billing/**"]
    public: ["svc/internal/billing/api"]
    owner: team-b
    subdomain: core
  shipping:
    paths: ["svc/internal/shipping/**"]
    public: ["svc/internal/shipping/api"]
    owner: team-c
    subdomain: core
` + declaredSurfaceRules,
	})

	code, state := checkStateOf(t, cfgPath)
	if code == 1 {
		t.Fatalf("check exit = 1, want no blocker for imports through declared public packages; findings: %+v", state.Findings)
	}
	for _, ruleID := range []string{ruleIDAPIOnly, ruleIDInternalAccess} {
		if got := findingsOf(state, ruleID); len(got) != 0 {
			t.Errorf("%s findings = %+v, want none", ruleID, got)
		}
	}
}

// TestRun_Check_ModuleCycleFiresWhereNodeCycleCannot pins module_cycle on the
// shape the node-level cycle rule is blind to: compiling Go whose two declared
// modules depend on each other through different files and packages. Go edges
// run file -> package, so `cycle` finds nothing; module_cycle reports each
// direction at its import line, and explain names the module pair instead of
// printing an empty edge.
func TestRun_Check_ModuleCycleFiresWhereNodeCycleCannot(t *testing.T) {
	t.Parallel()
	const (
		notifyGo = "internal/billing/app/notify.go"
		billGo   = "internal/shipping/app/bill.go"
	)
	cfgPath := writeRuleFixtureRepo(t, map[string]string{
		markerGoMod: fixtureShopGoMod,
		notifyGo: "package app\n\nimport \"example.com/shop/internal/shipping/api\"\n\n" +
			"func Notify() int { return api.Ship() }\n",
		fileShippingAPIGo: srcShipAPI,
		billGo: "package app\n\nimport billing \"example.com/shop/internal/billing/app\"\n\n" +
			"func Bill() int { return billing.Notify() }\n",
		defaultConfigPath: `version: 2
modules:
  billing:
    paths: ["internal/billing/**"]
    owner: team-b
    subdomain: core
  shipping:
    paths: ["internal/shipping/**"]
    owner: team-c
    subdomain: core
rules:
  - id: no_cycles
    type: cycle
    gate: fail
  - id: no_module_cycles
    type: module_cycle
    gate: fail
`,
	})

	code, state := checkStateOf(t, cfgPath)
	if code != 1 {
		t.Fatalf("check exit = %d, want 1 (module cycle blocks)", code)
	}
	if got := findingsOf(state, "no_cycles"); len(got) != 0 {
		t.Errorf("node-level cycle findings = %+v, want none: Go file -> package edges cannot close one", got)
	}
	for _, rule := range state.Decision.UnevaluatedRequiredRules {
		if rule.RuleID == ruleIDModuleCycles {
			t.Errorf("module_cycle listed unevaluated: %s", rule.Reason)
		}
	}
	got := findingsOf(state, ruleIDModuleCycles)
	want := []struct{ from, to, file string }{
		{fixtureModBilling, fixtureModShipping, notifyGo},
		{fixtureModShipping, fixtureModBilling, billGo},
	}
	if len(got) != len(want) {
		t.Fatalf("module_cycle findings = %+v, want one per direction", got)
	}
	for i, w := range want {
		f := got[i]
		if f.Edge.From.Module != w.from || f.Edge.To.Module != w.to || f.Edge.From.Path != "" || f.Edge.To.Path != "" || f.Edge.Kind != "module_dependency" {
			t.Errorf("finding %d edge = %+v, want module pair %s -> %s with empty paths", i, f.Edge, w.from, w.to)
		}
		if len(f.Locations) != 1 || f.Locations[0].File != w.file || f.Locations[0].Line != 3 {
			t.Errorf("finding %d locations = %+v, want %s:3", i, f.Locations, w.file)
		}
	}
	tasks := 0
	for _, task := range state.AgentTasks {
		if task.RuleID == ruleIDModuleCycles {
			tasks++
			if len(task.Files) != 1 {
				t.Errorf("agent task files = %v, want the import site", task.Files)
			}
		}
	}
	if tasks != 2 {
		t.Errorf("module_cycle agent tasks = %d, want 2", tasks)
	}

	code, stdout, stderr := runArchfit(t, cmdExplain, got[0].ID, "-c", cfgPath)
	if code != 0 {
		t.Fatalf("explain exit = %d\nstdout:\n%s\nstderr:\n%s", code, stdout, stderr)
	}
	if !strings.Contains(stdout, "modules:    billing -> shipping") || strings.Contains(stdout, "edge:       ->") {
		t.Errorf("explain output names no module pair or prints an empty edge:\n%s", stdout)
	}
}

// TestRun_Check_ModuleCycleIgnoresNonProductionSources pins the production
// scope end to end: the source inventory's file classes reach module_cycle, so
// a module cycle that closes only through generated code is not reported.
func TestRun_Check_ModuleCycleIgnoresNonProductionSources(t *testing.T) {
	t.Parallel()
	cfgPath := writeRuleFixtureRepo(t, map[string]string{
		markerGoMod: fixtureShopGoMod,
		"internal/billing/app/notify.go": "package app\n\nimport \"example.com/shop/internal/shipping/api\"\n\n" +
			"func Notify() int { return api.Ship() }\n",
		fileShippingAPIGo: srcShipAPI,
		"internal/shipping/app/zz_generated.go": "// Code generated by stubgen. DO NOT EDIT.\n\npackage app\n\n" +
			"import billing \"example.com/shop/internal/billing/app\"\n\nfunc Bill() int { return billing.Notify() }\n",
		defaultConfigPath: `version: 2
modules:
  billing:
    paths: ["internal/billing/**"]
    owner: team-b
    subdomain: core
  shipping:
    paths: ["internal/shipping/**"]
    owner: team-c
    subdomain: core
rules:
  - id: no_module_cycles
    type: module_cycle
    gate: fail
`,
	})
	code, state := checkStateOf(t, cfgPath)
	if got := findingsOf(state, ruleIDModuleCycles); len(got) != 0 || code == 1 {
		t.Fatalf("check exit = %d with module_cycle findings %+v, want none: the back edge is generated code", code, got)
	}
	for _, rule := range state.Decision.UnevaluatedRequiredRules {
		if rule.RuleID == ruleIDModuleCycles {
			t.Errorf("module_cycle listed unevaluated: %s", rule.Reason)
		}
	}
}

// TestRun_Check_ExistingInternalAccessFindingKeepsItsID pins baseline
// compatibility end to end: the coupled fixture's uses_internal edge keeps the
// fingerprint the engine assigned before declared surfaces decided (captured
// from the unmodified engine), so accepted baseline entries do not re-key.
func TestRun_Check_ExistingInternalAccessFindingKeepsItsID(t *testing.T) {
	t.Parallel()
	_, state := checkStateOf(t, writeCoupledRepo(t, coupledModulesCfg+declaredSurfaceRules))

	want := map[string]string{
		ruleIDAPIOnly:        "f810d2b178af5bf73b38ef270b476e1a",
		ruleIDInternalAccess: "740c5c4d955ce6e174ae4bf7d673a8a8", // gitleaks:allow — a finding fingerprint, not a secret
	}
	for ruleID, wantID := range want {
		got := findingsOf(state, ruleID)
		if len(got) != 1 || got[0].ID != wantID {
			t.Errorf("%s findings = %+v, want one finding with the pre-change ID %s", ruleID, got, wantID)
		}
	}
}
