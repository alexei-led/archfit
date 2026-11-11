package change_test

import (
	"reflect"
	"slices"
	"testing"

	"github.com/alexei-led/archfit/v3/internal/assessment/change"
	"github.com/alexei-led/archfit/v3/internal/model/evidence"
	"github.com/alexei-led/archfit/v3/internal/policy/resolved"
)

var sources = []string{
	"cmd/main.go",
	"pkg/chaos/chaos.go", "pkg/chaos/netem/netem.go",
	containerFile,
	"pkg/runtime/docker.go",
	"pkg/tc/netem.go", "pkg/tc/tc.go", "pkg/tc/tc_test.go",
	"pkg/util/util.go", "pkg/util/util.pb.go",
}

func imp(file string, line int, target string) evidence.ImportFact {
	return evidence.ImportFact{File: file, Line: line, Target: target}
}

func keys(errs []change.Error) []change.Key {
	out := make([]change.Key, len(errs))
	for i, e := range errs {
		out[i] = e.Key
	}
	return out
}

func TestEvaluateGroupsSitesByKey(t *testing.T) {
	facts := evidence.CheckFacts{Sources: sources, Imports: []evidence.ImportFact{
		imp("pkg/tc/tc.go", 5, "pkg/util"),
		imp("pkg/tc/netem.go", 12, "pkg/util"),
	}}
	got := change.Evaluate(policy(), facts).Errors
	if len(got) != 1 {
		t.Fatalf("got %d errors %v, want one graph error", len(got), keys(got))
	}
	want := []change.Site{{File: "pkg/tc/netem.go", Line: 12}, {File: "pkg/tc/tc.go", Line: 5}}
	if !reflect.DeepEqual(got[0].Sites, want) {
		t.Errorf("sites = %+v, want %+v", got[0].Sites, want)
	}
}

func TestEvaluateUsesJudge(t *testing.T) {
	use := imp("cmd/main.go", 3, "pkg/tc/netem")
	got := change.Evaluate(policy(), evidence.CheckFacts{Sources: sources, Imports: []evidence.ImportFact{use}}).Errors
	want := change.Judge(policy(), fileUse(use.File, use.Target))
	for i := range want {
		want[i].Sites = []change.Site{{File: use.File, Line: use.Line}}
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("Evaluate =\n%+v\nJudge on the same use =\n%+v", got, want)
	}
}

func TestEvaluateCycles(t *testing.T) {
	p := policy()
	for name, deps := range map[string][]string{util: {container}, container: {util}, tc: {runtime}, runtime: {tc}} {
		m := p.Modules[name]
		m.DependsOn = deps
		p.Modules[name] = m
	}
	facts := evidence.CheckFacts{Sources: sources, Imports: []evidence.ImportFact{
		imp("pkg/util/util.go", 3, "pkg/container"),
		imp(containerFile, 4, "pkg/util"),
		imp(containerFile, 9, "pkg/util"),
		imp("pkg/tc/tc.go", 2, runtimeDir),
		imp("pkg/runtime/docker.go", 2, "pkg/tc"),
		imp("cmd/main.go", 1, "pkg/util"),
	}}
	var cycles []change.Error
	for _, e := range change.Evaluate(p, facts).Errors {
		if e.Key.Rule == resolved.RuleCycle {
			cycles = append(cycles, e)
		}
	}
	if len(cycles) != 2 {
		t.Fatalf("got %d cycles %v, want 2", len(cycles), keys(cycles))
	}
	first := cycles[0]
	if first.Key.Symbol != "pkg_container,pkg_util" || first.What != "pkg_container and pkg_util depend on each other" {
		t.Errorf("first cycle = %+v", first)
	}
	wantSites := []change.Site{{File: containerFile, Line: 4}, {File: containerFile, Line: 9}, {File: "pkg/util/util.go", Line: 3}}
	if !reflect.DeepEqual(first.Sites, wantSites) {
		t.Errorf("one more use of a broken dependency is one more site: got %+v, want %+v", first.Sites, wantSites)
	}
	if cycles[1].Key.Symbol != "pkg_runtime,pkg_tc" {
		t.Errorf("second cycle = %+v", cycles[1].Key)
	}

	p.Builtin.Cycle = resolved.ModeOff
	for _, e := range change.Evaluate(p, facts).Errors {
		if e.Key.Rule == resolved.RuleCycle {
			t.Errorf("cycle off: got %+v", e.Key)
		}
	}
}

func TestEvaluateCycleOfThree(t *testing.T) {
	p := resolved.Policy{Modules: map[string]resolved.Module{
		"a": {Paths: []string{"a/**"}, DependsOn: []string{"b"}},
		"b": {Paths: []string{"b/**"}, DependsOn: []string{"c"}},
		"c": {Paths: []string{"c/**"}, DependsOn: []string{"a"}},
	}}.WithDefaults()
	facts := evidence.CheckFacts{Sources: []string{"a/a.go", "b/b.go", "c/c.go"}, Imports: []evidence.ImportFact{
		imp("a/a.go", 1, "b"), imp("b/b.go", 1, "c"), imp("c/c.go", 1, "a"),
	}}
	got := change.Evaluate(p, facts).Errors
	if len(got) != 1 || got[0].Key.Symbol != "a,b,c" || got[0].What != "a, b and c depend on each other" || len(got[0].Sites) != 3 {
		t.Errorf("got %+v, want one cycle of a, b and c with three sites", got)
	}
}

