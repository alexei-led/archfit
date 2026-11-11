package count_test

import (
	"crypto/sha256"
	"encoding/hex"
	"reflect"
	"slices"
	"strings"
	"testing"
	"time"

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

	chaosFile = "pkg/chaos/chaos.go"
	tcFile    = "pkg/tc/tc.go"
	netemFile = "pkg/tc/netem.go"
	utilFile  = "pkg/util/util.go"
	ifaceFile = "pkg/tc/iface.go"

	genFile     = "tools/gen.go"
	oldFile     = "tools/old.go"
	renamedFile = "tools/new.go"

	regex = "[a-zA-Z][a-zA-Z0-9.:_-]*"
)

var today = time.Date(2026, 11, 10, 0, 0, 0, 0, time.UTC)

var (
	graphKey    = change.Key{Rule: resolved.RuleGraph, From: tc, To: util, Symbol: "pkg/util"}
	copyKey     = change.Key{Rule: resolved.RuleDuplication, From: tc, To: util, Symbol: digestOf(regex)}
	expiredKey  = change.Key{Rule: count.RuleException, From: tc, To: util, Symbol: resolved.RuleGraph}
	sources     = []string{chaosFile, tcFile, netemFile, utilFile, "pkg/runtime/docker.go"}
	unreadBase  = evidence.UnknownItem{Specifier: "abc123", Reason: "the base abc123 cannot be read: no such commit", Cause: evidence.CauseBase, Fix: "run git fetch, then archfit check again"}
	missingTool = evidence.UnknownItem{Reason: "ast-grep is not installed", Cause: evidence.CauseTool, Fix: "install ast-grep"}
)

// policy: pkg_chaos may depend on pkg_util; pkg_tc and pkg_runtime on no
// module. Coupling and layers are off so each case shows one rule.
func policy(exceptions ...resolved.Exception) resolved.Policy {
	return resolved.Policy{
		Modules: map[string]resolved.Module{
			chaos:   {Paths: []string{"pkg/chaos/**"}, DependsOn: []string{util}},
			tc:      {Paths: []string{"pkg/tc/**"}},
			util:    {Paths: []string{"pkg/util/**"}},
			runtime: {Paths: []string{"pkg/runtime/**"}},
		},
		Builtin:    resolved.Builtin{Coupling: resolved.ModeOff, Layers: resolved.ModeOff},
		Exceptions: exceptions,
	}.WithDefaults()
}

func imp(file string, line int, target string) evidence.ImportFact {
	return evidence.ImportFact{File: file, Line: line, Target: target}
}

// lit is an occurrence of regex, the literal every copy case uses.
func lit(file string, line int) evidence.LiteralOccurrence {
	return evidence.LiteralOccurrence{File: file, Line: line, Value: regex, Shape: evidence.ShapeOther}
}

// tree builds the facts of a tree: the shared sources, extra files, and the
// files of the imports and literals.
type tree struct {
	files    []string
	imports  []evidence.ImportFact
	literals []evidence.LiteralOccurrence
	unknown  []evidence.UnknownItem
}

func (t tree) facts() evidence.CheckFacts {
	files := slices.Concat(sources, t.files)
	for _, i := range t.imports {
		files = append(files, i.File)
	}
	for _, l := range t.literals {
		files = append(files, l.File)
	}
	slices.Sort(files)
	return evidence.CheckFacts{
		Sources: slices.Compact(files), Imports: t.imports, Literals: t.literals, Unknown: t.unknown,
	}
}

func assess(p resolved.Policy, base, head tree, added evidence.AddedLines) count.Verdict {
	return count.Assess(count.Input{Policy: p, Base: base.facts(), Head: head.facts(), Added: added, Today: today})
}

func keys(errs []count.Error) []change.Key {
	out := make([]change.Key, 0, len(errs))
	for _, e := range errs {
		out = append(out, e.Key)
	}
	return out
}

func causes(rs []result.Unknown) []result.Cause {
	out := make([]result.Cause, 0, len(rs))
	for _, r := range rs {
		out = append(out, r.Cause)
	}
	return out
}

func day(offset int) time.Time { return today.AddDate(0, 0, offset) }

