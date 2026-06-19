// Package markdown renders a Diagnostic as Balanced-Coupling-aligned Markdown:
// lint-message advisory format, BC vocabulary throughout, clearly-labeled
// "Supporting structural metrics (beyond Balanced Coupling)" and
// "Distance confidence" sections. Reads well both as raw text and rendered
// Markdown; parseable by AI agents without NLP.
package markdown

import (
	"fmt"
	"io"
	"sort"
	"strconv"
	"strings"

	"github.com/alexei-led/archfit/internal/model/diagnostic"
	"github.com/alexei-led/archfit/internal/model/finding"
)

// confidenceHigh is the confidence value that needs no qualification in output.
const confidenceHigh = "high"

// beyondBCMetrics is the set of metric names that belong in the
// "Supporting structural metrics (beyond Balanced Coupling)" section.
// These are report-only and never gate. Everything else appears in the
// primary Metrics section (coupling_balance, encapsulation, etc.).
var beyondBCMetrics = map[string]bool{
	"cycle":                true,
	"blast_radius":         true,
	"propagation_cost":     true,
	"instability":          true,
	"abstractness":         true,
	"martin_distance":      true,
	"change_coupling":      true,
	"change_amplification": true,
	"hidden_coupling":      true,
	"structural_weight":    true,
	"complexity":           true,
	"risk_hub":             true,
	"coverage":             true,
}

// Renderer formats a Diagnostic as BC-aligned Markdown. Satisfies engine.Renderer.
type Renderer struct{}

// New returns a Renderer.
func New() *Renderer { return &Renderer{} }

// Format returns "markdown".
func (r *Renderer) Format() string { return "markdown" }

// Render writes the BC-aligned Markdown report for d to w.
// Sections follow design §8:
//  1. Verdict + config_hash + tool/coverage
//  2. Gate violations (rules)
//  3. Balanced Coupling advisories — lint-message format
//  4. Supporting structural metrics (beyond Balanced Coupling)
//  5. Distance confidence
//  6. Agent tasks
func (r *Renderer) Render(d diagnostic.Diagnostic, w io.Writer) error {
	var b strings.Builder
	verdict, exitCode := verdictLabel(d.Verdict)

	b.WriteString("# archfit report\n\n")
	fmt.Fprintf(&b, "**Verdict:** %s (exit %d)\n", verdict, exitCode)
	if d.ConfigHash != "" {
		fmt.Fprintf(&b, "**Config hash:** `%s`\n", d.ConfigHash)
	}

	b.WriteString("\n## Summary\n\n")
	fmt.Fprintf(&b, "- gate findings: %d\n", d.Summary.GateFindings)
	fmt.Fprintf(&b, "- warnings: %d\n", d.Summary.Warnings)
	fmt.Fprintf(&b, "- exceptions used: %d\n", d.Summary.ExceptionsUsed)

	// Split metrics: BC-primary vs beyond-BC.
	var primaryMetrics, beyondMetrics []diagnostic.MetricResult
	for _, m := range d.Metrics {
		if beyondBCMetrics[m.Name] {
			beyondMetrics = append(beyondMetrics, m)
		} else {
			primaryMetrics = append(primaryMetrics, m)
		}
	}

	if len(primaryMetrics) > 0 {
		b.WriteString("\n## Metrics\n\n")
		for _, m := range primaryMetrics {
			band := m.Band
			if m.Confidence != "" && m.Confidence != confidenceHigh {
				band = fmt.Sprintf("%s (%s confidence)", band, m.Confidence)
			}
			fmt.Fprintf(&b, "- **%s**: %s — %s\n", m.Name, m.Display, band)
		}
	}

	writeFileFacts(&b, d.FileFacts)

	writeDynamicImports(&b, d.DynamicImports)

	gate, advisories := splitFindings(d.Findings)
	if len(gate) > 0 {
		fmt.Fprintf(&b, "\n## Gate findings (%d)\n\n", len(gate))
		for _, f := range gate {
			writeGateFinding(&b, f)
		}
	}

	writeAgentTasks(&b, d.AgentTasks)

	if len(advisories) > 0 {
		writeBCAdvisories(&b, advisories)
	}

	if len(beyondMetrics) > 0 {
		writeBeyondBCMetrics(&b, beyondMetrics)
	}

	writeDistanceConfidence(&b, d)

	if len(d.ToolCoverage) > 0 {
		b.WriteString("\n## Coverage\n\n")
		for _, c := range d.ToolCoverage {
			extra := ""
			if c.FilesSeen > 0 {
				extra = fmt.Sprintf(" (%d files)", c.FilesSeen)
			}
			fmt.Fprintf(&b, "- %s: %s%s\n", c.Tool, c.Status, extra)
		}
	}

	_, err := io.WriteString(w, b.String())
	return err
}

