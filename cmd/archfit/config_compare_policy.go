package main

import (
	"encoding/json"
	"fmt"
	"io"
	"slices"

	"github.com/goccy/go-yaml"

	"github.com/alexei-led/archfit/v3/internal/application"
	"github.com/alexei-led/archfit/v3/internal/config"
	evidenceports "github.com/alexei-led/archfit/v3/internal/evidence/ports"
	"github.com/alexei-led/archfit/v3/internal/scope"
)

// policyChangeDoc is the policy_change section of archfit.config-compare.v1:
// what the candidate configuration changes in what blocks and what is read.
// direction is none, tightening, loosening, or mixed; a loosening always needs
// a human approver. Every list is present, empty when nothing changed.
type policyChangeDoc struct {
	Direction          string               `json:"direction"`
	RulesRemoved       []policyRuleDoc      `json:"rules_removed"`
	RulesLoosened      []policyRuleDeltaDoc `json:"rules_loosened"`
	WaiversAdded       []policyWaiverDoc    `json:"waivers_added"`
	AllowlistsWidened  []policyAllowDoc     `json:"allowlists_widened"`
	CoverageLost       []policyCoverageDoc  `json:"coverage_lost"`
	RulesAdded         []policyRuleDoc      `json:"rules_added"`
	RulesTightened     []policyRuleDeltaDoc `json:"rules_tightened"`
	WaiversRemoved     []policyWaiverDoc    `json:"waivers_removed"`
	AllowlistsNarrowed []policyAllowDoc     `json:"allowlists_narrowed"`
	CoverageGained     []policyCoverageDoc  `json:"coverage_gained"`
	// DeclarationsChanged are differences that cannot be ordered; they count
	// as loosening.
	DeclarationsChanged []policyDeclarationDoc `json:"declarations_changed"`
}

type policyDeclarationDoc struct {
	Key  string `json:"key"`
	From string `json:"from"`
	To   string `json:"to"`
}

type policyRuleDoc struct {
	ID   string `json:"id"`
	Type string `json:"type"`
	Gate string `json:"gate"`
}

type policyRuleDeltaDoc struct {
	ID    string `json:"id"`
	Field string `json:"field"`
	From  string `json:"from"`
	To    string `json:"to"`
}

type policyWaiverDoc struct {
	Rule       string `json:"rule"`
	From       string `json:"from,omitempty"`
	To         string `json:"to,omitempty"`
	ApprovedBy string `json:"approved_by"`
	Expires    string `json:"expires"`
}

type policyAllowDoc struct {
	Module string `json:"module"`
	Field  string `json:"field"`
	From   string `json:"from"`
	To     string `json:"to"`
}

type policyCoverageDoc struct {
	Kind   string `json:"kind"`
	Detail string `json:"detail"`
}

// inertConfigKeys are the top-level config keys that never change a verdict:
// the off-gate AI provider, accepted output fields, the schema version.
var inertConfigKeys = []string{"ai", "outputs", "version"}

// orderedConfigKeys are compared field by field, in order, by the classifier;
// they are left out of the unordered declarations.
var orderedConfigKeys = []string{"rules", "waivers", "modules", "exclude"}

