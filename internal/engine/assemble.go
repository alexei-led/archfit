package engine

import (
	"sort"
	"strings"

	"github.com/alexei-led/archfit/internal/config"
	"github.com/alexei-led/archfit/internal/model/coupling"
	"github.com/alexei-led/archfit/internal/model/diagnostic"
	"github.com/alexei-led/archfit/internal/model/finding"
	"github.com/alexei-led/archfit/internal/scope"
	"github.com/alexei-led/archfit/internal/status"
)

// deltaReport builds the delta-bucket block for a run. It returns nil outside
// delta mode and when no finding lands in any bucket, so the field is omitted
// from non-delta output (and the golden full-mode fixtures stay byte-identical).
func deltaReport(mode scope.ScopeMode, findings []finding.Finding, accepted status.AcceptedSet, changed []string) *diagnostic.DeltaReport {
	if mode != scope.ModeDelta {
		return nil
	}
	r := status.DeltaBuckets(findings, accepted, changed)
	if r.Empty() {
		return nil
	}
	return &diagnostic.DeltaReport{
		New:             r.New,
		Existing:        r.Existing,
		Resolved:        r.Resolved,
		SeverityChanged: r.SeverityChanged,
		TouchedByDelta:  r.TouchedByDelta,
	}
}

// stripPrefix removes the "kind:" prefix from a node ID (e.g. "file:pkg/a" → "pkg/a").
func stripPrefix(id string) string {
	for i := 0; i < len(id); i++ {
		if id[i] == ':' {
			return id[i+1:]
		}
	}
	return id
}

// dynamicImportSiteCap bounds the number of sample sites stored per module in a
// DynamicImport rollup. Count carries the true total; the cap keeps a module with
// hundreds of lazy imports from bloating the output.
const dynamicImportSiteCap = 5

// buildDynamicImports groups report-only dynamic/lazy import sites per module
// (module-map key, or the file's directory when unmapped) into DynamicImport
// rollups. Output is deterministic: modules sorted by name, sites already sorted
// by the detector, the per-module sample capped at dynamicImportSiteCap. Returns
// an empty (non-nil) slice when no sites were found. Never touches the graph,
// metrics, or the verdict — this is evidence only.
func buildDynamicImports(sites []diagnostic.DynamicImportSite, mm config.ModuleMap) []diagnostic.DynamicImport {
	byModule := make(map[string][]diagnostic.DynamicImportSite)
	for _, s := range sites {
		mod, ok := mm.ModuleFor(s.File)
		if !ok || mod == "" {
			mod = pathDir(s.File)
		}
		byModule[mod] = append(byModule[mod], s)
	}
	mods := make([]string, 0, len(byModule))
	for m := range byModule {
		mods = append(mods, m)
	}
	sort.Strings(mods)

	out := make([]diagnostic.DynamicImport, 0, len(mods))
	for _, m := range mods {
		ms := byModule[m]
		sample := ms
		if len(sample) > dynamicImportSiteCap {
			sample = sample[:dynamicImportSiteCap]
		}
		out = append(out, diagnostic.DynamicImport{
			Module: m,
			Count:  len(ms),
			Sites:  sample,
		})
	}
	return out
}

// buildRuntimeAsync groups async-integration sites per module and returns a
// deterministic per-module rollup for the diagnostic.
// Returns an empty (non-nil) slice when no sites were found.
// Never touches the graph, metrics, or the verdict — this is evidence only.
func buildRuntimeAsync(sites []diagnostic.RuntimeAsyncSite, confidence string, mm config.ModuleMap) []diagnostic.RuntimeAsyncModule {
	byModule := make(map[string][]diagnostic.RuntimeAsyncSite)
	for _, s := range sites {
		mod, ok := mm.ModuleFor(s.File)
		if !ok || mod == "" {
			mod = pathDir(s.File)
		}
		byModule[mod] = append(byModule[mod], s)
	}
	mods := make([]string, 0, len(byModule))
	for m := range byModule {
		mods = append(mods, m)
	}
	sort.Strings(mods)

	out := make([]diagnostic.RuntimeAsyncModule, 0, len(mods))
	for _, m := range mods {
		ss := byModule[m]
		kind := dominantKind(ss)
		out = append(out, diagnostic.RuntimeAsyncModule{
			Module:          m,
			IntegrationKind: kind,
			Count:           len(ss),
			Confidence:      confidence,
		})
	}
	return out
}

// dominantKind returns the most frequent IntegrationKind among sites.
// Ties broken alphabetically for determinism.
func dominantKind(sites []diagnostic.RuntimeAsyncSite) string {
	counts := make(map[string]int, len(sites))
	for _, s := range sites {
		counts[s.IntegrationKind]++
	}
	best, bestN := "", 0
	for k, n := range counts {
		if n > bestN || (n == bestN && k < best) {
			best, bestN = k, n
		}
	}
	return best
}

// pathDir returns the directory portion of a repo-relative slash path, or "."
// when the path has no directory. Used as the dynamic-import module key when the
// module map does not cover a file.
func pathDir(p string) string {
	if i := strings.LastIndex(p, "/"); i >= 0 {
		return p[:i]
	}
	return "."
}

// buildClassifiedEdgeSummary aggregates the full coupling.Index into a
// ClassifiedEdgeSummary for coupling_balance scoring. It counts every edge
// (including same_module), separates cross-boundary scored vs abstained edges,
// and computes the arithmetic mean book balance over scored edges.
// The summary uses string keys (not coupling package constants) so it stays
// usable from diagnostic (stdlib-only) and score packages.
func buildClassifiedEdgeSummary(idx coupling.Index) *diagnostic.ClassifiedEdgeSummary {
	s := &diagnostic.ClassifiedEdgeSummary{
		ByStrength:   make(map[string]int),
		ByDistance:   make(map[string]int),
		ByVolatility: make(map[string]int),
		BySeverity:   make(map[string]int),
	}
	balanceSum := 0
	for _, cl := range idx {
		s.Total++
		if cl.Distance == coupling.DistanceSameModule {
			s.SameModule++
			continue
		}
		// Cross-boundary edge.
		s.ByStrength[string(cl.Strength)]++
		s.ByDistance[string(cl.Distance)]++
		s.ByVolatility[string(cl.Volatility)]++
		if cl.Score.Scored {
			s.Scored++
			balanceSum += cl.Score.Balance
			s.BySeverity[string(cl.Score.Band)]++
		} else {
			s.Abstained++
			s.BySeverity["abstained"]++
		}
	}
	if s.Scored > 0 {
		s.MeanBalance = float64(balanceSum) / float64(s.Scored)
	}
	return s
}

// buildClassifiedEdgeSummaryWithLLM is like buildClassifiedEdgeSummary but also
// stamps the LLM-approved label count so score.go can lower coupling_balance
// confidence when a significant fraction of labels came from an LLM judge.
func buildClassifiedEdgeSummaryWithLLM(idx coupling.Index, llmApproved int) *diagnostic.ClassifiedEdgeSummary {
	s := buildClassifiedEdgeSummary(idx)
	s.LLMApproved = llmApproved
	return s
}

// countActive returns the number of findings whose status is not fixed.
func countActive(findings []finding.Finding) int {
	n := 0
	for _, f := range findings {
		if f.Status != finding.StatusFixed {
			n++
		}
	}
	return n
}
