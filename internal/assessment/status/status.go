package status

import (
	"sort"
	"strings"
	"time"

	"github.com/bmatcuk/doublestar/v4"

	"github.com/alexei-led/archfit/internal/assessment/finding"
	"github.com/alexei-led/archfit/internal/policy"
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
	for _, w := range waivers.Waivers {
		if !matchWaiver(w, f) {
			continue
		}
		if isExpired(w, now) {
			return finding.StatusExpiredWaiver
		}
		return finding.StatusWaived
	}

	// 4. Default.
	return finding.StatusNew
}

// matchWaiver reports whether w applies to f, regardless of expiry.
// An empty Rule, From, or To field matches any value.
func matchWaiver(w policy.WaiverDef, f *finding.Finding) bool {
	// Rule ID match.
	if w.Rule != "" && w.Rule != f.RuleID {
		return false
	}

	// From glob match against Edge.From.Path.
	if w.From != "" {
		matched, _ := doublestar.Match(w.From, f.Edge.From.Path)
		if !matched {
			return false
		}
	}

	// To glob match against Edge.To.Path.
	if w.To != "" {
		matched, _ := doublestar.Match(w.To, f.Edge.To.Path)
		if !matched {
			return false
		}
	}

	return true
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
	// Expiry is end-of-day: the waiver is valid on the expiry date itself,
	// expired the day after. Add 24 hours so "expires: 2025-01-31" covers all of Jan 31.
	return now.After(expiry.Add(24 * time.Hour))
}

// DeltaResult groups finding IDs by delta bucket. Buckets are mutually exclusive
// (precedence: resolved > new > severity_changed > touched_by_delta > existing)
// and each slice is sorted for deterministic output.
type DeltaResult struct {
	New             []string
	Existing        []string
	Resolved        []string
	SeverityChanged []string
	TouchedByDelta  []string
}

// Empty reports whether no finding landed in any bucket.
func (r DeltaResult) Empty() bool {
	return len(r.New)+len(r.Existing)+len(r.Resolved)+len(r.SeverityChanged)+len(r.TouchedByDelta) == 0
}

// DeltaBuckets categorizes findings for a delta run relative to the baseline and
// the changed-file set. It reuses the lifecycle status already on each finding
// and the accepted set's recorded severity — no re-classification. Each finding
// lands in exactly one bucket:
//
//   - Resolved: status fixed (a baseline finding no longer detected).
//   - New: status new (introduced by this change).
//   - SeverityChanged: a baseline finding whose severity differs from the
//     severity stored in the baseline (skipped when the baseline predates
//     severity tracking, i.e. the recorded severity is empty).
//   - TouchedByDelta: a remaining pre-existing finding whose edge endpoints sit
//     on a file in the changed set.
//   - Existing: any remaining pre-existing finding (untouched by this change).
//
// changed is the sorted list of repo-relative files in the delta scope.
func DeltaBuckets(findings []finding.Finding, accepted AcceptedSet, changed []string) DeltaResult {
	sevByFP := make(map[string]string)
	for _, e := range accepted.Entries() {
		sevByFP[e.Fingerprint] = e.Severity
	}

	var r DeltaResult
	for i := range findings {
		f := &findings[i]
		switch f.Status {
		case finding.StatusFixed:
			r.Resolved = append(r.Resolved, f.ID)
		case finding.StatusNew:
			r.New = append(r.New, f.ID)
		default:
			if sev, ok := sevByFP[f.ID]; ok && sev != "" && sev != string(f.Severity) {
				r.SeverityChanged = append(r.SeverityChanged, f.ID)
			} else if touchesChanged(f, changed) {
				r.TouchedByDelta = append(r.TouchedByDelta, f.ID)
			} else {
				r.Existing = append(r.Existing, f.ID)
			}
		}
	}

	for _, s := range [][]string{r.New, r.Existing, r.Resolved, r.SeverityChanged, r.TouchedByDelta} {
		sort.Strings(s)
	}
	return r
}

// matchedByModuleKey mirrors internal/rules' unexported matchedByModule
// MatchedBy key ("module") — the two packages agree on the key by convention,
// not by import, since status must not depend on the rules package.
const matchedByModuleKey = "module"

// touchesChanged reports whether f sits on a file in changed. Locations carry
// the real file evidence; edge endpoints are checked too, matching across path
// granularities (a finding path can be a package directory while changed lists
// files, so a prefix relation in either direction counts as a touch). An
// endpoint equal to the finding's MatchedBy module key is skipped when a
// location exists: the public_api_* rules stamp the bare config module key on
// both endpoints, and a key that collides with an unrelated real path (module
// "docs" owning src/domain/** next to a real docs/ dir) must not count as a
// touch. With no locations the endpoints remain the only, best-effort evidence.
func touchesChanged(f *finding.Finding, changed []string) bool {
	hasLoc := false
	for _, loc := range f.Locations {
		if loc.File == "" {
			continue
		}
		hasLoc = true
		for _, c := range changed {
			if pathRelated(loc.File, c) {
				return true
			}
		}
	}
	modKey := f.MatchedBy[matchedByModuleKey]
	for _, p := range []string{f.Edge.From.Path, f.Edge.To.Path} {
		if p == "" || (hasLoc && p == modKey) {
			continue
		}
		for _, c := range changed {
			if pathRelated(p, c) {
				return true
			}
		}
	}
	return false
}

// pathRelated reports whether a and b name the same file, or one is a directory
// prefix of the other (e.g. package "internal/foo" vs file "internal/foo/bar.go").
func pathRelated(a, b string) bool {
	return a == b || strings.HasPrefix(b, a+"/") || strings.HasPrefix(a, b+"/")
}
