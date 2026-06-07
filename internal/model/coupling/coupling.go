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
type Classification struct {
	Strength     Strength
	Distance     Distance
	Volatility   Volatility
	Explicitness Explicitness
}

// Index maps each edge's canonical key (from + "\x00" + to + "\x00" + kind)
// to its Classification. Built by classify.Run; consumed by rules and metrics.
type Index map[string]Classification
