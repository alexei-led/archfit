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
	"pkg/tc/netem.go", "pkg/tc/netem/netem.go", "pkg/tc/tc.go", "pkg/tc/tc_test.go",
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

func cyclePolicy() resolved.Policy {
	p := policy()
	for name, deps := range map[string][]string{util: {container}, container: {util}, tc: {runtime}, runtime: {tc}} {
		m := p.Modules[name]
		m.DependsOn = deps
		p.Modules[name] = m
	}
	return p
}

func cycleErrors(p resolved.Policy, facts evidence.CheckFacts) []change.Error {
	var out []change.Error
	for _, e := range change.Evaluate(p, facts).Errors {
		if e.Key.Rule == resolved.RuleCycle {
			out = append(out, e)
		}
	}
	return out
}

func TestEvaluateCycles(t *testing.T) {
	facts := evidence.CheckFacts{Sources: sources, Imports: []evidence.ImportFact{
		imp("pkg/util/util.go", 3, "pkg/container"),
		imp(containerFile, 4, "pkg/util"),
		imp(containerFile, 9, "pkg/util"),
		imp("pkg/tc/tc.go", 2, runtimeDir),
		imp("pkg/runtime/docker.go", 2, "pkg/tc"),
		imp("cmd/main.go", 1, "pkg/util"),
	}}
	got := cycleErrors(cyclePolicy(), facts)
	want := []change.Key{
		{Rule: resolved.RuleCycle, From: container, To: util},
		{Rule: resolved.RuleCycle, From: runtime, To: tc},
		{Rule: resolved.RuleCycle, From: tc, To: runtime},
		{Rule: resolved.RuleCycle, From: util, To: container},
	}
	if !reflect.DeepEqual(keys(got), want) {
		t.Fatalf("cycle keys =\n%v\nwant one per dependency inside a cycle\n%v", keys(got), want)
	}
	first := got[0]
	if first.What != "pkg_container -> pkg_util is in a cycle: pkg_container and pkg_util depend on each other" ||
		first.From != container || first.To != util || first.Next != fixStep() {
		t.Errorf("first cycle error = %+v", first)
	}
	// Design 7.2, added means more: one more use of a dependency inside a
	// cycle is one more site of the same key.
	wantSites := []change.Site{{File: containerFile, Line: 4}, {File: containerFile, Line: 9}}
	if !reflect.DeepEqual(first.Sites, wantSites) {
		t.Errorf("sites = %+v, want %+v", first.Sites, wantSites)
	}
}

func TestEvaluateCycleOfThree(t *testing.T) {
	p := resolved.Policy{Modules: map[string]resolved.Module{
		"a": {Paths: []string{"a/**"}, DependsOn: []string{"b"}},
		"b": {Paths: []string{"b/**"}, DependsOn: []string{"c"}},
		"c": {Paths: []string{"c/**"}, DependsOn: []string{"a", "d"}},
		"d": {Paths: []string{"d/**"}},
	}}.WithDefaults()
	facts := evidence.CheckFacts{Sources: []string{"a/a.go", "b/b.go", "c/c.go", "d/d.go"}, Imports: []evidence.ImportFact{
		imp("a/a.go", 1, "b"), imp("b/b.go", 1, "c"), imp("c/c.go", 1, "a"), imp("c/c.go", 2, "d"),
	}}
	got := cycleErrors(p, facts)
	if len(got) != 3 {
		t.Fatalf("got %v, want three dependencies in the cycle and not c -> d", keys(got))
	}
	if got[0].What != "a -> b is in a cycle: a, b and c depend on each other" {
		t.Errorf("what = %q", got[0].What)
	}
}

// A change that only deletes a dependency of a cycle adds no cycle key.
func TestEvaluateShrinkingCycleAddsNoKey(t *testing.T) {
	p := resolved.Policy{Modules: map[string]resolved.Module{
		"a": {Paths: []string{"a/**"}, DependsOn: []string{"b"}},
		"b": {Paths: []string{"b/**"}, DependsOn: []string{"a", "c"}},
		"c": {Paths: []string{"c/**"}, DependsOn: []string{"a"}},
	}}.WithDefaults()
	src := []string{"a/a.go", "b/b.go", "c/c.go"}
	base := evidence.CheckFacts{Sources: src, Imports: []evidence.ImportFact{
		imp("a/a.go", 1, "b"), imp("b/b.go", 1, "a"), imp("b/b.go", 2, "c"), imp("c/c.go", 1, "a"),
	}}
	head := evidence.CheckFacts{Sources: src, Imports: base.Imports[:3]}
	baseKeys := keys(cycleErrors(p, base))
	for _, k := range keys(cycleErrors(p, head)) {
		if !slices.Contains(baseKeys, k) {
			t.Errorf("head cycle key %+v is not on the base %v", k, baseKeys)
		}
	}
}

