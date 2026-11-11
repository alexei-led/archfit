package copies_test

import (
	"crypto/sha256"
	"encoding/hex"
	"reflect"
	"slices"
	"testing"
	"time"

	"github.com/alexei-led/archfit/v3/internal/assessment/change"
	"github.com/alexei-led/archfit/v3/internal/assessment/copies"
	"github.com/alexei-led/archfit/v3/internal/model/evidence"
	"github.com/alexei-led/archfit/v3/internal/model/result"
	"github.com/alexei-led/archfit/v3/internal/policy/resolved"
	"github.com/alexei-led/archfit/v3/internal/testutil"
)

// The pumba code the trap agents copied (design appendix D.4):
// pkg/util/util.go holds a regular expression at line 8 and an error text at
// line 12.
const (
	regex       = "[a-zA-Z][a-zA-Z0-9.:_-]*"
	regexDigest = "sha256:bdb110186c8a1667e309464631c5f85b46ca8a5d706bb5db988973b4b48ff107"
	errorText   = "bad network interface name: must match '%s'"
	errorDigest = "sha256:8942c53b984b51a421d3a2e04e77de185bb0fc7c5d5953c0b13b726958663172"
	runID       = "^[A-Za-z0-9_.-]+$"
	mediaType   = "application/json"
)

const (
	modTC     = "pkg_tc"
	modUtil   = "pkg_util"
	ifaceFile = "pkg/tc/iface.go"
	mainFile  = "cmd/main.go"
)

var expires = time.Date(2026, 12, 10, 0, 0, 0, 0, time.UTC)

func policy(mode resolved.Mode) resolved.Policy {
	return resolved.Policy{
		Modules: map[string]resolved.Module{
			"cmd":       {Paths: []string{"cmd/**"}},
			"pkg_chaos": {Paths: []string{"pkg/chaos/**"}},
			modTC:       {Paths: []string{"pkg/tc/**"}},
			modUtil:     {Paths: []string{"pkg/util/**"}},
		},
		Builtin:   resolved.Builtin{Duplication: mode},
		Tests:     []string{"**/*_test.go"},
		Generated: []string{"**/zz_generated*.go"},
	}.WithDefaults()
}

func lit(file string, line int, value string) evidence.LiteralOccurrence {
	return evidence.LiteralOccurrence{File: file, Line: line, Value: value, Shape: evidence.ShapeOther}
}

func shaped(shape evidence.LiteralShape, l evidence.LiteralOccurrence) evidence.LiteralOccurrence {
	l.Shape = shape
	return l
}

// pumba is the base tree: the interface check in pkg_util, the run-id rule in
// pkg_chaos, and a media type that cmd and pkg_chaos both hold, an existing
// copy.
var pumba = []evidence.LiteralOccurrence{
	lit(mainFile, 3, mediaType),
	lit("pkg/chaos/http.go", 3, mediaType),
	lit("pkg/chaos/runid.go", 5, runID),
	lit("pkg/util/util.go", 8, regex),
	lit("pkg/util/util.go", 12, errorText),
}

func with(tree []evidence.LiteralOccurrence, more ...evidence.LiteralOccurrence) []evidence.LiteralOccurrence {
	return slices.Concat(tree, more)
}

func without(tree []evidence.LiteralOccurrence, file string) []evidence.LiteralOccurrence {
	return slices.DeleteFunc(slices.Clone(tree), func(l evidence.LiteralOccurrence) bool { return l.File == file })
}

// upTo returns lines 1 to n.
func upTo(n int) []int {
	out := make([]int, 0, n)
	for line := 1; line <= n; line++ {
		out = append(out, line)
	}
	return out
}

func digest(value string) string {
	sum := sha256.Sum256([]byte(value))
	return "sha256:" + hex.EncodeToString(sum[:])
}

// added runs the rule on the base and on the head and returns the copies the
// change adds.
func added(p resolved.Policy, base, head []evidence.LiteralOccurrence, addedLines evidence.AddedLines) []change.Error {
	return copies.Added(copies.Find(base, p, nil, expires), copies.Find(head, p, addedLines, expires), addedLines)
}