// Design 15.1, added or existing: a wrong "existing" is the most dangerous
// mistake, and a wrong "added" fails a change that adds nothing.
func TestAssessAddedOrExisting(t *testing.T) {
	drift := tree{imports: []evidence.ImportFact{imp(tcFile, 5, "pkg/util")}}
	cases := []struct {
		name       string
		base, head tree
		added      evidence.AddedLines
		want       []change.Key
	}{
		{name: "unchanged tree", base: drift, head: drift},
		{
			name: "existing drift the change does not touch",
			base: drift, head: tree{imports: []evidence.ImportFact{imp(tcFile, 5, "pkg/util"), imp(chaosFile, 3, "pkg/util")}},
			added: evidence.AddedLines{chaosFile: {3}},
		},
		{
			name: "a new dependency", head: drift, added: evidence.AddedLines{tcFile: {5}},
			want: []change.Key{graphKey},
		},
		{
			name: "one more use of a broken dependency",
			base: drift, head: tree{imports: []evidence.ImportFact{imp(tcFile, 5, "pkg/util"), imp(netemFile, 7, "pkg/util")}},
			added: evidence.AddedLines{netemFile: {7}},
			want:  []change.Key{graphKey},
		},
		{
			name: "code moved within its module",
			base: drift, head: tree{imports: []evidence.ImportFact{imp(netemFile, 7, "pkg/util")}},
			added: evidence.AddedLines{netemFile: {7}},
		},
		{
			name: "a renamed file", base: drift,
			head: tree{imports: []evidence.ImportFact{imp("pkg/tc/control.go", 5, "pkg/util")}},
		},
		{
			name: "code moved to another module",
			base: drift, head: tree{imports: []evidence.ImportFact{imp("pkg/runtime/tc.go", 5, "pkg/util")}},
			added: evidence.AddedLines{"pkg/runtime/tc.go": {5}},
			want:  []change.Key{{Rule: resolved.RuleGraph, From: runtime, To: util, Symbol: "pkg/util"}},
		},
		{
			name: "a new unowned file", head: tree{files: []string{genFile}},
			added: evidence.AddedLines{genFile: {1, 2}},
			want:  []change.Key{{Rule: resolved.RuleUnowned, Symbol: genFile}},
		},
		{
			name: "a pure rename of an unowned file",
			base: tree{files: []string{oldFile}}, head: tree{files: []string{renamedFile}},
		},
		{
			name: "a rename of an unowned file with an edit",
			base: tree{files: []string{oldFile}}, head: tree{files: []string{renamedFile}},
			added: evidence.AddedLines{renamedFile: {3}},
			want:  []change.Key{{Rule: resolved.RuleUnowned, Symbol: renamedFile}},
		},
		{
			name:  "a copy on an added line",
			base:  tree{literals: []evidence.LiteralOccurrence{lit(utilFile, 8)}},
			head:  tree{literals: []evidence.LiteralOccurrence{lit(utilFile, 8), lit(ifaceFile, 12)}},
			added: evidence.AddedLines{ifaceFile: {12}},
			want:  []change.Key{copyKey},
		},
		{
			name:  "a copy the base already holds",
			base:  tree{literals: []evidence.LiteralOccurrence{lit(utilFile, 8), lit(ifaceFile, 12)}},
			head:  tree{literals: []evidence.LiteralOccurrence{lit(utilFile, 8), lit(ifaceFile, 14)}},
			added: evidence.AddedLines{ifaceFile: {1}},
		},
		{
			// R12 counts holders per tree: deleting the third holder leaves a
			// pair the base did not have, without a new copy.
			name: "a copy count that rises without an added line",
			base: tree{literals: []evidence.LiteralOccurrence{lit(utilFile, 8), lit(ifaceFile, 12), lit(chaosFile, 4)}},
			head: tree{literals: []evidence.LiteralOccurrence{lit(utilFile, 8), lit(ifaceFile, 12)}},
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			v := assess(policy(), c.base, c.head, c.added)
			want, status := c.want, result.StatusFail
			if want == nil {
				want, status = []change.Key{}, result.StatusPass
			}
			if v.Status != status || !reflect.DeepEqual(keys(v.Errors), want) {
				t.Fatalf("status %s, keys %v; want %s, %v", v.Status, keys(v.Errors), status, want)
			}
			for _, e := range v.Errors {
				if !e.FromChange || e.Exception != nil {
					t.Errorf("error %v: FromChange %t, Exception %v; want an error of the change", e.Key, e.FromChange, e.Exception)
				}
			}
			if len(v.Unknown) != 0 {
				t.Errorf("unknown = %+v, want none", v.Unknown)
			}
		})
	}
}

