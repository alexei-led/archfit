package modularity

import (
	"fmt"
	"sort"
	"strings"

	"github.com/alexei-led/archfit/internal/metrics/internal/result"
	"github.com/alexei-led/archfit/internal/model/diagnostic"
	"github.com/alexei-led/archfit/internal/model/signal"
)

// StructFieldDensityMetric reports estimated field counts for exported struct
// declarations extracted by ast-grep. It surfaces potential god-structs by field
// count. Report-only — Band: BandInformational, never gates.
// n/a when no struct_field syntax facts are present (sg absent or no Go/Rust code).
//
// Field count source: SyntaxFact.Count (line-range heuristic from the
// go-struct-field/rs-struct-field rules: endLine - startLine - 1).
// Ceiling: tuple structs, unit structs, and multi-line field declarations may
// diverge from the estimate.
type StructFieldDensityMetric struct{}

// Name returns "struct_field_density".
func (m StructFieldDensityMetric) Name() string { return "struct_field_density" }

// Version returns "struct_field_density.v1".
func (m StructFieldDensityMetric) Version() string { return "struct_field_density.v1" }

type structEntry struct {
	key   string // "module::StructName" or "file::StructName"
	count int
}

// Calculate aggregates struct field counts from struct_field facts.
// One fact per struct; Count = estimated field count from line range.
func (m StructFieldDensityMetric) Calculate(in signal.CommonInput) diagnostic.MetricResult {
	const def = "estimated struct field counts (line-range heuristic) — report-only; signals potential god-structs"
	if len(in.SyntaxFacts) == 0 {
		return result.NACount(m.Name(), m.Version(), def)
	}

	// Collect max field count per (module-or-file, struct-name).
	// "max" because the same struct name can appear in different files in the same
	// module; we surface the highest observed count.
	type structKey struct{ mod, name string }
	maxCounts := make(map[structKey]int)
	for _, f := range in.SyntaxFacts {
		if f.Kind != "struct_field" {
			continue
		}
		mod := f.Module
		if mod == "" {
			mod = f.File
		}
		k := structKey{mod: mod, name: f.Name}
		if f.Count > maxCounts[k] {
			maxCounts[k] = f.Count
		}
	}
	if len(maxCounts) == 0 {
		return result.NACount(m.Name(), m.Version(), def)
	}

	entries := make([]structEntry, 0, len(maxCounts))
	total := 0
	for k, c := range maxCounts {
		entries = append(entries, structEntry{key: k.mod + "::" + k.name, count: c})
		total += c
	}
	sort.Slice(entries, func(i, j int) bool {
		if entries[i].count != entries[j].count {
			return entries[i].count > entries[j].count
		}
		return entries[i].key < entries[j].key
	})

	confidence := result.ConfidenceHigh
	if len(entries) < result.ModularitySmallN {
		confidence = result.ConfidenceLow
	}
	return diagnostic.MetricResult{
		Name:       m.Name(),
		Value:      float64(total),
		Display:    structFieldDisplay(entries),
		Band:       result.BandInformational,
		Confidence: confidence,
		Version:    m.Version(),
		Mode:       result.ModeCount,
		Definition: def,
	}
}

func structFieldDisplay(entries []structEntry) string {
	if len(entries) == 0 {
		return "0 struct(s)"
	}
	var b strings.Builder
	fmt.Fprintf(&b, "%d struct(s), top by field count: ", len(entries))
	for i, e := range entries {
		if i == 5 {
			fmt.Fprintf(&b, "+%d more", len(entries)-5)
			break
		}
		if i > 0 {
			b.WriteString(", ")
		}
		fmt.Fprintf(&b, "%s (%d fields)", e.key, e.count)
	}
	return b.String()
}
