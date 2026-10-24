package scip

import (
	"encoding/json"
	"fmt"
	"slices"
	"sort"
	"strings"

	"github.com/alexei-led/archfit/v3/internal/model/evidence"
)

// rustPackageResult adds package-edge aliases to mapped module facts, not to the
// symbol graph or cached reader JSON. Rebuilding from required metadata on both
// cache paths preserves deep ownership and the reader's actual observed counts.
func rustPackageResult(ro pipelineResult, cov evidence.Coverage, metadata []byte) (pipelineResult, evidence.Coverage, bool) {
	if ro.indexer != indexerRust {
		return ro, cov, true
	}
	owners, issues, err := rustTargetPackageOwners(metadata)
	var out readerOutput
	if err == nil {
		err = json.Unmarshal(ro.raw, &out)
	}
	if err != nil {
		cov.Status, cov.Reason = evidence.StatusPartial, "Rust package aliases: "+err.Error()
		return ro, cov, false
	}
	ro.packageEdges, issues = rustObservedPackageEdges(out.Edges, owners, issues)
	if len(issues) > 0 {
		sort.Strings(issues)
		cov.Status = evidence.StatusPartial
		if cov.Reason != "" {
			cov.Reason += "; "
		}
		cov.Reason += "Rust package aliases partial: " + strings.Join(issues, ", ")
	}
	return ro, cov, true
}

// Only a unique workspace package may own a target prefix. A package alias that
// overlaps another package's target namespace is also unsafe in the shared map.
func rustTargetPackageOwners(metadata []byte) (map[string]string, []string, error) {
	var meta struct {
		Members  []string `json:"workspace_members"`
		Packages []struct {
			ID      string `json:"id"`
			Name    string `json:"name"`
			Targets []struct {
				Name string `json:"name"`
			} `json:"targets"`
		} `json:"packages"`
	}
	if err := json.Unmarshal(metadata, &meta); err != nil {
		return nil, nil, fmt.Errorf("invalid metadata: %w", err)
	}
	members := make(map[string]bool, len(meta.Members))
	for _, id := range meta.Members {
		members[id] = true
	}
	names := make(map[string]string)
	packages := make(map[string]string)
	targets := make(map[string]string)
	blockedNames := make(map[string]bool)
	var issues []string
	for _, p := range meta.Packages {
		if !members[p.ID] {
			continue
		}
		packages[p.ID] = p.Name
		if prior, ok := names[p.Name]; ok && prior != p.ID {
			blockedNames[p.Name] = true
			issues = append(issues, "ambiguous package "+p.Name)
		}
		names[p.Name] = p.ID
		for _, target := range p.Targets {
			crate := strings.ReplaceAll(target.Name, "-", "_")
			if prior, ok := targets[crate]; ok && prior != p.ID {
				targets[crate] = ""
			} else if !ok {
				targets[crate] = p.ID
			}
		}
	}
	for crate, id := range targets {
		if id == "" {
			blockedNames[crate] = true
			issues = append(issues, "ambiguous target crate "+crate)
		} else if packageID, ok := names[crate]; ok && packageID != id {
			blockedNames[crate] = true
			targets[crate] = ""
			issues = append(issues, "package/target namespace overlap "+crate)
		}
	}
	owners := make(map[string]string, len(targets))
	for crate, id := range targets {
		if id != "" && !blockedNames[packages[id]] {
			owners[crate] = packages[id]
		}
	}
	return owners, issues, nil
}

func rustObservedPackageEdges(edges []readerEdge, owners map[string]string, issues []string) ([]readerEdge, []string) {
	original := make(map[string]readerEdge, len(edges))
	aggregates := make(map[string]readerEdge)
	for _, edge := range edges {
		original[edge.From+"\x00"+edge.To] = edge
		from, _, _ := strings.Cut(edge.From, "::")
		to, _, _ := strings.Cut(edge.To, "::")
		from, to = owners[from], owners[to]
		// A declared Cargo dependency alone is never evidence of integration.
		if from == "" || to == "" || from == to || scipStrengthRank(edge.Strength) == 0 {
			continue
		}
		key := from + "\x00" + to
		aggregate := aggregates[key]
		aggregate.From, aggregate.To = from, to
		if scipStrengthRank(edge.Strength) > scipStrengthRank(aggregate.Strength) {
			aggregate.Strength = edge.Strength
		}
		if (edge.DataStrength == "contract" || edge.DataStrength == "model") && scipStrengthRank(edge.DataStrength) > scipStrengthRank(aggregate.DataStrength) {
			aggregate.DataStrength = edge.DataStrength
		}
		kinds := make(map[string]bool)
		for _, kind := range append(slices.Clone(aggregate.Connascence), edge.Connascence...) {
			kinds[kind] = true
		}
		aggregate.Connascence = nil
		for _, kind := range []string{"name", "type", "meaning", "algorithm", "position"} {
			if kinds[kind] {
				aggregate.Connascence = append(aggregate.Connascence, kind)
			}
		}
		aggregates[key] = aggregate
	}
	keys := make([]string, 0, len(aggregates))
	for key := range aggregates {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	aliases := make([]readerEdge, 0, len(keys))
	for _, key := range keys {
		aggregate := aggregates[key]
		if deep, exists := original[key]; exists {
			// One string key cannot express two different abstraction levels.
			// Preserve the real module fact, and disclose any lost aggregation.
			if deep.Strength != aggregate.Strength || deep.DataStrength != aggregate.DataStrength || !slices.Equal(deep.Connascence, aggregate.Connascence) {
				issues = append(issues, "aggregate overlaps module edge "+aggregate.From+" -> "+aggregate.To)
			}
			continue
		}
		aliases = append(aliases, aggregate)
	}
	return aliases, issues
}

// Match the reader's observed strength ordering; never infer from declarations.
func scipStrengthRank(strength string) int {
	switch strength {
	case "intrusive":
		return 4
	case "functional":
		return 3
	case "model":
		return 2
	case "contract":
		return 1
	default:
		return 0
	}
}
