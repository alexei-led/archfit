package decision

import (
	"encoding/json"
	"slices"
	"strings"
	"testing"

	"github.com/alexei-led/archfit/internal/assessment/finding"
	"github.com/alexei-led/archfit/internal/assessment/result"
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
	t.Run("base_finding_ids", testGitDeltaBaseFindingIDs)
	t.Run("cross_path_agreement", testGitDeltaCrossPathAgreement)
	t.Run("unpaired_reason", testGitDeltaUnpairedReason)
}

// testGitDeltaUnpairedReason pins the wording of the one output that explains
// why a delta could not be attributed. When the asymmetry that blocked pairing
// lives BELOW the raw coverage status, printing the status twice states two
// identical facts as the reason they could not be compared.
func testGitDeltaUnpairedReason(t *testing.T) {
	t.Parallel()
	goFam := AnalyzerFamily{name: toolGoPackages, primary: true}
	goGap := []result.CoverageGap{{Tool: toolGoPackages}}
	absent := []result.Coverage{covRow(toolGoPackages, result.StatusAbsent)}

	tests := []struct {
		name             string
		head, base       []result.Coverage
		headGap, baseGap []result.CoverageGap
		want             string
	}{
		{
			// head has the project markers (analyzer expected, missing); base does
			// not (language simply absent). Both rows read "absent".
			name: "equal raw statuses name the discriminator",
			head: absent, base: absent, headGap: goGap,
			want: "go/packages: head absent (analyzer expected, did not run), base absent (language not present)",
		},
		{
			// Equal statuses AND equal meanings still explain nothing on their own:
			// what the reader needs is why symmetry did not rescue the comparison.
			name: "equal statuses explain themselves even when both mean the same",
			head: []result.Coverage{covRow(toolGoPackages, result.StatusTimedOut)},
			base: []result.Coverage{covRow(toolGoPackages, result.StatusTimedOut)},
			want: "go/packages: head timed out (run did not finish), base timed out (run did not finish)",
		},
		{
			name: "duplicate rows on both sides say so",
			head: []result.Coverage{covRow(toolGoPackages, result.StatusOK), covRow(toolGoPackages, result.StatusOK)},
			base: []result.Coverage{covRow(toolGoPackages, result.StatusOK), covRow(toolGoPackages, result.StatusOK)},
			want: "go/packages: head ok+ok (duplicate coverage rows), base ok+ok (duplicate coverage rows)",
		},
		{
			name: "a missing row on both sides says so",
			want: "go/packages: head missing (no coverage row), base missing (no coverage row)",
		},
		{
			// Different raw statuses already carry the information; do not clutter.
			name: "differing raw statuses stay terse",
			head: []result.Coverage{covRow(toolGoPackages, result.StatusOK)}, base: absent, baseGap: goGap,
			want: "go/packages: head ok, base absent",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			ok, reasons := CompareAnalyzerEvidence([]AnalyzerFamily{goFam},
				AnalyzerEvidence{Coverage: tc.head, Gaps: tc.headGap},
				AnalyzerEvidence{Coverage: tc.base, Gaps: tc.baseGap})
			if ok {
				t.Fatalf("fixture regression: this shape must not pair (reasons %v)", reasons)
			}
			if len(reasons) != 1 || reasons[0] != tc.want {
				t.Fatalf("reason = %v, want [%q]", reasons, tc.want)
			}
			// The general invariant behind every row: two matching raw statuses
			// never explain why the comparison failed, so a reason that repeats one
			// must add what each side MEANT.
			if headRaw, baseRaw, _ := strings.Cut(reasons[0], ", base "); headRaw == "go/packages: head "+baseRaw &&
				!strings.Contains(reasons[0], "(") {
				t.Errorf("the reason states the same status twice and explains neither: %q", reasons[0])
			}
		})
	}
}

// gitGrade projects `--base`'s (comparable, reasons) result onto the SAME
// three-valued grade `config compare` reports, so the two paths are compared as
// grades rather than as booleans.
//
// The projection is the point of the guard. `--base` has no single grade field:
// its middle state — comparable, but with the degradation named in
// comparison_reasons — lives in the reasons slice. Reading only the bool
// collapses `comparable` and `comparable_with_gaps` into one bucket, and that
// boundary IS the silent-versus-disclosed boundary the whole design rests on.
func gitGrade(comparable bool, reasons []string) CoverageComparability {
	switch {
	case !comparable:
		return CoverageNotComparable
	case len(reasons) == 0:
		return CoverageComparable
	default:
		return CoverageComparableWithGaps
	}
}

