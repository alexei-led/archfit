package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

const (
	findingKindGate   = "gate"
	agreeApp          = "app"
	agreeWeb          = "web"
	agreeAppFile      = "internal/app/app.go"
	agreeAppPkg       = "internal/app"
	agreeDomainPkg    = "internal/domain"
	agreeWebOK        = "internal/web/ok.go"
	agreeDomainBad    = "internal/domain/bad.go"
	agreeInfraPkg     = "internal/infra"
	agreePyImporter   = "tools/a.py"
	answerUnconstrain = "unconstrained"
	findingStatusNew  = "new"
)

const agreementCfg = `version: 2
layers: [domain, application, infrastructure]
modules:
  domain:
    paths: ["internal/domain/**"]
    layer: domain
    visible_to: [app, web]
  app:
    paths: ["internal/app/**"]
    layer: application
    depends_on: [domain]
  infra:
    paths: ["internal/infra/**"]
    public: ["internal/infra"]
    internal: ["internal/infra/impl/**"]
    layer: infrastructure
  # web is declared by its package dir: a file resolves to it only through
  # ModuleForFile, as check resolves it.
  web:
    paths: ["internal/web"]
rules:
  - id: layer_order
    type: forbidden_layer_direction
    gate: fail
  - id: allowlists
    type: module_dependencies
    gate: fail
  - id: public_only
    type: public_api_only
    gate: fail
  - id: web_not_infra
    type: forbidden_dependency
    gate: fail
    from_module: web
    to_module: infra
  - id: web_not_domain
    type: forbidden_dependency
    gate: fail
    from: internal/web/**
    to: internal/domain
`

// agreementImport is one import the fixture holds, with the answer
// can-import must give for it.
type agreementImport struct {
	file, target, pkg, want string
}

var agreementImports = []agreementImport{
	{file: agreeDomainBad, target: agreeInfraPkg, pkg: testLayerDomain, want: answerDenied},
	{file: agreeAppFile, target: agreeDomainPkg, pkg: agreeApp, want: answerAllowed},
	{file: "internal/app/infra.go", target: agreeInfraPkg, pkg: agreeApp, want: answerDenied},
	{file: "internal/web/web.go", target: "internal/infra/impl", pkg: agreeWeb, want: answerDenied},
	{file: "internal/web/db.go", target: agreeDomainPkg, pkg: agreeWeb, want: answerDenied},
	{file: agreeWebOK, target: agreeAppPkg, pkg: agreeWeb, want: answerUnconstrain},
}

