package score

import (
	"fmt"
	"math"
	"sort"
	"strings"

	"github.com/alexei-led/archfit/internal/model/coupling"
	"github.com/alexei-led/archfit/internal/model/diagnostic"
	"github.com/alexei-led/archfit/internal/model/finding"
)

// boundary_integrity baselines used when encapsulation is structurally n/a
// (compiler-enforced package boundaries) and the import-cycle signal stands in.
// Held at medium confidence in that path, so boundaryCleanNoCycles reads
// "serviceable" (not "strong" — contract discipline stays unverified) and
// boundaryWithCycles reads "poor".
const (
	boundaryCleanNoCycles = 75 // 0 cycles + 0 gate violations: clean structural boundary
	boundaryWithCycles    = 40 // import cycles cross intended boundaries
)

// boundaryIntegrity scores whether code respects intended module/layer/ownership
// boundaries. Encapsulation (contract / (contract+intrusive) cross-boundary
// edges) sets the baseline; each active gate violation (a forbidden dependency
// that crossed a boundary) is a measured breach and subtracts a fixed penalty.
func boundaryIntegrity(mi metricIndex, gate []finding.Finding, base Confidence) Dimension {
	dim := Dimension{Name: DimBoundaryIntegrity, Confidence: base}
	// On a <2-module graph the encapsulation metric reports a vacuous 1.0 ("no
	// cross-boundary edges, so nothing leaks"). That is absence of evidence, not
	// earned encapsulation, so don't let it produce a strong band. Gate findings,
	// if any, are still real boundary breaches and take the normal path below.
	if degenerateGraph(mi) && len(gate) == 0 {
		return Dimension{
			Name: DimBoundaryIntegrity, Value: 50, Confidence: ConfidenceLow,
			Evidence: []string{"encapsulation vacuous: no cross-module boundaries on a graph with fewer than two connected modules"},
			Summary:  "boundary integrity unconfirmed: no internal module boundaries to assess",
		}
	}
	var value int
	// baselineMeasured: a real boundary baseline was established (from
	// encapsulation, or — when that is structurally n/a — from the import-cycle
	// signal), so 0 gate violations means "boundaries hold", not "unconfirmed".
	baselineMeasured := true
	if enc, ok := mi.measured("encapsulation"); ok {
		value = pct(enc.Value)
		dim.Evidence = append(dim.Evidence, fmt.Sprintf("encapsulation %.2f (%s)", enc.Value, enc.Band))
		dim.Confidence = minConf(base, metricConf(enc.Confidence))
	} else if cyc, ok := mi.measured("cycle"); ok {
		// Encapsulation is structurally unmeasurable in the common compiler-enforced
		// case (Go/TS: every cross-package reference goes through an exported API, so
		// there are no intrusive edges to form the contract/(contract+intrusive)
		// ratio). Rather than cap at a neutral, unconfirmed 50, assess the boundary
		// from the signal that IS measurable here: import cycles. Circular
		// dependencies are boundary breaks; their absence is a real (if partial)
		// boundary-health signal. Confidence is held at medium — contract discipline
		// (no model leakage through public types) stays unverified.
		if cyc.Value == 0 {
			value = boundaryCleanNoCycles
			dim.Evidence = append(dim.Evidence,
				"encapsulation n/a (compiler-enforced package boundary: no intrusive edges to score); 0 import cycles")
		} else {
			value = boundaryWithCycles
			dim.Evidence = append(dim.Evidence,
				fmt.Sprintf("encapsulation n/a; %d import cycle(s) cross intended boundaries", int(cyc.Value)))
		}
		dim.Confidence = minConf(base, ConfidenceMedium)
	} else {
		// Neither encapsulation nor cycles measurable — no boundary signal at all.
		baselineMeasured = false
		value = 50
		dim.Evidence = append(dim.Evidence,
			"encapsulation and import cycles both n/a — boundary baseline unmeasured")
		dim.Confidence = ConfidenceLow
	}

	if n := len(gate); n > 0 {
		pen := capInt(n*15, 70)
		value -= pen
		dim.Evidence = append(dim.Evidence, fmt.Sprintf("%d active gate violation(s): %s", n, sampleIDs(gate)))
		dim.Summary = "boundary violations present: forbidden dependencies cross intended boundaries"
	} else {
		dim.Evidence = append(dim.Evidence, "0 active gate violations")
		if baselineMeasured {
			dim.Summary = "no gate-level boundary violations; intended boundaries hold"
		} else {
			dim.Summary = "no gate-level boundary violations; encapsulation unmeasured, so boundary integrity is unconfirmed"
		}
	}
	dim.Value = value
	return dim
}

