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
	"slices"
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
	// evidenceNotApplicable — a primary analyzer reported absent with no
	// coverage gap, i.e. that language is simply not in this tree.
	evidenceNotApplicable
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
// both sides before an unmatched task can be called introduced.
type analyzerFamily struct {
	// name labels the family in comparison_reasons.
	name string
	// tools are the ToolCoverage names that belong to this family. The ast-grep
	// family owns two names because the pattern pass and the syntax pass share
	// one coverage name at runtime and differ only when syntax is disabled.
	tools []string
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
// not_applicable and ignore. That is what makes "a new language appeared in this
// change" an unknown origin rather than a silent pass.
func analyzerFamilies(cfg config.Config) []analyzerFamily {
	fams := make([]analyzerFamily, 0, len(languageRegistry)+5)
	for _, lang := range languageRegistry {
		fams = append(fams, analyzerFamily{name: lang.PrimaryTool, tools: []string{lang.PrimaryTool}, primary: true})
	}
	// One ast-grep family, not two: the syntax pass emits coverage under the
	// pattern pass's "ast-grep" name, and "ast-grep/syntax" appears only on the
	// disabled row the pipeline injects. Comparing the two names together keeps
	// both shapes decidable.
	if len(cfg.ForPatterns()) > 0 || cfg.ForSyntax().Enabled {
		fams = append(fams, analyzerFamily{name: toolAstGrep, tools: []string{toolAstGrep, toolAstGrepSyntax}})
	}
	if cfg.ScipEnabled() {
		fams = append(fams,
			analyzerFamily{name: toolScip, tools: []string{toolScip}},
			analyzerFamily{name: toolScipSymbols, tools: []string{toolScipSymbols}},
		)
	}
	if cfg.ClonesEnabled() {
		fams = append(fams, analyzerFamily{name: toolJscpd, tools: []string{toolJscpd}})
	}
	if cfg.CargoModulesEnabled() {
		fams = append(fams, analyzerFamily{name: toolCargoModules, tools: []string{toolCargoModules}})
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

func (s familySummary) count(want evidenceStatus) int {
	n := 0
	for _, st := range s.statuses {
		if st == want {
			n++
		}
	}
	return n
}

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
		if !slices.Contains(f.tools, c.Tool) {
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
		// A primary analyzer that is absent WITHOUT a coverage gap was
		// suppressed because the language is not in this tree — nothing was
		// lost. With a gap, an installable analyzer really did not run.
		if f.primary && !hasGap(gaps, c.Tool) {
			return evidenceNotApplicable
		}
		return evidenceUnavailable
	default:
		// partial, timed out, and any future status: not comparable.
		return evidenceUnavailable
	}
}

// familyComparable applies the pairing rules. A row that is disabled on BOTH
// sides is a deliberate opt-out rather than lost evidence, so it drops out of
// the comparison; what remains must line up one-for-one. A family whose every
// row drops out (all disabled, or all not-applicable) is simply inactive, which
// counts as comparable.
func familyComparable(head, base familySummary) bool {
	if head.rows() == 0 || base.rows() == 0 {
		return false // a missing coverage row is unavailable evidence
	}
	if head.count(evidenceUnavailable) > 0 || base.count(evidenceUnavailable) > 0 {
		return false
	}
	// The ast-grep family needs the symmetric-disabled carve-out: with syntax
	// off it carries an ok pattern row beside a disabled ast-grep/syntax row.
	hd, bd := head.count(evidenceDisabled), base.count(evidenceDisabled)
	if hd != bd {
		return false // asymmetric disabled state
	}
	// The remaining rows are ok or not_applicable, which the pairing table
	// treats as comparable in every combination. An extra or duplicate row on
	// one side only is still unavailable evidence.
	return head.rows()-hd == base.rows()-bd
}

func hasGap(gaps []diagnostic.CoverageGap, tool string) bool {
	for _, g := range gaps {
		if g.Tool == tool {
			return true
		}
	}
	return false
}
