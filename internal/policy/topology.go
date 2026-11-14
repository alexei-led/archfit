package policy

// ExternalSystemDef declares one external integration seam (`external_systems:`),
// per book Ch10 Example 1 (cross-vendor integration). Edges whose target matches
// a Targets glob enter coupling_balance scoring at the distance ladder's far end
// (declared_external, D=10) instead of the disclosed external exclusion.
// Scoring EVERY library import at D=10 would flood the metric with vendor noise —
// only a declared seam (a vendor SDK, a generated client) is an integration the
// architect chose to measure.
type ExternalSystemDef struct {
	// Targets are glob patterns matched against the classified edge target —
	// the same node identity the language extractor emits: a Go import path
	// ("github.com/aws/aws-sdk-go-v2/**"), a TS resolved package path
	// ("node_modules/@aws-sdk/**") or bare specifier, a Python dotted module
	// ("boto3.**"), or a Rust crate name ("aws_sdk_s3").
	Targets []string `yaml:"targets"`
	// Volatility of the external system: high | medium | low | frozen.
	// Default low — the book's generic-subdomain guidance (an external vendor
	// system is a generic capability, presumed stable unless declared otherwise).
	Volatility string `yaml:"volatility,omitempty"`
}

// DuplicatedKnowledgePolicy preserves the retired Archfit 3 YAML values for conversion.
type DuplicatedKnowledgePolicy string

const (
	// DuplicatedKnowledgePolicyScore is the retired score mode.
	DuplicatedKnowledgePolicyScore DuplicatedKnowledgePolicy = "score"
	// DuplicatedKnowledgePolicyAdvisory is the retired advisory mode.
	DuplicatedKnowledgePolicyAdvisory DuplicatedKnowledgePolicy = "advisory"
)
