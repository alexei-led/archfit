package repostate_test

import (
	"context"
	"errors"
	"reflect"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/alexei-led/archfit/v3/internal/application/repostate"
	"github.com/alexei-led/archfit/v3/internal/assessment/change"
	"github.com/alexei-led/archfit/v3/internal/assessment/count"
	"github.com/alexei-led/archfit/v3/internal/model/evidence"
	"github.com/alexei-led/archfit/v3/internal/model/result"
	"github.com/alexei-led/archfit/v3/internal/policy/resolved"
)

const (
	chaos   = "pkg_chaos"
	tc      = "pkg_tc"
	util    = "pkg_util"
	runtime = "pkg_runtime"

	chaosFile  = "pkg/chaos/chaos.go"
	tcFile     = "pkg/tc/tc.go"
	utilFile   = "pkg/util/util.go"
	dockerFile = "pkg/runtime/docker.go"

	regex = "[a-zA-Z][a-zA-Z0-9.:_-]*"

	tcGlob   = "pkg/tc/**"
	utilGlob = "pkg/util/**"
)

var today = time.Date(2026, 11, 10, 0, 0, 0, 0, time.UTC)

func day(offset int) time.Time { return today.AddDate(0, 0, offset) }

// policy: pkg_chaos may depend on pkg_util; pkg_tc and pkg_runtime on no
// module. Coupling and layers are off so each case shows one rule.
func policy(exceptions ...resolved.Exception) resolved.Policy {
	return resolved.Policy{
		Modules: map[string]resolved.Module{
			chaos:   {Paths: []string{"pkg/chaos/**"}, DependsOn: []string{util}},
			tc:      {Paths: []string{tcGlob}},
			util:    {Paths: []string{utilGlob}},
			runtime: {Paths: []string{"pkg/runtime/**"}},
		},
		Builtin:    resolved.Builtin{Coupling: resolved.ModeOff, Layers: resolved.ModeOff},
		Exceptions: exceptions,
	}.WithDefaults()
}

func imp(file string, line int, target string) evidence.ImportFact {
	return evidence.ImportFact{File: file, Line: line, Target: target}
}

func lit(file string, line int) evidence.LiteralOccurrence {
	return evidence.LiteralOccurrence{File: file, Line: line, Value: regex, Shape: evidence.ShapeOther}
}

// facts builds a head tree: the four modules' files, extra files, and the
// files of the imports and literals.
func facts(imports []evidence.ImportFact, literals []evidence.LiteralOccurrence, extra ...string) evidence.CheckFacts {
	files := slices.Concat([]string{chaosFile, tcFile, utilFile, dockerFile}, extra)
	for _, i := range imports {
		files = append(files, i.File)
	}
	for _, l := range literals {
		files = append(files, l.File)
	}
	slices.Sort(files)
	return evidence.CheckFacts{Sources: slices.Compact(files), Imports: imports, Literals: literals}
}

func evaluate(p resolved.Policy, head evidence.CheckFacts, on time.Time) repostate.Repository {
	return repostate.Evaluate(repostate.Input{Policy: p, Head: head, Today: on})
}

func keys(errs []count.Error) []change.Key {
	out := make([]change.Key, 0, len(errs))
	for _, e := range errs {
		out = append(out, e.Key)
	}
	return out
}

func graphX(expires time.Time) resolved.Exception {
	return resolved.Exception{Rule: resolved.RuleGraph, From: tc, To: util, Expires: expires, Why: "Request #431 is open."}
}

var (
	graphKey   = change.Key{Rule: resolved.RuleGraph, From: tc, To: util, Symbol: "pkg/util"}
	runtimeKey = change.Key{Rule: resolved.RuleGraph, From: tc, To: runtime, Symbol: "pkg/runtime"}
	expiredKey = change.Key{Rule: count.RuleException, From: tc, To: util, Symbol: resolved.RuleGraph}
	tcToUtil   = []evidence.ImportFact{imp(tcFile, 5, "pkg/util")}
)