func TestEvaluateCycleAndUnownedModes(t *testing.T) {
	facts := evidence.CheckFacts{Sources: append(slices.Clone(sources), toolFile), Imports: []evidence.ImportFact{
		imp("pkg/util/util.go", 3, "pkg/container"), imp(containerFile, 4, "pkg/util"),
	}}
	for _, mode := range []resolved.Mode{resolved.ModeDeny, resolved.ModeAsk, resolved.ModeOff} {
		t.Run(string(mode), func(t *testing.T) {
			p := cyclePolicy()
			p.Builtin.Cycle, p.Builtin.Unowned = mode, mode
			seen := map[string]bool{}
			for _, e := range change.Evaluate(p, facts).Errors {
				seen[e.Key.Rule] = true
				want := fixStep()
				if mode == resolved.ModeAsk {
					want = requestStep()
				}
				if e.Next != want {
					t.Errorf("%s in %s: next %+v, want %+v", e.Key.Rule, mode, e.Next, want)
				}
			}
			if on := mode != resolved.ModeOff; seen[resolved.RuleCycle] != on || seen[resolved.RuleUnowned] != on {
				t.Errorf("%s: rules with errors %v", mode, seen)
			}
		})
	}
}

func TestEvaluateTargets(t *testing.T) {
	p := resolved.Policy{Modules: map[string]resolved.Module{
		modApp:  {Paths: []string{"src/app/**"}, DependsOn: []string{"ui", "files"}},
		"ui":    {Paths: []string{"src/ui/**"}, Public: []string{"src/ui/api"}, Volatility: resolved.VolatilityHigh},
		"files": {Paths: []string{fooGlob}, Volatility: resolved.VolatilityHigh},
		"wide":  {Paths: []string{"pkg/**"}},
	}}.WithDefaults()
	src := []string{appTS, "src/ui/api/index.ts", storeTS, "src/app/main.go", fooFile}
	tests := []struct {
		name, file, target string
		want               []change.Key
	}{
		{"a file of a public directory", appTS, "src/ui/api/index.ts", nil},
		{"a file of a public directory that is not a source", appTS, "src/ui/api/Button.vue", nil},
		{"a file past the public part", appTS, storeTS,
			[]change.Key{{Rule: resolved.RuleCoupling, From: modApp, To: "ui", Symbol: storeTS}}},
		{"a package directory owned by file globs", "src/app/main.go", fooDir, nil},
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

func TestEvaluateFileGlobsCoverAPackage(t *testing.T) {
	p := resolved.Policy{
		Modules: map[string]resolved.Module{
			modApp: {Paths: []string{"app/**"}, DependsOn: []string{modFoo, "rt"}},
			modFoo: {Paths: []string{fooGlob, "pkg/foo/internal/**"}, Public: []string{fooGlob}, Volatility: resolved.VolatilityHigh},
			"rt":   {Paths: []string{"pkg/rt/**"}},
		},
		Rules: []resolved.Rule{{ID: "no-rt-files", Form: resolved.FormDeny, From: []string{modApp}, To: []string{"pkg/rt/*.go"}, Why: "w", Instead: "i"}},
	}.WithDefaults()
	src := []string{"app/main.go", fooFile, "pkg/foo/internal/x.go", "pkg/rt/rt.go"}
	tests := []struct {
		name, target string
		want         []change.Key
	}{
		{"a public file glob covers the package", fooDir, nil},
		{"a package past the public file glob", "pkg/foo/internal",
			[]change.Key{{Rule: resolved.RuleCoupling, From: modApp, To: modFoo, Symbol: "pkg/foo/internal"}}},
		{"a deny file glob covers the package", "pkg/rt",
			[]change.Key{{Rule: "no-rt-files", From: modApp, To: "rt", Symbol: "pkg/rt"}}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			facts := evidence.CheckFacts{Sources: src, Imports: []evidence.ImportFact{imp("app/main.go", 1, tt.target)}}
			if got := keys(change.Evaluate(p, facts).Errors); !reflect.DeepEqual(got, append([]change.Key{}, tt.want...)) {
				t.Errorf("got %v, want %v", got, tt.want)
			}
		})
	}
}

func TestEvaluateExcludedFilesDoNotDecideADirectory(t *testing.T) {
	p := resolved.Policy{
		Modules: map[string]resolved.Module{
			modApp:     {Paths: []string{"app/**"}},
			modFoo:     {Paths: []string{fooGlob}},
			"fixtures": {Paths: []string{"pkg/foo/foo_test.go"}},
		},
		Tests: []string{testGlob},
	}.WithDefaults()
	facts := evidence.CheckFacts{
		Sources: []string{"app/main.go", fooFile, "pkg/foo/foo_test.go"},
		Imports: []evidence.ImportFact{imp("app/main.go", 1, fooDir)},
	}
	want := []change.Key{{Rule: resolved.RuleGraph, From: modApp, To: modFoo, Symbol: fooDir}}
	if got := keys(change.Evaluate(p, facts).Errors); !reflect.DeepEqual(got, want) {
		t.Errorf("got %v, want %v: a test file of another module must not hide the package's module", got, want)
	}
}
