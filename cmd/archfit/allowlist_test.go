package main

import (
	"strings"
	"testing"

	"github.com/alexei-led/archfit/internal/model/report"
)

// Module-allowlist fixtures: billing is visible only to shipping, shipping may
// depend only on billing, catalog declares nothing. Every case runs the real
// `check` over a compiling Go repository.

const (
	ruleIDBoundaries   = "boundaries"
	fixtureModCatalog  = "catalog"
	fileBillingAPIGo   = "internal/billing/api/api.go"
	fileShippingShipGo = "internal/shipping/app/ship.go"
	srcBillAPI         = "package api\n\nfunc Bill() int { return 1 }\n"
	srcImportsBilling  = "import \"example.com/shop/internal/billing/api\"\n\nfunc Use() int { return api.Bill() }\n"
	allowlistConfig    = `version: 2
modules:
  billing:
    paths: ["internal/billing/**"]
    public: ["internal/billing/api"]
    visible_to: [shipping]
    owner: team-b
  shipping:
    paths: ["internal/shipping/**"]
    depends_on: [billing]
    owner: team-c
  catalog:
    paths: ["internal/catalog/**"]
    owner: team-d
rules:
  - id: boundaries
    type: module_dependencies
    gate: fail
`
)

// goFileImportingBilling is a Go file of package pkg that imports billing's API.
func goFileImportingBilling(pkg string) string {
	return "package " + pkg + "\n\n" + srcImportsBilling
}

// allowlistRepo writes the allowlist fixture plus extra files, with config as
// the .archfit.yaml body.
func allowlistRepo(t *testing.T, config string, extra map[string]string) string {
	t.Helper()
	files := map[string]string{
		markerGoMod:        fixtureShopGoMod,
		fileBillingAPIGo:   srcBillAPI,
		fileShippingShipGo: goFileImportingBilling("app"),
		defaultConfigPath:  config,
	}
	for path, src := range extra {
		files[path] = src
	}
	return writeRuleFixtureRepo(t, files)
}

// TestRun_Check_ModuleAllowlistBlocksANewPairOnly pins the allowlist end to
// end: an import on an allowed pair, however many files make it, keeps check
// off exit 1; a new module pair or an importer no module owns blocks with one
// finding per pair, a repair task that does not route through billing's public
// API, and an ID that a file move inside the importing package keeps; a waiver
// on the module pair releases it.
func TestRun_Check_ModuleAllowlistBlocksANewPairOnly(t *testing.T) {
	t.Parallel()
	const (
		catalogFile = "internal/catalog/app/list.go"
		movedFile   = "internal/catalog/app/zz_moved.go"
		toolFile    = "tools/gen/main.go"
	)
	waiver := `waivers:
  - rule: boundaries
    from: catalog
    to: billing
    reason: staged migration
    approved_by: architecture-owner
    expires: "2099-01-01"
`
	tests := []struct {
		name         string
		config       string
		extra        map[string]string
		wantBlocked  bool
		wantFrom     string // from module, or from path for an unowned importer
		wantViolates string
	}{
		{name: "allowed pair", config: allowlistConfig},
		{name: "second file on the allowed pair", config: allowlistConfig,
			extra: map[string]string{"internal/shipping/app/track.go": goFileImportingBilling("app")}},
		{name: "new module pair", config: allowlistConfig,
			extra:       map[string]string{catalogFile: goFileImportingBilling("app")},
			wantBlocked: true, wantFrom: fixtureModCatalog, wantViolates: "visible_to"},
		{name: "unowned importer", config: allowlistConfig,
			extra:       map[string]string{toolFile: goFileImportingBilling("main") + "\nfunc main() { _ = Use() }\n"},
			wantBlocked: true, wantFrom: "tools/gen", wantViolates: "visible_to"},
		{name: "waived module pair", config: allowlistConfig + waiver,
			extra: map[string]string{catalogFile: goFileImportingBilling("app")}},
	}
	ids := map[string]string{}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			code, state := checkStateOf(t, allowlistRepo(t, tc.config, tc.extra))
			got := findingsOf(state, ruleIDBoundaries)
			if !tc.wantBlocked {
				if code == 1 {
					t.Fatalf("check exit = 1, want no block; findings = %+v", got)
				}
				for _, f := range got {
					if f.Status != report.FindingStatusWaived {
						t.Errorf("active finding %+v, want none", f)
					}
				}
				return
			}
			if code != 1 {
				t.Fatalf("check exit = %d, want 1", code)
			}
			if len(got) != 1 {
				t.Fatalf("findings = %+v, want one", got)
			}
			f := got[0]
			from := f.Edge.From.Module
			if from == "" {
				from = f.Edge.From.Path
			}
			if from != tc.wantFrom || f.Edge.To.Module != fixtureModBilling || f.MatchedBy["violates"] != tc.wantViolates {
				t.Errorf("finding edge = %+v violates %q, want %s -> billing violating %s", f.Edge, f.MatchedBy["violates"], tc.wantFrom, tc.wantViolates)
			}
			ids[tc.name] = f.ID
			for _, task := range state.AgentTasks {
				if task.FindingID != f.ID {
					continue
				}
				if strings.Contains(strings.Join(task.Constraints, "\n"), "internal/billing/api") {
					t.Errorf("task constraints route through billing's public API: %q", task.Constraints)
				}
				if len(task.Files) == 0 {
					t.Error("task names no file to repair")
				}
			}
		})
	}

	t.Run("file move keeps the finding ID", func(t *testing.T) {
		_, state := checkStateOf(t, allowlistRepo(t, allowlistConfig, map[string]string{movedFile: goFileImportingBilling("app")}))
		got := findingsOf(state, ruleIDBoundaries)
		if len(got) != 1 {
			t.Fatalf("findings = %+v, want one", got)
		}
		if want := ids["new module pair"]; want != "" && got[0].ID != want {
			t.Errorf("finding ID = %s after the move, want %s", got[0].ID, want)
		}
	})
}

// TestRun_ConfigLint_ReportsUnknownAllowlistModules pins that an allowlist
// entry naming no module loads, is a lint error, and is a check config warning.
func TestRun_ConfigLint_ReportsUnknownAllowlistModules(t *testing.T) {
	t.Parallel()
	cfgPath := allowlistRepo(t, strings.Replace(allowlistConfig, "visible_to: [shipping]", "visible_to: [warehouse]", 1), nil)
	code, stdout, stderr := runArchfit(t, "config", "lint", "-c", cfgPath)
	if code != 1 || !strings.Contains(stdout+stderr, `modules.billing.visible_to[0]`) || !strings.Contains(stdout+stderr, "unknown_module") {
		t.Errorf("config lint exit = %d, want 1 naming modules.billing.visible_to[0] unknown_module\nstdout:\n%s\nstderr:\n%s", code, stdout, stderr)
	}
	code, _, stderr = runArchfit(t, cmdCheck, "-c", cfgPath)
	if code == 3 || !strings.Contains(stderr, `visible_to entry "warehouse" names no declared module`) {
		t.Errorf("check exit = %d, want a config warning naming the entry\nstderr:\n%s", code, stderr)
	}
}
