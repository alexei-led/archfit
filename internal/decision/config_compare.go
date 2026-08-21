// Package decision — configuration comparison.
//
// CompareConfigs is the pure decision behind `archfit config compare`: two
// completed measurements of the SAME source tree, one per configuration, turned
// into a report-only difference. It is not a gate and not a ranking — a higher
// candidate score is not evidence that the candidate configuration is better,
// so nothing here says "better", "improved", or "regressed".
//
// Two properties keep the comparison honest:
//
//   - Alternative configurations have no time order. The finding buckets are
//     current_only / candidate_only / both, never introduced/resolved.
//   - Reduced measurement is reported even when the score rises. A candidate
//     that scores fewer edges, abstains more, or pushes more edges out to
//     "external" measured LESS of the same tree, and that is a warning
//     regardless of which way the number moved.
package decision

import (
	"fmt"
	"sort"
	"strings"

	"github.com/alexei-led/archfit/internal/model/diagnostic"
	"github.com/alexei-led/archfit/internal/model/finding"
	"github.com/alexei-led/archfit/internal/score"
)

// CoverageComparability grades how far two runs' analyzer evidence can be
// compared at all. It is evidence quality, never a verdict.
type CoverageComparability string

// Coverage comparability grades, in ascending severity. The aggregate result is
// the worst grade over every compared analyzer.
const (
	// CoverageComparable — every compared analyzer ran on both sides.
	CoverageComparable CoverageComparability = "comparable"
	// CoverageComparableWithGaps — the sides agree, but at least one analyzer
	// was absent or disabled on BOTH sides, so the shared picture is incomplete.
	CoverageComparableWithGaps CoverageComparability = "comparable_with_gaps"
	// CoverageNotComparable — an analyzer's evidence differs between the sides,
	// is partial or timed out, or has a missing/duplicate coverage row. Any
	// difference in findings may be an artifact of the evidence, not the config.
	CoverageNotComparable CoverageComparability = "not_comparable"
)

// CoverageRowMissing is the placeholder rendered in a CoverageDetail when one
// side carries no coverage row for the analyzer at all.
const CoverageRowMissing = "missing"

// Measurement-loss warning codes. Stable identifiers — renderers and agents key
// off these, so treat a change as a contract change.
const (
	// WarnScoredFractionFell — the candidate put a smaller share of its
	// cross-boundary edges on a concrete book balance.
	WarnScoredFractionFell = "scored_fraction_fell"
	// WarnAbstainedEdgesRose — the candidate abstained on more cross-boundary
	// edges (strength or distance unknown).
	WarnAbstainedEdgesRose = "abstained_edges_rose"
	// WarnExternalEdgesRose — the candidate pushed more edges outside the
	// declared module set, excluding them from coupling_balance entirely.
	WarnExternalEdgesRose = "external_edges_rose"
)

// ConfigCompareSide is one configuration's completed measurement of the shared
// source tree. The config hash and source path stay with the caller: the
// comparison never reads them, because differing configuration is the whole
// point of the command.
type ConfigCompareSide struct {
	// Diag is the completed diagnostic for this side.
	Diag diagnostic.Diagnostic
	// Score is the scorecard synthesised from Diag.
	Score score.Scorecard
}

// ConfigCompareInput is the complete input to CompareConfigs.
type ConfigCompareInput struct {
	// Current is the measurement produced by the in-force configuration.
	Current ConfigCompareSide
	// Candidate is the measurement produced by the proposed configuration.
	Candidate ConfigCompareSide
}

// ConfigCompareResult is the pure comparison of two configurations over one
// source tree.
type ConfigCompareResult struct {
	// Findings buckets the observed finding IDs of both sides.
	Findings ConfigCompareFindings
	// Coverage grades whether the two sides rest on comparable analyzer evidence.
	Coverage ConfigCompareCoverage
	// ScoreDelta is candidate overall − current overall. Nil when either side
	// is unmeasured (score.BandNA): there is no number to subtract, and 0 would
	// read as "no change".
	ScoreDelta *int
	// Warnings lists measurement loss in the candidate, sorted by Code. Never
	// nil. Independent of ScoreDelta — measurement loss is reported even when
	// the score could not be computed or moved upward.
	Warnings []ConfigCompareWarning
}

