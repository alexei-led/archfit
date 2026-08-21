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
//   - SYMMETRIC degradations still pair, because neither side could have
//     produced a finding the other hid: a "partial" from unresolved import
//     specifiers (the normal dependency-cruiser/grimp state), a "partial" whose
//     inputs are all present with only edge precision degraded (go/packages when
//     a package does not type-check), and an analyzer unavailable on both sides.
//     All stay comparable and all always name themselves in comparison_reasons —
//     pairing them silently would trade one half of the defect for the other.
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
	// evidenceAbsent — an analyzer this run's config ACTIVATED reported absent:
	// the tool it needs did not run on this side. Symmetric, that is safe —
	// neither side ran it, so neither side has a finding the other could hide —
	// so it pairs, in DEGRADED form, which discloses the shared blindness.
	//
	// Whether a CoverageGap was raised is NOT part of this decision. A gap is
	// only emitted for analyzers listed in toolAffectedMetrics (the install-hint
	// table), so scip, scip-symbols and ast-grep never raise one however loudly
	// the config asked for them. Keying comparability on gap presence made a
	// presentation table stand in for evidence semantics and paired those three
	// SILENTLY — on archfit's own config, whose runtime image ships no SCIP
	// indexer. Every family reaching here comes from analyzerFamilies(cfg), so it
	// is config-activated by construction; that, not the gap table, is what makes
	// its absence a disclosed degradation.
	evidenceAbsent
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
	// evidencePartialDegraded — the analyzer covered every input (nothing is
	// missing from the graph) and lost only per-edge precision: go/packages when
	// some packages did not type-check, so their imports are all present and only
	// the go/types strength hints are gone. Like the unresolved case it pairs ONLY
	// with itself, and for a sharper reason: the precision it lost is exactly what
	// strength-derived findings rest on, so a degraded base can miss a
	// bc/imbalanced_coupling seam that an ok head reports. Symmetry is the whole
	// safety argument — pairing degraded against ok would manufacture a false
	// "introduced".
	evidencePartialDegraded
	// evidenceUnavailable — timed out, or partial from a run that did not finish
	// (see decision.PartialFromUnresolvedSpecifiers): the analyzer could have
	// produced findings and did not finish doing so, and unlike an uninstalled
	// tool the failure is flaky rather than structural, so symmetry proves
	// nothing. It pairs with nothing, including itself.
	evidenceUnavailable
)

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
		_, inBase := base[t.FindingID]
		switch {
		// The synthetic coupling-gate task is per-run trip state with no stable
		// base counterpart — decided before ID matching so it can never be
		// mistaken for a repaired or introduced seam.
		case t.RuleID == ruleIDBCCouplingGate:
			unknown = append(unknown, t.FindingID)
		case inBase:
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
		case familyPairedDegradedPrecision:
			reasons = append(reasons, fmt.Sprintf(
				"%s: head %s, base %s — every input covered on both sides, edge precision degraded",
				f.name, h.raw(), b.raw()))
		case familyPairedDegradedAbsent:
			reasons = append(reasons, fmt.Sprintf(
				"%s: head %s, base %s — analyzer unavailable on both sides; neither run could report its findings",
				f.name, h.raw(), b.raw()))
		case familyUnpaired:
			ok = false
			reasons = append(reasons, unpairedReason(f.name, h, b))
		}
	}
	return ok, reasons
}

