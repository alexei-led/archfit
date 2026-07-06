package score

import (
	"fmt"
	"math"

	"github.com/alexei-led/archfit/internal/model/coupling"
	"github.com/alexei-led/archfit/internal/model/diagnostic"
	"github.com/alexei-led/archfit/internal/model/finding"
)

const (
	minHighConfidenceScoredEdges      = 5
	minHighConfidenceConnectedModules = 3
)

// couplingBalance scores integration strength vs distance vs volatility using
// Vlad Khononov's balance formula from _Balancing Coupling in Software Design_ Ch10.
//
// When a ClassifiedEdgeSummary is supplied (populated from the full coupling.Index
// before advisory filtering, plus score-bearing clone-only duplicated-knowledge
// pairs when configured), the value comes from the mean book balance over all
// scored cross-boundary coupling facts:
//
//	value = round(100 × (MeanBalance − 1) / 9)
//
// This is a transparent linear rescale of the book's own 1–10 per-edge score
// (balance 1→value 0, balance 10→value 100). Confidence starts with the scored
// fraction (high ≥80%, medium 50–79%, low <50%) and high confidence is disallowed
// on tiny scored-edge or connected-module samples. Zero scored edges → n/a/low.
// The advisory worst-edge cap (critical band) is still applied on top: any
// critical edge → cap at 60; pervasive (≥5% of scored+abstained) → cap at 40.
//
// When summary is nil (backward compat), the function falls back to the legacy
// advisory-edge path using the edges []bcEdge slice.
func couplingBalance(edges []bcEdge, mi metricIndex, summary *diagnostic.ClassifiedEdgeSummary) Dimension {
	dim := Dimension{Name: DimCouplingBalance}

	// Degenerate import graph: <2 modules joined by dependency edges. That is
	// unmeasurable only when the classified summary has no internal cross-boundary
	// coupling facts either. Clone-only duplicated knowledge is deliberately a
	// score-bearing coupling fact without an import edge, so it must be allowed to
	// reach the summary path.
	if degenerateGraph(mi) && (summary == nil || summary.Scored+summary.Abstained == 0) {
		dim.Band = BandNA
		dim.Confidence = ConfidenceLow
		dim.Value = 0
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
		// coupling_balance — the book scores YOUR components only, not your libraries).
		crossBoundary := summary.Scored + summary.Abstained

		// Zero scored internal cross-boundary edges: coupling could not be measured.
		// Report n/a — do NOT fabricate a mid-band number. The cause is upstream
		// (no module map, file-path globs that never match dotted/crate node names,
		// an empty SCIP index, or a single-node graph); the evidence names it.
		if summary.Scored == 0 {
			dim.Band = BandNA
			dim.Confidence = ConfidenceLow
			dim.Value = 0
			dim.Evidence = []string{
				"0 scored internal cross-boundary edges — coupling balance unconfirmed (edge classification absent or all edges abstained)",
				fmt.Sprintf("worst-case (critical band) edges: %d", worst),
			}
			if summary.CloneOnlyScored > 0 || summary.CloneOnlyAdvisory > 0 {
				dim.Evidence = append(dim.Evidence,
					fmt.Sprintf("clone-only duplicated-knowledge pairs: %d scored, %d advisory-only",
						summary.CloneOnlyScored, summary.CloneOnlyAdvisory))
			}
			if summary.External > 0 {
				dim.Evidence = append(dim.Evidence,
					fmt.Sprintf("%d external/library edges excluded (external deps are not internal coupling seams)", summary.External))
			}
			dim.Summary = "coupling balance unconfirmed: no scored cross-boundary edges"
			return dim
		}

		// value = round(100 × (MeanBalance − 1) / 9): linear rescale of book's 1–10.
		value := int(math.Round(100 * (summary.MeanBalance - 1) / 9))

		// Confidence from internal-only scored fraction (external edges do not count).
		scoredPct := 100 * summary.Scored / crossBoundary
		conf := confidenceFromScoredPct(scoredPct)
		capReasons := summaryConfidenceCapReasons(summary, conf)

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

		// LLM-provenance labels lower confidence by one band when a non-high-confidence
		// LLM label actually filled at least one scored edge. Count applied edges, not
		// raw label rows: one module-pair label can classify many edges, and some
		// approved LLM labels may be beaten by static precedence.
		llmConfLowered := summary.LLMLowConfidenceEdges > 0
		if llmConfLowered {
			conf = lowerConf(conf)
		}
		conf = applySummaryConfidenceCap(conf, capReasons)

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
		dim.Evidence = appendTailRiskEvidence(dim.Evidence, summary)
		dim.Evidence = append(dim.Evidence, capReasons...)
		if summary.DeclaredExternal > 0 {
			dim.Evidence = append(dim.Evidence,
				fmt.Sprintf("%d declared external-system edges scored at D=10 (external_systems)", summary.DeclaredExternal))
		}
		if summary.CloneOnlyScored > 0 || summary.CloneOnlyAdvisory > 0 {
			dim.Evidence = append(dim.Evidence,
				fmt.Sprintf("clone-only duplicated-knowledge pairs: %d scored, %d advisory-only",
					summary.CloneOnlyScored, summary.CloneOnlyAdvisory))
		}
		if summary.External > 0 {
			dim.Evidence = append(dim.Evidence,
				fmt.Sprintf("%d external/library edges excluded (external deps are not internal coupling seams)", summary.External))
		}
		// Volatility triage disclosure: module counts by volatility source, so a
		// uniform-volatility repo reads as uniform-by-inheritance, not measured.
		if vp := summary.VolatilityProvenance; vp != nil {
			line := fmt.Sprintf("volatility provenance (modules): declared: %d, inherited: %d, cascade: %d",
				vp.Declared, vp.Inherited, vp.Cascade)
			if vp.Undeclared > 0 {
				line += fmt.Sprintf(", undeclared: %d", vp.Undeclared)
			}
			dim.Evidence = append(dim.Evidence, line)
		}
		dim.Evidence = appendLLMLabelEvidence(dim.Evidence, summary, llmConfLowered)
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
	if len(edges) == 0 {
		// Legacy path only (summary == nil): unreachable from engine.go, which always
		// populates ClassifiedEdges. Calibration suites construct bare diagnostics and
		// rely on this returning a non-penalising sentinel (e.g. all advisory edges
		// waived/baselined → triaged, not unmeasured). The n/a honesty fix applies to
		// the production summary path (Scored == 0) and the degenerate-graph path above.
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

func applySummaryConfidenceCap(conf Confidence, capReasons []string) Confidence {
	if len(capReasons) > 0 && conf == ConfidenceHigh {
		return ConfidenceMedium
	}
	return conf
}

func confidenceFromScoredPct(scoredPct int) Confidence {
	switch {
	case scoredPct >= 80:
		return ConfidenceHigh
	case scoredPct >= 50:
		return ConfidenceMedium
	default:
		return ConfidenceLow
	}
}

func summaryConfidenceCapReasons(summary *diagnostic.ClassifiedEdgeSummary, conf Confidence) []string {
	if conf != ConfidenceHigh {
		return nil
	}
	var reasons []string
	if summary.Scored < minHighConfidenceScoredEdges {
		reasons = append(reasons,
			fmt.Sprintf("sample size below high-confidence floor: %d scored internal cross-boundary edges (need ≥%d)",
				summary.Scored, minHighConfidenceScoredEdges))
	}
	if summary.ConnectedModules > 0 && summary.ConnectedModules < minHighConfidenceConnectedModules {
		reasons = append(reasons,
			fmt.Sprintf("connected module sample below high-confidence floor: %d connected modules (need ≥%d)",
				summary.ConnectedModules, minHighConfidenceConnectedModules))
	}
	return reasons
}

func appendTailRiskEvidence(evidence []string, summary *diagnostic.ClassifiedEdgeSummary) []string {
	tr := summary.TailRisk
	if tr == nil {
		return evidence
	}
	evidence = append(evidence,
		fmt.Sprintf("tail risk: worst balance %d/10, lower-decile balance %d/10, high-or-worse edges %d/%d (%d%%), critical %d, distributed-monolith %d",
			tr.WorstBalance, tr.LowerDecileBalance, tr.HighOrWorseEdges, summary.Scored,
			tr.HighOrWorseSharePct, tr.CriticalEdges, tr.DistributedMonolithEdges))
	if tr.CloneOnlyScored > 0 {
		evidence = append(evidence,
			fmt.Sprintf("clone-only tail: worst balance %d/10, high-or-worse %d/%d scored clone-only pairs",
				tr.CloneOnlyWorstBalance, tr.CloneOnlyHighOrWorseEdges, tr.CloneOnlyScored))
	}
	return evidence
}

// appendLLMLabelEvidence appends the llm-label disclosure lines: the approved
// llm-provenance label count (with the confidence-lowering note when it
// applied) and the labeled_llm edge bucket, which attributes the
// scored-fraction increase to the semantic layer — edges whose strength exists
// only because an approved llm label filled an otherwise-abstained cell.
func appendLLMLabelEvidence(evidence []string, summary *diagnostic.ClassifiedEdgeSummary, llmConfLowered bool) []string {
	if llmConfLowered {
		evidence = append(evidence,
			fmt.Sprintf("llm-provenance labels in effect: %d (confidence lowered)", summary.LLMApproved))
	} else if summary.LLMApproved > 0 {
		evidence = append(evidence,
			fmt.Sprintf("llm-provenance labels in effect: %d", summary.LLMApproved))
	}
	if summary.LabeledLLM > 0 {
		evidence = append(evidence,
			fmt.Sprintf("llm-labeled edges: %d (strength from approved llm-provenance labels)", summary.LabeledLLM))
	}
	return evidence
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
// the same filter IsActiveGateFinding and the gate verdict use. Baseline-accepted
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
