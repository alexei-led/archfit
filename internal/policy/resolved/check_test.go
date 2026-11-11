package resolved_test

import (
	"strings"
	"testing"

	"github.com/alexei-led/archfit/v3/internal/policy/resolved"
	"github.com/alexei-led/archfit/v3/internal/testutil"
)

const (
	modChaos = "pkg_chaos"
	modUtil  = "pkg_util"
	modTC    = "pkg_tc"
	why      = "a reason"
	owner    = "alexei-led"
	core     = "core"
	cmdGlob  = "cmd/**"
)

// base is a valid policy with three modules and the defaults applied.
func base() resolved.Policy {
	return resolved.Policy{
		Owners: []string{owner},
		Layers: []string{core, "cmd"},
		Modules: map[string]resolved.Module{
			modChaos: {Paths: []string{"pkg/chaos/**"}, Public: []string{"pkg/chaos"}, Layer: core, DependsOn: []string{modUtil}},
			modTC:    {Paths: []string{"pkg/tc/**"}, Volatility: resolved.VolatilityHigh},
			modUtil:  {Paths: []string{"pkg/util/**"}, Volatility: resolved.VolatilityLow},
		},
	}.WithDefaults()
}

func TestCheckValid(t *testing.T) {
	p := base()
	p.Rules = []resolved.Rule{
		{ID: "no-chaos-to-tc", Form: resolved.FormDeny, From: []string{modChaos, cmdGlob}, To: []string{modTC}, Why: why, Instead: "x"},
		{ID: "sql-in-util", Form: resolved.FormOnly, Use: resolved.SetDatabase, In: []string{modUtil}, Why: why},
	}
	p.Exceptions = []resolved.Exception{
		{Rule: "no-chaos-to-tc", From: modChaos, To: modTC, Why: why},
		{Rule: resolved.RuleUnowned, From: "scripts/**", Why: why},
	}
	if err := p.Check(); err != nil {
		t.Fatalf("Check: %v", err)
	}
}

func TestCheckMessages(t *testing.T) {
	tests := []struct {
		name   string
		change func(p *resolved.Policy)
		want   string
	}{
		{
			"unknown module in depends_on, with a suggestion",
			func(p *resolved.Policy) { setDeps(p, modChaos, "pkg_utl") },
			`modules.pkg_chaos.depends_on: "pkg_utl" is not a module: did you mean "pkg_util"?`,
		},
		{
			"unknown module with no close name",
			func(p *resolved.Policy) { setDeps(p, modChaos, "storage") },
			`modules.pkg_chaos.depends_on: "storage" is not a module`,
		},
		{
			"unknown module in deny",
			func(p *resolved.Policy) {
				p.Rules = []resolved.Rule{{ID: "r", Form: resolved.FormDeny, From: []string{modChaos}, To: []string{"pkg_tcc"}, Why: why}}
			},
			`rules[0] (r).deny.to: "pkg_tcc" is not a module: did you mean "pkg_tc"?`,
		},
		{
			"unknown module in only",
			func(p *resolved.Policy) {
				p.Rules = []resolved.Rule{{ID: "r", Form: resolved.FormOnly, Use: resolved.SetProcess, In: []string{"runtime"}, Why: why}}
			},
			`rules[0] (r).only.in: "runtime" is not a module`,
		},
		{
			"unknown module in an exception",
			func(p *resolved.Policy) {
				p.Exceptions = []resolved.Exception{{Rule: resolved.RuleGraph, From: "pkg_chao", To: modTC, Why: why}}
			},
			`exceptions[0].from: "pkg_chao" is not a module: did you mean "pkg_chaos"?`,
		},
		{
			"layer not in layers",
			func(p *resolved.Policy) { setLayer(p, modTC, "adapter") },
			`modules.pkg_tc.layer: "adapter" is not in layers (core, cmd)`,
		},
		{
			"layer without layers",
			func(p *resolved.Policy) { p.Layers = nil },
			`modules.pkg_chaos.layer: "core" is not in layers: add layers, inner first`,
		},
		{
			"exception for an unknown rule",
			func(p *resolved.Policy) {
				p.Rules = []resolved.Rule{{ID: "no-chaos-to-runtime", Form: resolved.FormNote, Note: "n", Why: why}}
				p.Exceptions = []resolved.Exception{{Rule: "no-chaos-to-runtme", From: modChaos, To: modTC, Why: why}}
			},
			`exceptions[0].rule: "no-chaos-to-runtme" is not a rule: did you mean "no-chaos-to-runtime"?`,
		},
		{
			"exception for a misspelled built-in rule",
			func(p *resolved.Policy) {
				p.Exceptions = []resolved.Exception{{Rule: "cycles", From: modChaos, To: modTC, Why: why}}
			},
			`exceptions[0].rule: "cycles" is not a rule: did you mean "cycle"?`,
		},
		{
			"duplicate rule id",
			func(p *resolved.Policy) {
				note := resolved.Rule{ID: "x", Form: resolved.FormNote, Note: "n", Why: why}
				p.Rules = []resolved.Rule{note, note}
			},
			`rules[1] (x).id: "x" is also the id of rules[0]: give each rule its own id`,
		},
		{
			"rule id of a built-in rule",
			func(p *resolved.Policy) {
				p.Rules = []resolved.Rule{{ID: resolved.RuleCycle, Form: resolved.FormNote, Note: "n", Why: why}}
			},
			`rules[0] (cycle).id: "cycle" is a built-in rule: pick another id`,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := base()
			tt.change(&p)
			err := p.Check()
			if err == nil {
				t.Fatalf("Check: want %q", tt.want)
			}
			if err.Error() != tt.want {
				t.Errorf("Check:\n got %q\nwant %q", err, tt.want)
			}
			if words := testutil.NotUsedIn(err.Error()); len(words) > 0 {
				t.Errorf("message uses %v, words design section 5 does not use", words)
			}
		})
	}
}

