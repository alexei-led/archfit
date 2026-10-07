// Package rule contains neutral identities shared by policy and assessment.
package rule

import "strings"

// Synthetic finding IDs emitted without a declared policy rule.
const (
	// RuleIDBCImbalancedCoupling identifies the balanced-coupling advisory.
	RuleIDBCImbalancedCoupling = "bc/imbalanced_coupling"
	// RuleIDDuplicatedKnowledge identifies the clone-only coupling advisory.
	RuleIDDuplicatedKnowledge = "bc/duplicated_knowledge"
	// RuleIDCouplingGate identifies a generated distributed-monolith gate finding.
	RuleIDCouplingGate = "bc/coupling_gate"
	// RuleIDMapUncoveredPath identifies an uncovered-path map advisory.
	RuleIDMapUncoveredPath = "map/uncovered_path"
	// RuleIDMapDeadRule identifies a dead-rule map advisory.
	RuleIDMapDeadRule = "map/dead_rule"
	// RuleIDMapStaleReview identifies a stale-review map advisory.
	RuleIDMapStaleReview = "map/stale_review"
	// RuleIDLabelsStale identifies stale pinned-label evidence.
	RuleIDLabelsStale = "labels/stale"
	// RuleIDMetricPrefix starts the ID of a tripped metric ratchet: the rule ID
	// is the prefix plus the metric name ("metric/cycle").
	RuleIDMetricPrefix = "metric/"
)

// IsMetricRatchet reports whether ruleID names a tripped metric ratchet.
func IsMetricRatchet(ruleID string) bool {
	return strings.HasPrefix(ruleID, RuleIDMetricPrefix)
}

// WaiverScope classifies synthetic findings that may be named without a declared rule.
type WaiverScope string

const (
	// WaiverScopeEdge permits endpoint-scoped waivers.
	WaiverScopeEdge WaiverScope = "edge"
	// WaiverScopeEdgeless permits rule-only waivers.
	WaiverScopeEdgeless WaiverScope = "edgeless"
	// WaiverScopeForbidden rejects waivers for the finding.
	WaiverScopeForbidden WaiverScope = "forbidden"
)

// SyntheticWaiverScope returns the waiver behavior for an assessment-owned finding ID.
func SyntheticWaiverScope(ruleID string) WaiverScope {
	if IsMetricRatchet(ruleID) {
		return WaiverScopeEdgeless
	}
	switch ruleID {
	case RuleIDMapUncoveredPath, RuleIDMapDeadRule, RuleIDMapStaleReview:
		return WaiverScopeEdgeless
	case RuleIDCouplingGate:
		return WaiverScopeForbidden
	case RuleIDBCImbalancedCoupling, RuleIDDuplicatedKnowledge, RuleIDLabelsStale:
		return WaiverScopeEdge
	default:
		return ""
	}
}