func TestEvaluateState(t *testing.T) {
	both := []evidence.ImportFact{imp(tcFile, 5, "pkg/util"), imp(tcFile, 6, "pkg/runtime")}
	cases := []struct {
		name           string
		exceptions     []resolved.Exception
		imports        []evidence.ImportFact
		want           repostate.State
		drift, expired []change.Key
	}{
		{name: "no error", imports: []evidence.ImportFact{imp(chaosFile, 3, "pkg/util")}, want: repostate.StateClean},
		{name: "an error no exception matches", imports: tcToUtil, want: repostate.StateDrift, drift: []change.Key{graphKey}},
		{name: "an error an exception covers", exceptions: []resolved.Exception{graphX(day(3))}, imports: tcToUtil, want: repostate.StateClean},
		{name: "an exception with no date", exceptions: []resolved.Exception{graphX(time.Time{})}, imports: tcToUtil, want: repostate.StateClean},
		{
			name: "an exception past its date, the code still breaks the rule", exceptions: []resolved.Exception{graphX(day(-1))},
			imports: tcToUtil, want: repostate.StateExpired, expired: []change.Key{expiredKey},
		},
		{name: "an exception past its date, the code fixed", exceptions: []resolved.Exception{graphX(day(-1))}, want: repostate.StateClean},
		{
			name: "an expired exception wins over drift", exceptions: []resolved.Exception{graphX(day(-1))},
			imports: both, want: repostate.StateExpired, drift: []change.Key{runtimeKey}, expired: []change.Key{expiredKey},
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			r := evaluate(policy(c.exceptions...), facts(c.imports, nil), today)
			if r.State != c.want {
				t.Errorf("state = %s, want %s", r.State, c.want)
			}
			if got, want := keys(r.Drift), orEmpty(c.drift); !reflect.DeepEqual(got, want) {
				t.Errorf("drift = %v, want %v", got, want)
			}
			if got, want := keys(r.Expired), orEmpty(c.expired); !reflect.DeepEqual(got, want) {
				t.Errorf("expired = %v, want %v", got, want)
			}
			for _, e := range slices.Concat(r.Drift, r.Expired) {
				if e.FromChange {
					t.Errorf("%v: FromChange true in the repository state", e.Key)
				}
			}
		})
	}
}

func orEmpty(ks []change.Key) []change.Key {
	if ks == nil {
		return []change.Key{}
	}
	return ks
}

// An exception holds through its date: the day itself has 0 days left and
// the day after it has expired.
func TestEvaluateExpiryWindow(t *testing.T) {
	expires := today
	cases := []struct {
		name    string
		on      time.Time
		listed  string
		expired bool
	}{
		{name: "15 days before", on: expires.AddDate(0, 0, -15)},
		{name: "14 days before", on: expires.AddDate(0, 0, -14), listed: "2026-11-10 (14 days)"},
		{name: "the day before", on: expires.AddDate(0, 0, -1), listed: "2026-11-10 (1 day)"},
		{name: "the day itself", on: expires, listed: "2026-11-10 (0 days)"},
		{name: "the day itself, late in the day", on: expires.Add(23 * time.Hour), listed: "2026-11-10 (0 days)"},
		{name: "the day after", on: expires.AddDate(0, 0, 1), expired: true},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			x := graphX(expires)
			r := evaluate(policy(x), facts(tcToUtil, nil), c.on)
			var want []repostate.Expiring
			if c.listed != "" {
				want = []repostate.Expiring{{Exception: x, Expires: c.listed}}
				if c.listed != resolved.FormatExpires(x.Expires, c.on) {
					t.Fatalf("test date %q disagrees with FormatExpires %q", c.listed, resolved.FormatExpires(x.Expires, c.on))
				}
			}
			if !reflect.DeepEqual(r.Expiring, orEmptyExpiring(want)) {
				t.Errorf("expiring = %+v, want %+v", r.Expiring, want)
			}
			if got := r.State == repostate.StateExpired; got != c.expired {
				t.Errorf("state = %s, want expired: %t", r.State, c.expired)
			}
		})
	}
}

func orEmptyExpiring(xs []repostate.Expiring) []repostate.Expiring {
	if xs == nil {
		return []repostate.Expiring{}
	}
	return xs
}

func TestEvaluateExpiringNeedsAnError(t *testing.T) {
	cases := []struct {
		name string
		x    resolved.Exception
	}{
		{name: "an exception in the window that covers nothing", x: graphX(day(3))},
		{name: "an exception with no date", x: graphX(time.Time{})},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			imports := tcToUtil
			if !c.x.Expires.IsZero() {
				imports = nil
			}
			if r := evaluate(policy(c.x), facts(imports, nil), today); len(r.Expiring) != 0 {
				t.Errorf("expiring = %+v, want none", r.Expiring)
			}
		})
	}
}

