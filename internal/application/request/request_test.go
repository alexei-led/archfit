package request_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/alexei-led/archfit/v3/internal/application/request"
	"github.com/alexei-led/archfit/v3/internal/model/result"
	"github.com/alexei-led/archfit/v3/internal/testutil"
)

// contractExamples holds the JSON of every example of design 8.1. The request
// goldens here are written from those examples by hand.
const contractExamples = "../../model/result/testdata"

const (
	exampleHead = "1a954628a960aaef81d7b2d4521929579f3541e6"
	treeAtCheck = "4b825dc642cb6eb9a060e54bf8d69288fbee4904"
	reasonGraph = "netem needs the shared retry helper"
	reasonCopy  = "tc parses the interface name before pkg_util is loaded"
	// laterDay is a day after the check, on an unchanged tree.
	laterDay = "2026-11-14"
)

// TestRequestsOfTheDesignExamples builds the request of the 8.1 graph error,
// the copy, and the `can` answer, and compares each with its golden.
func TestRequestsOfTheDesignExamples(t *testing.T) {
	for _, tc := range []struct {
		name string
		last request.Last
		in   request.Input
	}{
		{name: "graph", last: checked(t, "graph"), in: request.Input{Reason: reasonGraph}},
		{name: "duplication", last: checked(t, "duplication"), in: request.Input{Reason: reasonCopy, Expires: "2026-12-10"}},
		{name: "can", last: answered(t), in: request.Input{Reason: "  " + reasonGraph + "\n"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out, err := service(tc.last).Ask(context.Background(), tc.in)
			if err != nil {
				t.Fatalf("Ask: %v", err)
			}
			if out.Number != 0 {
				t.Errorf("number = %d without a sender, want 0", out.Number)
			}
			want, err := os.ReadFile(filepath.Join("testdata", tc.name+".json"))
			if err != nil {
				t.Fatal(err)
			}
			if got := encode(t, out.Request); !bytes.Equal(got, want) {
				t.Errorf("request differs from the golden:\n--- golden\n%s\n--- built\n%s", want, got)
			}
		})
	}
}

// TestAskRefuses pins every reason Ask builds no request, and that the worker
// is never asked to type a reason for one.
func TestAskRefuses(t *testing.T) {
	mixed := checked(t, "graph")
	mixed.Check.Errors = append(checked(t, "deny").Check.Errors, mixed.Check.Errors...)
	canYes := answered(t)
	canYes.Can.Answer, canYes.Can.Next = result.ReplyYes, nil
	canUnknown := answered(t)
	canUnknown.Can.Answer = result.ReplyUnknown
	canFix := answered(t)
	canFix.Can.Next = &result.Next{Kind: result.NextFix, Text: "fix the code"}
	empty := checked(t, "graph")
	empty.Check = nil
	badDate := checked(t, "graph")
	badDate.Check.Date = "10 Nov 2026"

	for _, tc := range []struct {
		name    string
		store   lastStore
		now     request.Checkout
		in      request.Input
		typed   *string
		noTTY   bool   // no terminal: Reason is nil
		today   string // the clock; the check's date when empty
		want    error
		asked   bool
		wantErr string
	}{
		// Cases with a terminal prove the worker is asked only for a
		// request that can be built.
		{name: "nothing stored", store: lastStore{}, want: request.ErrNoLastResult},
		{name: "a stored document with no output", store: stored(empty), want: request.ErrNoLastResult},
		{name: "another head", store: stored(checked(t, "graph")), now: request.Checkout{Head: "other", Tree: treeAtCheck}, want: request.ErrNoLastResult},
		{name: "a file edited after the check", store: stored(checked(t, "graph")), now: request.Checkout{Head: exampleHead, Tree: "edited"}, want: request.ErrNoLastResult},
		{name: "a stored output without a tree", store: stored(withTree(checked(t, "graph"), "")), now: request.Checkout{Head: exampleHead}, want: request.ErrNoLastResult},
		{name: "a pass", store: stored(checked(t, "pass")), want: request.ErrNothingToAsk, wantErr: "nothing to ask: archfit: pass"},
		{name: "a deny error says fix the code", store: stored(checked(t, "deny")), want: request.ErrNothingToAsk},
		{name: "an expired exception waits for its request", store: stored(checked(t, "expired_exception")), want: request.ErrNothingToAsk},
		{name: "no verdict", store: stored(checked(t, "policy_differs")), want: request.ErrNoVerdict},
		{name: "can answered yes", store: stored(canYes), want: request.ErrNothingToAsk},
		{name: "can answered unknown", store: stored(canUnknown), want: request.ErrNoVerdict},
		{name: "can says fix the code", store: stored(canFix), want: request.ErrNothingToAsk},
		// The expiry cases give no reason: the terminal must not be asked.
		{name: "a date that is not valid", store: stored(badDate), in: request.Input{Expires: "2026-12-10"}, want: request.ErrNoLastResult},
		{name: "expires on the check's date", store: stored(mixed), in: request.Input{Expires: "2026-11-10"}, want: request.ErrExpires},
		{name: "expires before the check's date", store: stored(mixed), in: request.Input{Expires: "2026-11-09"}, want: request.ErrExpires},
		{name: "expires after the check but not after today", store: stored(mixed), today: laterDay, in: request.Input{Expires: "2026-11-12"}, want: request.ErrExpires, wantErr: "--expires is not a later date: want a date after 2026-11-14, got 2026-11-12"},
		{name: "expires today", store: stored(mixed), today: laterDay, in: request.Input{Expires: laterDay}, want: request.ErrExpires},
		{name: "expires not ISO", store: stored(mixed), in: request.Input{Expires: "10/12/2026"}, want: request.ErrExpires},
		{name: "expires not a day", store: stored(mixed), in: request.Input{Expires: "2026-02-30"}, want: request.ErrExpires},
		{name: "no reason and no terminal", store: stored(mixed), noTTY: true, want: request.ErrNoReason, wantErr: `a reason is required: archfit request -m "<why>"`},
		{name: "a blank reason and no terminal", store: stored(mixed), in: request.Input{Reason: " \t\n"}, noTTY: true, want: request.ErrNoReason},
		{name: "the terminal gives no reason", store: stored(mixed), typed: new("  \n"), want: request.ErrNoReason, asked: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s := request.Service{Last: tc.store, Checkout: fixedCheckout{now: orDefault(tc.now)}, Clock: on(t, tc.today)}
			terminal := &reasonReader{}
			if tc.typed != nil {
				terminal.text = *tc.typed
			}
			if !tc.noTTY {
				s.Reason = terminal
			}
			_, err := s.Ask(context.Background(), tc.in)
			if !errors.Is(err, tc.want) {
				t.Fatalf("Ask error = %v, want %v", err, tc.want)
			}
			if tc.wantErr != "" && err.Error() != tc.wantErr {
				t.Errorf("message = %q, want %q", err.Error(), tc.wantErr)
			}
			if words := testutil.NotUsedIn(err.Error()); len(words) > 0 {
				t.Errorf("message %q uses %q (design 5, the Not used column)", err, words)
			}
			if asked := terminal.calls > 0; asked != tc.asked {
				t.Errorf("asked for a reason = %v, want %v", asked, tc.asked)
			}
		})
	}
}

