package config

import (
	"fmt"
	"strconv"

	evidenceports "github.com/alexei-led/archfit/internal/evidence/ports"
	"github.com/alexei-led/archfit/internal/policy"

	"github.com/goccy/go-yaml"
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

// MetricsConfig holds gate settings for computed metrics plus diagnostic-only
// complexity settings. Entries stays inline so existing object-shaped metric
// YAML keeps its public form, while function_loc_threshold is the one reserved
// scalar in the same block.
type MetricsConfig struct {
	FunctionLOCThreshold *int                          `yaml:"function_loc_threshold,omitempty" jsonschema:"minimum=1,default=60"`
	Entries              map[string]policy.MetricEntry `yaml:"-"`
}

// UnmarshalYAML decodes the one reserved scalar separately from ordinary
// object-shaped metric entries. Re-decoding each object with strict mode keeps
// unknown fields loud even though the outer key set is intentionally dynamic.
func (m *MetricsConfig) UnmarshalYAML(unmarshal func(any) error) error {
	var raw map[string]any
	if err := unmarshal(&raw); err != nil {
		return err
	}
	m.FunctionLOCThreshold = nil
	m.Entries = make(map[string]policy.MetricEntry, len(raw))
	for name, value := range raw {
		if name == "function_loc_threshold" {
			var threshold int
			switch number := value.(type) {
			case uint64:
				if strconv.IntSize == 32 && number > uint64(^uint32(0)>>1) {
					return fmt.Errorf("metrics.function_loc_threshold must be an integer in range (got %d)", number)
				}
				if strconv.IntSize == 64 && number > ^uint64(0)>>1 {
					return fmt.Errorf("metrics.function_loc_threshold must be an integer in range (got %d)", number)
				}
				threshold = int(number) //nolint:gosec // range checked for the platform int size above
			case int64:
				if strconv.IntSize == 32 && (number < int64(-1<<31) || number > int64(1<<31-1)) {
					return fmt.Errorf("metrics.function_loc_threshold must be an integer in range (got %d)", number)
				}
				threshold = int(number)
			default:
				return fmt.Errorf("metrics.function_loc_threshold must be an integer (got %T)", value)
			}
			m.FunctionLOCThreshold = &threshold
			continue
		}
		encoded, err := yaml.Marshal(value)
		if err != nil {
			return fmt.Errorf("metrics.%s: encode value: %w", name, err)
		}
		var entry policy.MetricEntry
		if err := yaml.UnmarshalWithOptions(encoded, &entry, yaml.DisallowUnknownField()); err != nil {
			return fmt.Errorf("metrics.%s: %w", name, err)
		}
		m.Entries[name] = entry
	}
	return nil
}

// MarshalYAML preserves the public mixed map shape for config rewrite paths.
func (m MetricsConfig) MarshalYAML() (any, error) {
	out := make(map[string]any, len(m.Entries)+1)
	for name, entry := range m.Entries {
		out[name] = entry
	}
	if m.FunctionLOCThreshold != nil {
		out["function_loc_threshold"] = *m.FunctionLOCThreshold
	}
	return out, nil
}

// MetricEntries returns only computed-metric gate configuration. The reserved
// function-size diagnostic threshold never enters metric evaluation or gates.
func (m MetricsConfig) MetricEntries() map[string]policy.MetricEntry { return m.Entries }

// FunctionLOCThresholdValue returns the configured function-size tail threshold.
// The setting is diagnostic only; its absent value is the published default.
func (m MetricsConfig) FunctionLOCThresholdValue() int {
	if m.FunctionLOCThreshold == nil {
		return policy.DefaultFunctionLOCThreshold
	}
	return *m.FunctionLOCThreshold
}

// CoverageConfig configures opt-in ingestion of coverage artifacts produced by
// the caller's CI. Archfit reads these files; it never executes the target
// repository's tests.
type CoverageConfig struct {
	Enabled bool             `yaml:"enabled,omitempty" jsonschema:"default=false"`
	Gate    GateMode         `yaml:"gate,omitempty"`
	Sources []CoverageSource `yaml:"sources,omitempty"`
}

// CoverageSource is one supplied coverage artifact. Format defaults to auto;
// SidecarPath defaults to <path>.sidecar.json when omitted.
type CoverageSource struct {
	Path        string `yaml:"path" jsonschema:"required"`
	Format      string `yaml:"format,omitempty" jsonschema:"enum=auto,enum=go-coverprofile,enum=lcov,enum=coverage-py-json,enum=llvm-cov-json"`
	SidecarPath string `yaml:"sidecar_path,omitempty"`
}

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
