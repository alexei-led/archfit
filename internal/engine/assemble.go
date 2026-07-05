package engine

import (
	"sort"
	"strings"

	"github.com/alexei-led/archfit/internal/classify"
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
		mod, ok := mm.ModuleForFile(s.File)
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
		mod, ok := mm.ModuleForFile(s.File)
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
// ClassifiedEdgeSummary for coupling_balance scoring. It counts every graph edge
// (including same_module), separates cross-boundary scored vs abstained edges,
// and computes the arithmetic mean book balance over scored edges.
func buildClassifiedEdgeSummary(idx coupling.Index) *diagnostic.ClassifiedEdgeSummary {
	return buildClassifiedEdgeSummaryWithCloneOnly(idx, nil, config.DuplicatedKnowledgePolicyAdvisory)
}

func buildClassifiedEdgeSummaryForRun(idx coupling.Index, cloneOnly []classify.CloneOnlyPair, policy config.DuplicatedKnowledgePolicy, mm config.ModuleMap) *diagnostic.ClassifiedEdgeSummary {
	return buildClassifiedEdgeSummaryWithCloneOnlyAndModules(idx, cloneOnly, policy, mm)
}

// buildClassifiedEdgeSummaryWithCloneOnly also folds clone-only duplicated
// knowledge into the summary when the explicit policy is score.
//
// Edges whose target is not a declared module (Distance == DistanceUnknown:
// stdlib, third-party/library dependencies, undeclared packages) are EXCLUDED
// from the Scored/Abstained distribution and counted in External instead.
// This is language-agnostic: classifyDistance sets DistanceUnknown for all
// languages (Go stdlib/3p, Rust dependency crates, TS node_modules, Python
// external imports). External dependency hygiene is tracked separately and does
// not affect coupling_balance — the book measures coupling among YOUR components,
// not your libraries. Exception: targets matching a config-declared
// `external_systems:` entry classify as DistanceExternal (D=10, book Ch10
// Example 1) and DO enter the scored distribution, counted in DeclaredExternal.
//
// Genuine internal coupling with known distance but unknown strength is still
// counted as Abstained — it stays in the denominator and honestly lowers
// confidence. Clone-only duplicated knowledge has known symmetric strength and
// module-pair distance, so under the score policy it enters the same distribution
// as a score-bearing coupling fact without inventing a graph edge.
//
// The summary uses string keys (not coupling package constants) so it stays
// usable from diagnostic (stdlib-only) and score packages.
func buildClassifiedEdgeSummaryWithCloneOnly(idx coupling.Index, cloneOnly []classify.CloneOnlyPair, policy config.DuplicatedKnowledgePolicy) *diagnostic.ClassifiedEdgeSummary {
	return buildClassifiedEdgeSummaryWithCloneOnlyAndModules(idx, cloneOnly, policy, config.ModuleMap{})
}

func buildClassifiedEdgeSummaryWithCloneOnlyAndModules(idx coupling.Index, cloneOnly []classify.CloneOnlyPair, policy config.DuplicatedKnowledgePolicy, mm config.ModuleMap) *diagnostic.ClassifiedEdgeSummary {
	s := &diagnostic.ClassifiedEdgeSummary{
		ByStrength:          make(map[string]int),
		ByDistance:          make(map[string]int),
		ByDistanceBasis:     make(map[string]int),
		ByVolatility:        make(map[string]int),
		BySeverity:          make(map[string]int),
		DistanceCompression: buildDistanceCompressionSummary(),
	}
	connectedModules := make(map[string]struct{})
	balanceSum := 0
	for key, cl := range idx {
		balanceSum += addClassificationToSummary(s, cl)
		addConnectedModules(connectedModules, key, cl, mm)
	}
	if len(cloneOnly) > 0 {
		switch config.NormalizeDuplicatedKnowledgePolicy(policy) {
		case config.DuplicatedKnowledgePolicyScore:
			for _, p := range cloneOnly {
				s.CloneOnlyScored++
				balanceSum += addClassificationToSummary(s, p.Classification)
				addConnectedModuleName(connectedModules, p.FromModule)
				addConnectedModuleName(connectedModules, p.ToModule)
			}
		default:
			s.CloneOnlyAdvisory += len(cloneOnly)
		}
	}
	if len(connectedModules) > 0 {
		s.ConnectedModules = len(connectedModules)
	}
	if s.Scored > 0 {
		s.MeanBalance = float64(balanceSum) / float64(s.Scored)
	}
	return s
}

func buildDistanceCompressionSummary() *diagnostic.DistanceCompressionSummary {
	ev := classify.DistanceCompression()
	return &diagnostic.DistanceCompressionSummary{
		CompressedMiddleRungs: ev.CompressedMiddleRungs,
		ImplementedRungs:      append([]int(nil), ev.ImplementedRungs...),
		OmittedRungs:          append([]int(nil), ev.OmittedRungs...),
		DeterministicSplits:   append([]string(nil), ev.DeterministicSplits...),
		Rationale:             ev.Rationale,
	}
}

func addConnectedModules(modules map[string]struct{}, key string, cl coupling.Classification, mm config.ModuleMap) {
	if cl.Distance == coupling.DistanceSameModule || cl.Distance == coupling.DistanceUnknown {
		return
	}
	from, to, ok := indexKeyEndpoints(key)
	if !ok {
		return
	}
	if mod, ok := mm.ModuleFor(stripPrefix(from)); ok {
		addConnectedModuleName(modules, mod)
	}
	if mod, ok := mm.ModuleFor(stripPrefix(to)); ok {
		addConnectedModuleName(modules, mod)
	}
}

func addConnectedModuleName(modules map[string]struct{}, module string) {
	if module != "" {
		modules[module] = struct{}{}
	}
}

func indexKeyEndpoints(key string) (string, string, bool) {
	from, rest, ok := strings.Cut(key, "\x00")
	if !ok {
		return "", "", false
	}
	to, _, ok := strings.Cut(rest, "\x00")
	if !ok {
		return "", "", false
	}
	return from, to, true
}

func addClassificationToSummary(s *diagnostic.ClassifiedEdgeSummary, cl coupling.Classification) int {
	s.Total++
	if cl.Distance == coupling.DistanceSameModule {
		s.SameModule++
		return 0
	}
	// External/library edge: target not a declared module. Excluded from the
	// coupling_balance denominator — the book scores YOUR components only.
	// Targets matching a declared external_systems entry are NOT here: they
	// classify as DistanceExternal and fall through into the scored distribution
	// below (the architect declared the seam, so it is measured).
	if cl.Distance == coupling.DistanceUnknown {
		s.External++
		return 0
	}
	if cl.Distance == coupling.DistanceExternal {
		s.DeclaredExternal++
	}
	if cl.DistanceBasis != coupling.DistanceBasisUnknown {
		s.ByDistanceBasis[string(cl.DistanceBasis)]++
	}
	// Cross-boundary edge: target resolves to a declared module, a declared
	// external system, or a score-bearing clone-only duplicated-knowledge pair.
	s.ByStrength[string(cl.Strength)]++
	s.ByDistance[string(cl.Distance)]++
	s.ByVolatility[string(cl.Volatility)]++
	if cl.StrengthFromLLM {
		s.LabeledLLM++
	}
	if cl.StrengthFromNonHighLLM {
		s.LLMLowConfidenceEdges++
	}
	if cl.Score.Scored {
		s.Scored++
		s.BySeverity[string(cl.Score.Band)]++
		// Genuine distributed-monolith: critical band AND high distance (diff
		// owner / deploy unit). A critical edge at cross_module_same_owner is
		// local coupling, not a distributed monolith — see ClassifiedEdgeSummary.
		if cl.Score.Band == coupling.SeverityCritical && coupling.DistanceIsHigh(cl.Distance) {
			s.DistributedMonolith++
		}
		return cl.Score.Balance
	}
	s.Abstained++
	s.BySeverity["abstained"]++
	return 0
}

var connascenceKindsToDiscloseWhenUnmeasured = []string{"position", "execution", "timing", "value", "identity"}

// buildConnascenceReport aggregates deterministic static connascence evidence
// attached to classifications. It is report-only and intentionally discloses
// unmeasured semantic/dynamic categories instead of guessing them.
func buildConnascenceReport(idx coupling.Index) *diagnostic.ConnascenceReport {
	r := &diagnostic.ConnascenceReport{
		ByKind:   make(map[string]int),
		BySource: make(map[string]int),
	}
	for _, cl := range idx {
		if len(cl.Connascence) == 0 {
			r.AbstainedEdges++
			continue
		}
		r.EdgesWithEvidence++
		for _, ev := range cl.Connascence {
			r.TotalEvidence++
			r.ByKind[string(ev.Kind)]++
			r.BySource[ev.Source]++
		}
	}
	r.Unmeasured = unmeasuredConnascenceKinds(r.ByKind)
	if len(r.ByKind) == 0 {
		r.ByKind = nil
	}
	if len(r.BySource) == 0 {
		r.BySource = nil
	}
	return r
}

func unmeasuredConnascenceKinds(byKind map[string]int) []string {
	out := make([]string, 0, len(connascenceKindsToDiscloseWhenUnmeasured))
	for _, kind := range connascenceKindsToDiscloseWhenUnmeasured {
		if byKind[kind] == 0 {
			out = append(out, kind)
		}
	}
	return out
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
