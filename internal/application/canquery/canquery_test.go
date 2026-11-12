package canquery_test

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path"
	"path/filepath"
	"reflect"
	"slices"
	"strings"
	"testing"

	"github.com/alexei-led/archfit/v3/internal/application/canquery"
	"github.com/alexei-led/archfit/v3/internal/assessment/change"
	"github.com/alexei-led/archfit/v3/internal/model/evidence"
	"github.com/alexei-led/archfit/v3/internal/model/result"
	"github.com/alexei-led/archfit/v3/internal/output/record"
	"github.com/alexei-led/archfit/v3/internal/policy/resolved"
	"github.com/alexei-led/archfit/v3/internal/testutil"
)

const (
	chaos   = "pkg_chaos"
	tc      = "pkg_tc"
	util    = "pkg_util"
	runtime = "pkg_runtime"
	cmd     = "cmd"
	core    = "core"

	goModule   = "github.com/alexei-led/pumba"
	dockerSDK  = "github.com/docker/docker"
	denyChaos  = "no-chaos-to-runtime"
	denyCmd    = "no-cmd-to-runtime"
	onlyDocker = "docker-sdk-in-runtime"
	onlyExec   = "process-in-runtime"

	chaosFile   = "pkg/chaos/chaos.go"
	cmdFile     = "cmd/main.go"
	toolFile    = "tools/gen.go"
	netemDir    = "pkg/tc/netem"
	runtimeDir  = "pkg/runtime"
	requestText = `archfit request -m "<why you need it>"`
	fixText     = "fix the code"
	bal9        = "  balance: 9/10 (min 7)\n"
	nextFix     = "  next: fix the code\n"

	chaosDir       = "pkg/chaos"
	chaosNetemFile = "pkg/chaos/netem/netem.go"
	tcDir          = "pkg/tc"
	utilDir        = "pkg/util"
	utilFile       = "pkg/util/util.go"
	osExec         = "os/exec"
	web            = "web"
	cmdRoot        = "cmd/root.go"
	vueFile        = "web/view.vue"
	tcTest         = "pkg/tc/tc_test.go"
	databaseSQL    = "database/sql"
	cobra          = "github.com/spf13/cobra"
	onlySQL        = "sql-in-runtime"
	subprocess     = "subprocess"
	runtimeGlob    = "pkg/runtime/**"
)

// sources is the tree of the tests: pumba's shape.
var sources = []string{
	cmdFile, cmdRoot,
	chaosFile, chaosNetemFile,
	"pkg/runtime/docker.go",
	"pkg/tc/netem.go", "pkg/tc/tc.go", "pkg/tc/tc_test.go", "pkg/tc/netem/netem.go",
	utilFile,
	"web/app.ts", "web/state.py", vueFile,
	toolFile,
	"go/tools.go", ".storybook/main.ts",
}

// policy is the typical policy of design 6.1 with a cmd module, as check
// reads it.
func policy() resolved.Policy {
	return resolved.Policy{
		Digest: "sha256:7b02700a93eb8af7192724bd1b5ff49fd340f0ef81cfee1fb0d8cdc1467fea8c",
		Layers: []string{core, cmd},
		Modules: map[string]resolved.Module{
			chaos: {
				Paths: []string{"pkg/chaos/**"}, Public: []string{chaosDir, "pkg/chaos/cliflags"},
				Volatility: resolved.VolatilityHigh, Layer: core, DependsOn: []string{util},
			},
			tc:      {Paths: []string{"pkg/tc/**"}, Public: []string{tcDir}, Volatility: resolved.VolatilityHigh, Layer: core},
			util:    {Paths: []string{"pkg/util/**"}, Volatility: resolved.VolatilityLow, Layer: core},
			runtime: {Paths: []string{runtimeGlob}, Volatility: resolved.VolatilityMedium, Layer: core},
			cmd:     {Paths: []string{"cmd/**"}, Layer: cmd, DependsOn: []string{chaos, tc, util, runtime}},
			web:     {Paths: []string{"web/**"}},
		},
		Rules: []resolved.Rule{
			{
				ID: denyChaos, Form: resolved.FormDeny, From: []string{chaos}, To: []string{runtime},
				Why: "A direct import ties a chaos action to one runtime.", Instead: "Add a method to container.Client.",
			},
			{ID: denyCmd, Form: resolved.FormDeny, From: []string{"cmd/**"}, To: []string{runtimeGlob}, Why: "w", Instead: "i"},
			{ID: onlyDocker, Form: resolved.FormOnly, Use: dockerSDK, In: []string{runtime}, Why: "One adapter owns the Docker API."},
			{ID: onlyExec, Form: resolved.FormOnly, Use: resolved.SetProcess, In: []string{runtime}, Why: "Side effects stay in adapters."},
			{ID: onlySQL, Form: resolved.FormOnly, Use: databaseSQL, In: []string{runtimeGlob}, Why: "One adapter owns the store."},
			{ID: "cobra-in-root", Form: resolved.FormOnly, Use: cobra, In: []string{cmdRoot}, Why: "One file builds the commands."},
		},
		Tests: []string{"**/*_test.go"},
	}.WithDefaults()
}

