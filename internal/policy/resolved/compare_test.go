package resolved_test

import (
	"reflect"
	"testing"
	"time"

	"github.com/alexei-led/archfit/v3/internal/policy/resolved"
)

const (
	on         = "true"
	owner2     = "octocat"
	denyFrom   = "rules.deny-r.deny.from"
	dupExpires = "exceptions (duplication pkg_tc → pkg_util).expires"
)

func day(d int) time.Time { return time.Date(2026, 11, d, 0, 0, 0, 0, time.UTC) }

// comparePolicy is base() with a rule, an exception and settings to change.
func comparePolicy() resolved.Policy {
	p := base()
	p.Rules = []resolved.Rule{
		{ID: "deny-r", Form: resolved.FormDeny, From: []string{"pkg/chaos/**"}, To: []string{modTC}, Why: why, Instead: "do y"},
		{ID: "only-r", Form: resolved.FormOnly, Use: resolved.SetProcess, In: []string{modUtil}, Why: why},
		{ID: "note-r", Form: resolved.FormNote, Note: "n", Why: why},
	}
	p.Exceptions = []resolved.Exception{{Rule: resolved.RuleDuplication, From: modTC, To: modUtil, Expires: day(19), Why: why}}
	p.Owners = []string{owner, owner2}
	p.Generated = []string{"gen/**"}
	p.Settings = map[string]string{"languages.go.enabled": on, "coverage.enabled": on}
	return p
}

