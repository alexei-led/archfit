package modularity

import (
	"fmt"
	"strings"

	"github.com/alexei-led/archfit/internal/metrics/internal/result"
	"github.com/alexei-led/archfit/internal/model/clone"
	"github.com/alexei-led/archfit/internal/model/diagnostic"
	"github.com/alexei-led/archfit/internal/model/signal"
)

// FunctionalCandidatesMetric reports cross-module pairs that share duplicated
// logic as detected by a clone detector (e.g. jscpd). It optionally flags which
// of those pairs also co-change, making them stronger refactoring candidates.
//
// DISTINCT from hidden_coupling: hidden_coupling finds pairs that co-change
// WITHOUT a static import edge (invisible runtime coupling). This metric is
// DUPLICATION-based — it surfaces pairs whose source code is literally copied,
// regardless of whether they import each other. A pair can appear in both
// metrics (cloned AND invisibly coupled), but each captures a different signal.
//
// Band is always "info" (report-only); this metric never gates.
type FunctionalCandidatesMetric struct{}

// Name returns "functional_candidates".
func (m FunctionalCandidatesMetric) Name() string { return "functional_candidates" }

// Version returns "functional_candidates.v1".
func (m FunctionalCandidatesMetric) Version() string { return "functional_candidates.v1" }

const functionalCandidatesDef = "cross-module pairs with duplicated logic (clone-based); " +
	"DISTINCT from hidden_coupling (co-change without static edge) — this is duplication-based"

// Calculate counts cross-module pairs sharing duplicated code blocks, with an
// optional co-change cross-reference. Reports n/a when no clone data is present.
func (m FunctionalCandidatesMetric) Calculate(in signal.MetricInput) diagnostic.MetricResult {
	if len(in.CloneClusters) == 0 || in.Graph == nil {
		return result.NACount(m.Name(), m.Version(), functionalCandidatesDef)
	}

	lang := dominantLanguage(in.Graph)

	// Map clone clusters to canonical cross-module pairs (deduped + sorted by ModulePairs).
	pairs := clone.ModulePairs(in.CloneClusters, func(f string) string {
		return fileToModuleKey(f, lang)
	})

	if len(pairs) == 0 {
		return result.NACount(m.Name(), m.Version(), functionalCandidatesDef)
	}

	// Build module-pair co-change set from CoChange for cross-reference.
	coChangePairs := make(map[[2]string]struct{}, len(in.CoChange))
	for fp := range in.CoChange {
		a := fileToModuleKey(fp[0], lang)
		b := fileToModuleKey(fp[1], lang)
		if a == "" || b == "" || a == b {
			continue
		}
		coChangePairs[orderedPair(a, b)] = struct{}{}
	}

	// Count how many clone pairs also co-change.
	alsoCoChange := 0
	for _, p := range pairs {
		if _, ok := coChangePairs[p]; ok {
			alsoCoChange++
		}
	}

	var disp strings.Builder
	fmt.Fprintf(&disp, "%d clone-duplicated cross-module pair(s)", len(pairs))
	if alsoCoChange > 0 {
		fmt.Fprintf(&disp, " (%d also co-change)", alsoCoChange)
	}

	return diagnostic.MetricResult{
		Name:       m.Name(),
		Value:      float64(len(pairs)),
		Display:    disp.String(),
		Band:       result.BandInformational,
		Confidence: result.ConfidenceHigh,
		Version:    m.Version(),
		Mode:       result.ModeCount,
		Definition: functionalCandidatesDef,
	}
}
