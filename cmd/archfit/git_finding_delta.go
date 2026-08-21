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
//     analyzer family covered both sides equivalently. Missing, timed-out, or
//     asymmetric evidence yields "unknown", never a false "introduced".
//   - Two SYMMETRIC degradations still pair, because neither side could have
//     produced a finding the other hid: a "partial" from unresolved import
//     specifiers (the normal dependency-cruiser/grimp state) and an analyzer
//     that was unavailable on both sides. Both stay comparable and both always
//     name themselves in comparison_reasons — pairing them silently would trade
//     one half of the defect for the other.
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
	"github.com/alexei-led/archfit/internal/decision"
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
	// evidenceAbsentGapped — an analyzer that config ENABLED reported absent and
	// raised a coverage gap: the tool it needs is not installed. Symmetric, that
	// is the same safety argument as evidenceAbsent — neither side ran it, so
	// neither side has a finding the other could hide — so it pairs, in DEGRADED
	// form, which discloses the shared blindness. Asymmetric it never pairs: the
	// gap is derived per side from that side's own tree, so head-gapped /
	// base-clean means the analyzer's absence is not shared.
	evidenceAbsentGapped
	// evidenceDisabled — the analyzer is turned off in config.
	evidenceDisabled
	// evidencePartialUnresolved — the analyzer ran over the whole tree and left
	// N import specifiers unresolved. That is the STEADY state for
	// dependency-cruiser and grimp (one unresolvable specifier anywhere sets it),
	// not a completion failure, so it must not read as "the analyzer did not
	// run". It pairs only with itself: both sides ran, both are equally
	// incomplete, and the degradation is disclosed in comparison_reasons with
	// each side's magnitude.
	evidencePartialUnresolved
	// evidenceUnavailable — timed out, or partial from a run that did not finish
	// (see decision.PartialFromUnresolvedSpecifiers): the analyzer could have
	// produced findings and did not finish doing so, and unlike an uninstalled
	// tool the failure is flaky rather than structural, so symmetry proves
	// nothing. It pairs with nothing, including itself.
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
// both sides equivalently, plus one reason per family that did not — or that
// paired only in degraded form, which stays comparable but is still disclosed.
func compareAnalyzerEvidence(fams []analyzerFamily, head, base analyzerEvidence) (bool, []string) {
	ok := true
	var reasons []string
	for _, f := range fams {
		h := summariseFamily(f, head)
		b := summariseFamily(f, base)
		switch pairFamily(h, b) {
		case familyPaired:
			continue
		case familyPairedDegradedUnresolved:
			reasons = append(reasons, fmt.Sprintf(
				"%s: head %s, base %s — unresolved import specifiers on both sides", f.name, h.raw(), b.raw()))
		case familyPairedDegradedAbsent:
			reasons = append(reasons, fmt.Sprintf(
				"%s: head %s, base %s — analyzer unavailable on both sides; neither run could report its findings",
				f.name, h.raw(), b.raw()))
		case familyUnpaired:
			ok = false
			reasons = append(reasons, fmt.Sprintf("%s: head %s, base %s", f.name, h.raw(), b.raw()))
		}
	}
	return ok, reasons
}

// familySummary is one family's normalised coverage on one side.
type familySummary struct {
	statuses []evidenceStatus
	// rawStatuses are the coverage rows' own status strings, sorted, for the
	// human-readable reason. Empty when the family had no row at all. A partial
	// row carries its unresolved magnitude: the degraded pairing rule is
	// magnitude-blind, so without the numbers nothing in the output separates a
	// handful of unresolved specifiers from a repo-wide resolution failure.
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
		out.rawStatuses = append(out.rawStatuses, rawCoverageStatus(c))
	}
	sort.Strings(out.rawStatuses)
	return out
}

// rawCoverageStatus renders one coverage row for a comparison reason: the status
// alone, plus the unresolved magnitude when it MEANS unresolved import
// specifiers. The same predicate gates it as gates the pairing, so a go/packages
// partial — where Unresolved counts skipped packages — never renders a number
// that reads as a specifier count.
func rawCoverageStatus(c diagnostic.Coverage) string {
	if !decision.PartialFromUnresolvedSpecifiers(c) {
		return c.Status
	}
	return fmt.Sprintf("%s (%s)", c.Status, decision.UnresolvedMagnitude(c))
}

