package modularity

import (
	"fmt"
	"sort"
	"strings"

	"github.com/alexei-led/archfit/internal/metrics/internal/result"
	"github.com/alexei-led/archfit/internal/model/diagnostic"
	"github.com/alexei-led/archfit/internal/model/signal"
)

// FileStructuralWeightMetric reports size skew at file granularity: individual
// source files whose LOC is a large multiple of the per-file median. This
// complements structural_weight (module-level) by catching a single large file
// hiding inside a module that is under the module threshold. Report-only.
type FileStructuralWeightMetric struct{}

// Name returns "file_structural_weight".
func (m FileStructuralWeightMetric) Name() string { return "file_structural_weight" }

// Version returns "file_structural_weight.v1".
func (m FileStructuralWeightMetric) Version() string { return "file_structural_weight.v1" }

type sizeFile struct {
	path string
	loc  int
	mult int
}

// Calculate flags individual files whose LOC exceeds godFileMultiple × the
// per-file median (and the absolute godFileFloor floor). n/a when FileLOC is
// empty; no graph resolution is required.
func (m FileStructuralWeightMetric) Calculate(in signal.SizeInput) diagnostic.MetricResult {
	def := "individual source files whose size (LOC) is a large multiple of the per-file median (single-file god-files)"
	if len(in.Size.FileLOC) == 0 {
		return result.NACount(m.Name(), m.Version(), def)
	}

	locs := make([]int, 0, len(in.Size.FileLOC))
	for _, n := range in.Size.FileLOC {
		locs = append(locs, n)
	}
	sort.Ints(locs)
	median := locs[len(locs)/2]
	threshold := median * godFileMultiple
	if threshold < godFileFloor {
		threshold = godFileFloor
	}

	var gods []sizeFile
	for path, l := range in.Size.FileLOC {
		if l >= threshold {
			mult := 1
			if median > 0 {
				mult = l / median
			}
			gods = append(gods, sizeFile{path: path, loc: l, mult: mult})
		}
	}
	// Sort LOC desc, path asc as tiebreaker — files tie more often than modules.
	sort.Slice(gods, func(i, j int) bool {
		if gods[i].loc != gods[j].loc {
			return gods[i].loc > gods[j].loc
		}
		return gods[i].path < gods[j].path
	})

	confidence := result.ConfidenceHigh
	if len(in.Size.FileLOC) < result.ModularitySmallN {
		confidence = result.ConfidenceLow
	}
	return diagnostic.MetricResult{
		Name: m.Name(), Value: float64(len(gods)), Display: godFileDisplay(gods, median),
		Band: result.BandInformational, Confidence: confidence, Version: m.Version(), Mode: result.ModeCount,
		Definition: def,
	}
}

func godFileDisplay(gods []sizeFile, median int) string {
	if len(gods) == 0 {
		return fmt.Sprintf("0 god-file(s) (median %d LOC)", median)
	}
	var b strings.Builder
	fmt.Fprintf(&b, "%d god-file(s) (median %d LOC): ", len(gods), median)
	for i, g := range gods {
		if i == 5 {
			fmt.Fprintf(&b, "+%d more", len(gods)-5)
			break
		}
		if i > 0 {
			b.WriteString(", ")
		}
		fmt.Fprintf(&b, "%s (%d LOC, %dx)", g.path, g.loc, g.mult)
	}
	return b.String()
}
