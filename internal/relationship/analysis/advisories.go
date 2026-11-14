package analysis

import (
	"crypto/sha256"
	"encoding/hex"
	"strconv"

	modelrule "github.com/alexei-led/archfit/v3/internal/model/rule"
	"github.com/alexei-led/archfit/v3/internal/relationship"
	"github.com/alexei-led/archfit/v3/internal/relationship/classify"
)

const relationshipScoreVersion = "bc_score.v7"
const (
	bcImbalancedRule = modelrule.RuleIDBCImbalancedCoupling
)

func advisoryCandidates(set relationship.Set, cfg classify.Config) []relationship.AdvisoryCandidate {
	out := make([]relationship.AdvisoryCandidate, 0, len(set.Edges))
	for _, edge := range set.Edges {
		if edge.Severity == relationship.SeverityNone || !severityAtLeast(edge.Severity, cfg.BCAdvisoryMinSeverity) {
			continue
		}
		matched := classificationMatched(edge.Classified, edge.Strength, edge.Distance, edge.Volatility, relationship.BalancingHypothesis(relationship.HypothesisInput{
			Strength: edge.Strength, Volatility: edge.Volatility, Band: edge.Severity,
			TargetPublic: len(cfg.Modules[edge.ToModule].Public) > 0, CohesiveRole: classify.CohesiveRole(cfg.Modules[edge.FromModule].Role),
		}))
		locs := append([]relationship.Location(nil), edge.Locations...)
		out = append(out, relationship.AdvisoryCandidate{
			ID: couplingAdvisoryID(edge.FromPath, edge.ToPath, edge.Kind), RuleID: bcImbalancedRule,
			Kind: "advisory", Severity: edge.Severity, From: edge.FromPath, To: edge.ToPath,
			FromModule: edge.FromModule, ToModule: edge.ToModule, EdgeKind: edge.Kind,
			Locations: locs, Why: bcAdvisoryWhy(edge), MatchedBy: matched,
		})
	}
	return out
}

func classificationMatched(cl relationship.Classification, strength relationship.Strength, distance relationship.Distance, volatility relationship.Volatility, hypothesis relationship.SeamHypothesis) map[string]string {
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
	if hypothesis != "" {
		matched["hypothesis"] = string(hypothesis)
	}
	return matched
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
func bcAdvisoryWhy(edge relationship.Edge) string {
	return "balanced coupling: " + string(edge.Strength) + " integration strength × " + string(edge.Distance) + " distance × " + string(edge.Volatility) + " volatility → " + string(edge.Severity) + " severity (" + bcRiskClause(edge) + ")"
}
func bcRiskClause(edge relationship.Edge) string {
	strength := strengthClause(edge.Strength)
	volatility := volatilityClause(edge.Volatility)
	if edge.Distance != relationship.DistanceExternal && edge.Distance != relationship.DistanceUnknown &&
		(edge.Strength == relationship.StrengthFunctional || edge.Strength == relationship.StrengthSymmetric) {
		volatility += " (higher of source and target)"
	}
	switch edge.Severity {
	case relationship.SeverityCritical:
		if edge.Distance == relationship.DistanceCrossDeployUnit {
			return strength + " across a deploy-unit boundary with " + volatility + " → distributed-monolith risk"
		}
		return strength + " across a module boundary with " + volatility + " → change propagation risk"
	case relationship.SeverityHigh:
		return strength + " across a boundary with " + volatility + " → likely cascading changes"
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
		return "high effective volatility"
	case relationship.VolatilityMedium:
		return "medium effective volatility"
	case relationship.VolatilityLow:
		return "low effective volatility"
	case relationship.VolatilityFrozen:
		return "frozen effective volatility"
	case relationship.VolatilityUndeclared:
		return "undeclared volatility"
	default:
		return "unknown volatility"
	}
}
