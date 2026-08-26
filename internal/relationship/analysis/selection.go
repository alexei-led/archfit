package analysis

import (
	"sort"
	"strings"

	"github.com/alexei-led/archfit/internal/relationship"
	"github.com/alexei-led/archfit/internal/relationship/labels"
)

// maxSelectionSamples caps the sample paths carried per reviewed module pair.
const maxSelectionSamples = 5

// RefinablePair is a data-only candidate for semantic coupling-label review.
type RefinablePair struct {
	From, To    string
	Strength    string
	EdgeCount   int
	SamplePaths []string
}

// AbstainedSample identifies one source location supporting an abstained edge.
type AbstainedSample struct {
	FromPath, ToPath string
	File             string
	Line             int
}

// AbstainedPair groups abstained edges by ordered module pair.
type AbstainedPair struct {
	From, To  string
	EdgeCount int
	Samples   []AbstainedSample
}

// RefinablePairs selects unresolved or heuristic-strength module pairs for
// semantic review. Relationship interpretation stays here; callers receive
// prompt-ready data.
//
// It walks EVERY edge kind, matching classify.Run and PairEvidence. Narrowing to
// dependency kinds hides the pairs whose only edges are `belongs_to` (the Rust
// cargo-modules containment edges): they are still classified, still scored, and
// still depress coupling_balance, so they must stay labelable.
func RefinablePairs(set relationship.Set, approved map[string]struct{}) []RefinablePair {
	type aggregate struct {
		strength string
		count    int
		samples  []string
	}
	pairs := map[string]*aggregate{}
	for _, edge := range set.Edges {
		if edge.Strength != relationship.StrengthFunctional && edge.Strength != relationship.StrengthModel && edge.Strength != relationship.StrengthUnknown {
			continue
		}
		key, ok := reviewableKey(edge, approved)
		if !ok {
			continue
		}
		a := pairs[key]
		if a == nil {
			a = &aggregate{strength: string(edge.Strength)}
			pairs[key] = a
		}
		a.count++
		if len(a.samples) < maxSelectionSamples {
			a.samples = append(a.samples, edge.FromPath+" -> "+edge.ToPath)
		}
	}
	out := make([]RefinablePair, 0, len(pairs))
	for key, a := range pairs {
		from, to, _ := strings.Cut(key, "\x00")
		sort.Strings(a.samples)
		out = append(out, RefinablePair{From: from, To: to, Strength: a.strength, EdgeCount: a.count, SamplePaths: a.samples})
	}
	sortByPair(out, func(p RefinablePair) (string, string) { return p.From, p.To })
	return out
}

// AbstainedPairs selects unknown-strength cross-module edges for review and
// reports the total number of abstained edges, including those beyond edgeCap.
// It walks every edge kind for the same reason RefinablePairs does.
func AbstainedPairs(set relationship.Set, approved map[string]struct{}, edgeCap, sampleCap int) (pairs []AbstainedPair, total int) {
	byKey := map[string]*AbstainedPair{}
	included := 0
	for _, edge := range set.Edges {
		if edge.Strength != relationship.StrengthUnknown {
			continue
		}
		key, ok := reviewableKey(edge, approved)
		if !ok {
			continue
		}
		total++
		if included >= edgeCap {
			continue
		}
		included++
		p := byKey[key]
		if p == nil {
			p = &AbstainedPair{From: edge.FromModule, To: edge.ToModule}
			byKey[key] = p
		}
		p.EdgeCount++
		if len(p.Samples) < sampleCap {
			s := AbstainedSample{FromPath: edge.FromPath, ToPath: edge.ToPath}
			if len(edge.Locations) > 0 {
				s.File, s.Line = edge.Locations[0].File, edge.Locations[0].Line
			}
			p.Samples = append(p.Samples, s)
		}
	}
	pairs = make([]AbstainedPair, 0, len(byKey))
	for _, p := range byKey {
		pairs = append(pairs, *p)
	}
	sortByPair(pairs, func(p AbstainedPair) (string, string) { return p.From, p.To })
	return pairs, total
}

// PairEvidence computes the current evidence hash per module pair (keyed by
// labels.Key): a hash over "fromPath\x00toPath\x00kind" for EVERY edge whose
// endpoints resolve to that ordered pair. Only pairs in wanted are hashed, so
// enrich stamps drafts with exactly the hash analysis later verifies.
//
// The edge set must stay identical to the one pairEvidence hashes during
// analysis. Narrowing either side (to dependency kinds, say) gives a pair
// carrying a `belongs_to` edge two different hashes, and every label approved
// for it then reads permanently stale.
func PairEvidence(set relationship.Set, wanted map[string]struct{}) map[string]string {
	if len(wanted) == 0 {
		return nil
	}
	items := map[string][]string{}
	for _, edge := range set.Edges {
		if edge.FromModule == "" || edge.ToModule == "" || edge.FromModule == edge.ToModule {
			continue
		}
		key := labels.Key(edge.FromModule, edge.ToModule)
		if _, ok := wanted[key]; !ok {
			continue
		}
		items[key] = append(items[key], edge.FromPath+"\x00"+edge.ToPath+"\x00"+edge.Kind)
	}
	evidence := make(map[string]string, len(items))
	for key, its := range items {
		evidence[key] = labels.HashItems(its)
	}
	return evidence
}

// reviewableKey returns the label key for a cross-module edge that still needs
// review, or false when the edge is same-module, unresolved, or already approved.
func reviewableKey(edge relationship.Edge, approved map[string]struct{}) (string, bool) {
	if edge.Distance == relationship.DistanceSameModule || edge.Distance == relationship.DistanceUnknown || edge.Distance == "" {
		return "", false
	}
	if edge.FromModule == "" || edge.ToModule == "" || edge.FromModule == edge.ToModule {
		return "", false
	}
	key := labels.Key(edge.FromModule, edge.ToModule)
	if _, ok := approved[key]; ok {
		return "", false
	}
	return key, true
}

func sortByPair[T any](in []T, key func(T) (string, string)) {
	sort.Slice(in, func(i, j int) bool {
		fromI, toI := key(in[i])
		fromJ, toJ := key(in[j])
		if fromI != fromJ {
			return fromI < fromJ
		}
		return toI < toJ
	})
}
