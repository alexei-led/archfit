package modularity

import (
	"fmt"
	"sort"
	"strings"

	"github.com/alexei-led/archfit/internal/metrics/internal/result"
	"github.com/alexei-led/archfit/internal/model/diagnostic"
	"github.com/alexei-led/archfit/internal/model/signal"
	"github.com/alexei-led/archfit/internal/model/symbol"
)

// cohesionTopN is the maximum number of fragmented modules displayed.
const cohesionTopN = 5

// cohesionMinSymbols is the minimum number of definition symbols a module must
// have before its internal connectivity is meaningful. Below it a module is too
// small for a lack-of-cohesion verdict (two or three symbols are trivially
// "fragmented"). Tunable; report-only.
const cohesionMinSymbols = 4

// cohesionMinDocs is the minimum number of distinct documents (files) a module
// must span to be measurable. SCIP indexers do not populate enclosing_range, so
// reference attribution is document-scoped: within a single document every
// definition is treated as the source of every reference, which over-connects
// same-document symbols and makes a single-document module look trivially
// cohesive. Only cross-document intra-module structure carries a trustworthy
// signal — see internal/extract/scip/scip_reader.py and the symbol.Graph doc.
//
// CAVEAT (Python/TS): for scip-python and scip-typescript a "module" is keyed
// per source file, so almost every module is single-document and therefore not
// measurable by this proxy. The metric reports n/a for such repos rather than a
// false cohesion verdict. Go packages span multiple files and are measurable.
const cohesionMinDocs = 2

const cohesionDef = "LCOM edge-density proxy: multi-document modules whose definition " +
	"symbols split into >1 connected component over same-module SCIP references " +
	"(fragmented = lacking cohesion). Document-scoped attribution makes " +
	"single-document modules (the common Python/TS shape) unmeasurable — n/a, not a " +
	"false verdict. Report-only; never gates."

// CohesionMetric is a report-only Lack-of-Cohesion-of-Methods (LCOM) edge-density
// proxy over the SCIP symbol graph. For each measurable module it builds the
// undirected graph of the module's definition symbols connected by same-module
// references (symbol.Graph.IntraRefs) and counts connected components: a cohesive
// module's symbols reference each other (one component); a module bundling
// unrelated concerns splits into several (LCOM4 > 1).
//
// It is intentionally DISTINCT from the removed cohesion_spread metric (which
// measured outbound cross-module spread and failed its validation gate): this
// proxy reasons over INTRA-module connectivity, not external dispersion.
//
// The metric is gated behind an eval (docs/plans/notes/gap-closure-task20-cohesion-eval.md):
// it is kept report-only (band "info", never gates) and disabled in archfit's own
// .archfit.yaml because the document-scoped attribution ceiling leaves it blind for
// single-file Python/TS modules and unable to credit internal sub-package cohesion.
type CohesionMetric struct{}

// Name returns "cohesion_lcom".
func (m CohesionMetric) Name() string { return "cohesion_lcom" }

// Version returns "cohesion_lcom.v1".
func (m CohesionMetric) Version() string { return "cohesion_lcom.v1" }

// cohesionEntry is one fragmented module for display.
type cohesionEntry struct {
	module     string
	symbols    int
	docs       int
	components int
}

