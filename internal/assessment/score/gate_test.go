package score

import (
	"strings"
	"testing"

	"github.com/alexei-led/archfit/internal/assessment/result"
)

// seam builds one ledger entry. Only the fields the gate is allowed to read are
// meaningful: the gate must decide from the qualification flag and identity, not
// from a score.
func seam(id, from, to string, distributed bool) result.Seam {
	return result.Seam{ID: id, FromModule: from, ToModule: to, DistributedMonolith: distributed,
		Strength: "intrusive", Distance: "cross_deploy_unit", CriticalEdges: 1, ScoredEdges: 1}
}

func refWith(ids ...string) SeamReference {
	set := make(map[string]struct{}, len(ids))
	for _, id := range ids {
		set[id] = struct{}{}
	}
	return SeamReference{Comparable: true, QualifyingSeamIDs: set}
}

// TestEvaluateSeamGateSelectsQualifyingSeams pins what the gate counts: seams
// carrying the qualification, in stable ID order, in both modes.
func TestEvaluateSeamGateSelectsQualifyingSeams(t *testing.T) {
	seams := []result.Seam{
		seam("s2", "b", "c", true),
		seam("s1", "a", "b", true),
		seam("s3", "c", "d", false),
	}

	got := EvaluateSeamGate(seams, SeamGate{Mode: SeamGateWarn}, SeamReference{})
	if len(got.Qualifying) != 2 {
		t.Fatalf("qualifying = %d, want 2 — only seams carrying the condition count", len(got.Qualifying))
	}
	if got.Qualifying[0].ID != "s1" || got.Qualifying[1].ID != "s2" {
		t.Errorf("qualifying order = %s,%s, want stable seam-ID order s1,s2",
			got.Qualifying[0].ID, got.Qualifying[1].ID)
	}
}

// TestEvaluateSeamGateAbstainsWithoutComparableReference pins the honest
// abstention: without a comparable reference the seam total is still reported,
// but no "newly introduced" claim is made and nothing blocks.
func TestEvaluateSeamGateAbstainsWithoutComparableReference(t *testing.T) {
	seams := []result.Seam{seam("s1", "a", "b", true)}

	for _, mode := range []string{SeamGateWarn, SeamGateFail} {
		t.Run(mode, func(t *testing.T) {
			got := EvaluateSeamGate(seams, SeamGate{Mode: mode},
				SeamReference{Reasons: []string{"legacy_score_snapshot_ignored"}})

			if got.Rated {
				t.Error("rated = true without a comparable reference")
			}
			if got.Blocked {
				t.Error("blocked = true without a comparable reference — an unrated gate never blocks")
			}
			if len(got.New) != 0 {
				t.Errorf("new = %d, want 0: 'newly introduced' is not a claim this run can make", len(got.New))
			}
			if len(got.Qualifying) != 1 {
				t.Errorf("qualifying = %d, want 1 — the seam total is reported even unrated", len(got.Qualifying))
			}
			if !strings.Contains(strings.Join(got.Reasons, "; "), "legacy_score_snapshot_ignored") {
				t.Errorf("reasons = %v, want the reference's own non-comparability reason disclosed", got.Reasons)
			}
		})
	}
}

// TestEvaluateSeamGateModeAndTolerance pins the block condition: fail mode, a
// comparable reference, and more new seams than the tolerance. Removing any one
// of the three must not block.
func TestEvaluateSeamGateModeAndTolerance(t *testing.T) {
	seams := []result.Seam{seam("s1", "a", "b", true), seam("s2", "b", "c", true)}

	tests := []struct {
		name        string
		gate        SeamGate
		ref         SeamReference
		wantNew     int
		wantBlocked bool
	}{
		{
			name: "warn mode reports new seams without blocking",
			gate: SeamGate{Mode: SeamGateWarn}, ref: refWith(), wantNew: 2,
		},
		{
			name: "fail mode blocks on a new seam past the tolerance",
			gate: SeamGate{Mode: SeamGateFail}, ref: refWith("s1"), wantNew: 1, wantBlocked: true,
		},
		{
			name: "fail mode tolerates up to max_new_seams",
			gate: SeamGate{Mode: SeamGateFail, MaxNewSeams: 1}, ref: refWith("s1"), wantNew: 1,
		},
		{
			name: "a pre-existing seam is not newly introduced",
			gate: SeamGate{Mode: SeamGateFail}, ref: refWith("s1", "s2"), wantNew: 0,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := EvaluateSeamGate(seams, tc.gate, tc.ref)
			if !got.Rated {
				t.Fatal("rated = false with a comparable reference")
			}
			if len(got.New) != tc.wantNew {
				t.Errorf("new = %d, want %d", len(got.New), tc.wantNew)
			}
			if got.Blocked != tc.wantBlocked {
				t.Errorf("blocked = %v, want %v (reasons %v)", got.Blocked, tc.wantBlocked, got.Reasons)
			}
			if tc.wantBlocked && len(got.Reasons) == 0 {
				t.Error("blocked with no reason — a blocking gate must say what it blocked on")
			}
		})
	}
}

// TestEvaluateSeamGateNamesTheSeamsItBlocksOn pins that a trip is actionable:
// the reasons carry the module pair, not just a count.
func TestEvaluateSeamGateNamesTheSeamsItBlocksOn(t *testing.T) {
	got := EvaluateSeamGate([]result.Seam{seam("s1", "billing", "shipping", true)},
		SeamGate{Mode: SeamGateFail}, refWith())

	joined := strings.Join(got.Reasons, "; ")
	if !strings.Contains(joined, "billing -> shipping") {
		t.Errorf("reasons = %q, want the offending module pair named", joined)
	}
	if !strings.Contains(joined, "max_new_seams") {
		t.Errorf("reasons = %q, want the knob that produced the block named", joined)
	}
}

// TestEvaluateSeamGateStaysSilentWithNothingToSay pins the disclosure rule: a
// repository with no qualifying seam has no claim to withhold, so an unrated
// gate emits no reason at all. Printing an abstention on every clean run trains
// readers to ignore the line that matters.
func TestEvaluateSeamGateStaysSilentWithNothingToSay(t *testing.T) {
	got := EvaluateSeamGate([]result.Seam{seam("s1", "a", "b", false)},
		SeamGate{Mode: SeamGateFail}, SeamReference{Reasons: []string{"legacy baseline"}})

	if len(got.Reasons) != 0 {
		t.Errorf("reasons = %v, want none when no seam qualifies", got.Reasons)
	}
	if got.Blocked || got.Rated {
		t.Errorf("blocked/rated = %v/%v, want false/false", got.Blocked, got.Rated)
	}
}