// TestOnlyAskErrorsJoin pins that a mixed result asks only about the error an
// owner may approve.
func TestOnlyAskErrorsJoin(t *testing.T) {
	mixed := checked(t, "graph")
	mixed.Check.Errors = append(checked(t, "deny").Check.Errors, mixed.Check.Errors...)
	out, err := service(mixed).Ask(context.Background(), request.Input{Reason: reasonGraph})
	if err != nil {
		t.Fatalf("Ask: %v", err)
	}
	if len(out.Request.Errors) != 1 || out.Request.Errors[0].Rule != "graph" {
		t.Errorf("errors = %+v, want only the graph error", out.Request.Errors)
	}
}

func TestReasonFromTheTerminal(t *testing.T) {
	s := service(checked(t, "graph"))
	terminal := &reasonReader{text: "  " + reasonGraph + "\n"}
	s.Reason = terminal
	out, err := s.Ask(context.Background(), request.Input{})
	if err != nil {
		t.Fatalf("Ask: %v", err)
	}
	if out.Request.Reason != reasonGraph || terminal.calls != 1 {
		t.Errorf("reason = %q after %d questions, want %q after 1", out.Request.Reason, terminal.calls, reasonGraph)
	}

	s.Reason = &reasonReader{}
	if out, err = s.Ask(context.Background(), request.Input{Reason: "given"}); err != nil || out.Request.Reason != "given" {
		t.Errorf("a given reason: %q, %v; want it used without a question", out.Request.Reason, err)
	}
}

func TestSender(t *testing.T) {
	s := service(checked(t, "graph"))
	sender := &fakeSender{number: 431}
	s.Sender = sender
	out, err := s.Ask(context.Background(), request.Input{Reason: reasonGraph})
	if err != nil {
		t.Fatalf("Ask: %v", err)
	}
	if out.Number != 431 {
		t.Errorf("number = %d, want 431", out.Number)
	}
	if len(sender.got) != 1 || sender.got[0].Reason != reasonGraph || len(sender.got[0].Errors) != 1 {
		t.Errorf("sent %+v, want the built request once", sender.got)
	}
}

