package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/alexei-led/archfit/internal/config"
	"github.com/alexei-led/archfit/internal/decision"
	"github.com/alexei-led/archfit/internal/model/diagnostic"
	"github.com/alexei-led/archfit/internal/model/finding"
	"github.com/alexei-led/archfit/internal/model/module"
	"github.com/alexei-led/archfit/internal/view"
)

// TestGitFindingDelta covers the `--base` git-origin block: how tasks are placed
// (introduced / pre-existing / unknown), which analyzer evidence is comparable,
// which families are active for a config, and the end-to-end `check --base
// --json` contract at every gate exit code.
//
// One exported test function by design — cmd/archfit sits at its public_api_max
// ceiling, so new coverage arrives as subtests, never as new exported names.
func TestGitFindingDelta(t *testing.T) {
	t.Parallel()
	t.Run("origin", testGitDeltaOrigin)
	t.Run("analyzer_evidence", testGitDeltaAnalyzerEvidence)
	t.Run("active_families", testGitDeltaActiveFamilies)
	t.Run("base_finding_ids", testGitDeltaBaseFindingIDs)
	t.Run("effective_config", testGitDeltaEffectiveConfig)
	t.Run("check_base_json", testGitDeltaCheckBaseJSON)
	t.Run("cross_path_agreement", testGitDeltaCrossPathAgreement)
}

// testGitDeltaCrossPathAgreement drives ONE table of coverage shapes through
// BOTH comparison paths — pairFamily here and decision.gradeTool behind
// `config compare` — and asserts they reach the same verdict.
//
// Three review rounds found the same defect: the two paths graded one input
// shape oppositely (symmetric partial-with-unresolved, then symmetric
// absent-with-a-gap). Prose saying "keep these in step" did not stop it, so the
// agreement is asserted mechanically. The ONE deliberate divergence is listed as
// data, not left implicit.
func testGitDeltaCrossPathAgreement(t *testing.T) {
	t.Parallel()
	partial := func(tool string, unresolved int) diagnostic.Coverage {
		c := covRow(tool, diagnostic.StatusPartial)
		c.Unresolved = unresolved
		return c
	}
	gapFor := func(tool string) []diagnostic.CoverageGap { return []diagnostic.CoverageGap{{Tool: tool}} }

	tests := []struct {
		name       string
		fam        analyzerFamily
		head, base []diagnostic.Coverage
		headGap    []diagnostic.CoverageGap
		baseGap    []diagnostic.CoverageGap
		// wantGit is `--base` comparability; wantDecision is `config compare`
		// reaching anything better than not_comparable. They must be equal unless
		// divergent says otherwise.
		wantGit      bool
		wantDecision bool
		// divergent marks the one shape the two paths are SPECIFIED to grade
		// differently, with the reason.
		divergent string
	}{
		{name: "ok both sides", fam: scipFamily, head: []diagnostic.Coverage{covRow(toolScip, diagnostic.StatusOK)}, base: []diagnostic.Coverage{covRow(toolScip, diagnostic.StatusOK)}, wantGit: true, wantDecision: true},
		{name: "absent both sides, no gap", fam: scipFamily, head: []diagnostic.Coverage{covRow(toolScip, diagnostic.StatusAbsent)}, base: []diagnostic.Coverage{covRow(toolScip, diagnostic.StatusAbsent)}, wantGit: true, wantDecision: true},
		{name: "absent both sides with a gap", fam: scipFamily, head: []diagnostic.Coverage{covRow(toolScip, diagnostic.StatusAbsent)}, base: []diagnostic.Coverage{covRow(toolScip, diagnostic.StatusAbsent)}, headGap: gapFor(toolScip), baseGap: gapFor(toolScip), wantGit: true, wantDecision: true},
		{name: "absent gapped on one side only", fam: scipFamily, head: []diagnostic.Coverage{covRow(toolScip, diagnostic.StatusAbsent)}, base: []diagnostic.Coverage{covRow(toolScip, diagnostic.StatusAbsent)}, headGap: gapFor(toolScip), wantGit: false, wantDecision: false},
		// The shape Fix 2 changed for a PRIMARY analyzer: the gap is decided
		// before the primary carve-out on both paths, so an uninstalled tool reads
		// the same whichever family it belongs to.
		{name: "primary absent both sides with a gap", fam: analyzerFamily{name: toolGoPackages, primary: true}, head: []diagnostic.Coverage{covRow(toolGoPackages, diagnostic.StatusAbsent)}, base: []diagnostic.Coverage{covRow(toolGoPackages, diagnostic.StatusAbsent)}, headGap: gapFor(toolGoPackages), baseGap: gapFor(toolGoPackages), wantGit: true, wantDecision: true},
		{name: "primary absent gapped on one side only", fam: analyzerFamily{name: toolGoPackages, primary: true}, head: []diagnostic.Coverage{covRow(toolGoPackages, diagnostic.StatusAbsent)}, base: []diagnostic.Coverage{covRow(toolGoPackages, diagnostic.StatusAbsent)}, headGap: gapFor(toolGoPackages), wantGit: false, wantDecision: false},
		{name: "absent against ok", fam: scipFamily, head: []diagnostic.Coverage{covRow(toolScip, diagnostic.StatusAbsent)}, base: []diagnostic.Coverage{covRow(toolScip, diagnostic.StatusOK)}, wantGit: false, wantDecision: false},
		{name: "disabled both sides", fam: scipFamily, head: []diagnostic.Coverage{covRow(toolScip, diagnostic.StatusDisabled)}, base: []diagnostic.Coverage{covRow(toolScip, diagnostic.StatusDisabled)}, wantGit: true, wantDecision: true},
		{name: "timed out both sides", fam: scipFamily, head: []diagnostic.Coverage{covRow(toolScip, diagnostic.StatusTimedOut)}, base: []diagnostic.Coverage{covRow(toolScip, diagnostic.StatusTimedOut)}, wantGit: false, wantDecision: false},
		{name: "duplicate rows both sides", fam: scipFamily, head: []diagnostic.Coverage{covRow(toolScip, diagnostic.StatusOK), covRow(toolScip, diagnostic.StatusOK)}, base: []diagnostic.Coverage{covRow(toolScip, diagnostic.StatusOK), covRow(toolScip, diagnostic.StatusOK)}, wantGit: false, wantDecision: false},
		{name: "specifier partial both sides", fam: analyzerFamily{name: toolDepCruiser}, head: []diagnostic.Coverage{partial(toolDepCruiser, 4)}, base: []diagnostic.Coverage{partial(toolDepCruiser, 9)}, wantGit: true, wantDecision: true},
		{name: "specifier partial against ok", fam: analyzerFamily{name: toolDepCruiser}, head: []diagnostic.Coverage{partial(toolDepCruiser, 4)}, base: []diagnostic.Coverage{covRow(toolDepCruiser, diagnostic.StatusOK)}, wantGit: false, wantDecision: false},
		{name: "partial with no unresolved count both sides", fam: analyzerFamily{name: toolDepCruiser}, head: []diagnostic.Coverage{covRow(toolDepCruiser, diagnostic.StatusPartial)}, base: []diagnostic.Coverage{covRow(toolDepCruiser, diagnostic.StatusPartial)}, wantGit: false, wantDecision: false},
		// go/packages counts SKIPPED PACKAGES in Unresolved, so its partial is a
		// run that did not finish — both paths must refuse it.
		{name: "go/packages skipped-package partial both sides", fam: analyzerFamily{name: toolGoPackages, primary: true}, head: []diagnostic.Coverage{partial(toolGoPackages, 3)}, base: []diagnostic.Coverage{partial(toolGoPackages, 3)}, wantGit: false, wantDecision: false},
		{
			name: "ok against a gapless-absent primary",
			fam:  analyzerFamily{name: toolGoPackages, primary: true},
			head: []diagnostic.Coverage{covRow(toolGoPackages, diagnostic.StatusOK)},
			base: []diagnostic.Coverage{covRow(toolGoPackages, diagnostic.StatusAbsent)},
			// --base compares two TREES, so a language appearing between them is
			// expected; `config compare` compares ONE tree, so the same status move
			// can only have been caused by the configuration.
			wantGit: true, wantDecision: false,
			divergent: "--base compares two trees, config compare compares one",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			gotGit, _ := compareAnalyzerEvidence([]analyzerFamily{tc.fam},
				analyzerEvidence{Coverage: tc.head, Gaps: tc.headGap},
				analyzerEvidence{Coverage: tc.base, Gaps: tc.baseGap})
			if gotGit != tc.wantGit {
				t.Fatalf("--base comparable = %v, want %v", gotGit, tc.wantGit)
			}

			var primary []string
			if tc.fam.primary {
				primary = []string{tc.fam.name}
			}
			cmp := decision.CompareConfigs(decision.ConfigCompareInput{
				Current: decision.ConfigCompareSide{Diag: diagnostic.Diagnostic{
					ToolCoverage: tc.head, CoverageGaps: tc.headGap, PrimaryExtractorTools: primary,
				}},
				Candidate: decision.ConfigCompareSide{Diag: diagnostic.Diagnostic{
					ToolCoverage: tc.base, CoverageGaps: tc.baseGap, PrimaryExtractorTools: primary,
				}},
			})
			gotDecision := cmp.Coverage.Status != decision.CoverageNotComparable
			if gotDecision != tc.wantDecision {
				t.Fatalf("config compare comparable = %v, want %v (status %q)", gotDecision, tc.wantDecision, cmp.Coverage.Status)
			}
			if tc.divergent == "" && gotGit != gotDecision {
				t.Fatalf("the two comparison paths disagree on an undocumented shape: --base=%v, config compare=%v (%q)",
					gotGit, gotDecision, cmp.Coverage.Status)
			}
		})
	}
}

