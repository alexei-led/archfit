// Package main — git-origin classification for `analyze/check --base <ref>`.
//
// The base sub-run already produces a full diagnostic; this file turns the two
// sides into the report-only git_finding_delta block: which of the CURRENT
// repair tasks the change introduced, which pre-date the base ref, and which
// cannot be placed at all.
//
// The comparison is pure and conservative:
//   - Only stable finding IDs are matched. Lifecycle labels (new/waived/…) and
//     gate-vs-advisory promotion are ignored — the same seam keeps its ID.
//   - An unmatched task is "introduced" only when every ACTIVE finding-producing
//     analyzer family covered both sides equivalently. Missing, partial,
//     timed-out, or asymmetric evidence yields "unknown", never a false
//     "introduced".
//
// Isolation: the only base-side inputs are finding IDs, coverage rows/gaps, and
// the config hash (see baseEvidence in worktree.go). Base paths, locations, and
// validation commands name a temporary worktree that is already deleted.
package main

import (
	"fmt"
	"sort"
	"strings"

	"github.com/alexei-led/archfit/internal/config"
	"github.com/alexei-led/archfit/internal/model/diagnostic"
	"github.com/alexei-led/archfit/internal/model/finding"
)

// evidenceStatus is one coverage row normalised for comparability.
type evidenceStatus int

const (
	// evidenceOK — the analyzer ran and reported coverage.
	evidenceOK evidenceStatus = iota
	// evidenceNotApplicable — a PRIMARY analyzer reported absent with no
	// coverage gap. Project markers decided that: the language is simply not in
	// this tree, so that side genuinely had nothing to find. It pairs with ok.
	evidenceNotApplicable
	// evidenceAbsent — a NON-PRIMARY analyzer reported absent with no coverage
	// gap. That is evidence about the TOOL (not installed, no indexer), not
	// about the tree, so it says nothing about what the side would have found.
	// It pairs only with itself: symmetric absence means neither side produced
	// findings from this analyzer, which is safe; asymmetric absence is not.
	evidenceAbsent
	// evidenceDisabled — the analyzer is turned off in config.
	evidenceDisabled
	// evidenceUnavailable — absent with a gap, partial, or timed out: the
	// analyzer could have produced findings and did not.
	evidenceUnavailable
)

// coverageRowMissing is the raw-status placeholder used in a comparison reason
// when a family has no coverage row at all on one side.
const coverageRowMissing = "missing"

// analyzerFamily is one finding-producing analyzer whose evidence must match on
// both sides before an unmatched task can be called introduced. One family is
// one ToolCoverage name: every analyzer reports under its own name, so a second
// row for the same name is a genuine anomaly, not a shape to accommodate.
type analyzerFamily struct {
	// name is the analyzer's ToolCoverage name; it also labels the family in
	// comparison_reasons.
	name string
	// primary marks a per-language dependency-graph analyzer, the only kind
	// whose gapless "absent" means "this language is not here".
	primary bool
}

// analyzerEvidence is one side's analyzer coverage plus the config hash that
// produced it.
type analyzerEvidence struct {
	Coverage []diagnostic.Coverage
	Gaps     []diagnostic.CoverageGap
	Hash     string
}

// gitDeltaInput is the complete input to the pure origin comparison.
type gitDeltaInput struct {
	BaseRef string
	// Tasks are the CURRENT run's agent_tasks[] — the repair work being placed.
	Tasks []diagnostic.AgentTask
	// BaseFindingIDs are the base run's observed finding IDs (fixed excluded).
	BaseFindingIDs []string
	Head           analyzerEvidence
	Base           analyzerEvidence
	Families       []analyzerFamily
}

// analyzerFamilies returns the finding-producing analyzer families to compare,
// with the opt-in ones dropped when their analyzer setting is off. Both sides of
// a --base run measure the same effective config, so activation is one decision,
// not two.
//
// The per-language primary families are always listed: a language missing from
// one tree shows up as absent-without-a-gap, which the pairing rules read as
// not_applicable — the one status that pairs with ok, because project markers
// prove that side had nothing of that language to find.
func analyzerFamilies(cfg config.Config) []analyzerFamily {
	fams := make([]analyzerFamily, 0, len(languageRegistry)+5)
	for _, lang := range languageRegistry {
		fams = append(fams, analyzerFamily{name: lang.PrimaryTool, primary: true})
	}
	// The pattern pass and the syntax pass are two analyzers with two coverage
	// names and two independent activation switches, so they compare separately.
	if len(cfg.ForPatterns()) > 0 {
		fams = append(fams, analyzerFamily{name: toolAstGrep})
	}
	if cfg.ForSyntax().Enabled {
		fams = append(fams, analyzerFamily{name: toolAstGrepSyntax})
	}
	if cfg.ScipEnabled() {
		fams = append(fams,
			analyzerFamily{name: toolScip},
			analyzerFamily{name: toolScipSymbols},
		)
	}
	if cfg.ClonesEnabled() {
		fams = append(fams, analyzerFamily{name: toolJscpd})
	}
	if cfg.CargoModulesEnabled() {
		fams = append(fams, analyzerFamily{name: toolCargoModules})
	}
	return fams
}

// baseFindingIDs projects the base diagnostic's observed findings to their
// stable IDs. Fixed entries are dropped: a finding the base run reports as fixed
// was not observed there, so it cannot make a current task pre-existing.
func baseFindingIDs(findings []finding.Finding) []string {
	ids := make([]string, 0, len(findings))
	for _, f := range findings {
		if f.Status == finding.StatusFixed {
			continue
		}
		ids = append(ids, f.ID)
	}
	sort.Strings(ids)
	return ids
}

