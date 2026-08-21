// Package baseline handles loading and saving the archfit baseline file,
// which tracks accepted findings and metric snapshots across runs.
package baseline

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/alexei-led/archfit/internal/model/coupling"
	"github.com/alexei-led/archfit/internal/model/diagnostic"
	"github.com/alexei-led/archfit/internal/score"
	"github.com/alexei-led/archfit/internal/status"
)

// SchemaVersion is the fixed schema_version value for baseline files.
const SchemaVersion = "archfit.baseline.v1"

// legacyRubricVersion is the rubric a score snapshot written before rubric
// tracking was recorded under. Rubric 1 is the only rubric shipped so far, so a
// legacy snapshot stays comparable with the current binary instead of reading
// as incompatible.
const legacyRubricVersion = 1

// Score-snapshot input names reported by ScoreSnapshotMismatches. They are the
// snapshot's own JSON field names so a disclosure can point at the stored value.
const (
	InputScoreVersion  = "score_version"
	InputRubricVersion = "rubric_version"
)

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
	// RubricVersion is the scorecard rubric (score.RubricVersion) the snapshot
	// was banded under. A rubric change re-cuts the band edges, so the stored
	// value is no longer the same measurement. Absent in baselines written
	// before rubric tracking — read as rubric 1, the only rubric shipped so far,
	// so a legacy snapshot keeps anchoring without a forced re-baseline.
	RubricVersion int `json:"rubric_version,omitempty"`
}

// rubricVersion returns the rubric the snapshot was banded under, defaulting a
// missing value to the pre-tracking rubric.
func (s ScoreSnapshot) rubricVersion() int {
	if s.RubricVersion == 0 {
		return legacyRubricVersion
	}
	return s.RubricVersion
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
// baseline carries no score snapshot or the snapshot is incompatible with the
// current binary — a cross-version drop is a methodology change, not a
// regression, so it must never anchor coupling.gate.max_drop.
func (b Baseline) CouplingScore() *int {
	if b.Score == nil || len(b.ScoreSnapshotMismatches()) > 0 {
		return nil
	}
	return &b.Score.CouplingBalance
}

// ScoreSnapshotMismatches names the stored score-snapshot inputs that differ
// from the current binary's, in stable order. Empty when no snapshot exists
// (nothing to disclose) or when the snapshot still anchors max_drop. Callers
// use it to say WHICH input made the snapshot incomparable instead of skipping
// the drop check silently.
func (b Baseline) ScoreSnapshotMismatches() []string {
	if b.Score == nil {
		return nil
	}
	var out []string
	if b.Score.ScoreVersion != coupling.ScoreVersion {
		out = append(out, InputScoreVersion)
	}
	if b.Score.rubricVersion() != score.RubricVersion {
		out = append(out, InputRubricVersion)
	}
	return out
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

	// Temp-file + rename: a crash mid-write must not leave a truncated baseline
	// that hard-fails every subsequent run's Load.
	tmp, err := os.CreateTemp(filepath.Dir(path), ".archfit-baseline-*")
	if err != nil {
		return fmt.Errorf("baseline: write %s: %w", path, err)
	}
	tmpName := tmp.Name()
	if _, err := tmp.Write(data); err != nil {
		_ = tmp.Close()
		_ = os.Remove(tmpName)
		return fmt.Errorf("baseline: write %s: %w", path, err)
	}
	if err := tmp.Close(); err != nil {
		_ = os.Remove(tmpName)
		return fmt.Errorf("baseline: write %s: %w", path, err)
	}
	if err := os.Rename(tmpName, path); err != nil {
		_ = os.Remove(tmpName)
		return fmt.Errorf("baseline: write %s: %w", path, err)
	}

	return nil
}
