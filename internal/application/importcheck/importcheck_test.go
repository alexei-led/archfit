package importcheck_test

import (
	"context"
	"errors"
	"reflect"
	"testing"
	"time"

	"github.com/alexei-led/archfit/v3/internal/application/canquery"
	"github.com/alexei-led/archfit/v3/internal/application/importcheck"
	"github.com/alexei-led/archfit/v3/internal/model/evidence"
	"github.com/alexei-led/archfit/v3/internal/model/result"
	"github.com/alexei-led/archfit/v3/internal/policy/resolved"
)

const (
	toUtil    = "./pkg/util"
	toNowhere = "./pkg/nowhere"
	forbidden = "forbidden"
	stdlibFmt = "fmt"
	tc        = "pkg_tc"
	util      = "pkg_util"
	tcFile    = "pkg/tc/netem.go"
	base      = "abc123"
)

var today = time.Date(2026, 11, 13, 0, 0, 0, 0, time.UTC)

type policies struct {
	p  resolved.Policy
	ok bool
}

func (f policies) Local(context.Context) (resolved.Policy, bool, error) { return f.p, f.ok, nil }

type facts evidence.CheckFacts

func (f facts) Head(context.Context, resolved.Policy) (evidence.CheckFacts, error) {
	return evidence.CheckFacts(f), nil
}

type repo struct {
	added evidence.AddedLines
	asked *string
}

func (r repo) AddedLines(_ context.Context, b string) (evidence.AddedLines, error) {
	*r.asked = b
	return r.added, nil
}

// can answers no for pkg/util and the library forbidden, yes for the rest,
// and records the targets it was asked about.
type can struct{ asked *[]string }

func (c can) Can(_ context.Context, q canquery.Question) (result.CanAnswer, error) {
	*c.asked = append(*c.asked, q.To)
	switch q.To {
	case toUtil:
		// A graph error, unless the question's Covered covers it.
		if q.Covered != nil && q.Covered(resolved.RuleGraph, tc, util) {
			return result.CanAnswer{Answer: result.ReplyYes}, nil
		}
		return result.CanAnswer{From: tc, To: util, Answer: result.ReplyNo, Rule: resolved.RuleGraph}, nil
	case forbidden:
		return result.CanAnswer{From: tc, To: forbidden, Answer: result.ReplyNo, Rule: "only-docker"}, nil
	case toNowhere:
		return result.CanAnswer{Answer: result.ReplyUnknown}, nil
	}
	return result.CanAnswer{Answer: result.ReplyYes}, nil
}

type clock struct{}

func (clock) Now() time.Time { return today }

// imports are four imports of tcFile: a first-party one the policy forbids
// (line 3), an allowed library (4), a library an only rule forbids (5), and a
// path in no module (6).
var imports = []evidence.ImportFact{
	{File: tcFile, Line: 3, Target: "pkg/util"},
	{File: tcFile, Line: 4, Target: stdlibFmt, External: true},
	{File: tcFile, Line: 5, Target: forbidden, External: true},
	{File: tcFile, Line: 6, Target: "pkg/nowhere"},
}

func TestRefusals(t *testing.T) {
	t.Parallel()
	graphException := resolved.Exception{Rule: resolved.RuleGraph, From: tc, To: util, Expires: today.AddDate(0, 0, 1)}
	for _, c := range []struct {
		name       string
		policy     policies
		base       string
		added      evidence.AddedLines
		wantLines  []int
		wantAsked  []string
		wantBaseIn string
	}{
		{name: "every no on an added line", policy: policies{ok: true}, base: base,
			added:     evidence.AddedLines{tcFile: {3, 4, 5, 6}},
			wantLines: []int{3, 5}, wantAsked: []string{toUtil, stdlibFmt, forbidden, toNowhere}, wantBaseIn: base},
		{name: "a line the change did not add is not asked about", policy: policies{ok: true}, base: base,
			added:     evidence.AddedLines{tcFile: {5}},
			wantLines: []int{5}, wantAsked: []string{forbidden}, wantBaseIn: base},
		{name: "no base: every line is added", policy: policies{ok: true},
			wantLines: []int{3, 5}, wantAsked: []string{toUtil, stdlibFmt, forbidden, toNowhere}},
		{name: "a no an exception covers is dropped", policy: policies{ok: true, p: resolved.Policy{Exceptions: []resolved.Exception{graphException}}},
			wantLines: []int{5}, wantAsked: []string{toUtil, stdlibFmt, forbidden, toNowhere}},
		{name: "no valid version 3 policy: nothing to ask", policy: policies{}},
	} {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			var asked []string
			var baseIn string
			svc := importcheck.Service{
				Policies: c.policy, Facts: facts{Imports: imports},
				Repository: repo{added: c.added, asked: &baseIn}, Can: can{asked: &asked}, Clock: clock{},
			}
			got, err := svc.Refusals(context.Background(), importcheck.Request{Base: c.base})
			if err != nil {
				t.Fatal(err)
			}
			var lines []int
			for _, r := range got {
				lines = append(lines, r.Line)
				if r.File != tcFile || r.Answer.Answer != result.ReplyNo {
					t.Errorf("refusal %+v, want a no on %s", r, tcFile)
				}
			}
			if !reflect.DeepEqual(lines, c.wantLines) || !reflect.DeepEqual(asked, c.wantAsked) || baseIn != c.wantBaseIn {
				t.Errorf("lines %v asked %v base %q; want %v %v %q", lines, asked, baseIn, c.wantLines, c.wantAsked, c.wantBaseIn)
			}
		})
	}
}

type failingCan struct{}

var errCan = errors.New("git failed")

func (failingCan) Can(context.Context, canquery.Question) (result.CanAnswer, error) {
	return result.CanAnswer{}, errCan
}

func TestRefusalsReturnsAPortError(t *testing.T) {
	t.Parallel()
	svc := importcheck.Service{Policies: policies{ok: true}, Facts: facts{Imports: imports}, Repository: repo{}, Can: failingCan{}, Clock: clock{}}
	if _, err := svc.Refusals(context.Background(), importcheck.Request{}); !errors.Is(err, errCan) {
		t.Errorf("error = %v, want the error of can", err)
	}
}
