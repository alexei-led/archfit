package finding

import (
	"crypto/sha256"
	"encoding/hex"

	"github.com/alexei-led/archfit/internal/model/graph"
)

// Status represents the lifecycle state of a finding.
type Status string

// Status constants for finding lifecycle (spec §9).
const (
	StatusNew           Status = "new"
	StatusBaseline      Status = "baseline"
	StatusExcepted      Status = "excepted"
	StatusExpiredExcept Status = "expired_exception"
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
	ID           string            `json:"id"`
	Kind         string            `json:"kind"`
	RuleID       string            `json:"rule_id"`
	Status       Status            `json:"status"`
	Severity     Severity          `json:"severity"`
	Confidence   string            `json:"confidence"`
	Edge         EdgeEvidence      `json:"edge"`
	MatchedBy    map[string]string `json:"matched_by"`
	Locations    []graph.Location  `json:"locations"`
	Why          string            `json:"why"`
	Constraint   string            `json:"constraint"`
	Alternatives []string          `json:"allowed_alternatives,omitempty"`
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
func New(ruleID string, e graph.Edge, locs []graph.Location) Finding {
	id := fingerprint(ruleID, e.From, e.To, string(e.Kind))
	return Finding{
		ID:     id,
		Kind:   "gate",
		RuleID: ruleID,
		Status: StatusNew,
		Edge: EdgeEvidence{
			From: Endpoint{Path: graph.NodePath(e.From)},
			To:   Endpoint{Path: graph.NodePath(e.To)},
			Kind: string(e.Kind),
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
