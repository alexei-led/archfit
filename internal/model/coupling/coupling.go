package coupling

// Strength classifies how a dependency is expressed at the API boundary.
type Strength string

// Strength constants (spec §18).
// contract and intrusive are decided deterministically (config globs,
// visibility); model and functional come from SCIP symbol-kind heuristics —
// the Tranche-2 enrich workflow refines those labels under human review.
const (
	StrengthContract   Strength = "contract"
	StrengthIntrusive  Strength = "intrusive"
	StrengthModel      Strength = "model"
	StrengthFunctional Strength = "functional"
	// StrengthSymmetric is bidirectional coupling at implementation level —
	// book ordinal 9, between functional (8) and intrusive (10).
	StrengthSymmetric Strength = "symmetric"
	StrengthUnknown   Strength = "unknown"
)

// Distance measures how far apart two modules are in the ownership hierarchy.
type Distance string

// Distance constants (spec §18).
const (
	DistanceSameModule           Distance = "same_module"
	DistanceCrossModuleSameOwner Distance = "cross_module_same_owner"
	DistanceCrossModuleDiffOwner Distance = "cross_module_different_owner"
	DistanceCrossDeployUnit      Distance = "cross_deploy_unit"
	DistanceUnknown              Distance = "unknown"
)

// Volatility classifies how likely a module's API is to change.
// Per Khononov, volatility is derived from the DDD subdomain (core→high,
// supporting→medium, generic→low) with an explicit per-module override.
type Volatility string

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
	VolatilityHigh       Volatility = "high"
	VolatilityMedium     Volatility = "medium"
	VolatilityLow        Volatility = "low"
	VolatilityUndeclared Volatility = "undeclared"
	VolatilityUnknown    Volatility = "unknown"
)

// VolatilityResolved reports whether v is a concrete level the tool can act on
// (high/medium/low), as opposed to undeclared (config gap) or unknown
// (unresolvable). Callers that need "do we actually have a volatility?" should
// use this rather than comparing against VolatilityUnknown alone.
func VolatilityResolved(v Volatility) bool {
	return v == VolatilityHigh || v == VolatilityMedium || v == VolatilityLow
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
	DistanceBasisUnknown    DistanceBasis = ""               // same_module or unknown distance
	DistanceBasisStructure  DistanceBasis = "code_structure" // structural tree-distance fallback
	DistanceBasisOwnership  DistanceBasis = "ownership"      // explicit or multi-owner signal
	DistanceBasisDeployUnit DistanceBasis = "deploy_unit"    // differing deploy units (absolute)
)

// Connascence is the degree of connascence detected on a cross-module edge.
// Report-only vocabulary — never scored, never gates.
type Connascence string

// Connascence degree constants. ConnascenceNone means no connascence detected.
const (
	ConnascenceNone      Connascence = ""
	ConnascenceType      Connascence = "type"      // CoT: struct/interface/field use
	ConnascenceAlgorithm Connascence = "algorithm" // CoA: clone pair crosses module boundary
)

// Classification holds the Balanced Coupling assessment for one graph edge.
// Strength and Distance are populated with high confidence;
// Volatility and Explicitness are derived from config subdomain/public globs.
// Severity is set by classify.Run for cross-boundary edges via BalanceResult.
// ContractRecommended is set when the target is a generic subdomain reached via
// non-contract strength — BC's anti-corruption-layer advisory signal.
// Score holds the continuous numeric risk score when a Scorer has been applied;
// zero-value when no scorer is configured (e.g. same-module or unknown-distance edges).
type Classification struct {
	Strength            Strength
	Distance            Distance
	Volatility          Volatility
	Explicitness        Explicitness
	Severity            Severity
	ContractRecommended bool      // generic-subdomain target reached via non-contract strength
	Score               EdgeScore // numeric score; zero when not scored
	// Connascence is the detected connascence degree for this edge.
	// CoT (type) when the edge is a cross-module struct/interface/field use,
	// derivable from model/contract strength with a SCIP hint.
	// CoA (algorithm) when a clone pair crosses this module boundary.
	// Report-only — not fed into the scorer or any gate decision.
	Connascence Connascence `json:"connascence,omitempty"`
	// DistanceBasis records which signal drove the composite distance.
	// Report-only — not fed into severity or scoring.
	DistanceBasis DistanceBasis `json:"distance_basis,omitempty"`
}