// writeAgreementRepo writes a Go module whose files hold exactly the
// agreementImports, plus a leaf package for every target.
func writeAgreementRepo(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	files := map[string]string{
		markerGoMod:                   "module example.com/agree\n\ngo 1.21\n",
		defaultConfigPath:             agreementCfg,
		"internal/domain/domain.go":   "package domain\n\nconst Name = \"domain\"\n",
		"internal/app/name.go":        "package app\n\nconst Name = \"app\"\n",
		"internal/infra/infra.go":     "package infra\n\nconst Name = \"infra\"\n",
		"internal/infra/impl/impl.go": "package impl\n\nconst Name = \"impl\"\n",
	}
	for _, imp := range agreementImports {
		alias := filepath.Base(imp.target)
		files[imp.file] = "package " + imp.pkg + "\n\nimport " + alias + "x \"example.com/agree/" + imp.target + "\"\n\n" +
			"var _ = " + alias + "x.Name\n"
	}
	for name, content := range files {
		path := filepath.Join(dir, name)
		if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	gitInitFixtureRepo(t, dir)
	return filepath.Join(dir, defaultConfigPath)
}

type policyAnswerDoc struct {
	SchemaVersion string `json:"schema_version"`
	Answers       []struct {
		Answer  string   `json:"answer"`
		Next    string   `json:"next"`
		Reasons []string `json:"reasons"`
		Denials []struct {
			FindingID string `json:"finding_id"`
			RuleID    string `json:"rule_id"`
			Goal      string `json:"goal"`
		} `json:"denials"`
	} `json:"answers"`
}

func canImport(t *testing.T, cfgPath string, args ...string) (int, policyAnswerDoc) {
	t.Helper()
	code, stdout, stderr := runArchfit(t, append([]string{"policy", "can-import", "-c", cfgPath}, args...)...)
	var doc policyAnswerDoc
	if code != 3 {
		if err := json.Unmarshal([]byte(stdout), &doc); err != nil {
			t.Fatalf("decode can-import answer: %v\nstdout:\n%s\nstderr:\n%s", err, stdout, stderr)
		}
	}
	return code, doc
}

// TestPolicyCanImportAgreesWithCheck is the agreement test: can-import
// denies exactly the imports check reports as active gate findings, in both
// directions, with the same rule IDs and finding IDs. The query runs before
// check, so it is proven not to need the fact cache check writes.
func TestPolicyCanImportAgreesWithCheck(t *testing.T) {
	t.Parallel()
	cfgPath := writeAgreementRepo(t)
	root := filepath.Dir(cfgPath)

	denied := map[string]string{} // finding ID -> rule ID, from can-import
	for _, imp := range agreementImports {
		code, doc := canImport(t, cfgPath, imp.file, imp.target)
		if len(doc.Answers) != 1 || doc.Answers[0].Answer != imp.want {
			t.Errorf("can-import %s %s = %+v, want %s", imp.file, imp.target, doc.Answers, imp.want)
			continue
		}
		if imp.want == answerUnconstrain && !strings.Contains(doc.Answers[0].Next, "not permission") {
			t.Errorf("next for an unconstrained import must say it is not permission: %q", doc.Answers[0].Next)
		}
		wantCode := 0
		if imp.want == answerDenied {
			wantCode = 1
		}
		if code != wantCode {
			t.Errorf("can-import %s %s exit = %d, want %d", imp.file, imp.target, code, wantCode)
		}
		for _, d := range doc.Answers[0].Denials {
			denied[d.FindingID] = d.RuleID
			if d.Goal == "" {
				t.Errorf("denial %s of %s carries no repair goal", d.FindingID, imp.file)
			}
		}
	}
	if _, err := os.Stat(filepath.Join(root, ".archfit-cache")); !os.IsNotExist(err) {
		t.Errorf("can-import created or found .archfit-cache (err=%v): the query must not touch the fact cache", err)
	}

	code, stdout, stderr := runArchfit(t, cmdCheck, "-c", cfgPath, "--progress=none", "--format=json")
	if code != 1 {
		t.Fatalf("check exit = %d, want 1\nstderr:\n%s", code, stderr)
	}
	var state struct {
		Findings []struct {
			ID     string `json:"id"`
			RuleID string `json:"rule_id"`
			Kind   string `json:"kind"`
			Status string `json:"status"`
		} `json:"findings"`
	}
	if err := json.Unmarshal([]byte(stdout), &state); err != nil {
		t.Fatalf("decode state: %v", err)
	}
	gates := map[string]string{}
	for _, f := range state.Findings {
		if f.Kind == findingKindGate && f.Status == findingStatusNew {
			gates[f.ID] = f.RuleID
		}
	}
	if len(gates) == 0 {
		t.Fatal("check reported no gate finding: the agreement is vacuous")
	}
	for id, rule := range gates {
		if denied[id] != rule {
			t.Errorf("check gate finding %s (%s) is not denied by can-import (got %q)", id, rule, denied[id])
		}
	}
	for id, rule := range denied {
		if gates[id] != rule {
			t.Errorf("can-import denial %s (%s) is not a check gate finding", id, rule)
		}
	}
}

func TestPolicyCanImportExitCodes(t *testing.T) {
	t.Parallel()
	cfgPath := writeAgreementRepo(t)
	for _, tc := range []struct {
		name string
		args []string
		cfg  string
		want int
	}{
		{name: "denied", args: []string{agreeDomainBad, agreeInfraPkg}, want: 1},
		{name: "one denied target among two", args: []string{agreeWebOK, agreeAppPkg, agreeDomainPkg}, want: 1},
		{name: answerUnconstrain, args: []string{agreeWebOK, agreeAppPkg}, want: 0},
		{name: "rust needs its analyzer", args: []string{"crates/core/src/lib.rs", "my_core"}, want: 2},
		{name: "import path is stripped", args: []string{agreeDomainBad, "example.com/agree/internal/infra"}, want: 1},
		{name: "not a source file", args: []string{"README.md", agreeAppPkg}, want: 3},
		{name: "missing config", args: []string{"a.go", "b"}, cfg: filepath.Join(t.TempDir(), "nope.yaml"), want: 3},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			cfg := cfgPath
			if tc.cfg != "" {
				cfg = tc.cfg
			}
			if code, doc := canImport(t, cfg, tc.args...); code != tc.want {
				t.Errorf("exit = %d, want %d: %+v", code, tc.want, doc)
			}
		})
	}
}

