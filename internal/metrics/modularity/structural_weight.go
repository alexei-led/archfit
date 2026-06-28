package modularity

import (
	"fmt"
	"sort"
	"strings"

	"github.com/alexei-led/archfit/internal/metrics/internal/modgraph"
	"github.com/alexei-led/archfit/internal/metrics/internal/result"
	"github.com/alexei-led/archfit/internal/model/diagnostic"
	"github.com/alexei-led/archfit/internal/model/fileclass"
	"github.com/alexei-led/archfit/internal/model/signal"
	"github.com/alexei-led/archfit/internal/syntax"
)

// StructuralWeightMetric reports size skew: modules far larger than the codebase
// median, which concentrate responsibilities (a cohesion smell). Report-only; the
// ranked god-module list is the signal, not a quality verdict.
type StructuralWeightMetric struct{}

// Name returns "structural_weight".
func (m StructuralWeightMetric) Name() string { return "structural_weight" }

// Version returns "structural_weight.v1".
func (m StructuralWeightMetric) Version() string { return "structural_weight.v1" }

type sizeMod struct {
	module string
	loc    int
	mult   int
}

// Calculate aggregates per-file LOC onto modules and flags god-modules. n/a
// without LOC data or a graph (the latter gives the language for file→module).
func (m StructuralWeightMetric) Calculate(in signal.SizeInput) diagnostic.MetricResult {
	def := "modules whose size (LOC) is a large multiple of the codebase median (size-skew god-modules)"
	if in.Graph == nil || len(in.Size.FileLOC) == 0 {
		return result.NACount(m.Name(), m.Version(), def)
	}
	resolve := modgraph.ModuleKeyResolver(in.Graph)
	// FileClassConfig{} is intentional: index files already incorporate the
	// user's config patterns; the fallback path uses built-in filename heuristics
	// (mock_*.go, *.pb.go, generated header, etc.).
	cfg := syntax.FileClassConfig{}
	modLOC := map[string]int{}
	for f, n := range in.Size.FileLOC {
		fc := syntax.LookupFileClass(f, in.Size.FileClassIndex, "", cfg)
		if !fileclass.IsProduction(fc) {
			continue
		}
		if k := resolve(f); k != "" {
			modLOC[k] += n
		}
	}
	if len(modLOC) < 2 {
		return result.NACount(m.Name(), m.Version(), def)
	}
	locs := make([]int, 0, len(modLOC))
	for _, l := range modLOC {
		locs = append(locs, l)
	}
	sort.Ints(locs)
	median := locs[len(locs)/2]
	threshold := median * godModuleMultiple
	if threshold < godModuleFloor {
		threshold = godModuleFloor
	}

	var gods []sizeMod
	for mod, l := range modLOC {
		if l >= threshold {
			mult := 1
			if median > 0 {
				mult = l / median
			}
			gods = append(gods, sizeMod{module: mod, loc: l, mult: mult})
		}
	}
	sort.Slice(gods, func(i, j int) bool { return gods[i].loc > gods[j].loc })

	confidence := result.ConfidenceHigh
	if len(modLOC) < result.ModularitySmallN {
		confidence = result.ConfidenceLow
	}
	return diagnostic.MetricResult{
		Name: m.Name(), Value: float64(len(gods)), Display: godDisplay(gods, median),
		Band: result.BandInformational, Confidence: confidence, Version: m.Version(), Mode: result.ModeCount,
		Definition: def,
	}
}

func godDisplay(gods []sizeMod, median int) string {
	if len(gods) == 0 {
		return fmt.Sprintf("0 god-modules (median %d LOC)", median)
	}
	var b strings.Builder
	fmt.Fprintf(&b, "%d god-module(s) (median %d LOC): ", len(gods), median)
	for i, g := range gods {
		if i == 5 {
			fmt.Fprintf(&b, "+%d more", len(gods)-5)
			break
		}
		if i > 0 {
			b.WriteString(", ")
		}
		fmt.Fprintf(&b, "%s (%d LOC, %dx)", result.ShortModule(g.module), g.loc, g.mult)
	}
	return b.String()
}
