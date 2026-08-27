package relationship_test

import (
	"testing"

	"github.com/alexei-led/archfit/internal/relationship"
)

// TestSeamIDIsStableAndOrdered pins the frozen seam identity. A seam ID must be
// reproducible across runs and must distinguish direction: a -> b and b -> a
// are two different seams with two different balancing hypotheses.
func TestSeamIDIsStableAndOrdered(t *testing.T) {
	forward := relationship.SeamID("alpha", "beta")
	// The digest itself, not just its shape. Every property below survives a
	// change of domain constant or separator — the ID would still be
	// deterministic, still 64 chars, still direction-sensitive — while every
	// stored seam comparison silently re-keys and a comparable baseline starts
	// reporting each existing seam as newly introduced.
	const wantForward = "9e775cc4ceb8ef47f7be35d44ea76fd8313096faac87628db147e0644fe3f2f5"
	if forward != wantForward {
		t.Errorf("SeamID(alpha, beta) = %q, want %q — the frozen preimage is "+
			`sha256("seam.v1\x00" + from + "\x00" + to)`, forward, wantForward)
	}
	if forward != relationship.SeamID("alpha", "beta") {
		t.Error("SeamID is not deterministic for identical modules")
	}
	if len(forward) != 64 {
		t.Errorf("SeamID length = %d, want a 64-char sha256 hex digest", len(forward))
	}
	if reverse := relationship.SeamID("beta", "alpha"); forward == reverse {
		t.Error("SeamID collapses direction: a -> b and b -> a must differ")
	}
	// The separator must not be forgeable by module names: "a\x00b" would
	// otherwise key the same seam as ("a", "b").
	if relationship.SeamID("alpha\x00beta", "") == forward {
		t.Error("SeamID separator is forgeable from module names")
	}
}

// TestSeamScoresNearestRank pins the frozen percentile rule: ceil(p*n)-1 over
// the ascending sort, with p10/p90 abstaining below ten samples.
func TestSeamScoresNearestRank(t *testing.T) {
	tests := []struct {
		name       string
		scores     []int
		wantN      int
		wantMin    int
		wantMedian int
		wantMax    int
		wantMean   float64
		wantP10    *int
		wantP90    *int
	}{
		{
			name: "empty scores measure nothing",
		},
		{
			name: "single sample abstains from deciles",
			// Unsorted input: the distribution sorts before ranking.
			scores: []int{4}, wantN: 1, wantMin: 4, wantMedian: 4, wantMax: 4, wantMean: 4,
		},
		{
			name:   "nine samples still abstain from deciles",
			scores: []int{9, 1, 8, 2, 7, 3, 6, 4, 5},
			wantN:  9, wantMin: 1, wantMedian: 5, wantMax: 9, wantMean: 5,
		},
		{
			name:   "ten samples report deciles at ceil(p*n)-1",
			scores: []int{10, 1, 9, 2, 8, 3, 7, 4, 6, 5},
			wantN:  10, wantMin: 1, wantMedian: 5, wantMax: 10, wantMean: 5.5,
			wantP10: intp(1), wantP90: intp(9),
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := relationship.SeamScores(tc.scores)
			if got.N != tc.wantN || got.Min != tc.wantMin || got.Median != tc.wantMedian || got.Max != tc.wantMax {
				t.Errorf("n/min/median/max = %d/%d/%d/%d, want %d/%d/%d/%d",
					got.N, got.Min, got.Median, got.Max, tc.wantN, tc.wantMin, tc.wantMedian, tc.wantMax)
			}
			if got.Mean != tc.wantMean {
				t.Errorf("mean = %v, want %v", got.Mean, tc.wantMean)
			}
			assertDecile(t, "p10", got.P10, tc.wantP10)
			assertDecile(t, "p90", got.P90, tc.wantP90)
		})
	}
}

func assertDecile(t *testing.T, name string, got, want *int) {
	t.Helper()
	switch {
	case got == nil && want == nil:
	case got == nil || want == nil:
		t.Errorf("%s = %v, want %v (nil below ten samples, a value at or above)", name, deref(got), deref(want))
	case *got != *want:
		t.Errorf("%s = %d, want %d", name, *got, *want)
	}
}

func deref(p *int) any {
	if p == nil {
		return nil
	}
	return *p
}

func intp(v int) *int { return &v }