// withLayerInversion lets pkg_util depend on cmd, so only the layers rule
// stands in the way.
func withLayerInversion() resolved.Policy {
	p := policy()
	m := p.Modules[util]
	m.DependsOn = []string{cmd}
	p.Modules[util] = m
	return p
}

type policySource struct {
	p   resolved.Policy
	err error
}

func (s policySource) Policy(context.Context) (resolved.Policy, error) { return s.p, s.err }

// tree is a checkout made of source files.
type tree struct {
	files []string
	err   error
}

func (t tree) Entry(_ context.Context, p string) (canquery.Entry, error) {
	if t.err != nil {
		return canquery.Entry{}, t.err
	}
	var e canquery.Entry
	for _, f := range t.files {
		switch {
		case f == p:
			return canquery.Entry{Kind: canquery.KindFile}, nil
		case path.Dir(f) == p:
			e.Kind, e.Files = canquery.KindDir, append(e.Files, f)
		case p == "." || strings.HasPrefix(f, p+"/"):
			e.Kind = canquery.KindDir
		}
	}
	return e, nil
}

// goModules maps the import paths of one Go module at the root.
type goModules struct{ err error }

func (g goModules) Dir(_ context.Context, importPath string) (string, bool, error) {
	if importPath == goModule {
		return ".", true, g.err
	}
	dir, ok := strings.CutPrefix(importPath, goModule+"/")
	return dir, ok, g.err
}

func service(p resolved.Policy) canquery.Service {
	return canquery.Service{Policies: policySource{p: p}, Tree: tree{files: sources}, Go: goModules{}}
}

// ask answers as for a protected policy, the setting of the records of design
// 8.1: an unprotected answer adds "policy: not protected" (design 9.4).
func ask(t *testing.T, p resolved.Policy, from, to string) result.CanAnswer {
	t.Helper()
	a, err := service(p).Can(context.Background(), canquery.Question{From: from, To: to, Stamp: canquery.Stamp{Protected: true}})
	if err != nil {
		t.Fatalf("Can(%s, %s): %v", from, to, err)
	}
	return a
}

func render(t *testing.T, a result.CanAnswer) string {
	t.Helper()
	var b bytes.Buffer
	if err := record.Answer(&b, a, record.Options{}); err != nil {
		t.Fatalf("record.Answer: %v", err)
	}
	return b.String()
}

// The cases of plan task 17. Each prints the record of design 8, and check
// judges the same import the same way (TestAgreesWithCheck).
type tcase struct {
	name     string
	policy   func() resolved.Policy
	from, to string
	want     string
	// imp is the import check judges for the same dependency; nil when the
	// answer is about no import.
	imp *evidence.ImportFact
}