func TestCompare(t *testing.T) {
	const (
		lo = resolved.Loosens
		ti = resolved.Tightens
		ne = resolved.Neither
	)
	tests := []struct {
		name   string
		change func(p *resolved.Policy)
		want   []resolved.Change
	}{
		{"no change", func(*resolved.Policy) {}, nil},
		// Loosens, and the reverse tightens.
		{"added depends_on", func(p *resolved.Policy) { setDeps(p, modChaos, modUtil, modTC) }, []resolved.Change{
			{lo, "modules.pkg_chaos.depends_on", `adds "pkg_tc"`},
		}},
		{"removed depends_on", func(p *resolved.Policy) { setDeps(p, modChaos) }, []resolved.Change{
			{ti, "modules.pkg_chaos.depends_on", `removes "pkg_util"`},
		}},
		{"added module path", func(p *resolved.Policy) { setPaths(p, modTC, "pkg/tc/**", "pkg/net/**") }, []resolved.Change{
			{lo, "modules.pkg_tc.paths", `adds "pkg/net/**"`},
		}},
		{"removed module path", func(p *resolved.Policy) { setPaths(p, modChaos, "pkg/chaos/core/**") }, []resolved.Change{
			{lo, "modules.pkg_chaos.paths", `removes "pkg/chaos/**"`},
		}},
		{"added module", func(p *resolved.Policy) {
			p.Modules["cli"] = resolved.Module{Paths: []string{cmdGlob}}
		}, []resolved.Change{{lo, "modules.cli", "adds the module"}}},
		{"removed module", func(p *resolved.Policy) { delete(p.Modules, modTC) }, []resolved.Change{
			{lo, "modules.pkg_tc", "removes the module"},
		}},
		{"wider public", func(p *resolved.Policy) { setPublic(p, modChaos, "pkg/chaos", "pkg/chaos/cli") }, []resolved.Change{
			{lo, "modules.pkg_chaos.public", `adds "pkg/chaos/cli"`},
		}},
		{"public replaced by a covering glob", func(p *resolved.Policy) { setPublic(p, modChaos, "pkg/chaos/**") }, []resolved.Change{
			{lo, "modules.pkg_chaos.public", `adds "pkg/chaos/**"`},
		}},
		{"public added to a whole module", func(p *resolved.Policy) { setPublic(p, modTC, "pkg/tc") }, []resolved.Change{
			{ti, "modules.pkg_tc.public", `removes "pkg/tc/**"`},
		}},
		{"path change of a whole-module public is said once", func(p *resolved.Policy) {
			setPaths(p, modTC, "pkg/tc/**", "pkg/net/**")
			setPublic(p, modTC, "pkg/tc/**", "pkg/net/**")
		}, []resolved.Change{{lo, "modules.pkg_tc.paths", `adds "pkg/net/**"`}}},
		{"lower volatility", func(p *resolved.Policy) { setVolatility(p, modTC, resolved.VolatilityMedium) }, []resolved.Change{
			{lo, "modules.pkg_tc.volatility", "high → medium"},
		}},
		{"higher volatility", func(p *resolved.Policy) { setVolatility(p, modUtil, resolved.VolatilityHigh) }, []resolved.Change{
			{ti, "modules.pkg_util.volatility", "low → high"},
		}},
		{"subdomain that keeps the volatility", func(p *resolved.Policy) {
			m := p.Modules[modTC]
			m.Subdomain = resolved.SubdomainCore
			p.Modules[modTC] = m
		}, []resolved.Change{{ne, "modules.pkg_tc.subdomain", "absent → core"}}},
		{"mode toward off", func(p *resolved.Policy) { p.Builtin.Coupling = resolved.ModeAsk }, []resolved.Change{
			{lo, "builtin.coupling", "deny → ask"},
		}},
		{"mode toward deny", func(p *resolved.Policy) { p.Builtin.Graph = resolved.ModeDeny }, []resolved.Change{
			{ti, "builtin.graph", "ask → deny"},
		}},
		{"lower min_balance", func(p *resolved.Policy) { p.Builtin.MinBalance = 6 }, []resolved.Change{
			{lo, "builtin.min_balance", "7 → 6"},
		}},
		{"higher min_balance", func(p *resolved.Policy) { p.Builtin.MinBalance = 8 }, []resolved.Change{
			{ti, "builtin.min_balance", "7 → 8"},
		}},
		{"removed rule", func(p *resolved.Policy) { p.Rules = p.Rules[1:] }, []resolved.Change{
			{lo, "rules.deny-r", "removes the deny rule"},
		}},
		{"removed note", func(p *resolved.Policy) { p.Rules = p.Rules[:2] }, []resolved.Change{
			{lo, "rules.note-r", "removes the note rule"},
		}},
		{"added rule", func(p *resolved.Policy) {
			p.Rules = append(p.Rules, resolved.Rule{ID: "new", Form: resolved.FormNote, Note: "m", Why: why})
		}, []resolved.Change{{ti, "rules.new", "adds a note rule"}}},
		{"deny from a module to a directory of its name", func(p *resolved.Policy) {
			p.Rules[0].From = []string{modChaos}
		}, []resolved.Change{
			{ti, denyFrom, `adds "pkg_chaos"`},
			{lo, denyFrom, `removes "pkg/chaos/**"`},
		}},
		{"deny from wider", func(p *resolved.Policy) { p.Rules[0].From = []string{"pkg/**"} }, []resolved.Change{
			{ti, denyFrom, `adds "pkg/**"`},
		}},
		{"deny to narrower", func(p *resolved.Policy) { p.Rules[0].To = []string{"pkg/tc/x/**"} }, []resolved.Change{
			{ti, "rules.deny-r.deny.to", `adds "pkg/tc/x/**"`},
			{lo, "rules.deny-r.deny.to", `removes "pkg_tc"`},
		}},
		{"only in more modules", func(p *resolved.Policy) { p.Rules[1].In = []string{modUtil, modTC} }, []resolved.Change{
			{lo, "rules.only-r.only.in", `adds "pkg_tc"`},
		}},
		{"only in fewer modules", func(p *resolved.Policy) { p.Rules[1].In = nil }, []resolved.Change{
			{ti, "rules.only-r.only.in", `removes "pkg_util"`},
		}},
		{"only of another library", func(p *resolved.Policy) { p.Rules[1].Use = resolved.SetNetwork }, []resolved.Change{
			{lo, "rules.only-r.only.use", `"process" → "network"`},
		}},
		{"note turned into deny", func(p *resolved.Policy) {
			p.Rules[2] = resolved.Rule{ID: "note-r", Form: resolved.FormDeny, From: []string{modUtil}, To: []string{modTC}, Why: why, Instead: "z"}
		}, []resolved.Change{
			{ti, "rules.note-r", "note → deny"},
			{ne, "rules.note-r.note", `"n" → ""`},
			{ne, "rules.note-r.instead", `"" → "z"`},
		}},
		{"deny turned into note", func(p *resolved.Policy) {
			p.Rules[0] = resolved.Rule{ID: "deny-r", Form: resolved.FormNote, Note: "n", Why: why}
		}, []resolved.Change{
			{lo, "rules.deny-r", "deny → note"},
			{ne, "rules.deny-r.note", `"" → "n"`},
			{ne, "rules.deny-r.instead", `"do y" → ""`},
		}},
		{"added exception", func(p *resolved.Policy) {
			p.Exceptions = append(p.Exceptions, resolved.Exception{Rule: resolved.RuleGraph, From: modTC, To: modChaos, Why: why})
		}, []resolved.Change{{lo, "exceptions (graph pkg_tc → pkg_chaos)", "adds the exception"}}},
		{"removed exception", func(p *resolved.Policy) { p.Exceptions = nil }, []resolved.Change{
			{ti, "exceptions (duplication pkg_tc → pkg_util)", "removes the exception"},
		}},
		{"later expires", func(p *resolved.Policy) { p.Exceptions[0].Expires = day(26) }, []resolved.Change{
			{lo, dupExpires, "2026-11-19 → 2026-11-26"},
		}},
		{"earlier expires", func(p *resolved.Policy) { p.Exceptions[0].Expires = day(12) }, []resolved.Change{
			{ti, dupExpires, "2026-11-19 → 2026-11-12"},
		}},
		{"removed expires", func(p *resolved.Policy) { p.Exceptions[0].Expires = time.Time{} }, []resolved.Change{
			{lo, dupExpires, "2026-11-19 → no date"},
		}},
		{"duplication pair written the other way", func(p *resolved.Policy) {
			p.Exceptions[0].From, p.Exceptions[0].To = modUtil, modTC
		}, nil},
		{"added generated glob", func(p *resolved.Policy) { p.Generated = append(p.Generated, "api/**") }, []resolved.Change{
			{lo, "generated", `adds "api/**"`},
		}},
		{"removed generated glob", func(p *resolved.Policy) { p.Generated = nil }, []resolved.Change{
			{ti, "generated", `removes "gen/**"`},
		}},
		{"added tests glob", func(p *resolved.Policy) { p.Tests = []string{"**/*_test.go"} }, []resolved.Change{
			{lo, "tests", `adds "**/*_test.go"`},
		}},
		{"added exclude glob", func(p *resolved.Policy) { p.Exclude = []string{"vendor/**"} }, []resolved.Change{
			{lo, "exclude", `adds "vendor/**"`},
		}},
		{"removed owner", func(p *resolved.Policy) { p.Owners = []string{owner2} }, []resolved.Change{
			{lo, "owners", "[alexei-led, octocat] → [octocat]"},
		}},
		{"owners in another order", func(p *resolved.Policy) { p.Owners = []string{owner2, owner} }, nil},
		{"gate to required", func(p *resolved.Policy) { p.Gate = resolved.GateRequired }, []resolved.Change{
			{ti, "gate", "advisory → required"},
		}},
		{"layers in another order", func(p *resolved.Policy) { p.Layers = []string{"cmd", core} }, []resolved.Change{
			{lo, "layers", "[core, cmd] → [cmd, core]"},
		}},
		{"layer added to a module", func(p *resolved.Policy) { setLayer(p, modTC, core) }, []resolved.Change{
			{ti, "modules.pkg_tc.layer", "absent → core"},
		}},
		{"layer removed from a module", func(p *resolved.Policy) { setLayer(p, modChaos, "") }, []resolved.Change{
			{lo, "modules.pkg_chaos.layer", "core → absent"},
		}},
		{"language setting", func(p *resolved.Policy) { p.Settings["languages.go.enabled"] = "false" }, []resolved.Change{
			{lo, "languages.go.enabled", "true → false"},
		}},
		{"added analyzer setting", func(p *resolved.Policy) { p.Settings["analyzers.syntax.enabled"] = on }, []resolved.Change{
			{lo, "analyzers.syntax.enabled", "absent → true"},
		}},
		{"coverage setting", func(p *resolved.Policy) { delete(p.Settings, "coverage.enabled") }, []resolved.Change{
			{ne, "coverage.enabled", "true → absent"},
		}},
		// Neither: text.
		{"why, instead and note text", func(p *resolved.Policy) {
			p.Rules[0].Why, p.Rules[0].Instead, p.Rules[2].Note = "b", "c", "d"
			p.Exceptions[0].Why = "e"
		}, []resolved.Change{
			{ne, "rules.deny-r.why", `"a reason" → "b"`},
			{ne, "rules.deny-r.instead", `"do y" → "c"`},
			{ne, "rules.note-r.note", `"n" → "d"`},
			{ne, "exceptions (duplication pkg_tc → pkg_util).why", `"a reason" → "e"`},
		}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			before, after := comparePolicy(), comparePolicy()
			tt.change(&after)
			got := resolved.Compare(before, after)
			if !reflect.DeepEqual(got.Changes, tt.want) {
				t.Errorf("Changes:\n got %+v\nwant %+v", got.Changes, tt.want)
			}
		})
	}
}