// normalizeCoverage maps one coverage row to its comparability status.
func normalizeCoverage(f analyzerFamily, c diagnostic.Coverage, gaps []diagnostic.CoverageGap) evidenceStatus {
	switch c.Status {
	case diagnostic.StatusOK:
		return evidenceOK
	case diagnostic.StatusDisabled:
		return evidenceDisabled
	case diagnostic.StatusAbsent:
		// With a coverage gap, an installable analyzer this config enabled really
		// did not run. Decided before the primary check: an uninstalled tool is
		// evidence about the HOST, so it reads the same for every family.
		if hasGap(gaps, c.Tool) {
			return evidenceAbsentGapped
		}
		// Gapless absence means two different things. For a primary analyzer the
		// project markers decided it: the language is not in this tree, so that
		// side had nothing to find. For anything else it only means the tool was
		// not available, which is silent about the tree.
		if f.primary {
			return evidenceNotApplicable
		}
		return evidenceAbsent
	case diagnostic.StatusPartial:
		// Shared with `config compare` so the two paths cannot grade one row
		// differently. Note that Unresolved > 0 is NOT the discriminator on its
		// own: go/packages also sets it, counting whole packages it skipped.
		if decision.PartialFromUnresolvedSpecifiers(c) {
			return evidencePartialUnresolved
		}
		return evidenceUnavailable
	default:
		// timed out and any future status: not comparable.
		return evidenceUnavailable
	}
}

// familyPairing is the outcome of pairing one analyzer family's two sides.
type familyPairing int

const (
	// familyPaired — the two sides rest on equivalent evidence.
	familyPaired familyPairing = iota
	// familyPairedDegradedUnresolved — the two sides pair, but both ran with
	// unresolved import specifiers. Comparable, and disclosed with magnitudes.
	familyPairedDegradedUnresolved
	// familyPairedDegradedAbsent — the two sides pair, but the analyzer was
	// unavailable on both. Comparable, and disclosed in comparison_reasons.
	familyPairedDegradedAbsent
	// familyUnpaired — the evidence does not support attributing an origin.
	familyUnpaired
)

// pairFamily applies the pairing rules to one analyzer's row on each side.
//
// Equal statuses always pair: an analyzer disabled on both sides is a deliberate
// opt-out, and one absent on both sides produced no findings on either side, so
// neither can hide an origin. The only cross-status pair is ok against
// not_applicable — that side genuinely has none of the language.
//
// Two shapes pair as DEGRADED rather than failing, both because SYMMETRY is the
// safety argument (neither side ran what the other could hide behind) and both
// disclosed in comparison_reasons, never silently:
//
//   - Symmetric partial-with-unresolved-specifiers. For dependency-cruiser and
//     grimp that status is the normal steady state; treating it as unusable made
//     the whole origin delta inert on every TypeScript and Python repo. The
//     residual risk — a base counterpart hidden behind an unresolved specifier —
//     is bounded only by the magnitudes the reason carries.
//   - Symmetric absent-with-a-coverage-gap: an enabled analyzer whose tool is
//     not installed on this host. Treating it as unusable made the delta inert
//     for a whole class of environments, including archfit's own runtime image
//     (no Rust toolchain) on any repo carrying a Cargo.toml. decision.gradeTool
//     already grades this shape symmetric-comparable; the two paths must agree.
//
// Asymmetry never pairs in either shape. For the gapped case that matters
// concretely: gaps are derived per side from that side's own tree, so a
// Cargo.toml ADDED by the change gives head a gap and base none.
//
// A missing row, a duplicated row, a timeout, and a partial from a run that did
// not finish are all unavailable evidence and pair with nothing.
func pairFamily(head, base familySummary) familyPairing {
	if head.rows() != 1 || base.rows() != 1 {
		return familyUnpaired // a missing or duplicated coverage row is unavailable evidence
	}
	h, b := head.statuses[0], base.statuses[0]
	switch {
	case h == evidencePartialUnresolved || b == evidencePartialUnresolved:
		if h == b {
			return familyPairedDegradedUnresolved
		}
		return familyUnpaired
	case h == evidenceAbsentGapped || b == evidenceAbsentGapped:
		if h == b {
			return familyPairedDegradedAbsent
		}
		return familyUnpaired
	case h == evidenceUnavailable || b == evidenceUnavailable:
		return familyUnpaired
	case h == b:
		return familyPaired
	case h == evidenceOK && b == evidenceNotApplicable,
		h == evidenceNotApplicable && b == evidenceOK:
		return familyPaired
	default:
		return familyUnpaired
	}
}

func hasGap(gaps []diagnostic.CoverageGap, tool string) bool {
	for _, g := range gaps {
		if g.Tool == tool {
			return true
		}
	}
	return false
}
