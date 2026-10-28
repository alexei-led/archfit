package main

import (
	"encoding/json"
	"fmt"
	"io"

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

// policyDeclarations reads the declarations a policy change is classified
// from, as they take effect: metric gates with their default (fail), disabled
// metrics as off, exclusions merged with the defaults and re-includes applied,
// optional analyzers as on or off (auto does not run one). Every other
// declaration that can change a verdict goes into Declarations, which the
// classifier cannot order and so counts as loosening.
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
	declarations := map[string]string{
		"layers":                       canonical(cfg.Layers),
		"target_branch":                cfg.TargetBranch,
		"coverage":                     canonical(cfg.Coverage),
		"coupling":                     canonical(cfg.Coupling),
		"file_class":                   canonical(cfg.FileClass),
		"external_systems":             canonical(cfg.ExternalSystems),
		"languages.go.gate":            string(cfg.Languages.Go.Gate),
		"languages.go.modules":         canonical(cfg.Languages.Go.Modules),
		"languages.typescript.gate":    string(cfg.Languages.TypeScript.Gate),
		"languages.python.gate":        string(cfg.Languages.Python.Gate),
		"languages.rust.gate":          string(cfg.Languages.Rust.Gate),
		"analyzers.syntax.gate":        string(cfg.Analyzers.Syntax.Gate),
		"analyzers.scip.gate":          string(cfg.Analyzers.Scip.Gate),
		"analyzers.scip.timeout":       cfg.Analyzers.Scip.Timeout,
		"analyzers.clones.gate":        string(cfg.Analyzers.Clones.Gate),
		"analyzers.clones.timeout":     cfg.Analyzers.Clones.Timeout,
		"analyzers.cargo_modules.gate": string(cfg.Analyzers.CargoModules.Gate),
	}
	review := cfg.ModuleReview
	review.Gate = ""
	declarations["module_review"] = canonical(review)
	for name, m := range cfg.Modules {
		for field, value := range map[string]any{
			"layer": m.Layer, "role": m.Role, "public": m.Public, "internal": m.Internal,
			"owner": m.Owner, "subdomain": m.Subdomain, "volatility": m.Volatility, "deploy_unit": m.DeployUnit,
		} {
			declarations["modules."+name+"."+field] = canonical(value)
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
		ModuleReviewGate: cfg.ModuleReview.Gate,
		Declarations:     declarations,
	}
}

// canonical encodes a declaration for comparison: Go structs marshal in field
// order and maps in key order, so equal declarations encode equally.
func canonical(v any) string {
	raw, err := json.Marshal(v)
	if err != nil {
		return fmt.Sprintf("%v", v)
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