func cases() []tcase {
	return []tcase{
		{"the 8.1 example", policy, tcDir, utilDir, "", &evidence.ImportFact{File: "pkg/tc/netem.go", Target: utilDir}},
		{"an allowed dependency with its balance", policy, chaosDir, utilDir,
			"pkg_chaos -> pkg_util: yes\n" + bal9, &evidence.ImportFact{File: chaosFile, Target: utilDir}},
		{"past the public part of a high module", policy, cmdFile, netemDir,
			"cmd -> pkg_tc: coupling: reaches past the public part\n  balance: 2/10 (min 7)\n" + nextFix,
			&evidence.ImportFact{File: cmdFile, Target: netemDir}},
		{"a deny rule with instead", policy, chaosNetemFile, runtimeDir,
			"pkg_chaos -> pkg_runtime: no-chaos-to-runtime: pkg_chaos must not depend on pkg_runtime\n" +
				"  why: A direct import ties a chaos action to one runtime.\n  instead: Add a method to container.Client.\n" + nextFix,
			&evidence.ImportFact{File: chaosNetemFile, Target: runtimeDir}},
		{"a library outside its only modules", policy, chaosFile, dockerSDK,
			"pkg_chaos -> github.com/docker/docker: docker-sdk-in-runtime: only pkg_runtime may use github.com/docker/docker\n" +
				"  why: One adapter owns the Docker API.\n" + nextFix,
			&evidence.ImportFact{File: chaosFile, Target: dockerSDK, External: true}},
		{"a layer inversion", withLayerInversion, utilFile, "cmd/main.go",
			"pkg_util -> cmd: layers: layer core -> layer cmd goes against the layer order\n" + nextFix,
			&evidence.ImportFact{File: utilFile, Target: cmdFile}},
		{"a declared test importer is out of every rule", policy, tcTest, utilDir,
			"pkg_tc -> pkg_util: yes\n", &evidence.ImportFact{File: tcTest, Target: utilDir}},
		{"a file of no known language reads the sets of every language", policy, vueFile, subprocess,
			"web -> subprocess: process-in-runtime: only pkg_runtime may use process\n" +
				"  why: Side effects stay in adapters.\n" + nextFix,
			&evidence.ImportFact{File: vueFile, Target: subprocess, External: true}},
		{"a path in no module", policy, toolFile, utilDir,
			"tools/gen.go -> pkg_util: unknown: tools/gen.go is in no module\n  next: archfit rules tools/gen.go\n", nil},
	}
}

func TestCases(t *testing.T) {
	golden, err := os.ReadFile(filepath.Join("..", "..", "output", "record", "testdata", "can.txt"))
	if err != nil {
		t.Fatal(err)
	}
	for _, tt := range cases() {
		t.Run(tt.name, func(t *testing.T) {
			want := tt.want
			if want == "" {
				want = string(golden)
			}
			if got := render(t, ask(t, tt.policy(), tt.from, tt.to)); got != want {
				t.Errorf("record:\n--- got\n%s--- want\n%s", got, want)
			}
		})
	}
}

// The 8.1 example is the contract's golden document, byte for byte.
func TestDesignExampleIsTheContractGolden(t *testing.T) {
	want, err := os.ReadFile(filepath.Join("..", "..", "model", "result", "testdata", "can.json"))
	if err != nil {
		t.Fatal(err)
	}
	stamp := canquery.Stamp{
		PolicyPath: ".archfit.yaml", Protected: true, Head: "1a954628a960aaef81d7b2d4521929579f3541e6",
		Date: "2026-11-10", Engine: "4.0.0",
	}
	a, err := service(policy()).Can(context.Background(), canquery.Question{From: tcDir, To: utilDir, Stamp: stamp})
	if err != nil {
		t.Fatal(err)
	}
	var got bytes.Buffer
	if err := record.AnswerJSON(&got, a); err != nil {
		t.Fatal(err)
	}
	if got.String() != string(want) {
		t.Errorf("JSON:\n--- got\n%s--- want\n%s", got.String(), want)
	}
}