// writeBCAdvisories renders coupling advisories in BC lint-message format.
// BC advisories (bc/imbalanced_coupling) get the structured lint-message block.
// All other advisories (staleness, map/*, labels/*) render as plain list items.
func writeBCAdvisories(b *strings.Builder, advisories []finding.Finding) {
	var bcFindings, otherFindings []finding.Finding
	for _, f := range advisories {
		if f.RuleID == "bc/imbalanced_coupling" {
			bcFindings = append(bcFindings, f)
		} else {
			otherFindings = append(otherFindings, f)
		}
	}

	if len(bcFindings) > 0 {
		edges := 0
		for _, f := range bcFindings {
			edges += rollupCount(f)
		}
		fmt.Fprintf(b, "\n## Balanced Coupling advisories (%d rollups, %d edges)\n\n", len(bcFindings), edges)
		b.WriteString("Same-shape edges between a module pair are grouped into one rollup.\n")
		b.WriteString("Integration strength × distance × volatility lint messages.\n")
		b.WriteString("Severity: `none` · `low` · `medium` · `high` · `critical`.\n\n")
		for i, f := range bcFindings {
			if i == 25 {
				fmt.Fprintf(b, "- ... +%d more rollups (use `--format json`)\n", len(bcFindings)-25)
				break
			}
			writeBCLintMessage(b, f)
		}
	}

	if len(otherFindings) > 0 {
		fmt.Fprintf(b, "\n## Advisories (%d)\n\n", len(otherFindings))
		for i, f := range otherFindings {
			if i == 25 {
				fmt.Fprintf(b, "- ... +%d more (use `--format json`)\n", len(otherFindings)-25)
				break
			}
			writeGateFinding(b, f)
		}
	}
}

// writeBCLintMessage renders one bc/imbalanced_coupling finding as a BC lint message:
//
//	ARCHFIT[BC-UNBALANCED <SEV>] from -> to  [<id8>]
//	  integration strength: <s>   distance: <d>   volatility: <v>
//	  score: <value>/10 (<band>) [<scorer>]
//	  why: <why>
//	  cheapest move: <move>
func writeBCLintMessage(b *strings.Builder, f finding.Finding) {
	from := f.Edge.From.Path
	to := f.Edge.To.Path
	if from == "" {
		from = f.Edge.From.Module
	}
	if to == "" {
		to = f.Edge.To.Module
	}

	sev := strings.ToUpper(string(f.Severity))
	idShort := f.ID
	if len(idShort) > 8 {
		idShort = idShort[:8]
	}
	fmt.Fprintf(b, "```\nARCHFIT[BC-UNBALANCED %s] %s -> %s  [%s]\n", sev, from, to, idShort)

	// Strength / distance / volatility from MatchedBy.
	strength := f.MatchedBy["strength"]
	distance := f.MatchedBy["distance"]
	volatility := f.MatchedBy["volatility"]
	scorer := f.MatchedBy["score"] // scorer name (e.g. "multiplicative")
	scoreValue := f.MatchedBy["score_value"]
	scoreBand := f.MatchedBy["score_band"]
	cheapestMove := f.MatchedBy["cheapest_move"]
	why := strings.TrimSpace(f.Why)

	if strength != "" || distance != "" || volatility != "" {
		fmt.Fprintf(b, "  integration strength: %-12s  distance: %-30s  volatility: %s\n",
			orUnknown(strength), orUnknown(distance), orUnknown(volatility))
	}
	switch {
	case scoreValue != "":
		// Numeric maintenance-effort score: <value>/10 (<band>) [<scorer>].
		fmt.Fprintf(b, "  score: %s/10 (%s) [%s]\n", scoreValue, orUnknown(scoreBand), orUnknown(scorer))
	case scorer != "":
		fmt.Fprintf(b, "  score: %s\n", scorer)
	}
	if why != "" {
		if len(why) > 200 {
			why = why[:197] + "..."
		}
		fmt.Fprintf(b, "  why: %s\n", why)
	}
	if cheapestMove != "" {
		fmt.Fprintf(b, "  cheapest move: %s\n", cheapestMove)
	}
	if n := rollupCount(f); n > 1 {
		members := f.MatchedBy["group_members"]
		if members != "" {
			fmt.Fprintf(b, "  rollup: %d same-shape edges (e.g. %s)\n", n, members)
		} else {
			fmt.Fprintf(b, "  rollup: %d same-shape edges\n", n)
		}
	}
	b.WriteString("```\n\n")
}

