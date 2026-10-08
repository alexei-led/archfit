package main

import (
	"encoding/json"
	"fmt"
	"maps"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/alexei-led/archfit/v3/internal/model/report"
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

	// The fail-gated rules of agreementCfg, one per edge class.
	ruleAllowlists   = "allowlists"
	ruleLayerOrder   = "layer_order"
	rulePublicOnly   = "public_only"
	ruleWebNotDomain = "web_not_domain"
	ruleWebNotInfra  = "web_not_infra"
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
	return writeAgreementFiles(t, dir, files)
}

// writeAgreementFiles writes files under dir, makes it a git repo, and
// returns the path of its config.
func writeAgreementFiles(t *testing.T, dir string, files map[string]string) string {
	t.Helper()
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

// agreementEdgeClasses names the rule of each edge class the agreement fixture
// must exercise. A class whose rule check never fires on is not compared, so
// deleting a rule from agreementCfg would narrow the gate in silence.
var agreementEdgeClasses = map[string]string{
	"allowlist":                   ruleAllowlists,
	"layer order":                 ruleLayerOrder,
	"internal/public surface":     rulePublicOnly,
	"forbidden_dependency path":   ruleWebNotDomain,
	"forbidden_dependency module": ruleWebNotInfra,
}

// TestErosion_PolicyQueryAgreesWithCheck (policy_query_agreement) asserts
// can-import denies exactly the imports check reports as active gate findings,
// in both directions, with the same rule IDs and finding IDs, over every edge
// class in agreementEdgeClasses. The query runs before check, so it is proven
// not to need the fact cache check writes.
//
// A pre-edit answer that disagrees with the gate is worse than no answer: an
// agent that asked first and was told "allowed" writes the import, and check
// then blocks it. The fixture is Go; TypeScript and Python run the same
// comparison in policy_agreement_lang_test.go.
func TestErosion_PolicyQueryAgreesWithCheck(t *testing.T) {
	t.Parallel()
	assertAgreement(t, writeAgreementRepo(t), agreementImports, agreementEdgeClasses)
}

// assertAgreement runs can-import over every import and check over the repo
// behind cfgPath, and reports each place their decisions differ.
func assertAgreement(t *testing.T, cfgPath string, imports []agreementImport, classes map[string]string) {
	t.Helper()
	root := filepath.Dir(cfgPath)

	denied := map[string]string{} // finding ID -> rule ID, from can-import
	for _, imp := range imports {
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
		// The statuses JudgeEdge denies on: what blocks check blocks the query.
		if f.Kind == findingKindGate && (f.Status == findingStatusNew || f.Status == report.FindingStatusExpiredWaiver) {
			gates[f.ID] = f.RuleID
		}
	}
	for _, problem := range agreementProblems(gates, denied, classes) {
		t.Error(problem)
	}
}

// TestErosion_PolicyQueryAgreementFiresOnAWrongDecision proves the predicate
// above reports a query that decides one edge class differently from check:
// a flipped answer, a lost denial, a denial check never reports, and a denial
// filed under another rule.
func TestErosion_PolicyQueryAgreementFiresOnAWrongDecision(t *testing.T) {
	t.Parallel()
	agreed := map[string]string{
		"f-allow": ruleAllowlists, "f-layer": ruleLayerOrder, "f-public": rulePublicOnly,
		"f-path": ruleWebNotDomain, "f-module": ruleWebNotInfra,
	}
	without := func(id string) map[string]string {
		out := maps.Clone(agreed)
		delete(out, id)
		return out
	}
	with := func(id, rule string) map[string]string {
		out := maps.Clone(agreed)
		out[id] = rule
		return out
	}
	cases := map[string]struct{ gates, denied map[string]string }{
		"query allows what the allowlist denies":              {gates: agreed, denied: without("f-allow")},
		"query allows a layer inversion":                      {gates: agreed, denied: without("f-layer")},
		"query allows an internal import":                     {gates: agreed, denied: without("f-public")},
		"query allows a forbidden path dependency":            {gates: agreed, denied: without("f-path")},
		"query denies what check does not report":             {gates: agreed, denied: with("f-extra", ruleAllowlists)},
		"query files a denial under another rule":             {gates: agreed, denied: with("f-module", ruleAllowlists)},
		"check never fires on a class, so it is not compared": {gates: without("f-layer"), denied: without("f-layer")},
		"no gate finding at all":                              {gates: map[string]string{}, denied: map[string]string{}},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			if problems := agreementProblems(tc.gates, tc.denied, agreementEdgeClasses); len(problems) == 0 {
				t.Errorf("agreementProblems(%v, %v) = none, want the disagreement reported", tc.gates, tc.denied)
			}
		})
	}

	if problems := agreementProblems(agreed, maps.Clone(agreed), agreementEdgeClasses); len(problems) != 0 {
		t.Errorf("agreementProblems(equal decisions) = %v, want none", problems)
	}
}