func setDeps(p *resolved.Policy, module string, deps ...string) {
	m := p.Modules[module]
	m.DependsOn = deps
	p.Modules[module] = m
}

func setLayer(p *resolved.Policy, module, layer string) {
	m := p.Modules[module]
	m.Layer = layer
	p.Modules[module] = m
}

func setPaths(p *resolved.Policy, module string, paths ...string) {
	m := p.Modules[module]
	m.Paths = paths
	p.Modules[module] = m
}

var tree = []string{
	"pkg/chaos/chaos.go", "pkg/chaos/cli/flags.go", "pkg/tc/tc.go", "pkg/util/util.go", "cmd/main.go", "scripts/run.sh",
}

func TestCheckTreeValid(t *testing.T) {
	p := base()
	p.Rules = []resolved.Rule{{ID: "r", Form: resolved.FormDeny, From: []string{cmdGlob}, To: []string{modTC}, Why: why}}
	p.Exceptions = []resolved.Exception{{Rule: resolved.RuleUnowned, From: "scripts/**", Why: why}}
	p.Tests = []string{"**/*_test.go", "pkg/chaos/cli/**"}
	// public: [pkg/chaos] names the package directory, which holds a file.
	if err := p.CheckTree(append(tree, "pkg/util/util_test.go")); err != nil {
		t.Fatalf("CheckTree: %v", err)
	}
}

func TestCheckTreeMessages(t *testing.T) {
	tests := []struct {
		name   string
		change func(p *resolved.Policy)
		want   string
	}{
		{
			"module path matches no file",
			func(p *resolved.Policy) { setPaths(p, modUtil, "pkg/util/**", "pkg/utils/**") },
			`modules.pkg_util.paths: "pkg/utils/**" matches no file`,
		},
		{
			"public matches no file",
			func(p *resolved.Policy) {
				m := p.Modules[modChaos]
				m.Public = []string{"pkg/chaos/api"}
				p.Modules[modChaos] = m
			},
			`modules.pkg_chaos.public: "pkg/chaos/api" matches no file`,
		},
		{
			"a directory as a module path owns no file",
			func(p *resolved.Policy) { setPaths(p, modTC, "pkg/tc") },
			`modules.pkg_tc.paths: "pkg/tc" matches no file`,
		},
		{
			"generated glob matches no file",
			func(p *resolved.Policy) { p.Generated = []string{"**/*.pb.go"} },
			`generated: "**/*.pb.go" matches no file`,
		},
		{
			"tests glob matches no file",
			func(p *resolved.Policy) { p.Tests = []string{"test/**"} },
			`tests: "test/**" matches no file`,
		},
		{
			"rule path glob matches no file",
			func(p *resolved.Policy) {
				p.Rules = []resolved.Rule{{ID: "r", Form: resolved.FormDeny, From: []string{"internal/**"}, To: []string{modTC}, Why: why}}
			},
			`rules[0] (r).deny.from: "internal/**" matches no file`,
		},
		{
			"unowned exception matches no file",
			func(p *resolved.Policy) {
				p.Exceptions = []resolved.Exception{{Rule: resolved.RuleUnowned, From: "tools/**", Why: why}}
			},
			`exceptions[0].from: "tools/**" matches no file`,
		},
		{
			"file in two modules",
			func(p *resolved.Policy) { setPaths(p, modTC, "pkg/tc/**", "pkg/chaos/**") },
			`modules: pkg_chaos and pkg_tc own the same files, such as pkg/chaos/chaos.go (2 files in all): make their paths not overlap`,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := base()
			tt.change(&p)
			err := p.CheckTree(tree)
			if err == nil {
				t.Fatalf("CheckTree: want %q", tt.want)
			}
			if err.Error() != tt.want {
				t.Errorf("CheckTree:\n got %q\nwant %q", err, tt.want)
			}
			if words := testutil.NotUsedIn(err.Error()); len(words) > 0 {
				t.Errorf("message uses %v, words design section 5 does not use", words)
			}
		})
	}
}

func TestCheckListsEveryMessage(t *testing.T) {
	p := base()
	setDeps(&p, modTC, "a", "b")
	err := p.Check()
	if err == nil || len(strings.Split(err.Error(), "\n")) != 2 {
		t.Errorf("Check = %v, want two lines", err)
	}
}

func TestModuleOf(t *testing.T) {
	p := base()
	setPaths(&p, modUtil, "pkg/**")
	tests := []struct {
		file, want string
		ok         bool
	}{
		{"pkg/chaos/chaos.go", modChaos, true}, // the nested module is more specific
		{"pkg/other/x.go", modUtil, true},
		{"cmd/main.go", "", false},
	}
	for _, tt := range tests {
		got, ok := p.ModuleOf(tt.file)
		if got != tt.want || ok != tt.ok {
			t.Errorf("ModuleOf(%q) = %q, %v; want %q, %v", tt.file, got, ok, tt.want, tt.ok)
		}
	}
	// A nested module is legal: the more specific glob owns the file.
	if err := p.CheckTree(tree[:4]); err != nil {
		t.Errorf("CheckTree with a nested module: %v", err)
	}
}
