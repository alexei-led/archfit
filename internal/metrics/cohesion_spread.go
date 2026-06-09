package metrics

import (
	"fmt"
	"sort"
	"strings"

	"github.com/alexei-led/archfit/internal/model/diagnostic"
	"github.com/alexei-led/archfit/internal/model/symbol"
)

// cohesionSpreadTopN is the maximum number of files displayed in the output.
const cohesionSpreadTopN = 5

// cohesionSpreadLOCFloor is the minimum file LOC for a source file to be
// included in the spread ranking. Files below this threshold are typically
// thin dispatchers whose many imports carry no design signal.
// Default 150; tunable via Task 5 validation.
const cohesionSpreadLOCFloor = 150

// subsystem strips the last dotted segment from a module path to derive its
// parent subsystem (e.g. "ccgram.providers.base" → "ccgram.providers").
// If the path has no dot, the path itself is the subsystem (no collapse to "").
func subsystem(mod string) string {
	if i := strings.LastIndexByte(mod, '.'); i > 0 {
		return mod[:i]
	}
	return mod
}

// fileSpread computes, for each source file (a value of g.Module), the count of
// distinct target subsystems its symbols reference outbound. Only cross-module
// edges are counted (Refs entries already hold only cross-module edges).
// Returns nil when g is empty.
func fileSpread(g symbol.Graph) map[string]int {
	if g.Empty() {
		return nil
	}

	// file → set of distinct target subsystems.
	spreading := make(map[string]map[string]struct{})
	for from, tos := range g.Refs {
		fromFile := g.Module[from]
		if fromFile == "" {
			continue
		}
		for to := range tos {
			if from == to {
				continue
			}
			toFile := g.Module[to]
			if toFile == "" || fromFile == toFile {
				continue
			}
			ss := subsystem(toFile)
			if spreading[fromFile] == nil {
				spreading[fromFile] = make(map[string]struct{})
			}
			spreading[fromFile][ss] = struct{}{}
		}
	}

	if len(spreading) == 0 {
		return nil
	}

	result := make(map[string]int, len(spreading))
	for f, targets := range spreading {
		result[f] = len(targets)
	}
	return result
}

// CohesionSpreadMetric ranks source files by their outbound spread: the count
// of distinct target subsystems their symbols reference. A file that reaches into
// many unrelated subsystems likely bundles several unrelated concerns.
//
// This is DISTINCT from risk_hub (which measures inbound cross-module surface
// breadth) and structural_weight (which measures LOC only). A file can win
// cohesion_spread while having zero inbound references and therefore zero
// risk_hub score — proving it adds a genuinely different signal.
//
// Subsystem granularity: parent-package strip (last dotted segment removed).
// Filter: files with LOC < cohesionSpreadLOCFloor are excluded (thin dispatchers
// carry no design signal). LOC is secondary sort (desc) after spread.
//
// Band is always "info" (report-only); this metric never gates.
type CohesionSpreadMetric struct{}

// cohesionSpreadEntry holds per-file spread data for ranking and display.
type cohesionSpreadEntry struct {
	file   string
	spread int
	loc    int
}

// Name returns "cohesion_spread".
func (m CohesionSpreadMetric) Name() string { return "cohesion_spread" }

// Version returns "cohesion_spread.v1".
func (m CohesionSpreadMetric) Version() string { return "cohesion_spread.v1" }

const cohesionSpreadDef = "outbound spread: count of distinct target subsystems (parent-package strip) " +
	"each file's symbols reference; LOC floor 150; " +
	"distinct from risk_hub (inbound breadth) — never gates"