// scipFamily is the non-primary analyzer family the cross-path table compares on.
var scipFamily = analyzerFamily{name: toolScip}

// gitDeltaRef is the base ref label used by the pure-comparison subtests.
const gitDeltaRef = "main"

func covRow(tool, status string) diagnostic.Coverage {
	return diagnostic.Coverage{Tool: tool, Status: status}
}

func agentTask(findingID, ruleID string) diagnostic.AgentTask {
	return diagnostic.AgentTask{FindingID: findingID, RuleID: ruleID}
}

// goPrimaryFamily is the single-family fixture used by the origin table: the
// pairing rules themselves are covered by testGitDeltaAnalyzerEvidence.
var goPrimaryFamily = []analyzerFamily{{name: toolGoPackages, primary: true}}

func testGitDeltaOrigin(t *testing.T) {
	t.Parallel()
	const hash = "cfg-hash"
	comparableSide := analyzerEvidence{Coverage: []diagnostic.Coverage{covRow(toolGoPackages, diagnostic.StatusOK)}, Hash: hash}
	partialSide := analyzerEvidence{Coverage: []diagnostic.Coverage{covRow(toolGoPackages, diagnostic.StatusPartial)}, Hash: hash}

	tests := []struct {
		name            string
		tasks           []diagnostic.AgentTask
		baseIDs         []string
		base            analyzerEvidence
		wantIntroduced  []string
		wantPreExisting []string
		wantUnknown     []string
		wantStatus      string
	}{
		{
			name:            "exact base match is pre-existing",
			tasks:           []diagnostic.AgentTask{agentTask("f1", "arch/forbidden")},
			baseIDs:         []string{"f1"},
			base:            comparableSide,
			wantPreExisting: []string{"f1"},
			wantStatus:      diagnostic.GitComparisonComparable,
		},
		{
			name:           "unmatched task with comparable evidence is introduced",
			tasks:          []diagnostic.AgentTask{agentTask("f2", "arch/forbidden")},
			baseIDs:        []string{"f1"},
			base:           comparableSide,
			wantIntroduced: []string{"f2"},
			wantStatus:     diagnostic.GitComparisonComparable,
		},
		{
			// A base entry the base run reported as fixed is dropped by
			// baseFindingIDs, so the same ID on head is genuinely new work.
			name:           "base fixed entry does not make a task pre-existing",
			tasks:          []diagnostic.AgentTask{agentTask("f1", "arch/forbidden")},
			baseIDs:        nil,
			base:           comparableSide,
			wantIntroduced: []string{"f1"},
			wantStatus:     diagnostic.GitComparisonComparable,
		},
		{
			name:        "unavailable analyzer evidence makes an unmatched task unknown",
			tasks:       []diagnostic.AgentTask{agentTask("f2", "arch/forbidden")},
			baseIDs:     []string{"f1"},
			base:        partialSide,
			wantUnknown: []string{"f2"},
			wantStatus:  diagnostic.GitComparisonUnknown,
		},
		{
			// Incomplete evidence never downgrades an exact ID match.
			name:            "exact match survives unavailable evidence",
			tasks:           []diagnostic.AgentTask{agentTask("f1", "arch/forbidden")},
			baseIDs:         []string{"f1"},
			base:            partialSide,
			wantPreExisting: []string{"f1"},
			wantStatus:      diagnostic.GitComparisonComparable,
		},
		{
			name:        "synthetic coupling-gate task is unknown before ID matching",
			tasks:       []diagnostic.AgentTask{agentTask(findingIDCouplingGate, ruleIDBCCouplingGate)},
			baseIDs:     []string{findingIDCouplingGate},
			base:        comparableSide,
			wantUnknown: []string{findingIDCouplingGate},
			wantStatus:  diagnostic.GitComparisonUnknown,
		},
		{
			name:    "config hash mismatch makes every unmatched task unknown",
			tasks:   []diagnostic.AgentTask{agentTask("f1", "arch/forbidden"), agentTask("f2", "arch/forbidden")},
			baseIDs: []string{"f1"},
			base: analyzerEvidence{
				Coverage: []diagnostic.Coverage{covRow(toolGoPackages, diagnostic.StatusOK)},
				Hash:     "other-hash",
			},
			wantPreExisting: []string{"f1"},
			wantUnknown:     []string{"f2"},
			wantStatus:      diagnostic.GitComparisonUnknown,
		},
		{
			name:            "lists use a stable sorted order",
			tasks:           []diagnostic.AgentTask{agentTask("z", "r"), agentTask("a", "r"), agentTask("m", "r"), agentTask("b", "r")},
			baseIDs:         []string{"m", "b"},
			base:            comparableSide,
			wantIntroduced:  []string{"a", "z"},
			wantPreExisting: []string{"b", "m"},
			wantStatus:      diagnostic.GitComparisonComparable,
		},
		{
			name:       "clean run still emits the block with empty lists",
			base:       comparableSide,
			wantStatus: diagnostic.GitComparisonComparable,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got := buildGitFindingDelta(gitDeltaInput{
				BaseRef:        gitDeltaRef,
				Tasks:          tc.tasks,
				BaseFindingIDs: tc.baseIDs,
				Head: analyzerEvidence{
					Coverage: []diagnostic.Coverage{covRow(toolGoPackages, diagnostic.StatusOK)},
					Hash:     hash,
				},
				Base:     tc.base,
				Families: goPrimaryFamily,
			})
			if got == nil {
				t.Fatal("buildGitFindingDelta returned nil; the block must always be present with --base")
			}
			if got.BaseRef != gitDeltaRef {
				t.Errorf("base_ref = %q, want %q", got.BaseRef, gitDeltaRef)
			}
			if got.ComparisonStatus != tc.wantStatus {
				t.Errorf("comparison_status = %q, want %q", got.ComparisonStatus, tc.wantStatus)
			}
			assertIDs(t, "introduced_finding_ids", got.IntroducedFindingIDs, tc.wantIntroduced)
			assertIDs(t, "pre_existing_finding_ids", got.PreExistingFindingIDs, tc.wantPreExisting)
			assertIDs(t, "unknown_origin_finding_ids", got.UnknownOriginFindingIDs, tc.wantUnknown)
			if got.ComparisonReasons == nil {
				t.Error("comparison_reasons must be a non-null array")
			}
			assertNonNullJSONArrays(t, got)
		})
	}

	// The whole point of the partial split: on a TypeScript or Python repo the
	// primary analyzer reports partial on both sides as its steady state. Before
	// the split that pinned every unmatched task to unknown, which made the
	// origin delta inert on those languages.
	t.Run("symmetric unresolved partial still places an unmatched task", func(t *testing.T) {
		t.Parallel()
		unresolvedSide := func(n int) analyzerEvidence {
			row := covRow(toolDepCruiser, diagnostic.StatusPartial)
			row.Unresolved = n
			return analyzerEvidence{Coverage: []diagnostic.Coverage{row}, Hash: hash}
		}
		got := buildGitFindingDelta(gitDeltaInput{
			BaseRef:        gitDeltaRef,
			Tasks:          []diagnostic.AgentTask{agentTask("f2", "arch/forbidden")},
			BaseFindingIDs: []string{"f1"},
			Head:           unresolvedSide(4),
			Base:           unresolvedSide(6),
			Families:       []analyzerFamily{{name: toolDepCruiser, primary: true}},
		})
		assertIDs(t, "introduced_finding_ids", got.IntroducedFindingIDs, []string{"f2"})
		assertIDs(t, "unknown_origin_finding_ids", got.UnknownOriginFindingIDs, nil)
		if got.ComparisonStatus != diagnostic.GitComparisonComparable {
			t.Errorf("comparison_status = %q, want %q", got.ComparisonStatus, diagnostic.GitComparisonComparable)
		}
		if len(got.ComparisonReasons) != 1 {
			t.Fatalf("comparison_reasons = %v, want the degradation disclosed", got.ComparisonReasons)
		}
	})
}