// buildGitFindingDelta places every current repair task in an origin bucket.
// Never returns nil — the caller only invokes it when --base is set, and the
// block must be present (with empty, non-null lists) even for a clean run.
func buildGitFindingDelta(in gitDeltaInput) *diagnostic.GitFindingDelta {
	comparable, reasons := compareAnalyzerEvidence(in.Families, in.Head, in.Base)
	// A config-hash mismatch means the two sides did not measure the same
	// intent, so nothing unmatched can be attributed to the code change.
	if in.Head.Hash != in.Base.Hash {
		comparable = false
		reasons = append(reasons, "config: head and base config hashes differ")
	}
	sort.Strings(reasons)

	base := make(map[string]struct{}, len(in.BaseFindingIDs))
	for _, id := range in.BaseFindingIDs {
		base[id] = struct{}{}
	}

	introduced, preExisting, unknown := []string{}, []string{}, []string{}
	for _, t := range in.Tasks {
		switch {
		// The synthetic coupling-gate task is per-run trip state with no stable
		// base counterpart — decided before ID matching so it can never be
		// mistaken for a repaired or introduced seam.
		case t.RuleID == ruleIDBCCouplingGate:
			unknown = append(unknown, t.FindingID)
		case contains(base, t.FindingID):
			preExisting = append(preExisting, t.FindingID)
		case comparable:
			introduced = append(introduced, t.FindingID)
		default:
			unknown = append(unknown, t.FindingID)
		}
	}
	sort.Strings(introduced)
	sort.Strings(preExisting)
	sort.Strings(unknown)

	status := diagnostic.GitComparisonComparable
	if len(unknown) > 0 {
		status = diagnostic.GitComparisonUnknown
	}
	if reasons == nil {
		reasons = []string{}
	}
	return &diagnostic.GitFindingDelta{
		BaseRef:                 in.BaseRef,
		ComparisonStatus:        status,
		IntroducedFindingIDs:    introduced,
		PreExistingFindingIDs:   preExisting,
		UnknownOriginFindingIDs: unknown,
		ComparisonReasons:       reasons,
	}
}

func contains(set map[string]struct{}, key string) bool {
	_, ok := set[key]
	return ok
}

// compareAnalyzerEvidence reports whether every active analyzer family covered
// both sides equivalently, plus one reason per family that did not.
func compareAnalyzerEvidence(fams []analyzerFamily, head, base analyzerEvidence) (bool, []string) {
	ok := true
	var reasons []string
	for _, f := range fams {
		h := summariseFamily(f, head)
		b := summariseFamily(f, base)
		if familyComparable(h, b) {
			continue
		}
		ok = false
		reasons = append(reasons, fmt.Sprintf("%s: head %s, base %s", f.name, h.raw(), b.raw()))
	}
	return ok, reasons
}

// familySummary is one family's normalised coverage on one side.
type familySummary struct {
	statuses []evidenceStatus
	// rawStatuses are the coverage rows' own status strings, sorted, for the
	// human-readable reason. Empty when the family had no row at all.
	rawStatuses []string
}

func (s familySummary) rows() int { return len(s.statuses) }

// raw renders the side's coverage statuses for a comparison reason.
func (s familySummary) raw() string {
	if len(s.rawStatuses) == 0 {
		return coverageRowMissing
	}
	return strings.Join(s.rawStatuses, "+")
}

// summariseFamily normalises every coverage row belonging to f on one side.
func summariseFamily(f analyzerFamily, side analyzerEvidence) familySummary {
	var out familySummary
	for _, c := range side.Coverage {
		if c.Tool != f.name {
			continue
		}
		out.statuses = append(out.statuses, normalizeCoverage(f, c, side.Gaps))
		out.rawStatuses = append(out.rawStatuses, c.Status)
	}
	sort.Strings(out.rawStatuses)
	return out
}

// normalizeCoverage maps one coverage row to its comparability status.
func normalizeCoverage(f analyzerFamily, c diagnostic.Coverage, gaps []diagnostic.CoverageGap) evidenceStatus {
	switch c.Status {
	case diagnostic.StatusOK:
		return evidenceOK
	case diagnostic.StatusDisabled:
		return evidenceDisabled
	case diagnostic.StatusAbsent:
		// With a coverage gap, an installable analyzer really did not run.
		if hasGap(gaps, c.Tool) {
			return evidenceUnavailable
		}
		// Gapless absence means two different things. For a primary analyzer the
		// project markers decided it: the language is not in this tree, so that
		// side had nothing to find. For anything else it only means the tool was
		// not available, which is silent about the tree.
		if f.primary {
			return evidenceNotApplicable
		}
		return evidenceAbsent
	default:
		// partial, timed out, and any future status: not comparable.
		return evidenceUnavailable
	}
}

// familyComparable applies the pairing rules to one analyzer's row on each side.
//
// Equal statuses always pair: an analyzer disabled on both sides is a deliberate
// opt-out, and one absent on both sides produced no findings on either side, so
// neither can hide an origin. The only cross-status pair is ok against
// not_applicable — that side genuinely has none of the language.
//
// A missing row, a duplicated row, and anything unavailable (absent with a gap,
// partial, timed out) are all unavailable evidence.
func familyComparable(head, base familySummary) bool {
	if head.rows() != 1 || base.rows() != 1 {
		return false // a missing or duplicated coverage row is unavailable evidence
	}
	h, b := head.statuses[0], base.statuses[0]
	if h == evidenceUnavailable || b == evidenceUnavailable {
		return false
	}
	if h == b {
		return true
	}
	return (h == evidenceOK && b == evidenceNotApplicable) ||
		(h == evidenceNotApplicable && b == evidenceOK)
}

func hasGap(gaps []diagnostic.CoverageGap, tool string) bool {
	for _, g := range gaps {
		if g.Tool == tool {
			return true
		}
	}
	return false
}
