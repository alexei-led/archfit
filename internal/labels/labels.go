// Package labels holds the pure logic for pinned coupling-strength labels —
// the human-reviewed output of `archfit enrich`. The gate (check) consumes
// APPROVED labels deterministically; DRAFT labels are inert. The package is
// deliberately LLM-free AND I/O-free: reading the labels file from disk lives in
// the internal/labels/labelsio adapter, so the os + YAML dependency never rides
// into the engine's import closure. The engine imports only this pure package
// (the arch ring test forbids internal/llm and internal/labels/labelsio here).
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
	"sort"
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

// ValidStrength reports whether s is a Balanced Coupling strength a label may
// pin (contract, model, functional, intrusive).
func ValidStrength(s string) bool {
	_, ok := validStrengths[s]
	return ok
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