// policyDeclarations reads the declarations a policy change is classified
// from, as they take effect: metric gates with their default (fail), disabled
// metrics as off, exclusions merged with the defaults and re-includes applied,
// optional analyzers as on or off (auto does not run one), module_review as the
// gate its pass runs at.
//
// Declarations hold the whole configuration by config path, minus what is
// classified in order and what is inert. It is built by subtraction, not by
// listing fields, so a key added to the configuration later is compared from
// the day it exists: an unknown difference counts as loosening.
func policyDeclarations(cfg config.Config) application.PolicyDeclarations {
	metrics := map[string]application.PolicyMetric{}
	for name, entry := range cfg.Metrics.MetricEntries() {
		m := application.PolicyMetric{Gate: entry.Gate, MaxNew: entry.MaxNew, MinDelta: entry.MinDelta}
		if entry.Enabled != nil && !*entry.Enabled {
			m.Gate = string(config.GateOff)
		}
		metrics[name] = m
	}
	runs := func(mode evidenceports.ToolMode) string {
		if mode == evidenceports.ModeOn {
			return string(evidenceports.ModeOn)
		}
		return string(evidenceports.ModeOff)
	}

	// The remainder of the configuration: enabled switches, the module review
	// gate and metric entries are compared in order above.
	rest := cfg
	rest.Languages.Go.Enabled, rest.Languages.TypeScript.Enabled = "", ""
	rest.Languages.Python.Enabled, rest.Languages.Rust.Enabled = "", ""
	rest.Analyzers.Syntax.Enabled, rest.Analyzers.Scip.Enabled = "", ""
	rest.Analyzers.Clones.Enabled, rest.Analyzers.CargoModules.Enabled = "", ""
	rest.ModuleReview.Gate = ""
	rest.Metrics.Entries = nil
	declarations := configByPath(rest)
	for _, key := range append(slices.Clone(inertConfigKeys), orderedConfigKeys...) {
		delete(declarations, key)
	}

	moduleDeclarations := make(map[string]string, len(cfg.Modules))
	for name, m := range cfg.Modules {
		m.Paths, m.DependsOn, m.VisibleTo = nil, nil, nil
		moduleDeclarations[name] = canonical(m)
	}

	reviewGate := string(config.GateOff)
	if cfg.ForStaleness().Enabled {
		reviewGate = cfg.ModuleReview.Gate
		if reviewGate == "" {
			reviewGate = string(config.GateWarn)
		}
	}
	return application.PolicyDeclarations{
		Rules: cfg.Rules, Waivers: cfg.Waivers, Modules: cfg.Modules, Metrics: metrics,
		Exclude: scope.MergeExclusions(cfg.Exclude),
		Languages: map[string]string{
			"go": string(cfg.Languages.Go.Enabled), "typescript": string(cfg.Languages.TypeScript.Enabled),
			"python": string(cfg.Languages.Python.Enabled), "rust": string(cfg.Languages.Rust.Enabled),
		},
		Analyzers: map[string]string{
			"syntax": runs(cfg.Analyzers.Syntax.Enabled), "scip": runs(cfg.Analyzers.Scip.Enabled),
			"clones": runs(cfg.Analyzers.Clones.Enabled), "cargo_modules": runs(cfg.Analyzers.CargoModules.Enabled),
		},
		ModuleReviewGate:   reviewGate,
		Declarations:       declarations,
		ModuleDeclarations: moduleDeclarations,
	}
}

// wholeConfigKey holds the whole configuration when it cannot be split by key.
const wholeConfigKey = "config"

// configByPath encodes each top-level key of a configuration, as its YAML
// names it, canonically.
func configByPath(cfg config.Config) map[string]string {
	raw, err := yaml.Marshal(cfg)
	if err != nil {
		return map[string]string{wholeConfigKey: canonical(cfg)}
	}
	var top map[string]any
	if err := yaml.Unmarshal(raw, &top); err != nil {
		return map[string]string{wholeConfigKey: canonical(cfg)}
	}
	out := make(map[string]string, len(top))
	for key, value := range top {
		out[key] = canonical(value)
	}
	return out
}

// canonical encodes a declaration for comparison: JSON sorts map keys and
// writes struct fields in order, so equal declarations encode equally.
func canonical(v any) string {
	raw, err := json.Marshal(v)
	if err != nil {
		return fmt.Sprintf("%#v", v)
	}
	return string(raw)
}