func TestAssessShowsAddedSitesFirst(t *testing.T) {
	base := tree{imports: []evidence.ImportFact{imp(tcFile, 5, "pkg/util")}}
	head := tree{imports: []evidence.ImportFact{imp(tcFile, 5, "pkg/util"), imp(tcFile, 9, "pkg/util")}}
	v := assess(policy(), base, head, evidence.AddedLines{tcFile: {9}})
	want := []change.Site{{File: tcFile, Line: 9}, {File: tcFile, Line: 5}}
	if len(v.Errors) != 1 || !reflect.DeepEqual(v.Errors[0].Sites, want) {
		t.Fatalf("errors = %+v, want one error with sites %v", v.Errors, want)
	}
	if got := v.Errors[0].Next; got.Kind != result.NextCommand {
		t.Errorf("next of a graph error (ask) = %+v, want a command", got)
	}
}

// When a change closes a cycle, the record shows first the errors whose site
// is on an added line.
func TestAssessClosingACycle(t *testing.T) {
	base := tree{imports: []evidence.ImportFact{imp(tcFile, 5, "pkg/util")}}
	head := tree{imports: []evidence.ImportFact{imp(tcFile, 5, "pkg/util"), imp(utilFile, 4, "pkg/tc")}}
	v := assess(policy(), base, head, evidence.AddedLines{utilFile: {4}})
	want := []change.Key{
		{Rule: resolved.RuleCycle, From: util, To: tc},
		{Rule: resolved.RuleGraph, From: util, To: tc, Symbol: "pkg/tc"},
		{Rule: resolved.RuleCycle, From: tc, To: util},
	}
	if v.Status != result.StatusFail || !reflect.DeepEqual(keys(v.Errors), want) {
		t.Fatalf("status %s, keys\n%v\nwant fail,\n%v", v.Status, keys(v.Errors), want)
	}
}

func TestAssessCopyProposesADate(t *testing.T) {
	base := tree{literals: []evidence.LiteralOccurrence{lit(utilFile, 8)}}
	head := tree{literals: []evidence.LiteralOccurrence{lit(utilFile, 8), lit(ifaceFile, 12)}}
	v := assess(policy(), base, head, evidence.AddedLines{ifaceFile: {12}})
	if len(v.Errors) != 1 {
		t.Fatalf("errors = %v, want one copy", keys(v.Errors))
	}
	e := v.Errors[0]
	if e.Sites[0] != (change.Site{File: ifaceFile, Line: 12}) || !strings.HasSuffix(e.Next.Text, "--expires 2026-12-10") {
		t.Errorf("copy = %+v, want it shown at %s:12 with a request that expires 30 days after the date", e, ifaceFile)
	}
}

func TestAssessExceptions(t *testing.T) {
	newDependency := tree{imports: []evidence.ImportFact{imp(tcFile, 5, "pkg/util")}}
	dependencyAdded := evidence.AddedLines{tcFile: {5}}
	exc := func(rule, from, to string, expires time.Time) resolved.Exception {
		return resolved.Exception{Rule: rule, From: from, To: to, Expires: expires, Why: "planned migration"}
	}
	cases := []struct {
		name       string
		exception  resolved.Exception
		base, head tree
		added      evidence.AddedLines
		want       []change.Key
	}{
		{
			name: "matches the rule, from and to", exception: exc(resolved.RuleGraph, tc, util, time.Time{}),
			head: newDependency, added: dependencyAdded,
		},
		{
			name: "another to does not match", exception: exc(resolved.RuleGraph, tc, chaos, time.Time{}),
			head: newDependency, added: dependencyAdded, want: []change.Key{graphKey},
		},
		{
			name: "another rule does not match", exception: exc(resolved.RuleCycle, tc, util, time.Time{}),
			head: newDependency, added: dependencyAdded, want: []change.Key{graphKey},
		},
		{
			name: "duplication matches the pair in either order", exception: exc(resolved.RuleDuplication, util, tc, time.Time{}),
			base:  tree{literals: []evidence.LiteralOccurrence{lit(utilFile, 8)}},
			head:  tree{literals: []evidence.LiteralOccurrence{lit(utilFile, 8), lit(ifaceFile, 12)}},
			added: evidence.AddedLines{ifaceFile: {12}},
		},
		{
			name: "unowned matches a path glob", exception: exc(resolved.RuleUnowned, "tools/**", "", time.Time{}),
			head: tree{files: []string{genFile}}, added: evidence.AddedLines{genFile: {1}},
		},
		{
			name: "holds on its expires date", exception: exc(resolved.RuleGraph, tc, util, day(0)),
			head: newDependency, added: dependencyAdded,
		},
		{
			name: "fails the day after, and the change added to it", exception: exc(resolved.RuleGraph, tc, util, day(-1)),
			head: newDependency, added: dependencyAdded, want: []change.Key{graphKey, expiredKey},
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			v := assess(policy(c.exception), c.base, c.head, c.added)
			want, status := c.want, result.StatusFail
			if want == nil {
				want, status = []change.Key{}, result.StatusPass
			}
			if v.Status != status || !reflect.DeepEqual(keys(v.Errors), want) {
				t.Fatalf("status %s, keys %v; want %s, %v", v.Status, keys(v.Errors), status, want)
			}
			for _, e := range v.Errors {
				if !e.FromChange {
					t.Errorf("error %v is not from the change; the change added to it", e.Key)
				}
			}
		})
	}
}

