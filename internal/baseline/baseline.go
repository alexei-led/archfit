// Package baseline handles loading and saving the archfit baseline file,
// which tracks accepted findings and metric snapshots across runs.
package baseline

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"

	"github.com/alexei-led/archfit/internal/model/diagnostic"
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
}

// Baseline is the on-disk baseline file structure.
type Baseline struct {
	SchemaVersion string                    `json:"schema_version"`
	Accepted      []AcceptedFinding         `json:"accepted"`
	Metrics       diagnostic.MetricSnapshot `json:"metrics"`
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