func TestEvaluateTargets(t *testing.T) {
	p := resolved.Policy{Modules: map[string]resolved.Module{
		"app":   {Paths: []string{"src/app/**"}, DependsOn: []string{"ui", "files"}},
		"ui":    {Paths: []string{"src/ui/**"}, Public: []string{"src/ui/api"}, Volatility: resolved.VolatilityHigh},
		"files": {Paths: []string{"pkg/foo/*.go"}, Volatility: resolved.VolatilityHigh},
		"wide":  {Paths: []string{"pkg/**"}},
	}}.WithDefaults()
	src := []string{appTS, "src/ui/api/index.ts", storeTS, "src/app/main.go", "pkg/foo/foo.go"}
	tests := []struct {
		name, file, target string
		want               []change.Key
	}{
		{"a file of a public directory", appTS, "src/ui/api/index.ts", nil},
		{"a file past the public part", appTS, storeTS,
			[]change.Key{{Rule: resolved.RuleCoupling, From: "app", To: "ui", Symbol: storeTS}}},
		{"a package directory owned by file globs", "src/app/main.go", "pkg/foo", nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			facts := evidence.CheckFacts{Sources: src, Imports: []evidence.ImportFact{imp(tt.file, 1, tt.target)}}
			if got := keys(change.Evaluate(p, facts).Errors); !reflect.DeepEqual(got, append([]change.Key{}, tt.want...)) {
				t.Errorf("got %v, want %v", got, tt.want)
			}
		})
	}
}

func TestEvaluateUnowned(t *testing.T) {
	tree := append(slices.Clone(sources), toolFile, "scripts/tool_test.go", "gen/api.pb.go")
	tests := []struct {
		name   string
		tweak  func(*resolved.Policy)
		wanted []string
	}{
		{"declared tests and generated are out", func(*resolved.Policy) {}, []string{toolFile}},
		{"no tests declared", func(p *resolved.Policy) { p.Tests = nil }, []string{toolFile, "scripts/tool_test.go"}},
		{"no generated declared", func(p *resolved.Policy) { p.Generated = nil }, []string{"gen/api.pb.go", toolFile}},
		{"off", func(p *resolved.Policy) { p.Builtin.Unowned = resolved.ModeOff }, nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := policy()
			tt.tweak(&p)
			var got []string
			for _, e := range change.Evaluate(p, evidence.CheckFacts{Sources: tree}).Errors {
				if e.Key.Rule != resolved.RuleUnowned {
					continue
				}
				got = append(got, e.Key.Symbol)
				if e.Sites[0] != (change.Site{File: e.Key.Symbol}) || e.What != "production code in no module" || e.Next != fixStep() {
					t.Errorf("unowned error = %+v", e)
				}
			}
			if !reflect.DeepEqual(got, tt.wanted) {
				t.Errorf("unowned files = %v, want %v", got, tt.wanted)
			}
		})
	}
}

func TestEvaluateSkipsTestAndGeneratedImporters(t *testing.T) {
	facts := evidence.CheckFacts{Sources: sources, Imports: []evidence.ImportFact{
		imp("pkg/tc/tc_test.go", 3, "pkg/util"),
		imp("pkg/util/util.pb.go", 3, "pkg/tc"),
	}}
	if got := change.Evaluate(policy(), facts).Errors; len(got) != 0 {
		t.Errorf("got %v, want no error", keys(got))
	}
}

func TestEvaluateRecordOrder(t *testing.T) {
	facts := evidence.CheckFacts{Sources: append(slices.Clone(sources), toolFile), Imports: []evidence.ImportFact{
		imp("pkg/tc/tc.go", 9, "pkg/util"),
		imp("pkg/chaos/chaos.go", 20, runtimeDir),
		{File: "pkg/chaos/chaos.go", Line: 4, Target: "os/exec", External: true},
	}}
	want := []change.Key{
		{Rule: onlyProcess, From: chaos, To: resolved.SetProcess, Symbol: "os/exec"},
		{Rule: resolved.RuleGraph, From: chaos, To: runtime, Symbol: runtimeDir},
		{Rule: denyRuntime, From: chaos, To: runtime, Symbol: runtimeDir},
		{Rule: resolved.RuleGraph, From: tc, To: util, Symbol: "pkg/util"},
		{Rule: resolved.RuleUnowned, Symbol: toolFile},
	}
	if got := keys(change.Evaluate(policy(), facts).Errors); !reflect.DeepEqual(got, want) {
		t.Errorf("order =\n%v\nwant\n%v", got, want)
	}
}

func TestEvaluateNotesAndEmptyFacts(t *testing.T) {
	got := change.Evaluate(policy(), evidence.CheckFacts{})
	if len(got.Errors) != 0 {
		t.Errorf("empty facts: got %v", keys(got.Errors))
	}
	if len(got.NotChecked) != 1 || got.NotChecked[0].ID != noteFanOut {
		t.Errorf("not checked = %+v, want the note %s", got.NotChecked, noteFanOut)
	}
}