// TestAgreesWithCheck judges each case's import as check does, with
// change.Evaluate on the same tree, and compares: a yes has no error of the
// dependency rules; a no is one of check's errors, with the same rule,
// phrase, instead, balance and next step; a path in no module is unowned.
func TestAgreesWithCheck(t *testing.T) {
	for _, tt := range cases() {
		t.Run(tt.name, func(t *testing.T) {
			p := tt.policy()
			a := ask(t, p, tt.from, tt.to)
			if tt.imp == nil {
				facts := evidence.CheckFacts{Sources: sources, Imports: []evidence.ImportFact{{File: tt.from, Line: 1, Target: tt.to}}}
				if !hasRule(change.Evaluate(p, facts).Errors, resolved.RuleUnowned, tt.from) || a.Answer != result.ReplyUnknown {
					t.Errorf("a path in no module: can %s, want check to give %s unowned", a.Answer, tt.from)
				}
				return
			}
			imp := *tt.imp
			imp.Line = 1
			errs := dependencyErrors(change.Evaluate(p, evidence.CheckFacts{Sources: sources, Imports: []evidence.ImportFact{imp}}).Errors)
			switch a.Answer {
			case result.ReplyYes:
				if len(errs) > 0 {
					t.Errorf("can says yes, check gives %+v", errs)
				}
			case result.ReplyNo:
				if !slices.ContainsFunc(errs, func(e change.Error) bool { return agrees(a, e) }) {
					t.Errorf("can says %+v, check gives %+v", a, errs)
				}
			default:
				t.Errorf("answer %s, want yes or no", a.Answer)
			}
		})
	}
}

func agrees(a result.CanAnswer, e change.Error) bool {
	why := ""
	if e.Key.Rule == a.Rule && !slices.Contains(resolved.BuiltinRules, e.Key.Rule) {
		why = e.Why
	}
	return e.Key.Rule == a.Rule && e.Phrase == a.What && e.Instead == a.Instead && why == a.Why &&
		reflect.DeepEqual(e.Balance, a.Balance) && a.Next != nil && e.Next.Kind == a.Next.Kind && e.Next.Text == a.Next.Text
}

// dependencyErrors drops the rules Judge does not apply to one dependency.
func dependencyErrors(errs []change.Error) []change.Error {
	var out []change.Error
	for _, e := range errs {
		switch e.Key.Rule {
		case resolved.RuleUnowned, resolved.RuleCycle, resolved.RuleDuplication:
		default:
			out = append(out, e)
		}
	}
	return out
}

func hasRule(errs []change.Error, rule, file string) bool {
	return slices.ContainsFunc(errs, func(e change.Error) bool { return e.Key.Rule == rule && e.Key.Symbol == file })
}

