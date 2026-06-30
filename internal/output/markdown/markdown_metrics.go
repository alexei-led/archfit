package markdown

import (
	"fmt"
	"sort"
	"strings"

	"github.com/alexei-led/archfit/internal/model/diagnostic"
)

// writeBeyondBCMetrics renders the "Supporting structural metrics (beyond Balanced
// Coupling)" section. These are report-only and never gate.
func writeBeyondBCMetrics(b *strings.Builder, metrics []diagnostic.MetricResult) {
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
func writeDistanceConfidence(b *strings.Builder, d diagnostic.Diagnostic) {
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
	if unresolved > 0 {
		fmt.Fprintf(b, "- unresolved modules: %d (edges to unknown modules use conservative distance)\n", unresolved)
	}
}

// fileFactsTopN is the number of modules listed per axis in the structural-facts section.
const fileFactsTopN = 5

// writeFileFacts prints the neutral structural-facts block: the top modules by
// each axis (inbound module fan-in, outbound destinations, LOC). Numbers only —
// no risk labels or ranking verdicts; the full per-module list is in the JSON
// output. Omitted entirely when no facts were collected (SCIP off/absent).
func writeFileFacts(b *strings.Builder, facts []diagnostic.FileFact) {
	if len(facts) == 0 {
		return
	}
	b.WriteString("\n## Structural facts (neutral evidence)\n\n")
	fmt.Fprintf(b, "%d modules; top %d per axis (full list in `--format json`):\n\n", len(facts), fileFactsTopN)

	axes := []struct {
		label string
		value func(diagnostic.FileFact) int
	}{
		{"inbound module fan-in", func(f diagnostic.FileFact) int { return f.InboundModuleFanIn }},
		{"outbound destinations", func(f diagnostic.FileFact) int { return f.OutboundDestinations }},
		{"LOC", func(f diagnostic.FileFact) int { return f.LOC }},
	}
	for _, axis := range axes {
		ranked := make([]diagnostic.FileFact, len(facts))
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
func writeSyntaxSurface(b *strings.Builder, facts []diagnostic.SyntaxFact) {
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
	var exported []diagnostic.SyntaxFact
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

// dynamicImportTopN is the number of modules listed in the dynamic-imports section.
const dynamicImportTopN = 10

// dynamicImportSampleN is the number of sample sites shown per module.
const dynamicImportSampleN = 3

// writeDynamicImports prints the report-only dynamic/lazy-import risk block:
// modules with non-top-level imports (Python) or require()/dynamic import() (TS),
// which are invisible to the static dependency graph and can hide cycles or
// undercount coupling. Counts + sample sites only — no risk verdict; the full
// list is in `--format json`. Omitted when none were found.
func writeDynamicImports(b *strings.Builder, dyn []diagnostic.DynamicImport) {
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

	ranked := make([]diagnostic.DynamicImport, len(dyn))
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
func writeDeprecatedDeps(b *strings.Builder, deps []diagnostic.DeprecatedDep) {
	if len(deps) == 0 {
		return
	}
	b.WriteString("\n## Manifest deprecation markers (report-only)\n\n")
	b.WriteString("Locally-declared deprecation/retraction markers found in checked-in manifest files.\n")
	b.WriteString("Report-only evidence — never gates. Cargo yanked and live EOL require archfit analyze --llm / enrich.\n\n")
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
func writeCoverageGaps(b *strings.Builder, gaps []diagnostic.CoverageGap) {
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

// writeConfigWarnings renders advisory config-quality warnings (under-specified
// modules, swallowed optional-tool errors) so they reach the report and CI
// instead of being stderr-only. Advisory — never gates. Omitted when empty.
func writeConfigWarnings(b *strings.Builder, warnings []string) {
	if len(warnings) == 0 {
		return
	}
	fmt.Fprintf(b, "\n## Config warnings (%d)\n\n", len(warnings))
	b.WriteString("Advisory — never gates. Under-specified modules degrade distance/volatility classification.\n\n")
	for _, w := range warnings {
		fmt.Fprintf(b, "- %s\n", w)
	}
}

// sampleSites renders up to dynamicImportSampleN sites as "file:line[kind]".
func sampleSites(sites []diagnostic.DynamicImportSite) string {
	parts := make([]string, 0, dynamicImportSampleN)
	for i, s := range sites {
		if i == dynamicImportSampleN {
			break
		}
		parts = append(parts, fmt.Sprintf("%s:%d[%s]", s.File, s.Line, s.Kind))
	}
	return strings.Join(parts, ", ")
}
