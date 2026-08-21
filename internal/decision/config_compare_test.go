package decision_test

import (
	"slices"
	"strings"
	"testing"

	"github.com/alexei-led/archfit/internal/decision"
	"github.com/alexei-led/archfit/internal/model/diagnostic"
	"github.com/alexei-led/archfit/internal/model/finding"
	"github.com/alexei-led/archfit/internal/score"
)

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

// primaryTool is a per-language graph analyzer name; gapTool is never primary,
// so the primary-absent carve-out cannot apply to it.
const (
	primaryTool = "go/packages"
	gapTool     = "cargo-modules"
)

// Finding IDs and the duplicate-row rendering reused across table cases.
const (
	idShared = "shared"
	idHere   = "here"
	idHalf   = "half"
	dupOK    = diagnostic.StatusOK + "+" + diagnostic.StatusOK
)

func cov(tool, status string) diagnostic.Coverage {
	return diagnostic.Coverage{Tool: tool, Status: status}
}

func gap(tool string) diagnostic.CoverageGap {
	return diagnostic.CoverageGap{Tool: tool, InstallCmd: "install " + tool}
}

// side builds one comparison side from coverage rows alone; findings, score,
// and edge counts default to empty.
func side(rows []diagnostic.Coverage, gaps []diagnostic.CoverageGap) decision.ConfigCompareSide {
	return decision.ConfigCompareSide{
		Diag: diagnostic.Diagnostic{
			Findings:              []finding.Finding{},
			ToolCoverage:          rows,
			CoverageGaps:          gaps,
			PrimaryExtractorTools: []string{primaryTool},
		},
		Score: measuredCard(70),
	}
}

func measuredCard(overall int) score.Scorecard {
	return score.Scorecard{
		RubricVersion: 1,
		Overall:       overall,
		OverallBand:   bandForValue(overall),
		Dimensions: []score.Dimension{
			{Name: score.DimCouplingBalance, Value: overall, Band: bandForValue(overall)},
		},
	}
}

func unmeasuredCard() score.Scorecard {
	return score.Scorecard{
		RubricVersion: 1,
		Overall:       0,
		OverallBand:   score.BandNA,
		Dimensions: []score.Dimension{
			{Name: score.DimCouplingBalance, Value: 0, Band: score.BandNA},
		},
	}
}

func findingsSide(fs ...finding.Finding) decision.ConfigCompareSide {
	return decision.ConfigCompareSide{
		Diag:  diagnostic.Diagnostic{Findings: fs, ToolCoverage: []diagnostic.Coverage{}},
		Score: measuredCard(70),
	}
}

func edgesSide(scored, abstained, external int) decision.ConfigCompareSide {
	return decision.ConfigCompareSide{
		Diag: diagnostic.Diagnostic{
			Findings:     []finding.Finding{},
			ToolCoverage: []diagnostic.Coverage{},
			ClassifiedEdges: &diagnostic.ClassifiedEdgeSummary{
				Scored: scored, Abstained: abstained, External: external,
			},
		},
		Score: measuredCard(70),
	}
}

func warningCodes(ws []decision.ConfigCompareWarning) []string {
	codes := make([]string, 0, len(ws))
	for _, w := range ws {
		codes = append(codes, w.Code)
	}
	return codes
}

// ---------------------------------------------------------------------------
// Identity
// ---------------------------------------------------------------------------