// ConfigCompareFindings buckets observed finding IDs across the two sides. All
// three lists are sorted, deduplicated, and never nil.
type ConfigCompareFindings struct {
	// CurrentOnlyIDs are observed only under the current configuration.
	CurrentOnlyIDs []string
	// CandidateOnlyIDs are observed only under the candidate configuration.
	CandidateOnlyIDs []string
	// BothIDs are observed under both configurations.
	BothIDs []string
}

// ConfigCompareCoverage is the aggregate analyzer-evidence grade plus one
// detail per analyzer that is not plainly comparable.
type ConfigCompareCoverage struct {
	// Status is the worst grade over every compared analyzer.
	Status CoverageComparability
	// Details holds one entry per analyzer graded below CoverageComparable,
	// sorted by Tool. Never nil; empty when every analyzer lined up.
	Details []CoverageDetail
}

// CoverageDetail explains one analyzer's contribution to the coverage grade.
type CoverageDetail struct {
	// Tool is the analyzer's coverage name.
	Tool string
	// Current is the analyzer's raw coverage status under the current config,
	// CoverageRowMissing when it has no row, or the joined statuses when it has
	// more than one.
	Current string
	// Candidate is the same rendering for the candidate config.
	Candidate string
	// Status is this analyzer's own comparability grade.
	Status CoverageComparability
	// Reason is a short human explanation of Status.
	Reason string
}

// ConfigCompareWarning is one measurement-loss signal with a stable code.
type ConfigCompareWarning struct {
	// Code is the stable identifier (one of the Warn* constants).
	Code string
	// Text is a short human explanation carrying the counts it rests on.
	Text string
}

// CompareConfigs compares two completed measurements of one source tree.
//
// It is pure: no I/O, no clock, no config parsing. Every slice in the result is
// non-nil and stably ordered.
func CompareConfigs(in ConfigCompareInput) ConfigCompareResult {
	return ConfigCompareResult{
		Findings:   compareFindings(in.Current.Diag.Findings, in.Candidate.Diag.Findings),
		Coverage:   compareCoverage(in.Current.Diag, in.Candidate.Diag),
		ScoreDelta: compareScore(in.Current.Score, in.Candidate.Score),
		Warnings:   measurementWarnings(in.Current.Diag.ClassifiedEdges, in.Candidate.Diag.ClassifiedEdges),
	}
}

// ---------------------------------------------------------------------------
// Findings
// ---------------------------------------------------------------------------

// observedFindingIDs projects a side's findings to the stable IDs it actually
// observed. Fixed entries are dropped — a finding reported as fixed was not
// observed under that configuration. Gate and advisory forms of the same seam
// share one ID, so promotion never splits a finding across two buckets.
func observedFindingIDs(findings []finding.Finding) map[string]struct{} {
	ids := make(map[string]struct{}, len(findings))
	for _, f := range findings {
		if f.Status == finding.StatusFixed {
			continue
		}
		ids[f.ID] = struct{}{}
	}
	return ids
}

// compareFindings splits the two observed ID sets into the three buckets.
func compareFindings(current, candidate []finding.Finding) ConfigCompareFindings {
	cur := observedFindingIDs(current)
	cand := observedFindingIDs(candidate)

	currentOnly, candidateOnly, both := []string{}, []string{}, []string{}
	for id := range cur {
		if _, ok := cand[id]; ok {
			both = append(both, id)
			continue
		}
		currentOnly = append(currentOnly, id)
	}
	for id := range cand {
		if _, ok := cur[id]; !ok {
			candidateOnly = append(candidateOnly, id)
		}
	}
	sort.Strings(currentOnly)
	sort.Strings(candidateOnly)
	sort.Strings(both)
	return ConfigCompareFindings{
		CurrentOnlyIDs:   currentOnly,
		CandidateOnlyIDs: candidateOnly,
		BothIDs:          both,
	}
}

// ---------------------------------------------------------------------------
// Coverage
// ---------------------------------------------------------------------------

// coverageReason strings. Each names the exact evidence problem so a caller can
// act on it without re-deriving the rule.
const (
	reasonRowCount    = "coverage row missing or duplicated"
	reasonUnstable    = "partial or timed-out coverage cannot be compared"
	reasonStatusMoved = "coverage status differs between the configurations"
	reasonUnknown     = "unrecognised coverage status"
	reasonAbsentBoth  = "analyzer absent under both configurations"
	reasonDisabled    = "analyzer disabled under both configurations"
	// reasonAbsentAsymmetric covers equal absent rows where only one
	// configuration reported a coverage gap: one side expected the analyzer to
	// run and the other did not, so the blindness is not shared.
	reasonAbsentAsymmetric = "analyzer absent, but only one configuration expected it to run"
)

