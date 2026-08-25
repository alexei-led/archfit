package markdown

import (
	"fmt"
	"sort"
	"strings"

	"github.com/alexei-led/archfit/internal/model/report"
)

// writeBeyondBCMetrics renders the "Supporting structural metrics (beyond Balanced
// Coupling)" section. These are report-only and never gate.
func writeBeyondBCMetrics(b *strings.Builder, metrics []report.MetricResult) {
	b.WriteString("\n## Supporting structural metrics (beyond Balanced Coupling)\n\n")
	b.WriteString("Report-only. These metrics support Balanced Coupling reasoning but never gate.\n\n")

	for _, m := range metrics {
		band := m.Band
		if m.Confidence != "" && m.Confidence != confidenceHigh {
			band = fmt.Sprintf("%s (%s confidence)", band, m.Confidence)
		}
		fmt.Fprintf(b, "- **%s**: %s — %s\n", m.Name, m.Display, band)
	}
}

// writeDistanceConfidence renders the "Distance confidence" section summarising
// how the distance dimension was resolved for this run.
// code_structure is always-on; ownership and deploy_unit come from tool coverage.
// Unresolved modules are counted from extractor coverage records.
func writeDistanceConfidence(b *strings.Builder, d report.Document) {
	// owner_source is a first-class diagnostic field (config|codeowners|git|none).
	ownerSrc := d.OwnerSource
	// Collect remaining distance-signal sources from tool coverage entries.
	deployUnitSrc := ""
	unresolved := 0
	for _, cov := range d.ToolCoverage {
		if cov.Tool == "deploy-unit" {
			deployUnitSrc = cov.Status
		}
		unresolved += cov.Unresolved
	}

	b.WriteString("\n## Distance confidence\n\n")
	b.WriteString("- `code_structure`: always on (deterministic tree-distance baseline)\n")
	if ownerSrc != "" {
		fmt.Fprintf(b, "- `owner_source`: %s\n", ownerSrc)
	} else {
		b.WriteString("- `owner_source`: not reported (CODEOWNERS or git-author fallback)\n")
	}
	if deployUnitSrc != "" {
		fmt.Fprintf(b, "- `deploy_unit_source`: %s\n", deployUnitSrc)
	} else {
		b.WriteString("- `deploy_unit_source`: not reported (auto-detect or config-authored)\n")
	}
	if dcx := d.DistanceContext; dcx != nil {
		fmt.Fprintf(b, "- `owner_model`: %s\n", dcx.OwnerModel)
		if dcx.DeployUnitDetectedModules > 0 {
			fmt.Fprintf(b, "- deploy-unit detector mapped modules: %d\n", dcx.DeployUnitDetectedModules)
		}
		if dcx.DeclaredExternalSystems > 0 {
			fmt.Fprintf(b, "- declared external systems: %d\n", dcx.DeclaredExternalSystems)
		}
		if dcx.RuntimeAsyncRelations > 0 {
			fmt.Fprintf(b, "- runtime async relations: %d\n", dcx.RuntimeAsyncRelations)
		}
		if len(dcx.RuntimeAsyncKinds) > 0 {
			fmt.Fprintf(b, "- runtime async kinds: %s\n", formatCounts(dcx.RuntimeAsyncKinds))
		}
		if len(dcx.DistanceBasis) > 0 {
			fmt.Fprintf(b, "- distance basis: %s\n", formatCounts(dcx.DistanceBasis))
		}
		if dcx.Interpretation != "" {
			fmt.Fprintf(b, "- interpretation: %s\n", dcx.Interpretation)
		}
		if dcx.RuntimeInterpretation != "" {
			fmt.Fprintf(b, "- runtime interpretation: %s\n", dcx.RuntimeInterpretation)
		}
	}
	if ce := d.ClassifiedEdges; ce != nil {
		if ce.ConnectedModules > 0 {
			fmt.Fprintf(b, "- connected modules in coupling sample: %d\n", ce.ConnectedModules)
		}
		if len(ce.ByDistanceBasis) > 0 && d.DistanceContext == nil {
			fmt.Fprintf(b, "- distance basis: %s\n", formatCounts(ce.ByDistanceBasis))
		}
		if dc := ce.DistanceCompression; dc != nil {
			fmt.Fprintf(b, "- distance rungs implemented: %s; omitted/compressed: %s\n", formatInts(dc.ImplementedRungs), formatInts(dc.OmittedRungs))
			if len(dc.CodeStructureBoundaryCounts) > 0 {
				fmt.Fprintf(b, "- code-structure boundary crossings: %s\n", formatDistanceCounts(dc.CodeStructureBoundaryCounts))
			}
			if len(dc.CodeStructureAncestorDepths) > 0 {
				fmt.Fprintf(b, "- code-structure shared-ancestor depth: %s\n", formatDistanceCounts(dc.CodeStructureAncestorDepths))
			}
			if dc.Rationale != "" {
				fmt.Fprintf(b, "- distance compression: %s\n", dc.Rationale)
			}
			for _, r := range dc.OmittedRungReasons {
				fmt.Fprintf(b, "- D=%d compressed: %s\n", r.Rung, r.Reason)
			}
		}
		if ce.DeclaredExternal > 0 {
			fmt.Fprintf(b, "- declared external-system edges scored at D=10: %d\n", ce.DeclaredExternal)
		}
		if ce.External > 0 {
			fmt.Fprintf(b, "- undeclared external/library edges excluded: %d\n", ce.External)
		}
		if ce.CloneOnlyScored > 0 || ce.CloneOnlyAdvisory > 0 {
			fmt.Fprintf(b, "- clone-only duplicated knowledge: %d scored, %d advisory-only\n", ce.CloneOnlyScored, ce.CloneOnlyAdvisory)
		}
		if tr := ce.TailRisk; tr != nil {
			fmt.Fprintf(b, "- tail risk: worst balance %d/10; lower-decile balance %d/10; high-or-worse edges %d/%d (%d%%); critical %d; distributed-monolith %d\n",
				tr.WorstBalance, tr.LowerDecileBalance, tr.HighOrWorseEdges, ce.Scored, tr.HighOrWorseSharePct, tr.CriticalEdges, tr.DistributedMonolithEdges)
			if tr.CloneOnlyScored > 0 {
				fmt.Fprintf(b, "- clone-only tail: worst balance %d/10; high-or-worse %d/%d scored clone-only pairs\n",
					tr.CloneOnlyWorstBalance, tr.CloneOnlyHighOrWorseEdges, tr.CloneOnlyScored)
			}
		}
	}
	if unresolved > 0 {
		fmt.Fprintf(b, "- unresolved modules: %d (edges to unknown modules use conservative distance)\n", unresolved)
	}
}

