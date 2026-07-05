package initcfg

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"

	"github.com/goccy/go-yaml"
)

// Draft status values for SubdomainDraft.Status.
const (
	SubdomainStatusDraft    = "draft"
	SubdomainStatusApproved = "approved"
)

// SubdomainDraft is one LLM-suggested subdomain assignment awaiting human review.
type SubdomainDraft struct {
	Module       string   `yaml:"module"`
	Subdomain    string   `yaml:"subdomain"`  // llm draft: core|supporting|generic
	Volatility   string   `yaml:"volatility"` // llm draft: low|medium|high (may be empty)
	Rationale    string   `yaml:"rationale"`
	EvidenceRefs []string `yaml:"evidence_refs,omitempty"`
	Basis        string   `yaml:"basis,omitempty"` // deterministic_fact or semantic_judgment
	Status       string   `yaml:"status"`          // SubdomainStatusDraft or SubdomainStatusApproved
}

// SubdomainDraftFile is the top-level container for .archfit-subdomains.yaml.
type SubdomainDraftFile struct {
	Version int              `yaml:"version"`
	Drafts  []SubdomainDraft `yaml:"drafts"`
}

// LoadSubdomainDrafts loads the draft file from path.
// Returns an empty file (version 1, no drafts) when the file is absent.
func LoadSubdomainDrafts(path string) (SubdomainDraftFile, error) {
	data, err := os.ReadFile(path) //#nosec G304
	if err != nil {
		if os.IsNotExist(err) {
			return SubdomainDraftFile{Version: 1}, nil
		}
		return SubdomainDraftFile{}, fmt.Errorf("initcfg: load subdomain drafts: %w", err)
	}
	var f SubdomainDraftFile
	if err := yaml.Unmarshal(data, &f); err != nil {
		return SubdomainDraftFile{}, fmt.Errorf("initcfg: parse subdomain drafts: %w", err)
	}
	if f.Version == 0 {
		f.Version = 1
	}
	return f, nil
}

// WriteSubdomainDrafts writes the draft file atomically (temp + rename).
func WriteSubdomainDrafts(path string, f SubdomainDraftFile) error {
	data, err := yaml.Marshal(f)
	if err != nil {
		return fmt.Errorf("initcfg: marshal subdomain drafts: %w", err)
	}
	dir := filepath.Dir(path)
	tmp, err := os.CreateTemp(dir, ".subdomains-*")
	if err != nil {
		return fmt.Errorf("initcfg: write subdomain drafts: %w", err)
	}
	defer os.Remove(tmp.Name()) //nolint:errcheck // best-effort cleanup on error paths
	if _, err := tmp.Write(data); err != nil {
		_ = tmp.Close()
		return fmt.Errorf("initcfg: write subdomain drafts: %w", err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("initcfg: write subdomain drafts: %w", err)
	}
	if err := os.Rename(tmp.Name(), path); err != nil {
		return fmt.Errorf("initcfg: write subdomain drafts: %w", err)
	}
	return nil
}

// MergeSubdomainDrafts merges newDrafts into existing:
//   - Approved entries are untouchable.
//   - An existing draft for the same module is replaced by the new draft.
//   - New modules not in existing are appended.
//   - Output is sorted by Module for a deterministic file.
func MergeSubdomainDrafts(existing SubdomainDraftFile, newDrafts []SubdomainDraft) SubdomainDraftFile {
	byModule := make(map[string]SubdomainDraft, len(existing.Drafts))
	for _, d := range existing.Drafts {
		byModule[d.Module] = d
	}
	for _, d := range newDrafts {
		cur, ok := byModule[d.Module]
		if ok && cur.Status == SubdomainStatusApproved {
			continue // approved entries are untouchable
		}
		byModule[d.Module] = d
	}

	merged := make([]SubdomainDraft, 0, len(byModule))
	for _, d := range byModule {
		merged = append(merged, d)
	}
	sort.Slice(merged, func(i, j int) bool {
		return merged[i].Module < merged[j].Module
	})

	return SubdomainDraftFile{Version: existing.Version, Drafts: merged}
}