// compareCoverage grades the union of analyzer names across both sides.
func compareCoverage(current, candidate diagnostic.Diagnostic) ConfigCompareCoverage {
	curRows := rowsByTool(current.ToolCoverage)
	candRows := rowsByTool(candidate.ToolCoverage)
	primary := primaryTools(current, candidate)

	status := CoverageComparable
	details := []CoverageDetail{}
	for _, tool := range unionTools(curRows, candRows) {
		cur, cand := curRows[tool], candRows[tool]
		grade, reason, ignored := gradeTool(tool, cur, cand, primary,
			current.CoverageGaps, candidate.CoverageGaps)
		if ignored {
			continue
		}
		if worseCoverage(grade, status) {
			status = grade
		}
		if grade == CoverageComparable {
			continue
		}
		details = append(details, CoverageDetail{
			Tool:      tool,
			Current:   renderRows(cur),
			Candidate: renderRows(cand),
			Status:    grade,
			Reason:    reason,
		})
	}
	return ConfigCompareCoverage{Status: status, Details: details}
}

// gradeTool applies the per-analyzer pairing rules. ignored is true for an
// analyzer that drops out of the comparison entirely.
func gradeTool(
	tool string,
	cur, cand []string,
	primary map[string]struct{},
	curGaps, candGaps []diagnostic.CoverageGap,
) (grade CoverageComparability, reason string, ignored bool) {
	// A side without exactly one row cannot be paired: nothing to compare
	// against, or no way to know which duplicate pairs with which.
	if len(cur) != 1 || len(cand) != 1 {
		return CoverageNotComparable, reasonRowCount, false
	}
	c, d := cur[0], cand[0]
	if unstableStatus(c) || unstableStatus(d) {
		return CoverageNotComparable, reasonUnstable, false
	}
	if c != d {
		return CoverageNotComparable, reasonStatusMoved, false
	}
	switch c {
	case diagnostic.StatusOK:
		return CoverageComparable, "", false
	case diagnostic.StatusAbsent:
		// A coverage gap is per-side evidence: it says this configuration
		// EXPECTED the analyzer to run. One side gapped and the other not is an
		// asymmetry — the two sides are not equally blind, so the absence is not
		// shared and the comparison cannot rest on it.
		curGapped, candGapped := hasCoverageGap(curGaps, tool), hasCoverageGap(candGaps, tool)
		if curGapped != candGapped {
			return CoverageNotComparable, reasonAbsentAsymmetric, false
		}
		// A per-language graph analyzer that is absent without a coverage gap was
		// suppressed because that language is not in the tree — nothing was lost,
		// so the analyzer drops out of the comparison entirely.
		if _, ok := primary[tool]; ok && !curGapped {
			return CoverageComparable, "", true
		}
		return CoverageComparableWithGaps, reasonAbsentBoth, false
	case diagnostic.StatusDisabled:
		return CoverageComparableWithGaps, reasonDisabled, false
	default:
		// An equal but unrecognised status is not provably comparable. Abstain
		// rather than assume a future status behaves like "ok".
		return CoverageNotComparable, reasonUnknown, false
	}
}

// unstableStatus reports whether a coverage status means the analyzer could
// have produced findings and did not finish doing so.
func unstableStatus(s string) bool {
	return s == diagnostic.StatusPartial || s == diagnostic.StatusTimedOut
}

// rowsByTool groups coverage statuses by analyzer name, sorted within each tool
// so a duplicate pair renders deterministically.
func rowsByTool(rows []diagnostic.Coverage) map[string][]string {
	out := make(map[string][]string, len(rows))
	for _, c := range rows {
		out[c.Tool] = append(out[c.Tool], c.Status)
	}
	for tool := range out {
		sort.Strings(out[tool])
	}
	return out
}

// unionTools returns every analyzer name seen on either side, sorted.
func unionTools(a, b map[string][]string) []string {
	seen := make(map[string]struct{}, len(a)+len(b))
	for tool := range a {
		seen[tool] = struct{}{}
	}
	for tool := range b {
		seen[tool] = struct{}{}
	}
	tools := make([]string, 0, len(seen))
	for tool := range seen {
		tools = append(tools, tool)
	}
	sort.Strings(tools)
	return tools
}

