// Package state owns the report's nine measurement envelopes and evidence
// completeness contracts. It is dependency-free; application projects these
// values into the report DTOs.
package state

// MeasurementStatus records whether a dimension's evidence was actually
// gathered. Missing evidence is explicit, never an implicit green result.
type MeasurementStatus string

// Measurement statuses for one dimension envelope.
const (
	Measured   MeasurementStatus = "measured"
	Partial    MeasurementStatus = "partial"
	Unmeasured MeasurementStatus = "unmeasured"
)

// Confidence is a dimension's confidence in its own evidence.
type Confidence string

// Confidence rungs. Unrated is the abstain rung: the evidence supports no
// confidence claim at all.
const (
	ConfidenceHigh    Confidence = "high"
	ConfidenceMedium  Confidence = "medium"
	ConfidenceLow     Confidence = "low"
	ConfidenceUnrated Confidence = "unrated"
)

// DimensionCount is the fixed number of architecture-state dimensions. Coverage
// counts must sum to it.
const DimensionCount = 9

// Dimension names. They are the envelope's Name field and the report's JSON
// keys.
const (
	DimensionIntent         = "intent"
	DimensionStructure      = "structure"
	DimensionModularity     = "modularity"
	DimensionCoupling       = "coupling"
	DimensionChangeLocality = "change_locality"
	DimensionComplexity     = "complexity"
	DimensionTestability    = "testability"
	DimensionOperations     = "operations"
	DimensionDrift          = "drift"
)

// Evidence owners for each dimension. Every dimension names the capability that
// produces its facts; an envelope with no owner cannot be measured by anyone.
const (
	OwnerIntent          = "policy+assessment/evaluation"
	OwnerStructure       = "relationship/facts"
	OwnerModularity      = "assessment/metrics"
	OwnerCoupling        = "relationship/analysis"
	OwnerChangeLocality  = "history/git"
	OwnerComplexity      = "relationship/analysis+syntax+evidence/acquisition"
	OwnerComplexityGraph = "relationship/analysis"
	OwnerComplexitySize  = "syntax+evidence/acquisition"
	OwnerTestability     = "syntax/fileclass"
	OwnerOperations      = "policy+evidence/acquisition"
	OwnerDrift           = "assessment/decision"
)

// MetricDenominator is the observed-over-total coverage of one metric. It makes
// a zero value distinguishable from an unmeasured one.
type MetricDenominator struct {
	Observed int
	Total    int
}

// MetricValue is one typed metric record. A dimension's metrics are always a
// slice of these, never a free-form map: the envelope is a reporting contract,
// not a property bag.
type MetricValue struct {
	Name        string
	Value       float64
	Unit        string
	Denominator *MetricDenominator
	Provenance  []string
}

// FindingRef points a dimension envelope at a finding the diagnostic already
// owns. Findings are never duplicated per dimension, so identity, status, and
// ordering keep exactly one owner.
type FindingRef struct {
	ID       string
	RuleID   string
	Kind     string
	Severity string
	Status   string
}

// RequiredFact is one fixed fact kind in a dimension's evidence contract.
// InClaim facts decide promotion. Out-of-claim facts document which unknown
// disclosures may coexist with a measured dimension without weakening its
// claim.
type RequiredFact struct {
	Name    string
	Owner   string
	InClaim bool
}