// rollupCount returns the number of edges a BC advisory represents: the value of
// MatchedBy["group_count"] when the advisory is a rollup, else 1.
func rollupCount(f finding.Finding) int {
	if n, err := strconv.Atoi(f.MatchedBy["group_count"]); err == nil && n > 0 {
		return n
	}
	return 1
}

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
	// Collect distance-signal sources from tool coverage entries.
	ownerSrc := ""
	deployUnitSrc := ""
	unresolved := 0
	for _, cov := range d.ToolCoverage {
		switch cov.Tool {
		case "ownership":
			ownerSrc = cov.Status
		case "deploy-unit":
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

// writeAgentTasks prints the structured repair-task block: one entry per
// active gate finding, with goal, involved files, constraints, and the exact
// validation command. Omitted when there are no active gate findings.
func writeAgentTasks(b *strings.Builder, tasks []diagnostic.AgentTask) {
	if len(tasks) == 0 {
		return
	}
	fmt.Fprintf(b, "\n## Agent tasks (%d)\n\n", len(tasks))
	for _, task := range tasks {
		fmt.Fprintf(b, "- **%s** [`%s`] %s\n", task.RuleID, task.FindingID[:min(8, len(task.FindingID))], task.Goal)
		if len(task.Files) > 0 {
			fmt.Fprintf(b, "  - files: %s\n", strings.Join(task.Files, ", "))
		}
		for _, c := range task.Constraints {
			fmt.Fprintf(b, "  - constraint: %s\n", c)
		}
		for _, v := range task.Validation {
			fmt.Fprintf(b, "  - validate: `%s`\n", v)
		}
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

// writeGateFinding prints one gate or non-BC advisory finding as a Markdown list item.
func writeGateFinding(b *strings.Builder, f finding.Finding) {
	edge := ""
	if f.Edge.From.Path != "" || f.Edge.To.Path != "" {
		edge = fmt.Sprintf(" — %s → %s", f.Edge.From.Path, f.Edge.To.Path)
	}
	why := strings.TrimSpace(f.Why)
	if len(why) > 140 {
		why = why[:137] + "..."
	}
	if why != "" {
		why = ": " + why
	}
	fmt.Fprintf(b, "- **%s** [%s] %s%s%s\n", f.RuleID, f.Severity, f.Status, edge, why)
}

func splitFindings(fs []finding.Finding) (gate, advisories []finding.Finding) {
	for _, f := range fs {
		if f.Kind == "gate" {
			gate = append(gate, f)
		} else {
			advisories = append(advisories, f)
		}
	}
	// Both lists get a total deterministic order so output never depends on the
	// incoming slice order (which originates from map iteration upstream).
	sort.SliceStable(gate, func(i, j int) bool { return findingLess(gate[i], gate[j]) })
	sort.SliceStable(advisories, func(i, j int) bool { return findingLess(advisories[i], advisories[j]) })
	return gate, advisories
}

// findingLess orders findings deterministically: severity descending, then a
// stable tie-break chain (rule id, status, from path, to path, edge kind, id).
// id is a unique fingerprint, so the order is total — equal-severity findings
// no longer depend on input order.
func findingLess(a, b finding.Finding) bool {
	if ra, rb := severityRank(a.Severity), severityRank(b.Severity); ra != rb {
		return ra > rb
	}
	if a.RuleID != b.RuleID {
		return a.RuleID < b.RuleID
	}
	if a.Status != b.Status {
		return a.Status < b.Status
	}
	if a.Edge.From.Path != b.Edge.From.Path {
		return a.Edge.From.Path < b.Edge.From.Path
	}
	if a.Edge.To.Path != b.Edge.To.Path {
		return a.Edge.To.Path < b.Edge.To.Path
	}
	if a.Edge.Kind != b.Edge.Kind {
		return a.Edge.Kind < b.Edge.Kind
	}
	return a.ID < b.ID
}

func severityRank(s finding.Severity) int {
	switch s {
	case "critical":
		return 4
	case "high":
		return 3
	case "medium":
		return 2
	case "low":
		return 1
	default:
		return 0
	}
}

func verdictLabel(v diagnostic.Verdict) (string, int) {
	switch v {
	case diagnostic.VerdictFail:
		return "fail", 1
	case diagnostic.VerdictWarn:
		return "warn", 2
	default:
		return "pass", 0
	}
}

// orUnknown returns s if non-empty, else "unknown".
func orUnknown(s string) string {
	if s == "" {
		return "unknown"
	}
	return s
}