// seeded is the seeded set: identical copies the rule must find, and
// rewritten copies and non-copies it must not. want lists the values the
// change adds a copy of.
var seeded = []struct {
	name  string
	base  []evidence.LiteralOccurrence
	head  []evidence.LiteralOccurrence
	added evidence.AddedLines
	want  []string
}{
	{
		name:  "seeded pumba copy: the regular expression and the error text",
		base:  pumba,
		head:  with(pumba, lit(ifaceFile, 8, regex), lit(ifaceFile, 12, errorText)),
		added: evidence.AddedLines{ifaceFile: upTo(16)},
		want:  []string{regex, errorText},
	},
	{
		name:  "one more site of an existing copy",
		base:  pumba,
		head:  with(pumba, lit("pkg/chaos/client.go", 7, mediaType)),
		added: evidence.AddedLines{"pkg/chaos/client.go": {7}},
		want:  []string{mediaType},
	},
	{
		name:  "a value of 12 runes in another script",
		base:  with(pumba, lit("pkg/util/hello.go", 4, "привет, мир!")),
		head:  with(pumba, lit("pkg/util/hello.go", 4, "привет, мир!"), lit("pkg/tc/hello.go", 4, "привет, мир!")),
		added: evidence.AddedLines{"pkg/tc/hello.go": upTo(5)},
		want:  []string{"привет, мир!"},
	},
	{
		name:  "a rewritten copy: the run-id rule with a length bound",
		base:  pumba,
		head:  with(pumba, lit("pkg/tc/recovery.go", 5, "^[A-Za-z0-9_.-]{1,48}$")),
		added: evidence.AddedLines{"pkg/tc/recovery.go": upTo(5)},
	},
	{
		name:  "a moved literal",
		base:  pumba,
		head:  with(without(pumba, "pkg/chaos/runid.go"), lit("pkg/tc/runid.go", 5, runID)),
		added: evidence.AddedLines{"pkg/tc/runid.go": upTo(5)},
	},
	{
		name:  "a copy into a declared test file",
		base:  pumba,
		head:  with(pumba, lit("pkg/tc/iface_test.go", 3, errorText)),
		added: evidence.AddedLines{"pkg/tc/iface_test.go": upTo(3)},
	},
	{
		name:  "a copy into a declared generated file",
		base:  pumba,
		head:  with(pumba, lit("pkg/tc/zz_generated.go", 3, errorText)),
		added: evidence.AddedLines{"pkg/tc/zz_generated.go": upTo(3)},
	},
	{
		name:  "a copy into code of no module",
		base:  pumba,
		head:  with(pumba, lit("scripts/check.go", 3, errorText)),
		added: evidence.AddedLines{"scripts/check.go": upTo(3)},
	},
	{
		name:  "a value of 11 runes",
		base:  with(pumba, lit("pkg/util/hello.go", 4, "привет, мир")),
		head:  with(pumba, lit("pkg/util/hello.go", 4, "привет, мир"), lit("pkg/tc/hello.go", 4, "привет, мир")),
		added: evidence.AddedLines{"pkg/tc/hello.go": upTo(5)},
	},
	{
		name:  "a single word",
		base:  with(pumba, shaped(evidence.ShapeWord, lit("pkg/util/names.go", 3, "containerRuntimeName"))),
		head:  with(pumba, shaped(evidence.ShapeWord, lit("pkg/util/names.go", 3, "containerRuntimeName")), shaped(evidence.ShapeWord, lit("pkg/tc/names.go", 3, "containerRuntimeName"))),
		added: evidence.AddedLines{"pkg/tc/names.go": upTo(3)},
	},
	{
		name:  "a struct tag",
		base:  with(pumba, shaped(evidence.ShapeTag, lit("pkg/util/types.go", 6, `json:"container_name,omitempty"`))),
		head:  with(pumba, shaped(evidence.ShapeTag, lit("pkg/util/types.go", 6, `json:"container_name,omitempty"`)), shaped(evidence.ShapeTag, lit("pkg/tc/types.go", 6, `json:"container_name,omitempty"`))),
		added: evidence.AddedLines{"pkg/tc/types.go": upTo(8)},
	},
	{
		name:  "a value held by three modules",
		base:  pumba,
		head:  with(pumba, lit(ifaceFile, 8, regex), lit("pkg/chaos/iface.go", 8, regex)),
		added: evidence.AddedLines{ifaceFile: upTo(8), "pkg/chaos/iface.go": upTo(8)},
	},
	{
		name:  "a copy inside one module",
		base:  pumba,
		head:  with(pumba, lit("pkg/util/more.go", 3, errorText)),
		added: evidence.AddedLines{"pkg/util/more.go": upTo(3)},
	},
	{
		name:  "an existing copy, and a change elsewhere",
		base:  pumba,
		head:  pumba,
		added: evidence.AddedLines{"pkg/tc/tc.go": {1}},
	},
	{
		name:  "an existing copy moved within its file",
		base:  pumba,
		head:  with(without(pumba, mainFile), lit(mainFile, 5, mediaType)),
		added: evidence.AddedLines{mainFile: {5}},
	},
	{
		name:  "deleting the third holder of a literal",
		base:  with(pumba, lit(ifaceFile, 8, regex), lit("pkg/chaos/iface.go", 8, regex)),
		head:  with(pumba, lit(ifaceFile, 8, regex)),
		added: evidence.AddedLines{},
	},
}