func TestCompareDirection(t *testing.T) {
	tests := []struct {
		name   string
		change func(p *resolved.Policy)
		want   resolved.Direction
	}{
		{"nothing", func(*resolved.Policy) {}, resolved.Neither},
		{"text only", func(p *resolved.Policy) { p.Rules[0].Why = "b" }, resolved.Neither},
		{"tightens only", func(p *resolved.Policy) {
			p.Builtin.MinBalance = 8
			p.Rules[0].Why = "b"
		}, resolved.Tightens},
		{"loosens", func(p *resolved.Policy) { p.Builtin.MinBalance = 6 }, resolved.Loosens},
		{"both kinds loosens", func(p *resolved.Policy) {
			p.Builtin.MinBalance = 8
			p.Builtin.Cycle = resolved.ModeOff
		}, resolved.Loosens},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			before, after := comparePolicy(), comparePolicy()
			tt.change(&after)
			if got := resolved.Compare(before, after).Direction; got != tt.want {
				t.Errorf("Direction = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestCompareGate(t *testing.T) {
	required, advisory := comparePolicy(), comparePolicy()
	required.Gate = resolved.GateRequired
	if c := resolved.Compare(required, advisory); c.Direction != resolved.Loosens ||
		!reflect.DeepEqual(c.Changes, []resolved.Change{{resolved.Loosens, "gate", "required → advisory"}}) {
		t.Errorf("required → advisory = %+v, want loosens", c)
	}
	if c := resolved.Compare(advisory, required); c.Direction != resolved.Tightens {
		t.Errorf("advisory → required = %+v, want tightens", c)
	}
}

func TestCompareModuleNameIsNotADirectory(t *testing.T) {
	before, after := comparePolicy(), comparePolicy()
	before.Modules["app"] = resolved.Module{Paths: []string{"src/**"}}
	after.Modules["app"] = resolved.Module{Paths: []string{"src/**"}}
	before.Rules[0].From = []string{"app"}
	after.Rules[0].From = []string{"app/**"}
	if c := resolved.Compare(before, after); c.Direction != resolved.Loosens {
		t.Errorf("deny from module app to path app/** = %+v, want loosens", c)
	}
}

func setPublic(p *resolved.Policy, module string, public ...string) {
	m := p.Modules[module]
	m.Public = public
	p.Modules[module] = m
}

func setVolatility(p *resolved.Policy, module string, v resolved.Volatility) {
	m := p.Modules[module]
	m.Volatility = v
	p.Modules[module] = m
}