// Design 6.4 and 7.2: after its date, while the code still breaks the rule,
// an exception fails every change, including one that touches nothing near it.
func TestAssessExpiredExceptionFailsAnUnrelatedChange(t *testing.T) {
	unrelated := evidence.AddedLines{chaosFile: {3}}
	cases := []struct {
		name  string
		why   string
		rule  string
		state tree
		next  result.Next
		what  string
	}{
		{
			name: "a broken dependency, with a request", rule: resolved.RuleGraph, why: "Request #431 is open.",
			state: tree{imports: []evidence.ImportFact{imp(tcFile, 5, "pkg/util")}},
			next:  result.Next{Kind: result.NextWait, Text: "wait for an owner: #431"},
			what:  "graph pkg_tc -> pkg_util expired on 2026-11-09",
		},
		{
			name: "an old copy, without a request", rule: resolved.RuleDuplication, why: "Kept until the parser moves.",
			state: tree{literals: []evidence.LiteralOccurrence{lit(utilFile, 8), lit(ifaceFile, 12)}},
			next:  result.Next{Kind: result.NextFix, Text: "fix the code, or ask an owner to extend the exception"},
			what:  "duplication pkg_tc -> pkg_util expired on 2026-11-09",
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			x := resolved.Exception{Rule: c.rule, From: tc, To: util, Expires: day(-1), Why: c.why}
			v := assess(policy(x), c.state, c.state, unrelated)
			if v.Status != result.StatusFail || len(v.Errors) != 1 {
				t.Fatalf("status %s, errors %v; want fail with the expired exception", v.Status, keys(v.Errors))
			}
			e := v.Errors[0]
			wantKey := change.Key{Rule: count.RuleException, From: tc, To: util, Symbol: c.rule}
			if e.Key != wantKey || e.FromChange || len(e.Sites) != 0 || e.Exception == nil || *e.Exception != x {
				t.Errorf("error = %+v, want key %v, not from the change, no sites, the exception", e, wantKey)
			}
			if e.What != c.what || e.Next != c.next || e.Why != "an exception with a date fails every change after that date" {
				t.Errorf("what %q, why %q, next %+v; want %q, next %+v", e.What, e.Why, e.Next, c.what, c.next)
			}
		})
	}
}

func TestAssessExpiredExceptionWithoutBrokenCode(t *testing.T) {
	x := resolved.Exception{Rule: resolved.RuleGraph, From: tc, To: util, Expires: day(-30), Why: "Request #431 is open."}
	v := assess(policy(x), tree{}, tree{}, nil)
	if v.Status != result.StatusPass || len(v.Errors) != 0 {
		t.Fatalf("status %s, errors %v; want pass: the code no longer breaks the rule", v.Status, keys(v.Errors))
	}
}

// The date boundary agrees with what the record prints for the exception.
func TestAssessExpiryAgreesWithFormatExpires(t *testing.T) {
	state := tree{imports: []evidence.ImportFact{imp(tcFile, 5, "pkg/util")}}
	for _, offset := range []int{-2, -1, 0, 1, 2} {
		expires := day(offset)
		x := resolved.Exception{Rule: resolved.RuleGraph, From: tc, To: util, Expires: expires}
		// A check run late in the day judges the same calendar day.
		in := count.Input{Policy: policy(x), Base: state.facts(), Head: state.facts(), Today: today.Add(23 * time.Hour)}
		expired := strings.HasSuffix(resolved.FormatExpires(expires, in.Today), "(expired)")
		if got := count.Assess(in).Status == result.StatusFail; got != expired {
			t.Errorf("expires %s: fails = %t, but FormatExpires says %q",
				expires.Format(time.DateOnly), got, resolved.FormatExpires(expires, in.Today))
		}
	}
}