// couplingBalance scores integration strength vs distance vs volatility using
// Vlad Khononov's balance formula from _Balancing Coupling in Software Design_ Ch10.
//
// When a ClassifiedEdgeSummary is supplied (populated from the full coupling.Index
// before advisory filtering), the value comes from the mean book balance over all
// scored cross-boundary edges:
//
//	value = round(100 × (MeanBalance − 1) / 9)
//
// This is a transparent linear rescale of the book's own 1–10 per-edge score
// (balance 1→value 0, balance 10→value 100). Confidence scales with the scored
// fraction (high ≥80%, medium 50–79%, low <50%). Zero scored edges → 60/mixed/low
// (unanalyzed sentinel). The advisory worst-edge cap (critical band) is still
// applied on top: any critical edge → cap at 60; pervasive (≥5% of
// scored+abstained) → cap at 40.
//
// When summary is nil (backward compat), the function falls back to the legacy
// advisory-edge path using the edges []bcEdge slice.
func couplingBalance(edges []bcEdge, mi metricIndex, summary *diagnostic.ClassifiedEdgeSummary) Dimension {
	dim := Dimension{Name: DimCouplingBalance}

	// Degenerate graph: <2 connected modules — coupling unmeasurable regardless of summary.
	if degenerateGraph(mi) {
		dim.Confidence = ConfidenceLow
		dim.Value = 50
		dim.Evidence = []string{"no classified coupling edges on a graph with fewer than two connected modules — coupling unmeasurable"}
		dim.Summary = "coupling balance unmeasured: no internal module structure"
		return dim
	}

	// Advisory worst-edge count — used for the cap logic and evidence in both paths.
	worst := 0
	for _, e := range edges {
		if e.worstCase() {
			worst += e.count
		}
	}

	// --- Distribution path (preferred): use the full classified-edge summary ---
	if summary != nil {
		// Internal cross-boundary edges only (external/library edges excluded from
		// coupling_balance — they are a dependency_graph_health concern, not a
		// coupling_balance concern; the book scores YOUR components only).
		crossBoundary := summary.Scored + summary.Abstained

		// Zero internal cross-boundary edges (or zero scored): unanalyzed sentinel.
		if summary.Scored == 0 {
			dim.Confidence = ConfidenceLow
			dim.Value = 60
			dim.Evidence = []string{
				"0 scored internal cross-boundary edges — coupling balance unconfirmed (edge classification absent or all edges abstained)",
				fmt.Sprintf("worst-case (critical band) edges: %d", worst),
			}
			if summary.External > 0 {
				dim.Evidence = append(dim.Evidence,
					fmt.Sprintf("%d external/library edges excluded — see dependency_graph_health", summary.External))
			}
			dim.Summary = "coupling balance unconfirmed: no scored cross-boundary edges"
			return dim
		}

		// value = round(100 × (MeanBalance − 1) / 9): linear rescale of book's 1–10.
		value := int(math.Round(100 * (summary.MeanBalance - 1) / 9))

		// Confidence from internal-only scored fraction (external edges do not count).
		var conf Confidence
		scoredPct := 100 * summary.Scored / crossBoundary
		switch {
		case scoredPct >= 80:
			conf = ConfidenceHigh
		case scoredPct >= 50:
			conf = ConfidenceMedium
		default:
			conf = ConfidenceLow
		}

		// Advisory cap. A genuine distributed monolith is the critical band AND high
		// distance (different owner / deploy unit); pervasive DM caps the value hard.
		// Critical edges at low distance (cross_module_same_owner) are local
		// high-strength/high-volatility coupling — still poor balance (cap at 60), but
		// the cascade is cheap (one owner, one binary) and it is NOT a distributed
		// monolith, so it must not trigger the pervasive-DM cap or label.
		criticalCount := summary.BySeverity[string(coupling.SeverityCritical)]
		dmCount := summary.DistributedMonolith
		switch {
		case crossBoundary > 0 && dmCount*100/crossBoundary >= 5:
			value = capInt(value, 40) // pervasive distributed-monolith risk
		case criticalCount > 0:
			value = capInt(value, 60) // critical-band coupling present (poor balance)
		}

		// LLM-provenance labels lower confidence by one band: the strength
		// classifications driving the balance came from an LLM judge (human-approved
		// but not human-judged). Only applied when LLM labels account for ≥20% of
		// scored edges so a single stray label does not flip a well-measured repo.
		llmConfLowered := summary.LLMApproved > 0 && summary.Scored > 0 &&
			summary.LLMApproved*100/summary.Scored >= 20
		if llmConfLowered {
			conf = lowerConf(conf)
		}

		dim.Value = value
		dim.Confidence = conf
		dim.Evidence = []string{
			fmt.Sprintf("%d scored internal cross-boundary edges; mean book balance %.1f/10 → value %d",
				summary.Scored, summary.MeanBalance, value),
			fmt.Sprintf("scored fraction: %d%% (%d scored, %d abstained, internal only)",
				scoredPct, summary.Scored, summary.Abstained),
			fmt.Sprintf("critical-band edges: %d (%d distributed-monolith: critical at high distance)",
				criticalCount, dmCount),
		}
		if summary.External > 0 {
			dim.Evidence = append(dim.Evidence,
				fmt.Sprintf("%d external/library edges excluded — see dependency_graph_health", summary.External))
		}
		if llmConfLowered {
			dim.Evidence = append(dim.Evidence,
				fmt.Sprintf("llm-provenance labels in effect: %d (confidence lowered)", summary.LLMApproved))
		} else if summary.LLMApproved > 0 {
			dim.Evidence = append(dim.Evidence,
				fmt.Sprintf("llm-provenance labels in effect: %d", summary.LLMApproved))
		}
		switch {
		case dmCount > 0:
			dim.Summary = "unbalanced coupling: distributed-monolith edges (high strength × high distance × high volatility) present"
		case criticalCount > 0:
			dim.Summary = "critical-band coupling at low distance — local high-strength/high-volatility coupling (cheap cascade), not a distributed monolith"
		default:
			dim.Summary = fmt.Sprintf("mean book balance %.1f/10 across %d scored internal cross-boundary edges",
				summary.MeanBalance, summary.Scored)
		}
		return dim
	}

	// Legacy fallback: advisory-edge path (summary == nil).
	// This path is unreachable from engine.go — the engine always populates
	// ClassifiedEdges. It exists for calibration test suites that construct
	// a bare Diagnostic without a ClassifiedEdges summary.
	// --- Legacy fallback: advisory-edge path (summary == nil) ---
	if len(edges) == 0 {
		dim.Confidence = ConfidenceLow
		dim.Value = 60
		dim.Evidence = []string{"no classified coupling edges — coupling balance unconfirmed (edge classification absent, e.g. SCIP not run, or all edges balanced)"}
		dim.Summary = "coupling balance unconfirmed: no classified cross-boundary edges"
		return dim
	}

	var totalEffort, totalEdges int
	for _, e := range edges {
		eff := e.effort
		if eff < 0 {
			eff = effortFromSeverity(e.severity)
		}
		totalEffort += eff * e.count
		totalEdges += e.count
	}
	meanEffort := float64(totalEffort) / float64(totalEdges)
	value := int(math.Round((1 - meanEffort/10) * 100))

	switch {
	case totalEdges > 0 && worst*100/totalEdges >= 5:
		value = capInt(value, 40)
	case worst > 0:
		value = capInt(value, 60)
	}

	dim.Value = value
	dim.Confidence = ConfidenceHigh
	dim.Evidence = []string{
		fmt.Sprintf("%d BC edges (%d rollups); weighted mean maintenance-effort %.1f/10", totalEdges, len(edges), meanEffort),
		fmt.Sprintf("worst-case high/high/high (distributed-monolith) edges: %d", worst),
	}
	if worst > 0 {
		dim.Summary = "unbalanced coupling: high strength × high distance × high volatility edges risk cascading changes"
	} else {
		dim.Summary = "coupling carries elevated maintenance effort but no distributed-monolith edges"
	}
	return dim
}

