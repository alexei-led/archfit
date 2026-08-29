package state

import "testing"

var promotionDimensions = []string{
	DimensionIntent,
	DimensionStructure,
	DimensionModularity,
	DimensionCoupling,
	DimensionChangeLocality,
	DimensionComplexity,
	DimensionTestability,
	DimensionOperations,
	DimensionDrift,
}

// TestPromotionIsMonotonic pins the shared evidence rule for all nine fixed
// contracts. Once a dimension is measured, removing any one in-claim fact can
// only lower it to partial and must name exactly the fact that was removed.
func TestPromotionIsMonotonic(t *testing.T) {
	t.Parallel()
	for _, dimension := range promotionDimensions {
		dimension := dimension
		t.Run(dimension, func(t *testing.T) {
			t.Parallel()
			facts := RequiredFacts(dimension)
			observed := inClaimFactNames(facts)
			if len(observed) < 2 {
				t.Fatalf("%s has %d in-claim facts, want at least two for the monotonic-removal invariant", dimension, len(observed))
			}

			status, unknown := Promote(dimension, observed, nil)
			if status != Measured || len(unknown) != 0 {
				t.Fatalf("full fact set = %q, unknown=%+v; want measured with no in-claim unknown", status, unknown)
			}
			for i, removed := range observed {
				remaining := append([]string(nil), observed[:i]...)
				remaining = append(remaining, observed[i+1:]...)
				status, unknown = Promote(dimension, remaining, nil)
				if status != Partial {
					t.Errorf("without %q status = %q, want partial", removed, status)
				}
				if len(unknown) != 1 || unknown[0].Fact != removed {
					t.Errorf("without %q unknown = %+v, want that fact alone", removed, unknown)
				}
			}

			status, unknown = Promote(dimension, nil, nil)
			if status != Unmeasured {
				t.Errorf("empty observed set status = %q, want unmeasured", status)
			}
			if len(unknown) != len(observed) {
				t.Errorf("empty observed set names %d facts, want all %d", len(unknown), len(observed))
			}

			status, unknown = Promote(dimension, nil, observed)
			if status != Measured || len(unknown) != 0 {
				t.Errorf("all facts producer-proved not applicable = %q, unknown=%+v; want measured", status, unknown)
			}
		})
	}
}

// TestNoMeasuredDimensionHasInClaimUnknowns pins both sides of the lenient/
// strict rule: all in-claim facts are closed before promotion, while every
// declared out-of-claim fact may still be disclosed on a measured envelope.
func TestNoMeasuredDimensionHasInClaimUnknowns(t *testing.T) {
	t.Parallel()
	for _, dimension := range promotionDimensions {
		dimension := dimension
		t.Run(dimension, func(t *testing.T) {
			t.Parallel()
			facts := RequiredFacts(dimension)
			status, missing := Promote(dimension, inClaimFactNames(facts), nil)
			if status != Measured || len(missing) != 0 {
				t.Fatalf("complete %s contract = %q, missing=%+v", dimension, status, missing)
			}
			inClaim := make(map[string]struct{})
			for _, fact := range facts {
				if fact.Name == "" || fact.Owner == "" {
					t.Errorf("%s carries incomplete required fact %+v", dimension, fact)
				}
				if fact.InClaim {
					inClaim[fact.Name] = struct{}{}
				}
			}
			for _, fact := range facts {
				if fact.InClaim {
					continue
				}
				if _, claimed := inClaim[fact.Name]; claimed {
					t.Errorf("measured %s unknown %q is also declared in claim", dimension, fact.Name)
				}
			}
		})
	}
}

func inClaimFactNames(facts []RequiredFact) []string {
	out := make([]string, 0, len(facts))
	for _, fact := range facts {
		if fact.InClaim {
			out = append(out, fact.Name)
		}
	}
	return out
}
