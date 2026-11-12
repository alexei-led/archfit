package policy3

import (
	"os"
	"path/filepath"
	"reflect"
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
)

// tree is a small Go repository: app imports core, core imports nothing.
var tree = evidence.CheckFacts{
	Sources: []string{
		mainFile, "cmd/tool/tool.go",
		"pkg/core/core.go", "pkg/core/model/model.go", "pkg/core/store/store.go",
		"pkg/util/util.go", "lib/lib.go", "lib/deep/deep.go",
	},
	Imports: []evidence.ImportFact{
		{File: mainFile, Line: 3, Target: "pkg/core"},
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
	if err := p.CheckTree(facts.Sources); err != nil {
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
	if want := []string{"lib/lib.go"}; !reflect.DeepEqual(conv.File.Modules["lib"].Paths, want) {
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
			want:    []policyfile.Rule{{ID: "no-view", Note: "No dependency from ** to internal/view/**.", Why: "Gone."}},
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
			want:    []policyfile.Rule{{ID: "soft", Note: "No dependency from pkg/core/** to pkg/util/**.", Why: "r"}},
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
			noteKey: "rules[0] (boundaries)", note: "the built-in graph rule replaces it; its rationale is dropped",
		},
		{
			name:    "new cross-module dependency",
			rule:    `{id: new-edges, type: new_cross_module_dependency}`,
			noteKey: "", note: "",
		},
		{
			name:    "a module cycle with gate warn",
			rule:    `{id: cycles, type: module_cycle, gate: warn}`,
			noteKey: "rules[0] (cycles)", note: "gate warn: the built-in cycle rule replaces it, mode deny",
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
			noteKey: "", note: "",
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
			rule: `{id: leak, type: public_api_type_leak}`,
			want: []policyfile.Rule{{ID: "leak", Note: noteOf["public_api_type_leak"], Why: whyUnchecked}},
		},
		{
			name: "forbidden pattern",
			rule: `{id: no-panic, type: forbidden_pattern, rationale: Return errors.}`,
			want: []policyfile.Rule{{ID: "no-panic", Note: noteOf["forbidden_pattern"], Why: "Return errors."}},
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
			conv := convertPolicy(t, modulesV2+"rules:\n  - "+tt.rule+"\n")
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
	Sources: []string{mainFile, "pkg/core/core.go", "pkg/util/util.go"},
	Imports: []evidence.ImportFact{{File: mainFile, Line: 3, Target: "pkg/core"}},
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