func TestPolicyCanImportNotDecidedByWholeGraphRules(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name, prefix, rules string
		args                []string
		wantCode            int
		wantAnswer, reason  string
	}{
		{name: "module_cycle on a cross-module edge", rules: "  - id: no_cycles\n    type: module_cycle\n    gate: fail\n",
			args: []string{agreeAppFile, agreeDomainPkg}, wantCode: 2, wantAnswer: answerNotDecided, reason: "no_cycles"},
		{name: "module_cycle from a package-dir module", rules: "  - id: no_cycles\n    type: module_cycle\n    gate: fail\n",
			args: []string{agreeWebOK, agreeAppPkg}, wantCode: 2, wantAnswer: answerNotDecided, reason: "no_cycles"},
		{name: "external Python import", args: []string{agreePyImporter, "requests"}, wantCode: 2, wantAnswer: answerNotDecided, reason: "first-party"},
		{name: "excluded Python importer is still extracted", prefix: "exclude: [\"tools/**\"]\n",
			rules: "  - id: no_tools_b\n    type: forbidden_dependency\n    gate: fail\n    from: \"tools.*\"\n    to: tools.b\n",
			args:  []string{agreePyImporter, "tools.b"}, wantCode: 1, wantAnswer: answerDenied},
		{name: "warn-gated module_cycle decides nothing", rules: "  - id: no_cycles\n    type: module_cycle\n    gate: warn\n",
			args: []string{agreeAppFile, agreeDomainPkg}, wantCode: 0, wantAnswer: answerAllowed, reason: "depends_on"},
		{name: "seam gate in mode fail", prefix: "coupling:\n  gate:\n    distributed_monolith:\n      mode: fail\n",
			args: []string{agreeAppFile, agreeDomainPkg}, wantCode: 2, wantAnswer: answerNotDecided, reason: "seam gate"},
		{name: "node cycle on a Python import", rules: "  - id: no_node_cycles\n    type: cycle\n    gate: fail\n",
			args: []string{agreePyImporter, "tools.b"}, wantCode: 2, wantAnswer: answerNotDecided, reason: "no_node_cycles"},
		{name: "node cycle never forms on Go", rules: "  - id: no_node_cycles\n    type: cycle\n    gate: fail\n",
			args: []string{agreeAppFile, agreeDomainPkg}, wantCode: 0, wantAnswer: answerAllowed, reason: "depends_on"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			cfgPath := writeAgreementRepo(t)
			body, err := os.ReadFile(cfgPath) //nolint:gosec // test temp path
			if err != nil {
				t.Fatal(err)
			}
			body = append(append([]byte(tc.prefix), body...), tc.rules...)
			if err := os.WriteFile(cfgPath, //nolint:gosec // test temp path
				body, 0o600); err != nil {
				t.Fatal(err)
			}
			code, doc := canImport(t, cfgPath, tc.args...)
			if code != tc.wantCode || len(doc.Answers) != 1 || doc.Answers[0].Answer != tc.wantAnswer {
				t.Fatalf("exit %d answer %+v, want %d and %s", code, doc.Answers, tc.wantCode, tc.wantAnswer)
			}
			if tc.reason != "" && !slices.ContainsFunc(doc.Answers[0].Reasons, func(r string) bool { return strings.Contains(r, tc.reason) }) {
				t.Errorf("reasons %v do not name %q", doc.Answers[0].Reasons, tc.reason)
			}
		})
	}
}

// TestPolicyCanImportAppliesTheBaseline pins that accepted debt is not a
// denial: check does not block a baselined finding, so can-import does not
// either, and it lists the finding as accepted rather than calling the edge
// allowed.
func TestPolicyCanImportAppliesTheBaseline(t *testing.T) {
	t.Parallel()
	cfgPath := writeAgreementRepo(t)
	if code, _, stderr := runArchfit(t, cmdBaseline, "-c", cfgPath); code != 0 {
		t.Fatalf("baseline exit = %d\n%s", code, stderr)
	}
	code, stdout, stderr := runArchfit(t, "policy", "can-import", "-c", cfgPath, agreeDomainBad, agreeInfraPkg)
	if code != 0 {
		t.Fatalf("exit = %d, want 0 for accepted debt\nstdout:\n%s\nstderr:\n%s", code, stdout, stderr)
	}
	var doc struct {
		Answers []struct {
			Answer   string `json:"answer"`
			Accepted []struct {
				Status string `json:"status"`
			} `json:"accepted"`
		} `json:"answers"`
	}
	if err := json.Unmarshal([]byte(stdout), &doc); err != nil {
		t.Fatal(err)
	}
	got := doc.Answers[0]
	if got.Answer != answerUnconstrain || len(got.Accepted) != 1 || got.Accepted[0].Status != "baseline" {
		t.Errorf("answer %+v, want unconstrained with one baseline finding", got)
	}
}

