package status

import (
	"time"

	"github.com/bmatcuk/doublestar/v4"

	"github.com/alexei-led/archfit/internal/baseline"
	"github.com/alexei-led/archfit/internal/config"
	"github.com/alexei-led/archfit/internal/model/finding"
)

// Assign labels each finding with its lifecycle status by comparing against the
// baseline and exception set. It also emits a finding per accepted fingerprint
// that is no longer present in the current run (status=fixed).
//
// Algorithm per finding f:
//  1. Fingerprint in base.Accepted → StatusBaseline
//  2. Matches an exception (rule, from glob, to glob) and not expired → StatusExcepted
//  3. Matches an exception but expiry has passed → StatusExpiredException
//  4. No match → StatusNew (default)
//
// Fixed findings: for every AcceptedFinding in base that has no matching
// fingerprint in the current finding set, a synthetic Finding is emitted with
// Status=StatusFixed.
//
// now is the reference time for expiry checks; pass time.Now() in production.
func Assign(
	findings []finding.Finding,
	base baseline.Baseline,
	exceptions config.ExceptionSet,
	now time.Time,
) []finding.Finding {
	// Build a set of current fingerprints for fixed-finding detection.
	current := make(map[string]struct{}, len(findings))
	for _, f := range findings {
		current[f.ID] = struct{}{}
	}

	// Copy findings so we don't mutate the caller's slice.
	out := make([]finding.Finding, len(findings))
	copy(out, findings)

	for i := range out {
		out[i].Status = assignOne(&out[i], base, exceptions, now)
	}

	// Emit fixed findings for accepted fingerprints absent from this run.
	for _, a := range base.Accepted {
		if _, present := current[a.Fingerprint]; present {
			continue
		}
		out = append(out, finding.Finding{
			ID:     a.Fingerprint,
			Kind:   "gate",
			RuleID: a.RuleID,
			Status: finding.StatusFixed,
		})
	}

	return out
}

// assignOne returns the status for a single finding.
func assignOne(
	f *finding.Finding,
	base baseline.Baseline,
	exceptions config.ExceptionSet,
	now time.Time,
) finding.Status {
	// 1. Baseline check.
	if base.HasFingerprint(f.ID) {
		return finding.StatusBaseline
	}

	// 2–3. Exception check.
	for _, exc := range exceptions.Exceptions {
		if !matchException(exc, f) {
			continue
		}
		if isExpired(exc, now) {
			return finding.StatusExpiredExcept
		}
		return finding.StatusExcepted
	}

	// 4. Default.
	return finding.StatusNew
}

// matchException reports whether exc applies to f, regardless of expiry.
// An empty Rule, From, or To field matches any value.
func matchException(exc config.ExceptionDef, f *finding.Finding) bool {
	// Rule ID match.
	if exc.Rule != "" && exc.Rule != f.RuleID {
		return false
	}

	// From glob match against Edge.From.Path.
	if exc.From != "" {
		matched, _ := doublestar.Match(exc.From, f.Edge.From.Path)
		if !matched {
			return false
		}
	}

	// To glob match against Edge.To.Path.
	if exc.To != "" {
		matched, _ := doublestar.Match(exc.To, f.Edge.To.Path)
		if !matched {
			return false
		}
	}

	return true
}

// isExpired reports whether exc has an expiry date that has passed relative to now.
// An empty Expires field is never expired.
func isExpired(exc config.ExceptionDef, now time.Time) bool {
	if exc.Expires == "" {
		return false
	}
	expiry, err := time.Parse("2006-01-02", exc.Expires)
	if err != nil {
		// Malformed date — treat as expired so the exception does not silently
		// suppress findings with an unenforceable expiry.
		return true
	}
	// Expiry is end-of-day: the exception is valid on the expiry date itself,
	// expired the day after. Add 24 hours so "expires: 2025-01-31" covers all of Jan 31.
	return now.After(expiry.Add(24 * time.Hour))
}
