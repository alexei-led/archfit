package engine

import (
	"crypto/sha256"
	"encoding/hex"
	"maps"
	"sort"
	"strconv"
	"strings"

	"github.com/alexei-led/archfit/internal/config"
	"github.com/alexei-led/archfit/internal/model/coupling"
	"github.com/alexei-led/archfit/internal/model/finding"
	"github.com/alexei-led/archfit/internal/model/graph"
	"github.com/alexei-led/archfit/internal/staleness"
	"github.com/alexei-led/archfit/internal/status"
)

// kindAdvisory is the finding kind for non-gating advisory findings.
const kindAdvisory = "advisory"

// collectAdvisories runs stage 8: coupling advisories, staleness advisories,
// stale label advisories, and the advisory status pass.
// staleLabelFnds is the slice produced by stage 3 (applyPinnedLabels).
func collectAdvisories(g *graph.Graph, couplingIdx coupling.Index, classifyCfg config.ClassifyConfig, staleLabelFnds []finding.Finding, in RunInput) []finding.Finding {
	mm := classifyCfg.ModuleMap

	var advisoryFindings []finding.Finding
	for _, e := range g.Edges() {
		key := e.From + "\x00" + e.To + "\x00" + string(e.Kind)
		cl, ok := couplingIdx[key]
		if !ok || cl.Severity == coupling.SeverityNone {
			continue
		}
		if !severityAtLeast(cl.Severity, classifyCfg.BCAdvisoryMinSeverity) {
			continue
		}
		fromPath := stripPrefix(e.From)
		toPath := stripPrefix(e.To)
		fromModule, _ := mm.ModuleFor(fromPath)
		toModule, _ := mm.ModuleFor(toPath)
		id := couplingAdvisoryID(fromPath, toPath, string(e.Kind))
		matched := map[string]string{
			"strength":   string(cl.Strength),
			"distance":   string(cl.Distance),
			"volatility": string(cl.Volatility),
		}
		// Attach continuous score fields when a scorer produced them.
		// score = scorer name; score_value = integer 0-10; score_band = severity band.
		if cl.Score.Reason != "" {
			matched["score"] = cl.Score.Reason
			matched["score_value"] = strconv.Itoa(cl.Score.Value)
			matched["score_band"] = string(cl.Score.Band)
			matched["score_version"] = coupling.ScoreVersion
		}
		if cl.Score.CheapestMove != "" {
			matched["cheapest_move"] = cl.Score.CheapestMove
		}
		af := finding.Finding{
			ID:       id,
			Kind:     kindAdvisory,
			RuleID:   "bc/imbalanced_coupling",
			Status:   finding.StatusNew,
			Severity: finding.Severity(cl.Severity),
			Edge: finding.EdgeEvidence{
				From: finding.Endpoint{Module: fromModule, Path: fromPath},
				To:   finding.Endpoint{Module: toModule, Path: toPath},
				Kind: string(e.Kind),
			},
			Locations: e.Locations,
			Why:       bcAdvisoryWhy(cl),
			MatchedBy: matched,
		}
		advisoryFindings = append(advisoryFindings, af)
	}
	// Append staleness advisories.
	advisoryFindings = append(advisoryFindings, staleness.Check(g, in.Staleness, in.Now)...)

	// Stale pinned labels were ignored during classification; advise re-enrich.
	advisoryFindings = append(advisoryFindings, staleLabelFnds...)

	// Apply baseline and exception status to advisory findings.
	// status.Assign also emits fixed gate findings; suppress those for the advisory pass
	// by discarding any synthetic kind=="gate" entries it appends.
	tagged := status.Assign(advisoryFindings, in.Accepted, in.Exceptions, in.Now, "advisory")
	advisoryFindings = advisoryFindings[:0]
	for _, f := range tagged {
		if f.Kind == kindAdvisory {
			advisoryFindings = append(advisoryFindings, f)
		}
	}
	// Collapse the BC-advisory flood: edges sharing the same coupling shape between
	// the same module pair roll up into one advisory with a count + representative IDs.
	return groupBCAdvisories(advisoryFindings)
}

// bcAdvisoryRollupCap bounds the number of per-edge member IDs stored on a grouped
// advisory's MatchedBy["group_members"]. group_count carries the true total; this cap
// keeps a 400-edge flood from bloating the output with one ID per edge.
const bcAdvisoryRollupCap = 8

// groupBCAdvisories collapses bc/imbalanced_coupling advisories that share the same
// shape — (fromModule, toModule, strength, distance, volatility, status) — into one
// rollup finding carrying a count and representative member IDs. Non-BC advisories
// (staleness, labels/stale) pass through unchanged. Ordering is deterministic: rollups
// are emitted in sorted key order, then the untouched non-BC advisories in input order.
//
// Status is part of the key so a baseline-suppressed or excepted edge is never folded
// into a "new" rollup's count. Cohesion (high strength + low distance) never reaches
// this pass — coupling.BalanceResult returns SeverityNone for it — so a rollup, like an
// individual advisory, never flags cohesion ("the good coupling") as a problem.
func groupBCAdvisories(advisories []finding.Finding) []finding.Finding {
	type groupKey struct {
		fromModule, toModule           string
		strength, distance, volatility string
		status                         finding.Status
	}
	groups := make(map[groupKey][]finding.Finding)
	keys := make([]groupKey, 0)
	var passthrough []finding.Finding

	for _, f := range advisories {
		if f.RuleID != "bc/imbalanced_coupling" {
			passthrough = append(passthrough, f)
			continue
		}
		k := groupKey{
			fromModule: f.Edge.From.Module,
			toModule:   f.Edge.To.Module,
			strength:   f.MatchedBy["strength"],
			distance:   f.MatchedBy["distance"],
			volatility: f.MatchedBy["volatility"],
			status:     f.Status,
		}
		if _, ok := groups[k]; !ok {
			keys = append(keys, k)
		}
		groups[k] = append(groups[k], f)
	}

	sort.Slice(keys, func(i, j int) bool {
		a, b := keys[i], keys[j]
		switch {
		case a.fromModule != b.fromModule:
			return a.fromModule < b.fromModule
		case a.toModule != b.toModule:
			return a.toModule < b.toModule
		case a.strength != b.strength:
			return a.strength < b.strength
		case a.distance != b.distance:
			return a.distance < b.distance
		case a.volatility != b.volatility:
			return a.volatility < b.volatility
		default:
			return a.status < b.status
		}
	})

	out := make([]finding.Finding, 0, len(keys)+len(passthrough))
	for _, k := range keys {
		out = append(out, rollupFinding(groups[k]))
	}
	out = append(out, passthrough...)
	return out
}

