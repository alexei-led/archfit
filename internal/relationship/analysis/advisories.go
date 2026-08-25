package analysis

import (
	"crypto/sha256"
	"encoding/hex"
	"sort"
	"strconv"

	"github.com/alexei-led/archfit/internal/policy"
	"github.com/alexei-led/archfit/internal/relationship"
	"github.com/alexei-led/archfit/internal/relationship/classify"
	"github.com/alexei-led/archfit/internal/relationship/coupling"
)

const relationshipScoreVersion = "bc_score.v6"
const (
	bcImbalancedRule        = "bc/imbalanced_coupling"
	duplicatedKnowledgeRule = "bc/duplicated_knowledge"
)

func advisoryCandidates(set relationship.Set, clones []relationship.CloneOnlyPair, cfg classify.Config) []relationship.AdvisoryCandidate {
	out := make([]relationship.AdvisoryCandidate, 0, len(set.Edges)+len(clones))
	for _, edge := range set.Edges {
		if edge.Severity == relationship.SeverityNone || !severityAtLeast(edge.Severity, cfg.BCAdvisoryMinSeverity) {
			continue
		}
		matched := classificationMatched(edge.Classified, edge.Strength, edge.Distance, edge.Volatility)
		locs := append([]relationship.Location(nil), edge.Locations...)
		locs = appendLocations(locs, edge.Classified.CloneLocations)
		out = append(out, relationship.AdvisoryCandidate{
			ID: couplingAdvisoryID(edge.FromPath, edge.ToPath, edge.Kind), RuleID: bcImbalancedRule,
			Kind: "advisory", Severity: edge.Severity, From: edge.FromPath, To: edge.ToPath,
			FromModule: edge.FromModule, ToModule: edge.ToModule, EdgeKind: edge.Kind,
			Locations: locs, Why: bcAdvisoryWhy(edge), MatchedBy: matched,
		})
	}
	for _, pair := range clones {
		if pair.Severity == relationship.SeverityNone || !severityAtLeast(pair.Severity, cfg.BCAdvisoryMinSeverity) {
			continue
		}
		matched := classificationMatched(pair.Classified, pair.Strength, pair.Distance, pair.Volatility)
		matched["score_policy"] = string(policy.NormalizeDuplicatedKnowledgePolicy(cfg.DuplicatedKnowledgePolicy))
		out = append(out, relationship.AdvisoryCandidate{
			ID: duplicatedKnowledgeID(pair.FromModule, pair.ToModule), RuleID: duplicatedKnowledgeRule,
			Kind: "advisory", Severity: pair.Severity, From: pair.FromPath, To: pair.ToPath,
			FromModule: pair.FromModule, ToModule: pair.ToModule, EdgeKind: "clone",
			Locations: append([]relationship.Location(nil), pair.Locations...),
			Why:       "duplicated knowledge: cross-module code clones between " + pair.FromModule + " and " + pair.ToModule + " with no import edge — symmetric functional coupling; a change to the shared logic must be repeated in both modules. Extract the shared knowledge, or accept the pair with an approved label",
			MatchedBy: matched,
		})
	}
	return out
}

func classificationMatched(cl relationship.Classification, strength relationship.Strength, distance relationship.Distance, volatility relationship.Volatility) map[string]string {
	matched := map[string]string{"strength": string(strength), "distance": string(distance), "volatility": string(volatility)}
	if cl.DistanceBasis != "" && cl.DistanceBasis != "unknown" {
		matched["distance_basis"] = cl.DistanceBasis
	}
	if cl.Score.Reason != "" {
		matched["score"] = cl.Score.Reason
		matched["score_value"] = strconv.Itoa(cl.Score.Value)
		matched["score_band"] = string(cl.Score.Band)
		matched["score_version"] = relationshipScoreVersion
	}
	if cl.Score.CheapestMove != "" {
		matched["cheapest_move"] = cl.Score.CheapestMove
	}
	return matched
}

func appendLocations(base, extra []relationship.Location) []relationship.Location {
	if len(extra) == 0 {
		return base
	}
	seen := make(map[relationship.Location]struct{}, len(base)+len(extra))
	out := make([]relationship.Location, 0, len(base)+len(extra))
	for _, loc := range append(base, extra...) {
		if _, ok := seen[loc]; !ok {
			seen[loc] = struct{}{}
			out = append(out, loc)
		}
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].File != out[j].File {
			return out[i].File < out[j].File
		}
		return out[i].Line < out[j].Line
	})
	return out
}

func severityAtLeast(got relationship.Severity, threshold string) bool {
	if threshold == "" {
		return true
	}
	rank := map[relationship.Severity]int{relationship.SeverityLow: 1, relationship.SeverityMedium: 2, relationship.SeverityHigh: 3, relationship.SeverityCritical: 4}
	return rank[got] >= rank[relationship.Severity(threshold)]
}

func couplingAdvisoryID(from, to, kind string) string {
	h := sha256.Sum256([]byte(bcImbalancedRule + "\x00" + from + "\x00" + to + "\x00" + kind))
	return hex.EncodeToString(h[:16])
}
func duplicatedKnowledgeID(from, to string) string {
	h := sha256.Sum256([]byte(duplicatedKnowledgeRule + "\x00" + from + "\x00" + to))
	return hex.EncodeToString(h[:16])
}

func bcAdvisoryWhy(edge relationship.Edge) string {
	return "balanced coupling: " + string(edge.Strength) + " integration strength × " + string(edge.Distance) + " distance × " + string(edge.Volatility) + " volatility → " + string(edge.Severity) + " severity (" + bcRiskClause(edge) + ")"
}
func bcRiskClause(edge relationship.Edge) string {
	strength := strengthClause(edge.Strength)
	volatility := volatilityClause(edge.Volatility)
	switch edge.Severity {
	case relationship.SeverityCritical:
		if coupling.DistanceIsHigh(edge.Distance) {
			return strength + " across a high-distance boundary to " + volatility + " → distributed-monolith risk"
		}
		return strength + " to " + volatility + " at low distance → local cascade (cheap to change; not a distributed monolith)"
	case relationship.SeverityHigh:
		if coupling.DistanceIsHigh(edge.Distance) {
			return strength + " across a boundary to " + volatility + " → likely cascading changes"
		}
		return strength + " to " + volatility + " at low distance → cascading changes contained to one owner"
	default:
		return "unbalanced coupling → elevated maintenance effort"
	}
}
func strengthClause(s relationship.Strength) string {
	switch s {
	case relationship.StrengthIntrusive:
		return "intrusive (implementation-level) coupling"
	case relationship.StrengthSymmetric:
		return "symmetric (bidirectional implementation-level) coupling"
	case relationship.StrengthFunctional:
		return "functional coupling"
	case relationship.StrengthModel:
		return "model coupling"
	case relationship.StrengthContract:
		return "contract coupling"
	default:
		return "unclassified-strength coupling"
	}
}
func volatilityClause(v relationship.Volatility) string {
	switch v {
	case relationship.VolatilityHigh:
		return "a volatile target"
	case relationship.VolatilityMedium:
		return "a moderately volatile target"
	case relationship.VolatilityLow:
		return "a low-volatility target"
	case relationship.VolatilityFrozen:
		return "a frozen target"
	case relationship.VolatilityUndeclared:
		return "a target of undeclared volatility"
	default:
		return "a target of unknown volatility"
	}
}