// bcEdge is a parsed Balanced-Coupling advisory edge (a rollup of count
// same-shape edges).
type bcEdge struct {
	strength, distance, volatility string
	band                           string // score_band
	severity                       string // finding severity
	effort                         int    // score_value 0-10, or -1 when absent
	count                          int    // group_count (≥1)
}

// worstCase reports whether the edge is the Balanced-Coupling worst case — high
// integration strength × high distance × high volatility — which the classifier
// scores as the critical band (a distributed-monolith risk).
func (e bcEdge) worstCase() bool {
	return e.severity == string(coupling.SeverityCritical) || e.band == string(coupling.SeverityCritical)
}

// bcEdges extracts the active Balanced-Coupling advisory edges from the findings,
// parsing the strength/distance/volatility/score fields the engine stamped into
// MatchedBy. Only active advisories (status new or expired_waiver) count —
// the same filter activeGateFindings and the gate verdict use. Baseline-accepted
// and waived edges are operator-suppressed debt; counting them would deflate
// coupling_balance for a repo that has triaged its coupling, diverging from the
// gate's own view. Fixed (resolved) advisories are skipped too.
func bcEdges(fs []finding.Finding) []bcEdge {
	var out []bcEdge
	for _, f := range fs {
		if f.RuleID != "bc/imbalanced_coupling" {
			continue
		}
		if !IsActiveGateFinding(f) {
			continue
		}
		e := bcEdge{
			strength:   f.MatchedBy["strength"],
			distance:   f.MatchedBy["distance"],
			volatility: f.MatchedBy["volatility"],
			band:       f.MatchedBy["score_band"],
			severity:   string(f.Severity),
			effort:     atoiDefault(f.MatchedBy["score_value"], -1),
			count:      atoiDefault(f.MatchedBy["group_count"], 1),
		}
		if e.count < 1 {
			e.count = 1
		}
		out = append(out, e)
	}
	return out
}