// Index maps each edge's canonical key (from + "\x00" + to + "\x00" + kind)
// to its Classification. Built by classify.Run; consumed by rules and metrics.
type Index map[string]Classification

// Severity expresses the risk level of an imbalanced or intrusive coupling edge.
// Empty string means no finding (balanced).
type Severity string

// Severity constants ordered from no finding to highest risk.
const (
	SeverityNone     Severity = ""
	SeverityLow      Severity = "low"
	SeverityMedium   Severity = "medium"
	SeverityHigh     Severity = "high"
	SeverityCritical Severity = "critical"
)

// strengthIsHigh returns true for strengths that represent high coupling intensity.
// Contract and model are low-coupling (explicit, stable API surface).
// Functional and intrusive are high-coupling (implementation-level dependency).
func strengthIsHigh(s Strength) bool {
	return s == StrengthFunctional || s == StrengthSymmetric || s == StrengthIntrusive
}

// distanceIsHigh returns true for distances that represent a large ownership gap.
func distanceIsHigh(d Distance) bool {
	return d == DistanceCrossModuleDiffOwner || d == DistanceCrossDeployUnit
}

// BalanceResult applies the Khononov balance formula to a Classification and returns
// the advisory Severity for the edge. SeverityNone means the edge is balanced (no finding).
//
// Severity table (Balanced Coupling model, Khononov):
//   - Intrusive: always surfaced, severity driven by distance/volatility.
//   - high strength + high distance + high volatility → critical.
//   - high strength + high distance + medium/undeclared/unknown volatility → medium.
//   - high strength + high distance + low volatility → low (BC: stable target neutralizes).
//   - low strength + low distance + high volatility → medium (over-decoupled volatile seam).
//   - high strength + low distance → none (cohesive — XOR modular quadrant).
//   - low strength + high distance → none (loose — XOR modular quadrant).
//   - low strength + low distance + low/unknown volatility → none (balanced).
func BalanceResult(c Classification) Severity {
	// Intrusive strength: always advisory, severity driven by distance.
	if c.Strength == StrengthIntrusive {
		if c.Distance == DistanceCrossDeployUnit {
			return SeverityCritical
		}
		if c.Distance == DistanceCrossModuleDiffOwner {
			if c.Volatility == VolatilityHigh {
				return SeverityHigh
			}
			return SeverityMedium
		}
		// intrusive + same-module or cross-module-same-owner: fall through to formula.
	}

	sHigh := strengthIsHigh(c.Strength)
	dHigh := distanceIsHigh(c.Distance)

	if sHigh == dHigh {
		if sHigh {
			// high strength + high distance: tight coupling across a large boundary.
			// Volatility modulates per BC: a low-volatility (stable) target neutralizes
			// the imbalance — the cascade rarely fires — so it drops to an advisory low;
			// high volatility amplifies it to critical.
			switch c.Volatility {
			case VolatilityHigh:
				return SeverityCritical
			case VolatilityLow:
				return SeverityLow
			default: // medium, unknown — conservative
				return SeverityMedium
			}
		}
		// low strength + low distance: over-decoupled volatile seam.
		if c.Volatility == VolatilityHigh {
			return SeverityMedium
		}
		return SeverityNone
	}

	// Asymmetric (XOR modular quadrants):
	//   high strength + low distance → cohesive (co-located tight coupling, acceptable).
	//   low strength + high distance → loose (contract across a large boundary, acceptable).
	// Both are BC-modular: no finding.
	return SeverityNone
}
