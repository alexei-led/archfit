// Package labelsio is the I/O adapter for pinned coupling labels: it reads and
// validates .archfit-labels.yaml from disk. It is split out from internal/labels
// (pure logic) so the os + YAML dependency does NOT ride into the engine's import
// closure — the engine consumes the pure helpers in internal/labels and never
// imports this package. The boundary is enforced by internal/arch_test.go and the
// engine_no_labelsio gate rule.
package labelsio

import (
	"errors"
	"fmt"
	"os"

	"github.com/goccy/go-yaml"

	"github.com/alexei-led/archfit/internal/relationship/labels"
)

// Load reads and strictly validates a labels file. A missing file is not an
// error — it returns (nil, nil): labels are optional. Any malformed entry is
// an error: a half-read labels file must never silently alter the gate.
func Load(path string) ([]labels.Label, error) {
	data, err := os.ReadFile(path) //nolint:gosec // path is the caller's config-relative labels file
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}

	var f labels.File
	if err := yaml.Unmarshal(data, &f); err != nil {
		return nil, fmt.Errorf("labels: parse %s: %w", path, err)
	}
	for i, l := range f.Labels {
		if l.From == "" || l.To == "" {
			return nil, fmt.Errorf("labels: entry %d: from and to are required", i)
		}
		if !labels.ValidStrength(l.Strength) {
			return nil, fmt.Errorf("labels: entry %d (%s -> %s): invalid strength %q", i, l.From, l.To, l.Strength)
		}
		if l.Status != labels.StatusDraft && l.Status != labels.StatusApproved {
			return nil, fmt.Errorf("labels: entry %d (%s -> %s): invalid status %q (draft|approved)", i, l.From, l.To, l.Status)
		}
		if l.Confidence != "" &&
			l.Confidence != labels.ConfidenceHigh &&
			l.Confidence != labels.ConfidenceMedium &&
			l.Confidence != labels.ConfidenceLow {
			return nil, fmt.Errorf("labels: entry %d (%s -> %s): invalid confidence %q (high|medium|low)", i, l.From, l.To, l.Confidence)
		}
		if l.Provenance != "" &&
			l.Provenance != labels.ProvenanceHuman &&
			l.Provenance != labels.ProvenanceLLM &&
			l.Provenance != labels.ProvenanceTool {
			return nil, fmt.Errorf("labels: entry %d (%s -> %s): invalid provenance %q (human|llm|tool)", i, l.From, l.To, l.Provenance)
		}
	}
	return f.Labels, nil
}