func policyChangeDocOf(c application.PolicyChange) policyChangeDoc {
	out := policyChangeDoc{Direction: c.Direction}
	for _, r := range c.RulesRemoved {
		out.RulesRemoved = append(out.RulesRemoved, policyRuleDoc{ID: r.ID, Type: r.Type, Gate: r.Gate})
	}
	for _, r := range c.RulesAdded {
		out.RulesAdded = append(out.RulesAdded, policyRuleDoc{ID: r.ID, Type: r.Type, Gate: r.Gate})
	}
	for _, r := range c.RulesLoosened {
		out.RulesLoosened = append(out.RulesLoosened, policyRuleDeltaDoc{ID: r.ID, Field: r.Field, From: r.From, To: r.To})
	}
	for _, r := range c.RulesTightened {
		out.RulesTightened = append(out.RulesTightened, policyRuleDeltaDoc{ID: r.ID, Field: r.Field, From: r.From, To: r.To})
	}
	for _, w := range c.WaiversAdded {
		out.WaiversAdded = append(out.WaiversAdded, policyWaiverDoc{Rule: w.Rule, From: w.From, To: w.To, ApprovedBy: w.ApprovedBy, Expires: w.Expires})
	}
	for _, w := range c.WaiversRemoved {
		out.WaiversRemoved = append(out.WaiversRemoved, policyWaiverDoc{Rule: w.Rule, From: w.From, To: w.To, ApprovedBy: w.ApprovedBy, Expires: w.Expires})
	}
	for _, a := range c.AllowlistsWidened {
		out.AllowlistsWidened = append(out.AllowlistsWidened, policyAllowDoc{Module: a.Module, Field: a.Field, From: a.From, To: a.To})
	}
	for _, a := range c.AllowlistsNarrowed {
		out.AllowlistsNarrowed = append(out.AllowlistsNarrowed, policyAllowDoc{Module: a.Module, Field: a.Field, From: a.From, To: a.To})
	}
	for _, cc := range c.CoverageLost {
		out.CoverageLost = append(out.CoverageLost, policyCoverageDoc{Kind: cc.Kind, Detail: cc.Detail})
	}
	for _, cc := range c.CoverageGained {
		out.CoverageGained = append(out.CoverageGained, policyCoverageDoc{Kind: cc.Kind, Detail: cc.Detail})
	}
	for _, d := range c.DeclarationsChanged {
		out.DeclarationsChanged = append(out.DeclarationsChanged, policyDeclarationDoc{Key: d.Key, From: d.From, To: d.To})
	}
	out.DeclarationsChanged = nonNilSlice(out.DeclarationsChanged)
	out.RulesRemoved, out.RulesAdded = nonNilSlice(out.RulesRemoved), nonNilSlice(out.RulesAdded)
	out.RulesLoosened, out.RulesTightened = nonNilSlice(out.RulesLoosened), nonNilSlice(out.RulesTightened)
	out.WaiversAdded, out.WaiversRemoved = nonNilSlice(out.WaiversAdded), nonNilSlice(out.WaiversRemoved)
	out.AllowlistsWidened, out.AllowlistsNarrowed = nonNilSlice(out.AllowlistsWidened), nonNilSlice(out.AllowlistsNarrowed)
	out.CoverageLost, out.CoverageGained = nonNilSlice(out.CoverageLost), nonNilSlice(out.CoverageGained)
	return out
}

func nonNilSlice[T any](in []T) []T {
	if in == nil {
		return []T{}
	}
	return in
}

// renderPolicyChangeText writes the policy change section of the text report.
func renderPolicyChangeText(w io.Writer, c policyChangeDoc) {
	_, _ = fmt.Fprintf(w, "\npolicy change: %s\n", c.Direction)
	for _, r := range c.RulesRemoved {
		_, _ = fmt.Fprintf(w, "  - rule removed: %s (%s, gate %s)\n", r.ID, r.Type, r.Gate)
	}
	for _, r := range c.RulesLoosened {
		_, _ = fmt.Fprintf(w, "  - rule loosened: %s %s: %s -> %s\n", r.ID, r.Field, r.From, r.To)
	}
	for _, wv := range c.WaiversAdded {
		_, _ = fmt.Fprintf(w, "  - waiver added: %s (approved by %s, expires %s)\n", wv.Rule, wv.ApprovedBy, wv.Expires)
	}
	for _, a := range c.AllowlistsWidened {
		_, _ = fmt.Fprintf(w, "  - allowlist widened: %s.%s: %s -> %s\n", a.Module, a.Field, a.From, a.To)
	}
	for _, cc := range c.CoverageLost {
		_, _ = fmt.Fprintf(w, "  - coverage lost: %s: %s\n", cc.Kind, cc.Detail)
	}
	for _, d := range c.DeclarationsChanged {
		_, _ = fmt.Fprintf(w, "  - changed (counts as loosening): %s: %s -> %s\n", d.Key, d.From, d.To)
	}
	for _, r := range c.RulesAdded {
		_, _ = fmt.Fprintf(w, "  + rule added: %s (%s, gate %s)\n", r.ID, r.Type, r.Gate)
	}
	for _, r := range c.RulesTightened {
		_, _ = fmt.Fprintf(w, "  + rule tightened: %s %s: %s -> %s\n", r.ID, r.Field, r.From, r.To)
	}
	for _, wv := range c.WaiversRemoved {
		_, _ = fmt.Fprintf(w, "  + waiver removed: %s\n", wv.Rule)
	}
	for _, a := range c.AllowlistsNarrowed {
		_, _ = fmt.Fprintf(w, "  + allowlist narrowed: %s.%s: %s -> %s\n", a.Module, a.Field, a.From, a.To)
	}
	for _, cc := range c.CoverageGained {
		_, _ = fmt.Fprintf(w, "  + coverage gained: %s: %s\n", cc.Kind, cc.Detail)
	}
}
