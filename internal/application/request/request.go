// Package request builds the request of `archfit request -m "<why>"`: it
// asks an owner to change the policy for the errors of the last `check`, or
// for the dependency of the last `archfit can` answer (design 10.3). The
// worker writes only the reason; the rest comes from that output.
//
// # The request
//
// Milestone 2 hands it to the App, which owns the request contract (design
// 11.7) and opens an issue for it. The App takes the dependency and the
// balance from its own verified result; the numbers here are as reported.
//
//	reason         the worker's reason, required
//	expires        the date asked for an exception (YYYY-MM-DD); absent
//	               when the worker asks for a permanent answer
//	errors         the errors asked about, in record order:
//	  path, line   where the error is; absent for a `can` answer
//	  rule         the rule id, such as graph or duplication
//	  from, to     the modules of the dependency or the copy
//	  balance      the balance, when the rule is about coupling
//	head           the HEAD commit the output was produced on
//	base           the merge base of the check; absent for a `can` answer
//	policy_digest  the digest of the policy that judged the output
//
// Only the errors an owner may approve join: those whose next step is a
// command (`archfit request`), which the engine gives to an `ask` rule. An
// error that says "fix the code" or "wait for an owner: #n" is not asked
// again.
//
// # The last output
//
// `check` and `can` store what they produce, with the git tree id of the
// working tree, in `.archfit-cache/last.json` at the root of the checkout:
//
//	{"tree": "<working tree id>", "output": <the --format json document>}
//
// The tree id is what internal/history/git.WorkingTreeID returns. Every run
// writes the file, `pass` and `unknown` too, so "last" is the last
// command, not the last failure; a `check --policy` preview does not. The
// output is last only for the same HEAD and the same working tree: an edit
// after the check, a commit or a checkout makes it "run archfit check first"
// (design 4, no stale state).
//
// # Errors
//
// Every error of Service.Ask is one of the sentinels below, wrapped with its
// message, or an error of a port. Milestone 1 has no App sender (D5): Ask
// returns the request with number 0, and the CLI prints it and exits 2.
package request

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/alexei-led/archfit/v3/internal/model/result"
)

// Errors of Service.Ask, tested with errors.Is.
var (
	// ErrNoLastResult: nothing is stored for this HEAD and working tree.
	ErrNoLastResult = errors.New("no check result for this tree")
	// ErrNoVerdict: the last output is unknown.
	ErrNoVerdict = errors.New("the last output has no verdict")
	// ErrNothingToAsk: the last output has no error an owner may approve.
	ErrNothingToAsk = errors.New("nothing to ask")
	// ErrNoReason: no reason was given, and none was typed. A usage error.
	ErrNoReason = errors.New("a reason is required")
	// ErrExpires: --expires is not a date after the output's date. A usage
	// error.
	ErrExpires = errors.New("--expires is not a later date")
)

// Request asks an owner to change the policy. The package doc describes its
// fields.
type Request struct {
	Reason       string  `json:"reason"`
	Expires      string  `json:"expires,omitempty"`
	Errors       []Error `json:"errors"`
	Head         string  `json:"head,omitempty"`
	Base         string  `json:"base,omitempty"`
	PolicyDigest string  `json:"policy_digest"`
}

// Error is one error the request asks about.
type Error struct {
	Path    string          `json:"path,omitempty"`
	Line    int             `json:"line,omitempty"`
	Rule    string          `json:"rule"`
	From    string          `json:"from"`
	To      string          `json:"to"`
	Balance *result.Balance `json:"balance,omitempty"`
}

// Input is what the worker gives on the command line.
type Input struct {
	// Reason is the value of -m.
	Reason string
	// Expires is the value of --expires, YYYY-MM-DD; empty when not given.
	Expires string
}

// Last is the stored output of the last `check` or `can`: exactly one of
// Check and Can is set.
type Last struct {
	// Tree is the git tree id of the working tree the output was produced on.
	Tree  string
	Check *result.Result
	Can   *result.CanAnswer
}

// Checkout is the HEAD commit and the git tree id of the working tree now.
type Checkout struct {
	Head string
	Tree string
}

// LastStore reads the last output. ok is false when nothing usable is stored:
// no file, or a document of another schema.
type LastStore interface {
	Last(ctx context.Context) (last Last, ok bool, err error)
}

// CheckoutReader reads the checkout now.
type CheckoutReader interface {
	Checkout(ctx context.Context) (Checkout, error)
}

// ReasonReader asks the worker for the reason. The CLI wires it only on a
// terminal; elsewhere a missing -m is a usage error.
type ReasonReader interface {
	Reason(ctx context.Context) (string, error)
}

// Sender hands a request to the App and returns its number, the issue
// number. Milestone 1 has none (D5).
type Sender interface {
	Send(ctx context.Context, r Request) (int, error)
}

// Service is the `archfit request` use case. Last and Checkout are required;
// Reason and Sender may be nil.
type Service struct {
	Last     LastStore
	Checkout CheckoutReader
	Reason   ReasonReader
	Sender   Sender
}

