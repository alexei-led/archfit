package change_test

import (
	"fmt"
	"reflect"
	"testing"

	"github.com/alexei-led/archfit/v3/internal/assessment/change"
	"github.com/alexei-led/archfit/v3/internal/model/result"
	"github.com/alexei-led/archfit/v3/internal/policy/resolved"
	"github.com/alexei-led/archfit/v3/internal/testutil"
)

const (
	chaos     = "pkg_chaos"
	tc        = "pkg_tc"
	util      = "pkg_util"
	container = "pkg_container"
	runtime   = "pkg_runtime"
	cmd       = "cmd"
	core      = "core"

	runtimeDir    = "pkg/runtime"
	runtimeGlob   = "pkg/runtime/**"
	cmdGlob       = "cmd/**"
	toolFile      = "scripts/tool.go"
	appTS         = "src/app/main.ts"
	containerFile = "pkg/container/container.go"
	storeTS       = "src/ui/state/store.ts"
	testGlob      = "**/*_test.go"
	pbGlob        = "**/*.pb.go"
	modApp        = "app"
	modFoo        = "foo"
	fooGlob       = "pkg/foo/*.go"
	fooFile       = "pkg/foo/foo.go"
	fooDir        = "pkg/foo"
	appGlob       = "app/**"
	appMain       = "app/main.go"

	denyRuntime = "no-chaos-to-runtime"
	denyCmd     = "no-cmd-to-runtime"
	onlyDocker  = "docker-sdk-in-runtime"
	onlyProcess = "process-in-runtime"
	noteFanOut  = "fan-out-through-runner"

	dockerSDK     = "github.com/docker/docker"
	whyRuntime    = "A direct import ties a chaos action to one runtime."
	insteadClient = "Add a method to container.Client."
	requestNext   = `archfit request -m "<why you need it>"`
)

// policy is the typical policy of design 6.1, with the defaults applied.
func policy() resolved.Policy {
	return resolved.Policy{
		Owners: []string{"alexei-led"},
		Layers: []string{core, cmd},
		Modules: map[string]resolved.Module{
			chaos: {
				Paths: []string{"pkg/chaos/**"}, Public: []string{"pkg/chaos", "pkg/chaos/cliflags"},
				Volatility: resolved.VolatilityHigh, Layer: core, DependsOn: []string{container, util},
			},
			tc:        {Paths: []string{"pkg/tc/**"}, Public: []string{"pkg/tc"}, Volatility: resolved.VolatilityHigh, Layer: core},
			util:      {Paths: []string{"pkg/util/**"}, Volatility: resolved.VolatilityLow, Layer: core},
			container: {Paths: []string{"pkg/container/**"}, Volatility: resolved.VolatilityMedium, Layer: core},
			runtime:   {Paths: []string{runtimeGlob}, Volatility: resolved.VolatilityMedium, Layer: core},
			cmd:       {Paths: []string{cmdGlob}, Layer: cmd, DependsOn: []string{chaos, tc, util, container, runtime}},
		},
		Rules: []resolved.Rule{
			{ID: denyRuntime, Form: resolved.FormDeny, From: []string{chaos}, To: []string{runtime}, Why: whyRuntime, Instead: insteadClient},
			{ID: denyCmd, Form: resolved.FormDeny, From: []string{cmdGlob}, To: []string{runtimeGlob}, Why: "w", Instead: "i"},
			{ID: onlyDocker, Form: resolved.FormOnly, Use: dockerSDK, In: []string{runtime}, Why: "One adapter owns the Docker API."},
			{ID: onlyProcess, Form: resolved.FormOnly, Use: resolved.SetProcess, In: []string{runtime, "scripts/**"}, Why: "Side effects stay in adapters."},
			{ID: noteFanOut, Form: resolved.FormNote, Note: "Chaos actions fan out through RunOnContainers.", Why: "One place checks the limit."},
		},
		Tests:     []string{"**/*_test.go"},
		Generated: []string{pbGlob},
	}.WithDefaults()
}

// fileUse is a Go import of a file of the tree.
func fileUse(file, target string) change.Use {
	return change.Use{From: change.End{Path: file, IsFile: true}, To: change.End{Path: target}, Lang: resolved.LanguageGo}
}

// libraryUse is an import of a library.
func libraryUse(file, library string, lang resolved.Language) change.Use {
	return change.Use{From: change.End{Path: file, IsFile: true}, To: change.End{Path: library, Library: true}, Lang: lang}
}