func TestAddedOnTheSeededSet(t *testing.T) {
	p := policy(resolved.ModeAsk)
	var copiesSeeded, truePositives, falsePositives int
	for _, tt := range seeded {
		t.Run(tt.name, func(t *testing.T) {
			var got []string
			for _, e := range added(p, tt.base, tt.head, tt.added) {
				got = append(got, e.Key.Symbol)
			}
			var want []string
			for _, v := range tt.want {
				want = append(want, digest(v))
			}
			slices.Sort(got)
			slices.Sort(want)
			copiesSeeded += len(want)
			for _, s := range got {
				if slices.Contains(want, s) {
					truePositives++
				} else {
					falsePositives++
				}
			}
			if !slices.Equal(got, want) {
				t.Errorf("added copies = %v, want %v", got, want)
			}
		})
	}
	precision, recall := 1.0, 1.0
	if flagged := truePositives + falsePositives; flagged > 0 {
		precision = float64(truePositives) / float64(flagged)
	}
	if copiesSeeded > 0 {
		recall = float64(truePositives) / float64(copiesSeeded)
	}
	t.Logf("seeded set: %d cases, %d copies; precision %.2f, recall %.2f", len(seeded), copiesSeeded, precision, recall)
	if precision != 1 || recall != 1 {
		t.Errorf("precision %.2f, recall %.2f on the seeded set, want 1 and 1", precision, recall)
	}
}

func TestSeededPumbaCopyRecord(t *testing.T) {
	p := policy(resolved.ModeAsk)
	head := with(pumba, lit(ifaceFile, 8, regex), lit(ifaceFile, 12, errorText))
	got := added(p, pumba, head, evidence.AddedLines{ifaceFile: upTo(16)})
	next := result.Next{
		Kind: result.NextCommand,
		Text: `use pkg_util, or run: archfit request -m "<why a copy is needed>" --expires 2026-12-10`,
	}
	why := "two copies must change together and nothing checks that they do"
	want := []change.Error{
		{
			Key:   change.Key{Rule: resolved.RuleDuplication, From: modTC, To: modUtil, Symbol: regexDigest},
			From:  modTC,
			To:    modUtil,
			Sites: []change.Site{{File: ifaceFile, Line: 8}, {File: "pkg/util/util.go", Line: 8}},
			What:  "this literal already lives in pkg/util/util.go:8",
			Why:   why,
			Next:  next,
		},
		{
			Key:   change.Key{Rule: resolved.RuleDuplication, From: modTC, To: modUtil, Symbol: errorDigest},
			From:  modTC,
			To:    modUtil,
			Sites: []change.Site{{File: ifaceFile, Line: 12}, {File: "pkg/util/util.go", Line: 12}},
			What:  "this literal already lives in pkg/util/util.go:12",
			Why:   why,
			Next:  next,
		},
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("added copies =\n%+v\nwant\n%+v", got, want)
	}
}

func TestDeletingTheThirdHolderMakesAKeyButAddsNoCopy(t *testing.T) {
	p := policy(resolved.ModeAsk)
	base := with(pumba, lit(ifaceFile, 8, regex), lit("pkg/chaos/iface.go", 8, regex))
	head := with(pumba, lit(ifaceFile, 8, regex))
	wantKey := change.Key{Rule: resolved.RuleDuplication, From: modTC, To: modUtil, Symbol: regexDigest}
	keys := func(errs []change.Error) []change.Key {
		out := make([]change.Key, 0, len(errs))
		for _, e := range errs {
			out = append(out, e.Key)
		}
		return out
	}
	if slices.Contains(keys(copies.Find(base, p, nil, expires)), wantKey) {
		t.Errorf("base: three holders, yet the key %v is there", wantKey)
	}
	headErrs := copies.Find(head, p, evidence.AddedLines{}, expires)
	if !slices.Contains(keys(headErrs), wantKey) {
		t.Errorf("head: two holders, want the key %v in %v", wantKey, keys(headErrs))
	}
	if got := copies.Added(copies.Find(base, p, nil, expires), headErrs, evidence.AddedLines{}); len(got) != 0 {
		t.Errorf("Added = %+v, want nothing: no copy is on an added line", got)
	}
}