// A use of the public part is contract strength: 9 at every volatility
// (design 6.2), as `archfit can` shows it. A repeated depends_on entry is one
// allowed dependency.
func TestEvaluateBalances(t *testing.T) {
	cases := []struct {
		name       string
		volatility resolved.Volatility
		minBalance int
		want       result.Balance
	}{
		{name: "a low target", volatility: resolved.VolatilityLow, want: result.Balance{Value: 9, Min: 7}},
		{name: "a high target", volatility: resolved.VolatilityHigh, want: result.Balance{Value: 9, Min: 7}},
		{name: "the policy's min_balance", volatility: resolved.VolatilityLow, minBalance: 8, want: result.Balance{Value: 9, Min: 8}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			p := resolved.Policy{
				Modules: map[string]resolved.Module{
					tc:    {Paths: []string{tcGlob}, DependsOn: []string{util}},
					chaos: {Paths: []string{"pkg/chaos/**"}, DependsOn: []string{util, tc, util}},
					util:  {Paths: []string{utilGlob}, Volatility: c.volatility},
				},
				Builtin: resolved.Builtin{MinBalance: c.minBalance},
			}.WithDefaults()
			r := evaluate(p, facts(nil, nil), today)
			want := []repostate.Balance{
				{From: chaos, To: tc, Balance: result.Balance{Value: 9, Min: c.want.Min}},
				{From: chaos, To: util, Balance: c.want},
				{From: tc, To: util, Balance: c.want},
			}
			if !reflect.DeepEqual(r.Balances, want) {
				t.Errorf("balances = %+v, want %+v", r.Balances, want)
			}
		})
	}
	t.Run("no allowed dependency", func(t *testing.T) {
		p := resolved.Policy{Modules: map[string]resolved.Module{util: {Paths: []string{utilGlob}}}}.WithDefaults()
		if r := evaluate(p, facts(nil, nil), today); len(r.Balances) != 0 {
			t.Errorf("balances = %+v, want none", r.Balances)
		}
	})
}

var dockerOnly = resolved.Rule{
	ID: "docker-in-runtime", Form: resolved.FormOnly, Use: "github.com/docker/docker",
	In: []string{runtime}, Why: "One adapter owns the Docker API.",
}

func TestEvaluateExisting(t *testing.T) {
	docker := func(file string) evidence.ImportFact {
		return evidence.ImportFact{File: file, Line: 4, Target: "github.com/docker/docker", External: true}
	}
	cases := []struct {
		name     string
		imports  []evidence.ImportFact
		literals []evidence.LiteralOccurrence
		extra    []string
		want     []resolved.Exception
		// unnamed: the tree has an error no exception can name.
		unnamed bool
	}{
		{name: "no error"},
		{
			name:    "one exception for every site of a rule and place",
			imports: []evidence.ImportFact{imp(tcFile, 5, "pkg/util"), imp("pkg/tc/netem.go", 7, "pkg/util"), imp(tcFile, 6, "pkg/runtime")},
			want: []resolved.Exception{
				{Rule: resolved.RuleGraph, From: tc, To: runtime},
				{Rule: resolved.RuleGraph, From: tc, To: util},
			},
		},
		{
			name:     "a copy names the sorted pair",
			literals: []evidence.LiteralOccurrence{lit(utilFile, 15), lit(tcFile, 9)},
			want:     []resolved.Exception{{Rule: resolved.RuleDuplication, From: tc, To: util}},
		},
		{
			name:  "unowned code names the file as a glob",
			extra: []string{"tools/gen[1].go"},
			want:  []resolved.Exception{{Rule: resolved.RuleUnowned, From: `tools/gen\[1\].go`}},
		},
		{
			name:    "an only rule names no to",
			imports: []evidence.ImportFact{docker(tcFile)},
			want:    []resolved.Exception{{Rule: dockerOnly.ID, From: tc}},
		},
		{
			name:    "an importer in no module is named by its unowned file only",
			imports: []evidence.ImportFact{docker("tools/run.go")},
			want:    []resolved.Exception{{Rule: resolved.RuleUnowned, From: "tools/run.go"}},
			unnamed: true,
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			p := policy()
			p.Rules = []resolved.Rule{dockerOnly}
			head := facts(c.imports, c.literals, c.extra...)
			r := evaluate(p, head, today)
			if got := slices.ContainsFunc(r.Drift, func(e count.Error) bool { return e.Key.From == "" && e.Key.Rule != resolved.RuleUnowned }); got != c.unnamed {
				t.Errorf("an error no exception can name: %t, want %t", got, c.unnamed)
			}
			if !reflect.DeepEqual(r.Existing, orEmptyExceptions(c.want)) {
				t.Fatalf("existing = %+v, want %+v", r.Existing, c.want)
			}
			for _, x := range r.Existing {
				if x.From == "" {
					t.Errorf("%+v: an exception with no from", x)
				}
			}
			// The exceptions init writes cover every error they were written
			// for: what is left is only what no exception can name.
			p.Exceptions = r.Existing
			after := evaluate(p, head, today)
			for _, e := range after.Drift {
				if e.Key.From != "" || e.Key.Rule == resolved.RuleUnowned {
					t.Errorf("%v: still drift with the existing exceptions", e.Key)
				}
			}
		})
	}
}

