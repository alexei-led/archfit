package config

import (
	"github.com/alexei-led/archfit/internal/model/module"
	"github.com/alexei-led/archfit/internal/view"
)

// ToolMode is the public config lifecycle view of a tool enable state.
type ToolMode = view.ToolMode

// Tool enable modes.
const (
	ModeAuto = view.ModeAuto
	ModeOn   = view.ModeOn
	ModeOff  = view.ModeOff
)

// ModuleDef is the policy-facing architectural module definition.
type ModuleDef = module.ModuleDef

// ModuleMap is the policy-facing module lookup contract.
type ModuleMap = module.Map

// Module role values exposed through the policy contract.
const (
	RoleCompositionRoot = module.RoleCompositionRoot
	RoleAdapter         = module.RoleAdapter
	RoleCore            = module.RoleCore
	RoleSharedModel     = module.RoleSharedModel
	RoleGenerated       = module.RoleGenerated
	RoleTest            = module.RoleTest
)

// ModuleRootDirs returns the configured source roots for each module.
func ModuleRootDirs(modules map[string]ModuleDef) map[string]string {
	return module.RootDirs(modules)
}

// MetricsConfig holds settings for all metrics, keyed by metric name.
type MetricsConfig map[string]view.MetricEntry

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
	DuplicatedKnowledge view.DuplicatedKnowledgePolicy `yaml:"duplicated_knowledge,omitempty"`
	// VolatilityCascade enables the book Ch9 propagation pass: a module strongly
	// coupled to a high-effective-volatility module inherits raised effective
	// volatility. The pass runs to a deterministic fixpoint and never lowers values.
	VolatilityCascade bool `yaml:"volatility_cascade,omitempty"`
	// Gate makes the synthesised coupling_balance score gate the verdict.
	// Absent (nil) = coupling stays advisory, today's behavior. An unmeasured
	// score (band n/a) never trips the gate regardless of these knobs.
	Gate *CouplingGateDef `yaml:"gate,omitempty"`
}

// CouplingGateDef configures the coupling_balance verdict gate
// (`coupling.gate:`). At least one knob must be set; validate() rejects an
// empty block.
type CouplingGateDef struct {
	// MinBand is the band floor: poor | mixed | serviceable | strong. The
	// verdict fails when the current coupling_balance band ranks below it.
	// critical is rejected — no band ranks below it, so it could never trip.
	MinBand string `yaml:"min_band,omitempty"`
	// MaxDrop is the tolerated point drop of the coupling_balance value against
	// the score stored in .archfit-baseline.json. Unset = no drop check;
	// 0 = any drop fails. Skipped when the baseline carries no stored score.
	MaxDrop *int `yaml:"max_drop,omitempty"`
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