func formatInts(values []int) string {
	if len(values) == 0 {
		return "-"
	}
	parts := make([]string, len(values))
	for i, v := range values {
		parts[i] = fmt.Sprintf("D=%d", v)
	}
	return strings.Join(parts, ", ")
}

func formatDistanceCounts(values []report.DistanceCount) string {
	if len(values) == 0 {
		return "-"
	}
	parts := make([]string, 0, len(values))
	for _, v := range values {
		parts = append(parts, fmt.Sprintf("%d→%d", v.Value, v.Count))
	}
	return strings.Join(parts, ", ")
}

// fileFactsTopN is the number of modules listed per axis in the structural-facts section.
const fileFactsTopN = 5

// writeFileFacts prints the neutral structural-facts block: the top modules by
// each axis (inbound module fan-in, outbound destinations, LOC). Numbers only —
// no risk labels or ranking verdicts; the full per-module list is in the JSON
// output. Omitted entirely when no facts were collected (SCIP off/absent).
func writeFileFacts(b *strings.Builder, facts []report.FileFact) {
	if len(facts) == 0 {
		return
	}
	b.WriteString("\n## Structural facts (neutral evidence)\n\n")
	fmt.Fprintf(b, "%d modules; top %d per axis (full list in `--format json`):\n\n", len(facts), fileFactsTopN)

	axes := []struct {
		label string
		value func(report.FileFact) int
	}{
		{"inbound module fan-in", func(f report.FileFact) int { return f.InboundModuleFanIn }},
		{"outbound destinations", func(f report.FileFact) int { return f.OutboundDestinations }},
		{"LOC", func(f report.FileFact) int { return f.LOC }},
	}
	for _, axis := range axes {
		ranked := make([]report.FileFact, len(facts))
		copy(ranked, facts)
		sort.SliceStable(ranked, func(i, j int) bool {
			if v1, v2 := axis.value(ranked[i]), axis.value(ranked[j]); v1 != v2 {
				return v1 > v2
			}
			return ranked[i].Module < ranked[j].Module
		})
		fmt.Fprintf(b, "- %s:", axis.label)
		for i, f := range ranked {
			if i == fileFactsTopN || axis.value(f) == 0 {
				break
			}
			if i > 0 {
				b.WriteString(",")
			}
			fmt.Fprintf(b, " %s (%d)", f.Module, axis.value(f))
		}
		b.WriteString("\n")
	}
}

