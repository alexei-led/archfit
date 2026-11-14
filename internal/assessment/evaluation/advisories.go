package evaluation

import (
	"maps"
	"slices"
	"sort"
	"strconv"
	"strings"

	"github.com/alexei-led/archfit/v3/internal/assessment/finding"
	"github.com/alexei-led/archfit/v3/internal/relationship"
)

const advisoryKind = finding.KindAdvisory
const bcRollupCap = 8

func candidateFindings(candidates []relationship.AdvisoryCandidate) []finding.Finding {
	out := make([]finding.Finding, 0, len(candidates))
	for _, c := range candidates {
		matched := make(map[string]string, len(c.MatchedBy))
		maps.Copy(matched, c.MatchedBy)
		out = append(out, finding.Finding{ID: c.ID, Kind: advisoryKind, RuleID: c.RuleID, Status: finding.StatusNew, Severity: finding.Severity(c.Severity), Edge: finding.EdgeEvidence{From: finding.Endpoint{Module: c.FromModule, Path: c.From}, To: finding.Endpoint{Module: c.ToModule, Path: c.To}, Kind: c.EdgeKind}, Locations: c.Locations, Why: c.Why, MatchedBy: matched})
	}
	return out
}

func groupBCAdvisories(in []finding.Finding) []finding.Finding {
	type key struct {
		from, to, strength, distance, volatility string
		status                                   finding.Status
	}
	groups := map[key][]finding.Finding{}
	keys := make([]key, 0)
	passthrough := make([]finding.Finding, 0)
	for _, f := range in {
		if f.RuleID != finding.RuleIDBCImbalanced || f.Status == finding.StatusFixed {
			passthrough = append(passthrough, f)
			continue
		}
		k := key{f.Edge.From.Module, f.Edge.To.Module, f.MatchedBy["strength"], f.MatchedBy["distance"], f.MatchedBy["volatility"], f.Status}
		if _, ok := groups[k]; !ok {
			keys = append(keys, k)
		}
		groups[k] = append(groups[k], f)
	}
	sort.Slice(keys, func(i, j int) bool {
		a, b := keys[i], keys[j]
		if a.from != b.from {
			return a.from < b.from
		}
		if a.to != b.to {
			return a.to < b.to
		}
		if a.strength != b.strength {
			return a.strength < b.strength
		}
		if a.distance != b.distance {
			return a.distance < b.distance
		}
		if a.volatility != b.volatility {
			return a.volatility < b.volatility
		}
		return a.status < b.status
	})
	out := make([]finding.Finding, 0, len(keys)+len(passthrough))
	for _, k := range keys {
		out = append(out, rollup(groups[k]))
	}
	return append(out, passthrough...)
}
func rollup(members []finding.Finding) finding.Finding {
	sort.Slice(members, func(i, j int) bool { return members[i].ID < members[j].ID })
	rep := members[0]
	matched := make(map[string]string, len(rep.MatchedBy)+2)
	maps.Copy(matched, rep.MatchedBy)
	matched["group_count"] = strconv.Itoa(len(members))
	ids := make([]string, 0, len(members))
	for _, m := range members {
		ids = append(ids, m.ID)
	}
	matched["group_members"] = strings.Join(ids[:min(len(ids), bcRollupCap)], ",")
	rep.Members = ids
	rep.MatchedBy = matched
	seen := map[relationship.Location]struct{}{}
	locs := []relationship.Location{}
	for _, m := range members {
		for _, l := range m.Locations {
			if _, ok := seen[l]; !ok {
				seen[l] = struct{}{}
				locs = append(locs, l)
			}
		}
	}
	sort.Slice(locs, func(i, j int) bool {
		if locs[i].File != locs[j].File {
			return locs[i].File < locs[j].File
		}
		return locs[i].Line < locs[j].Line
	})
	rep.Locations = locs
	rep.Edge.From.Path, rep.Edge.To.Path = groupEdgePaths(members, locs)
	return rep
}

// groupEdgePaths returns honest from/to paths for a rolled-up finding: the pair
// belonging to whichever member owns the FIRST merged location, so the reported
// paths and locations[0] name the same member edge. members arrive sorted by ID
// (rollup sorts), so members[0] is the representative used when no location
// determines an owner — its pair is still a real member edge, and wiping it
// would strip the finding's only path evidence.
func groupEdgePaths(members []finding.Finding, locs []relationship.Location) (fromPath, toPath string) {
	if len(locs) > 0 {
		for _, m := range members {
			if slices.Contains(m.Locations, locs[0]) {
				return m.Edge.From.Path, m.Edge.To.Path
			}
		}
	}
	return members[0].Edge.From.Path, members[0].Edge.To.Path
}