// agreementProblems compares check's active gate findings with can-import's
// denials, both as finding ID -> rule ID, and lists every disagreement plus
// every edge class check never fired on. It is the single predicate behind
// the real-run check and its fixture. classes is the fixture's edge classes
// (class name -> rule ID).
func agreementProblems(gates, denied, classes map[string]string) []string {
	var out []string
	fired := map[string]bool{}
	for id, rule := range gates {
		fired[rule] = true
		if denied[id] != rule {
			out = append(out, fmt.Sprintf("check gate finding %s (%s) is not denied by can-import (got %q)", id, rule, denied[id]))
		}
	}
	for id, rule := range denied {
		if gates[id] != rule {
			out = append(out, fmt.Sprintf("can-import denial %s (%s) is not a check gate finding", id, rule))
		}
	}
	for class, rule := range classes {
		if !fired[rule] {
			out = append(out, fmt.Sprintf("check reported no %s gate finding (rule %s): that class is not compared", class, rule))
		}
	}
	slices.Sort(out)
	return out
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
		pyPackage           bool
		wantCode            int
		wantAnswer, reason  string
	}{
		{name: "module_cycle on a cross-module edge", rules: "  - id: no_cycles\n    type: module_cycle\n    gate: fail\n",
			args: []string{agreeAppFile, agreeDomainPkg}, wantCode: 2, wantAnswer: answerNotDecided, reason: "no_cycles"},
		{name: "module_cycle from a package-dir module", rules: "  - id: no_cycles\n    type: module_cycle\n    gate: fail\n",
			args: []string{agreeWebOK, agreeAppPkg}, wantCode: 2, wantAnswer: answerNotDecided, reason: "no_cycles"},
		{name: "external Python import", args: []string{agreePyImporter, "requests"}, pyPackage: true, wantCode: 2, wantAnswer: answerNotDecided, reason: "grimp builds"},
		{name: "excluded Python importer is still extracted", prefix: "exclude: [\"tools/**\"]\n",
			rules: "  - id: no_tools_b\n    type: forbidden_dependency\n    gate: fail\n    from: \"tools.*\"\n    to: tools.b\n",
			args:  []string{agreePyImporter, "tools.b"}, pyPackage: true, wantCode: 1, wantAnswer: answerDenied},
		{name: "TypeScript without a project", args: []string{"web/a.ts", "web/b.ts"},
			wantCode: 0, wantAnswer: answerUnconstrain, reason: "no typescript project"},
		{name: "warn-gated module_cycle decides nothing", rules: "  - id: no_cycles\n    type: module_cycle\n    gate: warn\n",
			args: []string{agreeAppFile, agreeDomainPkg}, wantCode: 0, wantAnswer: answerAllowed, reason: "depends_on"},
		{name: "seam gate in mode fail", prefix: "coupling:\n  gate:\n    distributed_monolith:\n      mode: fail\n",
			args: []string{agreeAppFile, agreeDomainPkg}, wantCode: 2, wantAnswer: answerNotDecided, reason: "seam gate"},
		{name: "node cycle on a Python import", rules: "  - id: no_node_cycles\n    type: cycle\n    gate: fail\n",
			args: []string{agreePyImporter, "tools.b"}, pyPackage: true, wantCode: 2, wantAnswer: answerNotDecided, reason: "no_node_cycles"},
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
			if tc.pyPackage {
				for _, name := range []string{"pyproject.toml", "tools/__init__.py", "tools/a.py", "tools/b.py"} {
					path := filepath.Join(filepath.Dir(cfgPath), name)
					if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
						t.Fatal(err)
					}
					if err := os.WriteFile(path, nil, 0o600); err != nil {
						t.Fatal(err)
					}
				}
			}
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
	if want := []string{ruleLayerOrder, ruleAllowlists}; !slices.Equal(ruleIDs, want) {
		t.Errorf("rules = %v, want %v", ruleIDs, want)
	}
	if doc.Paths[1].Module != "" || len(doc.Paths[1].Rules) != 0 {
		t.Errorf("an unowned path has a module or rules: %+v", doc.Paths[1])
	}
}