// TestRecordDirection: the record starts in the module whose site the change
// adds, even when that module is second in the key; the first added site
// comes first and the others follow in file and line order.
func TestRecordDirection(t *testing.T) {
	const value = "container is not running"
	p := policy(resolved.ModeAsk)
	base := []evidence.LiteralOccurrence{lit("pkg/tc/a.go", 3, value), lit("pkg/tc/b.go", 9, value)}
	head := with(base, lit("pkg/util/x.go", 4, value), lit("pkg/util/a.go", 7, value))
	addedLines := evidence.AddedLines{"pkg/util/a.go": {7}, "pkg/util/x.go": {4}}
	got := added(p, base, head, addedLines)
	if len(got) != 1 {
		t.Fatalf("added copies = %+v, want one", got)
	}
	e := got[0]
	wantKey := change.Key{Rule: resolved.RuleDuplication, From: modTC, To: modUtil, Symbol: digest(value)}
	wantSites := []change.Site{{File: "pkg/util/a.go", Line: 7}, {File: "pkg/tc/a.go", Line: 3}, {File: "pkg/tc/b.go", Line: 9}, {File: "pkg/util/x.go", Line: 4}}
	if e.Key != wantKey || e.From != modUtil || e.To != modTC {
		t.Errorf("key %+v, from %q, to %q; want key %+v, from pkg_util, to pkg_tc", e.Key, e.From, e.To, wantKey)
	}
	if !slices.Equal(e.Sites, wantSites) {
		t.Errorf("sites = %v, want %v", e.Sites, wantSites)
	}
	if want := "this literal already lives in pkg/tc/a.go:3"; e.What != want {
		t.Errorf("what = %q, want %q", e.What, want)
	}
	if want := `use pkg_tc, or run: archfit request -m "<why a copy is needed>" --expires 2026-12-10`; e.Next.Text != want {
		t.Errorf("next = %q, want %q", e.Next.Text, want)
	}
}

// TestFindWithoutAddedLines: on the base no site is added, so the record
// starts in the first module in order.
func TestFindWithoutAddedLines(t *testing.T) {
	got := copies.Find(pumba, policy(resolved.ModeAsk), nil, expires)
	if len(got) != 1 {
		t.Fatalf("Find = %+v, want the media type copy alone", got)
	}
	e := got[0]
	if e.From != "cmd" || e.To != "pkg_chaos" || e.Sites[0] != (change.Site{File: mainFile, Line: 3}) {
		t.Errorf("from %q, to %q, first site %v; want cmd, pkg_chaos, cmd/main.go:3", e.From, e.To, e.Sites[0])
	}
	if want := "this literal already lives in pkg/chaos/http.go:3"; e.What != want {
		t.Errorf("what = %q, want %q", e.What, want)
	}
}

func TestFindReturnsRecordOrder(t *testing.T) {
	head := with(pumba, lit(ifaceFile, 12, errorText), lit(ifaceFile, 8, regex))
	got := copies.Find(head, policy(resolved.ModeAsk), evidence.AddedLines{ifaceFile: upTo(16)}, expires)
	order := make([]change.Site, 0, len(got))
	for _, e := range got {
		order = append(order, e.Sites[0])
	}
	want := []change.Site{{File: mainFile, Line: 3}, {File: ifaceFile, Line: 8}, {File: ifaceFile, Line: 12}}
	if !slices.Equal(order, want) {
		t.Errorf("first sites = %v, want %v", order, want)
	}
}

func TestNextByMode(t *testing.T) {
	head := with(pumba, lit(ifaceFile, 8, regex))
	addedLines := evidence.AddedLines{ifaceFile: {8}}
	tests := []struct {
		mode resolved.Mode
		want []result.Next
	}{
		{resolved.ModeAsk, []result.Next{{
			Kind: result.NextCommand,
			Text: `use pkg_util, or run: archfit request -m "<why a copy is needed>" --expires 2026-12-10`,
		}}},
		{resolved.ModeDeny, []result.Next{{Kind: result.NextFix, Text: "fix the code"}}},
		{resolved.ModeOff, nil},
	}
	for _, tt := range tests {
		t.Run(string(tt.mode), func(t *testing.T) {
			var got []result.Next
			for _, e := range added(policy(tt.mode), pumba, head, addedLines) {
				got = append(got, e.Next)
			}
			if !slices.Equal(got, tt.want) {
				t.Errorf("next = %+v, want %+v", got, tt.want)
			}
		})
	}
	if got := copies.Find(pumba, policy(resolved.ModeOff), nil, expires); got != nil {
		t.Errorf("Find with duplication off = %+v, want nil", got)
	}
}

func TestRecordUsesTheWordsOfTheDesign(t *testing.T) {
	head := with(pumba, lit(ifaceFile, 8, regex))
	for _, mode := range []resolved.Mode{resolved.ModeAsk, resolved.ModeDeny} {
		for _, e := range copies.Find(head, policy(mode), evidence.AddedLines{ifaceFile: {8}}, expires) {
			for _, text := range []string{e.What, e.Why, e.Next.Text} {
				if words := testutil.NotUsedIn(text); len(words) > 0 {
					t.Errorf("%s: %q uses %v", mode, text, words)
				}
			}
		}
	}
}
