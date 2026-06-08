package coupling

// Strength classifies how a dependency is expressed at the API boundary.
type Strength string

// Strength constants (spec §18 / Phase 1 subset).
// Only contract and intrusive are high-confidence in Phase 1.
// model and functional are deferred to Phase 2.
const (
	StrengthContract   Strength = "contract"
	StrengthIntrusive  Strength = "intrusive"
	StrengthModel      Strength = "model"
	StrengthFunctional Strength = "functional"
	StrengthUnknown    Strength = "unknown"
)

// Distance measures how far apart two modules are in the ownership hierarchy.
type Distance string

// Distance constants (Phase 1 subset used by unbalanced-edge metric).
const (
	DistanceSameModule           Distance = "same_module"
	DistanceCrossModuleSameOwner Distance = "cross_module_same_owner"
	DistanceCrossModuleDiffOwner Distance = "cross_module_different_owner"
	DistanceCrossDeployUnit      Distance = "cross_deploy_unit"
	DistanceUnknown              Distance = "unknown"
)

// Volatility classifies how likely a module's API is to change.
type Volatility string

// Volatility constants derived from subdomain classification.
const (
	VolatilityHigh    Volatility = "high"
	VolatilityMedium  Volatility = "medium"
	VolatilityLow     Volatility = "low"
	VolatilityUnknown Volatility = "unknown"
)

// Explicitness classifies whether the coupling is via a declared contract.
type Explicitness string

// Explicitness constants (Phase 2 fills this out fully).
const (
	ExplicitnessExplicit Explicitness = "explicit"
	ExplicitnessImplicit Explicitness = "implicit"
	ExplicitnessUnknown  Explicitness = "unknown"
)

// Classification holds the Balanced Coupling assessment for one graph edge.
// Phase 1 populates Strength and Distance with high confidence;
// Volatility and Explicitness are derived from config subdomain/public globs.
// Severity is set by classify.Run for cross-boundary edges via BalanceResult.
type Classification struct {
	Strength     Strength
	Distance     Distance
	Volatility   Volatility
	Explicitness Explicitness
	Severity     Severity
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
	return s == StrengthFunctional || s == StrengthIntrusive
}

// distanceIsHigh returns true for distances that represent a large ownership gap.
func distanceIsHigh(d Distance) bool {
	return d == DistanceCrossModuleDiffOwner || d == DistanceCrossDeployUnit
}

// BalanceResult applies the Khononov balance formula to a Classification and returns
// the advisory Severity for the edge. SeverityNone means the edge is balanced (no finding).
//
// Severity table (spec §18):
//   - Intrusive: always surfaced, severity driven by distance/volatility.
//   - high strength + high distance + high volatility → critical.
//   - high strength + high distance + low/unknown volatility → medium.
//   - low strength + low distance + high volatility → medium (over-decoupled volatile seam).
//   - high strength + low distance → low (high cohesion, usually acceptable).
//   - low strength + high distance → low (loose coupling across a large boundary).
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
			if c.Volatility == VolatilityHigh {
				return SeverityCritical
			}
			return SeverityMedium
		}
		// low strength + low distance: over-decoupled volatile seam.
		if c.Volatility == VolatilityHigh {
			return SeverityMedium
		}
		return SeverityNone
	}

	// Asymmetric (low+high or high+low): mismatched but not the worst case.
	return SeverityLow
}
