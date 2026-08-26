// Package baseline handles loading and saving the archfit baseline file,
// which tracks accepted findings, a metric snapshot, and the architecture-state
// reference a later run compares against.
package baseline

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/alexei-led/archfit/internal/assessment/status"
	"github.com/alexei-led/archfit/internal/model/report"
)

// SchemaVersion is the schema_version this binary writes. Schema v2 stores the
// architecture-state reference (fingerprints, hard-gate findings, seams, and
// dimension snapshots) and no repository scalar.
const SchemaVersion = "archfit.baseline.v2"

// LegacySchemaVersion is the pre-state baseline. It stays READABLE — its
// accepted finding fingerprints are still the user's acceptance decisions — but
// its scalar score snapshot is ignored and it can never support a
// state/dimension/seam comparison. It is never auto-rewritten: a rewrite would
// silently upgrade a file the owner never re-reviewed.
const LegacySchemaVersion = "archfit.baseline.v1"

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

// ScoreSnapshot is the retired v1 scalar snapshot. It is decoded so a v1 file
// round-trips through Load without data loss and is never written back: schema
// v2 retired the scalar gate, so a stored repository score anchors nothing.
type ScoreSnapshot struct {
	CouplingBalance int    `json:"coupling_balance"`
	Band            string `json:"band"`
	ScoreVersion    string `json:"score_version,omitempty"`
	RubricVersion   int    `json:"rubric_version,omitempty"`
}

// CoverageSnapshot is a stored dimension's denominator.
type CoverageSnapshot struct {
	Basis    string `json:"basis"`
	Observed int    `json:"observed"`
	Total    int    `json:"total"`
}

// MetricSnapshotValue is one stored dimension metric. Only the three fields a
// later delta needs are persisted: a stored provenance list would grow the file
// without ever being compared.
type MetricSnapshotValue struct {
	Name  string  `json:"name"`
	Value float64 `json:"value"`
	Unit  string  `json:"unit"`
}

// DimensionSnapshot is one architecture-state dimension as the reference
// recorded it.
type DimensionSnapshot struct {
	Name     string                `json:"name"`
	Status   string                `json:"status"`
	Gate     string                `json:"gate"`
	Coverage CoverageSnapshot      `json:"coverage"`
	Metrics  []MetricSnapshotValue `json:"metrics"`
}

// StateSnapshot is the architecture-state reference a v2 baseline stores.
//
// The four fingerprints are stored together with the facts they qualify: a
// dimension or seam delta may be claimed only when all four still match, so a
// reader can tell a code change from a policy change without guessing.
type StateSnapshot struct {
	ConfigHash    string `json:"config_hash"`
	ModelHash     string `json:"model_hash"`
	LabelsHash    string `json:"labels_hash"`
	RubricVersion string `json:"rubric_version"`
	// HardGateFindingIDs are the reference's active blocker IDs, so a later run
	// can name which blockers are new without re-deriving them from the
	// accepted set (which mixes gates and advisories).
	HardGateFindingIDs []string `json:"hard_gate_finding_ids"`
	// QualifyingSeamIDs are the reference's distributed-monolith seams. Absent
	// (not empty) in any baseline written before the seam ledger existed, which
	// is why a pre-state file can never be read as "there were no seams then".
	QualifyingSeamIDs []string            `json:"qualifying_seam_ids"`
	Dimensions        []DimensionSnapshot `json:"dimensions"`
}

// Baseline is the on-disk baseline file structure.
type Baseline struct {
	SchemaVersion string                `json:"schema_version"`
	Accepted      []AcceptedFinding     `json:"accepted"`
	Metrics       report.MetricSnapshot `json:"metrics"`
	// Score is the retired v1 scalar snapshot; present only in a legacy file.
	Score *ScoreSnapshot `json:"score,omitempty"`
	// State is the architecture-state reference. Absent in a legacy file.
	State *StateSnapshot `json:"state,omitempty"`
}

// Legacy reports that this baseline predates the architecture-state contract.
func (b Baseline) Legacy() bool { return b.SchemaVersion == LegacySchemaVersion }

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

// Load reads the baseline from path. A missing file returns an empty Baseline
// (not an error). Both the current and the legacy schema load; any other
// schema_version is an error. Load never writes: a legacy file stays a legacy
// file until its owner re-baselines deliberately.
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

	if b.SchemaVersion != SchemaVersion && b.SchemaVersion != LegacySchemaVersion {
		return Baseline{}, fmt.Errorf("baseline: schema version mismatch in %s: got %q, want %q or %q",
			path, b.SchemaVersion, SchemaVersion, LegacySchemaVersion)
	}

	return b, nil
}

// Save writes b to path as the current schema. The retired scalar snapshot is
// dropped rather than carried forward: a v2 file that still stored a repository
// score would invite a consumer to gate on it again.
func Save(_ context.Context, path string, b Baseline) error {
	b.SchemaVersion = SchemaVersion
	b.Score = nil

	// Ensure non-nil slices so JSON serializes as [] not null.
	if b.Accepted == nil {
		b.Accepted = []AcceptedFinding{}
	}
	if b.Metrics == nil {
		b.Metrics = report.MetricSnapshot{}
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