// syntaxSurfaceExportedTopN is the cap on exported declarations listed in the
// Syntax surface section. Full list remains in `--format json`.
const syntaxSurfaceExportedTopN = 20

// writeSyntaxSurface prints the neutral syntax-surface block: declaration
// counts by kind, the public API (exported declarations) grouped by file, and
// detected architectural roles/routes. Omitted entirely when SyntaxFacts is
// empty (syntax disabled or sg absent) — no false signal, no empty section.
func writeSyntaxSurface(b *strings.Builder, facts []report.SyntaxFact) {
	if len(facts) == 0 {
		return
	}

	// Aggregate totals.
	kindCounts := make(map[string]int)
	var exportedCount int
	for _, f := range facts {
		kindCounts[f.Kind]++
		if f.Exported {
			exportedCount++
		}
	}

	b.WriteString("\n## Syntax surface (neutral evidence)\n\n")
	fmt.Fprintf(b, "%d declaration(s) extracted by ast-grep (full list in `--format json`):\n\n", len(facts))

	// Kind counts — deterministic order: sort keys.
	kinds := make([]string, 0, len(kindCounts))
	for k := range kindCounts {
		kinds = append(kinds, k)
	}
	sort.Strings(kinds)
	for _, k := range kinds {
		fmt.Fprintf(b, "- %s: %d\n", k, kindCounts[k])
	}
	fmt.Fprintf(b, "- exported (public API): %d\n", exportedCount)

	// Per-module declaration counts — deterministic order: sort module keys.
	// Facts with an empty Module (outside declared modules) are bucketed as "(unscoped)".
	moduleCounts := make(map[string]int)
	for _, f := range facts {
		mod := f.Module
		if mod == "" {
			mod = "(unscoped)"
		}
		moduleCounts[mod]++
	}
	if len(moduleCounts) > 0 {
		b.WriteString("\nPer module:\n\n")
		mods := make([]string, 0, len(moduleCounts))
		for m := range moduleCounts {
			mods = append(mods, m)
		}
		sort.Strings(mods)
		for _, m := range mods {
			fmt.Fprintf(b, "- %s: %d\n", m, moduleCounts[m])
		}
	}

	// Public API list (exported declarations), grouped by file, capped.
	var exported []report.SyntaxFact
	for _, f := range facts {
		if f.Exported {
			exported = append(exported, f)
		}
	}
	if len(exported) > 0 {
		b.WriteString("\n### Public API\n\n")
		// Group by file — facts are pre-sorted (File, StartLine) upstream.
		printed := 0
		var curFile string
		for _, f := range exported {
			if printed >= syntaxSurfaceExportedTopN {
				fmt.Fprintf(b, "- ... +%d more exported declarations (use `--format json`)\n", exportedCount-printed)
				break
			}
			if f.File != curFile {
				curFile = f.File
				if f.Module != "" {
					fmt.Fprintf(b, "\n`%s` [%s]:\n", f.File, f.Module)
				} else {
					fmt.Fprintf(b, "\n`%s`:\n", f.File)
				}
			}
			line := fmt.Sprintf("- `%s` (%s)", f.Name, f.Kind)
			if f.Framework != "" {
				line += " [" + f.Framework + "]"
			}
			b.WriteString(line + "\n")
			printed++
		}
	}

	// Routes summary.
	routeCount := 0
	for _, f := range facts {
		if f.Kind == "route" {
			routeCount++
		}
	}
	if routeCount > 0 {
		b.WriteString("\n### Detected routes\n\n")
		fmt.Fprintf(b, "- route: %d registration(s)\n", routeCount)
	}
}

