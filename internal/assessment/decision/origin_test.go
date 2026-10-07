package decision

import (
	"maps"
	"slices"
	"strings"
	"testing"

	"github.com/alexei-led/archfit/internal/assessment/finding"
	"github.com/alexei-led/archfit/internal/assessment/result"
	"github.com/alexei-led/archfit/internal/model/evidence"
)

// TestOriginClassification covers the `--base` origin classification: how findings are placed
// (introduced / pre-existing / unknown), which analyzer evidence is comparable,
// which families are active for a config, and the end-to-end `check --base
// --json` contract at every gate exit code.
//
// One exported test function by design — cmd/archfit sits at its public_api_max
// ceiling, so new coverage arrives as subtests, never as new exported names.
func TestOriginClassification(t *testing.T) {
	t.Parallel()
	t.Run("origin", testOriginBuckets)
	t.Run("analyzer_evidence", testOriginAnalyzerEvidence)
	t.Run("base_finding_ids", testOriginBaseFindingIDs)
	t.Run("cross_path_agreement", testOriginCrossPathAgreement)
	t.Run("unpaired_reason", testOriginUnpairedReason)
}

// testOriginUnpairedReason pins the wording of the one output that explains
// why a delta could not be attributed. When the asymmetry that blocked pairing
// lives BELOW the raw coverage status, printing the status twice states two
// identical facts as the reason they could not be compared.
func testOriginUnpairedReason(t *testing.T) {
	t.Parallel()
	goFam := AnalyzerFamily{name: toolGoPackages, primary: true}
	goGap := []evidence.CoverageGap{{Tool: toolGoPackages}}
	absent := []evidence.Coverage{covRow(toolGoPackages, evidence.StatusAbsent)}

	tests := []struct {
		name             string
		head, base       []evidence.Coverage
		headGap, baseGap []evidence.CoverageGap
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
			head: []evidence.Coverage{covRow(toolGoPackages, evidence.StatusTimedOut)},
			base: []evidence.Coverage{covRow(toolGoPackages, evidence.StatusTimedOut)},
			want: "go/packages: head timed out (run did not finish), base timed out (run did not finish)",
		},
		{
			name: "duplicate rows on both sides say so",
			head: []evidence.Coverage{covRow(toolGoPackages, evidence.StatusOK), covRow(toolGoPackages, evidence.StatusOK)},
			base: []evidence.Coverage{covRow(toolGoPackages, evidence.StatusOK), covRow(toolGoPackages, evidence.StatusOK)},
			want: "go/packages: head ok+ok (duplicate coverage rows), base ok+ok (duplicate coverage rows)",
		},
		{
			name: "a missing row on both sides says so",
			want: "go/packages: head missing (no coverage row), base missing (no coverage row)",
		},
		{
			// Different raw statuses already carry the information; do not clutter.
			name: "differing raw statuses stay terse",
			head: []evidence.Coverage{covRow(toolGoPackages, evidence.StatusOK)}, base: absent, baseGap: goGap,
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
// origin_reasons — lives in the reasons slice. Reading only the bool
// collapses `comparable` and `comparable_with_gaps` into one bucket, and that
// boundary IS the silent-versus-disclosed boundary the whole design rests on.
func gitGrade(evidenceComparable bool, reasons []string) CoverageComparability {
	switch {
	case !evidenceComparable:
		return CoverageNotComparable
	case len(reasons) == 0:
		return CoverageComparable
	default:
		return CoverageComparableWithGaps
	}
}

// testOriginCrossPathAgreement drives ONE table of coverage shapes through
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
func testOriginCrossPathAgreement(t *testing.T) {
	t.Parallel()
	partial := func(tool string, unresolved int) evidence.Coverage {
		c := covRow(tool, evidence.StatusPartial)
		c.Unresolved = unresolved
		return c
	}
	// go/packages splits Unresolved by what the incompleteness cost: packages
	// missing from the graph (a finding can hide behind them) versus packages
	// that merely failed to type-check (every import is still there).
	goPartial := func(missing, precision int) evidence.Coverage {
		c := partial(toolGoPackages, missing+precision)
		c.UnresolvedInputsMissing = missing
		c.UnresolvedPrecisionOnly = precision
		return c
	}
	gapFor := func(tool string) []evidence.CoverageGap { return []evidence.CoverageGap{{Tool: tool}} }
	goFamily := AnalyzerFamily{name: toolGoPackages, primary: true}
	rows := func(cs ...evidence.Coverage) []evidence.Coverage { return cs }

	const (
		full       = CoverageComparable
		withGaps   = CoverageComparableWithGaps
		notCompare = CoverageNotComparable
	)

	tests := []struct {
		name       string
		fam        AnalyzerFamily
		head, base []evidence.Coverage
		headGap    []evidence.CoverageGap
		baseGap    []evidence.CoverageGap
		// want is the grade BOTH paths must reach, unless wantDecision is set.
		want CoverageComparability
		// wantDecision, when set, is the grade `config compare` reaches instead —
		// a divergence the two paths are SPECIFIED to have. divergent says why.
		wantDecision CoverageComparability
		divergent    string
	}{
		{name: "ok both sides", fam: scipFamily, head: rows(covRow(toolScip, evidence.StatusOK)), base: rows(covRow(toolScip, evidence.StatusOK)), want: full},
		// Every family reaching pairFamily was ACTIVATED by the effective config,
		// so its absence is shared blindness that must be disclosed — whether or
		// not it is in the install-hint table that emits CoverageGaps. scip is not
		// in that table, which is how this row used to pair silently on one path
		// and grade comparable_with_gaps on the other.
		{name: "absent both sides, no gap", fam: scipFamily, head: rows(covRow(toolScip, evidence.StatusAbsent)), base: rows(covRow(toolScip, evidence.StatusAbsent)), want: withGaps},
		{name: "absent both sides with a gap", fam: scipFamily, head: rows(covRow(toolScip, evidence.StatusAbsent)), base: rows(covRow(toolScip, evidence.StatusAbsent)), headGap: gapFor(toolScip), baseGap: gapFor(toolScip), want: withGaps},
		// Gap presence is not evidence for a NON-primary family, so a gap on one
		// side only does not make the two sides unequally blind: both are absent.
		{name: "absent gapped on one side only", fam: scipFamily, head: rows(covRow(toolScip, evidence.StatusAbsent)), base: rows(covRow(toolScip, evidence.StatusAbsent)), headGap: gapFor(toolScip), want: withGaps},
		{name: "primary absent both sides with a gap", fam: goFamily, head: rows(covRow(toolGoPackages, evidence.StatusAbsent)), base: rows(covRow(toolGoPackages, evidence.StatusAbsent)), headGap: gapFor(toolGoPackages), baseGap: gapFor(toolGoPackages), want: withGaps},
		// For a PRIMARY family a missing gap IS evidence — the language's project
		// markers are absent — so gapped against gapless is a real asymmetry.
		{name: "primary absent gapped on one side only", fam: goFamily, head: rows(covRow(toolGoPackages, evidence.StatusAbsent)), base: rows(covRow(toolGoPackages, evidence.StatusAbsent)), headGap: gapFor(toolGoPackages), want: notCompare},
		// Both sides not_applicable: the language is in neither tree. gradeTool
		// drops the analyzer from the comparison entirely (ignored), which must
		// leave the overall grade at comparable with no detail.
		{name: "primary absent both sides, no gap", fam: goFamily, head: rows(covRow(toolGoPackages, evidence.StatusAbsent)), base: rows(covRow(toolGoPackages, evidence.StatusAbsent)), want: full},
		{name: "absent against ok", fam: scipFamily, head: rows(covRow(toolScip, evidence.StatusAbsent)), base: rows(covRow(toolScip, evidence.StatusOK)), want: notCompare},
		{
			name: "disabled both sides", fam: scipFamily,
			head: rows(covRow(toolScip, evidence.StatusDisabled)), base: rows(covRow(toolScip, evidence.StatusDisabled)),
			// --base measures ONE config against two trees: an analyzer that config
			// turned off is chosen scope, not blindness the run imposed, and it
			// produced no finding on either side that the other could hide. `config
			// compare` weighs TWO configs and reports the measurement neither of
			// them buys. Same input, different question, different grade.
			want: full, wantDecision: withGaps,
			divergent: "a deliberate opt-out is scope for --base and lost measurement for config compare",
		},
		{name: "timed out both sides", fam: scipFamily, head: rows(covRow(toolScip, evidence.StatusTimedOut)), base: rows(covRow(toolScip, evidence.StatusTimedOut)), want: notCompare},
		{name: "duplicate rows both sides", fam: scipFamily, head: rows(covRow(toolScip, evidence.StatusOK), covRow(toolScip, evidence.StatusOK)), base: rows(covRow(toolScip, evidence.StatusOK), covRow(toolScip, evidence.StatusOK)), want: notCompare},
		{name: "specifier partial both sides", fam: AnalyzerFamily{name: toolDepCruiser}, head: rows(partial(toolDepCruiser, 4)), base: rows(partial(toolDepCruiser, 9)), want: withGaps},
		{name: "specifier partial against ok", fam: AnalyzerFamily{name: toolDepCruiser}, head: rows(partial(toolDepCruiser, 4)), base: rows(covRow(toolDepCruiser, evidence.StatusOK)), want: notCompare},
		{name: "partial with no unresolved count both sides", fam: AnalyzerFamily{name: toolDepCruiser}, head: rows(covRow(toolDepCruiser, evidence.StatusPartial)), base: rows(covRow(toolDepCruiser, evidence.StatusPartial)), want: notCompare},
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
		{name: "go/packages degraded-precision partial against ok", fam: goFamily, head: rows(goPartial(0, 1)), base: rows(covRow(toolGoPackages, evidence.StatusOK)), want: notCompare},
		{name: "go/packages degraded-precision against a missing-input partial", fam: goFamily, head: rows(goPartial(0, 1)), base: rows(goPartial(2, 1)), want: notCompare},
		{
			name: "ok against a gapless-absent primary", fam: goFamily,
			head: rows(covRow(toolGoPackages, evidence.StatusOK)),
			base: rows(covRow(toolGoPackages, evidence.StatusAbsent)),
			// --base compares two TREES, so a language appearing between them is
			// expected; `config compare` compares ONE tree, so the same status move
			// can only have been caused by the configuration.
			want: full, wantDecision: notCompare,
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
				Current: ConfigCompareSide{Diag: result.Result{
					ToolCoverage: tc.head, CoverageGaps: tc.headGap, PrimaryExtractorTools: primary,
					MeasurementProfile: &evidence.MeasurementProfile{Version: evidence.MeasurementProfileVersion, SettingsHash: "settings", Producers: []evidence.MeasurementProducer{{Tool: "loc", SemanticsVersion: "loc.v1", Status: evidence.StatusOK}}},
				}},
				Candidate: ConfigCompareSide{Diag: result.Result{
					ToolCoverage: tc.base, CoverageGaps: tc.baseGap, PrimaryExtractorTools: primary,
					MeasurementProfile: &evidence.MeasurementProfile{Version: evidence.MeasurementProfileVersion, SettingsHash: "settings", Producers: []evidence.MeasurementProducer{{Tool: "loc", SemanticsVersion: "loc.v1", Status: evidence.StatusOK}}},
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

// toolGoPackages is the Go primary analyzer's coverage name, restated locally:
// assessment compares coverage rows by name and never imports the extractors.
const toolGoPackages = "go/packages"

const ruleForbidden = "arch/forbidden"

func covRow(tool, status string) evidence.Coverage {
	return evidence.Coverage{Tool: tool, Status: status}
}

func gateFinding(id string) finding.Finding {
	return finding.Finding{ID: id, RuleID: ruleForbidden, Kind: finding.KindGate, Status: finding.StatusNew}
}

func rollupFinding(id string, members ...string) finding.Finding {
	return finding.Finding{ID: id, RuleID: finding.RuleIDBCImbalanced, Kind: finding.KindAdvisory, Status: finding.StatusNew, Members: members}
}

// seamGateAB is the ID seamGateFinding gives the a -> b seam.
const seamGateAB = "coupling-gate/a-b"

func seamGateFinding(from, to string) finding.Finding {
	return finding.Finding{ID: "coupling-gate/" + from + "-" + to, RuleID: finding.RuleIDCouplingGate, Kind: finding.KindGate,
		Status: finding.StatusNew, Edge: finding.EdgeEvidence{From: finding.Endpoint{Module: from}, To: finding.Endpoint{Module: to}}}
}

// goPrimaryFamily is the single-family fixture used by the origin table: the
// pairing rules themselves are covered by testOriginAnalyzerEvidence.
var goPrimaryFamily = []AnalyzerFamily{{name: toolGoPackages, primary: true}}

func testOriginBuckets(t *testing.T) {
	t.Parallel()
	okSide := AnalyzerEvidence{Coverage: []evidence.Coverage{covRow(toolGoPackages, evidence.StatusOK)}}
	partialSide := AnalyzerEvidence{Coverage: []evidence.Coverage{covRow(toolGoPackages, evidence.StatusPartial)}}
	fixed := gateFinding("gone")
	fixed.Status = finding.StatusFixed

	tests := []struct {
		name           string
		head           []finding.Finding
		base           []BaseFinding
		baseSeams      []ModulePair
		baseSide       AnalyzerEvidence
		wantOrigins    map[string]finding.Origin
		wantIntroduced []string
		wantResolved   []string
		wantStatus     string
	}{
		{
			name: "exact base match is pre-existing", head: []finding.Finding{gateFinding("f1")},
			base: []BaseFinding{{ID: "f1"}}, baseSide: okSide,
			wantOrigins: map[string]finding.Origin{"f1": finding.OriginPreExisting}, wantStatus: OriginComparable,
		},
		{
			name: "unmatched finding with comparable evidence is introduced", head: []finding.Finding{gateFinding("f2")},
			base: []BaseFinding{{ID: "f1"}}, baseSide: okSide,
			wantOrigins:    map[string]finding.Origin{"f2": finding.OriginIntroduced},
			wantIntroduced: []string{"f2"}, wantResolved: []string{"f1"}, wantStatus: OriginComparable,
		},
		{
			name: "unavailable evidence makes an unmatched finding unknown and claims nothing resolved",
			head: []finding.Finding{gateFinding("f2")}, base: []BaseFinding{{ID: "f1"}}, baseSide: partialSide,
			wantOrigins: map[string]finding.Origin{"f2": finding.OriginUnknown}, wantStatus: OriginUnknown,
		},
		{
			name: "an exact match survives unavailable evidence", head: []finding.Finding{gateFinding("f1")},
			base: []BaseFinding{{ID: "f1"}}, baseSide: partialSide,
			wantOrigins: map[string]finding.Origin{"f1": finding.OriginPreExisting}, wantStatus: OriginUnknown,
		},
		{
			name: "a fixed finding gets no origin and is never introduced", head: []finding.Finding{fixed},
			baseSide: okSide, wantOrigins: map[string]finding.Origin{}, wantStatus: OriginComparable,
		},
		{
			// The representative is the smallest member ID: dropping the old
			// representative "a" moves the rollup ID to "b" although no edge is new.
			name: "a rollup whose representative left stays pre-existing",
			head: []finding.Finding{rollupFinding("b", "b", "c")},
			base: []BaseFinding{{ID: "a", Members: []string{"a", "b", "c"}}}, baseSide: okSide,
			wantOrigins: map[string]finding.Origin{"b": finding.OriginPreExisting}, wantStatus: OriginComparable,
		},
		{
			name: "a rollup that gained an edge is introduced even when its ID existed",
			head: []finding.Finding{rollupFinding("a", "a", "b", "z")},
			base: []BaseFinding{{ID: "a", Members: []string{"a", "b"}}}, baseSide: okSide,
			wantOrigins:    map[string]finding.Origin{"a": finding.OriginIntroduced},
			wantIntroduced: []string{"a"}, wantStatus: OriginComparable,
		},
		{
			name:        "a base rollup is resolved only when every edge is gone",
			head:        []finding.Finding{rollupFinding("b", "b")},
			base:        []BaseFinding{{ID: "a", Members: []string{"a", "b"}}, {ID: "x", Members: []string{"x", "y"}}},
			baseSide:    okSide,
			wantOrigins: map[string]finding.Origin{"b": finding.OriginPreExisting}, wantResolved: []string{"x"},
			wantStatus: OriginComparable,
		},
		{
			name: "a seam that qualified in base is pre-existing", head: []finding.Finding{seamGateFinding("a", "b")},
			baseSeams: []ModulePair{{From: "a", To: "b"}}, baseSide: okSide,
			wantOrigins: map[string]finding.Origin{seamGateAB: finding.OriginPreExisting}, wantStatus: OriginComparable,
		},
		{
			name: "a seam that did not qualify in base is introduced", head: []finding.Finding{seamGateFinding("a", "b")},
			baseSeams: []ModulePair{{From: "b", To: "a"}}, baseSide: okSide,
			wantOrigins:    map[string]finding.Origin{seamGateAB: finding.OriginIntroduced},
			wantIntroduced: []string{seamGateAB}, wantStatus: OriginComparable,
		},
		{
			name: "lists use a stable sorted order",
			head: []finding.Finding{gateFinding("z"), gateFinding("a"), gateFinding("m"), gateFinding("b")},
			base: []BaseFinding{{ID: "q"}, {ID: "m"}, {ID: "b"}, {ID: "c"}}, baseSide: okSide,
			wantOrigins: map[string]finding.Origin{"z": finding.OriginIntroduced, "a": finding.OriginIntroduced,
				"m": finding.OriginPreExisting, "b": finding.OriginPreExisting},
			wantIntroduced: []string{"a", "z"}, wantResolved: []string{"c", "q"}, wantStatus: OriginComparable,
		},
		{
			name: "a clean run still has empty lists", baseSide: okSide,
			wantOrigins: map[string]finding.Origin{}, wantStatus: OriginComparable,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got := ClassifyOrigins(OriginEvidence{
				Findings: tc.head, BaseFindings: tc.base, BaseSeams: tc.baseSeams,
				Head: okSide, Base: tc.baseSide, Families: goPrimaryFamily,
			})
			if got.Status != tc.wantStatus {
				t.Errorf("status = %q, want %q", got.Status, tc.wantStatus)
			}
			if !maps.Equal(got.Origins, tc.wantOrigins) {
				t.Errorf("origins = %v, want %v", got.Origins, tc.wantOrigins)
			}
			assertIDs(t, "introduced_finding_ids", got.Introduced, tc.wantIntroduced)
			assertIDs(t, "resolved_finding_ids", got.Resolved, tc.wantResolved)
			if got.Reasons == nil {
				t.Error("origin reasons must be a non-null array")
			}
		})
	}

	// The whole point of the partial split: on a TypeScript or Python repo the
	// primary analyzer reports partial on both sides as its steady state. Before
	// the split that pinned every unmatched finding to unknown, which made the
	// origin delta inert on those languages.
	t.Run("symmetric unresolved partial still places an unmatched finding", func(t *testing.T) {
		t.Parallel()
		unresolvedSide := func(n int) AnalyzerEvidence {
			row := covRow(toolDepCruiser, evidence.StatusPartial)
			row.Unresolved = n
			return AnalyzerEvidence{Coverage: []evidence.Coverage{row}}
		}
		got := ClassifyOrigins(OriginEvidence{
			Findings: []finding.Finding{gateFinding("f2")}, BaseFindings: []BaseFinding{{ID: "f1"}},
			Head: unresolvedSide(4), Base: unresolvedSide(6),
			Families: []AnalyzerFamily{{name: toolDepCruiser, primary: true}},
		})
		assertIDs(t, "introduced_finding_ids", got.Introduced, []string{"f2"})
		if got.Status != OriginComparable {
			t.Errorf("status = %q, want %q", got.Status, OriginComparable)
		}
		if len(got.Reasons) != 1 {
			t.Fatalf("reasons = %v, want the degradation disclosed", got.Reasons)
		}
	})

	// One binary and one config measured both trees, so a profile difference
	// came from the trees: it pairs and is named, never a blanket unknown.
	t.Run("a measurement profile difference pairs and is named", func(t *testing.T) {
		t.Parallel()
		profile := func(settings, version string) *evidence.MeasurementProfile {
			return &evidence.MeasurementProfile{SettingsHash: settings, Producers: []evidence.MeasurementProducer{
				{Tool: toolGoPackages, SemanticsVersion: "go/packages.v1", ToolVersion: version, Status: evidence.StatusOK},
			}}
		}
		head, base := okSide, okSide
		head.Profile, base.Profile = profile("aaaaaaaaaaaaaaaa", "go1.25.3"), profile("bbbbbbbbbbbbbbbb", "go1.25.1")
		got := ClassifyOrigins(OriginEvidence{
			Findings: []finding.Finding{gateFinding("f2")}, Head: head, Base: base, Families: goPrimaryFamily,
		})
		if got.Origins["f2"] != finding.OriginIntroduced || got.Status != OriginComparable {
			t.Errorf("origin = %q, status = %q; a tree-driven profile difference must not unpair", got.Origins["f2"], got.Status)
		}
		want := []string{
			"go/packages: tool version differs (head go1.25.3, base go1.25.1)",
			"measurement settings: head aaaaaaaaaaaa, base bbbbbbbbbbbb — tree-derived inputs differ (Go environment, TypeScript config)",
		}
		if !slices.Equal(got.Reasons, want) {
			t.Errorf("reasons = %v, want %v", got.Reasons, want)
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

func testOriginAnalyzerEvidence(t *testing.T) {
	t.Parallel()
	goFam := AnalyzerFamily{name: toolGoPackages, primary: true}
	dcFam := AnalyzerFamily{name: toolDepCruiser, primary: true}
	scipFam := AnalyzerFamily{name: toolScip}
	astFam := AnalyzerFamily{name: toolAstGrep}
	goGap := []evidence.CoverageGap{{Tool: toolGoPackages}}
	scipGap := []evidence.CoverageGap{{Tool: toolScip}}

	// unresolvedRow is the dependency-cruiser/grimp steady state: the analyzer
	// walked the whole tree and could not resolve n import specifiers.
	unresolvedRow := func(n int) evidence.Coverage {
		c := covRow(toolDepCruiser, evidence.StatusPartial)
		c.Unresolved = n
		return c
	}
	// goUnresolvedRow is NOT that shape: go/packages counts whole packages it
	// SKIPPED because they failed to load, so its partial means the run did not
	// finish over the tree.
	goUnresolvedRow := func(n int) evidence.Coverage {
		c := covRow(toolGoPackages, evidence.StatusPartial)
		c.Unresolved = n
		return c
	}

	tests := []struct {
		name           string
		fam            AnalyzerFamily
		head, base     []evidence.Coverage
		headGap, bsGap []evidence.CoverageGap
		want           bool
		// degraded marks a pair that IS comparable but must still disclose one
		// reason (symmetric unresolved-specifier partial).
		degraded bool
	}{
		{name: "ok/ok", fam: goFam, head: []evidence.Coverage{covRow(toolGoPackages, evidence.StatusOK)}, base: []evidence.Coverage{covRow(toolGoPackages, evidence.StatusOK)}, want: true},
		{name: "ok/not_applicable", fam: goFam, head: []evidence.Coverage{covRow(toolGoPackages, evidence.StatusOK)}, base: []evidence.Coverage{covRow(toolGoPackages, evidence.StatusAbsent)}, want: true},
		{name: "not_applicable/ok", fam: goFam, head: []evidence.Coverage{covRow(toolGoPackages, evidence.StatusAbsent)}, base: []evidence.Coverage{covRow(toolGoPackages, evidence.StatusOK)}, want: true},
		{name: "not_applicable both sides is ignored", fam: goFam, head: []evidence.Coverage{covRow(toolGoPackages, evidence.StatusAbsent)}, base: []evidence.Coverage{covRow(toolGoPackages, evidence.StatusAbsent)}, want: true},
		{name: "primary absent with a coverage gap is unavailable", fam: goFam, head: []evidence.Coverage{covRow(toolGoPackages, evidence.StatusOK)}, base: []evidence.Coverage{covRow(toolGoPackages, evidence.StatusAbsent)}, bsGap: goGap, want: false},
		{name: "partial with no unresolved count is unavailable", fam: goFam, head: []evidence.Coverage{covRow(toolGoPackages, evidence.StatusOK)}, base: []evidence.Coverage{covRow(toolGoPackages, evidence.StatusPartial)}, want: false},
		// dependency-cruiser and grimp mark a COMPLETED run partial as soon as one
		// import specifier anywhere fails to resolve. Symmetric, it is shared
		// incompleteness over one tree: comparable, but always disclosed.
		{name: "symmetric unresolved partial is comparable and disclosed", fam: dcFam, head: []evidence.Coverage{unresolvedRow(4)}, base: []evidence.Coverage{unresolvedRow(9)}, want: true, degraded: true},
		{name: "unresolved partial never pairs with ok", fam: dcFam, head: []evidence.Coverage{unresolvedRow(4)}, base: []evidence.Coverage{covRow(toolDepCruiser, evidence.StatusOK)}, want: false},
		{name: "unresolved partial never pairs with a failed partial", fam: dcFam, head: []evidence.Coverage{unresolvedRow(4)}, base: []evidence.Coverage{covRow(toolDepCruiser, evidence.StatusPartial)}, want: false},
		// go/packages sets Unresolved on a partial too, but there it counts whole
		// packages it SKIPPED because they failed to load — the "did not finish"
		// meaning. A symmetric Go partial must NOT read as shared incompleteness,
		// or a base side with N unloaded packages produces a false "introduced".
		{name: "go/packages skipped-package partial is unavailable on both sides", fam: goFam, head: []evidence.Coverage{goUnresolvedRow(3)}, base: []evidence.Coverage{goUnresolvedRow(3)}, want: false},
		{name: "go/packages skipped-package partial never pairs with ok", fam: goFam, head: []evidence.Coverage{goUnresolvedRow(3)}, base: []evidence.Coverage{covRow(toolGoPackages, evidence.StatusOK)}, want: false},
		{name: "timed out is unavailable", fam: goFam, head: []evidence.Coverage{covRow(toolGoPackages, evidence.StatusTimedOut)}, base: []evidence.Coverage{covRow(toolGoPackages, evidence.StatusOK)}, want: false},
		{name: "missing row on one side is unavailable", fam: goFam, base: []evidence.Coverage{covRow(toolGoPackages, evidence.StatusOK)}, want: false},
		{name: "missing row on both sides is unavailable", fam: goFam, want: false},
		{name: "duplicate row on one side is unavailable", fam: goFam, head: []evidence.Coverage{covRow(toolGoPackages, evidence.StatusOK), covRow(toolGoPackages, evidence.StatusOK)}, base: []evidence.Coverage{covRow(toolGoPackages, evidence.StatusOK)}, want: false},
		// Every analyzer owns its own coverage name, so a repeated name is an
		// anomaly on BOTH sides too — there is no way to know which duplicate
		// pairs with which. Same rule as gradeTool.
		{name: "matching duplicate rows are still unavailable", fam: astFam, head: []evidence.Coverage{covRow(toolAstGrep, evidence.StatusOK), covRow(toolAstGrep, evidence.StatusOK)}, base: []evidence.Coverage{covRow(toolAstGrep, evidence.StatusOK), covRow(toolAstGrep, evidence.StatusOK)}, want: false},
		{name: "the pattern pass ignores the syntax pass's own row", fam: astFam, head: []evidence.Coverage{covRow(toolAstGrep, evidence.StatusOK), covRow(toolAstGrepSyntax, evidence.StatusDisabled)}, base: []evidence.Coverage{covRow(toolAstGrep, evidence.StatusOK), covRow(toolAstGrepSyntax, evidence.StatusOK)}, want: true},
		{name: "the syntax pass compares on its own row", fam: AnalyzerFamily{name: toolAstGrepSyntax}, head: []evidence.Coverage{covRow(toolAstGrep, evidence.StatusOK), covRow(toolAstGrepSyntax, evidence.StatusDisabled)}, base: []evidence.Coverage{covRow(toolAstGrep, evidence.StatusOK), covRow(toolAstGrepSyntax, evidence.StatusOK)}, want: false},
		{name: "disabled on both sides is ignored", fam: scipFam, head: []evidence.Coverage{covRow(toolScip, evidence.StatusDisabled)}, base: []evidence.Coverage{covRow(toolScip, evidence.StatusDisabled)}, want: true},
		{name: "disabled on one side only is unavailable", fam: scipFam, head: []evidence.Coverage{covRow(toolScip, evidence.StatusOK)}, base: []evidence.Coverage{covRow(toolScip, evidence.StatusDisabled)}, want: false},
		// A non-primary analyzer's absence is evidence about the TOOL, not the
		// tree: asymmetric absence could hide a base finding, symmetric absence
		// means neither side produced one.
		{name: "non-primary absent on one side only is unavailable", fam: scipFam, head: []evidence.Coverage{covRow(toolScip, evidence.StatusOK)}, base: []evidence.Coverage{covRow(toolScip, evidence.StatusAbsent)}, want: false},
		// CHANGED from degraded:false — this row pinned the defect. A CoverageGap
		// is only emitted for tools in the install-hint table, and scip is not in
		// it, so this shape (the live one on archfit's own config wherever no SCIP
		// indexer is installed) paired SILENTLY while gradeTool graded the
		// identical row comparable_with_gaps and emitted a detail. Every family
		// compared here was activated by the effective config, so its absence is
		// always shared blindness that must be disclosed.
		{name: "non-primary absent on both sides is comparable and disclosed", fam: scipFam, head: []evidence.Coverage{covRow(toolScip, evidence.StatusAbsent)}, base: []evidence.Coverage{covRow(toolScip, evidence.StatusAbsent)}, want: true, degraded: true},
		// CHANGED from want:false. An enabled analyzer whose tool is missing on
		// the host reports absent WITH a gap on BOTH sides. Symmetric, that is the
		// same safety argument as gapless symmetric absence — neither side ran it,
		// so neither has a finding the other hides — and gradeTool
		// already grades it comparable_with_gaps. Failing it made --base
		// permanently all-unknown wherever an enabled analyzer is uninstalled,
		// including archfit's own runtime image on any repo with a Cargo.toml.
		// It pairs DEGRADED: the shared blindness is always disclosed.
		{name: "absent with a coverage gap on both sides is comparable and disclosed", fam: scipFam, head: []evidence.Coverage{covRow(toolScip, evidence.StatusAbsent)}, base: []evidence.Coverage{covRow(toolScip, evidence.StatusAbsent)}, headGap: scipGap, bsGap: scipGap, want: true, degraded: true},
		{name: "primary absent with a coverage gap on both sides is comparable and disclosed", fam: goFam, head: []evidence.Coverage{covRow(toolGoPackages, evidence.StatusAbsent)}, base: []evidence.Coverage{covRow(toolGoPackages, evidence.StatusAbsent)}, headGap: goGap, bsGap: goGap, want: true, degraded: true},
		// CHANGED from want:false. Gap presence discriminates for PRIMARY families
		// only, where a missing gap proves the language is absent from that tree.
		// For a non-primary family both sides are plainly absent and equally blind
		// however the install-hint table happened to classify them, so they pair —
		// degraded, never silently. The gap-asymmetry rule still holds where it
		// means something: see the primary rows below.
		{name: "non-primary absent gapped on head only is comparable and disclosed", fam: scipFam, head: []evidence.Coverage{covRow(toolScip, evidence.StatusAbsent)}, base: []evidence.Coverage{covRow(toolScip, evidence.StatusAbsent)}, headGap: scipGap, want: true, degraded: true},
		{name: "non-primary absent gapped on base only is comparable and disclosed", fam: scipFam, head: []evidence.Coverage{covRow(toolScip, evidence.StatusAbsent)}, base: []evidence.Coverage{covRow(toolScip, evidence.StatusAbsent)}, bsGap: scipGap, want: true, degraded: true},
		// The gap is derived per side from that side's own tree, so a project
		// marker ADDED by the change gaps head and leaves base not_applicable.
		// For a PRIMARY family that asymmetry is real — one side has none of the
		// language, the other has it and could not analyze it — so it never pairs.
		{name: "primary absent gapped on head only is unavailable", fam: goFam, head: []evidence.Coverage{covRow(toolGoPackages, evidence.StatusAbsent)}, base: []evidence.Coverage{covRow(toolGoPackages, evidence.StatusAbsent)}, headGap: goGap, want: false},
		{name: "primary absent gapped on base only is unavailable", fam: goFam, head: []evidence.Coverage{covRow(toolGoPackages, evidence.StatusAbsent)}, base: []evidence.Coverage{covRow(toolGoPackages, evidence.StatusAbsent)}, bsGap: goGap, want: false},
		{name: "absent gapped never pairs with ok", fam: scipFam, head: []evidence.Coverage{covRow(toolScip, evidence.StatusAbsent)}, base: []evidence.Coverage{covRow(toolScip, evidence.StatusOK)}, headGap: scipGap, want: false},
		{name: "absent gapped never pairs with a timeout", fam: scipFam, head: []evidence.Coverage{covRow(toolScip, evidence.StatusAbsent)}, base: []evidence.Coverage{covRow(toolScip, evidence.StatusTimedOut)}, headGap: scipGap, want: false},
		// A timeout is flaky, not structural: symmetry proves nothing about what
		// either side would have found, so it stays unavailable on both sides.
		{name: "timed out on both sides is still unavailable", fam: scipFam, head: []evidence.Coverage{covRow(toolScip, evidence.StatusTimedOut)}, base: []evidence.Coverage{covRow(toolScip, evidence.StatusTimedOut)}, want: false},
		{name: "non-primary absent never pairs with ok", fam: scipFam, head: []evidence.Coverage{covRow(toolScip, evidence.StatusAbsent)}, base: []evidence.Coverage{covRow(toolScip, evidence.StatusOK)}, want: false},
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
			AnalyzerEvidence{Coverage: []evidence.Coverage{unresolvedRow(4)}},
			AnalyzerEvidence{Coverage: []evidence.Coverage{heavy}})
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
		head := AnalyzerEvidence{Coverage: []evidence.Coverage{
			covRow(toolScip, evidence.StatusOK),
			covRow(toolGoPackages, evidence.StatusOK),
			covRow(toolJscpd, evidence.StatusOK),
		}}
		base := AnalyzerEvidence{Coverage: []evidence.Coverage{
			covRow(toolScip, evidence.StatusPartial),
			covRow(toolGoPackages, evidence.StatusTimedOut),
			covRow(toolJscpd, evidence.StatusOK),
		}}
		delta := ClassifyOrigins(OriginEvidence{Head: head, Base: base, Families: fams})
		if len(delta.Reasons) != 2 {
			t.Fatalf("reasons = %v, want one per unavailable family", delta.Reasons)
		}
		if !slices.IsSorted(delta.Reasons) {
			t.Errorf("reasons must be sorted: %v", delta.Reasons)
		}
	})
}
func testOriginBaseFindingIDs(t *testing.T) {
	t.Parallel()
	got := BaseFindings([]finding.Finding{
		{ID: "z", Kind: finding.KindAdvisory, Status: finding.StatusNew, Members: []string{"z", "zz"}},
		{ID: "gone", Kind: finding.KindGate, Status: finding.StatusFixed},
		{ID: "a", Kind: finding.KindGate, Status: finding.StatusWaived},
		{ID: "m", Kind: finding.KindAdvisory, Status: finding.StatusExpiredWaiver},
	})
	want := []BaseFinding{{ID: "a"}, {ID: "m"}, {ID: "z", Members: []string{"z", "zz"}}}
	if !slices.EqualFunc(got, want, func(a, b BaseFinding) bool { return a.ID == b.ID && slices.Equal(a.Members, b.Members) }) {
		t.Errorf("BaseFindings = %v, want %v (fixed dropped, sorted, kind ignored, members kept)", got, want)
	}
}
