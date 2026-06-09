package metrics

import (
	"fmt"
	"sort"
	"strings"

	"github.com/alexei-led/archfit/internal/model/diagnostic"
	"github.com/alexei-led/archfit/internal/model/symbol"
)

// sharedStateHubTopN is the maximum number of files displayed in the output.
const sharedStateHubTopN = 5

// sharedStateHubHotThreshold is the minimum per-symbol fan-in count to consider
// a symbol "hot" when computing the secondary hotCount sort key.
// Default 8; tunable via Task 5 validation.
const sharedStateHubHotThreshold = 8

const sharedStateHubDef = "per-file max single-symbol fan-in: the highest count of distinct " +
	"documents referencing any one symbol in the file; secondary: count of symbols with " +
	"fan-in >= 8 (hot threshold); distinct from risk_hub (surface breadth) — never gates"

// fileMaxFanIn computes, for each file (a value of g.Module), the maximum
// single-symbol fan-in among its symbols, the name of that symbol, and the
// count of the file's symbols whose fan-in meets sharedStateHubHotThreshold.
// Returns nil maps when g is empty.
func fileMaxFanIn(g symbol.Graph) (maxFI map[string]int, topSym map[string]string, hot map[string]int) {
	if g.Empty() {
		return nil, nil, nil
	}
	// Collect all symbols that have a module assignment.
	maxFI = make(map[string]int)
	topSym = make(map[string]string)
	hot = make(map[string]int)

	for sym, file := range g.Module {
		if file == "" {
			continue
		}
		fi := g.FanIn[sym]
		if fi > maxFI[file] {
			maxFI[file] = fi
			topSym[file] = sym
		}
		if fi >= sharedStateHubHotThreshold {
			hot[file]++
		}
	}

	if len(maxFI) == 0 {
		return nil, nil, nil
	}
	return maxFI, topSym, hot
}

// SharedStateHubMetric ranks source files by the maximum fan-in of any single
// symbol they own. A file with one heavily-referenced symbol (e.g. a mutable
// singleton) ranks high even if its total exported surface is narrow.
//
// This is DISTINCT from risk_hub (which measures the *breadth* of a module's
// cross-module surface, i.e. how many of its symbols are referenced at all).
// A file can win shared_state_hub while having a surface breadth of one —
// exactly the pattern risk_hub misses (narrow surface, deep per-symbol fan-in).
//
// Rank: max fan-in desc, then hotCount desc, then file name asc (total order).
// Band is always "info" (report-only); this metric never gates.
type SharedStateHubMetric struct{}

// sharedStateHubEntry holds per-file fan-in data for ranking and display.
type sharedStateHubEntry struct {
	file     string
	maxFanIn int
	topSym   string
	hotCount int
}

// Name returns "shared_state_hub".
func (m SharedStateHubMetric) Name() string { return "shared_state_hub" }

// Version returns "shared_state_hub.v1".
func (m SharedStateHubMetric) Version() string { return "shared_state_hub.v1" }

// Calculate ranks source files by maximum single-symbol fan-in. Returns n/a
// when SymbolGraph is empty (SCIP off or indexer absent) — never a false zero.
func (m SharedStateHubMetric) Calculate(in MetricInput) diagnostic.MetricResult {
	def := sharedStateHubDef
	if in.SymbolGraph.Empty() {
		return naCount(m.Name(), m.Version(), def)
	}

	maxFI, topSym, hot := fileMaxFanIn(in.SymbolGraph)
	if len(maxFI) == 0 {
		return naCount(m.Name(), m.Version(), def)
	}

	entries := make([]sharedStateHubEntry, 0, len(maxFI))
	for file, fi := range maxFI {
		entries = append(entries, sharedStateHubEntry{
			file:     file,
			maxFanIn: fi,
			topSym:   topSym[file],
			hotCount: hot[file],
		})
	}

	// Deterministic total order: max fan-in desc, then hotCount desc, then file name asc.
	sort.Slice(entries, func(i, j int) bool {
		if entries[i].maxFanIn != entries[j].maxFanIn {
			return entries[i].maxFanIn > entries[j].maxFanIn
		}
		if entries[i].hotCount != entries[j].hotCount {
			return entries[i].hotCount > entries[j].hotCount
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
		Display:    sharedStateHubDisplay(entries, sharedStateHubTopN),
		Band:       bandInformational,
		Confidence: confidence,
		Version:    m.Version(),
		Mode:       modeCount,
		Definition: def,
	}
}

// sharedStateHubDisplay renders the top-N entries compactly for human/LLM output.
func sharedStateHubDisplay(entries []sharedStateHubEntry, topN int) string {
	if len(entries) == 0 {
		return "0 shared-state hubs"
	}
	var b strings.Builder
	fmt.Fprintf(&b, "%d shared-state hub(s): ", len(entries))
	for i, e := range entries {
		if i == topN {
			fmt.Fprintf(&b, "+%d more", len(entries)-topN)
			break
		}
		if i > 0 {
			b.WriteString(", ")
		}
		fmt.Fprintf(&b, "%s [top:%s fan-in=%d, %d hot]",
			shortModule(e.file), shortModule(e.topSym), e.maxFanIn, e.hotCount)
	}
	return b.String()
}