// writeConnascenceSummary prints the report-only deterministic connascence block.
// It is deliberately compact: counts by kind/source plus explicit unmeasured
// categories, with no score language.
func writeConnascenceSummary(b *strings.Builder, r *report.ConnascenceReport) {
	if r == nil {
		return
	}
	b.WriteString("\n## Connascence evidence (deterministic)\n\n")
	b.WriteString("Report-only. Static facts only; semantic and dynamic categories without deterministic evidence stay unmeasured.\n\n")
	fmt.Fprintf(b, "- edges with evidence: %d\n", r.EdgesWithEvidence)
	fmt.Fprintf(b, "- abstained edges: %d\n", r.AbstainedEdges)
	fmt.Fprintf(b, "- total evidence facts: %d\n", r.TotalEvidence)
	if r.StrengthInferredEdges > 0 {
		fmt.Fprintf(b, "- strength inferred from connascence: %d edges\n", r.StrengthInferredEdges)
	}
	if len(r.ByKind) > 0 {
		fmt.Fprintf(b, "- by kind: %s\n", formatCounts(r.ByKind))
	}
	if len(r.BySource) > 0 {
		fmt.Fprintf(b, "- by source: %s\n", formatCounts(r.BySource))
	}
	if len(r.Unmeasured) > 0 {
		fmt.Fprintf(b, "- unmeasured: %s\n", strings.Join(r.Unmeasured, ", "))
	}
	if len(r.Roadmap) > 0 {
		fmt.Fprintf(b, "- roadmap: %s\n", formatConnascenceRoadmap(r.Roadmap))
	}
}

func formatConnascenceRoadmap(items []report.ConnascenceRoadmapItem) string {
	parts := make([]string, 0, len(items))
	for _, item := range items {
		part := item.Kind + "=" + item.CurrentStatus
		if len(item.RelatedSignals) > 0 {
			part += " (signals " + strings.Join(item.RelatedSignals, "/") + ")"
		}
		parts = append(parts, part)
	}
	return strings.Join(parts, ", ")
}

func writeDynamicConnascenceSignals(b *strings.Builder, block *report.DynamicConnascenceSignals) {
	if block == nil || len(block.Signals) == 0 {
		return
	}
	total := 0
	for _, s := range block.Signals {
		total += s.Count
	}
	b.WriteString("\n## Dynamic connascence signals (report-only)\n\n")
	b.WriteString("Report-only. Static dynamic-import and runtime-async sites can guide Ch6 execution/timing review, but they are not runtime measurements and never change score or verdict.\n\n")
	fmt.Fprintf(b, "- signals: %d across %d module/source group(s)\n", total, len(block.Signals))
	if len(block.Unmeasured) > 0 {
		fmt.Fprintf(b, "- still unmeasured: %s\n", strings.Join(block.Unmeasured, ", "))
	}
	if block.ReportOnlyReason != "" {
		fmt.Fprintf(b, "- reason: %s\n", block.ReportOnlyReason)
	}

	ranked := make([]report.DynamicConnascenceSignal, len(block.Signals))
	copy(ranked, block.Signals)
	sort.SliceStable(ranked, func(i, j int) bool {
		if ranked[i].Count != ranked[j].Count {
			return ranked[i].Count > ranked[j].Count
		}
		if ranked[i].Kind != ranked[j].Kind {
			return ranked[i].Kind < ranked[j].Kind
		}
		if ranked[i].Module != ranked[j].Module {
			return ranked[i].Module < ranked[j].Module
		}
		return ranked[i].Target < ranked[j].Target
	})
	for i, s := range ranked {
		if i == runtimeAsyncTopN {
			fmt.Fprintf(b, "- ... +%d more signals (use `--format json`)\n", len(ranked)-runtimeAsyncTopN)
			break
		}
		target := ""
		if s.Target != "" {
			target = " → `" + mdTableCell(s.Target) + "`"
		}
		fmt.Fprintf(b, "- **%s**%s [%s; related: %s; measured=%t]: %d", s.Module, target, s.Kind, strings.Join(s.RelatedConnascence, "/"), s.Measured, s.Count)
		if len(s.Sites) > 0 {
			fmt.Fprintf(b, " (e.g. %s)", dynamicConnascenceSites(s.Sites))
		}
		b.WriteByte('\n')
	}
}

