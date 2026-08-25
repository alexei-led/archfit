package evaluation

import (
	"crypto/sha256"
	"encoding/hex"
	"maps"
	"sort"
	"strconv"
	"strings"

	"github.com/alexei-led/archfit/internal/assessment/finding"
	"github.com/alexei-led/archfit/internal/relationship"
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

func staleLabelFindings(keys []string) []finding.Finding {
	out := make([]finding.Finding, 0, len(keys))
	for _, key := range keys {
		from, to, ok := strings.Cut(key, "\x00")
		if !ok {
			continue
		}
		out = append(out, finding.Finding{ID: staleLabelID(from, to), Kind: advisoryKind, RuleID: "labels/stale", Status: finding.StatusNew, Severity: finding.SeverityLow, Edge: finding.EdgeEvidence{From: finding.Endpoint{Module: from}, To: finding.Endpoint{Module: to}}, Why: "pinned label evidence is stale: the " + from + " -> " + to + " dependency surface changed since approval; label ignored — re-run `archfit enrich` and re-review"})
	}
	return out
}

func resolveEvidence(s relationship.Set, findings []finding.Finding) []finding.Finding {
	out := make([]finding.Finding, 0, len(findings))
	for _, f := range findings {
		if edge, ok := s.FindByFindingEdge(f.Edge.From.Path, f.Edge.To.Path, f.Edge.Kind); ok {
			f.Edge.From.Module, f.Edge.To.Module = edge.FromModule, edge.ToModule
			f.Severity = severityFor(edge.Strength, edge.Distance)
		}
		out = append(out, f)
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
		if f.RuleID != finding.RuleIDBCImbalanced {
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
	ids := make([]string, 0, bcRollupCap)
	for i, m := range members {
		if i == bcRollupCap {
			break
		}
		ids = append(ids, m.ID)
	}
	matched["group_members"] = strings.Join(ids, ",")
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
	if len(locs) > 0 {
		for _, m := range members {
			for _, l := range m.Locations {
				if l == locs[0] {
					rep.Edge.From.Path = m.Edge.From.Path
					rep.Edge.To.Path = m.Edge.To.Path
					break
				}
			}
		}
	}
	return rep
}
func staleLabelID(from, to string) string { return fingerprint("labels/stale", from+"\x00"+to) }
func fingerprint(rule, subject string) string {
	h := sha256.Sum256([]byte(rule + "\x00" + subject))
	return hex.EncodeToString(h[:16])
}
