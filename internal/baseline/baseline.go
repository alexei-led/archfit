// Package baseline handles loading and saving the archfit baseline file,
// which tracks accepted findings and metric snapshots across runs.
package baseline

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"

	"github.com/alexei-led/archfit/internal/model/coupling"
	"github.com/alexei-led/archfit/internal/model/diagnostic"
	"github.com/alexei-led/archfit/internal/status"
)

// SchemaVersion is the fixed schema_version value for baseline files.
const SchemaVersion = "archfit.baseline.v1"

// AcceptedFinding records a finding that has been accepted into the baseline.
// Fingerprint is the SHA256 hex ID from finding.New; RuleID is the rule that produced it.
// Kind distinguishes gate findings from advisory findings; empty means "gate" for
// backward compatibility with baseline files written before this field was added.
type AcceptedFinding struct {
	Fingerprint string `json:"fingerprint"`
	RuleID      string `json:"rule_id"`
	Kind        string `json:"kind,omitempty"`
	// Severity is the finding's severity at acceptance time. Recorded so a delta
	// run can flag findings whose severity later changed. Omitted (empty) in
	// baselines written before severity tracking — treated as "unknown", never a
	// severity change.
	Severity string `json:"severity,omitempty"`
}

// ScoreSnapshot records the synthesised coupling_balance score at baseline
// time, anchoring the coupling.gate.max_drop check on later runs. Written only
// when the score was measured — an n/a (unmeasured) synthesis stores nothing,
// so it can never anchor a phantom drop.
type ScoreSnapshot struct {
	CouplingBalance int `json:"coupling_balance"`
	// Band is disclosure-only, for humans reading the baseline JSON — no code
	// path reads it back (min_band gates on the current run's band).
	Band string `json:"band"`
	// ScoreVersion is the scorer formula version (coupling.ScoreVersion) the
	// snapshot was computed under. Ordinal reassignment makes scores
	// incomparable across versions, so CouplingScore refuses to anchor
	// max_drop on a mismatched snapshot. Empty in baselines written before
	// version tracking — treated as stale (re-baseline to re-anchor).
	ScoreVersion string `json:"score_version,omitempty"`
}

// Baseline is the on-disk baseline file structure.
type Baseline struct {
	SchemaVersion string                    `json:"schema_version"`
	Accepted      []AcceptedFinding         `json:"accepted"`
	Metrics       diagnostic.MetricSnapshot `json:"metrics"`
	// Score is the coupling_balance snapshot; omitted in baselines written
	// before score tracking or while the score was unmeasured.
	Score *ScoreSnapshot `json:"score,omitempty"`
}

// CouplingScore returns the stored coupling_balance value, or nil when the
// baseline carries no score snapshot or the snapshot was computed under a
// different scorer version — a cross-version drop is a methodology change,
// not a regression, so it must never anchor coupling.gate.max_drop.
func (b Baseline) CouplingScore() *int {
	if b.Score == nil || b.Score.ScoreVersion != coupling.ScoreVersion {
		return nil
	}
	return &b.Score.CouplingBalance
}

// ScoreVersionStale reports whether a score snapshot exists but was computed
// under a different scorer version than the current binary. Callers use it to
// disclose why max_drop was skipped instead of skipping silent.
func (b Baseline) ScoreVersionStale() bool {
	return b.Score != nil && b.Score.ScoreVersion != coupling.ScoreVersion
}

// HasFingerprint reports whether the given fingerprint exists in the baseline's
// accepted findings.
func (b Baseline) HasFingerprint(fingerprint string) bool {
	for _, a := range b.Accepted {
		if a.Fingerprint == fingerprint {
			return true
		}
	}
	return false
}

// Entries returns the accepted findings as status entries, satisfying
// status.AcceptedSet. The dependency points outward-in: the persistence
// layer adapts to the core's interface, never the reverse.
func (b Baseline) Entries() []status.AcceptedEntry {
	out := make([]status.AcceptedEntry, 0, len(b.Accepted))
	for _, a := range b.Accepted {
		out = append(out, status.AcceptedEntry{
			Fingerprint: a.Fingerprint,
			RuleID:      a.RuleID,
			Kind:        a.Kind,
			Severity:    a.Severity,
		})
	}
	return out
}

var _ status.AcceptedSet = Baseline{}

// Load reads the baseline from path. If the file does not exist, it returns an
// empty Baseline (not an error). If the file exists but has a mismatched
// schema_version, it returns an error (exit-3-style).
func Load(_ context.Context, path string) (Baseline, error) {
	data, err := os.ReadFile(path) //nolint:gosec // path comes from trusted CLI/config input
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return Baseline{}, nil
		}
		return Baseline{}, fmt.Errorf("baseline: read %s: %w", path, err)
	}

	var b Baseline
	if err := json.Unmarshal(data, &b); err != nil {
		return Baseline{}, fmt.Errorf("baseline: parse %s: %w", path, err)
	}

	if b.SchemaVersion != SchemaVersion {
		return Baseline{}, fmt.Errorf("baseline: schema version mismatch in %s: got %q, want %q", path, b.SchemaVersion, SchemaVersion)
	}

	return b, nil
}

// Save writes b to path as JSON. It sets SchemaVersion before writing.
func Save(_ context.Context, path string, b Baseline) error {
	b.SchemaVersion = SchemaVersion

	// Ensure non-nil slices so JSON serializes as [] not null.
	if b.Accepted == nil {
		b.Accepted = []AcceptedFinding{}
	}
	if b.Metrics == nil {
		b.Metrics = diagnostic.MetricSnapshot{}
	}

	data, err := json.MarshalIndent(b, "", "  ")
	if err != nil {
		return fmt.Errorf("baseline: marshal: %w", err)
	}

	if err := os.WriteFile(path, data, 0o600); err != nil {
		return fmt.Errorf("baseline: write %s: %w", path, err)
	}

	return nil
}