func dynamicConnascenceSites(sites []report.DynamicConnascenceSite) string {
	parts := make([]string, 0, runtimeAsyncSampleN)
	for i, s := range sites {
		if i == runtimeAsyncSampleN {
			break
		}
		parts = append(parts, fmt.Sprintf("%s:%d[%s]", s.File, s.Line, s.Kind))
	}
	return strings.Join(parts, ", ")
}

func formatCounts(counts map[string]int) string {
	keys := make([]string, 0, len(counts))
	for k := range counts {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	parts := make([]string, 0, len(keys))
	for _, k := range keys {
		parts = append(parts, fmt.Sprintf("%s=%d", k, counts[k]))
	}
	return strings.Join(parts, ", ")
}

func writeSemanticStrengthOverlay(b *strings.Builder, overlay *report.SemanticStrengthOverlay) {
	if overlay == nil || len(overlay.ByLanguage) == 0 {
		return
	}
	b.WriteString("\n## Semantic strength overlay\n\n")
	b.WriteString("Report-only. SCIP refines extractor strength hints; these counters never gate directly.\n\n")
	languages := make([]string, 0, len(overlay.ByLanguage))
	for language := range overlay.ByLanguage {
		languages = append(languages, language)
	}
	sort.Strings(languages)
	for _, language := range languages {
		stats := overlay.ByLanguage[language]
		fmt.Fprintf(b, "- %s: candidates=%d, applied=%d, missed=%d", language, stats.CandidateEdges, stats.Applied, stats.Missed)
		if len(stats.Before) > 0 {
			fmt.Fprintf(b, ", before: %s", formatCounts(stats.Before))
		}
		if len(stats.After) > 0 {
			fmt.Fprintf(b, ", after: %s", formatCounts(stats.After))
		}
		b.WriteByte('\n')
	}
}

// dynamicImportTopN is the number of modules listed in the dynamic-imports section.
const dynamicImportTopN = 10

// dynamicImportSampleN is the number of sample sites shown per module.
const dynamicImportSampleN = 3

// writeDynamicImports prints the report-only dynamic/lazy-import risk block:
// modules with non-top-level imports (Python) or require()/dynamic import() (TS),
// which are invisible to the static dependency graph and can hide cycles or
// undercount coupling. Counts + sample sites only — no risk verdict; the full
// list is in `--format json`. Omitted when none were found.
func writeDynamicImports(b *strings.Builder, dyn []report.DynamicImport) {
	if len(dyn) == 0 {
		return
	}
	total := 0
	for _, d := range dyn {
		total += d.Count
	}
	b.WriteString("\n## Dynamic / lazy imports (hidden-coupling risk)\n\n")
	b.WriteString("Report-only. Dynamic/lazy imports are invisible to the static dependency\n")
	b.WriteString("graph, so they can hide cycles and undercount coupling.\n\n")
	fmt.Fprintf(b, "%d sites across %d modules (full list in `--format json`):\n\n", total, len(dyn))

	ranked := make([]report.DynamicImport, len(dyn))
	copy(ranked, dyn)
	sort.SliceStable(ranked, func(i, j int) bool {
		if ranked[i].Count != ranked[j].Count {
			return ranked[i].Count > ranked[j].Count
		}
		return ranked[i].Module < ranked[j].Module
	})
	for i, d := range ranked {
		if i == dynamicImportTopN {
			fmt.Fprintf(b, "- ... +%d more modules (use `--format json`)\n", len(ranked)-dynamicImportTopN)
			break
		}
		fmt.Fprintf(b, "- **%s**: %d (e.g. %s)\n", d.Module, d.Count, sampleSites(d.Sites))
	}
}

// runtimeAsyncTopN is the number of module→runtime-target links listed in the
// runtime async section.
const runtimeAsyncTopN = 10

// runtimeAsyncSampleN is the number of sample sites shown per runtime edge.
const runtimeAsyncSampleN = 3

// writeRuntimeAsync prints the report-only runtime async bridge block. The
// module rollup preserves the historical view; the relationship-level edges add
// a concrete source-module → runtime-target fact set for future runtime-distance
// review without changing today's score.
func writeRuntimeAsync(b *strings.Builder, modules []report.RuntimeAsyncModule, edges []report.RuntimeAsyncEdge) {
	if len(modules) == 0 && len(edges) == 0 {
		return
	}
	total := 0
	for _, m := range modules {
		total += m.Count
	}
	b.WriteString("\n## Runtime async bridges (report-only)\n\n")
	b.WriteString("Report-only. Runtime async evidence is grouped by module and by concrete\n")
	b.WriteString("module→runtime-target relation; it never changes distance, score, or gate verdicts.\n\n")
	fmt.Fprintf(b, "%d sites across %d modules", total, len(modules))
	if len(edges) > 0 {
		fmt.Fprintf(b, " and %d module→target relation(s)", len(edges))
	}
	b.WriteString(". Full list in `--format json`.\n\n")

	ranked := make([]report.RuntimeAsyncEdge, len(edges))
	copy(ranked, edges)
	sort.SliceStable(ranked, func(i, j int) bool {
		if ranked[i].Count != ranked[j].Count {
			return ranked[i].Count > ranked[j].Count
		}
		if ranked[i].FromModule != ranked[j].FromModule {
			return ranked[i].FromModule < ranked[j].FromModule
		}
		if ranked[i].Target != ranked[j].Target {
			return ranked[i].Target < ranked[j].Target
		}
		return ranked[i].IntegrationKind < ranked[j].IntegrationKind
	})
	for i, e := range ranked {
		if i == runtimeAsyncTopN {
			fmt.Fprintf(b, "- ... +%d more relations (use `--format json`)\n", len(ranked)-runtimeAsyncTopN)
			break
		}
		fmt.Fprintf(b, "- **%s** → `%s` [%s]: %d (e.g. %s)\n", e.FromModule, mdTableCell(e.Target), e.IntegrationKind, e.Count, runtimeAsyncSites(e.Sites))
	}
}

func runtimeAsyncSites(sites []report.RuntimeAsyncSite) string {
	parts := make([]string, 0, runtimeAsyncSampleN)
	for i, s := range sites {
		if i == runtimeAsyncSampleN {
			break
		}
		parts = append(parts, fmt.Sprintf("%s:%d[%s]", s.File, s.Line, s.IntegrationKind))
	}
	return strings.Join(parts, ", ")
}

func writeDistanceConfigCandidates(b *strings.Builder, candidates []report.DistanceConfigCandidate) {
	if len(candidates) == 0 {
		return
	}
	total := 0
	for _, c := range candidates {
		total += c.Count
	}
	b.WriteString("\n## Distance config candidates (review-only)\n\n")
	b.WriteString("Report-only. Static external, runtime, and dynamic evidence can suggest `external_systems` or `deploy_unit` review, but these candidates never change distance, score, or gate verdicts.\n\n")
	fmt.Fprintf(b, "%d signal(s) across %d candidate(s):\n\n", total, len(candidates))

	ranked := make([]report.DistanceConfigCandidate, len(candidates))
	copy(ranked, candidates)
	sort.SliceStable(ranked, func(i, j int) bool {
		if ranked[i].Count != ranked[j].Count {
			return ranked[i].Count > ranked[j].Count
		}
		if ranked[i].SourceBlock != ranked[j].SourceBlock {
			return ranked[i].SourceBlock < ranked[j].SourceBlock
		}
		if ranked[i].Module != ranked[j].Module {
			return ranked[i].Module < ranked[j].Module
		}
		if ranked[i].Target != ranked[j].Target {
			return ranked[i].Target < ranked[j].Target
		}
		return ranked[i].IntegrationKind < ranked[j].IntegrationKind
	})
	for i, c := range ranked {
		if i == runtimeAsyncTopN {
			fmt.Fprintf(b, "- ... +%d more candidates (use `--format json`)\n", len(ranked)-runtimeAsyncTopN)
			break
		}
		fmt.Fprintf(b, "- **%s** → `%s` [%s from %s; action=%s]: %d", c.Module, mdTableCell(c.Target), c.IntegrationKind, c.SourceBlock, c.SuggestedReviewAction, c.Count)
		if len(c.EvidenceSites) > 0 {
			fmt.Fprintf(b, " (e.g. %s)", distanceCandidateSites(c.EvidenceSites))
		}
		b.WriteByte('\n')
	}
}

func distanceCandidateSites(sites []report.DistanceConfigEvidenceSite) string {
	parts := make([]string, 0, runtimeAsyncSampleN)
	for i, s := range sites {
		if i == runtimeAsyncSampleN {
			break
		}
		parts = append(parts, fmt.Sprintf("%s:%d[%s]", s.File, s.Line, s.Kind))
	}
	return strings.Join(parts, ", ")
}

func writeVolatilityCorroboration(b *strings.Builder, block *report.VolatilityCorroboration) {
	if block == nil {
		return
	}
	b.WriteString("\n## Volatility corroboration (report-only)\n\n")
	b.WriteString("Report-only. Source-control touch frequency is supporting evidence for Ch9 volatility judgments and never changes score or gate verdicts.\n\n")
	fmt.Fprintf(b, "- source: %s\n", block.Source)
	fmt.Fprintf(b, "- status: %s\n", block.Status)
	if block.CommitWindow > 0 {
		fmt.Fprintf(b, "- recent-history window: %d commits\n", block.CommitWindow)
	}
	if block.FullHistory {
		b.WriteString("- history scope: full history fallback\n")
	}
	if block.CommitsScanned > 0 {
		fmt.Fprintf(b, "- commits scanned: %d\n", block.CommitsScanned)
	}
	if block.ModulesTouched > 0 {
		fmt.Fprintf(b, "- modules touched: %d\n", block.ModulesTouched)
	}
	if block.Caveat != "" {
		fmt.Fprintf(b, "- caveat: %s\n", block.Caveat)
	}
	if len(block.TopTouched) == 0 {
		return
	}
	b.WriteString("\nTop touched modules:\n\n")
	for _, item := range block.TopTouched {
		fmt.Fprintf(b, "- **%s**: %d commit(s)", item.Module, item.TouchCommits)
		if item.DeclaredVolatility != "" {
			fmt.Fprintf(b, " [declared volatility=%s]", item.DeclaredVolatility)
		}
		b.WriteByte('\n')
	}
}

// writeDeprecatedDeps prints the report-only locally-declared deprecation/
// mdTableCell escapes pipe characters and collapses newlines in a string so it
// can be safely embedded in a Markdown table cell without corrupting the table.
func mdTableCell(s string) string {
	s = strings.ReplaceAll(s, "\n", " ")
	s = strings.ReplaceAll(s, "\r", "")
	s = strings.ReplaceAll(s, "|", `\|`)
	return s
}

// retraction marker block: go.mod retract directives and package.json
// "deprecated" fields. Omitted when none were found.
// Never gates — evidence only.
func writeDeprecatedDeps(b *strings.Builder, deps []report.DeprecatedDep) {
	if len(deps) == 0 {
		return
	}
	b.WriteString("\n## Manifest deprecation markers (report-only)\n\n")
	b.WriteString("Locally-declared deprecation/retraction markers found in checked-in manifest files.\n")
	b.WriteString("Report-only evidence — never gates. Cargo yanked and live EOL require archfit analyze --ai-summary / enrich.\n\n")
	fmt.Fprintf(b, "| file | kind | subject | note |\n")
	fmt.Fprintf(b, "|------|------|---------|------|\n")
	for _, d := range deps {
		note := d.Note
		if note == "" {
			note = "—"
		}
		fmt.Fprintf(b, "| `%s` | %s | `%s` | %s |\n", mdTableCell(d.File), d.Kind, mdTableCell(d.Subject), mdTableCell(note))
	}
}

// writeCoverageGaps renders the warn-loud "Coverage gaps" section: one line per
// analyzer that did not run, the metrics its absence leaves unmeasured, and how
// to install it. This is what turns archfit's silent degradation into a visible,
// actionable list — a missing tool is reported, never scored as good architecture.
// Omitted when no gap was recorded.
func writeCoverageGaps(b *strings.Builder, gaps []report.CoverageGap) {
	if len(gaps) == 0 {
		return
	}
	fmt.Fprintf(b, "\n## Coverage gaps (%d)\n\n", len(gaps))
	b.WriteString("Analyzers that did not run. Their metrics are reported as n/a (never green) — install to measure them.\n\n")
	for _, g := range gaps {
		fmt.Fprintf(b, "- **%s** [gate: %s] — affects %s\n", g.Tool, g.Gate, strings.Join(g.AffectedMetrics, ", "))
		fmt.Fprintf(b, "  - install: `%s`\n", g.InstallCmd)
	}
}

const configWarningModuleSampleLimit = 15

// writeConfigWarnings renders advisory config-quality warnings (under-specified
// modules, swallowed optional-tool errors) so they reach the report and CI
// instead of being stderr-only. Advisory — never gates. Omitted when empty.
func writeConfigWarnings(b *strings.Builder, warnings []string) {
	if len(warnings) == 0 {
		return
	}
	fmt.Fprintf(b, "\n## Config warnings (%d)\n\n", len(warnings))
	b.WriteString("Advisory — never gates. Under-specified modules degrade distance/volatility classification.\n\n")
	for _, group := range groupConfigWarnings(warnings) {
		if len(group.modules) <= 1 {
			fmt.Fprintf(b, "- %s\n", group.first)
			continue
		}
		shown := min(len(group.modules), configWarningModuleSampleLimit)
		fmt.Fprintf(b, "- modules omitting %s: %s", group.omits, quoteJoin(group.modules[:shown]))
		if hidden := len(group.modules) - shown; hidden > 0 {
			fmt.Fprintf(b, ", …and %d more modules", hidden)
		}
		b.WriteByte('\n')
	}
}

type configWarningGroup struct {
	first   string
	omits   string
	modules []string
}

func groupConfigWarnings(warnings []string) []configWarningGroup {
	groups := make([]configWarningGroup, 0, len(warnings))
	byKey := make(map[string]int)
	for _, w := range warnings {
		moduleName, omits, ok := parseModuleOmissionWarning(w)
		if !ok {
			groups = append(groups, configWarningGroup{first: w})
			continue
		}
		key := "module omits " + omits
		idx, ok := byKey[key]
		if !ok {
			byKey[key] = len(groups)
			groups = append(groups, configWarningGroup{first: w, omits: omits})
			idx = len(groups) - 1
		}
		groups[idx].modules = append(groups[idx].modules, moduleName)
	}
	return groups
}

func parseModuleOmissionWarning(w string) (moduleName, omits string, ok bool) {
	const prefix = `module "`
	const middle = `" omits `
	if !strings.HasPrefix(w, prefix) {
		return "", "", false
	}
	rest := strings.TrimPrefix(w, prefix)
	moduleName, omits, found := strings.Cut(rest, middle)
	if !found || moduleName == "" || omits == "" {
		return "", "", false
	}
	return moduleName, omits, true
}

func quoteJoin(values []string) string {
	quoted := make([]string, 0, len(values))
	for _, v := range values {
		quoted = append(quoted, fmt.Sprintf("%q", v))
	}
	return strings.Join(quoted, ", ")
}

// sampleSites renders up to dynamicImportSampleN sites as "file:line[kind]".
func sampleSites(sites []report.DynamicImportSite) string {
	parts := make([]string, 0, dynamicImportSampleN)
	for i, s := range sites {
		if i == dynamicImportSampleN {
			break
		}
		parts = append(parts, fmt.Sprintf("%s:%d[%s]", s.File, s.Line, s.Kind))
	}
	return strings.Join(parts, ", ")
}