func orEmptyExceptions(xs []resolved.Exception) []resolved.Exception {
	if xs == nil {
		return []resolved.Exception{}
	}
	return xs
}

func TestEvaluateNotesAndUnknown(t *testing.T) {
	note := resolved.Rule{ID: "fan-out-through-runner", Form: resolved.FormNote, Note: "Chaos actions fan out through RunOnContainers.", Why: "One place checks the target limit."}
	p := policy()
	p.Rules = []resolved.Rule{dockerOnly, note}
	head := facts(nil, nil)
	missing := evidence.UnknownItem{Reason: "ast-grep is not installed", Cause: evidence.CauseTool, Fix: "install ast-grep"}
	head.Unknown = []evidence.UnknownItem{missing}
	r := evaluate(p, head, today)
	if !reflect.DeepEqual(r.NotChecked, []resolved.Rule{note}) {
		t.Errorf("not checked = %+v, want the note", r.NotChecked)
	}
	if !reflect.DeepEqual(r.Unknown, head.Unknown) {
		t.Errorf("unknown = %+v, want %+v", r.Unknown, head.Unknown)
	}
	if r.State != repostate.StateClean {
		t.Errorf("state = %s, want clean: unknown items do not change the state", r.State)
	}
}

type policySource struct {
	p   resolved.Policy
	err error
}

func (s policySource) Policy(context.Context) (resolved.Policy, error) { return s.p, s.err }

type headFacts struct {
	facts evidence.CheckFacts
	err   error
}

func (h headFacts) Head(context.Context, string) (evidence.CheckFacts, error) { return h.facts, h.err }

// tree is the working tree: its files, or an error for each method.
type tree struct {
	files        []string
	idErr, lsErr error
}

const treeID = "4b825dc642cb6eb9a060e54bf8d69288fbee4904"

func (w tree) WorkingTree(context.Context) (string, error) { return treeID, w.idErr }

func (w tree) Files(_ context.Context, id string) ([]string, error) {
	if id != treeID {
		return nil, errors.New("unknown tree " + id)
	}
	return w.files, w.lsErr
}

func TestServiceState(t *testing.T) {
	errPort := errors.New("port failed")
	unfit := policy()
	unfit.Modules["pkg_gone"] = resolved.Module{Paths: []string{"pkg/gone/**"}}
	// The Go and Python extractors leave test files out of Sources; the
	// policy's tests glob matches files of the tree all the same.
	withTests := policy()
	withTests.Tests = []string{"**/*_test.go"}
	head := facts(tcToUtil, nil)
	files := slices.Concat(head.Sources, []string{"go.mod", "pkg/tc/tc_test.go"})
	cases := []struct {
		name    string
		source  policySource
		tree    tree
		head    headFacts
		wantErr string
		want    repostate.State
	}{
		{name: "a state", source: policySource{p: policy()}, tree: tree{files: files}, head: headFacts{facts: head}, want: repostate.StateDrift},
		{
			name: "a tests glob that matches only files the extractors leave out", source: policySource{p: withTests},
			tree: tree{files: files}, head: headFacts{facts: head}, want: repostate.StateDrift,
		},
		{name: "the policy cannot be read", source: policySource{err: errPort}, wantErr: errPort.Error()},
		{name: "the tree id cannot be read", source: policySource{p: policy()}, tree: tree{idErr: errPort}, wantErr: errPort.Error()},
		{name: "the tree cannot be listed", source: policySource{p: policy()}, tree: tree{lsErr: errPort}, wantErr: errPort.Error()},
		{name: "the head cannot be read", source: policySource{p: policy()}, tree: tree{files: files}, head: headFacts{err: errPort}, wantErr: errPort.Error()},
		{name: "a policy that does not fit the tree", source: policySource{p: unfit}, tree: tree{files: files}, head: headFacts{facts: head}, wantErr: "pkg/gone/**"},
		{
			name: "a tests glob that matches no file of the tree", source: policySource{p: withTests},
			tree: tree{files: head.Sources}, head: headFacts{facts: head}, wantErr: "**/*_test.go",
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			s := repostate.Service{Policies: c.source, Tree: c.tree, Facts: c.head}
			r, err := s.State(context.Background(), ".", today)
			switch {
			case c.wantErr != "":
				if err == nil || !strings.Contains(err.Error(), c.wantErr) {
					t.Fatalf("err = %v, want one naming %q", err, c.wantErr)
				}
			case err != nil:
				t.Fatalf("err = %v", err)
			case r.State != c.want:
				t.Errorf("state = %s, want %s", r.State, c.want)
			}
		})
	}
}