// Outcome is the built request and its number.
type Outcome struct {
	Request Request
	// Number is the request number the App gave; 0 when no sender is wired.
	Number int
}

// Ask builds the request from the last output and sends it when a sender is
// wired. The reason is asked for last, so a worker never types one that
// cannot be used.
func (s Service) Ask(ctx context.Context, in Input) (Outcome, error) {
	last, err := s.current(ctx)
	if err != nil {
		return Outcome{}, err
	}
	req, date, err := fromLast(last)
	if err != nil {
		return Outcome{}, err
	}
	if req.Expires, err = expires(in.Expires, date); err != nil {
		return Outcome{}, err
	}
	if req.Reason, err = s.reason(ctx, in.Reason); err != nil {
		return Outcome{}, err
	}
	if s.Sender == nil {
		return Outcome{Request: req}, nil
	}
	number, err := s.Sender.Send(ctx, req)
	if err != nil {
		return Outcome{}, fmt.Errorf("send the request: %w", err)
	}
	return Outcome{Request: req, Number: number}, nil
}

// current returns the stored output when it was produced on this HEAD and
// this working tree.
func (s Service) current(ctx context.Context) (Last, error) {
	last, ok, err := s.Last.Last(ctx)
	if err != nil {
		return Last{}, fmt.Errorf("read the last output: %w", err)
	}
	if !ok || (last.Check == nil) == (last.Can == nil) {
		return Last{}, fmt.Errorf("%w: run archfit check first", ErrNoLastResult)
	}
	now, err := s.Checkout.Checkout(ctx)
	if err != nil {
		return Last{}, fmt.Errorf("read the checkout: %w", err)
	}
	if last.Tree == "" || last.Tree != now.Tree || head(last) != now.Head {
		return Last{}, fmt.Errorf("%w: run archfit check first", ErrNoLastResult)
	}
	return last, nil
}

func head(last Last) string {
	if last.Check != nil {
		return last.Check.Head
	}
	return last.Can.Head
}

// fromLast builds the request without reason and expiry, and returns the
// date the output was produced on.
func fromLast(last Last) (Request, string, error) {
	if c := last.Check; c != nil {
		if c.Status == result.StatusUnknown {
			return Request{}, "", fmt.Errorf("%w: do what its next step says, then run archfit check", ErrNoVerdict)
		}
		var errs []Error
		for _, e := range c.Errors {
			if e.Next.Kind == result.NextCommand {
				errs = append(errs, Error{Path: e.Path, Line: e.Line, Rule: e.Rule, From: e.From, To: e.To, Balance: e.Balance})
			}
		}
		if len(errs) == 0 {
			return Request{}, "", nothingToAsk(c.Status)
		}
		return Request{Errors: errs, Head: c.Head, Base: c.Base, PolicyDigest: c.Policy.Digest}, c.Date, nil
	}
	a := last.Can
	switch {
	case a.Answer == result.ReplyUnknown:
		return Request{}, "", fmt.Errorf("%w: do what its next step says, then run archfit can again", ErrNoVerdict)
	case a.Answer == result.ReplyYes:
		return Request{}, "", fmt.Errorf("%w: archfit can answered yes", ErrNothingToAsk)
	case a.Next == nil || a.Next.Kind != result.NextCommand:
		return Request{}, "", fmt.Errorf("%w: the rule says to fix the code", ErrNothingToAsk)
	}
	errs := []Error{{Rule: a.Rule, From: a.From, To: a.To, Balance: a.Balance}}
	return Request{Errors: errs, Head: a.Head, PolicyDigest: a.Policy.Digest}, a.Date, nil
}

func nothingToAsk(status result.Status) error {
	if status == result.StatusPass {
		return fmt.Errorf("%w: archfit: pass", ErrNothingToAsk)
	}
	return fmt.Errorf("%w: do what each next step says", ErrNothingToAsk)
}

// expires checks that value, when given, is a date after the output's date.
func expires(value, date string) (string, error) {
	if value == "" {
		return "", nil
	}
	day, err := time.Parse(time.DateOnly, value)
	if err != nil {
		return "", fmt.Errorf("%w: want a date after %s as YYYY-MM-DD, got %q", ErrExpires, date, value)
	}
	from, err := time.Parse(time.DateOnly, date)
	if err != nil {
		return "", fmt.Errorf("%w: its date %q is not valid: run archfit check again", ErrNoLastResult, date)
	}
	if !day.After(from) {
		return "", fmt.Errorf("%w: want a date after %s, got %s", ErrExpires, date, value)
	}
	return value, nil
}

// reason returns the given reason, or asks for one when it is blank and a
// terminal is wired.
func (s Service) reason(ctx context.Context, given string) (string, error) {
	why := strings.TrimSpace(given)
	if why == "" && s.Reason != nil {
		typed, err := s.Reason.Reason(ctx)
		if err != nil {
			return "", fmt.Errorf("read the reason: %w", err)
		}
		why = strings.TrimSpace(typed)
	}
	if why == "" {
		return "", fmt.Errorf(`%w: archfit request -m "<why>"`, ErrNoReason)
	}
	return why, nil
}