// Design 7.2: when the comparison cannot be made, the status is unknown. The
// base unread stops the comparison and lists no error; other unknown items
// lose to an added error, which the worker must fix anyway.
func TestAssessUnknown(t *testing.T) {
	newDependency := []evidence.ImportFact{imp(tcFile, 5, "pkg/util")}
	unresolved := func(file string, line int) evidence.UnknownItem {
		return evidence.UnknownItem{
			File: file, Line: line, Specifier: "@/config", Reason: `"@/config" does not resolve`,
			Cause: evidence.CauseImport, Fix: "fix the import, or the tsconfig paths",
		}
	}
	cases := []struct {
		name       string
		base, head tree
		added      evidence.AddedLines
		status     result.Status
		errors     int
		causes     []result.Cause
	}{
		{
			name: "the base cannot be read", base: tree{unknown: []evidence.UnknownItem{unreadBase}},
			head: tree{imports: newDependency}, added: evidence.AddedLines{tcFile: {5}},
			status: result.StatusUnknown, causes: []result.Cause{result.CauseBase},
		},
		{
			name: "a tool is missing", base: tree{unknown: []evidence.UnknownItem{missingTool}},
			head: tree{unknown: []evidence.UnknownItem{missingTool}}, added: evidence.AddedLines{chaosFile: {1}},
			status: result.StatusUnknown, causes: []result.Cause{result.CauseTool},
		},
		{
			name: "an unresolved import in a changed file",
			head: tree{unknown: []evidence.UnknownItem{unresolved(netemFile, 9)}}, added: evidence.AddedLines{netemFile: {9}},
			status: result.StatusUnknown, causes: []result.Cause{result.CauseImport},
		},
		{
			name: "the base cannot read a changed file",
			base: tree{unknown: []evidence.UnknownItem{unresolved(netemFile, 9)}}, added: evidence.AddedLines{netemFile: {2}},
			status: result.StatusUnknown, causes: []result.Cause{result.CauseBase},
		},
		{
			name:   "a new unknown item in an unchanged file",
			head:   tree{unknown: []evidence.UnknownItem{unresolved(utilFile, 3)}},
			added:  evidence.AddedLines{"tsconfig.json": {4}},
			status: result.StatusUnknown, causes: []result.Cause{result.CauseImport},
		},
		{
			name:   "the same unknown item on the base and the head, in an unchanged file",
			base:   tree{unknown: []evidence.UnknownItem{unresolved(utilFile, 3)}},
			head:   tree{unknown: []evidence.UnknownItem{unresolved(utilFile, 3)}},
			added:  evidence.AddedLines{chaosFile: {1}},
			status: result.StatusPass, causes: []result.Cause{},
		},
		{
			name:   "an added error wins over an unknown item",
			head:   tree{imports: newDependency, unknown: []evidence.UnknownItem{unresolved(utilFile, 3)}},
			added:  evidence.AddedLines{tcFile: {5}},
			status: result.StatusFail, errors: 1, causes: []result.Cause{result.CauseImport},
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			v := assess(policy(), c.base, c.head, c.added)
			if v.Status != c.status || len(v.Errors) != c.errors || !reflect.DeepEqual(causes(v.Unknown), c.causes) {
				t.Fatalf("status %s, errors %v, causes %v; want %s, %d errors, %v",
					v.Status, keys(v.Errors), causes(v.Unknown), c.status, c.errors, c.causes)
			}
			for _, r := range v.Unknown {
				if r.What == "" || r.Why == "" || r.Next.Kind != result.NextRestore || r.Next.Text == "" {
					t.Errorf("reason %+v: want what, why and a restore step", r)
				}
			}
		})
	}
}

func TestAssessUnknownReasonNamesTheItem(t *testing.T) {
	v := assess(policy(), tree{unknown: []evidence.UnknownItem{unreadBase}}, tree{}, nil)
	want := result.Unknown{
		Cause: result.CauseBase, What: unreadBase.Reason,
		Why:  "the base cannot be read, so the change cannot be compared with it",
		Next: result.Next{Kind: result.NextRestore, Text: unreadBase.Fix},
	}
	if len(v.Unknown) != 1 || v.Unknown[0] != want {
		t.Errorf("unknown = %+v, want [%+v]", v.Unknown, want)
	}
}

func TestAssessExpiredExceptionAndBaseUnread(t *testing.T) {
	x := resolved.Exception{Rule: resolved.RuleGraph, From: tc, To: util, Expires: day(-1)}
	head := tree{imports: []evidence.ImportFact{imp(tcFile, 5, "pkg/util")}}
	v := assess(policy(x), tree{unknown: []evidence.UnknownItem{unreadBase}}, head, nil)
	if v.Status != result.StatusUnknown || len(v.Errors) != 0 {
		t.Errorf("status %s, errors %v; want unknown and no error when the base cannot be read", v.Status, keys(v.Errors))
	}
}

func digestOf(value string) string {
	sum := sha256.Sum256([]byte(value))
	return "sha256:" + hex.EncodeToString(sum[:])
}