// byRule returns the error of one rule, and whether there is one.
func byRule(errs []change.Error, rule string) (change.Error, bool) {
	for _, e := range errs {
		if e.Key.Rule == rule {
			return e, true
		}
	}
	return change.Error{}, false
}

func requestStep() result.Next {
	return result.Next{Kind: result.NextCommand, Text: requestNext, Note: "do not copy the code"}
}

func fixStep() result.Next { return result.Next{Kind: result.NextFix, Text: "fix the code"} }

// The record of design 8.1, "a dependency only an owner can allow".
func TestJudgeGraphExample(t *testing.T) {
	got := change.Judge(policy(), fileUse("pkg/tc/netem.go", "pkg/util"))
	want := []change.Error{{
		Key:  change.Key{Rule: resolved.RuleGraph, From: tc, To: util, Symbol: "pkg/util"},
		From: tc, To: util,
		What: "pkg_tc -> pkg_util is not an allowed dependency", Phrase: "not an allowed dependency",
		Why:     "a new dependency between modules needs an owner",
		Balance: &result.Balance{Value: 9, Min: 7}, Next: requestStep(),
	}}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("Judge =\n%+v\nwant\n%+v", got, want)
	}
}

func TestJudgeAllowedDependency(t *testing.T) {
	if got := change.Judge(policy(), fileUse("pkg/chaos/chaos.go", "pkg/util")); len(got) != 0 {
		t.Errorf("an allowed use of the public part: got %+v, want no error", got)
	}
}

func TestJudgeModuleNamedAsTarget(t *testing.T) {
	use := change.Use{From: change.End{Module: tc}, To: change.End{Module: util}}
	got, ok := byRule(change.Judge(policy(), use), resolved.RuleGraph)
	if !ok || got.Key.Symbol != util || *got.Balance != (result.Balance{Value: 9, Min: 7}) {
		t.Errorf("a module name means its public part: got %+v, %v", got, ok)
	}
}

func TestJudgeModes(t *testing.T) {
	tests := []struct {
		rule string
		set  func(*resolved.Builtin, resolved.Mode)
		use  change.Use
	}{
		{resolved.RuleGraph, func(b *resolved.Builtin, m resolved.Mode) { b.Graph = m }, fileUse("pkg/tc/tc.go", "pkg/util")},
		{resolved.RuleCoupling, func(b *resolved.Builtin, m resolved.Mode) { b.Coupling = m }, fileUse("cmd/main.go", "pkg/tc/netem")},
		{resolved.RuleLayers, func(b *resolved.Builtin, m resolved.Mode) { b.Layers = m }, fileUse("pkg/util/util.go", "cmd")},
	}
	for _, tt := range tests {
		for _, mode := range []resolved.Mode{resolved.ModeDeny, resolved.ModeAsk, resolved.ModeOff} {
			t.Run(tt.rule+" "+string(mode), func(t *testing.T) {
				p := policy()
				tt.set(&p.Builtin, mode)
				got, ok := byRule(change.Judge(p, tt.use), tt.rule)
				switch {
				case mode == resolved.ModeOff && ok:
					t.Errorf("off: got %+v, want no error", got)
				case mode == resolved.ModeDeny && (!ok || got.Next != fixStep()):
					t.Errorf("deny: got %+v, %v; want next %+v", got.Next, ok, fixStep())
				case mode == resolved.ModeAsk && (!ok || got.Next != requestStep()):
					t.Errorf("ask: got %+v, %v; want next %+v", got.Next, ok, requestStep())
				}
			})
		}
	}
}