// TestPortErrors pins that a failing port stops Ask with its error.
func TestPortErrors(t *testing.T) {
	boom := errors.New("boom")
	for _, tc := range []struct {
		name string
		s    request.Service
	}{
		{name: "store", s: request.Service{Last: lastStore{err: boom}, Checkout: fixedCheckout{}, Clock: on(t, "")}},
		{name: "checkout", s: request.Service{Last: stored(checked(t, "graph")), Checkout: fixedCheckout{err: boom}, Clock: on(t, "")}},
		{name: "terminal", s: request.Service{Last: stored(checked(t, "graph")), Checkout: fixedCheckout{now: orDefault(request.Checkout{})}, Clock: on(t, ""), Reason: &reasonReader{err: boom}}},
		{name: "sender", s: request.Service{Last: stored(checked(t, "graph")), Checkout: fixedCheckout{now: orDefault(request.Checkout{})}, Clock: on(t, ""), Sender: &fakeSender{err: boom}}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			in := request.Input{Reason: "x"}
			if tc.name == "terminal" {
				in.Reason = ""
			}
			if _, err := tc.s.Ask(context.Background(), in); !errors.Is(err, boom) {
				t.Errorf("Ask error = %v, want %v", err, boom)
			}
		})
	}
}

func TestGoldensUseNoNotUsedWord(t *testing.T) {
	goldens, err := filepath.Glob(filepath.Join("testdata", "*.json"))
	if err != nil || len(goldens) == 0 {
		t.Fatalf("goldens: %v, %d found", err, len(goldens))
	}
	for _, path := range goldens {
		raw, err := os.ReadFile(path) //nolint:gosec // golden files of this package
		if err != nil {
			t.Fatal(err)
		}
		if words := testutil.NotUsedIn(string(raw)); len(words) > 0 {
			t.Errorf("%s uses %q (design 5, the Not used column)", path, words)
		}
	}
}

func service(last request.Last) request.Service {
	return request.Service{Last: stored(last), Checkout: fixedCheckout{now: orDefault(request.Checkout{})}, Clock: fixedClock(checkDate)}
}

// on is the clock at day, or on the check's date when day is empty, late in
// the day so only the date counts.
func on(t *testing.T, day string) fixedClock {
	t.Helper()
	if day == "" {
		return fixedClock(checkDate)
	}
	at, err := time.Parse(time.DateOnly, day)
	if err != nil {
		t.Fatal(err)
	}
	return fixedClock(at.Add(23 * time.Hour))
}

// checkDate is the date of every design example, at 23:00.
var checkDate = time.Date(2026, 11, 10, 23, 0, 0, 0, time.UTC)

type fixedClock time.Time

func (c fixedClock) Now() time.Time { return time.Time(c) }

func checked(t *testing.T, name string) request.Last {
	t.Helper()
	var r result.Result
	decodeExample(t, name, &r)
	return request.Last{Tree: treeAtCheck, Check: &r}
}

func answered(t *testing.T) request.Last {
	t.Helper()
	var a result.CanAnswer
	decodeExample(t, "can", &a)
	return request.Last{Tree: treeAtCheck, Can: &a}
}

func withTree(last request.Last, tree string) request.Last {
	last.Tree = tree
	return last
}

func decodeExample(t *testing.T, name string, v any) {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join(contractExamples, name+".json")) //nolint:gosec // fixed example names
	if err != nil {
		t.Fatal(err)
	}
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.DisallowUnknownFields()
	if err := dec.Decode(v); err != nil {
		t.Fatalf("decode %s: %v", name, err)
	}
}

func encode(t *testing.T, v any) []byte {
	t.Helper()
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	enc.SetIndent("", "  ")
	if err := enc.Encode(v); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

// orDefault is the checkout the examples were produced on, unless the case
// names another.
func orDefault(now request.Checkout) request.Checkout {
	if now == (request.Checkout{}) {
		return request.Checkout{Head: exampleHead, Tree: treeAtCheck}
	}
	return now
}

type lastStore struct {
	last request.Last
	ok   bool
	err  error
}

func stored(last request.Last) lastStore { return lastStore{last: last, ok: true} }

func (s lastStore) Last(context.Context) (request.Last, bool, error) { return s.last, s.ok, s.err }

type fixedCheckout struct {
	now request.Checkout
	err error
}

func (c fixedCheckout) Checkout(context.Context) (request.Checkout, error) { return c.now, c.err }

type reasonReader struct {
	text  string
	err   error
	calls int
}

func (r *reasonReader) Reason(context.Context) (string, error) {
	r.calls++
	return r.text, r.err
}

type fakeSender struct {
	number int
	err    error
	got    []request.Request
}

func (s *fakeSender) Send(_ context.Context, r request.Request) (int, error) {
	s.got = append(s.got, r)
	return s.number, s.err
}
