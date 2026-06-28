package modularity

import (
	"fmt"
	"path/filepath"
	"strings"

	"github.com/alexei-led/archfit/internal/metrics/internal/modgraph"
	"github.com/alexei-led/archfit/internal/metrics/internal/result"
	"github.com/alexei-led/archfit/internal/model/clone"
	"github.com/alexei-led/archfit/internal/model/diagnostic"
	"github.com/alexei-led/archfit/internal/model/fileclass"
	"github.com/alexei-led/archfit/internal/model/signal"
	"github.com/alexei-led/archfit/internal/syntax"
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
// Test and generated files are excluded from the count (production-code signal
// only); the excluded cluster count is surfaced in the evidence string so
// nothing is hidden.
//
// FileClassIndex keys and clone Cluster.Files values must both be repo-relative
// slash paths for the index lookup to hit. When the index is nil (loc walk did
// not run), LookupFileClass falls back to built-in filename/path patterns only
// (mock_*.go, *.pb.go, _test.go, generated header, etc.).
func (m FunctionalCandidatesMetric) Calculate(in signal.DuplicationInput) diagnostic.MetricResult {
	if len(in.Duplication.Clusters) == 0 || in.Graph == nil {
		return result.NACount(m.Name(), m.Version(), functionalCandidatesDef)
	}

	// Pre-filter: drop clusters where any file is Test or Generated.
	// FileClassConfig{} is intentional: index files already incorporate the
	// user's config patterns; the fallback path uses built-in filename heuristics
	// (mock_*.go, *.pb.go, _test.go suffix, generated header, etc.).
	cfg := syntax.FileClassConfig{}
	prodClusters := make([]clone.Cluster, 0, len(in.Duplication.Clusters))
	excludedClusters := 0
	for _, c := range in.Duplication.Clusters {
		if isTestOrGeneratedCluster(c.Files, in.Size.FileClassIndex, cfg) {
			excludedClusters++
			continue
		}
		prodClusters = append(prodClusters, c)
	}

	resolve := modgraph.ModuleKeyResolver(in.Graph)

	// Map clone clusters to canonical cross-module pairs (deduped + sorted by ModulePairs).
	pairs := clone.ModulePairs(prodClusters, resolve)

	if len(pairs) == 0 {
		return result.NACount(m.Name(), m.Version(), functionalCandidatesDef)
	}

	// Build module-pair co-change set from CoChange for cross-reference.
	coChangePairs := make(map[[2]string]struct{}, len(in.History.CoChange))
	for fp := range in.History.CoChange {
		a := resolve(fp[0])
		b := resolve(fp[1])
		if a == "" || b == "" || a == b {
			continue
		}
		coChangePairs[modgraph.OrderedPair(a, b)] = struct{}{}
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
	if excludedClusters > 0 {
		fmt.Fprintf(&disp, " (%d test/generated/vendor excluded)", excludedClusters)
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

// isTestOrGeneratedCluster reports whether any file in the cluster is not a
// production file (test, generated, or vendor). A cluster containing such a
// file is excluded from the production metric — we do not want mock/test/vendor
// clone noise inflating the count.
//
// The language is derived from the file extension for the LookupFileClass
// fallback path (files not in the index). Empty cfg means built-in patterns only.
func isTestOrGeneratedCluster(files []string, index map[string]fileclass.FileClass, cfg syntax.FileClassConfig) bool {
	for _, f := range files {
		lang := langFromExt(filepath.Ext(f))
		fc := syntax.LookupFileClass(filepath.ToSlash(f), index, lang, cfg)
		if !fileclass.IsProduction(fc) {
			return true
		}
	}
	return false
}

// langFromExt maps a file extension (with leading dot) to a language tag
// recognised by syntax.IsTestFile and syntax.ClassifyFile. Unknown extensions
// return "". Must stay in sync with the canonical set: go, python, typescript, rust.
func langFromExt(ext string) string {
	switch ext {
	case ".go":
		return "go"
	case ".ts", ".tsx":
		return "typescript" // was "ts" — IsTestFile only handles "typescript"
	case ".js", ".jsx", ".mjs", ".cjs":
		return "js" // no IsTestFile case for JS; kept for future extension
	case ".py":
		return "python"
	case ".rs":
		return "rust"
	case ".java":
		return "java"
	case ".rb":
		return "ruby"
	default:
		return ""
	}
}