// assertIDs compares one ID list against its expectation and rejects a nil
// slice, which would serialise as JSON null instead of [].
func assertIDs(t *testing.T, field string, got, want []string) {
	t.Helper()
	if got == nil {
		t.Errorf("%s must be a non-null array", field)
		return
	}
	if want == nil {
		want = []string{}
	}
	if !slices.Equal(got, want) {
		t.Errorf("%s = %v, want %v", field, got, want)
	}
}

func assertNonNullJSONArrays(t *testing.T, d *diagnostic.GitFindingDelta) {
	t.Helper()
	raw, err := json.Marshal(d)
	if err != nil {
		t.Fatalf("marshal git_finding_delta: %v", err)
	}
	if strings.Contains(string(raw), "null") {
		t.Errorf("git_finding_delta must never serialise a null list: %s", raw)
	}
}

func testGitDeltaAnalyzerEvidence(t *testing.T) {
	t.Parallel()
	goFam := analyzerFamily{name: toolGoPackages, primary: true}
	dcFam := analyzerFamily{name: toolDepCruiser, primary: true}
	scipFam := analyzerFamily{name: toolScip}
	astFam := analyzerFamily{name: toolAstGrep}
	goGap := []diagnostic.CoverageGap{{Tool: toolGoPackages}}
	scipGap := []diagnostic.CoverageGap{{Tool: toolScip}}

	// unresolvedRow is the dependency-cruiser/grimp steady state: the analyzer
	// walked the whole tree and could not resolve n import specifiers.
	unresolvedRow := func(n int) diagnostic.Coverage {
		c := covRow(toolDepCruiser, diagnostic.StatusPartial)
		c.Unresolved = n
		return c
	}
	// goUnresolvedRow is NOT that shape: go/packages counts whole packages it
	// SKIPPED because they failed to load, so its partial means the run did not
	// finish over the tree.
	goUnresolvedRow := func(n int) diagnostic.Coverage {
		c := covRow(toolGoPackages, diagnostic.StatusPartial)
		c.Unresolved = n
		return c
	}

	tests := []struct {
		name           string
		fam            analyzerFamily
		head, base     []diagnostic.Coverage
		headGap, bsGap []diagnostic.CoverageGap
		want           bool
		// degraded marks a pair that IS comparable but must still disclose one
		// reason (symmetric unresolved-specifier partial).
		degraded bool
	}{
		{name: "ok/ok", fam: goFam, head: []diagnostic.Coverage{covRow(toolGoPackages, diagnostic.StatusOK)}, base: []diagnostic.Coverage{covRow(toolGoPackages, diagnostic.StatusOK)}, want: true},
		{name: "ok/not_applicable", fam: goFam, head: []diagnostic.Coverage{covRow(toolGoPackages, diagnostic.StatusOK)}, base: []diagnostic.Coverage{covRow(toolGoPackages, diagnostic.StatusAbsent)}, want: true},
		{name: "not_applicable/ok", fam: goFam, head: []diagnostic.Coverage{covRow(toolGoPackages, diagnostic.StatusAbsent)}, base: []diagnostic.Coverage{covRow(toolGoPackages, diagnostic.StatusOK)}, want: true},
		{name: "not_applicable both sides is ignored", fam: goFam, head: []diagnostic.Coverage{covRow(toolGoPackages, diagnostic.StatusAbsent)}, base: []diagnostic.Coverage{covRow(toolGoPackages, diagnostic.StatusAbsent)}, want: true},
		{name: "primary absent with a coverage gap is unavailable", fam: goFam, head: []diagnostic.Coverage{covRow(toolGoPackages, diagnostic.StatusOK)}, base: []diagnostic.Coverage{covRow(toolGoPackages, diagnostic.StatusAbsent)}, bsGap: goGap, want: false},
		{name: "partial with no unresolved count is unavailable", fam: goFam, head: []diagnostic.Coverage{covRow(toolGoPackages, diagnostic.StatusOK)}, base: []diagnostic.Coverage{covRow(toolGoPackages, diagnostic.StatusPartial)}, want: false},
		// dependency-cruiser and grimp mark a COMPLETED run partial as soon as one
		// import specifier anywhere fails to resolve. Symmetric, it is shared
		// incompleteness over one tree: comparable, but always disclosed.
		{name: "symmetric unresolved partial is comparable and disclosed", fam: dcFam, head: []diagnostic.Coverage{unresolvedRow(4)}, base: []diagnostic.Coverage{unresolvedRow(9)}, want: true, degraded: true},
		{name: "unresolved partial never pairs with ok", fam: dcFam, head: []diagnostic.Coverage{unresolvedRow(4)}, base: []diagnostic.Coverage{covRow(toolDepCruiser, diagnostic.StatusOK)}, want: false},
		{name: "unresolved partial never pairs with a failed partial", fam: dcFam, head: []diagnostic.Coverage{unresolvedRow(4)}, base: []diagnostic.Coverage{covRow(toolDepCruiser, diagnostic.StatusPartial)}, want: false},
		// go/packages sets Unresolved on a partial too, but there it counts whole
		// packages it SKIPPED because they failed to load — the "did not finish"
		// meaning. A symmetric Go partial must NOT read as shared incompleteness,
		// or a base side with N unloaded packages produces a false "introduced".
		{name: "go/packages skipped-package partial is unavailable on both sides", fam: goFam, head: []diagnostic.Coverage{goUnresolvedRow(3)}, base: []diagnostic.Coverage{goUnresolvedRow(3)}, want: false},
		{name: "go/packages skipped-package partial never pairs with ok", fam: goFam, head: []diagnostic.Coverage{goUnresolvedRow(3)}, base: []diagnostic.Coverage{covRow(toolGoPackages, diagnostic.StatusOK)}, want: false},
		{name: "timed out is unavailable", fam: goFam, head: []diagnostic.Coverage{covRow(toolGoPackages, diagnostic.StatusTimedOut)}, base: []diagnostic.Coverage{covRow(toolGoPackages, diagnostic.StatusOK)}, want: false},
		{name: "missing row on one side is unavailable", fam: goFam, base: []diagnostic.Coverage{covRow(toolGoPackages, diagnostic.StatusOK)}, want: false},
		{name: "missing row on both sides is unavailable", fam: goFam, want: false},
		{name: "duplicate row on one side is unavailable", fam: goFam, head: []diagnostic.Coverage{covRow(toolGoPackages, diagnostic.StatusOK), covRow(toolGoPackages, diagnostic.StatusOK)}, base: []diagnostic.Coverage{covRow(toolGoPackages, diagnostic.StatusOK)}, want: false},
		// Every analyzer owns its own coverage name, so a repeated name is an
		// anomaly on BOTH sides too — there is no way to know which duplicate
		// pairs with which. Same rule as decision.gradeTool.
		{name: "matching duplicate rows are still unavailable", fam: astFam, head: []diagnostic.Coverage{covRow(toolAstGrep, diagnostic.StatusOK), covRow(toolAstGrep, diagnostic.StatusOK)}, base: []diagnostic.Coverage{covRow(toolAstGrep, diagnostic.StatusOK), covRow(toolAstGrep, diagnostic.StatusOK)}, want: false},
		{name: "the pattern pass ignores the syntax pass's own row", fam: astFam, head: []diagnostic.Coverage{covRow(toolAstGrep, diagnostic.StatusOK), covRow(toolAstGrepSyntax, diagnostic.StatusDisabled)}, base: []diagnostic.Coverage{covRow(toolAstGrep, diagnostic.StatusOK), covRow(toolAstGrepSyntax, diagnostic.StatusOK)}, want: true},
		{name: "the syntax pass compares on its own row", fam: analyzerFamily{name: toolAstGrepSyntax}, head: []diagnostic.Coverage{covRow(toolAstGrep, diagnostic.StatusOK), covRow(toolAstGrepSyntax, diagnostic.StatusDisabled)}, base: []diagnostic.Coverage{covRow(toolAstGrep, diagnostic.StatusOK), covRow(toolAstGrepSyntax, diagnostic.StatusOK)}, want: false},
		{name: "disabled on both sides is ignored", fam: scipFam, head: []diagnostic.Coverage{covRow(toolScip, diagnostic.StatusDisabled)}, base: []diagnostic.Coverage{covRow(toolScip, diagnostic.StatusDisabled)}, want: true},
		{name: "disabled on one side only is unavailable", fam: scipFam, head: []diagnostic.Coverage{covRow(toolScip, diagnostic.StatusOK)}, base: []diagnostic.Coverage{covRow(toolScip, diagnostic.StatusDisabled)}, want: false},
		// A non-primary analyzer's gapless absence is evidence about the TOOL,
		// not the tree: asymmetric absence could hide a base finding, symmetric
		// absence means neither side produced one.
		{name: "non-primary absent on one side only is unavailable", fam: scipFam, head: []diagnostic.Coverage{covRow(toolScip, diagnostic.StatusOK)}, base: []diagnostic.Coverage{covRow(toolScip, diagnostic.StatusAbsent)}, want: false},
		{name: "non-primary absent on both sides is comparable", fam: scipFam, head: []diagnostic.Coverage{covRow(toolScip, diagnostic.StatusAbsent)}, base: []diagnostic.Coverage{covRow(toolScip, diagnostic.StatusAbsent)}, want: true},
		// CHANGED from want:false. An enabled analyzer whose tool is missing on
		// the host reports absent WITH a gap on BOTH sides. Symmetric, that is the
		// same safety argument as gapless symmetric absence — neither side ran it,
		// so neither has a finding the other hides — and decision.gradeTool
		// already grades it comparable_with_gaps. Failing it made --base
		// permanently all-unknown wherever an enabled analyzer is uninstalled,
		// including archfit's own runtime image on any repo with a Cargo.toml.
		// It pairs DEGRADED: the shared blindness is always disclosed.
		{name: "absent with a coverage gap on both sides is comparable and disclosed", fam: scipFam, head: []diagnostic.Coverage{covRow(toolScip, diagnostic.StatusAbsent)}, base: []diagnostic.Coverage{covRow(toolScip, diagnostic.StatusAbsent)}, headGap: scipGap, bsGap: scipGap, want: true, degraded: true},
		{name: "primary absent with a coverage gap on both sides is comparable and disclosed", fam: goFam, head: []diagnostic.Coverage{covRow(toolGoPackages, diagnostic.StatusAbsent)}, base: []diagnostic.Coverage{covRow(toolGoPackages, diagnostic.StatusAbsent)}, headGap: goGap, bsGap: goGap, want: true, degraded: true},
		// The gap is derived per side from that side's own tree, so a project
		// marker ADDED by the change gaps head and not base. That asymmetry means
		// the two sides are not equally blind and must never pair.
		{name: "absent gapped on head only is unavailable", fam: scipFam, head: []diagnostic.Coverage{covRow(toolScip, diagnostic.StatusAbsent)}, base: []diagnostic.Coverage{covRow(toolScip, diagnostic.StatusAbsent)}, headGap: scipGap, want: false},
		{name: "absent gapped on base only is unavailable", fam: scipFam, head: []diagnostic.Coverage{covRow(toolScip, diagnostic.StatusAbsent)}, base: []diagnostic.Coverage{covRow(toolScip, diagnostic.StatusAbsent)}, bsGap: scipGap, want: false},
		{name: "absent gapped never pairs with ok", fam: scipFam, head: []diagnostic.Coverage{covRow(toolScip, diagnostic.StatusAbsent)}, base: []diagnostic.Coverage{covRow(toolScip, diagnostic.StatusOK)}, headGap: scipGap, want: false},
		{name: "absent gapped never pairs with a timeout", fam: scipFam, head: []diagnostic.Coverage{covRow(toolScip, diagnostic.StatusAbsent)}, base: []diagnostic.Coverage{covRow(toolScip, diagnostic.StatusTimedOut)}, headGap: scipGap, want: false},
		// A timeout is flaky, not structural: symmetry proves nothing about what
		// either side would have found, so it stays unavailable on both sides.
		{name: "timed out on both sides is still unavailable", fam: scipFam, head: []diagnostic.Coverage{covRow(toolScip, diagnostic.StatusTimedOut)}, base: []diagnostic.Coverage{covRow(toolScip, diagnostic.StatusTimedOut)}, want: false},
		{name: "non-primary absent never pairs with ok", fam: scipFam, head: []diagnostic.Coverage{covRow(toolScip, diagnostic.StatusAbsent)}, base: []diagnostic.Coverage{covRow(toolScip, diagnostic.StatusOK)}, want: false},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			ok, reasons := compareAnalyzerEvidence([]analyzerFamily{tc.fam},
				analyzerEvidence{Coverage: tc.head, Gaps: tc.headGap},
				analyzerEvidence{Coverage: tc.base, Gaps: tc.bsGap})
			if ok != tc.want {
				t.Fatalf("comparable = %v, want %v (reasons: %v)", ok, tc.want, reasons)
			}
			// A family reports one reason when it is unpairable, and also when it
			// pairs only in degraded form — the loss is disclosed either way.
			wantReasons := 1
			if tc.want && !tc.degraded {
				wantReasons = 0
			}
			if len(reasons) != wantReasons {
				t.Fatalf("reasons = %v, want %d", reasons, wantReasons)
			}
			if wantReasons == 1 && !strings.HasPrefix(reasons[0], tc.fam.name+": ") {
				t.Errorf("reason %q must name the family %q", reasons[0], tc.fam.name)
			}
		})
	}

	// The degraded pairing rule is magnitude-blind on purpose, so the reason it
	// emits is the ONLY place the magnitude can appear. Without the numbers,
	// 4 unresolved specifiers and 5000 of 6000 read identically.
	t.Run("a degraded reason carries both sides' unresolved magnitudes", func(t *testing.T) {
		t.Parallel()
		heavy := unresolvedRow(5000)
		heavy.SpecifiersSeen = 6000
		ok, reasons := compareAnalyzerEvidence([]analyzerFamily{dcFam},
			analyzerEvidence{Coverage: []diagnostic.Coverage{unresolvedRow(4)}},
			analyzerEvidence{Coverage: []diagnostic.Coverage{heavy}})
		if !ok || len(reasons) != 1 {
			t.Fatalf("comparable=%v reasons=%v, want comparable with one reason", ok, reasons)
		}
		for _, want := range []string{"4 unresolved", "5000/6000 unresolved"} {
			if !strings.Contains(reasons[0], want) {
				t.Errorf("reason %q must contain %q", reasons[0], want)
			}
		}
	})

	t.Run("reasons are sorted and one per family", func(t *testing.T) {
		t.Parallel()
		fams := []analyzerFamily{
			{name: toolScip},
			{name: toolGoPackages, primary: true},
			{name: toolJscpd},
		}
		head := analyzerEvidence{Coverage: []diagnostic.Coverage{
			covRow(toolScip, diagnostic.StatusOK),
			covRow(toolGoPackages, diagnostic.StatusOK),
			covRow(toolJscpd, diagnostic.StatusOK),
		}}
		base := analyzerEvidence{Coverage: []diagnostic.Coverage{
			covRow(toolScip, diagnostic.StatusPartial),
			covRow(toolGoPackages, diagnostic.StatusTimedOut),
			covRow(toolJscpd, diagnostic.StatusOK),
		}}
		delta := buildGitFindingDelta(gitDeltaInput{BaseRef: gitDeltaRef, Head: head, Base: base, Families: fams})
		if len(delta.ComparisonReasons) != 2 {
			t.Fatalf("comparison_reasons = %v, want one per unavailable family", delta.ComparisonReasons)
		}
		if !slices.IsSorted(delta.ComparisonReasons) {
			t.Errorf("comparison_reasons must be sorted: %v", delta.ComparisonReasons)
		}
	})
}