// TestCompareConfigs_Identity pins the shape the `config compare <same> <same>`
// smoke run depends on: no finding movement, no score delta, no warnings, and
// every result slice non-nil.
func TestCompareConfigs_Identity(t *testing.T) {
	s := decision.ConfigCompareSide{
		Diag: diagnostic.Diagnostic{
			Findings: []finding.Finding{
				{ID: "f1", Kind: finding.KindGate, Status: finding.StatusNew},
			},
			ToolCoverage:          []diagnostic.Coverage{cov(primaryTool, diagnostic.StatusOK)},
			PrimaryExtractorTools: []string{primaryTool},
			ClassifiedEdges:       &diagnostic.ClassifiedEdgeSummary{Scored: 10, Abstained: 2, External: 3},
		},
		Score: measuredCard(70),
	}

	got := decision.CompareConfigs(decision.ConfigCompareInput{Current: s, Candidate: s})

	if !slices.Equal(got.Findings.BothIDs, []string{"f1"}) {
		t.Errorf("BothIDs = %v, want [f1]", got.Findings.BothIDs)
	}
	if len(got.Findings.CurrentOnlyIDs) != 0 || len(got.Findings.CandidateOnlyIDs) != 0 {
		t.Errorf("one-sided buckets not empty: %+v", got.Findings)
	}
	if got.Findings.CurrentOnlyIDs == nil || got.Findings.CandidateOnlyIDs == nil || got.Findings.BothIDs == nil {
		t.Errorf("finding lists must be non-nil: %+v", got.Findings)
	}
	if got.Coverage.Status != decision.CoverageComparable {
		t.Errorf("Coverage.Status = %q, want %q", got.Coverage.Status, decision.CoverageComparable)
	}
	if got.Coverage.Details == nil || len(got.Coverage.Details) != 0 {
		t.Errorf("Coverage.Details = %+v, want empty non-nil", got.Coverage.Details)
	}
	if got.ScoreDelta == nil || *got.ScoreDelta != 0 {
		t.Errorf("ScoreDelta = %v, want 0", got.ScoreDelta)
	}
	if got.Warnings == nil || len(got.Warnings) != 0 {
		t.Errorf("Warnings = %+v, want empty non-nil", got.Warnings)
	}
}

// TestCompareConfigs_EmptyInput pins that a fully empty pair still produces a
// complete, non-nil result rather than a nil-slice payload.
func TestCompareConfigs_EmptyInput(t *testing.T) {
	got := decision.CompareConfigs(decision.ConfigCompareInput{})

	if got.Coverage.Status != decision.CoverageComparable {
		t.Errorf("Coverage.Status = %q, want %q", got.Coverage.Status, decision.CoverageComparable)
	}
	if got.Findings.BothIDs == nil || got.Coverage.Details == nil || got.Warnings == nil {
		t.Errorf("result slices must be non-nil: %+v", got)
	}
	// Both sides carry a zero Scorecard, whose OverallBand is "" — not BandNA —
	// so the delta is a real 0 rather than unknown.
	if got.ScoreDelta == nil || *got.ScoreDelta != 0 {
		t.Errorf("ScoreDelta = %v, want 0", got.ScoreDelta)
	}
}

// ---------------------------------------------------------------------------
// Findings
// ---------------------------------------------------------------------------

