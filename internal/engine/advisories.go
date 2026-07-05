package engine

import (
	"crypto/sha256"
	"encoding/hex"
	"maps"
	"slices"
	"sort"
	"strconv"
	"strings"

	"github.com/alexei-led/archfit/internal/classify"
	"github.com/alexei-led/archfit/internal/config"
	"github.com/alexei-led/archfit/internal/model/coupling"
	"github.com/alexei-led/archfit/internal/model/finding"
	"github.com/alexei-led/archfit/internal/model/graph"
	"github.com/alexei-led/archfit/internal/staleness"
	"github.com/alexei-led/archfit/internal/status"
)

// kindAdvisory is the finding kind for non-gating advisory findings.
const kindAdvisory = "advisory"

// RuleIDBCImbalanced is the rule ID stamped on a Balanced-Coupling advisory
// finding. Exported so the composition root (cmd) matches promotable findings
// against the same constant instead of a drifting duplicate literal.
const RuleIDBCImbalanced = "bc/imbalanced_coupling"

// RuleIDBCDuplicatedKnowledge is the rule ID for the duplicated-knowledge
// advisory: a cross-module clone pair with NO import edge between the modules
// (book Ch7 — functional coupling through shared logic, invisible to the
// import graph). Report-only: never promoted by the coupling gate, which
// matches RuleIDBCImbalanced only.
const RuleIDBCDuplicatedKnowledge = "bc/duplicated_knowledge"

// edgeKindClone marks a duplicated-knowledge finding's edge evidence: the two
// endpoints are linked by duplicated code, not an import.
const edgeKindClone = "clone"

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
		if cl.DistanceBasis != coupling.DistanceBasisUnknown {
			matched["distance_basis"] = string(cl.DistanceBasis)
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
			RuleID:   RuleIDBCImbalanced,
			Status:   finding.StatusNew,
			Severity: finding.Severity(cl.Severity),
			Edge: finding.EdgeEvidence{
				From: finding.Endpoint{Module: fromModule, Path: fromPath},
				To:   finding.Endpoint{Module: toModule, Path: toPath},
				Kind: string(e.Kind),
			},
			Locations: withCloneLocations(e.Locations, cl.CloneLocations),
			Why:       bcAdvisoryWhy(cl),
			MatchedBy: matched,
		}
		advisoryFindings = append(advisoryFindings, af)
	}
	// Duplicated knowledge (book Ch7): cross-module clone pairs with NO import
	// edge. The symmetric-upgrade path only sees pairs that have an edge;
	// without this pass, copy-paste drift between unconnected modules is
	// invisible end-to-end. Report-only — never promoted by the coupling gate.
	advisoryFindings = append(advisoryFindings, duplicatedKnowledgeAdvisories(g, classifyCfg)...)

	// Append staleness advisories.
	advisoryFindings = append(advisoryFindings, staleness.Check(g, in.Staleness, in.Now)...)

	// Stale pinned labels were ignored during classification; advise re-enrich.
	advisoryFindings = append(advisoryFindings, staleLabelFnds...)

	// Apply baseline and exception status to advisory findings.
	// status.Assign also emits fixed gate findings; suppress those for the advisory pass
	// by discarding any synthetic kind=="gate" entries it appends.
	tagged := status.Assign(advisoryFindings, in.Accepted, in.Waivers, in.Now, "advisory")
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
// Status is part of the key so a baseline-suppressed or waived edge is never folded
// into a "new" rollup's count. Cohesion (high strength + low distance) never reaches
// this pass — cohesion scores SeverityNone via the book formula — so a rollup, like an
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
		if f.RuleID != RuleIDBCImbalanced {
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
	rep.Edge.From.Path, rep.Edge.To.Path = groupEdgePaths(members, rep.Locations)
	return rep
}

// groupEdgePaths returns honest edge.from.path/edge.to.path for a rolled-up
// finding: the (from, to) pair belonging to whichever member owns the first
// (sorted) merged location — never an arbitrary hash-ID-ordered representative
// that can point at a different member's file than locations[0] does. When no
// owner is determinable (locations empty — TS edges carry no Locations), the
// representative's own pair is kept: it is a genuine member edge of the group,
// and wiping it to "" would strip the finding's only path evidence. members
// must arrive sorted by ID (rollupFinding sorts), so members[0] is the
// representative. Either way the pair names one real member edge; its form is
// the graph node's — a repo file for Go/TS, a dotted module ID or crate name
// for Python/Rust module graphs.
func groupEdgePaths(members []finding.Finding, locs []graph.Location) (fromPath, toPath string) {
	if len(locs) > 0 {
		first := locs[0]
		for _, m := range members {
			if slices.Contains(m.Locations, first) {
				return m.Edge.From.Path, m.Edge.To.Path
			}
		}
	}
	return members[0].Edge.From.Path, members[0].Edge.To.Path
}