// testGitDeltaCrossPathAgreement drives ONE table of coverage shapes through
// BOTH comparison paths — pairFamily here and gradeTool behind
// `config compare` — and asserts they reach the same three-valued grade.
//
// Four review rounds found the same defect: the two paths graded one input shape
// oppositely (symmetric partial-with-unresolved, then symmetric absent-with-a-
// gap, then symmetric absent-WITHOUT-a-gap). The third slipped through the
// guard's own predecessor, which discarded the reasons and compared
// `Status != not_comparable` — so the row asserting agreement passed green while
// one path paired silently and the other disclosed. This version compares the
// full grade, requires a reason exactly when a grade is not `comparable`, and
// asserts documented divergences POSITIVELY, so a row whose comment claims a
// divergence fails once the paths converge.
func testGitDeltaCrossPathAgreement(t *testing.T) {
	t.Parallel()
	partial := func(tool string, unresolved int) result.Coverage {
		c := covRow(tool, result.StatusPartial)
		c.Unresolved = unresolved
		return c
	}
	// go/packages splits Unresolved by what the incompleteness cost: packages
	// missing from the graph (a finding can hide behind them) versus packages
	// that merely failed to type-check (every import is still there).
	goPartial := func(missing, precision int) result.Coverage {
		c := partial(toolGoPackages, missing+precision)
		c.UnresolvedInputsMissing = missing
		c.UnresolvedPrecisionOnly = precision
		return c
	}
	gapFor := func(tool string) []result.CoverageGap { return []result.CoverageGap{{Tool: tool}} }
	goFamily := AnalyzerFamily{name: toolGoPackages, primary: true}
	rows := func(cs ...result.Coverage) []result.Coverage { return cs }

	const (
		comparable = CoverageComparable
		withGaps   = CoverageComparableWithGaps
		notCompare = CoverageNotComparable
	)

	tests := []struct {
		name       string
		fam        AnalyzerFamily
		head, base []result.Coverage
		headGap    []result.CoverageGap
		baseGap    []result.CoverageGap
		// want is the grade BOTH paths must reach, unless wantDecision is set.
		want CoverageComparability
		// wantDecision, when set, is the grade `config compare` reaches instead —
		// a divergence the two paths are SPECIFIED to have. divergent says why.
		wantDecision CoverageComparability
		divergent    string
	}{
		{name: "ok both sides", fam: scipFamily, head: rows(covRow(toolScip, result.StatusOK)), base: rows(covRow(toolScip, result.StatusOK)), want: comparable},
		// Every family reaching pairFamily was ACTIVATED by the effective config,
		// so its absence is shared blindness that must be disclosed — whether or
		// not it is in the install-hint table that emits CoverageGaps. scip is not
		// in that table, which is how this row used to pair silently on one path
		// and grade comparable_with_gaps on the other.
		{name: "absent both sides, no gap", fam: scipFamily, head: rows(covRow(toolScip, result.StatusAbsent)), base: rows(covRow(toolScip, result.StatusAbsent)), want: withGaps},
		{name: "absent both sides with a gap", fam: scipFamily, head: rows(covRow(toolScip, result.StatusAbsent)), base: rows(covRow(toolScip, result.StatusAbsent)), headGap: gapFor(toolScip), baseGap: gapFor(toolScip), want: withGaps},
		// Gap presence is not evidence for a NON-primary family, so a gap on one
		// side only does not make the two sides unequally blind: both are absent.
		{name: "absent gapped on one side only", fam: scipFamily, head: rows(covRow(toolScip, result.StatusAbsent)), base: rows(covRow(toolScip, result.StatusAbsent)), headGap: gapFor(toolScip), want: withGaps},
		{name: "primary absent both sides with a gap", fam: goFamily, head: rows(covRow(toolGoPackages, result.StatusAbsent)), base: rows(covRow(toolGoPackages, result.StatusAbsent)), headGap: gapFor(toolGoPackages), baseGap: gapFor(toolGoPackages), want: withGaps},
		// For a PRIMARY family a missing gap IS evidence — the language's project
		// markers are absent — so gapped against gapless is a real asymmetry.
		{name: "primary absent gapped on one side only", fam: goFamily, head: rows(covRow(toolGoPackages, result.StatusAbsent)), base: rows(covRow(toolGoPackages, result.StatusAbsent)), headGap: gapFor(toolGoPackages), want: notCompare},
		// Both sides not_applicable: the language is in neither tree. gradeTool
		// drops the analyzer from the comparison entirely (ignored), which must
		// leave the overall grade at comparable with no detail.
		{name: "primary absent both sides, no gap", fam: goFamily, head: rows(covRow(toolGoPackages, result.StatusAbsent)), base: rows(covRow(toolGoPackages, result.StatusAbsent)), want: comparable},
		{name: "absent against ok", fam: scipFamily, head: rows(covRow(toolScip, result.StatusAbsent)), base: rows(covRow(toolScip, result.StatusOK)), want: notCompare},
		{
			name: "disabled both sides", fam: scipFamily,
			head: rows(covRow(toolScip, result.StatusDisabled)), base: rows(covRow(toolScip, result.StatusDisabled)),
			// --base measures ONE config against two trees: an analyzer that config
			// turned off is chosen scope, not blindness the run imposed, and it
			// produced no finding on either side that the other could hide. `config
			// compare` weighs TWO configs and reports the measurement neither of
			// them buys. Same input, different question, different grade.
			want: comparable, wantDecision: withGaps,
			divergent: "a deliberate opt-out is scope for --base and lost measurement for config compare",
		},
		{name: "timed out both sides", fam: scipFamily, head: rows(covRow(toolScip, result.StatusTimedOut)), base: rows(covRow(toolScip, result.StatusTimedOut)), want: notCompare},
		{name: "duplicate rows both sides", fam: scipFamily, head: rows(covRow(toolScip, result.StatusOK), covRow(toolScip, result.StatusOK)), base: rows(covRow(toolScip, result.StatusOK), covRow(toolScip, result.StatusOK)), want: notCompare},
		{name: "specifier partial both sides", fam: AnalyzerFamily{name: toolDepCruiser}, head: rows(partial(toolDepCruiser, 4)), base: rows(partial(toolDepCruiser, 9)), want: withGaps},
		{name: "specifier partial against ok", fam: AnalyzerFamily{name: toolDepCruiser}, head: rows(partial(toolDepCruiser, 4)), base: rows(covRow(toolDepCruiser, result.StatusOK)), want: notCompare},
		{name: "partial with no unresolved count both sides", fam: AnalyzerFamily{name: toolDepCruiser}, head: rows(covRow(toolDepCruiser, result.StatusPartial)), base: rows(covRow(toolDepCruiser, result.StatusPartial)), want: notCompare},
		// go/packages counts SKIPPED PACKAGES in Unresolved, so its partial is a
		// run that did not finish — both paths must refuse it.
		{name: "go/packages skipped-package partial both sides", fam: goFamily, head: rows(partial(toolGoPackages, 3)), base: rows(partial(toolGoPackages, 3)), want: notCompare},
		// A partial earned only by packages that did not TYPE-CHECK is the other
		// half of that counter: every import reached both graphs, so neither side
		// can hide a finding from the other and both paths pair it, degraded. One
		// such package anywhere used to make --base inert on an ordinary Go repo.
		{name: "go/packages degraded-precision partial both sides", fam: goFamily, head: rows(goPartial(0, 1)), base: rows(goPartial(0, 12)), want: withGaps},
		// The precision that degraded is what strength-derived findings rest on,
		// so the shape pairs with itself and nothing else.
		{name: "go/packages degraded-precision partial against ok", fam: goFamily, head: rows(goPartial(0, 1)), base: rows(covRow(toolGoPackages, result.StatusOK)), want: notCompare},
		{name: "go/packages degraded-precision against a missing-input partial", fam: goFamily, head: rows(goPartial(0, 1)), base: rows(goPartial(2, 1)), want: notCompare},
		{
			name: "ok against a gapless-absent primary", fam: goFamily,
			head: rows(covRow(toolGoPackages, result.StatusOK)),
			base: rows(covRow(toolGoPackages, result.StatusAbsent)),
			// --base compares two TREES, so a language appearing between them is
			// expected; `config compare` compares ONE tree, so the same status move
			// can only have been caused by the configuration.
			want: comparable, wantDecision: notCompare,
			divergent: "--base compares two trees, config compare compares one",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			comparableOK, reasons := CompareAnalyzerEvidence([]AnalyzerFamily{tc.fam},
				AnalyzerEvidence{Coverage: tc.head, Gaps: tc.headGap},
				AnalyzerEvidence{Coverage: tc.base, Gaps: tc.baseGap})
			gotGit := gitGrade(comparableOK, reasons)
			if gotGit != tc.want {
				t.Fatalf("--base grade = %q, want %q (reasons %v)", gotGit, tc.want, reasons)
			}
			// A grade below `comparable` must SAY so, naming the family. Silence is
			// the defect this guard exists to catch, and gitGrade cannot detect it
			// on its own — it reads the reasons to derive the grade.
			assertGradeDisclosed(t, "--base", gotGit, len(reasons) > 0)
			for _, r := range reasons {
				if !strings.Contains(r, tc.fam.name) {
					t.Errorf("comparison reason does not name its family %q: %q", tc.fam.name, r)
				}
			}

			var primary []string
			if tc.fam.primary {
				primary = []string{tc.fam.name}
			}
			cmp := CompareConfigs(ConfigCompareInput{
				Current: ConfigCompareSide{Diag: result.Diagnostic{
					ToolCoverage: tc.head, CoverageGaps: tc.headGap, PrimaryExtractorTools: primary,
				}},
				Candidate: ConfigCompareSide{Diag: result.Diagnostic{
					ToolCoverage: tc.base, CoverageGaps: tc.baseGap, PrimaryExtractorTools: primary,
				}},
			})
			gotDecision := cmp.Coverage.Status
			assertGradeDisclosed(t, "config compare", gotDecision, len(cmp.Coverage.Details) > 0)

			wantDecision := tc.want
			if tc.wantDecision != "" {
				wantDecision = tc.wantDecision
			}
			if gotDecision != wantDecision {
				t.Fatalf("config compare grade = %q, want %q", gotDecision, wantDecision)
			}
			// Divergences are asserted in BOTH directions: an undocumented one
			// fails, and a documented one that has since converged fails too, so a
			// row's comment cannot quietly become a lie.
			switch {
			case tc.divergent == "" && gotGit != gotDecision:
				t.Fatalf("the two comparison paths disagree on an undocumented shape: --base=%q, config compare=%q",
					gotGit, gotDecision)
			case tc.divergent != "" && gotGit == gotDecision:
				t.Fatalf("row is marked divergent (%s) but both paths now grade %q — delete the divergence",
					tc.divergent, gotGit)
			}
		})
	}
}

