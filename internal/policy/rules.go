package policy

import (
	"github.com/alexei-led/archfit/internal/model/pattern"
)

// GateMode controls how a missing tool or regressed metric affects the verdict.
type GateMode string

// GateOff, GateWarn, and GateFail define gate behavior.
const (
	GateOff  GateMode = "off"
	GateWarn GateMode = "warn"
	GateFail GateMode = "fail"
)

// RuleDef declares a single architecture rule.
type RuleDef struct {
	ID        string        `yaml:"id"`
	Type      string        `yaml:"type"`
	Gate      string        `yaml:"gate"`
	From      string        `yaml:"from"`
	To        string        `yaml:"to"`
	Max       *int          `yaml:"max,omitempty"`       // public_api_max: exported-declaration ceiling per module
	Threshold *int          `yaml:"threshold,omitempty"` // reserved: per-rule integer threshold
	Patterns  []pattern.Def `yaml:"patterns,omitempty"`
}

// RuleConfig is the rule-evaluation projection of the policy: the declared
// rules plus the topology they resolve module identity against.
type RuleConfig struct {
	Rules     []RuleDef
	Layers    []string
	ModuleMap ModuleMap
}

// MetricEntry holds the settings for a single metric inside the metrics map.
// Gate sets what a baseline regression does to the verdict: off skips the
// check, warn caps at WARN, fail/unset blocks — the rule-gate convention.
// MinDelta (ratio metrics) is the tolerated drop below the baseline; MaxNew
// (count metrics) is the allowed increase. Both default to 0 when unset (any
// worsening move trips). Validation rejects a knob on a metric of the wrong
// kind — pointers so a zero-valued wrong-kind knob (e.g. `max_new: 0` on a
// ratio metric) is still seen as present and rejected, not silently inert.
type MetricEntry struct {
	// Metrics run by default: a knob-only entry (e.g. only `gate: warn`)
	// stays enabled, and only an explicit `enabled: false` disables the
	// metric. (Pointer so absent and false decode differently.)
	Enabled  *bool    `yaml:"enabled"`
	Gate     string   `yaml:"gate"`
	MinDelta *float64 `yaml:"min_delta"`
	MaxNew   *int     `yaml:"max_new"`
}

// MetricConfig is the per-metric policy projection.
type MetricConfig = MetricEntry

// WaiverDef grants an approved, time-boxed deviation from a rule (`waivers:`).
// A finding matching a waiver is suppressed until `expires` passes, after which
// it gates again. reason/approved_by record the governance trail.
type WaiverDef struct {
	Rule       string `yaml:"rule"`
	From       string `yaml:"from"`
	To         string `yaml:"to"`
	Reason     string `yaml:"reason"`
	ApprovedBy string `yaml:"approved_by"`
	Expires    string `yaml:"expires"`
}

// WaiverSet is the approved rule deviations (`waivers:`) that suppress matching
// gate findings until they expire.
type WaiverSet struct {
	Waivers []WaiverDef
}
