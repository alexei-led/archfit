package coupling

import "github.com/alexei-led/archfit/internal/relationship"

// Strength classifies how a dependency is expressed at the API boundary.
// It is an alias of relationship.Strength so the coupling classification model
// shares one canonical integration-strength vocabulary with assessment.
type Strength = relationship.Strength

// Strength constants (spec §18).
// contract and intrusive are decided deterministically (config globs,
// visibility); model and functional come from SCIP symbol-kind heuristics —
// the Tranche-2 enrich workflow refines those labels under human review.
const (
	StrengthContract   = relationship.StrengthContract
	StrengthIntrusive  = relationship.StrengthIntrusive
	StrengthModel      = relationship.StrengthModel
	StrengthFunctional = relationship.StrengthFunctional
	// StrengthSymmetric is bidirectional coupling at implementation level —
	// book ordinal 9, between functional (8) and intrusive (10).
	StrengthSymmetric = relationship.StrengthSymmetric
	StrengthUnknown   = relationship.StrengthUnknown
)

// Distance measures how far apart two modules are in the ownership hierarchy.
// It is an alias of relationship.Distance so the coupling classification model
// shares one canonical distance vocabulary with assessment.
type Distance = relationship.Distance

// Distance constants (spec §18).
// DistanceExternal is a config-declared external integration seam
// (`external_systems:`) — book Ch10 Example 1, cross-vendor integration, the
// far end of the distance ladder (D=10). Only DECLARED external targets get
// it; undeclared external edges stay DistanceUnknown and are excluded from
// coupling_balance (scoring every library import at D=10 would flood the
// metric with vendor noise).
const (
	DistanceSameModule           = relationship.DistanceSameModule
	DistanceCrossModuleSameOwner = relationship.DistanceCrossModuleSameOwner
	DistanceCrossModuleDiffOwner = relationship.DistanceCrossModuleDiffOwner
	DistanceCrossDeployUnit      = relationship.DistanceCrossDeployUnit
	DistanceExternal             = relationship.DistanceExternal
	DistanceUnknown              = relationship.DistanceUnknown
)

// Volatility classifies how likely a module's API is to change.
// Per Khononov, volatility is derived from the DDD subdomain (core→high,
// supporting→low, generic→low) with an explicit per-module override.
// It is an alias of relationship.Volatility.
type Volatility = relationship.Volatility

// Volatility constants derived from subdomain classification.
//
// Undeclared and Unknown are distinct on purpose:
//   - Undeclared: the target module IS known, but the config omits both a
//     volatility and a subdomain (and no path heuristic matched). This is a
//     config gap the user can close — the advice is "declare subdomain/
//     volatility", not "lower volatility".
//   - Unknown: the target could not even be resolved to a module (no config
//     module owns it), so volatility is genuinely indeterminate.
//
// Both are scored conservatively (treated as potentially volatile); they differ
// only in the guidance surfaced to the user.
const (
	VolatilityFrozen     = relationship.VolatilityFrozen // frozen/legacy systems; V=1 (most stable)
	VolatilityLow        = relationship.VolatilityLow
	VolatilityMedium     = relationship.VolatilityMedium
	VolatilityHigh       = relationship.VolatilityHigh
	VolatilityUndeclared = relationship.VolatilityUndeclared
	VolatilityUnknown    = relationship.VolatilityUnknown
)

// VolatilityResolved reports whether v is a concrete level the tool can act on
// (frozen/low/medium/high), as opposed to undeclared (config gap) or unknown
// (unresolvable). Callers that need "do we actually have a volatility?" should
// use this rather than comparing against VolatilityUnknown alone.
func VolatilityResolved(v Volatility) bool {
	return relationship.VolatilityResolved(v)
}

// ConnascenceKind names a book Ch6 connascence category. Static evidence may
// attach to coupling edges; dynamic categories are disclosed as unmeasured until
// deterministic runtime-trace evidence exists. These labels never feed scoring.
type ConnascenceKind string

// Connascence categories from Connascence of Name through Connascence of
// Identity. Not every category is currently measured; unsupported kinds are
// disclosed as unmeasured in the diagnostic summary.
const (
	ConnascenceName      ConnascenceKind = "name"
	ConnascenceType      ConnascenceKind = "type"
	ConnascenceMeaning   ConnascenceKind = "meaning"
	ConnascenceAlgorithm ConnascenceKind = "algorithm"
	ConnascencePosition  ConnascenceKind = "position"
	ConnascenceExecution ConnascenceKind = "execution"
	ConnascenceTiming    ConnascenceKind = "timing"
	ConnascenceValue     ConnascenceKind = "value"
	ConnascenceIdentity  ConnascenceKind = "identity"
)

// ConnascenceEvidence is one deterministic static connascence fact attached to
// an edge classification. Source names the compiler/tool fact that produced it;
// Detail is optional context for humans. It is report-only.
type ConnascenceEvidence struct {
	Kind   ConnascenceKind `json:"kind"`
	Source string          `json:"source"`
	Detail string          `json:"detail,omitempty"`
}

// Location is a report-only file location attached to coupling evidence.
type Location struct {
	File string `json:"file"`
	Line int    `json:"line,omitempty"`
}