// withCloneLocations appends cloneLocations onto base — the edge's baseline
// Locations plus the real duplicated-code file:line pairs a Symmetric-strength
// clone upgrade contributed (classify.go) — deduplicated and sorted, so a
// Rust crate edge's Cargo.toml:0 baseline is joined by the actual clone site
// instead of standing alone as the only evidence. base is returned unchanged
// (no allocation) when cloneLocations is empty — the common case.
func withCloneLocations(base, cloneLocations []graph.Location) []graph.Location {
	if len(cloneLocations) == 0 {
		return base
	}
	seen := make(map[graph.Location]struct{}, len(base)+len(cloneLocations))
	locs := make([]graph.Location, 0, len(base)+len(cloneLocations))
	for _, l := range base {
		if _, ok := seen[l]; ok {
			continue
		}
		seen[l] = struct{}{}
		locs = append(locs, l)
	}
	for _, l := range cloneLocations {
		if _, ok := seen[l]; ok {
			continue
		}
		seen[l] = struct{}{}
		locs = append(locs, l)
	}
	sort.Slice(locs, func(i, j int) bool {
		if locs[i].File != locs[j].File {
			return locs[i].File < locs[j].File
		}
		return locs[i].Line < locs[j].Line
	})
	return locs
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
		" → " + string(cl.Severity) + " severity (" + bcRiskClause(cl) + ")"
}

// bcRiskClause names the Balanced Coupling risk for an edge in Vlad Khononov's
// vocabulary. A critical edge is "distributed-monolith risk" ONLY at high distance
// (different owner / deploy unit — the book's high strength × high distance × high
// volatility worst case). A critical edge at low distance (cross_module_same_owner)
// is local coupling to a volatile target: its cascade is cheap (one owner, one
// binary), so it is named as such, NOT as a distributed monolith — recommending
// "introduce a contract" there would be cargo-cult. High severity splits on the
// same distance test: only a genuinely high-distance edge is "across a boundary";
// a low-distance one names its cascade as contained. Cohesion (high strength + low
// distance, balanced) never reaches here — the book formula scores it SeverityNone.
//
// The clause names the edge's ACTUAL matched_by.strength and matched_by.volatility —
// never a fixed severity-level narrative — so an agent reading the prose reaches
// the same remediation an agent reading matched_by would (verified wrong on ccgram:
// 15/16 critical edges were StrengthModel, book ordinal 3/10 — low, not "high-strength").
func bcRiskClause(cl coupling.Classification) string {
	strengthDesc := strengthClause(cl.Strength)
	volatilityDesc := volatilityClause(cl.Volatility)
	switch cl.Severity {
	case coupling.SeverityCritical:
		if coupling.DistanceIsHigh(cl.Distance) {
			return strengthDesc + " across a high-distance boundary to " + volatilityDesc + " → distributed-monolith risk"
		}
		return strengthDesc + " to " + volatilityDesc + " at low distance → local cascade (cheap to change; not a distributed monolith)"
	case coupling.SeverityHigh:
		if coupling.DistanceIsHigh(cl.Distance) {
			return strengthDesc + " across a boundary to " + volatilityDesc + " → likely cascading changes"
		}
		return strengthDesc + " to " + volatilityDesc + " at low distance → cascading changes contained to one owner"
	default:
		return "unbalanced coupling → elevated maintenance effort"
	}
}

// strengthClause names the actual integration-strength level of an edge, in
// Balanced Coupling vocabulary — never a fixed "high-strength" placeholder.
func strengthClause(s coupling.Strength) string {
	switch s {
	case coupling.StrengthIntrusive:
		return "intrusive (implementation-level) coupling"
	case coupling.StrengthSymmetric:
		return "symmetric (bidirectional implementation-level) coupling"
	case coupling.StrengthFunctional:
		return "functional coupling"
	case coupling.StrengthModel:
		return "model coupling"
	case coupling.StrengthContract:
		return "contract coupling"
	default:
		return "unclassified-strength coupling"
	}
}

