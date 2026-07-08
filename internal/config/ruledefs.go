package config

// RuleDef declares a single architecture rule.
type RuleDef struct {
	ID        string       `yaml:"id"`
	Type      string       `yaml:"type"`
	Gate      string       `yaml:"gate"`
	From      string       `yaml:"from"`
	To        string       `yaml:"to"`
	Max       *int         `yaml:"max,omitempty"`       // public_api_max: exported-declaration ceiling per module
	Threshold *int         `yaml:"threshold,omitempty"` // reserved: per-rule integer threshold
	Patterns  []PatternDef `yaml:"patterns,omitempty"`
}

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