// Explicitness classifies whether the coupling is via a declared contract.
type Explicitness string

// Explicitness constants. Derived from strength (contract→explicit,
// intrusive→implicit) or an extractor AST hint when present.
const (
	ExplicitnessExplicit Explicitness = "explicit"
	ExplicitnessImplicit Explicitness = "implicit"
	ExplicitnessUnknown  Explicitness = "unknown"
)

// DistanceBasis records which signal drove the distance composite for an edge.
// Set by classify.Run; empty (DistanceBasisUnknown) when distance is unknown or same_module.
type DistanceBasis string

// DistanceBasis signal constants. DistanceBasisUnknown (empty string) is used for
// same_module and unknown-distance edges and omits from JSON output via omitempty.
const (
	DistanceBasisUnknown    DistanceBasis = ""                  // same_module or unknown distance
	DistanceBasisStructure  DistanceBasis = "code_structure"    // structural tree-distance fallback
	DistanceBasisOwnership  DistanceBasis = "ownership"         // explicit or multi-owner signal
	DistanceBasisDeployUnit DistanceBasis = "deploy_unit"       // differing deploy units (absolute)
	DistanceBasisExternal   DistanceBasis = "declared_external" // target matched an external_systems entry
)

// EdgeScore is the pure score result carried by a classified relationship.
type EdgeScore struct {
	Scored       bool
	Balance      int
	Value        int
	Band         Severity
	Reason       string
	Breakdown    ScoreBreakdown
	CheapestMove string
}

// ScoreBreakdown records the raw ordinals used by a relationship scorer.
type ScoreBreakdown struct {
	StrengthVal   int
	DistanceVal   int
	VolatilityVal int
	Modularity    int
	VolDiscount   int
}

// Classification holds the Balanced Coupling assessment for one graph edge.
// It carries evidence and a score but no scoring behavior.
type Classification struct {
	Strength            Strength
	Distance            Distance
	Volatility          Volatility
	Explicitness        Explicitness
	Severity            Severity
	ContractRecommended bool      // generic-subdomain target reached via non-contract strength
	Score               EdgeScore // numeric score; zero when not scored
	// DistanceBasis records which signal drove the composite distance.
	// Report-only — not fed into severity or scoring.
	DistanceBasis DistanceBasis `json:"distance_basis,omitempty"`
	// CloneLocations carries the real duplicated-code file:line locations (both
	// sides) that drove a Symmetric-strength upgrade from cross-module clone
	// detection. Populated only when the upgrade actually fired from a clone
	// pair (classify.go); empty for edges whose Symmetric strength came from any
	// other source, including an extractor's Symmetric StrengthHint. Report-only
	// — appended onto the finding's Locations downstream (engine/advisories.go),
	// never fed into distance/volatility/scoring.
	CloneLocations []Location `json:"clone_locations,omitempty"`
	// StrengthFromLLM records that Strength came from an approved
	// llm-provenance label filling a cell every static source left unknown.
	// Report-only — drives the classified_edges.labeled_llm disclosure count,
	// never scoring.
	StrengthFromLLM bool `json:"strength_from_llm,omitempty"`
	// StrengthFromNonHighLLM records that StrengthFromLLM came from a label whose
	// confidence was not high. Report-only — score confidence consumes the applied
	// edge count rather than raw approved-label rows.
	StrengthFromNonHighLLM bool `json:"strength_from_non_high_llm,omitempty"`
	// StrengthFromConnascence records that a deterministic static connascence fact
	// (meaning/algorithm/position) refined an otherwise unresolved or public-floor
	// strength to model or functional. Report-only disclosure of a deterministic
	// fallback path; never affects distance, explicitness, or confidence bands.
	StrengthFromConnascence bool `json:"strength_from_connascence,omitempty"`
	// Connascence carries deterministic static connascence evidence for this edge.
	// The summary block is report-only; the evidence may also refine an otherwise
	// unresolved/public-floor strength through StrengthFromConnascence.
	Connascence []ConnascenceEvidence `json:"connascence,omitempty"`
}

// Index maps each edge's canonical key (from + "\x00" + to + "\x00" + kind)
// to its Classification. Built by classify.Run; consumed by rules and metrics.
type Index map[string]Classification

// Severity expresses the risk level of an imbalanced or intrusive coupling edge.
// Empty string means no finding (balanced). It is an alias of relationship.Severity.
type Severity = relationship.Severity

// Severity constants ordered from no finding to highest risk.
const (
	SeverityNone     = relationship.SeverityNone
	SeverityLow      = relationship.SeverityLow
	SeverityMedium   = relationship.SeverityMedium
	SeverityHigh     = relationship.SeverityHigh
	SeverityCritical = relationship.SeverityCritical
)

// DistanceIsHigh returns true for distances that represent a large socio-technical
// gap — a different owner, a separate deployment unit, or a declared external
// system (a different vendor entirely). These are the only distances at which
// tight coupling is a genuine "distributed monolith"; coupling at
// cross_module_same_owner (a single owner/binary) is local, and its cascade is
// cheap, so it must not be framed as distributed-monolith risk.
func DistanceIsHigh(d Distance) bool {
	return d == DistanceCrossModuleDiffOwner || d == DistanceCrossDeployUnit || d == DistanceExternal
}
