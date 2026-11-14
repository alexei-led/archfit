package config

import (
	evidenceports "github.com/alexei-led/archfit/v3/internal/evidence/ports"
	"github.com/alexei-led/archfit/v3/internal/policy"
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

// MetricsConfig holds internal metric settings and a diagnostic function-size threshold.
type MetricsConfig struct {
	FunctionLOCThreshold *int                          `yaml:"function_loc_threshold,omitempty" jsonschema:"minimum=1,default=60"`
	Entries              map[string]policy.MetricEntry `yaml:"-"`
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
// SidecarPath defaults to <path>.sidecar.json when omitted. Pointer limits
// distinguish omission (use the bounded default) from an explicit invalid zero.
type CoverageSource struct {
	Path        string `yaml:"path" jsonschema:"required"`
	Format      string `yaml:"format,omitempty" jsonschema:"enum=auto,enum=go-coverprofile,enum=lcov,enum=coverage-py-json,enum=llvm-cov-json,enum=cobertura"`
	SidecarPath string `yaml:"sidecar_path,omitempty"`
	MaxBytes    *int64 `yaml:"max_bytes,omitempty" jsonschema:"minimum=1,default=67108864"`
	MaxFacts    *int   `yaml:"max_facts,omitempty" jsonschema:"minimum=1,default=1000000"`
}

// ModuleReviewConfig configures the review of the module declarations:
// production source no module owns (map/uncovered_path, a blocker when gate is
// fail), module paths globs that match nothing, and modules whose
// `reviewed_at` is older than stale_after. Only unowned source can block.
type ModuleReviewConfig struct {
	StaleAfter string `yaml:"stale_after,omitempty"`
	Gate       string `yaml:"gate,omitempty"`
	// ChangeGate is the posture for production source a change adds that no
	// analysis covers (map/uncovered_change): off, warn (default), or fail.
	// fail makes the change unknown, never a pass.
	ChangeGate string `yaml:"change_gate,omitempty"`
}

// CouplingConfig tunes the Balanced-Coupling advisory pass (`coupling:`).
type CouplingConfig struct {
	// MinSeverity is the minimum severity for a coupling advisory to appear:
	// low | medium | high | critical (default medium). low is noisy on
	// well-designed codebases; high/critical surface only intrusive/functional
	// coupling across large boundaries.
	MinSeverity string `yaml:"min_severity,omitempty"`
	// VolatilityCascade enables the book Ch9 propagation pass: a module strongly
	// coupled to a high-effective-volatility module inherits raised effective
	// volatility. The pass runs to a deterministic fixpoint and never lowers values.
	VolatilityCascade bool `yaml:"volatility_cascade,omitempty"`
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
