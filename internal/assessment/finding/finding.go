package finding

import (
	"crypto/sha256"
	"encoding/hex"

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
	RuleIDBCImbalanced = "bc/imbalanced_coupling"
	// RuleIDDuplicatedKnowledge is the clone-only coupling advisory.
	RuleIDDuplicatedKnowledge = "bc/duplicated_knowledge"
	// RuleIDCouplingGate is the synthetic finding a tripped coupling gate emits
	// when it has no promotable advisory.
	RuleIDCouplingGate = "bc/coupling_gate"
)

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

// fingerprint computes hex(sha256(ruleID + "\x00" + from + "\x00" + to + "\x00" + kind)[:16]).
// Slicing 16 bytes before hex-encoding produces a 32-character hex string.
func fingerprint(ruleID, from, to, kind string) string {
	h := sha256.Sum256([]byte(ruleID + "\x00" + from + "\x00" + to + "\x00" + kind))
	return hex.EncodeToString(h[:16])
}
