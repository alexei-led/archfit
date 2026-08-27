package config

import (
	evidenceports "github.com/alexei-led/archfit/internal/evidence/ports"
	"github.com/alexei-led/archfit/internal/policy"
)

// ToolMode is the public config lifecycle view of a tool enable state.
type ToolMode = evidenceports.ToolMode

// Tool enable modes.
const (
	ModeAuto = evidenceports.ModeAuto
	ModeOn   = evidenceports.ModeOn
	ModeOff  = evidenceports.ModeOff
)

// ModuleDef is the policy-facing architectural module definition.
type ModuleDef = policy.ModuleDef

// ModuleMap is the policy-facing module lookup contract.
type ModuleMap = policy.ModuleMap

// Module role values exposed through the policy contract.
const (
	RoleCompositionRoot = policy.RoleCompositionRoot
	RoleAdapter         = policy.RoleAdapter
	RoleCore            = policy.RoleCore
	RoleSharedModel     = policy.RoleSharedModel
	RoleGenerated       = policy.RoleGenerated
	RoleTest            = policy.RoleTest
)

// ModuleRootDirs returns the configured source roots for each module.
func ModuleRootDirs(modules map[string]ModuleDef) map[string]string {
	return policy.ModuleRootDirs(modules)
}

// MetricsConfig holds settings for all metrics, keyed by metric name.
type MetricsConfig map[string]policy.MetricEntry

// ModuleReviewConfig configures staleness gating of the module declarations:
// archfit warns (or fails) when a module's `reviewed_at` is older than
// stale_after, nudging a periodic re-check of the architecture map.
type ModuleReviewConfig struct {
	StaleAfter string `yaml:"stale_after,omitempty"`
	Gate       string `yaml:"gate,omitempty"`
}

// CouplingConfig tunes the Balanced-Coupling advisory pass (`coupling:`).
type CouplingConfig struct {
	// MinSeverity is the minimum severity for a coupling advisory to appear:
	// low | medium | high | critical (default medium). low is noisy on
	// well-designed codebases; high/critical surface only intrusive/functional
	// coupling across large boundaries.
	MinSeverity string `yaml:"min_severity,omitempty"`
	// DuplicatedKnowledge controls clone-only duplicated knowledge (cross-module
	// clone pairs with no import edge). "score" (default) includes those pairs in
	// coupling_balance; "advisory" preserves the v4 report-only behavior.
	DuplicatedKnowledge policy.DuplicatedKnowledgePolicy `yaml:"duplicated_knowledge,omitempty"`
	// VolatilityCascade enables the book Ch9 propagation pass: a module strongly
	// coupled to a high-effective-volatility module inherits raised effective
	// volatility. The pass runs to a deterministic fixpoint and never lowers values.
	VolatilityCascade bool `yaml:"volatility_cascade,omitempty"`
	// Gate configures the coupling verdict gate (`coupling.gate:`). An absent
	// block is not an absent policy: the distributed-monolith rule always runs
	// and defaults to mode warn with max_new_seams 0.
	Gate *CouplingGateDef `yaml:"gate,omitempty"`
}

// CouplingGateDef configures the coupling verdict gate (`coupling.gate:`).
type CouplingGateDef struct {
	// MinBand and MaxDrop are RETIRED v1 knobs. They gated the verdict on the
	// repository coupling scalar, which schema v2 removes as a decision input.
	//
	// They remain decodable for exactly one reason: `config update
	// --migration-only` has to read a v1 file to migrate it. validate() rejects
	// them with the migration command, so no analysis can consume them.
	MinBand string `yaml:"min_band,omitempty"`
	MaxDrop *int   `yaml:"max_drop,omitempty"`
	// DistributedMonolith is the v2 coupling gate: it counts logical seams, not
	// import edges, and blocks only on newly introduced qualifying seams
	// against a comparable reference.
	DistributedMonolith *DistributedMonolithDef `yaml:"distributed_monolith,omitempty"`
}

// DistributedMonolithDef configures the distributed-monolith seam rule
// (`coupling.gate.distributed_monolith:`).
//
// A qualifying seam is one ordered module pair with at least one active edge in
// the critical band at high distance (different owner or different deploy
// unit). The rule counts seams, not edges: forty imports expressing one seam
// are one seam.
type DistributedMonolithDef struct {
	// Mode is warn (diagnostic, the default) or fail (blocking). Fail is opt-in
	// and is never inferred by the migration: it stays an owner decision taken
	// after a report-only run against a comparable reference.
	Mode string `yaml:"mode,omitempty"`
	// MaxNewSeams is the tolerated number of newly introduced qualifying seams.
	// Unset means 0. It applies only in fail mode and only when the comparison
	// reference is comparable.
	MaxNewSeams *int `yaml:"max_new_seams,omitempty"`
}

// OutputsConfig controls which output formats are produced.
type OutputsConfig struct {
	JSON     bool `yaml:"json"`
	Markdown bool `yaml:"markdown"`
	SARIF    bool `yaml:"sarif"`
}

// ---------------------------------------------------------------------------
// View types — narrow projections of Config passed to each pipeline stage.
// ---------------------------------------------------------------------------

// FileClassDef holds optional user-supplied patterns for source-file
// classification. Auto-detection (generated-header sniff, naming conventions,
// language test patterns) runs first; these fields extend or fine-tune it for
// custom mock frameworks and project-specific conventions.
type FileClassDef struct {
	// GeneratedGlobs are glob patterns (matched against the repo-relative
	// slash path) that classify a file as Generated. Supports doublestar (`**`)
	// semantics (github.com/bmatcuk/doublestar); `**` matches across path
	// separators. Example: ["**/generated/**", "*.pb.go"]
	GeneratedGlobs []string `yaml:"generated_globs,omitempty"`
	// TestGlobs are glob patterns that classify a file as Test.
	// Example: ["*_helpers_test.go", "testutil/**"]
	TestGlobs []string `yaml:"test_globs,omitempty"`
	// MockFrameworks contains filename prefix/suffix patterns (matched against
	// the base filename only) that identify mock files as Generated. Used for
	// custom mock code-generators not covered by built-in naming conventions.
	// Example: ["fake_", "_double"]
	MockFrameworks []string `yaml:"mock_frameworks,omitempty"`
}