// Required fact names. Collectors use these constants when reporting missing
// evidence so contract tests can distinguish in-claim gaps from permitted
// out-of-claim disclosures without matching prose.
const (
	FactDeclaredIntentInventory          = "declared intent inventory"
	FactPrimaryDependencyInventory       = "primary dependency inventory"
	FactInternalEdgeClassification       = "internal edge classification"
	FactExternalDependencyStructure      = "external dependency structure"
	FactDeclaredModuleInventory          = "declared module inventory"
	FactModuleBoundaryAttribution        = "module boundary attribution"
	FactModuleGraphShape                 = "module graph shape"
	FactInferredPublicSurface            = "inferred public surface"
	FactCouplingCandidateInventory       = "coupling candidate inventory"
	FactCouplingStrength                 = "coupling strength"
	FactCouplingDistance                 = "coupling distance"
	FactCouplingVolatility               = "coupling volatility"
	FactExtractorResolutionWithinCeiling = "extractor resolution within ceiling"
	FactLocalAndUndeclaredCoupling       = "local and undeclared-external coupling"
	FactEligibleCommitSample             = "eligible commit sample"
	FactCommitModuleAttribution          = "commit-to-module attribution"
	FactEssentialAccidentalVolatility    = "essential vs accidental volatility"
	FactDeclaredModuleGraph              = "declared module graph"
	FactDependencyChainDepth             = "dependency chain depth"
	FactModuleFanInDistribution          = "module fan-in distribution"
	FactModuleFanOutDistribution         = "module fan-out distribution"
	FactCodeSizeTail                     = "code size tail"
	FactFunctionLengthDistribution       = "function length distribution"
	FactCognitiveComplexity              = "cognitive complexity"
	FactProductionSourceInventory        = "production source inventory"
	FactSuppliedCoverageUnits            = "supplied coverage units"
	FactCoveragePathResolution           = "coverage path resolution"
	FactCoverageModuleAttribution        = "coverage module attribution"
	FactCoverageFreshness                = "coverage freshness"
	FactAssertionQuality                 = "assertion quality"
	FactBoundaryTestSemantics            = "boundary test semantics"
	FactDeclaredOperationalTopology      = "declared operational topology"
	FactCorroboratedDeployUnit           = "corroborated deploy unit"
	FactOwnerProvenance                  = "owner provenance"
	FactTopologyReconciliation           = "declared-to-corroborated topology reconciliation"
	FactRuntimeTopology                  = "observed runtime topology"
	FactSupplyChainInventory             = "supply-chain inventory"
	FactAnalyzerHealth                   = "analyzer health"
	FactAdmissiblePersistedReference     = "admissible persisted reference"
	FactCompleteTwoSidedSeamIdentity     = "complete two-sided seam identity"
	FactBaseComparison                   = "base comparison"
)