func TestPolicyCanImportOutOfScope(t *testing.T) {
	t.Parallel()
	cfgPath := writeAgreementRepo(t)
	body, err := os.ReadFile(cfgPath) //nolint:gosec // test temp path
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(cfgPath, //nolint:gosec // test temp path
		append([]byte("exclude: [\"internal/domain/**\"]\n"), body...), 0o600); err != nil {
		t.Fatal(err)
	}
	code, doc := canImport(t, cfgPath, agreeDomainBad, agreeInfraPkg)
	if code != 0 || doc.Answers[0].Answer != answerUnconstrain || len(doc.Answers[0].Reasons) != 1 {
		t.Fatalf("exit %d answer %+v, want 0 and unconstrained with the scope reason", code, doc.Answers)
	}
}

func TestPolicyCanImportPathSpellings(t *testing.T) {
	t.Parallel()
	cfgPath := writeAgreementRepo(t)
	root := filepath.Dir(cfgPath)
	sub := filepath.Join(root, "policy")
	if err := os.MkdirAll(sub, 0o750); err != nil {
		t.Fatal(err)
	}
	body, err := os.ReadFile(cfgPath) //nolint:gosec // test temp path
	if err != nil {
		t.Fatal(err)
	}
	subCfg := filepath.Join(sub, defaultConfigPath)
	if err := os.WriteFile(subCfg, body, 0o600); err != nil { //nolint:gosec // test temp path
		t.Fatal(err)
	}
	for _, tc := range []struct {
		name, cfg, from string
	}{
		{name: "relative", cfg: cfgPath, from: agreeDomainBad},
		{name: "absolute", cfg: cfgPath, from: filepath.Join(root, filepath.FromSlash(agreeDomainBad))},
		{name: "config below the git root", cfg: subCfg, from: agreeDomainBad},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			if code, doc := canImport(t, tc.cfg, tc.from, agreeInfraPkg); code != 1 {
				t.Errorf("exit = %d, want 1 (denied): %+v", code, doc.Answers)
			}
		})
	}
}

func TestPolicyWhere(t *testing.T) {
	t.Parallel()
	cfgPath := writeAgreementRepo(t)
	code, stdout, stderr := runArchfit(t, "policy", "where", "-c", cfgPath, "internal/app/infra.go", "docs/readme.md")
	if code != 0 {
		t.Fatalf("where exit = %d\nstderr:\n%s", code, stderr)
	}
	var doc struct {
		Paths []struct {
			Path      string   `json:"path"`
			Module    string   `json:"module"`
			Layer     string   `json:"layer"`
			DependsOn []string `json:"depends_on"`
			Rules     []struct {
				ID    string `json:"id"`
				Match string `json:"match"`
			} `json:"rules"`
		} `json:"paths"`
	}
	if err := json.Unmarshal([]byte(stdout), &doc); err != nil {
		t.Fatalf("decode: %v\n%s", err, stdout)
	}
	app := doc.Paths[0]
	if app.Module != agreeApp || app.Layer != testLayerApplication || !slices.Equal(app.DependsOn, []string{testLayerDomain}) {
		t.Errorf("where internal/app/infra.go = %+v", app)
	}
	ruleIDs := make([]string, 0, len(app.Rules))
	for _, r := range app.Rules {
		ruleIDs = append(ruleIDs, r.ID)
	}
	if want := []string{"layer_order", "allowlists"}; !slices.Equal(ruleIDs, want) {
		t.Errorf("rules = %v, want %v", ruleIDs, want)
	}
	if doc.Paths[1].Module != "" || len(doc.Paths[1].Rules) != 0 {
		t.Errorf("an unowned path has a module or rules: %+v", doc.Paths[1])
	}
}