// Design 6.2: a use of the public part passes at every volatility; a use
// past it takes the higher volatility of the two and passes only into low.
func TestJudgeCouplingMatrix(t *testing.T) {
	volatilities := []resolved.Volatility{resolved.VolatilityLow, resolved.VolatilityMedium, resolved.VolatilityHigh}
	pastBalance := map[resolved.Volatility]int{resolved.VolatilityLow: 8, resolved.VolatilityMedium: 5, resolved.VolatilityHigh: 2}
	higher := func(a, b resolved.Volatility) resolved.Volatility {
		if a == resolved.VolatilityHigh || b == resolved.VolatilityHigh {
			return resolved.VolatilityHigh
		}
		if a == resolved.VolatilityMedium || b == resolved.VolatilityMedium {
			return resolved.VolatilityMedium
		}
		return resolved.VolatilityLow
	}
	for _, source := range volatilities {
		for _, target := range volatilities {
			p := resolved.Policy{Modules: map[string]resolved.Module{
				"src": {Paths: []string{"src/**"}, Volatility: source, DependsOn: []string{"dst"}},
				"dst": {Paths: []string{"dst/**"}, Public: []string{"dst/api"}, Volatility: target},
			}}.WithDefaults()
			for _, tt := range []struct {
				name, target string
				want         int
			}{
				{"public", "dst/api", 9},
				{"past", "dst/store", pastBalance[higher(source, target)]},
			} {
				t.Run(fmt.Sprintf("%s %s to %s", tt.name, source, target), func(t *testing.T) {
					_, ok := byRule(change.Judge(p, fileUse("src/a.go", tt.target)), resolved.RuleCoupling)
					if failed := tt.want < resolved.DefaultMinBalance; ok != failed {
						t.Fatalf("coupling error = %v, want %v (balance %d)", ok, failed, tt.want)
					}
					// At a min of 10 every cell is an error, so its balance can be read.
					strict := p
					strict.Builtin.MinBalance = 10
					got, _ := byRule(change.Judge(strict, fileUse("src/a.go", tt.target)), resolved.RuleCoupling)
					if got.Balance == nil || got.Balance.Value != tt.want {
						t.Errorf("balance = %+v, want %d", got.Balance, tt.want)
					}
				})
			}
		}
	}
}

func TestJudgeCouplingText(t *testing.T) {
	got, ok := byRule(change.Judge(policy(), fileUse("cmd/main.go", "pkg/tc/netem")), resolved.RuleCoupling)
	want := change.Error{
		Key:  change.Key{Rule: resolved.RuleCoupling, From: cmd, To: tc, Symbol: "pkg/tc/netem"},
		From: cmd, To: tc,
		What: "cmd -> pkg_tc reaches past the public part of pkg_tc", Phrase: "reaches past the public part",
		Why:     "new knowledge across a module boundary needs a balance of at least 7",
		Balance: &result.Balance{Value: 2, Min: 7}, Next: fixStep(),
	}
	if !ok || !reflect.DeepEqual(got, want) {
		t.Errorf("got %+v, %v\nwant %+v", got, ok, want)
	}
	p := policy()
	p.Builtin.MinBalance = 10
	got, _ = byRule(change.Judge(p, fileUse("cmd/main.go", "pkg/tc")), resolved.RuleCoupling)
	if got.What != "cmd -> pkg_tc uses the public part of pkg_tc" || got.Phrase != "uses the public part" {
		t.Errorf("a public use below a min of 10: what = %q, phrase = %q", got.What, got.Phrase)
	}
}

func TestJudgeLayers(t *testing.T) {
	p := policy()
	u := p.Modules[util]
	u.DependsOn = []string{cmd}
	p.Modules[util] = u
	tests := []struct {
		name string
		use  change.Use
		want bool
	}{
		{"inner into outer", fileUse("pkg/util/util.go", "cmd"), true},
		{"outer into inner", fileUse("cmd/main.go", "pkg/util"), false},
		{"same layer", fileUse("pkg/chaos/chaos.go", "pkg/util"), false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, ok := byRule(change.Judge(p, tt.use), resolved.RuleLayers)
			if ok != tt.want {
				t.Fatalf("layers error = %v, want %v", ok, tt.want)
			}
			if ok && (got.What != "pkg_util (core) -> cmd (cmd) goes against the layer order" ||
				got.Phrase != "layer core -> layer cmd goes against the layer order") {
				t.Errorf("what = %q, phrase = %q", got.What, got.Phrase)
			}
		})
	}
	noLayer := policy()
	m := noLayer.Modules[util]
	m.Layer, m.DependsOn = "", []string{cmd}
	noLayer.Modules[util] = m
	if _, ok := byRule(change.Judge(noLayer, fileUse("pkg/util/util.go", "cmd")), resolved.RuleLayers); ok {
		t.Error("a module with no layer has no layer rule")
	}
}