// TestArguments reads each kind of argument. The rows name what the answer
// says: its answer, rule and the labels of its two ends.
func TestArguments(t *testing.T) {
	tests := []struct {
		name     string
		from, to string
		want     result.CanAnswer
	}{
		{"module names; a module as the target means its public part", tc, util,
			result.CanAnswer{From: tc, To: util, Answer: result.ReplyNo, Rule: resolved.RuleGraph}},
		{"Go import paths through go.mod", goModule + "/pkg/tc", goModule + "/pkg/util",
			result.CanAnswer{From: tc, To: util, Answer: result.ReplyNo, Rule: resolved.RuleGraph}},
		{"a path written with ./ and a trailing slash", "./pkg/tc/", "pkg/util/",
			result.CanAnswer{From: tc, To: util, Answer: result.ReplyNo, Rule: resolved.RuleGraph}},
		{"a file not written yet takes its module from its path", "pkg/tc/new.go", utilDir,
			result.CanAnswer{From: tc, To: util, Answer: result.ReplyNo, Rule: resolved.RuleGraph}},
		{"a library with no only rule", chaosFile, "requests",
			result.CanAnswer{From: chaos, To: "requests", Answer: result.ReplyYes}},
		{"a mistyped path of the tree is not a library", tcDir, "pkg/utl",
			result.CanAnswer{From: tc, To: "pkg/utl", Answer: result.ReplyUnknown}},
		{"a target in no module", tcDir, toolFile,
			result.CanAnswer{From: tc, To: toolFile, Answer: result.ReplyUnknown}},
		{"one module", "pkg/tc/tc.go", netemDir, result.CanAnswer{From: tc, To: tc, Answer: result.ReplyYes}},
		{"a package set from a Go file", chaosFile, osExec,
			result.CanAnswer{From: chaos, To: osExec, Answer: result.ReplyNo, Rule: onlyExec}},
		{"a package set from a directory of Go files", chaosDir, osExec,
			result.CanAnswer{From: chaos, To: osExec, Answer: result.ReplyNo, Rule: onlyExec}},
		{"a package set from a module: its language is unknown", chaos, osExec,
			result.CanAnswer{From: chaos, To: osExec, Answer: result.ReplyUnknown}},
		{"a package set from a directory of two languages", web, subprocess,
			result.CanAnswer{From: web, To: subprocess, Answer: result.ReplyUnknown}},
		{"the library of a rule from a module needs no language", chaos, dockerSDK,
			result.CanAnswer{From: chaos, To: dockerSDK, Answer: result.ReplyNo, Rule: onlyDocker}},
		{"a path below a library from a module depends on the language", chaos, dockerSDK + "/client",
			result.CanAnswer{From: chaos, To: dockerSDK + "/client", Answer: result.ReplyUnknown}},
		{"a path-glob deny over an importer module name", cmd, runtime,
			result.CanAnswer{From: cmd, To: runtime, Answer: result.ReplyUnknown}},
		{"a path-glob deny over a target module name", cmdFile, runtime,
			result.CanAnswer{From: cmd, To: runtime, Answer: result.ReplyUnknown}},
		{"a path-glob deny over a file and a directory", cmdFile, runtimeDir,
			result.CanAnswer{From: cmd, To: runtime, Answer: result.ReplyNo, Rule: denyCmd}},
		{"a path-glob in that may cover a module name", runtime, databaseSQL,
			result.CanAnswer{From: runtime, To: databaseSQL, Answer: result.ReplyUnknown}},
		{"a path-glob in that cannot cover a module name", chaos, databaseSQL,
			result.CanAnswer{From: chaos, To: databaseSQL, Answer: result.ReplyNo, Rule: onlySQL}},
		{"a file inside a path-glob in", "pkg/runtime/docker.go", databaseSQL,
			result.CanAnswer{From: runtime, To: databaseSQL, Answer: result.ReplyYes}},
		{"a declared test file not written yet", "pkg/tc/new_test.go", utilDir,
			result.CanAnswer{From: tc, To: util, Answer: result.ReplyYes}},
		{"the files of a directory answer differently", "./cmd", cobra,
			result.CanAnswer{From: cmd, To: cobra, Answer: result.ReplyUnknown}},
		{"a dotted name whose first part is a root directory", tcDir, "web.state",
			result.CanAnswer{From: tc, To: "web.state", Answer: result.ReplyUnknown}},
		{"a target written ./path", tcDir, "./missing.go",
			result.CanAnswer{From: tc, To: "./missing.go", Answer: result.ReplyUnknown}},
		{"a Go library whose first label names a root directory", chaosFile, "go.uber.org/zap",
			result.CanAnswer{From: chaos, To: "go.uber.org/zap", Answer: result.ReplyYes}},
		{"a file not written yet under a dot directory", tcDir, ".storybook/new.ts",
			result.CanAnswer{From: tc, To: ".storybook/new.ts", Answer: result.ReplyUnknown}},
		{"a rule to fix the code wins over one to ask", chaosDir, netemDir,
			result.CanAnswer{From: chaos, To: tc, Answer: result.ReplyNo, Rule: resolved.RuleCoupling}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			a := ask(t, policy(), tt.from, tt.to)
			got := result.CanAnswer{From: a.From, To: a.To, Answer: a.Answer, Rule: a.Rule}
			if got != tt.want {
				t.Errorf("got %+v\nwant %+v\n%s", got, tt.want, render(t, a))
			}
		})
	}
}

