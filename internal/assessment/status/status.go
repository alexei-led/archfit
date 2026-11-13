package status

import (
	"time"

	"github.com/bmatcuk/doublestar/v4"

	"github.com/alexei-led/archfit/v3/internal/assessment/finding"
	"github.com/alexei-led/archfit/v3/internal/policy"
)

// Assign labels findings using the waiver set without changing the input slice.
// now is the reference time for expiry checks.
func Assign(findings []finding.Finding, waivers policy.WaiverSet, now time.Time) []finding.Finding {
	out := make([]finding.Finding, len(findings))
	copy(out, findings)
	for i := range out {
		out[i].Status = assignOne(&out[i], waivers, now)
	}
	return out
}

func assignOne(f *finding.Finding, waivers policy.WaiverSet, now time.Time) finding.Status {
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