func TestJudgeDeny(t *testing.T) {
	tests := []struct {
		name string
		use  change.Use
		want *change.Error
	}{
		{"by module, the 8.1 example", fileUse("pkg/chaos/netem/netem.go", runtimeDir), &change.Error{
			Key:  change.Key{Rule: denyRuntime, From: chaos, To: runtime, Symbol: runtimeDir},
			From: chaos, To: runtime,
			What: "pkg_chaos must not depend on pkg_runtime", Phrase: "pkg_chaos must not depend on pkg_runtime",
			Why: whyRuntime, Instead: insteadClient, Next: fixStep(),
		}},
		{"by path: the key holds modules, the text the rule's names", fileUse("cmd/main.go", "pkg/runtime/docker"), &change.Error{
			Key:  change.Key{Rule: denyCmd, From: cmd, To: runtime, Symbol: "pkg/runtime/docker"},
			From: cmd, To: runtime,
			What: "cmd/** must not depend on pkg/runtime/**", Phrase: "cmd/** must not depend on pkg/runtime/**",
			Why: "w", Instead: "i", Next: fixStep(),
		}},
		{"another target", fileUse("pkg/chaos/chaos.go", "pkg/util"), nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			errs := change.Judge(policy(), tt.use)
			rule := denyRuntime
			if tt.want != nil {
				rule = tt.want.Key.Rule
			}
			got, ok := byRule(errs, rule)
			if tt.want == nil {
				if ok {
					t.Errorf("got %+v, want no deny error", got)
				}
				return
			}
			if !ok || !reflect.DeepEqual(got, *tt.want) {
				t.Errorf("got %+v, %v\nwant %+v", got, ok, *tt.want)
			}
		})
	}
}

func TestJudgeOnly(t *testing.T) {
	tests := []struct {
		name, rule string
		use        change.Use
		wantWhat   string
	}{
		{"a library outside its modules", onlyDocker, libraryUse("pkg/chaos/chaos.go", dockerSDK+"/client", resolved.LanguageGo),
			"pkg_chaos uses github.com/docker/docker/client (github.com/docker/docker), which only pkg_runtime may use"},
		{"the library itself", onlyDocker, libraryUse("pkg/chaos/chaos.go", dockerSDK, resolved.LanguageGo),
			"pkg_chaos uses github.com/docker/docker, which only pkg_runtime may use"},
		{"a library in its module", onlyDocker, libraryUse("pkg/runtime/docker.go", dockerSDK, resolved.LanguageGo), ""},
		{"a package set outside its modules", onlyProcess, libraryUse("pkg/chaos/chaos.go", "os/exec", resolved.LanguageGo),
			"pkg_chaos uses os/exec (process), which only pkg_runtime and scripts/** may use"},
		{"a package set of another language", onlyProcess, libraryUse("pkg/chaos/chaos.py", "os/exec", resolved.LanguagePython), ""},
		{"a package set with no language", onlyProcess, libraryUse("pkg/chaos/chaos.x", "subprocess", ""),
			"pkg_chaos uses subprocess (process), which only pkg_runtime and scripts/** may use"},
		{"a package set in a path of in", onlyProcess, libraryUse("scripts/run.go", "os/exec", resolved.LanguageGo), ""},
		{"an importer in no module", onlyProcess, libraryUse("tools/gen.go", "os/exec", resolved.LanguageGo),
			"tools/gen.go uses os/exec (process), which only pkg_runtime and scripts/** may use"},
	}
	// The phrase `archfit can` prints states the rule; the importer is in its head.
	wantPhrase := map[string]string{
		onlyDocker:  "only pkg_runtime may use github.com/docker/docker",
		onlyProcess: "only pkg_runtime and scripts/** may use process",
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, ok := byRule(change.Judge(policy(), tt.use), tt.rule)
			if ok != (tt.wantWhat != "") {
				t.Fatalf("only error = %v (%+v), want %v", ok, got, tt.wantWhat != "")
			}
			// The key's From is the importer's module, empty for a file in no module.
			wantFrom, _ := policy().ModuleOf(tt.use.From.Path)
			if ok && (got.What != tt.wantWhat || got.Phrase != wantPhrase[tt.rule] || got.Next != fixStep() ||
				got.Key.To != policyUse(tt.rule) || got.Key.From != wantFrom || got.From != wantFrom) {
				t.Errorf("got what %q, phrase %q, next %+v, key %+v, from %q; want what %q, phrase %q, from %q",
					got.What, got.Phrase, got.Next, got.Key, got.From, tt.wantWhat, wantPhrase[tt.rule], wantFrom)
			}
		})
	}
}

func TestJudgeDenyByPathKeepsModulesApart(t *testing.T) {
	p := policy()
	p.Rules = []resolved.Rule{{ID: "no-pkg-to-runtime", Form: resolved.FormDeny, From: []string{"pkg/**"}, To: []string{runtime}, Why: "w", Instead: "i"}}
	a, okA := byRule(change.Judge(p, fileUse("pkg/chaos/chaos.go", runtimeDir)), "no-pkg-to-runtime")
	b, okB := byRule(change.Judge(p, fileUse("pkg/tc/tc.go", runtimeDir)), "no-pkg-to-runtime")
	if !okA || !okB || a.Key == b.Key || a.Key.From != chaos || b.Key.From != tc {
		t.Errorf("one path glob over two modules: keys %+v and %+v, want one per module", a.Key, b.Key)
	}
}

