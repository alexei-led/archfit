package status

import (
	"time"

	"github.com/bmatcuk/doublestar/v4"

	"github.com/alexei-led/archfit/v3/internal/assessment/finding"
	"github.com/alexei-led/archfit/v3/internal/policy"
)

// AcceptedEntry is one accepted finding from a prior run: the fingerprint that
// identifies it, the rule that produced it, the finding kind (gate or advisory;
// empty means "gate" for backward compatibility), and the severity recorded when
// it was accepted (empty for baselines written before severity was tracked).
type AcceptedEntry struct {
	Fingerprint string
	RuleID      string
	Kind        string
	Severity    string
}

// AcceptedSet is the read-only view of previously accepted findings that
// status assignment needs. The persistence layer (internal/baseline)
// implements it; status never touches storage concerns — the dependency
// points outward-in.
type AcceptedSet interface {
	// HasFingerprint reports whether the fingerprint was accepted.
	HasFingerprint(fingerprint string) bool
	// Entries returns all accepted findings, in stored order.
	Entries() []AcceptedEntry
}

// Empty is the accepted set of a run with no persisted baseline: nothing was
// ever accepted, so every finding is new and nothing can be reported fixed.
type Empty struct{}

// HasFingerprint always reports false.
func (Empty) HasFingerprint(string) bool { return false }

// Entries returns no accepted findings.
func (Empty) Entries() []AcceptedEntry { return nil }

// Assign labels each finding with its lifecycle status by comparing against the
// accepted set and exception set. It also emits a finding per accepted
// fingerprint that is no longer present in the current run (status=fixed).
//
// forKind scopes fixed-finding emission: only baseline entries whose Kind field
// matches forKind are emitted as fixed. Entries with an empty Kind field are
// treated as "gate" (backward compatibility with pre-Kind baseline files).
// otherCurrent supplies findings from other passes for disappearance detection;
// they are not classified or included in the returned findings.
//
// Algorithm per finding f:
//  1. Fingerprint in base.Accepted → StatusBaseline
//  2. Matches a waiver (rule, from glob, to glob) and not expired → StatusWaived
//  3. Matches a waiver but expiry has passed → StatusExpiredWaiver
//  4. No match → StatusNew (default)
//
// now is the reference time for expiry checks; pass time.Now() in production.
func Assign(
	findings []finding.Finding,
	accepted AcceptedSet,
	waivers policy.WaiverSet,
	now time.Time,
	forKind string,
	otherCurrent ...finding.Finding,
) []finding.Finding {
	// Build a set of current fingerprints for fixed-finding detection.
	current := make(map[string]struct{}, len(findings)+len(otherCurrent))
	for _, f := range findings {
		current[f.ID] = struct{}{}
	}
	for _, f := range otherCurrent {
		current[f.ID] = struct{}{}
	}

	// Copy findings so we don't mutate the caller's slice.
	out := make([]finding.Finding, len(findings))
	copy(out, findings)

	for i := range out {
		out[i].Status = assignOne(&out[i], accepted, waivers, now)
	}

	// Emit fixed findings only for accepted entries whose kind matches this pass.
	// Empty Kind in the entry means "gate" (backward compat).
	for _, a := range accepted.Entries() {
		if _, present := current[a.Fingerprint]; present {
			continue
		}
		entryKind := a.Kind
		if entryKind == "" {
			entryKind = "gate"
		}
		if entryKind != forKind {
			continue
		}
		out = append(out, finding.Finding{
			ID:     a.Fingerprint,
			Kind:   entryKind,
			RuleID: a.RuleID,
			Status: finding.StatusFixed,
		})
	}

	return out
}

// assignOne returns the status for a single finding.
func assignOne(
	f *finding.Finding,
	accepted AcceptedSet,
	waivers policy.WaiverSet,
	now time.Time,
) finding.Status {
	// 1. Accepted-set check.
	if accepted.HasFingerprint(f.ID) {
		return finding.StatusBaseline
	}

	// 2–3. Waiver check.
	hasExpiredMatch := false
	for _, w := range waivers.Waivers {
		if !matchWaiver(w, f) {
			continue
		}
		if isExpired(w, now) {
			hasExpiredMatch = true
			continue
		}
		return finding.StatusWaived
	}
	if hasExpiredMatch {
		return finding.StatusExpiredWaiver
	}

	// 4. Default.
	return finding.StatusNew
}

// matchWaiver reports whether w applies to f, regardless of expiry.
// An empty Rule, From, or To field matches any value. Synthetic module-only
// findings use the endpoint module when no endpoint path is available.
func matchWaiver(w policy.WaiverDef, f *finding.Finding) bool {
	// Rule ID match.
	if w.Rule != "" && w.Rule != f.RuleID {
		return false
	}

	// From glob match against Edge.From.Path.
	if !matchEndpoint(w.From, f.Edge.From) {
		return false
	}

	// To glob match against Edge.To.Path.
	if !matchEndpoint(w.To, f.Edge.To) {
		return false
	}

	return true
}

func matchEndpoint(glob string, endpoint finding.Endpoint) bool {
	if glob == "" {
		return true
	}
	value := endpoint.Path
	if value == "" {
		value = endpoint.Module
	}
	if value == "" {
		return false
	}
	matched, _ := doublestar.Match(glob, value)
	return matched
}

// isExpired reports whether w has an expiry date that has passed relative to now.
// An empty Expires field is never expired.
func isExpired(w policy.WaiverDef, now time.Time) bool {
	if w.Expires == "" {
		return false
	}
	expiry, err := time.Parse("2006-01-02", w.Expires)
	if err != nil {
		// Malformed date — treat as expired so the waiver does not silently
		// suppress findings with an unenforceable expiry.
		return true
	}
	// Expiry is end-of-day: the waiver is valid on the expiry date itself and
	// expired at midnight on the following day.
	return !now.Before(expiry.Add(24 * time.Hour))
}