func testGitDeltaActiveFamilies(t *testing.T) {
	t.Parallel()
	names := func(fams []analyzerFamily) []string {
		out := make([]string, 0, len(fams))
		for _, f := range fams {
			out = append(out, f.name)
		}
		return out
	}
	primaries := names(analyzerFamilies(config.Config{}))
	if len(primaries) != len(languageRegistry) {
		t.Fatalf("a bare config must activate only the per-language primaries, got %v", primaries)
	}

	on := config.Analyzer{Enabled: view.ModeOn}
	timedOn := config.TimedAnalyzer{Enabled: view.ModeOn}
	tests := []struct {
		name string
		cfg  config.Config
		want []string
	}{
		{name: "rule patterns activate the ast-grep pattern pass only", cfg: config.Config{
			Rules: []view.RuleDef{{ID: "r", Patterns: []view.PatternDef{{ID: "p", Lang: "go", Rule: "x"}}}},
		}, want: []string{toolAstGrep}},
		{name: "syntax activates the ast-grep syntax pass only", cfg: config.Config{
			Analyzers: config.AnalyzersConfig{Syntax: on},
		}, want: []string{toolAstGrepSyntax}},
		{name: "patterns and syntax activate two independent families", cfg: config.Config{
			Rules:     []view.RuleDef{{ID: "r", Patterns: []view.PatternDef{{ID: "p", Lang: "go", Rule: "x"}}}},
			Analyzers: config.AnalyzersConfig{Syntax: on},
		}, want: []string{toolAstGrep, toolAstGrepSyntax}},
		{name: "scip activates both scip rows", cfg: config.Config{
			Analyzers: config.AnalyzersConfig{Scip: timedOn},
		}, want: []string{toolScip, toolScipSymbols}},
		{name: "clones activate jscpd", cfg: config.Config{
			Analyzers: config.AnalyzersConfig{Clones: timedOn},
		}, want: []string{toolJscpd}},
		{name: "cargo modules activate cargo-modules", cfg: config.Config{
			Analyzers: config.AnalyzersConfig{CargoModules: on},
		}, want: []string{toolCargoModules}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got := names(analyzerFamilies(tc.cfg))
			for _, want := range tc.want {
				if !slices.Contains(got, want) {
					t.Errorf("family %q missing from %v", want, got)
				}
			}
			if len(got) != len(primaries)+len(tc.want) {
				t.Errorf("families = %v, want the primaries plus %v", got, tc.want)
			}
		})
	}
}