// primaryTools is the union of both sides' declared per-language graph
// analyzers. Empty when neither diagnostic declares any, which conservatively
// makes every absent row a reported gap instead of an ignored one.
func primaryTools(a, b diagnostic.Diagnostic) map[string]struct{} {
	out := make(map[string]struct{}, len(a.PrimaryExtractorTools)+len(b.PrimaryExtractorTools))
	for _, t := range a.PrimaryExtractorTools {
		out[t] = struct{}{}
	}
	for _, t := range b.PrimaryExtractorTools {
		out[t] = struct{}{}
	}
	return out
}

func hasCoverageGap(gaps []diagnostic.CoverageGap, tool string) bool {
	for _, g := range gaps {
		if g.Tool == tool {
			return true
		}
	}
	return false
}

// renderRows renders one side's coverage statuses for a detail entry.
func renderRows(rows []string) string {
	if len(rows) == 0 {
		return CoverageRowMissing
	}
	return strings.Join(rows, "+")
}

// coverageRank orders the grades so the aggregate can take the worst.
func coverageRank(c CoverageComparability) int {
	switch c {
	case CoverageNotComparable:
		return 2
	case CoverageComparableWithGaps:
		return 1
	case CoverageComparable:
		return 0
	default:
		return 0
	}
}

func worseCoverage(a, b CoverageComparability) bool {
	return coverageRank(a) > coverageRank(b)
}

// ---------------------------------------------------------------------------
// Measurement
// ---------------------------------------------------------------------------

// compareScore returns the signed overall delta, or nil when either side could
// not be measured. A nil delta is "unknown", which is not the same as 0.
func compareScore(current, candidate score.Scorecard) *int {
	if current.OverallBand.Unmeasured() || candidate.OverallBand.Unmeasured() {
		return nil
	}
	delta := candidate.Overall - current.Overall
	return &delta
}

// measurementWarnings reports where the candidate configuration measured LESS of
// the same tree. Score direction is never consulted: a configuration that hides
// edges scores higher precisely because it stopped looking.
//
// The three channels are deliberately asymmetric. Scored coverage is a share of
// the cross-boundary edges the scorer considered, so it is compared as a
// fraction. External edges are excluded from that denominator entirely, so their
// growth is only visible as an absolute count.
func measurementWarnings(current, candidate *diagnostic.ClassifiedEdgeSummary) []ConfigCompareWarning {
	curScored, curAbstained, curExternal := edgeCounts(current)
	candScored, candAbstained, candExternal := edgeCounts(candidate)

	warnings := []ConfigCompareWarning{}
	if curDen, candDen := curScored+curAbstained, candScored+candAbstained; scoredShareFell(curScored, curDen, candScored, candDen) {
		warnings = append(warnings, ConfigCompareWarning{
			Code: WarnScoredFractionFell,
			Text: fmt.Sprintf("scored share of cross-boundary edges fell (%d/%d to %d/%d)",
				curScored, curDen, candScored, candDen),
		})
	}
	if candAbstained > curAbstained {
		warnings = append(warnings, ConfigCompareWarning{
			Code: WarnAbstainedEdgesRose,
			Text: fmt.Sprintf("abstained cross-boundary edges rose (%d to %d)", curAbstained, candAbstained),
		})
	}
	if candExternal > curExternal {
		warnings = append(warnings, ConfigCompareWarning{
			Code: WarnExternalEdgesRose,
			Text: fmt.Sprintf("edges excluded as external rose (%d to %d)", curExternal, candExternal),
		})
	}
	sort.Slice(warnings, func(i, j int) bool { return warnings[i].Code < warnings[j].Code })
	return warnings
}

// scoredShareFell compares the two scored shares without floating point, so an
// identical pair of counts can never warn through rounding.
//
// A current side with no cross-boundary edges at all has no share to fall from,
// so it never warns. A candidate that dropped to none, having started from some,
// lost all of its measurement and always warns.
func scoredShareFell(curScored, curDen, candScored, candDen int) bool {
	if curDen == 0 {
		return false
	}
	if candDen == 0 {
		return true
	}
	return int64(candScored)*int64(curDen) < int64(curScored)*int64(candDen)
}

// edgeCounts reads the three measurement counters, treating an absent summary
// (classification did not run) as zero.
func edgeCounts(s *diagnostic.ClassifiedEdgeSummary) (scored, abstained, external int) {
	if s == nil {
		return 0, 0, 0
	}
	return s.Scored, s.Abstained, s.External
}
