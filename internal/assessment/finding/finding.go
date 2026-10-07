package finding

import (
	"crypto/sha256"
	"encoding/hex"
	"strings"

	modelrule "github.com/alexei-led/archfit/internal/model/rule"
	"github.com/alexei-led/archfit/internal/relationship"
)

// Status represents the lifecycle state of a finding.
type Status string

// Status constants for finding lifecycle (spec §9).
const (
	StatusNew           Status = "new"
	StatusBaseline      Status = "baseline"
	StatusWaived        Status = "waived"
	StatusExpiredWaiver Status = "expired_waiver"
	StatusFixed         Status = "fixed"
)

// Origin places a finding relative to the base ref of an `analyze/check
// --base` run. It is presentation only: no gate, verdict, or baseline reads it.
type Origin string

// Origin values. A run without --base leaves every origin empty.
const (
	// OriginIntroduced means the change added the finding: the base run did not
	// observe it, and the analyzer evidence of both runs pairs.
	OriginIntroduced Origin = "introduced"
	// OriginPreExisting means the base run observed the finding too.
	OriginPreExisting Origin = "pre_existing"
	// OriginUnknown means the analyzer evidence could not establish an origin.
	OriginUnknown Origin = "unknown"
)

// Severity represents the severity level of a finding.
type Severity string

// Severity constants (spec §9): critical > high > medium > low.
const (
	SeverityCritical Severity = "critical"
	SeverityHigh     Severity = "high"
	SeverityMedium   Severity = "medium"
	SeverityLow      Severity = "low"
)

// Kind classifies a finding: a blocking gate violation vs a non-blocking
// advisory. Stored in the string field Finding.Kind.
const (
	KindGate     = "gate"
	KindAdvisory = "advisory"
)

// Rule IDs that assessment itself emits (rather than reading from declared
// policy). They are part of the published finding contract, so they are named
// once here instead of being repeated as literals at every producer and
// consumer.
const (
	// RuleIDBCImbalanced is the balanced-coupling advisory the coupling gate
	// promotes to a gate finding.
	RuleIDBCImbalanced = modelrule.RuleIDBCImbalancedCoupling
	// RuleIDDuplicatedKnowledge is the clone-only coupling advisory.
	RuleIDDuplicatedKnowledge = modelrule.RuleIDDuplicatedKnowledge
	// RuleIDCouplingGate is the synthetic finding a tripped coupling gate emits
	// when it has no promotable advisory.
	RuleIDCouplingGate = modelrule.RuleIDCouplingGate
	// RuleIDMapUncoveredPath is the map completeness finding: production
	// source no declared module owns. module_review.gate: fail makes it a
	// gate finding.
	RuleIDMapUncoveredPath = modelrule.RuleIDMapUncoveredPath
	// RuleIDMetricPrefix starts the rule ID of a tripped metric ratchet.
	RuleIDMetricPrefix = modelrule.RuleIDMetricPrefix
)

// IsMetricRatchet reports whether ruleID names a tripped metric ratchet.
func IsMetricRatchet(ruleID string) bool { return modelrule.IsMetricRatchet(ruleID) }

// Endpoint identifies one side of a finding edge (resolved at diagnostic assembly).
type Endpoint struct {
	Module string `json:"module"`
	Path   string `json:"path"`
}

// EdgeEvidence is the finding-level edge representation (spec §9).
// Distinct from graph.Edge: carries {module, path} endpoints, not kind:path IDs.
type EdgeEvidence struct {
	From Endpoint `json:"from"`
	To   Endpoint `json:"to"`
	Kind string   `json:"kind"`
}

// Finding represents one rule violation detected in the dependency graph (spec §9/§12).
type Finding struct {
	ID           string                  `json:"id"`
	Kind         string                  `json:"kind"`
	RuleID       string                  `json:"rule_id"`
	Status       Status                  `json:"status"`
	Severity     Severity                `json:"severity"`
	Confidence   string                  `json:"confidence"`
	Edge         EdgeEvidence            `json:"edge"`
	MatchedBy    map[string]string       `json:"matched_by"`
	Locations    []relationship.Location `json:"locations"`
	Why          string                  `json:"why"`
	Constraint   string                  `json:"constraint"`
	Alternatives []string                `json:"allowed_alternatives,omitempty"`
	// Rationale is the rule's declared rationale, already appended to Why. It
	// is not serialized: the repair task repeats it in its constraints.
	Rationale string `json:"-"`
	// Members are the IDs of every finding a rollup stands for, sorted, the
	// representative included. Empty for a finding that is not a rollup. Not
	// serialized: matched_by.group_members carries a capped list for readers,
	// while a baseline capture must accept every member.
	Members []string `json:"-"`
	// Origin is set only by an `analyze/check --base` run. Not serialized here:
	// the report projection carries it.
	Origin Origin `json:"-"`
}

// New creates a Finding with a stable fingerprint ID derived from (ruleID, from, to, kind).
//
// The ID is computed as hex(sha256(ruleID + "\x00" + from + "\x00" + to + "\x00" + kind)[:16]),
// producing a 32-character hex string. Line numbers do not affect the ID; the same
// violation found at different positions remains one finding, with all positions in Locations.
//
// Kind defaults to "gate". Status defaults to "new".
// Edge.From.Path and Edge.To.Path are set to the bare repo-relative path (kind: prefix stripped).
// Edge.From.Module, Edge.To.Module, Severity, and MatchedBy are left zero — filled later
// by the rule and diagnostic assembly stage (engine Task 16).
func New(ruleID string, e relationship.Edge, locs []relationship.Location) Finding {
	id := fingerprint(ruleID, e.FromID, e.ToID, e.Kind)
	return Finding{
		ID:     id,
		Kind:   KindGate,
		RuleID: ruleID,
		Status: StatusNew,
		Edge: EdgeEvidence{
			From: Endpoint{Path: relationship.NodePath(e.FromID)},
			To:   Endpoint{Path: relationship.NodePath(e.ToID)},
			Kind: e.Kind,
		},
		Locations: locs,
	}
}

// NewKeyed creates a Finding whose subject is not one graph edge — a module
// pair, a pattern match — with a stable fingerprint over the rule ID, the
// finding's edge kind, and the subject keys, in order:
// hex(sha256(ruleID + "\x00" + kind + "\x00" + key...)[:16]), the same
// 32-character scheme as New. Line numbers never enter keys, so a moved
// violation keeps its ID.
//
// Kind defaults to "gate" and Status to "new"; Edge.Kind is kind. Endpoints,
// locations, severity, and text are the caller's.
func NewKeyed(ruleID, kind string, keys ...string) Finding {
	parts := append([]string{ruleID, kind}, keys...)
	h := sha256.Sum256([]byte(strings.Join(parts, "\x00")))
	return Finding{
		ID:     hex.EncodeToString(h[:16]),
		Kind:   KindGate,
		RuleID: ruleID,
		Status: StatusNew,
		Edge:   EdgeEvidence{Kind: kind},
	}
}

// fingerprint computes hex(sha256(ruleID + "\x00" + from + "\x00" + to + "\x00" + kind)[:16]).
// Slicing 16 bytes before hex-encoding produces a 32-character hex string.
func fingerprint(ruleID, from, to, kind string) string {
	h := sha256.Sum256([]byte(ruleID + "\x00" + from + "\x00" + to + "\x00" + kind))
	return hex.EncodeToString(h[:16])
}
