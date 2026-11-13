package changecheck

import (
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/alexei-led/archfit/v3/internal/application/repostate"
	"github.com/alexei-led/archfit/v3/internal/assessment/change"
	"github.com/alexei-led/archfit/v3/internal/assessment/count"
	"github.com/alexei-led/archfit/v3/internal/model/result"
)

// project is the one place that builds the result of a check: the status
// and its line, the errors in the order count gives them, the reasons for
// unknown, the repository state on a push, the policy, the commits, the date
// and the engine version. Lists are never nil.
func project(r run, protected bool, now time.Time, engine string) result.Result {
	res := result.Result{
		Schema: result.SchemaVersion,
		Status: result.StatusUnknown,
		Errors: []result.Error{},
		Policy: result.Policy{
			Path: r.policyPath, Digest: r.digest, Protected: protected, Differs: r.differs, Target: r.target,
		},
		Base:   r.base,
		Head:   r.head,
		Date:   now.UTC().Format(time.DateOnly),
		Engine: engine,
	}
	res.Unknown = make([]result.Unknown, 0, len(r.reasons))
	for _, why := range r.reasons {
		res.Unknown = append(res.Unknown, result.Unknown{
			Path: why.path, Cause: why.cause, What: why.what, Why: why.why,
			Next: result.Next{Kind: why.kind, Text: why.next},
		})
	}
	if v := r.verdict; v != nil {
		res.Status = v.Status
		for _, e := range v.Errors {
			res.Errors = append(res.Errors, projectError(e, r.policyPath))
		}
		res.Unknown = append(res.Unknown, v.Unknown...)
	}
	if r.repository != nil {
		res.Repository = projectRepository(*r.repository, r.policyPath)
	}
	res.Summary = summary(res.Status, res.Errors)
	return res
}

// projectRepository maps the repository state: its drift in the fields of
// an error, and the exceptions close to their date. The expired exceptions
// are not repeated: the result's errors list them for every run.
func projectRepository(s repostate.Repository, policyPath string) *result.Repository {
	out := &result.Repository{
		State:    result.RepositoryState(s.State),
		Drift:    make([]result.Error, 0, len(s.Drift)),
		Expiring: make([]result.ExpiringException, 0, len(s.Expiring)),
	}
	for _, e := range s.Drift {
		out.Drift = append(out.Drift, projectError(e, policyPath))
	}
	for _, x := range s.Expiring {
		out.Expiring = append(out.Expiring, result.ExpiringException{
			Rule: x.Exception.Rule, From: x.Exception.From, To: x.Exception.To,
			Expires: x.Exception.Expires.UTC().Format(time.DateOnly), Why: x.Exception.Why,
		})
	}
	return out
}

// projectError maps an error of count. The record shows the first site; an
// expired exception has no site and names the policy file.
func projectError(e count.Error, policyPath string) result.Error {
	out := result.Error{
		Rule: e.Key.Rule, What: e.What, Why: e.Why, Instead: e.Instead, Balance: e.Balance, Next: e.Next,
		From: e.From, To: e.To, Key: keyText(e.Key), FromChange: e.FromChange,
	}
	if x := e.Exception; x != nil {
		out.Path = policyPath
		out.Exception = &result.Exception{Rule: x.Rule, Expires: x.Expires.UTC().Format(time.DateOnly)}
		return out
	}
	out.Path, out.Line = e.Sites[0].File, e.Sites[0].Line
	return out
}

// keyText writes a key as its parts that are set, separated by spaces: the
// rule, the modules, the symbol. An expired exception's key holds the rule it
// covers as its symbol and writes it after "exception": "exception
// duplication pkg_tc pkg_util".
func keyText(k change.Key) string {
	parts := []string{k.Rule, k.From, k.To, k.Symbol}
	if k.Rule == count.RuleException {
		parts = []string{k.Rule, k.Symbol, k.From, k.To}
	}
	return strings.Join(slices.DeleteFunc(parts, func(p string) bool { return p == "" }), " ")
}

// summary is the status line of the record: "archfit: pass", "archfit:
// unknown, no verdict", or "archfit: fail, N errors", with "(not from this
// change)" when the change added none of them.
func summary(status result.Status, errs []result.Error) string {
	switch status {
	case result.StatusPass:
		return "archfit: pass"
	case result.StatusFail:
		noun := "errors"
		if len(errs) == 1 {
			noun = "error"
		}
		line := fmt.Sprintf("archfit: fail, %d %s", len(errs), noun)
		if !slices.ContainsFunc(errs, func(e result.Error) bool { return e.FromChange }) {
			line += " (not from this change)"
		}
		return line
	default:
		return "archfit: unknown, no verdict"
	}
}