// Calculate ranks source files by outbound subsystem spread. Returns n/a when
// SymbolGraph is empty (SCIP off or indexer absent) — never a false zero.
func (m CohesionSpreadMetric) Calculate(in MetricInput) diagnostic.MetricResult {
	def := cohesionSpreadDef
	if in.SymbolGraph.Empty() {
		return naCount(m.Name(), m.Version(), def)
	}

	spread := fileSpread(in.SymbolGraph)
	if len(spread) == 0 {
		return naCount(m.Name(), m.Version(), def)
	}

	// Build per-module LOC lookup keyed the same way as g.Module values.
	// FileLOC keys are slash-paths (e.g. "ccgram/handlers/polling/directory_callbacks.py");
	// g.Module values for Python are dotted (e.g. "ccgram.handlers.polling.directory_callbacks").
	// fileToModuleKey converts FileLOC slash-paths to the same key format as g.Module.
	// We must detect the language; use dominantLanguage when in.Graph is present, else
	// infer from g.Module paths (dotted → python, slash → go).
	lang := inferSymbolLang(in)
	locByModule := buildLocByModule(in.FileLOC, lang)

	// Rank: spread desc, then LOC desc, then file name asc (deterministic total order).
	entries := make([]cohesionSpreadEntry, 0, len(spread))
	for file, s := range spread {
		loc := locByModule[file]
		if loc < cohesionSpreadLOCFloor {
			continue
		}
		entries = append(entries, cohesionSpreadEntry{file: file, spread: s, loc: loc})
	}

	if len(entries) == 0 {
		return naCount(m.Name(), m.Version(), def)
	}

	sort.Slice(entries, func(i, j int) bool {
		if entries[i].spread != entries[j].spread {
			return entries[i].spread > entries[j].spread
		}
		if entries[i].loc != entries[j].loc {
			return entries[i].loc > entries[j].loc
		}
		return entries[i].file < entries[j].file
	})

	confidence := confidenceHigh
	if len(entries) < modularitySmallN {
		confidence = confidenceLow
	}

	return diagnostic.MetricResult{
		Name:       m.Name(),
		Value:      float64(len(entries)),
		Display:    cohesionSpreadDisplay(entries, cohesionSpreadTopN),
		Band:       bandInformational,
		Confidence: confidence,
		Version:    m.Version(),
		Mode:       modeCount,
		Definition: def,
	}
}

// inferSymbolLang returns the dominant language for the SymbolGraph.
// When in.Graph is present, delegates to dominantLanguage.
// Otherwise infers from g.Module values: dotted → python, slash → go.
func inferSymbolLang(in MetricInput) string {
	if in.Graph != nil {
		return dominantLanguage(in.Graph)
	}
	// Infer from first non-empty module path.
	for _, mod := range in.SymbolGraph.Module {
		if mod == "" {
			continue
		}
		if strings.ContainsRune(mod, '.') {
			return "python"
		}
		if strings.ContainsRune(mod, '/') {
			return "go"
		}
		return ""
	}
	return ""
}

// buildLocByModule converts the FileLOC map (slash-path keys) to a map keyed
// by the same module-key format as g.Module (dotted for Python, dir-path for Go).
// Files that do not map to a module key are dropped.
func buildLocByModule(fileLOC map[string]int, lang string) map[string]int {
	out := make(map[string]int, len(fileLOC))
	for f, n := range fileLOC {
		if k := fileToModuleKey(f, lang); k != "" {
			out[k] += n
		}
	}
	return out
}

// cohesionSpreadDisplay renders the top-N spread entries for human/LLM output.
func cohesionSpreadDisplay(entries []cohesionSpreadEntry, topN int) string {
	if len(entries) == 0 {
		return "0 high-spread files"
	}
	var b strings.Builder
	fmt.Fprintf(&b, "%d high-spread file(s): ", len(entries))
	for i, e := range entries {
		if i == topN {
			fmt.Fprintf(&b, "+%d more", len(entries)-topN)
			break
		}
		if i > 0 {
			b.WriteString(", ")
		}
		fmt.Fprintf(&b, "%s [spread %d, LOC %d]", shortModule(e.file), e.spread, e.loc)
	}
	return b.String()
}