// activeGateFindings returns gate findings that still count against the verdict
// (status new or expired_waiver), sorted by ID for deterministic sampling.
func activeGateFindings(fs []finding.Finding) []finding.Finding {
	var out []finding.Finding
	for _, f := range fs {
		if f.Kind == finding.KindGate && IsActiveGateFinding(f) {
			out = append(out, f)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out
}

// IsActiveGateFinding reports whether f is an active gate finding — one that
// counts against the verdict (status new or expired_waiver). Shared with
// internal/decision so the recommendation buckets match the gate verdict.
func IsActiveGateFinding(f finding.Finding) bool {
	return f.Status == finding.StatusNew || f.Status == finding.StatusExpiredWaiver
}

// effortFromSeverity approximates a 0-10 maintenance-effort score for a BC edge
// with no scorer value, from its severity band.
func effortFromSeverity(sev string) int {
	switch sev {
	case string(coupling.SeverityCritical):
		return 9
	case string(coupling.SeverityHigh):
		return 7
	case string(coupling.SeverityMedium):
		return 5
	case string(coupling.SeverityLow):
		return 3
	default:
		return 5
	}
}

// sampleIDs renders up to three short finding IDs for evidence.
func sampleIDs(fs []finding.Finding) string {
	const maxIDs = 3
	ids := make([]string, 0, maxIDs+1)
	for i, f := range fs {
		if i == maxIDs {
			ids = append(ids, "...")
			break
		}
		id := f.ID
		if len(id) > 8 {
			id = id[:8]
		}
		ids = append(ids, id)
	}
	return strings.Join(ids, ", ")
}