// Calculate computes per-module LCOM and reports fragmented modules. Returns n/a
// when SCIP is off, no same-module edges exist, or no module is measurable
// (every module is single-document — the document-scoped attribution ceiling).
func (m CohesionMetric) Calculate(in signal.SymbolInput) diagnostic.MetricResult {
	g := in.Symbol.Graph
	if g.Empty() || len(g.IntraRefs) == 0 {
		return result.NACount(m.Name(), m.Version(), cohesionDef)
	}

	symbolsByModule, docsByModule := moduleSymbolSets(g)

	entries := make([]cohesionEntry, 0)
	measurable := 0
	for mod, syms := range symbolsByModule {
		if len(syms) < cohesionMinSymbols || len(docsByModule[mod]) < cohesionMinDocs {
			continue
		}
		measurable++
		components := lcomComponents(syms, g.IntraRefs)
		if components > 1 {
			entries = append(entries, cohesionEntry{
				module:     mod,
				symbols:    len(syms),
				docs:       len(docsByModule[mod]),
				components: components,
			})
		}
	}

	if measurable == 0 {
		return result.NACount(m.Name(), m.Version(), cohesionDef)
	}

	// Rank: most components first, then most symbols, then module name (total order).
	sort.Slice(entries, func(i, j int) bool {
		if entries[i].components != entries[j].components {
			return entries[i].components > entries[j].components
		}
		if entries[i].symbols != entries[j].symbols {
			return entries[i].symbols > entries[j].symbols
		}
		return entries[i].module < entries[j].module
	})

	confidence := result.ConfidenceHigh
	if measurable < result.ModularitySmallN {
		confidence = result.ConfidenceLow
	}

	return diagnostic.MetricResult{
		Name:       m.Name(),
		Value:      float64(len(entries)),
		Display:    cohesionDisplay(entries, measurable),
		Band:       result.BandInformational,
		Confidence: confidence,
		Version:    m.Version(),
		Mode:       result.ModeCount,
		Definition: cohesionDef,
	}
}

// moduleSymbolSets inverts the symbol graph into per-module definition-symbol
// sets and per-module distinct-document sets.
func moduleSymbolSets(g symbol.Graph) (syms map[string]map[string]struct{}, docs map[string]map[string]struct{}) {
	syms = make(map[string]map[string]struct{}, len(g.Module))
	docs = make(map[string]map[string]struct{}, len(g.Module))
	for sym, mod := range g.Module {
		if mod == "" {
			continue
		}
		if syms[mod] == nil {
			syms[mod] = make(map[string]struct{})
			docs[mod] = make(map[string]struct{})
		}
		syms[mod][sym] = struct{}{}
		if p := g.Path[sym]; p != "" {
			docs[mod][p] = struct{}{}
		}
	}
	return syms, docs
}

// lcomComponents counts connected components (LCOM4) of the undirected graph
// whose nodes are the module's symbols and whose edges are same-module
// references with both endpoints in the module. Isolated symbols each form their
// own component.
func lcomComponents(syms map[string]struct{}, intra map[string]map[string]struct{}) int {
	parent := make(map[string]string, len(syms))
	for s := range syms {
		parent[s] = s
	}
	find := func(s string) string {
		for parent[s] != s {
			parent[s] = parent[parent[s]] // path halving
			s = parent[s]
		}
		return s
	}
	union := func(a, b string) {
		ra, rb := find(a), find(b)
		if ra != rb {
			parent[ra] = rb
		}
	}
	for from, tos := range intra {
		if _, ok := syms[from]; !ok {
			continue
		}
		for to := range tos {
			if _, ok := syms[to]; ok {
				union(from, to)
			}
		}
	}
	roots := make(map[string]struct{}, len(syms))
	for s := range syms {
		roots[find(s)] = struct{}{}
	}
	return len(roots)
}

// cohesionDisplay renders the top fragmented modules compactly for human/LLM output.
func cohesionDisplay(entries []cohesionEntry, measurable int) string {
	if len(entries) == 0 {
		return fmt.Sprintf("0 fragmented module(s) (%d measurable)", measurable)
	}
	var b strings.Builder
	fmt.Fprintf(&b, "%d fragmented module(s) of %d measurable: ", len(entries), measurable)
	for i, e := range entries {
		if i == cohesionTopN {
			fmt.Fprintf(&b, "+%d more", len(entries)-cohesionTopN)
			break
		}
		if i > 0 {
			b.WriteString(", ")
		}
		fmt.Fprintf(&b, "%s (%d parts, %d symbols, %d files)",
			result.ShortModule(e.module), e.components, e.symbols, e.docs)
	}
	return b.String()
}