// assertGradeDisclosed pins the invariant both paths share: any grade other than
// `comparable` carries the evidence for it — a comparison reason on the --base
// side, a CoverageDetail on the `config compare` side — and `comparable` carries
// none. A degradation nobody can read is the same defect as no degradation.
func assertGradeDisclosed(t *testing.T, path string, grade CoverageComparability, disclosed bool) {
	t.Helper()
	if want := grade != CoverageComparable; disclosed != want {
		t.Errorf("%s: grade %q disclosed = %v, want %v", path, grade, disclosed, want)
	}
}

// scipFamily is the non-primary analyzer family the cross-path table compares on.
var scipFamily = AnalyzerFamily{name: toolScip}

// gitDeltaRef is the base ref label used by the pure-comparison subtests.
const gitDeltaRef = "main"

// toolGoPackages is the Go primary analyzer's coverage name, restated locally:
// assessment compares coverage rows by name and never imports the extractors.
const toolGoPackages = "go/packages"

// couplingGateFindingID mirrors evaluation's synthetic coupling-gate finding ID.
// The origin comparison must place it as unknown-origin without depending on
// the evaluator that emits it.
const couplingGateFindingID = "coupling-gate"

func covRow(tool, status string) result.Coverage {
	return result.Coverage{Tool: tool, Status: status}
}

