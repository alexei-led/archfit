// Package agenttask assembles the structured repair-task block (spec §13) from
// gate findings. One task per ACTIVE gate finding, derived deterministically
// from the finding plus rule/module configuration — a coding agent consumes
// the block mechanically: goal, constraints, files, validation commands.
package agenttask

import (
	"fmt"
	"sort"

	"github.com/alexei-led/archfit/internal/model/diagnostic"
	"github.com/alexei-led/archfit/internal/model/finding"
)

// Build returns one AgentTask per active gate finding (status new or
// expired_exception). Advisory findings never produce tasks — they are
// signals, not orders.
//
// ruleTypes maps rule ID → rule type (drives the goal template).
// modulePublic maps module name → its public path globs (becomes a constraint
// for the edge's target module when present).
// validation lists the exact commands that re-verify the gate; passed through
// verbatim to every task.
//
// Output is sorted by FindingID; all nested slices carry a total order.
func Build(
	findings []finding.Finding,
	ruleTypes map[string]string,
	modulePublic map[string][]string,
	validation []string,
) []diagnostic.AgentTask {
	tasks := []diagnostic.AgentTask{}
	for _, f := range findings {
		if f.Kind != "gate" {
			continue
		}
		if f.Status != finding.StatusNew && f.Status != finding.StatusExpiredExcept {
			continue
		}
		tasks = append(tasks, diagnostic.AgentTask{
			FindingID:   f.ID,
			RuleID:      f.RuleID,
			Goal:        goalFor(ruleTypes[f.RuleID], f),
			Constraints: constraintsFor(f, modulePublic),
			Files:       filesFor(f),
			Validation:  append([]string{}, validation...),
		})
	}
	sort.Slice(tasks, func(i, j int) bool { return tasks[i].FindingID < tasks[j].FindingID })
	return tasks
}

// goalFor instantiates the rule type's repair-goal template with the finding's
// edge. Unknown rule types fall back to the finding's Why text — never empty.
func goalFor(ruleType string, f finding.Finding) string {
	from, to := f.Edge.From.Path, f.Edge.To.Path
	toMod := f.Edge.To.Module
	if toMod == "" {
		toMod = to
	}
	switch ruleType {
	case "forbidden_dependency":
		return fmt.Sprintf("Remove the forbidden dependency from %s on %s; depend on %s's public API or move the shared code to an allowed location.", from, to, toMod)
	case "public_api_only", "internal_api_access":
		return fmt.Sprintf("Replace the internal-API access from %s to %s with %s's public API.", from, to, toMod)
	case "forbidden_layer_direction":
		return fmt.Sprintf("Remove the layer-inverting dependency from %s to %s: inner layers must not import outer layers — introduce an abstraction in the inner layer instead.", from, to)
	case "new_cross_module_dependency":
		return fmt.Sprintf("Review the new cross-module dependency from %s to %s: either remove it, route it through %s's public API, or accept it explicitly with `archfit baseline`.", from, to, toMod)
	case "cycle":
		return "Break the import cycle: " + f.Why
	default:
		if f.Why != "" {
			return f.Why
		}
		return fmt.Sprintf("Resolve the %s violation on the edge %s -> %s.", f.RuleID, from, to)
	}
}

// constraintsFor joins the finding's constraint text, its allowed
// alternatives, and the target module's public surface.
func constraintsFor(f finding.Finding, modulePublic map[string][]string) []string {
	out := []string{}
	if f.Constraint != "" {
		out = append(out, f.Constraint)
	}
	for _, alt := range f.Alternatives {
		out = append(out, "allowed alternative: "+alt)
	}
	if pub := modulePublic[f.Edge.To.Module]; len(pub) > 0 {
		out = append(out, fmt.Sprintf("public surface of module %q: %v", f.Edge.To.Module, pub))
	}
	return out
}

// filesFor returns the deduplicated, sorted repo-relative files involved:
// edge endpoints plus every finding location.
func filesFor(f finding.Finding) []string {
	set := map[string]struct{}{}
	if f.Edge.From.Path != "" {
		set[f.Edge.From.Path] = struct{}{}
	}
	if f.Edge.To.Path != "" {
		set[f.Edge.To.Path] = struct{}{}
	}
	for _, loc := range f.Locations {
		if loc.File != "" {
			set[loc.File] = struct{}{}
		}
	}
	files := make([]string, 0, len(set))
	for p := range set {
		files = append(files, p)
	}
	sort.Strings(files)
	return files
}