func TestCompareConfigs_Findings(t *testing.T) {
	tests := []struct {
		name                       string
		current, candidate         []finding.Finding
		wantCurrent, wantCandidate []string
		wantBoth                   []string
	}{
		{
			name:          "disjoint sets split into one-sided buckets",
			current:       []finding.Finding{{ID: "a"}, {ID: "b"}},
			candidate:     []finding.Finding{{ID: "c"}},
			wantCurrent:   []string{"a", "b"},
			wantCandidate: []string{"c"},
			wantBoth:      []string{},
		},
		{
			name:          "gate on one side and advisory on the other is the same finding",
			current:       []finding.Finding{{ID: idShared, Kind: finding.KindGate}},
			candidate:     []finding.Finding{{ID: idShared, Kind: finding.KindAdvisory}},
			wantCurrent:   []string{},
			wantCandidate: []string{},
			wantBoth:      []string{idShared},
		},
		{
			name:          "fixed entries are not observed",
			current:       []finding.Finding{{ID: "gone", Status: finding.StatusFixed}, {ID: idHere}},
			candidate:     []finding.Finding{{ID: idHere}, {ID: "also-gone", Status: finding.StatusFixed}},
			wantCurrent:   []string{},
			wantCandidate: []string{},
			wantBoth:      []string{idHere},
		},
		{
			name:          "lifecycle labels do not split a matching id",
			current:       []finding.Finding{{ID: "x", Status: finding.StatusNew}},
			candidate:     []finding.Finding{{ID: "x", Status: finding.StatusWaived}},
			wantCurrent:   []string{},
			wantCandidate: []string{},
			wantBoth:      []string{"x"},
		},
		{
			name:          "ids are sorted and deduplicated",
			current:       []finding.Finding{{ID: "z"}, {ID: "a"}, {ID: "z"}},
			candidate:     []finding.Finding{},
			wantCurrent:   []string{"a", "z"},
			wantCandidate: []string{},
			wantBoth:      []string{},
		},
		{
			name:          "a fixed entry on one side only is current-only on the other",
			current:       []finding.Finding{{ID: idHalf}},
			candidate:     []finding.Finding{{ID: idHalf, Status: finding.StatusFixed}},
			wantCurrent:   []string{idHalf},
			wantCandidate: []string{},
			wantBoth:      []string{},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := decision.CompareConfigs(decision.ConfigCompareInput{
				Current:   findingsSide(tt.current...),
				Candidate: findingsSide(tt.candidate...),
			}).Findings
			if !slices.Equal(got.CurrentOnlyIDs, tt.wantCurrent) {
				t.Errorf("CurrentOnlyIDs = %v, want %v", got.CurrentOnlyIDs, tt.wantCurrent)
			}
			if !slices.Equal(got.CandidateOnlyIDs, tt.wantCandidate) {
				t.Errorf("CandidateOnlyIDs = %v, want %v", got.CandidateOnlyIDs, tt.wantCandidate)
			}
			if !slices.Equal(got.BothIDs, tt.wantBoth) {
				t.Errorf("BothIDs = %v, want %v", got.BothIDs, tt.wantBoth)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// Coverage — every ordered status pair
// ---------------------------------------------------------------------------

// TestCompareCoverage_StatusPairs enumerates all 25 ordered pairs over the five
// coverage statuses, then the three cases where "partial" means a completed run
// with unresolved import specifiers. The tool here is NOT primary, so the
// absent/absent carve-out never applies and each pair reads its plain rule.
//
// Every "partial" in the 25-pair table carries Unresolved == 0 — the shape a
// failed extractor, a rejected ast-grep rule file, or an empty SCIP index
// produces. Those never pair.
func TestCompareCoverage_StatusPairs(t *testing.T) {
	statuses := []string{
		diagnostic.StatusOK,
		diagnostic.StatusPartial,
		diagnostic.StatusAbsent,
		diagnostic.StatusDisabled,
		diagnostic.StatusTimedOut,
	}
	// want[current][candidate]
	want := map[string]map[string]decision.CoverageComparability{
		diagnostic.StatusOK: {
			diagnostic.StatusOK:       decision.CoverageComparable,
			diagnostic.StatusPartial:  decision.CoverageNotComparable,
			diagnostic.StatusAbsent:   decision.CoverageNotComparable,
			diagnostic.StatusDisabled: decision.CoverageNotComparable,
			diagnostic.StatusTimedOut: decision.CoverageNotComparable,
		},
		diagnostic.StatusPartial: {
			diagnostic.StatusOK:       decision.CoverageNotComparable,
			diagnostic.StatusPartial:  decision.CoverageNotComparable,
			diagnostic.StatusAbsent:   decision.CoverageNotComparable,
			diagnostic.StatusDisabled: decision.CoverageNotComparable,
			diagnostic.StatusTimedOut: decision.CoverageNotComparable,
		},
		diagnostic.StatusAbsent: {
			diagnostic.StatusOK:       decision.CoverageNotComparable,
			diagnostic.StatusPartial:  decision.CoverageNotComparable,
			diagnostic.StatusAbsent:   decision.CoverageComparableWithGaps,
			diagnostic.StatusDisabled: decision.CoverageNotComparable,
			diagnostic.StatusTimedOut: decision.CoverageNotComparable,
		},
		diagnostic.StatusDisabled: {
			diagnostic.StatusOK:       decision.CoverageNotComparable,
			diagnostic.StatusPartial:  decision.CoverageNotComparable,
			diagnostic.StatusAbsent:   decision.CoverageNotComparable,
			diagnostic.StatusDisabled: decision.CoverageComparableWithGaps,
			diagnostic.StatusTimedOut: decision.CoverageNotComparable,
		},
		diagnostic.StatusTimedOut: {
			diagnostic.StatusOK:       decision.CoverageNotComparable,
			diagnostic.StatusPartial:  decision.CoverageNotComparable,
			diagnostic.StatusAbsent:   decision.CoverageNotComparable,
			diagnostic.StatusDisabled: decision.CoverageNotComparable,
			diagnostic.StatusTimedOut: decision.CoverageNotComparable,
		},
	}

	for _, cur := range statuses {
		for _, cand := range statuses {
			t.Run(cur+"/"+cand, func(t *testing.T) {
				got := decision.CompareConfigs(decision.ConfigCompareInput{
					Current:   side([]diagnostic.Coverage{cov(gapTool, cur)}, nil),
					Candidate: side([]diagnostic.Coverage{cov(gapTool, cand)}, nil),
				}).Coverage
				if got.Status != want[cur][cand] {
					t.Errorf("status = %q, want %q", got.Status, want[cur][cand])
				}
				if want[cur][cand] == decision.CoverageComparable {
					if len(got.Details) != 0 {
						t.Errorf("details = %+v, want none for a comparable pair", got.Details)
					}
					return
				}
				if len(got.Details) != 1 {
					t.Fatalf("details = %+v, want exactly one", got.Details)
				}
				d := got.Details[0]
				if d.Tool != gapTool || d.Current != cur || d.Candidate != cand || d.Reason == "" {
					t.Errorf("detail = %+v, want tool/status echo with a reason", d)
				}
			})
		}
	}

	// dependency-cruiser and grimp report "partial" whenever ONE import
	// specifier anywhere in the tree is unresolvable — the normal steady state,
	// not a completion failure. Grading it not_comparable pinned every
	// TypeScript and Python comparison to not_comparable forever.
	//
	// The TOOL NAME is the discriminator, not Unresolved > 0: go/packages sets
	// Unresolved on a partial too, counting whole packages it SKIPPED because
	// they failed to load. That is "did not finish" and must stay not_comparable.
	unresolved := func(tool string, n int) diagnostic.Coverage {
		c := cov(tool, diagnostic.StatusPartial)
		c.Unresolved = n
		return c
	}
	const (
		specifierTool = "dependency-cruiser"
		grimpTool     = "grimp"
		goTool        = "go/packages"
	)
	partialPairs := []struct {
		name               string
		current, candidate diagnostic.Coverage
		want               decision.CoverageComparability
	}{
		{"unresolved_both", unresolved(specifierTool, 3), unresolved(specifierTool, 7), decision.CoverageComparableWithGaps},
		{"unresolved_both_grimp", unresolved(grimpTool, 3), unresolved(grimpTool, 7), decision.CoverageComparableWithGaps},
		{"unresolved_current_only", unresolved(specifierTool, 3), cov(specifierTool, diagnostic.StatusPartial), decision.CoverageNotComparable},
		{"unresolved_candidate_only", cov(specifierTool, diagnostic.StatusPartial), unresolved(specifierTool, 3), decision.CoverageNotComparable},
		{"unresolved_vs_ok", unresolved(specifierTool, 3), cov(specifierTool, diagnostic.StatusOK), decision.CoverageNotComparable},
		{"ok_vs_unresolved", cov(specifierTool, diagnostic.StatusOK), unresolved(specifierTool, 3), decision.CoverageNotComparable},
		// go/packages counts SKIPPED PACKAGES in Unresolved, so a symmetric Go
		// partial is two runs that both failed to load part of the tree — never
		// comparable, however equal the two counts look.
		{"go_packages_skipped_both", unresolved(goTool, 3), unresolved(goTool, 3), decision.CoverageNotComparable},
		{"go_packages_skipped_vs_ok", unresolved(goTool, 3), cov(goTool, diagnostic.StatusOK), decision.CoverageNotComparable},
		// Neither does any other partial producer that happens to set a count.
		{"unknown_tool_unresolved_both", unresolved(gapTool, 3), unresolved(gapTool, 3), decision.CoverageNotComparable},
	}
	for _, tt := range partialPairs {
		t.Run("partial_"+tt.name, func(t *testing.T) {
			got := decision.CompareConfigs(decision.ConfigCompareInput{
				Current:   side([]diagnostic.Coverage{tt.current}, nil),
				Candidate: side([]diagnostic.Coverage{tt.candidate}, nil),
			}).Coverage
			if got.Status != tt.want {
				t.Errorf("status = %q, want %q", got.Status, tt.want)
			}
			if len(got.Details) != 1 {
				t.Fatalf("details = %+v, want exactly one — the loss must be disclosed", got.Details)
			}
			if got.Details[0].Reason == "" {
				t.Errorf("detail has no reason: %+v", got.Details[0])
			}
		})
	}

	// The reason for a comparable_with_gaps partial pair must carry BOTH sides'
	// magnitudes: the pairing rule is magnitude-blind, so 3 unresolved and
	// 5000/6000 unresolved are indistinguishable without them. Current/Candidate
	// stay the raw status — they are typed machine fields, not prose.
	t.Run("partial_reason_carries_magnitudes", func(t *testing.T) {
		cur := unresolved(specifierTool, 3)
		cand := unresolved(specifierTool, 5000)
		cand.SpecifiersSeen = 6000
		got := decision.CompareConfigs(decision.ConfigCompareInput{
			Current:   side([]diagnostic.Coverage{cur}, nil),
			Candidate: side([]diagnostic.Coverage{cand}, nil),
		}).Coverage
		if len(got.Details) != 1 {
			t.Fatalf("details = %+v, want exactly one", got.Details)
		}
		d := got.Details[0]
		if !strings.Contains(d.Reason, "3 unresolved") || !strings.Contains(d.Reason, "5000/6000 unresolved") {
			t.Errorf("reason must name both magnitudes, got %q", d.Reason)
		}
		if d.Current != diagnostic.StatusPartial || d.Candidate != diagnostic.StatusPartial {
			t.Errorf("Current/Candidate must stay raw statuses, got %q/%q", d.Current, d.Candidate)
		}
	})
}

// TestCompareCoverage_RowCount pins the missing-row and duplicate-row rules,
// including the duplicate-on-both-sides case. Every analyzer now reports under
// its own coverage name (the pattern pass is "ast-grep", the syntax pass
// "ast-grep/syntax"), so a repeated name is a genuine anomaly — graded
// not_comparable rather than pairing the duplicates by guesswork.
func TestCompareCoverage_RowCount(t *testing.T) {
	tests := []struct {
		name               string
		current, candidate []diagnostic.Coverage
		wantCurrent        string
		wantCandidate      string
	}{
		{
			name:          "row missing on the current side",
			current:       nil,
			candidate:     []diagnostic.Coverage{cov(gapTool, diagnostic.StatusOK)},
			wantCurrent:   decision.CoverageRowMissing,
			wantCandidate: diagnostic.StatusOK,
		},
		{
			name:          "row missing on the candidate side",
			current:       []diagnostic.Coverage{cov(gapTool, diagnostic.StatusOK)},
			candidate:     nil,
			wantCurrent:   diagnostic.StatusOK,
			wantCandidate: decision.CoverageRowMissing,
		},
		{
			name:          "duplicate row on the candidate side only",
			current:       []diagnostic.Coverage{cov(gapTool, diagnostic.StatusOK)},
			candidate:     []diagnostic.Coverage{cov(gapTool, diagnostic.StatusOK), cov(gapTool, diagnostic.StatusOK)},
			wantCurrent:   diagnostic.StatusOK,
			wantCandidate: dupOK,
		},
		{
			name:          "duplicate row on both sides is still not comparable",
			current:       []diagnostic.Coverage{cov(gapTool, diagnostic.StatusOK), cov(gapTool, diagnostic.StatusOK)},
			candidate:     []diagnostic.Coverage{cov(gapTool, diagnostic.StatusOK), cov(gapTool, diagnostic.StatusOK)},
			wantCurrent:   dupOK,
			wantCandidate: dupOK,
		},
		{
			name:          "a missing primary row is not silently ignored",
			current:       []diagnostic.Coverage{cov(primaryTool, diagnostic.StatusAbsent)},
			candidate:     nil,
			wantCurrent:   diagnostic.StatusAbsent,
			wantCandidate: decision.CoverageRowMissing,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := decision.CompareConfigs(decision.ConfigCompareInput{
				Current:   side(tt.current, nil),
				Candidate: side(tt.candidate, nil),
			}).Coverage
			if got.Status != decision.CoverageNotComparable {
				t.Errorf("status = %q, want %q", got.Status, decision.CoverageNotComparable)
			}
			if len(got.Details) != 1 {
				t.Fatalf("details = %+v, want exactly one", got.Details)
			}
			d := got.Details[0]
			if d.Current != tt.wantCurrent || d.Candidate != tt.wantCandidate {
				t.Errorf("detail sides = %q/%q, want %q/%q", d.Current, d.Candidate, tt.wantCurrent, tt.wantCandidate)
			}
		})
	}
}

// TestCompareCoverage_PrimaryAbsent pins the per-side gap condition, the same
// rule the git-origin delta applies: a coverage gap says THIS configuration
// expected the analyzer to run, so a gap on one side only is an asymmetry, not
// shared blindness. An equal primary absent pair drops out only when neither
// side gapped; an equal gap on both sides is shared blindness.
func TestCompareCoverage_PrimaryAbsent(t *testing.T) {
	tests := []struct {
		name            string
		tool            string
		curGap, candGap bool
		wantStatus      decision.CoverageComparability
		wantDetails     int
	}{
		{
			name:        "primary absent with no gap on either side drops out",
			tool:        primaryTool,
			wantStatus:  decision.CoverageComparable,
			wantDetails: 0,
		},
		{
			name:        "primary absent with a gap on the current side only is asymmetric",
			tool:        primaryTool,
			curGap:      true,
			wantStatus:  decision.CoverageNotComparable,
			wantDetails: 1,
		},
		{
			name:        "primary absent with a gap on the candidate side only is asymmetric",
			tool:        primaryTool,
			candGap:     true,
			wantStatus:  decision.CoverageNotComparable,
			wantDetails: 1,
		},
		{
			name:        "primary absent with a gap on both sides is a shared gap",
			tool:        primaryTool,
			curGap:      true,
			candGap:     true,
			wantStatus:  decision.CoverageComparableWithGaps,
			wantDetails: 1,
		},
		{
			name:        "a non-primary absent pair never drops out",
			tool:        gapTool,
			wantStatus:  decision.CoverageComparableWithGaps,
			wantDetails: 1,
		},
		// CHANGED from wantStatus: not_comparable. Gap presence is only evidence
		// for a PRIMARY analyzer, where its ABSENCE proves the language is not in
		// the tree. A CoverageGap is emitted solely for tools carrying an install
		// hint, so most non-primary analyzers (scip, scip-symbols, ast-grep) can
		// never raise one however loudly the config asked for them; reading a
		// missing gap as "this configuration did not expect the analyzer" made a
		// presentation table stand in for evidence. Both sides are plainly absent
		// and equally blind, which is comparable-with-the-loss-reported.
		{
			name:        "a non-primary absent pair gapped on one side only is still a shared gap",
			tool:        gapTool,
			candGap:     true,
			wantStatus:  decision.CoverageComparableWithGaps,
			wantDetails: 1,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var curGaps, candGaps []diagnostic.CoverageGap
			if tt.curGap {
				curGaps = []diagnostic.CoverageGap{gap(tt.tool)}
			}
			if tt.candGap {
				candGaps = []diagnostic.CoverageGap{gap(tt.tool)}
			}
			rows := []diagnostic.Coverage{cov(tt.tool, diagnostic.StatusAbsent)}
			got := decision.CompareConfigs(decision.ConfigCompareInput{
				Current:   side(rows, curGaps),
				Candidate: side(rows, candGaps),
			}).Coverage
			if got.Status != tt.wantStatus {
				t.Errorf("status = %q, want %q", got.Status, tt.wantStatus)
			}
			if len(got.Details) != tt.wantDetails {
				t.Errorf("details = %+v, want %d", got.Details, tt.wantDetails)
			}
		})
	}
}

// TestCompareCoverage_PrimaryToolsUnknown pins the conservative fallback: with
// no declared primary analyzers, an absent pair is reported as a gap rather than
// assumed to be an inapplicable language.
func TestCompareCoverage_PrimaryToolsUnknown(t *testing.T) {
	bare := decision.ConfigCompareSide{
		Diag: diagnostic.Diagnostic{
			Findings:     []finding.Finding{},
			ToolCoverage: []diagnostic.Coverage{cov(primaryTool, diagnostic.StatusAbsent)},
		},
		Score: measuredCard(70),
	}
	got := decision.CompareConfigs(decision.ConfigCompareInput{Current: bare, Candidate: bare}).Coverage
	if got.Status != decision.CoverageComparableWithGaps {
		t.Errorf("status = %q, want %q", got.Status, decision.CoverageComparableWithGaps)
	}
}

// TestCompareCoverage_Aggregate pins the union, the worst-grade priority, and
// the stable detail order.
func TestCompareCoverage_Aggregate(t *testing.T) {
	current := side([]diagnostic.Coverage{
		cov("zeta", diagnostic.StatusDisabled),
		cov(primaryTool, diagnostic.StatusOK),
		cov("alpha", diagnostic.StatusOK),
	}, nil)
	candidate := side([]diagnostic.Coverage{
		cov("zeta", diagnostic.StatusDisabled),
		cov(primaryTool, diagnostic.StatusOK),
		cov("alpha", diagnostic.StatusPartial),
	}, nil)

	got := decision.CompareConfigs(decision.ConfigCompareInput{Current: current, Candidate: candidate}).Coverage

	// alpha is not_comparable and zeta is comparable_with_gaps: the worst wins.
	if got.Status != decision.CoverageNotComparable {
		t.Errorf("status = %q, want %q", got.Status, decision.CoverageNotComparable)
	}
	wantTools := []string{"alpha", "zeta"}
	gotTools := make([]string, 0, len(got.Details))
	for _, d := range got.Details {
		gotTools = append(gotTools, d.Tool)
	}
	if !slices.Equal(gotTools, wantTools) {
		t.Errorf("detail tools = %v, want %v (sorted, ok rows omitted)", gotTools, wantTools)
	}
	if got.Details[0].Status != decision.CoverageNotComparable {
		t.Errorf("alpha status = %q, want %q", got.Details[0].Status, decision.CoverageNotComparable)
	}
	if got.Details[1].Status != decision.CoverageComparableWithGaps {
		t.Errorf("zeta status = %q, want %q", got.Details[1].Status, decision.CoverageComparableWithGaps)
	}
}

// TestCompareCoverage_UnknownStatus pins that an unrecognised status abstains
// instead of being treated like a clean run.
func TestCompareCoverage_UnknownStatus(t *testing.T) {
	rows := []diagnostic.Coverage{cov(gapTool, "some-future-status")}
	got := decision.CompareConfigs(decision.ConfigCompareInput{
		Current:   side(rows, nil),
		Candidate: side(rows, nil),
	}).Coverage
	if got.Status != decision.CoverageNotComparable {
		t.Errorf("status = %q, want %q", got.Status, decision.CoverageNotComparable)
	}
}

// ---------------------------------------------------------------------------
// Score delta
// ---------------------------------------------------------------------------

func TestCompareConfigs_ScoreDelta(t *testing.T) {
	tests := []struct {
		name               string
		current, candidate score.Scorecard
		wantNil            bool
		want               int
	}{
		{name: "candidate higher", current: measuredCard(60), candidate: measuredCard(75), want: 15},
		{name: "candidate lower", current: measuredCard(75), candidate: measuredCard(60), want: -15},
		{name: "equal", current: measuredCard(70), candidate: measuredCard(70), want: 0},
		{name: "current unmeasured", current: unmeasuredCard(), candidate: measuredCard(70), wantNil: true},
		{name: "candidate unmeasured", current: measuredCard(70), candidate: unmeasuredCard(), wantNil: true},
		{name: "both unmeasured", current: unmeasuredCard(), candidate: unmeasuredCard(), wantNil: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			in := decision.ConfigCompareInput{Current: findingsSide(), Candidate: findingsSide()}
			in.Current.Score = tt.current
			in.Candidate.Score = tt.candidate
			got := decision.CompareConfigs(in).ScoreDelta
			if tt.wantNil {
				if got != nil {
					t.Fatalf("ScoreDelta = %d, want nil for an unmeasured side", *got)
				}
				return
			}
			if got == nil {
				t.Fatalf("ScoreDelta = nil, want %d", tt.want)
			}
			if *got != tt.want {
				t.Errorf("ScoreDelta = %d, want %d", *got, tt.want)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// Measurement-loss warnings
// ---------------------------------------------------------------------------

func TestCompareConfigs_MeasurementWarnings(t *testing.T) {
	tests := []struct {
		name                                 string
		curScored, curAbstained, curExternal int
		candScored, candAbstained, candExt   int
		want                                 []string
	}{
		{
			name:      "identical counts warn about nothing",
			curScored: 10, curAbstained: 4, curExternal: 7,
			candScored: 10, candAbstained: 4, candExt: 7,
			want: []string{},
		},
		{
			name:      "scored share fell while the total grew",
			curScored: 10, curAbstained: 0,
			candScored: 15, candAbstained: 15,
			want: []string{decision.WarnAbstainedEdgesRose, decision.WarnScoredFractionFell},
		},
		{
			name:      "equal share at a different scale does not warn",
			curScored: 5, curAbstained: 5,
			candScored: 10, candAbstained: 10,
			want: []string{decision.WarnAbstainedEdgesRose},
		},
		{
			name:      "all measurement lost",
			curScored: 10, curAbstained: 2,
			candScored: 0, candAbstained: 0,
			want: []string{decision.WarnScoredFractionFell},
		},
		{
			name:      "nothing measured on the current side cannot fall",
			curScored: 0, curAbstained: 0,
			candScored: 0, candAbstained: 0,
			want: []string{},
		},
		{
			// The guard is on the CURRENT denominator alone: a candidate that
			// measured edges where the current config measured none has no share
			// to have fallen from, only more abstention to report.
			name:      "measurement appearing on the candidate side cannot fall",
			curScored: 0, curAbstained: 0,
			candScored: 5, candAbstained: 3,
			want: []string{decision.WarnAbstainedEdgesRose},
		},
		{
			name:      "external edges rose on their own channel",
			curScored: 10, curAbstained: 0, curExternal: 1,
			candScored: 10, candAbstained: 0, candExt: 9,
			want: []string{decision.WarnExternalEdgesRose},
		},
		{
			name:      "external edges fell does not warn",
			curScored: 10, curExternal: 9,
			candScored: 10, candExt: 1,
			want: []string{},
		},
		{
			name:      "abstained fell does not warn",
			curScored: 10, curAbstained: 5,
			candScored: 10, candAbstained: 1,
			want: []string{},
		},
		{
			name:      "all three channels sorted by code",
			curScored: 10, curAbstained: 0, curExternal: 0,
			candScored: 1, candAbstained: 9, candExt: 4,
			want: []string{
				decision.WarnAbstainedEdgesRose,
				decision.WarnExternalEdgesRose,
				decision.WarnScoredFractionFell,
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := decision.CompareConfigs(decision.ConfigCompareInput{
				Current:   edgesSide(tt.curScored, tt.curAbstained, tt.curExternal),
				Candidate: edgesSide(tt.candScored, tt.candAbstained, tt.candExt),
			}).Warnings
			if !slices.Equal(warningCodes(got), tt.want) {
				t.Fatalf("codes = %v, want %v", warningCodes(got), tt.want)
			}
			for _, w := range got {
				if w.Text == "" {
					t.Errorf("warning %q has no text", w.Code)
				}
			}
		})
	}
}

// TestCompareConfigs_WarningsIgnoreScoreDirection pins the guardrail: a
// candidate that scores HIGHER while measuring less still warns.
func TestCompareConfigs_WarningsIgnoreScoreDirection(t *testing.T) {
	current := edgesSide(20, 0, 0)
	current.Score = measuredCard(40)
	candidate := edgesSide(10, 10, 0)
	candidate.Score = measuredCard(95)

	got := decision.CompareConfigs(decision.ConfigCompareInput{Current: current, Candidate: candidate})

	if got.ScoreDelta == nil || *got.ScoreDelta != 55 {
		t.Fatalf("ScoreDelta = %v, want +55", got.ScoreDelta)
	}
	want := []string{decision.WarnAbstainedEdgesRose, decision.WarnScoredFractionFell}
	if !slices.Equal(warningCodes(got.Warnings), want) {
		t.Errorf("codes = %v, want %v despite the higher candidate score", warningCodes(got.Warnings), want)
	}
}

// TestCompareConfigs_WarningsWithoutScoreDelta pins that measurement loss is
// still reported when the candidate cannot be scored at all.
//
// It also pins the deliberate channel split: reclassifying edges as external
// removes them from the scored/abstained denominator, so the share stays flat
// and only WarnExternalEdgesRose catches the loss. Ratio-ing all three counters
// would silence this case.
func TestCompareConfigs_WarningsWithoutScoreDelta(t *testing.T) {
	current := edgesSide(20, 0, 0)
	candidate := edgesSide(2, 0, 18)
	candidate.Score = unmeasuredCard()

	got := decision.CompareConfigs(decision.ConfigCompareInput{Current: current, Candidate: candidate})

	if got.ScoreDelta != nil {
		t.Errorf("ScoreDelta = %d, want nil", *got.ScoreDelta)
	}
	if !slices.Equal(warningCodes(got.Warnings), []string{decision.WarnExternalEdgesRose}) {
		t.Errorf("codes = %v, want [%s]", warningCodes(got.Warnings), decision.WarnExternalEdgesRose)
	}
}

// TestCompareConfigs_NilClassifiedEdges pins that an absent edge summary reads
// as zero counts on both channels rather than panicking or warning falsely.
func TestCompareConfigs_NilClassifiedEdges(t *testing.T) {
	bare := findingsSide()
	if got := decision.CompareConfigs(decision.ConfigCompareInput{Current: bare, Candidate: bare}); len(got.Warnings) != 0 {
		t.Errorf("warnings = %v, want none when neither side classified edges", warningCodes(got.Warnings))
	}
	// Current measured, candidate did not classify at all: total measurement loss.
	got := decision.CompareConfigs(decision.ConfigCompareInput{
		Current:   edgesSide(10, 1, 0),
		Candidate: bare,
	})
	if !slices.Equal(warningCodes(got.Warnings), []string{decision.WarnScoredFractionFell}) {
		t.Errorf("codes = %v, want [%s]", warningCodes(got.Warnings), decision.WarnScoredFractionFell)
	}
}