// testGitDeltaBaseFindingIDs pins the base projection: every lifecycle label
// except `fixed` is observed evidence, and the finding's kind is irrelevant —
// a base advisory still matches a head task promoted to gate kind, because the
// stable ID is all that crosses over.
func testGitDeltaBaseFindingIDs(t *testing.T) {
	t.Parallel()
	got := baseFindingIDs([]finding.Finding{
		{ID: "z", Kind: string(finding.KindAdvisory), Status: finding.StatusNew},
		{ID: "gone", Kind: string(finding.KindGate), Status: finding.StatusFixed},
		{ID: "a", Kind: string(finding.KindGate), Status: finding.StatusWaived},
		{ID: "m", Kind: string(finding.KindAdvisory), Status: finding.StatusExpiredWaiver},
	})
	if want := []string{"a", "m", "z"}; !slices.Equal(got, want) {
		t.Errorf("baseFindingIDs = %v, want %v (fixed dropped, sorted, kind ignored)", got, want)
	}
}

// testGitDeltaEffectiveConfig covers the base sub-run's config contract: it gets
// the caller's effective config (flag overrides included) through an independent
// module map, so the head pipeline's owner and deploy-unit backfill cannot leak
// head-tree evidence into the base measurement.
func testGitDeltaEffectiveConfig(t *testing.T) {
	t.Parallel()
	t.Run("module map is independent of the head config", func(t *testing.T) {
		t.Parallel()
		original := config.Config{Modules: map[string]module.ModuleDef{
			"a": {Paths: []string{"pkg/a/**"}},
		}}
		snapshot := withIndependentModules(original)
		// Stand in for runPipeline's owner backfill, which writes through the map.
		original.FillMissingOwners(map[string]string{"a": "head-tree-owner"})
		if def := snapshot.Modules["a"]; def.Owner != "" {
			t.Errorf("base config inherited a head-tree owner %q", def.Owner)
		}
		if def := original.Modules["a"]; def.Owner != "head-tree-owner" {
			t.Errorf("head config lost its own backfill: %+v", def)
		}
	})

	// The call site, not the helper: analyze.go must snapshot the module map
	// BEFORE the head pipeline backfills owners into it. Replace
	// `withIndependentModules(cfg)` with `cfg` there and the base run inherits
	// the head tree's per-module owners, classifies the shared edge at
	// cross_module_different_owner it never observed, and reports a critical
	// finding that makes a genuinely pre-existing seam look pre-existing for the
	// wrong reason — or, as here, hides that the seam is unchanged.
	t.Run("head-tree owners do not reach the base measurement", func(t *testing.T) {
		t.Parallel()
		cfgPath := gitDeltaOwnerFixtureRepo(t)
		code, stdout, stderr := runArchfit(t, cmdAnalyze, flagBase, diffBaseRef, "--json", "-c", cfgPath)
		if code != 0 {
			t.Fatalf("analyze --base: exit = %d\nstdout:\n%s\nstderr:\n%s", code, stdout, stderr)
		}
		var got gitDeltaJSON
		if err := json.Unmarshal([]byte(stdout), &got); err != nil {
			t.Fatalf("invalid JSON: %v\n%s", err, stdout)
		}
		if got.GitFindingDelta == nil {
			t.Fatalf("--base --json must emit git_finding_delta\n%s", stdout)
		}
		// The head tree splits ownership, so the shared edge crosses owners and
		// scores critical. Without that split it is a same-owner sibling edge
		// below coupling.min_severity, so the base side reports nothing.
		if len(got.AgentTasks) != 1 {
			t.Fatalf("fixture regression: the head owner split must produce exactly one task: %+v", got.AgentTasks)
		}
		d := got.GitFindingDelta
		if len(d.ComparisonReasons) != 0 {
			t.Fatalf("fixture regression: both sides compile, want no comparison_reasons, got %v", d.ComparisonReasons)
		}
		if !slices.Contains(d.Introduced, got.AgentTasks[0].FindingID) {
			t.Errorf("the base run measured head-tree owners: introduced = %v, pre_existing = %v, unknown = %v",
				d.Introduced, d.PreExisting, d.UnknownOrigin)
		}
	})

	t.Run("analyzer overrides still reach the base run", func(t *testing.T) {
		t.Parallel()
		cfgPath := gitDeltaFixtureRepo(t, coupledModulesCfg)
		code, stdout, stderr := runArchfit(t, cmdAnalyze, flagBase, diffBaseRef, "--json", "--lang", "go", "-c", cfgPath)
		if code != 0 {
			t.Fatalf("analyze --base --lang go: exit = %d\nstdout:\n%s\nstderr:\n%s", code, stdout, stderr)
		}
		var got gitDeltaJSON
		if err := json.Unmarshal([]byte(stdout), &got); err != nil {
			t.Fatalf("invalid JSON: %v\n%s", err, stdout)
		}
		if got.GitFindingDelta == nil {
			t.Fatalf("--base with a --lang override must still emit git_finding_delta\n%s", stdout)
		}
		// Both sides ran the same forced analyzer set, so nothing is unavailable.
		if len(got.GitFindingDelta.ComparisonReasons) != 0 {
			t.Errorf("comparison_reasons = %v, want none when both sides force the same analyzers",
				got.GitFindingDelta.ComparisonReasons)
		}
	})
}

