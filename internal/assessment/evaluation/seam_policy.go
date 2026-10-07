package evaluation

import (
	"slices"

	"github.com/alexei-led/archfit/internal/assessment/finding"
	"github.com/alexei-led/archfit/internal/assessment/result"
	"github.com/alexei-led/archfit/internal/policy"
)

// Seam policy statuses (seams[].policy). The first matching one wins.
const (
	// seamViolation: an active gate finding names the pair.
	seamViolation = "violation"
	// seamAccepted: a gate finding on the pair is baselined or waived.
	// Accepted debt is not permission.
	seamAccepted = "accepted"
	// seamAdvisory: another active finding names the pair.
	seamAdvisory = "advisory"
	// seamAllowed: a fail-gated allowlist or layer rule proves the pair
	// permitted.
	seamAllowed = "allowed"
	// seamObserved: no finding names the pair and no rule decides it.
	seamObserved = "observed"
)

// attachSeamPolicy sets the policy status of every seam from the findings the
// report publishes and the permission predicate `archfit policy can-import`
// uses. It runs after finalize, which adds the seam-gate findings. A finding
// names a pair through its edge's modules; a fixed one names nothing.
func attachSeamPolicy(diag *result.Result, p policy.PolicySnapshot) {
	type pair struct{ from, to string }
	rank := map[pair]int{}
	for _, f := range diag.Findings {
		r := 0
		switch {
		case f.Status == finding.StatusFixed:
		case f.Kind == finding.KindGate && (f.Status == finding.StatusNew || f.Status == finding.StatusExpiredWaiver):
			r = 3
		case f.Kind == finding.KindGate:
			r = 2
		default:
			r = 1
		}
		k := pair{f.Edge.From.Module, f.Edge.To.Module}
		rank[k] = max(rank[k], r)
	}
	for i := range diag.Seams {
		s := &diag.Seams[i]
		switch rank[pair{s.FromModule, s.ToModule}] {
		case 3:
			s.Policy = seamViolation
		case 2:
			s.Policy = seamAccepted
		case 1:
			s.Policy = seamAdvisory
		default:
			s.Policy = seamObserved
			if len(pairPermission(p, s.FromModule, s.ToModule)) > 0 {
				s.Policy = seamAllowed
			}
		}
	}
}

// pairPermission names what permits a dependency from declared module from on
// declared module to: an allowlist that names the pair, or the layer order,
// each only under a fail-gated rule that enforces it. Empty means no rule
// decides the pair. A module no config declares (a go.work member) has no
// allowlist and no layer, so nothing permits it.
func pairPermission(p policy.PolicySnapshot, from, to string) []string {
	rules := p.Gates.Rules
	mm := rules.ModuleMap
	if !mm.Has(from) || !mm.Has(to) || from == to {
		return nil
	}
	var reasons []string
	for _, def := range rules.Rules {
		if !def.Blocks() {
			continue
		}
		switch def.Type {
		case ruleTypeModuleDependencies:
			if why := allowlistPermission(mm, p.Topology.Modules, from, to); why != "" {
				reasons = append(reasons, why)
			}
		case ruleTypeLayerOrder:
			if why := moduleLayerReason(mm, rules.Layers, from, to); why != "" {
				reasons = append(reasons, why)
			}
		}
	}
	return sortedUnique(reasons)
}

// allowlistPermission names the allowlist that permits a dependency between
// two different declared modules, or "". `archfit policy can-import` and the
// seam policy share it.
func allowlistPermission(mm policy.ModuleMap, modules map[string]policy.ModuleDef, from, to string) string {
	if from == "" || to == "" || from == to || mm.DeniedDependency(from, to) != nil {
		return ""
	}
	return allowlistReason(modules, from, to)
}

// moduleLayerReason names the layer order that permits module from to depend
// on module to: both layers ranked, and the dependency points from an outer
// (higher-rank) layer to the same or an inner one. Empty otherwise.
func moduleLayerReason(mm policy.ModuleMap, layers []string, from, to string) string {
	fromLayer, ok := mm.LayerForName(from)
	if !ok {
		return ""
	}
	toLayer, ok := mm.LayerForName(to)
	if !ok {
		return ""
	}
	fromRank, toRank := slices.Index(layers, fromLayer), slices.Index(layers, toLayer)
	if fromRank < 0 || toRank < 0 || fromRank < toRank {
		return ""
	}
	return "layer " + fromLayer + " may depend on layer " + toLayer
}
