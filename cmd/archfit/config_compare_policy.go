package main

import (
	"fmt"
	"io"

	"github.com/alexei-led/archfit/v3/internal/application"
	"github.com/alexei-led/archfit/v3/internal/config"
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

// policyDeclarations reads the declarations a policy change is classified from.
func policyDeclarations(cfg config.Config) application.PolicyDeclarations {
	metricGates := map[string]string{}
	for name, entry := range cfg.Metrics.MetricEntries() {
		gate := entry.Gate
		if entry.Enabled != nil && !*entry.Enabled {
			gate = ""
		}
		metricGates[name] = gate
	}
	return application.PolicyDeclarations{
		Rules: cfg.Rules, Waivers: cfg.Waivers, Modules: cfg.Modules,
		MetricGates: metricGates, Exclude: cfg.Exclude,
		Languages: map[string]string{
			"go": string(cfg.Languages.Go.Enabled), "typescript": string(cfg.Languages.TypeScript.Enabled),
			"python": string(cfg.Languages.Python.Enabled), "rust": string(cfg.Languages.Rust.Enabled),
		},
		Analyzers: map[string]string{
			"syntax": string(cfg.Analyzers.Syntax.Enabled), "scip": string(cfg.Analyzers.Scip.Enabled),
			"clones": string(cfg.Analyzers.Clones.Enabled), "cargo_modules": string(cfg.Analyzers.CargoModules.Enabled),
		},
		ModuleReviewGate: cfg.ModuleReview.Gate,
	}
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
