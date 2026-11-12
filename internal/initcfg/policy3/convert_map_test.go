package policy3

import (
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"strings"
	"testing"

	"github.com/alexei-led/archfit/v3/internal/config/policyfile"
	"github.com/alexei-led/archfit/v3/internal/model/evidence"
	"github.com/alexei-led/archfit/v3/internal/policy/resolved"
)

const (
	remoteOwner = "origin-owner"
	modApp      = "app"
	modCore     = "core"
	modUtil     = "util"
	corePaths   = "pkg/core/**"
	mainFile    = "cmd/main.go"
	coreToUtil  = "no-core-to-util"
	dirAB       = "a/b"
	dirAC       = "a/c"
	coreFile    = "pkg/core/core.go"
	libFile     = "lib/lib.go"
	corePkg     = "pkg/core"
	shopOrders  = "shop/orders.py"
	utilFile    = "pkg/util/util.go"
)

// tree is a small Go repository: app imports core, core imports nothing.
var tree = evidence.CheckFacts{
	Sources: []string{
		mainFile, "cmd/tool/tool.go",
		coreFile, "pkg/core/model/model.go", "pkg/core/store/store.go",
		utilFile, libFile, "lib/deep/deep.go",
	},
	Imports: []evidence.ImportFact{
		{File: mainFile, Line: 3, Target: corePkg},
		{File: mainFile, Line: 4, Target: "pkg/core/model"},
		{File: "cmd/tool/tool.go", Line: 3, Target: "pkg/util"},
	},
}

// modulesV2 are the modules of tree in an Archfit 3 policy.
const modulesV2 = `modules:
  app:
    paths: [cmd/**]
    depends_on: [core]
  core:
    paths: [pkg/core/**]
    depends_on: []
  util:
    paths: [pkg/util/**]
    depends_on: []
`

