package repostate

import (
	"time"

	"github.com/alexei-led/archfit/v3/internal/assessment/count"
	"github.com/alexei-led/archfit/v3/internal/model/report"
	"github.com/alexei-led/archfit/v3/internal/model/result"
)

// Report projects the state into the report contract. policyPath names the
// place of an expired exception; protected says whether a workflow guards the
// policy (design 9.4). Every list is non-nil, so the JSON has each key.
func (r Repository) Report(policyPath string, protected bool) report.RepositoryState {
	out := report.RepositoryState{
		State:      string(r.State),
		Drift:      make([]report.RepositoryError, 0, len(r.Drift)),
		Expired:    make([]report.RepositoryError, 0, len(r.Expired)),
		Expiring:   make([]report.ExpiringException, 0, len(r.Expiring)),
		NotChecked: make([]report.NoteRule, 0, len(r.NotChecked)),
		Balances:   make([]report.AllowedDependency, 0, len(r.Balances)),
		Unread:     make([]report.UnreadPlace, 0, len(r.Unknown)),
		Protected:  protected,
	}
	for _, e := range r.Drift {
		out.Drift = append(out.Drift, reportError(e, policyPath))
	}
	for _, e := range r.Expired {
		out.Expired = append(out.Expired, reportError(e, policyPath))
	}
	for _, x := range r.Expiring {
		out.Expiring = append(out.Expiring, report.ExpiringException{
			Rule: x.Exception.Rule, From: x.Exception.From, To: x.Exception.To, Expires: x.Expires, Why: x.Exception.Why,
		})
	}
	for _, n := range r.NotChecked {
		out.NotChecked = append(out.NotChecked, report.NoteRule{ID: n.ID, Note: n.Note, Why: n.Why})
	}
	for _, b := range r.Balances {
		out.Balances = append(out.Balances, report.AllowedDependency{From: b.From, To: b.To, Balance: balance(b.Balance)})
	}
	for _, u := range r.Unknown {
		out.Unread = append(out.Unread, report.UnreadPlace{Path: u.File, Line: u.Line, Reason: u.Reason, Fix: u.Fix})
	}
	return out
}

// reportError maps an error of count, as the check's record does: its first
// site, or the policy file for an expired exception.
func reportError(e count.Error, policyPath string) report.RepositoryError {
	out := report.RepositoryError{
		Rule: e.Key.Rule, From: e.From, To: e.To, What: e.What, Why: e.Why, Instead: e.Instead,
	}
	if e.Balance != nil {
		b := balance(*e.Balance)
		out.Balance = &b
	}
	if x := e.Exception; x != nil {
		out.Path = policyPath
		if !x.Expires.IsZero() {
			out.Expires = x.Expires.UTC().Format(time.DateOnly)
		}
		return out
	}
	if len(e.Sites) > 0 {
		out.Path, out.Line = e.Sites[0].File, e.Sites[0].Line
	}
	return out
}

func balance(b result.Balance) report.RepositoryBalance {
	return report.RepositoryBalance{Value: b.Value, Min: b.Min}
}
