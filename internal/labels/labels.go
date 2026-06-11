// Package labels loads and validates pinned coupling-strength labels —
// the human-reviewed output of `archfit enrich`. The gate (check) consumes
// APPROVED labels deterministically; DRAFT labels are inert. The package is
// deliberately LLM-free: it reads plain YAML, so the verdict never depends on
// a model call (the arch ring test forbids internal/llm here).
//
// A label refines the integration strength of all edges between one ordered
// module pair (model-vs-functional is the usual refinement; the deterministic
// heuristic blanket-labels most call edges "functional"). Precedence in
// classify: config public/internal globs > approved labels > extractor hint.
//
// Each label carries an evidence hash — a content hash of the import-graph
// edges between the module pair at enrich time (config-module namespace, so
// it works on every run, no SCIP required). On full runs a mismatch means the
// dependency surface changed since the label was reviewed: the label is
// ignored and a labels/stale advisory is emitted. Delta runs see a partial
// graph and skip the freshness check; human approval is the authority there.
package labels

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"sort"

	"github.com/goccy/go-yaml"
)

// Status values for a label.
const (
	StatusDraft    = "draft"
	StatusApproved = "approved"
)

// validStrengths are the Balanced Coupling strengths a label may pin.
var validStrengths = map[string]struct{}{
	"contract": {}, "model": {}, "functional": {}, "intrusive": {},
}

// Label pins the integration strength for all edges between an ordered module
// pair. Only Status == approved is consumed by the gate.
type Label struct {
	From         string `yaml:"from"`
	To           string `yaml:"to"`
	Strength     string `yaml:"strength"`
	Rationale    string `yaml:"rationale"`
	EvidenceHash string `yaml:"evidence_hash"`
	Status       string `yaml:"status"`
}

// File is the on-disk shape of .archfit-labels.yaml.
type File struct {
	Version int     `yaml:"version"`
	Labels  []Label `yaml:"labels"`
}

// Key returns the lookup key for an ordered module pair.
func Key(fromModule, toModule string) string {
	return fromModule + "\x00" + toModule
}

// Load reads and strictly validates a labels file. A missing file is not an
// error — it returns (nil, nil): labels are optional. Any malformed entry is
// an error: a half-read labels file must never silently alter the gate.
func Load(path string) ([]Label, error) {
	data, err := os.ReadFile(path) //nolint:gosec // path is the caller's config-relative labels file
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}

	var f File
	if err := yaml.Unmarshal(data, &f); err != nil {
		return nil, fmt.Errorf("labels: parse %s: %w", path, err)
	}
	for i, l := range f.Labels {
		if l.From == "" || l.To == "" {
			return nil, fmt.Errorf("labels: entry %d: from and to are required", i)
		}
		if _, ok := validStrengths[l.Strength]; !ok {
			return nil, fmt.Errorf("labels: entry %d (%s -> %s): invalid strength %q", i, l.From, l.To, l.Strength)
		}
		if l.Status != StatusDraft && l.Status != StatusApproved {
			return nil, fmt.Errorf("labels: entry %d (%s -> %s): invalid status %q (draft|approved)", i, l.From, l.To, l.Status)
		}
	}
	return f.Labels, nil
}

// HashItems returns the canonical evidence hash for a set of evidence items
// (the engine uses "fromPath\x00toPath\x00kind" per import-graph edge of the
// module pair; enrich writes the same). Sorted before hashing — deterministic
// regardless of input order.
func HashItems(items []string) string {
	sorted := make([]string, len(items))
	copy(sorted, items)
	sort.Strings(sorted)

	h := sha256.New()
	for _, it := range sorted {
		h.Write([]byte(it))
		h.Write([]byte{'\n'})
	}
	return hex.EncodeToString(h.Sum(nil))
}

// Approved partitions labels into the consumable map (Key(from,to) → strength)
// and the stale list. Draft labels are skipped entirely.
//
// evidence maps Key(from,to) → current evidence hash (HashItems over the
// pair's import-graph edges). Freshness: a label whose EvidenceHash does not
// match existing evidence is STALE — not applied. A label with an empty
// EvidenceHash (hand-authored), or whose pair has no current evidence (the
// edges are gone — the label is moot), or when evidence is nil (delta run:
// partial graph), applies/passes without the check.
func Approved(lbls []Label, evidence map[string]string) (approved map[string]string, stale []Label) {
	approved = map[string]string{}
	for _, l := range lbls {
		if l.Status != StatusApproved {
			continue
		}
		if current, ok := evidence[Key(l.From, l.To)]; ok && l.EvidenceHash != "" && l.EvidenceHash != current {
			stale = append(stale, l)
			continue
		}
		approved[Key(l.From, l.To)] = l.Strength
	}
	return approved, stale
}