// unpairedReason renders the reason one family blocked origin classification.
//
// Two raw statuses that MATCH never explain themselves. "cargo: head absent,
// base absent" was the message for head-has-Cargo.toml (analyzer expected,
// missing) against base-has-none (Rust not in that tree): two identical facts
// offered as the reason they could not be compared, distinguishable from the
// symmetric-blindness message that stays COMPARABLE only by a trailing clause.
// The same emptiness afflicts every other equal-raw failure — "head timed_out,
// base timed_out", "head ok+ok, base ok+ok" — where what the reader needs is why
// symmetry did NOT rescue the comparison, which the status cannot say.
//
// So whenever the raw statuses match, both sides carry their meaning, even when
// the meanings match too. Differing raw statuses already carry the information
// and stay terse.
func unpairedReason(name string, head, base familySummary) string {
	if head.raw() == base.raw() {
		return fmt.Sprintf("%s: head %s (%s), base %s (%s)",
			name, head.raw(), head.meaning(), base.raw(), base.meaning())
	}
	return fmt.Sprintf("%s: head %s, base %s", name, head.raw(), base.raw())
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

// raw renders the side's coverage statuses for a comparison reason. The
// no-row placeholder is decision.CoverageRowMissing, the same string
// `config compare` renders for the same shape.
func (s familySummary) raw() string {
	if len(s.rawStatuses) == 0 {
		return decision.CoverageRowMissing
	}
	return strings.Join(s.rawStatuses, "+")
}

// meaning renders what this side's normalised status MEANS, for the reasons
// where the raw status is ambiguous on its own. Total over evidenceStatus so a
// new status cannot silently render as an empty parenthetical.
func (s familySummary) meaning() string {
	if s.rows() != 1 {
		if s.rows() == 0 {
			return "no coverage row"
		}
		return "duplicate coverage rows"
	}
	switch s.statuses[0] {
	case evidenceOK:
		return "analyzer ran"
	case evidenceNotApplicable:
		return "language not present"
	case evidenceAbsent:
		return "analyzer expected, did not run"
	case evidenceDisabled:
		return "disabled in config"
	case evidencePartialUnresolved:
		return "ran whole tree, unresolved import specifiers"
	case evidencePartialDegraded:
		return "covered every input, degraded edge precision"
	default: // evidenceUnavailable
		// Covers timeouts and every partial that left inputs OUT of the graph.
		// Both mean the analyzer could have reported findings over this tree and
		// did not get through it.
		return "run did not finish"
	}
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
// alone, plus the magnitude of whichever degradation earned it. The same
// predicates gate the number as gate the pairing, and each labels its own
// condition, so a go/packages partial never renders a count that reads as
// unresolved import specifiers.
func rawCoverageStatus(c diagnostic.Coverage) string {
	switch {
	case decision.PartialFromUnresolvedSpecifiers(c):
		return fmt.Sprintf("%s (%s)", c.Status, decision.UnresolvedMagnitude(c))
	case decision.PartialFromDegradedPrecision(c):
		return fmt.Sprintf("%s (%s)", c.Status, decision.DegradedMagnitude(c))
	default:
		return c.Status
	}
}

// normalizeCoverage maps one coverage row to its comparability status.
func normalizeCoverage(f analyzerFamily, c diagnostic.Coverage, gaps []diagnostic.CoverageGap) evidenceStatus {
	switch c.Status {
	case diagnostic.StatusOK:
		return evidenceOK
	case diagnostic.StatusDisabled:
		return evidenceDisabled
	case diagnostic.StatusAbsent:
		// The gapless carve-out belongs to PRIMARY families only. There a missing
		// gap is a positive finding about the TREE: buildCoverageGaps suppressed
		// it because the language's project markers are absent, so that side
		// genuinely had nothing of that language to find. Two pipeline rules keep
		// that the only cause, and both exist so "we did not look" can never render
		// as "there is nothing here": a primary switched off in config arrives here
		// as StatusDisabled rather than absent (markDisabledPrimaries), and a
		// primary with gate: off over a language that IS present keeps its gap
		// (buildCoverageGaps suppresses on the marker, not on the gate).
		//
		// For every other family a missing gap says nothing — those analyzers are
		// mostly not in the install-hint table at all — so the absence stays what
		// the coverage row already said: the config activated this analyzer and it
		// did not run here.
		if f.primary && !decision.HasCoverageGap(gaps, c.Tool) {
			return evidenceNotApplicable
		}
		return evidenceAbsent
	case diagnostic.StatusPartial:
		// Both predicates are shared with `config compare` so the two paths cannot
		// grade one row differently. Note that Unresolved > 0 is NOT the
		// discriminator on its own: go/packages also sets it, counting packages
		// whose load did not complete — and the two ways it can fail decide
		// differently, which is why the second predicate reads the typed counters
		// that split them rather than the row's prose reason.
		switch {
		case decision.PartialFromUnresolvedSpecifiers(c):
			return evidencePartialUnresolved
		case decision.PartialFromDegradedPrecision(c):
			return evidencePartialDegraded
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
	// familyPairedDegradedPrecision — the two sides pair, but both ran with their
	// inputs complete and their per-edge precision degraded. Comparable, and
	// disclosed with each side's count.
	familyPairedDegradedPrecision
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
// Three shapes pair as DEGRADED rather than failing, all because SYMMETRY is the
// safety argument (neither side ran what the other could hide behind) and all
// disclosed in comparison_reasons, never silently:
//
//   - Symmetric partial-with-unresolved-specifiers. For dependency-cruiser and
//     grimp that status is the normal steady state; treating it as unusable made
//     the whole origin delta inert on every TypeScript and Python repo. The
//     residual risk — a base counterpart hidden behind an unresolved specifier —
//     is bounded only by the magnitudes the reason carries.
//   - Symmetric partial-with-complete-inputs. go/packages reports partial when
//     any package fails to type-check, and one such package anywhere made the
//     delta inert on an ordinary Go repo — the project's own primary language.
//     Nothing is missing from either graph there; only strength precision is,
//     which is why the shape pairs with itself and with nothing else.
//   - Symmetric absent: an analyzer the config activated that did not run on
//     either side, typically because its tool is not installed on this host.
//     Treating it as unusable made the delta inert for a whole class of
//     environments, including archfit's own runtime image (no Rust toolchain, no
//     SCIP indexer) on any repo carrying a Cargo.toml. decision.gradeTool grades
//     this shape symmetric-comparable; the two paths must agree.
//
// Symmetric partial-with-unresolved-specifiers never pairs asymmetrically. Nor
// does absent against not_applicable: only a PRIMARY family produces
// not_applicable, and there the two sides are not equally blind — one has none
// of the language, the other has it and could not analyze it (a Cargo.toml ADDED
// by the change is the concrete case).
//
// A missing row, a duplicated row, a timeout, and a partial from a run that did
// not finish are all unavailable evidence and pair with nothing.
func pairFamily(head, base familySummary) familyPairing {
	if head.rows() != 1 || base.rows() != 1 {
		return familyUnpaired // a missing or duplicated coverage row is unavailable evidence
	}
	h, b := head.statuses[0], base.statuses[0]
	// Every degraded shape is tested BEFORE the generic h == b arm below, so a
	// degraded side can only ever pair with the same degradation — never with ok
	// through the equality shortcut, and never with another kind of degradation.
	switch {
	case h == evidencePartialUnresolved || b == evidencePartialUnresolved:
		if h == b {
			return familyPairedDegradedUnresolved
		}
		return familyUnpaired
	case h == evidencePartialDegraded || b == evidencePartialDegraded:
		if h == b {
			return familyPairedDegradedPrecision
		}
		return familyUnpaired
	case h == evidenceAbsent || b == evidenceAbsent:
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