// gitDeltaFixtureRepo builds a two-commit Go repo: the base commit holds only
// pkg/b, the head commit adds a pkg/a → pkg/b importer. The head run therefore
// carries a cross-module edge the base ref does not, and BOTH sides compile, so
// go/packages reports ok on both and the evidence is genuinely comparable.
func gitDeltaFixtureRepo(t *testing.T, cfgBody string) string {
	t.Helper()
	dir := t.TempDir()
	write := func(name, content string) {
		path := filepath.Join(dir, name)
		if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	write(markerGoMod, "module example.com/test\n\ngo 1.21\n")
	write("pkg/b/api/api.go", "package api\n\nfunc Secret() string { return \"s\" }\n")
	write(defaultConfigPath, cfgBody)
	gitInitFixtureRepo(t, dir)
	gitCommitAll(t, dir, "base: pkg/b only")

	write("pkg/a/a.go", "package a\n\nimport \"example.com/test/pkg/b/api\"\n\n"+
		"func UseSecret() string { return api.Secret() }\n")
	gitCommitAll(t, dir, "head: add the cross-module importer")
	return filepath.Join(dir, defaultConfigPath)
}

// gitDeltaOwnerFixtureRepo builds a two-commit Go repo whose CODE stays put and
// whose OWNERSHIP moves: pkg/a → pkg/b exists in both commits, but the base
// commit gives the whole tree one owner while the head commit splits it in two.
// Neither module declares an owner, so each side must resolve its own from its
// own CODEOWNERS — which is exactly what the head pipeline's owner backfill
// would destroy if the base run shared its module map.
func gitDeltaOwnerFixtureRepo(t *testing.T) string {
	t.Helper()
	// min_severity keeps the same-owner form of the edge below the advisory
	// floor; the coupling gate promotes the surviving advisory to a gate task so
	// it reaches agent_tasks[], which is what the origin delta classifies.
	const ownerSplitCfg = `version: 1
modules:
  a:
    paths: ["pkg/a/**"]
  b:
    paths: ["pkg/b/**"]
coupling:
  min_severity: high
  gate:
    min_band: strong
`
	dir := t.TempDir()
	write := func(name, content string) {
		path := filepath.Join(dir, name)
		if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	write(markerGoMod, "module example.com/test\n\ngo 1.21\n")
	write("pkg/b/api/api.go", "package api\n\nfunc Secret() string { return \"s\" }\n")
	write("pkg/a/a.go", "package a\n\nimport \"example.com/test/pkg/b/api\"\n\n"+
		"func UseSecret() string { return api.Secret() }\n")
	write(defaultConfigPath, ownerSplitCfg)
	write(".github/CODEOWNERS", "* @team-one\n")
	gitInitFixtureRepo(t, dir)
	gitCommitAll(t, dir, "base: one owner for the whole tree")

	write(".github/CODEOWNERS", "/pkg/a/ @team-a\n/pkg/b/ @team-b\n")
	gitCommitAll(t, dir, "head: split ownership in two")
	return filepath.Join(dir, defaultConfigPath)
}

// gitDeltaJSON is the minimal decoder for the block under test.
type gitDeltaJSON struct {
	GitFindingDelta *struct {
		BaseRef           string   `json:"base_ref"`
		ComparisonStatus  string   `json:"comparison_status"`
		Introduced        []string `json:"introduced_finding_ids"`
		PreExisting       []string `json:"pre_existing_finding_ids"`
		UnknownOrigin     []string `json:"unknown_origin_finding_ids"`
		ComparisonReasons []string `json:"comparison_reasons"`
	} `json:"git_finding_delta"`
	AgentTasks []struct {
		FindingID string `json:"finding_id"`
		RuleID    string `json:"rule_id"`
	} `json:"agent_tasks"`
}

func testGitDeltaCheckBaseJSON(t *testing.T) {
	t.Parallel()
	const warnRule = `rules:
  - id: no-a-to-b
    type: forbidden_dependency
    gate: warn
    from: "pkg/a/**"
    to: "pkg/b/**"
`
	const failRule = `rules:
  - id: no-a-to-b
    type: forbidden_dependency
    gate: fail
    from: "pkg/a/**"
    to: "pkg/b/**"
`
	tests := []struct {
		name string
		// cfgBody selects the gate outcome; wantCode is the exit code the run
		// must produce with AND without --base (the delta is report-only).
		cfgBody string
		// wantIntroduced is the number of current repair tasks the base ref does
		// not carry. The head commit adds the only violating import, so a
		// blocking rule must attribute its task to this change.
		wantIntroduced int
		// wantUnknownGate requires the synthetic coupling-gate task and the
		// resulting "unknown" comparison status.
		wantUnknownGate bool
		// extraArgs are appended to BOTH the --base run and the plain run, so
		// the report-only assertion still compares like with like.
		extraArgs []string
		wantCode  int
	}{
		{name: "clean gate exits 0", cfgBody: coupledModulesCfg, wantCode: 0},
		{name: "blocking rule exits 1", cfgBody: coupledModulesCfg + failRule, wantIntroduced: 1, wantCode: 1},
		{name: "warning rule exits 2", cfgBody: coupledModulesCfg + warnRule, wantCode: 2},
		// With advisories off there is no BC advisory to promote, so a tripped
		// coupling gate emits the synthetic bc/coupling_gate task. That task is
		// per-run trip state with no stable base counterpart, so it is placed as
		// unknown before ID matching — the production path that reaches
		// comparison_status "unknown".
		{
			name:            "a synthetic coupling-gate task is unknown origin",
			cfgBody:         coupledModulesCfg + "coupling:\n  gate:\n    min_band: strong\n",
			extraArgs:       []string{flagNoAdvisories},
			wantUnknownGate: true,
			wantCode:        1,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			cfgPath := gitDeltaFixtureRepo(t, tc.cfgBody)
			baseArgs := append([]string{cmdCheck, flagBase, diffBaseRef, "--json", "-c", cfgPath}, tc.extraArgs...)
			code, stdout, stderr := runArchfit(t, baseArgs...)
			if code != tc.wantCode {
				t.Fatalf("check --base --json: exit = %d, want %d\nstdout:\n%s\nstderr:\n%s", code, tc.wantCode, stdout, stderr)
			}
			// Report-only: the same run without --base reaches the same verdict.
			plainArgs := append([]string{cmdCheck, "--json", "-c", cfgPath}, tc.extraArgs...)
			if plain, _, _ := runArchfit(t, plainArgs...); plain != code {
				t.Errorf("--base changed the exit code: %d with, %d without", code, plain)
			}
			var got gitDeltaJSON
			if err := json.Unmarshal([]byte(stdout), &got); err != nil {
				t.Fatalf("invalid JSON: %v\n%s", err, stdout)
			}
			if got.GitFindingDelta == nil {
				t.Fatalf("--base --json must emit git_finding_delta\n%s", stdout)
			}
			d := got.GitFindingDelta
			if d.BaseRef != diffBaseRef {
				t.Errorf("base_ref = %q, want %q", d.BaseRef, diffBaseRef)
			}
			if d.Introduced == nil || d.PreExisting == nil || d.UnknownOrigin == nil || d.ComparisonReasons == nil {
				t.Errorf("every git_finding_delta list must be a non-null array: %s", stdout)
			}
			if len(d.ComparisonReasons) != 0 {
				t.Errorf("both fixture sides compile; want no comparison_reasons, got %v", d.ComparisonReasons)
			}
			if len(d.Introduced) != tc.wantIntroduced {
				t.Errorf("introduced_finding_ids = %v, want %d entr(y|ies)", d.Introduced, tc.wantIntroduced)
			}
			if tc.wantUnknownGate {
				synthetic := ""
				for _, task := range got.AgentTasks {
					if task.RuleID == ruleIDBCCouplingGate {
						synthetic = task.FindingID
					}
				}
				if synthetic == "" {
					t.Fatalf("fixture regression: the coupling gate did not trip, so no synthetic task exists: %s", stdout)
				}
				if !slices.Contains(d.UnknownOrigin, synthetic) {
					t.Errorf("the synthetic coupling-gate task must land in unknown_origin_finding_ids: %+v", d)
				}
			}
			// Every current repair task lands in exactly one origin bucket.
			total := len(d.Introduced) + len(d.PreExisting) + len(d.UnknownOrigin)
			if total != len(got.AgentTasks) {
				t.Errorf("origin buckets hold %d ids for %d agent_tasks", total, len(got.AgentTasks))
			}
			if len(d.UnknownOrigin) == 0 && d.ComparisonStatus != diagnostic.GitComparisonComparable {
				t.Errorf("comparison_status = %q with no unknown-origin task", d.ComparisonStatus)
			}
			if len(d.UnknownOrigin) > 0 && d.ComparisonStatus != diagnostic.GitComparisonUnknown {
				t.Errorf("comparison_status = %q with %d unknown-origin tasks", d.ComparisonStatus, len(d.UnknownOrigin))
			}
			// Isolation: the base worktree is deleted before output is read, so
			// no base-side path may appear anywhere in the head report.
			if strings.Contains(stdout, baseWorktreesDir(filepath.Dir(cfgPath))) {
				t.Errorf("head output leaked a base-worktree path: %s", stdout)
			}
		})
	}
}