// convertFiles writes the Archfit 3 files, converts them for facts, and
// fails the test when the result does not load and fit the tree.
func convertFiles(t *testing.T, files map[string]string, facts evidence.CheckFacts) Conversion {
	t.Helper()
	dir := t.TempDir()
	for name, content := range files {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	old, err := readArchfit3(dir)
	if err != nil {
		t.Fatalf("readArchfit3: %v", err)
	}
	conv, data, err := convert(old, facts, nil, remoteOwner)
	if err != nil {
		t.Fatalf("convert: %v", err)
	}
	p, err := policyfile.Resolve(data)
	if err != nil {
		t.Fatalf("Resolve: %v\n%s", err, data)
	}
	if err := p.CheckTree(factFiles(facts)); err != nil {
		t.Fatalf("CheckTree: %v\n%s", err, data)
	}
	return conv
}

func convertPolicy(t *testing.T, policy string) Conversion {
	t.Helper()
	return convertFiles(t, map[string]string{PolicyFile: "version: 2\n" + policy}, tree)
}

// noted reports whether a note of key says want.
func noted(conv Conversion, key, want string) bool {
	for _, n := range conv.NotConverted {
		if n.Key == key && strings.Contains(n.Why, want) {
			return true
		}
	}
	return false
}

func wantNote(t *testing.T, conv Conversion, key, want string) {
	t.Helper()
	if !noted(conv, key, want) {
		t.Errorf("no note %s: %q in\n%s", key, want, conv.Text())
	}
}

func TestConvertModulePaths(t *testing.T) {
	conv := convertPolicy(t, `modules:
  app:
    paths: [cmd, gone/**]
    depends_on: []
  lib:
    paths: [lib/lib.go]
    depends_on: []
  empty:
    paths: [nothing/here]
`)
	app := conv.File.Modules[modApp]
	if want := []string{"cmd/*"}; !reflect.DeepEqual(app.Paths, want) {
		t.Errorf("app paths = %v, want %v: a bare directory is that package", app.Paths, want)
	}
	if want := []string{libFile}; !reflect.DeepEqual(conv.File.Modules["lib"].Paths, want) {
		t.Errorf("lib paths = %v, want %v: a file stays", conv.File.Modules["lib"].Paths, want)
	}
	wantNote(t, conv, "modules.app.paths", `"gone/**" matches no source file: dropped`)
	if _, ok := conv.File.Modules["empty"]; ok {
		t.Error("a module with no path that matches a source is kept")
	}
	wantNote(t, conv, "modules.empty", "the module is dropped")

	p := resolved.Policy{Modules: map[string]resolved.Module{modApp: {Paths: app.Paths}}}
	if _, ok := p.ModuleOf("cmd/tool/tool.go"); ok {
		t.Error("cmd/tool/tool.go is in app: a directory below a bare Archfit 3 path stays unowned")
	}
	if m, _ := p.ModuleOf(mainFile); m != modApp {
		t.Errorf("cmd/main.go is in %q, want app", m)
	}
}

func TestConvertModuleKeys(t *testing.T) {
	conv := convertPolicy(t, `layers: [core, cmd]
modules:
  app:
    paths: [cmd/**]
    layer: cmd
    volatility: Frozen
    subdomain: Generic
    owner: alexei-led
    deploy_unit: cli
    role: composition_root
    visible_to: [core]
    depends_on: [core]
  core:
    paths: [pkg/core/**]
    public: [pkg/core, pkg/core/model, pkg/core/store, pkg/gone]
    layer: core
    volatility: legacy
    owner: alexei-led
    internal: [pkg/core/store]
    depends_on: []
  util:
    paths: [pkg/util/**]
    volatility: stormy
    subdomain: odd
    depends_on: []
`)
	app, core, util := conv.File.Modules[modApp], conv.File.Modules[modCore], conv.File.Modules[modUtil]
	if app.Layer != "cmd" || app.Volatility != policyfile.VolatilityLow || app.Subdomain != policyfile.SubdomainGeneric {
		t.Errorf("app = %+v, want layer cmd, volatility low, subdomain generic", app)
	}
	wantNote(t, conv, "modules.app.volatility", "Frozen is low")
	wantNote(t, conv, "modules.core.volatility", "legacy is low")
	if util.Volatility != "" || util.Subdomain != "" {
		t.Errorf("util = %+v, want no volatility and no subdomain", util)
	}
	wantNote(t, conv, "modules.util.volatility", `"stormy" is not low, medium or high`)
	wantNote(t, conv, "modules.util.subdomain", `"odd" is not core, supporting or generic`)
	wantNote(t, conv, "modules.*.owner", "dropped (2 modules)")
	wantNote(t, conv, "modules.*.deploy_unit", "dropped (1 modules)")
	wantNote(t, conv, "modules.*.role", "dropped (1 modules)")
	wantNote(t, conv, "modules.*.visible_to", "the graph rule and public cover it")
	wantNote(t, conv, "modules.*.internal", "public covers it")
	wantNote(t, conv, "modules.core.public", `"pkg/gone" matches no file or directory: dropped`)
	if want := []string{corePaths}; !reflect.DeepEqual(core.Public, want) {
		t.Errorf("core public = %v, want %v: every package of core is public (D8)", core.Public, want)
	}
}

func TestCollapse(t *testing.T) {
	tests := []struct {
		name                     string
		public, packages, others []string
		want                     []string
	}{
		{name: "every package public", public: []string{"a", dirAB, dirAC}, packages: []string{"a", dirAB, dirAC},
			want: []string{"a/**"}},
		{name: "a package not public", public: []string{"a", dirAB}, packages: []string{"a", dirAB, dirAC},
			want: []string{"a", dirAB}},
		{name: "another module below", public: []string{"a", dirAB}, packages: []string{"a", dirAB}, others: []string{"a/x"},
			want: []string{"a", dirAB}},
		{name: "a single package", public: []string{"a"}, packages: []string{"a"}, want: []string{"a"}},
		{name: "children of a directory with no package", public: []string{dirAB, dirAC}, packages: []string{dirAB, dirAC},
			want: []string{"a/**"}},
		{name: "a glob stays", public: []string{"a/*/cmd"}, packages: []string{"a/x/cmd", "a/x"}, want: []string{"a/*/cmd"}},
		{name: "absent", public: nil, packages: []string{"a"}, want: nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := collapse(tt.public, tt.packages, tt.others); !reflect.DeepEqual(got, tt.want) {
				t.Errorf("collapse = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestConvertDependsOn(t *testing.T) {
	conv := convertPolicy(t, `layers: [inner, outer]
modules:
  app:
    paths: [cmd/**]
  core:
    paths: [pkg/core/**]
    layer: inner
    role: domain
    depends_on: ["layer:inner", "role:adapter", "u*", ghost]
  util:
    paths: [pkg/util/**]
    layer: inner
    role: adapter
    depends_on: []
`)
	if want := []string{modCore, modUtil}; !reflect.DeepEqual(conv.File.Modules[modApp].DependsOn, want) {
		t.Errorf("app depends_on = %v, want %v: absent means the modules it uses today", conv.File.Modules[modApp].DependsOn, want)
	}
	wantNote(t, conv, "modules.app.depends_on", "absent, so Archfit 3 allowed any module: written as the modules it uses today (2)")
	if want := []string{modUtil}; !reflect.DeepEqual(conv.File.Modules[modCore].DependsOn, want) {
		t.Errorf("core depends_on = %v, want %v: selectors name modules, never the module itself", conv.File.Modules[modCore].DependsOn, want)
	}
	wantNote(t, conv, "modules.core.depends_on", `"ghost" names no module: dropped`)
}

func TestConvertRules(t *testing.T) {
	tests := []struct {
		name, rule string
		want       []policyfile.Rule
		builtin    policyfile.Builtin
		noInstead  []string
		noteKey    string
		note       string
		// checked is the built-in rule a waiver of the rule becomes an
		// exception of.
		checked string
	}{
		{
			name: "forbidden dependency",
			rule: `{id: no-core-to-util, type: forbidden_dependency, from: pkg/core/**, to: pkg/util, gate: fail, rationale: Core stays pure., alternatives: [Pass a function in., Move it to core.], docs: docs/adr/1.md}`,
			want: []policyfile.Rule{{ID: coreToUtil, Deny: &policyfile.Deny{From: policyfile.Names{corePaths}, To: policyfile.Names{"pkg/util/*"}},
				Why: "Core stays pure. See docs/adr/1.md.", Instead: "Pass a function in.; Move it to core."}},
		},
		{
			name: "module selectors",
			rule: `{id: no-app-to-util, type: forbidden_dependency, from_module: app, to_module: "u*", rationale: r, alternatives: [a]}`,
			want: []policyfile.Rule{{ID: "no-app-to-util", Deny: &policyfile.Deny{From: policyfile.Names{modApp}, To: policyfile.Names{modUtil}},
				Why: "r", Instead: "a"}},
		},
		{
			name:      "no alternatives",
			rule:      `{id: no-core-to-util, type: forbidden_dependency, from: pkg/core/**, to: pkg/util/**, rationale: Core stays pure.}`,
			want:      []policyfile.Rule{{ID: coreToUtil, Note: "No dependency from pkg/core/** to pkg/util/**.", Why: "Core stays pure."}},
			noInstead: []string{coreToUtil},
			noteKey:   "rules[0] (no-core-to-util)", note: "Archfit 3 named no alternatives",
		},
		{
			name:    "a guard that matches nothing",
			rule:    `{id: no-view, type: forbidden_dependency, from: "**", to: internal/view/**, guard: true, rationale: Gone., alternatives: [a]}`,
			want:    []policyfile.Rule{{ID: "no-view", Note: "No dependency from ** to internal/view/**. Archfit 3 named instead: a", Why: "Gone."}},
			noteKey: "rules[0] (no-view)", note: `to "internal/view/**" matches no file (a guard: Archfit 4 has none)`,
		},
		{
			name:    "no rationale",
			rule:    `{id: no-core-to-util, type: forbidden_dependency, from: pkg/core/**, to: pkg/util/**, alternatives: [a]}`,
			want:    []policyfile.Rule{{ID: coreToUtil, Deny: &policyfile.Deny{From: policyfile.Names{corePaths}, To: policyfile.Names{"pkg/util/**"}}, Why: "Converted from the Archfit 3 rule no-core-to-util, which gave no reason.", Instead: "a"}},
			noteKey: "rules[0] (no-core-to-util)", note: "Archfit 3 gave no rationale",
		},
		{
			name:    "a forbidden dependency with gate warn",
			rule:    `{id: soft, type: forbidden_dependency, from: pkg/core/**, to: pkg/util/**, gate: warn, rationale: r, alternatives: [a]}`,
			want:    []policyfile.Rule{{ID: "soft", Note: "No dependency from pkg/core/** to pkg/util/**. Archfit 3 named instead: a", Why: "r"}},
			noteKey: "rules[0] (soft)", note: "gate warn: written as a note",
		},
		{
			name:    "gate off",
			rule:    `{id: dead, type: forbidden_dependency, from: pkg/core/**, to: pkg/util/**, gate: "off", rationale: r, alternatives: [a]}`,
			noteKey: "rules[0] (dead)", note: "gate off: dropped",
		},
		{
			name:    "module dependencies",
			rule:    `{id: boundaries, type: module_dependencies, gate: fail, rationale: The map is the design.}`,
			noteKey: "rules[0] (boundaries).rationale", note: `dropped: Archfit 4 has no place for it (was "The map is the design.")`,
			checked: resolved.RuleGraph,
		},
		{
			name:    "new cross-module dependency",
			rule:    `{id: new-edges, type: new_cross_module_dependency}`,
			checked: resolved.RuleGraph,
		},
		{
			name:    "a module cycle with gate warn",
			rule:    `{id: cycles, type: module_cycle, gate: warn}`,
			noteKey: "rules[0] (cycles)", note: "gate warn: the built-in cycle rule replaces it, mode deny",
			checked: resolved.RuleCycle,
		},
		{
			name:    "a cycle with gate off",
			rule:    `{id: cycles, type: cycle, gate: "off"}`,
			builtin: policyfile.Builtin{Cycle: policyfile.ModeOff},
			noteKey: "rules[0] (cycles)", note: "gate off: the built-in cycle rule is off",
		},
		{
			name:    "layer direction",
			rule:    `{id: layers, type: forbidden_layer_direction, gate: fail}`,
			checked: resolved.RuleLayers,
		},
		{
			name:    "public api only",
			rule:    `{id: api, type: public_api_only}`,
			noteKey: "rules[0] (api)", note: "public_api_only: dropped: the coupling rule and public cover it",
		},
		{
			name:    "internal api access",
			rule:    `{id: inner, type: internal_api_access}`,
			noteKey: "rules[0] (inner)", note: "internal_api_access: dropped",
		},
		{
			name:    "public api max",
			rule:    `{id: small-api, type: public_api_max, max: 40, gate: warn}`,
			want:    []policyfile.Rule{{ID: "small-api", Note: "A module has at most 40 exported declarations.", Why: whyUnchecked}},
			noteKey: "rules[0] (small-api)", note: "public_api_max: written as a note: nothing checks it in Archfit 4",
		},
		{
			name: "public api change",
			rule: `{id: api-change, type: public_api_change, rationale: Contracts move slowly.}`,
			want: []policyfile.Rule{{ID: "api-change", Note: noteOf["public_api_change"], Why: "Contracts move slowly."}},
		},
		{
			name: "public api type leak",
			rule: `{id: leak, type: public_api_type_leak, docs: docs/adr/2.md}`,
			want: []policyfile.Rule{{ID: "leak", Note: noteOf["public_api_type_leak"], Why: whyUnchecked + " See docs/adr/2.md."}},
		},
		{
			name:    "forbidden pattern",
			rule:    `{id: no-panic, type: forbidden_pattern, from: pkg/**, rationale: Return errors., patterns: [{id: p1, regex: "panic\\("}]}`,
			want:    []policyfile.Rule{{ID: "no-panic", Note: noteOf["forbidden_pattern"], Why: "Return errors."}},
			noteKey: "rules[0] (no-panic).patterns", note: `dropped: Archfit 4 has no place for it (was [{id: p1, regex: "panic\\("}])`,
		},
		{
			name:    "an unknown type",
			rule:    `{id: odd, type: telepathy}`,
			noteKey: "rules[0] (odd)", note: `type "telepathy": dropped`,
		},
		{
			name:    "a code id",
			rule:    `{id: D-12, type: forbidden_dependency, from: pkg/core/**, to: pkg/util/**, rationale: r, alternatives: [a]}`,
			want:    []policyfile.Rule{{ID: "forbidden-dependency-d-12", Deny: &policyfile.Deny{From: policyfile.Names{corePaths}, To: policyfile.Names{"pkg/util/**"}}, Why: "r", Instead: "a"}},
			noteKey: "rules[0] (D-12)", note: "the id is now forbidden-dependency-d-12",
		},
		{
			name:    "an id of a built-in rule",
			rule:    `{id: graph, type: public_api_max, max: 3}`,
			want:    []policyfile.Rule{{ID: "public-api-max-graph", Note: "A module has at most 3 exported declarations.", Why: whyUnchecked}},
			noteKey: "rules[0] (graph)", note: "the id is now public-api-max-graph",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			policy := modulesV2 + "rules:\n  - " + tt.rule + "\n"
			id := strings.TrimSpace(strings.SplitN(strings.SplitN(tt.rule, "id: ", 2)[1], ",", 2)[0])
			if tt.checked != "" {
				policy += "waivers:\n  - {rule: " + id + ", from: app, to: core, reason: r, approved_by: a, expires: \"2027-01-01\"}\n"
			}
			conv := convertPolicy(t, policy)
			if tt.checked != "" {
				want := policyfile.Exception{Rule: tt.checked, From: modApp, To: modCore, Expires: "2027-01-01", Why: "r"}
				if len(conv.File.Exceptions) == 0 || conv.File.Exceptions[0] != want {
					t.Errorf("exceptions = %+v, want first %+v: a waiver of the rule is an exception of %s", conv.File.Exceptions, want, tt.checked)
				}
			}
			if !reflect.DeepEqual(conv.File.Rules, tt.want) {
				t.Errorf("rules = %+v, want %+v", conv.File.Rules, tt.want)
			}
			if conv.File.Builtin.Cycle != tt.builtin.Cycle || conv.File.Builtin.Graph != tt.builtin.Graph || conv.File.Builtin.Layers != tt.builtin.Layers {
				t.Errorf("builtin = %+v, want %+v", conv.File.Builtin, tt.builtin)
			}
			if !reflect.DeepEqual(conv.NoInstead, tt.noInstead) {
				t.Errorf("NoInstead = %v, want %v", conv.NoInstead, tt.noInstead)
			}
			if tt.noteKey != "" {
				wantNote(t, conv, tt.noteKey, tt.note)
			}
		})
	}
}

// graphTree is a tree whose only use between modules is app's use of core.
var graphTree = evidence.CheckFacts{
	Sources: []string{mainFile, coreFile, utilFile},
	Imports: []evidence.ImportFact{{File: mainFile, Line: 3, Target: corePkg}},
}

// brokenGraph is a policy whose app uses core without an allowed dependency:
// one graph error on graphTree.
const brokenGraph = `modules:
  app:
    paths: [cmd/**]
    depends_on: []
  core:
    paths: [pkg/core/**]
    depends_on: []
  util:
    paths: [pkg/util/**]
    depends_on: []
rules:
  - {id: boundaries, type: module_dependencies}
`

func TestConvertWaivers(t *testing.T) {
	conv := convertFiles(t, map[string]string{PolicyFile: "version: 2\n" + brokenGraph + `waivers:
  - {rule: boundaries, from: cmd, to: pkg/core, reason: Until the split., approved_by: alexei-led, expires: "2027-01-31"}
  - {rule: bc/imbalanced_coupling, from: cmd/**, to: pkg/core/**, reason: "", approved_by: a, expires: "2027-02-01"}
  - {rule: map/dead_rule, reason: r, approved_by: a, expires: "2027-01-01"}
  - {rule: boundaries, from: "**", to: pkg/core, reason: r, approved_by: a, expires: "2027-01-01"}
`}, graphTree)
	want := []policyfile.Exception{
		{Rule: resolved.RuleGraph, From: modApp, To: modCore, Expires: "2027-01-31", Why: "Until the split."},
		{Rule: resolved.RuleCoupling, From: modApp, To: modCore, Expires: "2027-02-01", Why: whyWaiver},
	}
	if !reflect.DeepEqual(conv.File.Exceptions, want) {
		t.Errorf("exceptions = %+v, want %+v: the waiver covers the graph error, so no undated exception repeats it", conv.File.Exceptions, want)
	}
	wantNote(t, conv, "waivers[0] (boundaries).approved_by", `dropped: Archfit 4 has no place for it (was "alexei-led")`)
	wantNote(t, conv, "waivers[2] (map/dead_rule)", "nothing checks its rule in Archfit 4")
	wantNote(t, conv, "waivers[3] (boundaries)", `"**" matches files of 3 modules (app, core, util)`)
}

func TestConvertBaseline(t *testing.T) {
	baseline := `{"schema_version": "archfit.baseline.v3", "accepted": [
  {"fingerprint": "a1", "rule_id": "boundaries", "kind": "gate"},
  {"fingerprint": "a2", "rule_id": "bc/duplicated_knowledge", "kind": "advisory"},
  {"fingerprint": "a3", "rule_id": "size", "kind": "advisory"},
  {"fingerprint": "a4", "rule_id": "size", "kind": "advisory"}],
  "metrics": {"cycle": {"value": 0}}}`
	conv := convertFiles(t, map[string]string{
		PolicyFile:   "version: 2\n" + brokenGraph + "  - {id: size, type: public_api_max, max: 9}\n",
		BaselineFile: baseline,
	}, graphTree)
	want := []policyfile.Exception{{Rule: resolved.RuleGraph, From: modApp, To: modCore, Why: whyBaseline}}
	if !reflect.DeepEqual(conv.File.Exceptions, want) {
		t.Errorf("exceptions = %+v, want %+v", conv.File.Exceptions, want)
	}
	wantNote(t, conv, "exceptions.graph", "1 written for the errors of today; the Archfit 3 baseline accepted 1")
	wantNote(t, conv, "exceptions.duplication", "0 written for the errors of today; the Archfit 3 baseline accepted 1")
	wantNote(t, conv, BaselineFile, "2 accepted entries of size dropped: nothing checks that rule in Archfit 4")
	wantNote(t, conv, BaselineFile+" metrics", "no metric ratchets")
}

func TestConvertExistingErrorWithoutBaseline(t *testing.T) {
	conv := convertFiles(t, map[string]string{PolicyFile: "version: 2\n" + brokenGraph}, graphTree)
	want := []policyfile.Exception{{Rule: resolved.RuleGraph, From: modApp, To: modCore, Why: whyException}}
	if !reflect.DeepEqual(conv.File.Exceptions, want) {
		t.Errorf("exceptions = %+v, want %+v: every error of today gets an exception", conv.File.Exceptions, want)
	}
	wantNote(t, conv, "exceptions.graph", "1 written for the errors of today; the Archfit 3 baseline accepted 0")
}

func TestConvertAppLabelsAndDrafts(t *testing.T) {
	conv := convertFiles(t, map[string]string{
		PolicyFile:                 "version: 2\n" + modulesV2,
		AppFile:                    "version: 1\nowners: [alexei-led, reviewer]\ngate: required\nworkflow: .github/workflows/archfit.yaml\ndata_mode: customer\n",
		LabelsFile:                 "version: 1\nlabels:\n  - {from: app, to: core, strength: model, rationale: r}\n",
		".archfit-volatility.yaml": "volatility: {}\n",
	}, tree)
	if want := []string{"alexei-led", "reviewer"}; !reflect.DeepEqual(conv.File.Owners, want) {
		t.Errorf("owners = %v, want %v", conv.File.Owners, want)
	}
	if conv.File.Gate != policyfile.GateRequired {
		t.Errorf("gate = %q, want required", conv.File.Gate)
	}
	wantNote(t, conv, AppFile+" workflow", "dropped")
	wantNote(t, conv, AppFile+" data_mode", "dropped")
	wantNote(t, conv, LabelsFile, "app -> core model: dropped")
	wantNote(t, conv, ".archfit-volatility.yaml", "not read and not deleted")

	bare := convertPolicy(t, modulesV2)
	if want := []string{remoteOwner}; !reflect.DeepEqual(bare.File.Owners, want) {
		t.Errorf("owners without an App file = %v, want %v", bare.File.Owners, want)
	}
	wantNote(t, bare, "owners", "the owner of the origin remote")
}

func TestConvertTopLevelKeys(t *testing.T) {
	conv := convertPolicy(t, `target_branch: master
coupling: {min_severity: medium}
metrics: {cycle: {gate: fail}}
outputs: {json: true}
languages:
  go: {enabled: true, typed: true}
  python: {enabled: false, package: shop}
analyzers:
  syntax: {enabled: true, gate: fail}
  scip: {enabled: true}
coverage:
  enabled: true
  gate: warn
  sources: [{path: c.out, format: go-coverprofile, stale: 1}]
exclude: [vendor/**]
`+modulesV2)
	wantNote(t, conv, "target_branch", "the check compares a change with its base")
	wantNote(t, conv, "coupling", "the coupling rule replaces it")
	wantNote(t, conv, "metrics", "no metric ratchets")
	wantNote(t, conv, "outputs", "Archfit 4 has no such key")
	wantNote(t, conv, "languages.go.typed", "dropped")
	wantNote(t, conv, "analyzers.syntax.gate", "dropped")
	wantNote(t, conv, "analyzers.scip", "dropped")
	wantNote(t, conv, "coverage.gate", "dropped")
	wantNote(t, conv, "coverage.sources[0].stale", "dropped")
	f := conv.File
	if f.Languages.Go.Enabled != policyfile.SwitchOn || f.Languages.Python.Enabled != policyfile.SwitchOff || f.Languages.Python.Package != "shop" {
		t.Errorf("languages = %+v", f.Languages)
	}
	if f.Analyzers.Syntax.Enabled != policyfile.SwitchOn {
		t.Errorf("analyzers = %+v", f.Analyzers)
	}
	if !f.Coverage.Enabled || len(f.Coverage.Sources) != 1 || f.Coverage.Sources[0].Path != "c.out" {
		t.Errorf("coverage = %+v", f.Coverage)
	}
	if want := []string{"vendor/**"}; !reflect.DeepEqual(f.Exclude, want) {
		t.Errorf("exclude = %v, want %v", f.Exclude, want)
	}
}

func TestConversionText(t *testing.T) {
	tests := []struct {
		name string
		conv Conversion
		want string
	}{
		{name: "nothing", want: "not converted: nothing\n"},
		{
			name: "rules waiting for instead come first",
			conv: Conversion{NoInstead: []string{"a", "b"}, NotConverted: []Note{{Key: "ai", Why: "dropped"}}},
			want: "not checked until an owner writes instead (Archfit 3 named no alternatives): a, b\n" +
				"not converted or changed:\n  ai: dropped\n",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.conv.Text(); got != tt.want {
				t.Errorf("Text =\n%s\nwant\n%s", got, tt.want)
			}
		})
	}
}

func TestConvertGateOffKeepsABuiltinAnotherRuleEnforces(t *testing.T) {
	tests := []struct {
		name, rules string
		want        policyfile.Builtin
		note        string
	}{
		{
			name:  "graph",
			rules: "  - {id: deps, type: module_dependencies}\n  - {id: new-deps, type: new_cross_module_dependency, gate: \"off\"}\n",
			note:  "gate off, but deps enforces the built-in graph rule: it stays on",
		},
		{
			name:  "cycle",
			rules: "  - {id: cyc, type: cycle, gate: \"off\"}\n  - {id: mod-cyc, type: module_cycle}\n",
			note:  "gate off, but mod-cyc enforces the built-in cycle rule: it stays on",
		},
		{
			name:  "both off",
			rules: "  - {id: cyc, type: cycle, gate: \"off\"}\n  - {id: mod-cyc, type: module_cycle, gate: \"off\"}\n",
			want:  policyfile.Builtin{Cycle: policyfile.ModeOff},
			note:  "gate off: the built-in cycle rule is off",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			conv := convertPolicy(t, modulesV2+"rules:\n"+tt.rules)
			if b := conv.File.Builtin; b.Graph != tt.want.Graph || b.Cycle != tt.want.Cycle {
				t.Errorf("builtin = %+v, want %+v", b, tt.want)
			}
			if !strings.Contains(conv.Text(), tt.note) {
				t.Errorf("no note %q in\n%s", tt.note, conv.Text())
			}
		})
	}
}

func TestConvertDenyErrorInNoModule(t *testing.T) {
	facts := evidence.CheckFacts{
		Sources: []string{mainFile, libFile, "lib/use.go", coreFile},
		Imports: []evidence.ImportFact{
			{File: mainFile, Line: 3, Target: "lib"},
			{File: "lib/use.go", Line: 4, Target: corePkg},
		},
	}
	conv := convertFiles(t, map[string]string{PolicyFile: `version: 2
modules:
  app:
    paths: [cmd/**]
    depends_on: [core]
  core:
    paths: [pkg/core/**]
    depends_on: []
rules:
  - {id: no-lib, type: forbidden_dependency, from_module: app, to: lib/**, rationale: r, alternatives: [a]}
  - {id: no-lib-core, type: forbidden_dependency, from: lib/**, to_module: core, rationale: r, alternatives: [a]}
`}, facts)
	for _, x := range conv.File.Exceptions {
		if x.Rule == "no-lib" || x.Rule == "no-lib-core" {
			t.Errorf("exception %+v: code in no module has no place an exception can name", x)
		}
	}
	wantNote(t, conv, "exceptions.no-lib", "no exception can name the error from app to lib at cmd/main.go:3: the code is in no module")
	wantNote(t, conv, "exceptions.no-lib-core", "no exception can name the error from no module to core at lib/use.go:4")
}

// TestConvertDeclaresTests: the converted policy declares the test files the
// facts name, though Go sources leave them out, so a literal a test copies
// from another module is no error and gets no exception.
func TestConvertDeclaresTests(t *testing.T) {
	const copied = "order not found in the store"
	tests := []struct {
		name string
		copy string
		want []policyfile.Exception
	}{
		{name: "a test copies the literal", copy: "pkg/core/core_test.go"},
		{
			name: "production code copies the literal", copy: coreFile,
			want: []policyfile.Exception{{Rule: resolved.RuleDuplication, From: modCore, To: modUtil, Why: whyException}},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			facts := evidence.CheckFacts{
				Sources: []string{mainFile, coreFile, utilFile},
				Literals: []evidence.LiteralOccurrence{
					{File: "pkg/core/core_test.go", Line: 9, Value: "place an order twice", Shape: evidence.ShapeOther},
					{File: tt.copy, Line: 5, Value: copied, Shape: evidence.ShapeOther},
					{File: utilFile, Line: 3, Value: copied, Shape: evidence.ShapeOther},
				},
			}
			conv := convertFiles(t, map[string]string{PolicyFile: "version: 2\n" + modulesV2}, facts)
			if want := []string{"**/*_test.go"}; !reflect.DeepEqual(conv.File.Tests, want) {
				t.Errorf("tests = %v, want %v", conv.File.Tests, want)
			}
			if !slices.Equal(conv.File.Exceptions, tt.want) {
				t.Errorf("exceptions = %+v, want %+v", conv.File.Exceptions, tt.want)
			}
		})
	}
}

func TestConvertPackageDirectoryGlobs(t *testing.T) {
	facts := evidence.CheckFacts{Sources: []string{"pkg/a/a.go", "pkg/b/b.go", "web/x.ts"}}
	conv := convertFiles(t, map[string]string{PolicyFile: `version: 2
modules:
  pkgs:
    paths: [pkg/*]
  web:
    paths: [web/*.ts]
`}, facts)
	if want := []string{"pkg/*/*"}; !reflect.DeepEqual(conv.File.Modules["pkgs"].Paths, want) {
		t.Errorf("pkgs paths = %v, want %v: pkg/* named package directories", conv.File.Modules["pkgs"].Paths, want)
	}
	if want := []string{"web/*.ts"}; !reflect.DeepEqual(conv.File.Modules["web"].Paths, want) {
		t.Errorf("web paths = %v, want %v: a file glob stays", conv.File.Modules["web"].Paths, want)
	}
}

func TestConvertWaiverByModuleName(t *testing.T) {
	conv := convertFiles(t, map[string]string{PolicyFile: "version: 2\n" + brokenGraph + `waivers:
  - {rule: boundaries, from: app, to: core, reason: Until the split., approved_by: a, expires: "2027-01-31"}
`}, graphTree)
	want := []policyfile.Exception{{Rule: resolved.RuleGraph, From: modApp, To: modCore, Expires: "2027-01-31", Why: "Until the split."}}
	if !reflect.DeepEqual(conv.File.Exceptions, want) {
		t.Errorf("exceptions = %+v, want %+v: Archfit 3 matched a module rule's waiver by module name", conv.File.Exceptions, want)
	}
}

func TestConvertPackageGlobBesideABareDirectory(t *testing.T) {
	facts := evidence.CheckFacts{Sources: []string{"api/api.go", "api/v1/v1.go", "web/x.ts", "web/sub/y.ts"}}
	conv := convertFiles(t, map[string]string{PolicyFile: `version: 2
modules:
  api:
    paths: [api]
  handlers:
    paths: [api/*]
  web:
    paths: [web/*]
`}, facts)
	for name, want := range map[string][]string{"api": {"api/*"}, "handlers": {"api/*/*"}, "web": {"web/*"}} {
		if got := conv.File.Modules[name].Paths; !reflect.DeepEqual(got, want) {
			t.Errorf("%s paths = %v, want %v", name, got, want)
		}
	}
}

func TestConvertPythonDottedSelectors(t *testing.T) {
	const pyA, pyB = "fixture_py/a.py", "fixture_py/b/__init__.py"
	facts := evidence.CheckFacts{Sources: []string{
		"fixture_py/__init__.py", pyA, pyB,
		"fixture_py/b/_internal/__init__.py", "fixture_py/b/_internal/impl.py", "fixture_py/b/_internal/deep/x.py",
	}}
	conv := convertFiles(t, map[string]string{PolicyFile: `version: 2
modules:
  a:
    paths: [fixture_py.a]
  b:
    paths: [fixture_py.b, fixture_py.b.**]
    public: [fixture_py.b]
rules:
  - {id: no-internal, type: forbidden_dependency, from: fixture_py.a, to: fixture_py.b._internal.*, rationale: r, alternatives: [use b]}
`}, facts)
	if want := []string{pyA}; !reflect.DeepEqual(conv.File.Modules["a"].Paths, want) {
		t.Errorf("a paths = %v, want %v", conv.File.Modules["a"].Paths, want)
	}
	b := conv.File.Modules["b"]
	if want := []string{pyB, "fixture_py/b/**"}; !reflect.DeepEqual(b.Paths, want) {
		t.Errorf("b paths = %v, want %v", b.Paths, want)
	}
	if want := []string{pyB}; !reflect.DeepEqual(b.Public, want) {
		t.Errorf("b public = %v, want %v", b.Public, want)
	}
	want := policyfile.Deny{From: policyfile.Names{pyA}, To: policyfile.Names{"fixture_py/b/_internal/**"}}
	if len(conv.File.Rules) != 1 || conv.File.Rules[0].Deny == nil || !reflect.DeepEqual(*conv.File.Rules[0].Deny, want) {
		t.Errorf("rules = %+v, want deny %+v", conv.File.Rules, want)
	}
}

func TestPythonGlobs(t *testing.T) {
	tests := []struct {
		dotted string
		want   []string
	}{
		{"shop.orders", []string{shopOrders, "shop/orders/__init__.py", "src/shop/orders.py", "src/shop/orders/__init__.py"}},
		{"shop.orders.*", []string{"shop/orders/**", "src/shop/orders/**"}},
		{"shop.orders.**", []string{"shop/orders/**", "src/shop/orders/**"}},
		{"shop.*.api", []string{"shop/*/**/api.py", "shop/*/**/api/__init__.py", "src/shop/*/**/api.py", "src/shop/*/**/api/__init__.py"}},
	}
	for _, tt := range tests {
		if got := pythonGlobs(tt.dotted); !reflect.DeepEqual(got, tt.want) {
			t.Errorf("pythonGlobs(%q) = %v, want %v", tt.dotted, got, tt.want)
		}
	}
}

func TestPythonInnerWildcardNeedsAPackage(t *testing.T) {
	for _, dotted := range []string{"shop.*.api", "shop.**.api"} {
		globs := pythonGlobs(dotted)
		for file, want := range map[string]bool{"shop/api.py": false, "shop/orders/api.py": true, "shop/a/b/api.py": true} {
			if got := matchesGlob(globs, file); got != want {
				t.Errorf("%s covers %s = %v, want %v", dotted, file, got, want)
			}
		}
	}
}

func TestConvertPythonNamespacePackage(t *testing.T) {
	facts := evidence.CheckFacts{Sources: []string{shopOrders, "shop/pay/card.py"}}
	conv := convertFiles(t, map[string]string{PolicyFile: `version: 2
modules:
  root:
    paths: [shop]
  parts:
    paths: [shop.*]
`}, facts)
	if _, ok := conv.File.Modules["root"]; ok {
		t.Errorf("root = %+v: a namespace package had no node in Archfit 3", conv.File.Modules["root"])
	}
	if want := []string{"shop/**"}; !reflect.DeepEqual(conv.File.Modules["parts"].Paths, want) {
		t.Errorf("parts paths = %v, want %v", conv.File.Modules["parts"].Paths, want)
	}
}

func TestConvertPythonTopLevelPackageBesideItsModules(t *testing.T) {
	facts := evidence.CheckFacts{Sources: []string{"shop/__init__.py", shopOrders, "shop/pay/card.py"}}
	conv := convertFiles(t, map[string]string{PolicyFile: `version: 2
modules:
  root:
    paths: [shop]
  parts:
    paths: [shop.*]
`}, facts)
	if want := []string{"shop/__init__.py"}; !reflect.DeepEqual(conv.File.Modules["root"].Paths, want) {
		t.Errorf("root paths = %v, want %v: the dotted name shop is the package", conv.File.Modules["root"].Paths, want)
	}
	if want := []string{"shop/**"}; !reflect.DeepEqual(conv.File.Modules["parts"].Paths, want) {
		t.Errorf("parts paths = %v, want %v", conv.File.Modules["parts"].Paths, want)
	}
}
