// Package labelsio is the I/O adapter for pinned coupling labels: it reads and
// validates .archfit-labels.yaml from disk. It is split out from internal/labels
// (pure logic) so the os + YAML dependency does NOT ride into the engine's import
// closure — the engine consumes the pure helpers in internal/labels and never
// imports this package. The boundary is enforced by internal/arch_test.go and the
// engine_no_labelsio gate rule.
package labelsio

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/goccy/go-yaml"

	"github.com/alexei-led/archfit/internal/application"
	"github.com/alexei-led/archfit/internal/relationship/labels"
)

// Loader is the concrete filesystem adapter used by the technical pipeline.
type Loader struct{}

// ApplicationStore adapts the labels file to application review DTOs.
type ApplicationStore struct{}

// Load reads application label DTOs from disk.
func (ApplicationStore) Load(_ context.Context, path string) ([]application.EnrichmentLabel, error) {
	in, err := Load(path)
	if err != nil {
		return nil, err
	}
	out := make([]application.EnrichmentLabel, len(in))
	for i, l := range in {
		out[i] = application.EnrichmentLabel{From: l.From, To: l.To, Strength: l.Strength, Rationale: l.Rationale, EvidenceRefs: append([]string(nil), l.EvidenceRefs...), Basis: l.Basis, EvidenceHash: l.EvidenceHash, Status: l.Status, Confidence: l.Confidence, Provenance: l.Provenance}
	}
	return out, nil
}

// Save writes application label DTOs to disk.
func (ApplicationStore) Save(_ context.Context, path string, in []application.EnrichmentLabel) error {
	out := make([]labels.Label, len(in))
	for i, l := range in {
		out[i] = labels.Label{From: l.From, To: l.To, Strength: l.Strength, Rationale: l.Rationale, EvidenceRefs: append([]string(nil), l.EvidenceRefs...), Basis: l.Basis, EvidenceHash: l.EvidenceHash, Status: l.Status, Confidence: l.Confidence, Provenance: l.Provenance}
	}
	return Write(path, out)
}

// Load reads labels from path.
func (Loader) Load(path string) ([]labels.Label, error) { return Load(path) }

// Write atomically writes the labels file using the established YAML schema.
func Write(path string, in []labels.Label) error {
	data, err := yaml.Marshal(labels.File{Version: 1, Labels: in})
	if err != nil {
		return err
	}
	tmp, err := os.CreateTemp(filepath.Dir(path), ".labels-*")
	if err != nil {
		return err
	}
	defer os.Remove(tmp.Name()) //nolint:errcheck // best-effort cleanup on error paths
	if _, err := tmp.Write(data); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return os.Rename(tmp.Name(), path)
}

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
	// Shape checks only: labelsio has no module map, so module existence is
	// checked where the labels meet the config (acquisition).
	if err := labels.Validate(f.Labels, nil); err != nil {
		return nil, fmt.Errorf("%w (%s)", err, path)
	}
	return f.Labels, nil
}
