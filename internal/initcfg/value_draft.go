package initcfg

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"

	"github.com/goccy/go-yaml"
)

// Draft status values shared by the single-scalar draft files (owner, volatility).
const (
	DraftStatusDraft    = "draft"
	DraftStatusApproved = "approved"
)

// ValueDraft is one LLM-suggested single-scalar module annotation awaiting human
// review. Owner and volatility share this shape (module → one value); the
// containing ValueDraftFile.Field records which module field the value targets.
// This mirrors SubdomainDraft but for fields that carry exactly one scalar.
type ValueDraft struct {
	Module       string   `yaml:"module"`
	Value        string   `yaml:"value"`
	Rationale    string   `yaml:"rationale"`
	EvidenceRefs []string `yaml:"evidence_refs,omitempty"`
	Basis        string   `yaml:"basis,omitempty"` // deterministic_fact or semantic_judgment
	Status       string   `yaml:"status"`          // DraftStatusDraft or DraftStatusApproved
	// Confidence records how trustworthy the judgment is (high|medium|low).
	// Set by the LLM draft path; empty = unset (treated as high for human-approved entries).
	Confidence string `yaml:"confidence,omitempty"`
	// Provenance records the source of the judgment (human|llm|tool).
	// Set to "llm" by the LLM draft path; empty = unset (treated as human).
	Provenance string `yaml:"provenance,omitempty"`
}

// ValueDraftFile is the top-level container for .archfit-owners.yaml /
// .archfit-volatility.yaml.
type ValueDraftFile struct {
	Version int          `yaml:"version"`
	Field   string       `yaml:"field"` // "owner" | "volatility"
	Drafts  []ValueDraft `yaml:"drafts"`
}

// LoadValueDrafts loads the draft file from path. Returns an empty file
// (version 1, given field, no drafts) when the file is absent.
func LoadValueDrafts(path, field string) (ValueDraftFile, error) {
	data, err := os.ReadFile(path) //#nosec G304
	if err != nil {
		if os.IsNotExist(err) {
			return ValueDraftFile{Version: 1, Field: field}, nil
		}
		return ValueDraftFile{}, fmt.Errorf("initcfg: load %s drafts: %w", field, err)
	}
	var f ValueDraftFile
	if err := yaml.Unmarshal(data, &f); err != nil {
		return ValueDraftFile{}, fmt.Errorf("initcfg: parse %s drafts: %w", field, err)
	}
	if f.Version == 0 {
		f.Version = 1
	}
	if f.Field == "" {
		f.Field = field
	}
	return f, nil
}

// WriteValueDrafts writes the draft file atomically (temp + rename).
func WriteValueDrafts(path string, f ValueDraftFile) error {
	data, err := yaml.Marshal(f)
	if err != nil {
		return fmt.Errorf("initcfg: marshal %s drafts: %w", f.Field, err)
	}
	dir := filepath.Dir(path)
	tmp, err := os.CreateTemp(dir, ".valuedraft-*")
	if err != nil {
		return fmt.Errorf("initcfg: write %s drafts: %w", f.Field, err)
	}
	defer os.Remove(tmp.Name()) //nolint:errcheck // best-effort cleanup on error paths
	if _, err := tmp.Write(data); err != nil {
		_ = tmp.Close()
		return fmt.Errorf("initcfg: write %s drafts: %w", f.Field, err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("initcfg: write %s drafts: %w", f.Field, err)
	}
	if err := os.Rename(tmp.Name(), path); err != nil {
		return fmt.Errorf("initcfg: write %s drafts: %w", f.Field, err)
	}
	return nil
}

// MergeValueDrafts merges newDrafts into existing:
//   - Approved entries are untouchable.
//   - An existing draft for the same module is replaced by the new draft.
//   - New modules not in existing are appended.
//   - Output is sorted by Module for a deterministic file.
func MergeValueDrafts(existing ValueDraftFile, newDrafts []ValueDraft) ValueDraftFile {
	byModule := make(map[string]ValueDraft, len(existing.Drafts))
	for _, d := range existing.Drafts {
		byModule[d.Module] = d
	}
	for _, d := range newDrafts {
		cur, ok := byModule[d.Module]
		if ok && cur.Status == DraftStatusApproved {
			continue // approved entries are untouchable
		}
		byModule[d.Module] = d
	}

	merged := make([]ValueDraft, 0, len(byModule))
	for _, d := range byModule {
		merged = append(merged, d)
	}
	sort.Slice(merged, func(i, j int) bool {
		return merged[i].Module < merged[j].Module
	})

	return ValueDraftFile{Version: existing.Version, Field: existing.Field, Drafts: merged}
}