func agentTask(findingID, ruleID string) result.AgentTask {
	return result.AgentTask{FindingID: findingID, RuleID: ruleID}
}

// goPrimaryFamily is the single-family fixture used by the origin table: the
// pairing rules themselves are covered by testGitDeltaAnalyzerEvidence.
var goPrimaryFamily = []AnalyzerFamily{{name: toolGoPackages, primary: true}}

func testGitDeltaOrigin(t *testing.T) {
	t.Parallel()
	const hash = "cfg-hash"
	comparableSide := AnalyzerEvidence{Coverage: []result.Coverage{covRow(toolGoPackages, result.StatusOK)}, Hash: hash}
	partialSide := AnalyzerEvidence{Coverage: []result.Coverage{covRow(toolGoPackages, result.StatusPartial)}, Hash: hash}

	tests := []struct {
		name            string
		tasks           []result.AgentTask
		baseIDs         []string
		base            AnalyzerEvidence
		wantIntroduced  []string
		wantPreExisting []string
		wantUnknown     []string
		wantStatus      string
	}{
		{
			name:            "exact base match is pre-existing",
			tasks:           []result.AgentTask{agentTask("f1", "arch/forbidden")},
			baseIDs:         []string{"f1"},
			base:            comparableSide,
			wantPreExisting: []string{"f1"},
			wantStatus:      result.GitComparisonComparable,
		},
		{
			name:           "unmatched task with comparable evidence is introduced",
			tasks:          []result.AgentTask{agentTask("f2", "arch/forbidden")},
			baseIDs:        []string{"f1"},
			base:           comparableSide,
			wantIntroduced: []string{"f2"},
			wantStatus:     result.GitComparisonComparable,
		},
		{
			// A base entry the base run reported as fixed is dropped by
			// BaseFindingIDs, so the same ID on head is genuinely new work.
			name:           "base fixed entry does not make a task pre-existing",
			tasks:          []result.AgentTask{agentTask("f1", "arch/forbidden")},
			baseIDs:        nil,
			base:           comparableSide,
			wantIntroduced: []string{"f1"},
			wantStatus:     result.GitComparisonComparable,
		},
		{
			name:        "unavailable analyzer evidence makes an unmatched task unknown",
			tasks:       []result.AgentTask{agentTask("f2", "arch/forbidden")},
			baseIDs:     []string{"f1"},
			base:        partialSide,
			wantUnknown: []string{"f2"},
			wantStatus:  result.GitComparisonUnknown,
		},
		{
			// Incomplete evidence never downgrades an exact ID match.
			name:            "exact match survives unavailable evidence",
			tasks:           []result.AgentTask{agentTask("f1", "arch/forbidden")},
			baseIDs:         []string{"f1"},
			base:            partialSide,
			wantPreExisting: []string{"f1"},
			wantStatus:      result.GitComparisonComparable,
		},
		{
			name:        "synthetic coupling-gate task is unknown before ID matching",
			tasks:       []result.AgentTask{agentTask(couplingGateFindingID, finding.RuleIDCouplingGate)},
			baseIDs:     []string{couplingGateFindingID},
			base:        comparableSide,
			wantUnknown: []string{couplingGateFindingID},
			wantStatus:  result.GitComparisonUnknown,
		},
		{
			name:    "config hash mismatch makes every unmatched task unknown",
			tasks:   []result.AgentTask{agentTask("f1", "arch/forbidden"), agentTask("f2", "arch/forbidden")},
			baseIDs: []string{"f1"},
			base: AnalyzerEvidence{
				Coverage: []result.Coverage{covRow(toolGoPackages, result.StatusOK)},
				Hash:     "other-hash",
			},
			wantPreExisting: []string{"f1"},
			wantUnknown:     []string{"f2"},
			wantStatus:      result.GitComparisonUnknown,
		},
		{
			name:            "lists use a stable sorted order",
			tasks:           []result.AgentTask{agentTask("z", "r"), agentTask("a", "r"), agentTask("m", "r"), agentTask("b", "r")},
			baseIDs:         []string{"m", "b"},
			base:            comparableSide,
			wantIntroduced:  []string{"a", "z"},
			wantPreExisting: []string{"b", "m"},
			wantStatus:      result.GitComparisonComparable,
		},
		{
			name:       "clean run still emits the block with empty lists",
			base:       comparableSide,
			wantStatus: result.GitComparisonComparable,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got := BuildGitFindingDelta(GitDeltaInput{
				BaseRef:        gitDeltaRef,
				Tasks:          tc.tasks,
				BaseFindingIDs: tc.baseIDs,
				Head: AnalyzerEvidence{
					Coverage: []result.Coverage{covRow(toolGoPackages, result.StatusOK)},
					Hash:     hash,
				},
				Base:     tc.base,
				Families: goPrimaryFamily,
			})
			if got == nil {
				t.Fatal("BuildGitFindingDelta returned nil; the block must always be present with --base")
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
		unresolvedSide := func(n int) AnalyzerEvidence {
			row := covRow(toolDepCruiser, result.StatusPartial)
			row.Unresolved = n
			return AnalyzerEvidence{Coverage: []result.Coverage{row}, Hash: hash}
		}
		got := BuildGitFindingDelta(GitDeltaInput{
			BaseRef:        gitDeltaRef,
			Tasks:          []result.AgentTask{agentTask("f2", "arch/forbidden")},
			BaseFindingIDs: []string{"f1"},
			Head:           unresolvedSide(4),
			Base:           unresolvedSide(6),
			Families:       []AnalyzerFamily{{name: toolDepCruiser, primary: true}},
		})
		assertIDs(t, "introduced_finding_ids", got.IntroducedFindingIDs, []string{"f2"})
		assertIDs(t, "unknown_origin_finding_ids", got.UnknownOriginFindingIDs, nil)
		if got.ComparisonStatus != result.GitComparisonComparable {
			t.Errorf("comparison_status = %q, want %q", got.ComparisonStatus, result.GitComparisonComparable)
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

func assertNonNullJSONArrays(t *testing.T, d *result.GitFindingDelta) {
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
	goFam := AnalyzerFamily{name: toolGoPackages, primary: true}
	dcFam := AnalyzerFamily{name: toolDepCruiser, primary: true}
	scipFam := AnalyzerFamily{name: toolScip}
	astFam := AnalyzerFamily{name: toolAstGrep}
	goGap := []result.CoverageGap{{Tool: toolGoPackages}}
	scipGap := []result.CoverageGap{{Tool: toolScip}}

	// unresolvedRow is the dependency-cruiser/grimp steady state: the analyzer
	// walked the whole tree and could not resolve n import specifiers.
	unresolvedRow := func(n int) result.Coverage {
		c := covRow(toolDepCruiser, result.StatusPartial)
		c.Unresolved = n
		return c
	}
	// goUnresolvedRow is NOT that shape: go/packages counts whole packages it
	// SKIPPED because they failed to load, so its partial means the run did not
	// finish over the tree.
	goUnresolvedRow := func(n int) result.Coverage {
		c := covRow(toolGoPackages, result.StatusPartial)
		c.Unresolved = n
		return c
	}

	tests := []struct {
		name           string
		fam            AnalyzerFamily
		head, base     []result.Coverage
		headGap, bsGap []result.CoverageGap
		want           bool
		// degraded marks a pair that IS comparable but must still disclose one
		// reason (symmetric unresolved-specifier partial).
		degraded bool
	}{
		{name: "ok/ok", fam: goFam, head: []result.Coverage{covRow(toolGoPackages, result.StatusOK)}, base: []result.Coverage{covRow(toolGoPackages, result.StatusOK)}, want: true},
		{name: "ok/not_applicable", fam: goFam, head: []result.Coverage{covRow(toolGoPackages, result.StatusOK)}, base: []result.Coverage{covRow(toolGoPackages, result.StatusAbsent)}, want: true},
		{name: "not_applicable/ok", fam: goFam, head: []result.Coverage{covRow(toolGoPackages, result.StatusAbsent)}, base: []result.Coverage{covRow(toolGoPackages, result.StatusOK)}, want: true},
		{name: "not_applicable both sides is ignored", fam: goFam, head: []result.Coverage{covRow(toolGoPackages, result.StatusAbsent)}, base: []result.Coverage{covRow(toolGoPackages, result.StatusAbsent)}, want: true},
		{name: "primary absent with a coverage gap is unavailable", fam: goFam, head: []result.Coverage{covRow(toolGoPackages, result.StatusOK)}, base: []result.Coverage{covRow(toolGoPackages, result.StatusAbsent)}, bsGap: goGap, want: false},
		{name: "partial with no unresolved count is unavailable", fam: goFam, head: []result.Coverage{covRow(toolGoPackages, result.StatusOK)}, base: []result.Coverage{covRow(toolGoPackages, result.StatusPartial)}, want: false},
		// dependency-cruiser and grimp mark a COMPLETED run partial as soon as one
		// import specifier anywhere fails to resolve. Symmetric, it is shared
		// incompleteness over one tree: comparable, but always disclosed.
		{name: "symmetric unresolved partial is comparable and disclosed", fam: dcFam, head: []result.Coverage{unresolvedRow(4)}, base: []result.Coverage{unresolvedRow(9)}, want: true, degraded: true},
		{name: "unresolved partial never pairs with ok", fam: dcFam, head: []result.Coverage{unresolvedRow(4)}, base: []result.Coverage{covRow(toolDepCruiser, result.StatusOK)}, want: false},
		{name: "unresolved partial never pairs with a failed partial", fam: dcFam, head: []result.Coverage{unresolvedRow(4)}, base: []result.Coverage{covRow(toolDepCruiser, result.StatusPartial)}, want: false},
		// go/packages sets Unresolved on a partial too, but there it counts whole
		// packages it SKIPPED because they failed to load — the "did not finish"
		// meaning. A symmetric Go partial must NOT read as shared incompleteness,
		// or a base side with N unloaded packages produces a false "introduced".
		{name: "go/packages skipped-package partial is unavailable on both sides", fam: goFam, head: []result.Coverage{goUnresolvedRow(3)}, base: []result.Coverage{goUnresolvedRow(3)}, want: false},
		{name: "go/packages skipped-package partial never pairs with ok", fam: goFam, head: []result.Coverage{goUnresolvedRow(3)}, base: []result.Coverage{covRow(toolGoPackages, result.StatusOK)}, want: false},
		{name: "timed out is unavailable", fam: goFam, head: []result.Coverage{covRow(toolGoPackages, result.StatusTimedOut)}, base: []result.Coverage{covRow(toolGoPackages, result.StatusOK)}, want: false},
		{name: "missing row on one side is unavailable", fam: goFam, base: []result.Coverage{covRow(toolGoPackages, result.StatusOK)}, want: false},
		{name: "missing row on both sides is unavailable", fam: goFam, want: false},
		{name: "duplicate row on one side is unavailable", fam: goFam, head: []result.Coverage{covRow(toolGoPackages, result.StatusOK), covRow(toolGoPackages, result.StatusOK)}, base: []result.Coverage{covRow(toolGoPackages, result.StatusOK)}, want: false},
		// Every analyzer owns its own coverage name, so a repeated name is an
		// anomaly on BOTH sides too — there is no way to know which duplicate
		// pairs with which. Same rule as gradeTool.
		{name: "matching duplicate rows are still unavailable", fam: astFam, head: []result.Coverage{covRow(toolAstGrep, result.StatusOK), covRow(toolAstGrep, result.StatusOK)}, base: []result.Coverage{covRow(toolAstGrep, result.StatusOK), covRow(toolAstGrep, result.StatusOK)}, want: false},
		{name: "the pattern pass ignores the syntax pass's own row", fam: astFam, head: []result.Coverage{covRow(toolAstGrep, result.StatusOK), covRow(toolAstGrepSyntax, result.StatusDisabled)}, base: []result.Coverage{covRow(toolAstGrep, result.StatusOK), covRow(toolAstGrepSyntax, result.StatusOK)}, want: true},
		{name: "the syntax pass compares on its own row", fam: AnalyzerFamily{name: toolAstGrepSyntax}, head: []result.Coverage{covRow(toolAstGrep, result.StatusOK), covRow(toolAstGrepSyntax, result.StatusDisabled)}, base: []result.Coverage{covRow(toolAstGrep, result.StatusOK), covRow(toolAstGrepSyntax, result.StatusOK)}, want: false},
		{name: "disabled on both sides is ignored", fam: scipFam, head: []result.Coverage{covRow(toolScip, result.StatusDisabled)}, base: []result.Coverage{covRow(toolScip, result.StatusDisabled)}, want: true},
		{name: "disabled on one side only is unavailable", fam: scipFam, head: []result.Coverage{covRow(toolScip, result.StatusOK)}, base: []result.Coverage{covRow(toolScip, result.StatusDisabled)}, want: false},
		// A non-primary analyzer's absence is evidence about the TOOL, not the
		// tree: asymmetric absence could hide a base finding, symmetric absence
		// means neither side produced one.
		{name: "non-primary absent on one side only is unavailable", fam: scipFam, head: []result.Coverage{covRow(toolScip, result.StatusOK)}, base: []result.Coverage{covRow(toolScip, result.StatusAbsent)}, want: false},
		// CHANGED from degraded:false — this row pinned the defect. A CoverageGap
		// is only emitted for tools in the install-hint table, and scip is not in
		// it, so this shape (the live one on archfit's own config wherever no SCIP
		// indexer is installed) paired SILENTLY while gradeTool graded the
		// identical row comparable_with_gaps and emitted a detail. Every family
		// compared here was activated by the effective config, so its absence is
		// always shared blindness that must be disclosed.
		{name: "non-primary absent on both sides is comparable and disclosed", fam: scipFam, head: []result.Coverage{covRow(toolScip, result.StatusAbsent)}, base: []result.Coverage{covRow(toolScip, result.StatusAbsent)}, want: true, degraded: true},
		// CHANGED from want:false. An enabled analyzer whose tool is missing on
		// the host reports absent WITH a gap on BOTH sides. Symmetric, that is the
		// same safety argument as gapless symmetric absence — neither side ran it,
		// so neither has a finding the other hides — and gradeTool
		// already grades it comparable_with_gaps. Failing it made --base
		// permanently all-unknown wherever an enabled analyzer is uninstalled,
		// including archfit's own runtime image on any repo with a Cargo.toml.
		// It pairs DEGRADED: the shared blindness is always disclosed.
		{name: "absent with a coverage gap on both sides is comparable and disclosed", fam: scipFam, head: []result.Coverage{covRow(toolScip, result.StatusAbsent)}, base: []result.Coverage{covRow(toolScip, result.StatusAbsent)}, headGap: scipGap, bsGap: scipGap, want: true, degraded: true},
		{name: "primary absent with a coverage gap on both sides is comparable and disclosed", fam: goFam, head: []result.Coverage{covRow(toolGoPackages, result.StatusAbsent)}, base: []result.Coverage{covRow(toolGoPackages, result.StatusAbsent)}, headGap: goGap, bsGap: goGap, want: true, degraded: true},
		// CHANGED from want:false. Gap presence discriminates for PRIMARY families
		// only, where a missing gap proves the language is absent from that tree.
		// For a non-primary family both sides are plainly absent and equally blind
		// however the install-hint table happened to classify them, so they pair —
		// degraded, never silently. The gap-asymmetry rule still holds where it
		// means something: see the primary rows below.
		{name: "non-primary absent gapped on head only is comparable and disclosed", fam: scipFam, head: []result.Coverage{covRow(toolScip, result.StatusAbsent)}, base: []result.Coverage{covRow(toolScip, result.StatusAbsent)}, headGap: scipGap, want: true, degraded: true},
		{name: "non-primary absent gapped on base only is comparable and disclosed", fam: scipFam, head: []result.Coverage{covRow(toolScip, result.StatusAbsent)}, base: []result.Coverage{covRow(toolScip, result.StatusAbsent)}, bsGap: scipGap, want: true, degraded: true},
		// The gap is derived per side from that side's own tree, so a project
		// marker ADDED by the change gaps head and leaves base not_applicable.
		// For a PRIMARY family that asymmetry is real — one side has none of the
		// language, the other has it and could not analyze it — so it never pairs.
		{name: "primary absent gapped on head only is unavailable", fam: goFam, head: []result.Coverage{covRow(toolGoPackages, result.StatusAbsent)}, base: []result.Coverage{covRow(toolGoPackages, result.StatusAbsent)}, headGap: goGap, want: false},
		{name: "primary absent gapped on base only is unavailable", fam: goFam, head: []result.Coverage{covRow(toolGoPackages, result.StatusAbsent)}, base: []result.Coverage{covRow(toolGoPackages, result.StatusAbsent)}, bsGap: goGap, want: false},
		{name: "absent gapped never pairs with ok", fam: scipFam, head: []result.Coverage{covRow(toolScip, result.StatusAbsent)}, base: []result.Coverage{covRow(toolScip, result.StatusOK)}, headGap: scipGap, want: false},
		{name: "absent gapped never pairs with a timeout", fam: scipFam, head: []result.Coverage{covRow(toolScip, result.StatusAbsent)}, base: []result.Coverage{covRow(toolScip, result.StatusTimedOut)}, headGap: scipGap, want: false},
		// A timeout is flaky, not structural: symmetry proves nothing about what
		// either side would have found, so it stays unavailable on both sides.
		{name: "timed out on both sides is still unavailable", fam: scipFam, head: []result.Coverage{covRow(toolScip, result.StatusTimedOut)}, base: []result.Coverage{covRow(toolScip, result.StatusTimedOut)}, want: false},
		{name: "non-primary absent never pairs with ok", fam: scipFam, head: []result.Coverage{covRow(toolScip, result.StatusAbsent)}, base: []result.Coverage{covRow(toolScip, result.StatusOK)}, want: false},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			ok, reasons := CompareAnalyzerEvidence([]AnalyzerFamily{tc.fam},
				AnalyzerEvidence{Coverage: tc.head, Gaps: tc.headGap},
				AnalyzerEvidence{Coverage: tc.base, Gaps: tc.bsGap})
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
		ok, reasons := CompareAnalyzerEvidence([]AnalyzerFamily{dcFam},
			AnalyzerEvidence{Coverage: []result.Coverage{unresolvedRow(4)}},
			AnalyzerEvidence{Coverage: []result.Coverage{heavy}})
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
		fams := []AnalyzerFamily{
			{name: toolScip},
			{name: toolGoPackages, primary: true},
			{name: toolJscpd},
		}
		head := AnalyzerEvidence{Coverage: []result.Coverage{
			covRow(toolScip, result.StatusOK),
			covRow(toolGoPackages, result.StatusOK),
			covRow(toolJscpd, result.StatusOK),
		}}
		base := AnalyzerEvidence{Coverage: []result.Coverage{
			covRow(toolScip, result.StatusPartial),
			covRow(toolGoPackages, result.StatusTimedOut),
			covRow(toolJscpd, result.StatusOK),
		}}
		delta := BuildGitFindingDelta(GitDeltaInput{BaseRef: gitDeltaRef, Head: head, Base: base, Families: fams})
		if len(delta.ComparisonReasons) != 2 {
			t.Fatalf("comparison_reasons = %v, want one per unavailable family", delta.ComparisonReasons)
		}
		if !slices.IsSorted(delta.ComparisonReasons) {
			t.Errorf("comparison_reasons must be sorted: %v", delta.ComparisonReasons)
		}
	})
}
func testGitDeltaBaseFindingIDs(t *testing.T) {
	t.Parallel()
	got := BaseFindingIDs([]finding.Finding{
		{ID: "z", Kind: string(finding.KindAdvisory), Status: finding.StatusNew},
		{ID: "gone", Kind: string(finding.KindGate), Status: finding.StatusFixed},
		{ID: "a", Kind: string(finding.KindGate), Status: finding.StatusWaived},
		{ID: "m", Kind: string(finding.KindAdvisory), Status: finding.StatusExpiredWaiver},
	})
	if want := []string{"a", "m", "z"}; !slices.Equal(got, want) {
		t.Errorf("BaseFindingIDs = %v, want %v (fixed dropped, sorted, kind ignored)", got, want)
	}
}

// testGitDeltaEffectiveConfig covers the base sub-run's config contract: it gets
// the caller's effective config (flag overrides included) through an independent
// module map, so the head pipeline's owner and deploy-unit backfill cannot leak
// head-tree evidence into the base measurement.

// gitDeltaFixtureRepo builds a two-commit Go repo: the base commit holds only
// pkg/b, the head commit adds a pkg/a → pkg/b importer. The head run therefore
// carries a cross-module edge the base ref does not, and BOTH sides compile, so
// go/packages reports ok on both and the evidence is genuinely comparable.

// gitDeltaOwnerFixtureRepo builds a two-commit Go repo whose CODE stays put and
// whose OWNERSHIP moves: pkg/a → pkg/b exists in both commits, but the base
// commit gives the whole tree one owner while the head commit splits it in two.
// Neither module declares an owner, so each side must resolve its own from its
// own CODEOWNERS — which is exactly what the head pipeline's owner backfill
// would destroy if the base run shared its module map.