// RequiredFacts returns the source-fixed fact contract for one dimension.
// Runtime configuration may change which producers are applicable, but it can
// never remove a fact kind from this set.
func RequiredFacts(dimension string) []RequiredFact {
	required := func(name, owner string) RequiredFact {
		return RequiredFact{Name: name, Owner: owner, InClaim: true}
	}
	outOfClaim := func(name, owner string) RequiredFact {
		return RequiredFact{Name: name, Owner: owner, InClaim: false}
	}
	switch dimension {
	case DimensionIntent:
		return []RequiredFact{
			required(FactDeclaredIntentInventory, OwnerIntent),
		}
	case DimensionStructure:
		return []RequiredFact{
			required(FactPrimaryDependencyInventory, OwnerStructure),
			required(FactInternalEdgeClassification, OwnerStructure),
			outOfClaim(FactExternalDependencyStructure, OwnerStructure),
		}
	case DimensionModularity:
		return []RequiredFact{
			required(FactDeclaredModuleInventory, OwnerModularity),
			required(FactModuleBoundaryAttribution, OwnerModularity),
			required(FactModuleGraphShape, OwnerModularity),
			outOfClaim(FactInferredPublicSurface, OwnerModularity),
		}
	case DimensionCoupling:
		return []RequiredFact{
			required(FactCouplingCandidateInventory, OwnerCoupling),
			required(FactCouplingStrength, OwnerCoupling),
			required(FactCouplingDistance, OwnerCoupling),
			required(FactCouplingVolatility, OwnerCoupling),
			required(FactExtractorResolutionWithinCeiling, OwnerCoupling),
			outOfClaim(FactLocalAndUndeclaredCoupling, OwnerCoupling),
		}
	case DimensionChangeLocality:
		return []RequiredFact{
			required(FactEligibleCommitSample, OwnerChangeLocality),
			required(FactCommitModuleAttribution, OwnerChangeLocality),
			outOfClaim(FactEssentialAccidentalVolatility, OwnerChangeLocality),
		}
	case DimensionComplexity:
		return []RequiredFact{
			required(FactDeclaredModuleGraph, OwnerComplexityGraph),
			required(FactDependencyChainDepth, OwnerComplexityGraph),
			required(FactModuleFanInDistribution, OwnerComplexityGraph),
			required(FactModuleFanOutDistribution, OwnerComplexityGraph),
			outOfClaim(FactCodeSizeTail, OwnerComplexitySize),
			outOfClaim(FactFunctionLengthDistribution, OwnerComplexitySize),
			outOfClaim(FactCognitiveComplexity, OwnerComplexitySize),
		}
	case DimensionTestability:
		return []RequiredFact{
			required(FactProductionSourceInventory, OwnerTestability),
			required(FactSuppliedCoverageUnits, OwnerTestability),
			required(FactCoveragePathResolution, OwnerTestability),
			required(FactCoverageModuleAttribution, OwnerTestability),
			required(FactCoverageFreshness, OwnerTestability),
			outOfClaim(FactAssertionQuality, OwnerTestability),
			outOfClaim(FactBoundaryTestSemantics, OwnerTestability),
		}
	case DimensionOperations:
		return []RequiredFact{
			required(FactDeclaredOperationalTopology, OwnerOperations),
			required(FactCorroboratedDeployUnit, OwnerOperations),
			required(FactOwnerProvenance, OwnerOperations),
			required(FactTopologyReconciliation, OwnerOperations),
			outOfClaim(FactRuntimeTopology, OwnerOperations),
			outOfClaim(FactSupplyChainInventory, OwnerOperations),
			outOfClaim(FactAnalyzerHealth, OwnerOperations),
		}
	case DimensionDrift:
		return []RequiredFact{
			required(FactAdmissiblePersistedReference, OwnerDrift),
			required(FactCompleteTwoSidedSeamIdentity, OwnerDrift),
			outOfClaim(FactBaseComparison, OwnerDrift),
		}
	default:
		return nil
	}
}

// Promote applies the shared evidence-completeness rule for one applicable
// dimension. Callers establish applicability before invoking it. Required facts
// covered by observed or notApplicable produce measured; a missing set with at
// least one observed in-claim fact produces partial; no observed in-claim fact
// produces unmeasured. Every missing in-claim fact is returned explicitly.
func Promote(dimension string, observed, notApplicable []string) (MeasurementStatus, []UnknownFact) {
	observedSet := make(map[string]struct{}, len(observed))
	for _, name := range observed {
		observedSet[name] = struct{}{}
	}
	notApplicableSet := make(map[string]struct{}, len(notApplicable))
	for _, name := range notApplicable {
		notApplicableSet[name] = struct{}{}
	}

	observedInClaim := 0
	missing := make([]UnknownFact, 0)
	for _, fact := range RequiredFacts(dimension) {
		if !fact.InClaim {
			continue
		}
		if _, ok := observedSet[fact.Name]; ok {
			observedInClaim++
			continue
		}
		if _, ok := notApplicableSet[fact.Name]; ok {
			continue
		}
		missing = append(missing, UnknownFact{
			Fact: fact.Name, Reason: "the required fact was not observed or proved not applicable", Owner: fact.Owner,
		})
	}
	if len(missing) == 0 {
		return Measured, []UnknownFact{}
	}
	if observedInClaim > 0 {
		return Partial, missing
	}
	return Unmeasured, missing
}

// UnknownFact names one thing a dimension could not observe, why, and which
// capability would have to observe it. It is how partial/unmeasured states stay
// honest instead of silently reading as zero.
type UnknownFact struct {
	Fact   string
	Reason string
	Owner  string
}

// Coverage is a dimension's denominator: what was counted, how much of it was
// observed, and out of how much.
type Coverage struct {
	Basis    string
	Observed int
	Total    int
}

// Dimension is the one common envelope every architecture-state dimension uses.
type Dimension struct {
	Name       string
	Owner      string
	Status     MeasurementStatus
	Confidence Confidence
	Coverage   Coverage
	Metrics    []MetricValue
	Findings   []FindingRef
	Unknown    []UnknownFact
}

