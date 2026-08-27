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
// classify depends on provenance: human/tool labels are a reviewer's verdict
// (config public/internal globs > approved labels > extractor hint), while
// llm-provenance labels only fill cells every static source left unknown
// (globs and hints beat them — compiler-grade beats LLM).
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
	"fmt"
	"sort"
	"strings"
)

// Status values for a label.
const (
	StatusDraft    = "draft"
	StatusApproved = "approved"
)

// Provenance values indicate the source of a label's judgment.
const (
	ProvenanceHuman = "human"
	ProvenanceLLM   = "llm"
	ProvenanceTool  = "tool"
)

// Confidence values indicate how trustworthy a label's judgment is.
const (
	ConfidenceHigh   = "high"
	ConfidenceMedium = "medium"
	ConfidenceLow    = "low"
)

// validStrengths are the Balanced Coupling strengths a label may pin.
var validStrengths = map[string]struct{}{
	"contract": {}, "model": {}, "functional": {}, "symmetric": {}, "intrusive": {},
}

// ValidStrength reports whether s is a Balanced Coupling strength a label may
// pin (contract, model, functional, symmetric, intrusive).
func ValidStrength(s string) bool {
	_, ok := validStrengths[s]
	return ok
}

// Label pins the integration strength for all edges between an ordered module
// pair. Only Status == approved is consumed by the gate.
type Label struct {
	From         string   `yaml:"from"`
	To           string   `yaml:"to"`
	Strength     string   `yaml:"strength"`
	Rationale    string   `yaml:"rationale"`
	EvidenceRefs []string `yaml:"evidence_refs,omitempty"`
	Basis        string   `yaml:"basis,omitempty"`
	EvidenceHash string   `yaml:"evidence_hash"`
	Status       string   `yaml:"status"`
	// Confidence records how trustworthy the strength judgment is (high|medium|low).
	// Empty means unset (treated as high for approved human labels). Optional — omitted
	// from YAML when unset so existing files remain valid.
	Confidence string `yaml:"confidence,omitempty"`
	// Provenance records the source of the judgment (human|llm|tool). Empty means
	// unset (treated as human). Optional — omitted from YAML when unset.
	Provenance string `yaml:"provenance,omitempty"`
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

// isEffective reports whether l is an approved, non-stale label.
// A label is stale when its stored EvidenceHash disagrees with the current
// evidence for the pair (same rule as Approved). Empty EvidenceHash (hand-authored),
// a pair with no current evidence (edges gone — label is moot), and a nil
// evidence map (delta run: partial graph) all pass without the check.
func isEffective(l Label, evidence map[string]string) bool {
	if l.Status != StatusApproved {
		return false
	}
	h, ok := evidence[Key(l.From, l.To)]
	return !ok || l.EvidenceHash == "" || l.EvidenceHash == h
}

// Approved partitions labels into two consumable maps (Key(from,to) → strength)
// — human-authority labels (provenance human/tool/unset) and llm-provenance
// labels — plus the stale list. Draft labels are skipped entirely.
//
// The split exists because the two provenances carry different classify
// precedence: a human label is a reviewer's verdict (beats the extractor hint,
// refines a public-glob contract floor), while an llm label only fills cells
// every static source left unknown — it never displaces a static
// classification (compiler-grade beats LLM, same rule as SCIP-for-Go).
//
// evidence maps Key(from,to) → current evidence hash (HashItems over the
// pair's import-graph edges). Freshness: a label whose EvidenceHash does not
// match existing evidence is STALE — not applied. A label with an empty
// EvidenceHash (hand-authored), or whose pair has no current evidence (the
// edges are gone — the label is moot), or when evidence is nil (delta run:
// partial graph), applies/passes without the check.
func Approved(lbls []Label, evidence map[string]string) (approved, llmApproved map[string]string, stale []Label) {
	approved = map[string]string{}
	llmApproved = map[string]string{}
	for _, l := range lbls {
		if !isEffective(l, evidence) {
			if l.Status == StatusApproved {
				stale = append(stale, l)
			}
			continue
		}
		if l.Provenance == ProvenanceLLM {
			llmApproved[Key(l.From, l.To)] = l.Strength
			continue
		}
		approved[Key(l.From, l.To)] = l.Strength
	}
	return approved, llmApproved, stale
}

// LLMApprovedCount returns the count of approved labels whose provenance is
// "llm" (and confidence is not "high"). These labels lower the consuming
// dimension's confidence by one band — they have been human-approved but the
// original judgment came from an LLM, not a human reading the code.
// Draft labels and stale labels (same freshness rules as Approved) are excluded.
func LLMApprovedCount(lbls []Label, evidence map[string]string) int {
	n := 0
	for _, l := range lbls {
		if !isEffective(l, evidence) {
			continue
		}
		if l.Provenance == ProvenanceLLM && l.Confidence != ConfidenceHigh {
			n++
		}
	}
	return n
}

// LLMConfidenceByKey returns confidence values for effective approved
// llm-provenance labels, keyed by Key(from,to). Draft and stale labels are
// excluded with the same freshness rules as Approved.
func LLMConfidenceByKey(lbls []Label, evidence map[string]string) map[string]string {
	out := map[string]string{}
	for _, l := range lbls {
		if !isEffective(l, evidence) {
			continue
		}
		if l.Provenance == ProvenanceLLM {
			out[Key(l.From, l.To)] = l.Confidence
		}
	}
	return out
}

// EvidenceHashByKey returns each effective label's OWN stored evidence hash,
// keyed by Key(from,to). It is not the hash computed for the current run: a
// hand-authored label is effective with no stored hash at all (isEffective
// skips freshness for it), and publishing the run's hash under a field
// documenting what the approval rested on would claim evidence nobody saw.
// An entry is absent, or present and empty, exactly when the label stored none.
func EvidenceHashByKey(lbls []Label, evidence map[string]string) map[string]string {
	out := map[string]string{}
	for _, l := range lbls {
		if !isEffective(l, evidence) {
			continue
		}
		out[Key(l.From, l.To)] = l.EvidenceHash
	}
	return out
}

// Validate checks the relational invariants of a label set: no self-pair, no
// duplicate ordered pair, and — when modules is non-empty — both endpoints
// declared.
//
// These are hard errors, not diagnostics. A label pins the integration strength
// of every edge in its module pair, so an unknown module or a duplicated pair
// means the file no longer describes a decision anyone can act on: the first is
// an override that silently applies to nothing, the second is two answers to
// one question with the winner decided by file order. Neither can produce a
// valid report, so both belong to the exit-3 class.
//
// modules is the set of declared module names. Pass nil to check only what does
// not need a module map — the shape checks still hold, so a labels file can be
// validated before a config is in hand.
func Validate(lbls []Label, modules map[string]struct{}) error {
	seen := make(map[string]int, len(lbls))
	for i, l := range lbls {
		if l.From == l.To {
			return fmt.Errorf("labels: entry %d (%s -> %s): a module cannot be labelled against itself", i, l.From, l.To)
		}
		key := Key(l.From, l.To)
		if first, dup := seen[key]; dup {
			return fmt.Errorf("labels: entry %d duplicates entry %d (%s -> %s): one module pair has one strength",
				i, first, l.From, l.To)
		}
		seen[key] = i
		if len(modules) == 0 {
			continue
		}
		for _, endpoint := range []string{l.From, l.To} {
			if _, ok := modules[endpoint]; !ok {
				return fmt.Errorf("labels: entry %d (%s -> %s): module %q is not declared in the config — the override would apply to nothing",
					i, l.From, l.To, endpoint)
			}
		}
	}
	return nil
}

// FileHash is the canonical fingerprint of a label set.
//
// Labels override integration strength, which moves seam severity and the
// distributed-monolith qualification. Two runs whose labels differ are
// therefore measuring under different rules, and a numerical delta between them
// would attribute a policy change to the code. Draft labels are inert and are
// deliberately excluded: adding one changes nothing measurable, so it must not
// invalidate a comparison.
//
// Entries are sorted by their ordered-pair key, so the hash is independent of
// file order.
func FileHash(lbls []Label) string {
	items := make([]string, 0, len(lbls))
	for _, l := range lbls {
		if l.Status != StatusApproved {
			continue
		}
		items = append(items, strings.Join([]string{
			l.From, l.To, l.Strength, l.EvidenceHash, l.Confidence, l.Provenance,
		}, "\x01"))
	}
	if len(items) == 0 {
		return ""
	}
	return HashItems(items)
}
