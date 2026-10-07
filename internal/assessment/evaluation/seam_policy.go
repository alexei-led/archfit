package evaluation

import (
	"path"
	"slices"

	"github.com/alexei-led/archfit/internal/assessment/finding"
	"github.com/alexei-led/archfit/internal/assessment/result"
	"github.com/alexei-led/archfit/internal/policy"
	"github.com/alexei-led/archfit/internal/relationship"
)

// languageTypeScript is the TypeScript language id; like Go, it spells an
// importing node as a file path.
const languageTypeScript = "typescript"

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
// names the pair of its edge endpoints, each placed under the module the seam
// ledger uses for it; a fixed finding, or a baselined or waived advisory,
// names nothing.
func attachSeamPolicy(diag *result.Result, p policy.PolicySnapshot) {
	type pair struct{ from, to string }
	rank := map[pair]int{}
	for _, f := range diag.Findings {
		r := 0
		active := f.Status == finding.StatusNew || f.Status == finding.StatusExpiredWaiver
		switch {
		case f.Status == finding.StatusFixed:
		case f.Kind == finding.KindGate && active:
			r = 3
		case f.Kind == finding.KindGate:
			r = 2
		case active:
			r = 1
		}
		k := pair{endpointModule(diag.SeamEndpointModules, f.Edge.From), endpointModule(diag.SeamEndpointModules, f.Edge.To)}
		rank[k] = max(rank[k], r)
	}
	for i := range diag.Seams {
		s := &diag.Seams[i]
		switch rank[pair{s.FromModule, s.ToModule}] {
		case 3:
			s.Policy = seamViolation
			// An active gate finding already tells the owner what to do; relationship
			// analysis cannot see it, so the hypothesis is set here.
			s.Hypothesis = string(relationship.SeamHypothesisFollowRule)
		case 2:
			s.Policy = seamAccepted
		case 1:
			s.Policy = seamAdvisory
		case 0:
			s.Policy = seamObserved
			if pairPermitted(p, s.FromModule, s.ToModule) {
				s.Policy = seamAllowed
			}
		}
	}
}

// endpointModule is the seam-ledger module of one finding endpoint: the module
// the graph puts its path under, else the module the finding names.
func endpointModule(modules map[string]string, e finding.Endpoint) string {
	if m, ok := modules[e.Path]; ok && e.Path != "" {
		return m
	}
	return e.Module
}

// seamEndpointModules indexes every edge endpoint path, and the directory of
// each importing Go or TypeScript file (the package a module-pair finding
// names), under the module the seam ledger keys the edge by.
func seamEndpointModules(s relationship.Set) map[string]string {
	out := make(map[string]string, 2*len(s.Edges))
	for _, e := range s.Edges {
		if e.FromModule != "" {
			out[e.FromPath] = e.FromModule
		}
		if e.ToModule != "" {
			out[e.ToPath] = e.ToModule
		}
	}
	for _, e := range s.Edges {
		// Only Go and TypeScript spell an importer as a file path; a dotted
		// Python module or a crate::mod node has no directory. The root
		// package is "." (main.go).
		if e.FromModule == "" || (e.Language != goLanguage && e.Language != languageTypeScript) {
			continue
		}
		if dir := path.Dir(e.FromPath); out[dir] == "" {
			out[dir] = e.FromModule
		}
	}
	return out
}

// pairPermitted reports whether a fail-gated rule permits a dependency from
// declared module from on declared module to: an allowlist that names the
// pair, or the layer order. It is the permission half of `archfit policy
// can-import`; the run itself has evaluated the whole-graph rules can-import
// leaves not_decided for one edge. A module no config declares (a go.work
// member) has no allowlist and no layer, so nothing permits it.
func pairPermitted(p policy.PolicySnapshot, from, to string) bool {
	rules := p.Gates.Rules
	mm := rules.ModuleMap
	if !mm.Has(from) || !mm.Has(to) || from == to {
		return false
	}
	for _, def := range rules.Rules {
		if !def.Blocks() {
			continue
		}
		switch def.Type {
		case ruleTypeModuleDependencies:
			if allowlistPermission(mm, p.Topology.Modules, from, to) != "" {
				return true
			}
		case ruleTypeLayerOrder:
			if moduleLayerPermits(mm, rules.Layers, from, to) {
				return true
			}
		}
	}
	return false
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

// moduleLayerPermits reports whether the layer order permits module from to
// depend on module to: both layers ranked, and the dependency points from an
// outer (higher-rank) layer to the same or an inner one, as the layer rule
// judges an edge.
func moduleLayerPermits(mm policy.ModuleMap, layers []string, from, to string) bool {
	fromLayer, ok := mm.LayerForName(from)
	if !ok {
		return false
	}
	toLayer, ok := mm.LayerForName(to)
	if !ok {
		return false
	}
	fromRank, toRank := slices.Index(layers, fromLayer), slices.Index(layers, toLayer)
	return fromRank >= 0 && toRank >= 0 && fromRank >= toRank
}