// rollupFinding builds one grouped advisory from same-shape members. The member with
// the smallest ID is the representative (its edge, why, severity, and score all apply
// to every member, since they share the group key); group_count and up to
// bcAdvisoryRollupCap sorted member IDs are added to a cloned MatchedBy, and all member
// locations are merged so no file:line evidence is lost.
func rollupFinding(members []finding.Finding) finding.Finding {
	sort.Slice(members, func(i, j int) bool { return members[i].ID < members[j].ID })
	rep := members[0]

	matched := make(map[string]string, len(rep.MatchedBy)+2)
	maps.Copy(matched, rep.MatchedBy)
	matched["group_count"] = strconv.Itoa(len(members))
	ids := make([]string, 0, bcAdvisoryRollupCap)
	for i, m := range members {
		if i == bcAdvisoryRollupCap {
			break
		}
		ids = append(ids, m.ID)
	}
	matched["group_members"] = strings.Join(ids, ",")

	rep.MatchedBy = matched
	rep.Locations = mergeLocations(members)
	return rep
}

// mergeLocations returns the deduplicated, sorted union of all members' locations.
func mergeLocations(members []finding.Finding) []graph.Location {
	seen := make(map[graph.Location]struct{})
	var locs []graph.Location
	for _, m := range members {
		for _, l := range m.Locations {
			if _, ok := seen[l]; ok {
				continue
			}
			seen[l] = struct{}{}
			locs = append(locs, l)
		}
	}
	sort.Slice(locs, func(i, j int) bool {
		if locs[i].File != locs[j].File {
			return locs[i].File < locs[j].File
		}
		return locs[i].Line < locs[j].Line
	})
	return locs
}

// severityFor maps coupling classification to finding severity.
// Intrusive coupling that crosses module boundaries → high; otherwise medium.
func severityFor(strength, distance string) finding.Severity {
	if strength == "intrusive" &&
		distance != "same_module" &&
		distance != "unknown" &&
		distance != "" {
		return finding.SeverityHigh
	}
	return finding.SeverityMedium
}

// severityAtLeast reports whether got meets or exceeds the threshold.
// Empty threshold means no filter (all severities pass). Order: low < medium < high < critical.
func severityAtLeast(got coupling.Severity, threshold string) bool {
	if threshold == "" {
		return true
	}
	rank := map[string]int{"low": 1, "medium": 2, "high": 3, "critical": 4}
	return rank[string(got)] >= rank[threshold]
}

// bcAdvisoryWhy builds a concise BC-vocabulary why string for a coupling advisory.
// It uses strength, distance, and volatility to produce a human-readable explanation
// following Balanced Coupling vocabulary (integration strength, distance, volatility),
// and names the resulting risk in Vlad Khononov's terms.
func bcAdvisoryWhy(cl coupling.Classification) string {
	return "balanced coupling: " + string(cl.Strength) + " integration strength" +
		" × " + string(cl.Distance) + " distance" +
		" × " + string(cl.Volatility) + " volatility" +
		" → " + string(cl.Severity) + " severity (" + bcRiskClause(cl.Severity) + ")"
}

// bcRiskClause names the Balanced Coupling risk for an advisory severity in Vlad
// Khononov's vocabulary. The worst case (critical = high strength + high distance +
// high volatility) is cascading changes across a knowledge boundary — a distributed-
// monolith risk; lesser bands are elevated maintenance effort. Cohesion (high strength
// + low distance) never reaches here — coupling.BalanceResult returns SeverityNone for
// it — so it is never framed as a problem.
func bcRiskClause(sev coupling.Severity) string {
	switch sev {
	case coupling.SeverityCritical:
		return "cascading changes across a knowledge boundary → distributed-monolith risk"
	case coupling.SeverityHigh:
		return "tight coupling across a volatile boundary → likely cascading changes"
	default:
		return "unbalanced coupling → elevated maintenance effort"
	}
}

// couplingAdvisoryID returns a stable 32-character hex fingerprint for a coupling advisory
// finding, derived from (from, to, kind) — same scheme as finding.fingerprint.
func couplingAdvisoryID(from, to, kind string) string {
	h := sha256.Sum256([]byte("bc/imbalanced_coupling\x00" + from + "\x00" + to + "\x00" + kind))
	return hex.EncodeToString(h[:16])
}

// staleLabelID returns a stable fingerprint for a labels/stale advisory.
func staleLabelID(from, to string) string {
	h := sha256.Sum256([]byte("labels/stale\x00" + from + "\x00" + to))
	return hex.EncodeToString(h[:16])
}