// unknownQuestions are the questions an argument cannot answer, each with
// the record that names the file to ask about instead.
var unknownQuestions = []struct{ from, to, want string }{
	{chaos, osExec, "pkg_chaos -> os/exec: unknown: rule process-in-runtime reads os/exec by the language of the importer, " +
		"and pkg_chaos has none\n  next: archfit can <a file of pkg_chaos> os/exec\n"},
	{cmd, runtime, "cmd -> pkg_runtime: unknown: rule no-cmd-to-runtime names paths, and cmd is a module\n" +
		"  next: archfit can <a file of cmd> pkg_runtime\n"},
	{cmdFile, runtime, "cmd -> pkg_runtime: unknown: rule no-cmd-to-runtime names paths, and pkg_runtime is a module\n" +
		"  next: archfit can cmd/main.go <a file of pkg_runtime>\n"},
	{runtime, databaseSQL, "pkg_runtime -> database/sql: unknown: rule sql-in-runtime names paths, and pkg_runtime is a module\n" +
		"  next: archfit can <a file of pkg_runtime> database/sql\n"},
	{"./cmd", cobra, "cmd -> github.com/spf13/cobra: unknown: the files of ./cmd answer differently\n" +
		"  next: archfit can <a file of ./cmd> github.com/spf13/cobra\n"},
}

func TestUnknownNamesTheStep(t *testing.T) {
	for _, q := range unknownQuestions {
		if got := render(t, ask(t, policy(), q.from, q.to)); got != q.want {
			t.Errorf("record:\n--- got\n%s--- want\n%s", got, q.want)
		}
	}
}

// An ask rule's next step names the request, which can then use the answer.
func TestNoKeepsTheStepOfItsRule(t *testing.T) {
	graph := ask(t, policy(), tc, util)
	if graph.Next == nil || *graph.Next != (result.Next{Kind: result.NextCommand, Text: requestText}) {
		t.Errorf("graph next = %+v, want the request without a note", graph.Next)
	}
	coupling := ask(t, policy(), cmdFile, netemDir)
	if coupling.Next == nil || coupling.Next.Kind != result.NextFix || coupling.Next.Text != fixText || coupling.Why != "" {
		t.Errorf("coupling: next %+v, why %q; want fix the code and no why", coupling.Next, coupling.Why)
	}
	yes := ask(t, policy(), chaosDir, utilDir)
	if yes.Next != nil || yes.What != "" || yes.Rule != "" {
		t.Errorf("yes: %+v, want no rule, what or next", yes)
	}
}

func TestPortErrorsComeBack(t *testing.T) {
	errPort := errors.New("port")
	tests := []struct {
		name string
		s    canquery.Service
	}{
		{"policy", canquery.Service{Policies: policySource{err: errPort}, Tree: tree{files: sources}, Go: goModules{}}},
		{"tree", canquery.Service{Policies: policySource{p: policy()}, Tree: tree{err: errPort}, Go: goModules{}}},
		{"go.mod", canquery.Service{Policies: policySource{p: policy()}, Tree: tree{files: sources}, Go: goModules{err: errPort}}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			a, err := tt.s.Can(context.Background(), canquery.Question{From: goModule + "/pkg/tc", To: utilDir})
			if !errors.Is(err, errPort) || !reflect.DeepEqual(a, result.CanAnswer{}) {
				t.Errorf("got %+v, %v; want no answer and the port's error", a, err)
			}
		})
	}
}

func TestBothArgumentsAreRequired(t *testing.T) {
	for _, q := range []canquery.Question{{From: tcDir}, {To: utilDir}} {
		if _, err := service(policy()).Can(context.Background(), q); !errors.Is(err, canquery.ErrUsage) {
			t.Errorf("Can(%+v) = %v, want ErrUsage", q, err)
		}
	}
}

func TestAnswersUseOnlyDesignWords(t *testing.T) {
	for _, tt := range cases() {
		if words := testutil.NotUsedIn(render(t, ask(t, tt.policy(), tt.from, tt.to))); len(words) > 0 {
			t.Errorf("%s uses %q", tt.name, words)
		}
	}
	for _, q := range unknownQuestions {
		if words := testutil.NotUsedIn(q.want); len(words) > 0 {
			t.Errorf("can %s %s uses %q", q.from, q.to, words)
		}
	}
}