// volatilityClause names the actual volatility level of an edge's target.
func volatilityClause(v coupling.Volatility) string {
	switch v {
	case coupling.VolatilityHigh:
		return "a volatile target"
	case coupling.VolatilityMedium:
		return "a moderately volatile target"
	case coupling.VolatilityLow:
		return "a low-volatility target"
	case coupling.VolatilityFrozen:
		return "a frozen target"
	case coupling.VolatilityUndeclared:
		return "a target of undeclared volatility"
	default:
		return "a target of unknown volatility"
	}
}

// duplicatedKnowledgeAdvisories builds one bc/duplicated_knowledge advisory per
// duplicated-knowledge pair (classify.CloneOnlyPairs): a cross-module clone pair
// whose modules share no import edge, scored with the standard book formula at
// symmetric strength. Findings honor the same coupling.min_severity floor as
// bc/imbalanced_coupling advisories; the pair-level ID is stable across runs so
// baseline acceptance suppresses it like any other advisory.
func duplicatedKnowledgeAdvisories(g *graph.Graph, classifyCfg config.ClassifyConfig) []finding.Finding {
	var out []finding.Finding
	for _, p := range classify.CloneOnlyPairs(g, classifyCfg) {
		cl := p.Classification
		if cl.Severity == coupling.SeverityNone || !severityAtLeast(cl.Severity, classifyCfg.BCAdvisoryMinSeverity) {
			continue
		}
		matched := map[string]string{
			"strength":     string(cl.Strength),
			"distance":     string(cl.Distance),
			"volatility":   string(cl.Volatility),
			"score_policy": string(config.NormalizeDuplicatedKnowledgePolicy(classifyCfg.DuplicatedKnowledgePolicy)),
		}
		if cl.DistanceBasis != coupling.DistanceBasisUnknown {
			matched["distance_basis"] = string(cl.DistanceBasis)
		}
		if cl.Score.Reason != "" {
			matched["score"] = cl.Score.Reason
			matched["score_value"] = strconv.Itoa(cl.Score.Value)
			matched["score_band"] = string(cl.Score.Band)
			matched["score_version"] = coupling.ScoreVersion
		}
		if cl.Score.CheapestMove != "" {
			matched["cheapest_move"] = cl.Score.CheapestMove
		}
		out = append(out, finding.Finding{
			ID:       duplicatedKnowledgeID(p.FromModule, p.ToModule),
			Kind:     kindAdvisory,
			RuleID:   RuleIDBCDuplicatedKnowledge,
			Status:   finding.StatusNew,
			Severity: finding.Severity(cl.Severity),
			Edge: finding.EdgeEvidence{
				From: finding.Endpoint{Module: p.FromModule, Path: p.FromPath},
				To:   finding.Endpoint{Module: p.ToModule, Path: p.ToPath},
				Kind: edgeKindClone,
			},
			Locations: p.Locations,
			Why: "duplicated knowledge: cross-module code clones between " + p.FromModule +
				" and " + p.ToModule + " with no import edge — symmetric functional coupling; " +
				"a change to the shared logic must be repeated in both modules. Extract the " +
				"shared knowledge, or accept the pair with an approved label",
			MatchedBy: matched,
		})
	}
	return out
}

// couplingAdvisoryID returns a stable 32-character hex fingerprint for a coupling advisory
// finding, derived from (from, to, kind) — same scheme as finding.fingerprint.
func couplingAdvisoryID(from, to, kind string) string {
	h := sha256.Sum256([]byte("bc/imbalanced_coupling\x00" + from + "\x00" + to + "\x00" + kind))
	return hex.EncodeToString(h[:16])
}

// duplicatedKnowledgeID returns a stable fingerprint for a bc/duplicated_knowledge
// advisory, derived from the canonical module pair — independent of which files
// carry the clones, so the finding survives clone movement within the modules.
func duplicatedKnowledgeID(fromModule, toModule string) string {
	h := sha256.Sum256([]byte(RuleIDBCDuplicatedKnowledge + "\x00" + fromModule + "\x00" + toModule))
	return hex.EncodeToString(h[:16])
}

// staleLabelID returns a stable fingerprint for a labels/stale advisory.
func staleLabelID(from, to string) string {
	h := sha256.Sum256([]byte("labels/stale\x00" + from + "\x00" + to))
	return hex.EncodeToString(h[:16])
}