func policyUse(rule string) string {
	for _, r := range policy().Rules {
		if r.ID == rule {
			return r.Use
		}
	}
	return ""
}

func TestJudgeNoteGivesNoError(t *testing.T) {
	p := policy()
	p.Rules = []resolved.Rule{{ID: noteFanOut, Form: resolved.FormNote, Note: "n", Why: "w"}}
	if got := change.Judge(p, fileUse("pkg/chaos/chaos.go", "pkg/util")); len(got) != 0 {
		t.Errorf("a note: got %+v, want no error", got)
	}
}

func TestJudgeSameModuleAndNoModule(t *testing.T) {
	p := policy()
	for _, use := range []change.Use{
		fileUse("pkg/tc/tc.go", "pkg/tc/netem"),
		fileUse("tools/gen.go", "pkg/util"),
		fileUse("pkg/tc/tc.go", "vendor/x"),
	} {
		if got := change.Judge(p, use); len(got) != 0 {
			t.Errorf("Judge(%+v) = %+v, want no error", use, got)
		}
	}
}

func TestJudgeTextsUseOnlyDesignWords(t *testing.T) {
	p := policy()
	for _, m := range []string{util, tc} {
		mod := p.Modules[m]
		mod.DependsOn = []string{cmd}
		p.Modules[m] = mod
	}
	uses := []change.Use{
		fileUse("pkg/tc/tc.go", "pkg/util"), fileUse("cmd/main.go", "pkg/tc/netem"), fileUse("pkg/util/util.go", "cmd"),
		fileUse("pkg/chaos/chaos.go", runtimeDir), fileUse("cmd/main.go", "pkg/runtime/x"),
		libraryUse("pkg/chaos/chaos.go", dockerSDK, resolved.LanguageGo), libraryUse("pkg/chaos/chaos.go", "os/exec", resolved.LanguageGo),
	}
	p.Builtin.Graph = resolved.ModeAsk
	for _, use := range uses {
		for _, e := range change.Judge(p, use) {
			for _, text := range []string{e.What, e.Phrase, e.Why, e.Next.Text, e.Next.Note} {
				if words := testutil.NotUsedIn(text); len(words) > 0 {
					t.Errorf("%s: %q uses %q", e.Key.Rule, text, words)
				}
			}
		}
	}
}

// BalanceOf is the balance Judge puts on graph and coupling errors, and the
// balance `archfit can` shows for an allowed dependency.
func TestBalanceOf(t *testing.T) {
	tests := []struct {
		name string
		use  change.Use
		want *result.Balance
	}{
		{"a use of the public part of a low module", fileUse("pkg/chaos/chaos.go", "pkg/util"), &result.Balance{Value: 9, Min: 7}},
		{"a use past the public part of a high module", fileUse("cmd/main.go", "pkg/tc/netem"), &result.Balance{Value: 2, Min: 7}},
		{"a module named on its own means its public part",
			change.Use{From: change.End{Module: cmd}, To: change.End{Module: tc}}, &result.Balance{Value: 9, Min: 7}},
		{"one module", fileUse("pkg/tc/tc.go", "pkg/tc/netem"), nil},
		{"an importer in no module", fileUse("tools/gen.go", "pkg/util"), nil},
		{"a target in no module", fileUse("pkg/tc/tc.go", "vendor/x"), nil},
		{"a library", libraryUse("pkg/chaos/chaos.go", dockerSDK, resolved.LanguageGo), nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := change.BalanceOf(policy(), tt.use); !reflect.DeepEqual(got, tt.want) {
				t.Errorf("BalanceOf = %+v, want %+v", got, tt.want)
			}
		})
	}
	p := policy()
	p.Builtin.MinBalance = 8
	if got := change.BalanceOf(p, fileUse("pkg/chaos/chaos.go", "pkg/util")); got == nil || got.Min != 8 {
		t.Errorf("min = %+v, want the policy's min_balance 8", got)
	}
	graph, _ := byRule(change.Judge(policy(), fileUse("pkg/tc/netem.go", "pkg/util")), resolved.RuleGraph)
	if want := change.BalanceOf(policy(), fileUse("pkg/tc/netem.go", "pkg/util")); !reflect.DeepEqual(graph.Balance, want) {
		t.Errorf("the graph error carries %+v, BalanceOf gives %+v", graph.Balance, want)
	}
}