// Dimensions holds the nine envelopes as named fields rather than a map: each
// dimension has a compile-time name and owner, and the contract order is fixed
// by declaration order instead of by map iteration.
type Dimensions struct {
	Intent         Dimension
	Structure      Dimension
	Modularity     Dimension
	Coupling       Dimension
	ChangeLocality Dimension
	Complexity     Dimension
	Testability    Dimension
	Operations     Dimension
	Drift          Dimension
}

// All returns the nine envelopes in their fixed contract order.
func (d Dimensions) All() []Dimension {
	return []Dimension{
		d.Intent, d.Structure, d.Modularity, d.Coupling, d.ChangeLocality,
		d.Complexity, d.Testability, d.Operations, d.Drift,
	}
}

// Each returns pointers to the nine envelopes in contract order so collectors
// can fill them in place without repeating the field list.
func (d *Dimensions) Each() []*Dimension {
	return []*Dimension{
		&d.Intent, &d.Structure, &d.Modularity, &d.Coupling, &d.ChangeLocality,
		&d.Complexity, &d.Testability, &d.Operations, &d.Drift,
	}
}

// CountStatuses tallies the nine envelopes by measurement status. It is purely
// mechanical — no policy or thresholds participate.
func (d Dimensions) CountStatuses() (measured, partial, unmeasured int) {
	for _, dim := range d.All() {
		switch dim.Status {
		case Measured:
			measured++
		case Partial:
			partial++
		default:
			unmeasured++
		}
	}
	return measured, partial, unmeasured
}

// Architecture contains the report measurement envelopes, without a verdict.
type Architecture struct{ Dimensions Dimensions }

// ConfidenceFor is the confidence a measurement status supports on its own.
//
// V1 derives confidence from status because status is the one evidence-quality
// fact all nine dimensions share. A dimension with a sharper signal — a
// low-confidence contributing metric, a truncated tool run — lowers it further
// from its own evidence; nothing may raise it above this rung.
func ConfidenceFor(s MeasurementStatus) Confidence {
	switch s {
	case Measured:
		return ConfidenceHigh
	case Partial:
		return ConfidenceMedium
	default:
		return ConfidenceUnrated
	}
}

// NewDimension returns one honestly unmeasured envelope with deterministic
// non-nil collections. Collectors start here and fill in what they observed.
func NewDimension(name, owner string) Dimension {
	return Dimension{
		Name: name, Owner: owner,
		Status: Unmeasured, Confidence: ConfidenceUnrated,
		Metrics: []MetricValue{}, Findings: []FindingRef{}, Unknown: []UnknownFact{},
	}
}

// New returns nine named, owned, unmeasured envelopes with deterministic
// collections and explicit missing-evidence reasons.
func New() Architecture {
	st := Architecture{
		Dimensions: Dimensions{
			Intent:         NewDimension(DimensionIntent, OwnerIntent),
			Structure:      NewDimension(DimensionStructure, OwnerStructure),
			Modularity:     NewDimension(DimensionModularity, OwnerModularity),
			Coupling:       NewDimension(DimensionCoupling, OwnerCoupling),
			ChangeLocality: NewDimension(DimensionChangeLocality, OwnerChangeLocality),
			Complexity:     NewDimension(DimensionComplexity, OwnerComplexity),
			Testability:    NewDimension(DimensionTestability, OwnerTestability),
			Operations:     NewDimension(DimensionOperations, OwnerOperations),
			Drift:          NewDimension(DimensionDrift, OwnerDrift),
		},
	}
	for _, dim := range st.Dimensions.Each() {
		// An unmeasured envelope with no stated reason is indistinguishable from
		// one nobody bothered to explain. Collectors build their envelopes with
		// NewDimension and state their own reason; this default covers the
		// envelopes no collector reached.
		dim.Unknown = []UnknownFact{{
			Fact:   dim.Name,
			Reason: "no collector reported this dimension",
			Owner:  dim.Owner,
		}}
	}
	return st
}
