package evaluation

import (
	"slices"
	"sort"
	"time"

	"github.com/alexei-led/archfit/internal/assessment/agenttask"
	"github.com/alexei-led/archfit/internal/assessment/finding"
	rulespkg "github.com/alexei-led/archfit/internal/assessment/rules"
	"github.com/alexei-led/archfit/internal/assessment/status"
	"github.com/alexei-led/archfit/internal/policy"
	"github.com/alexei-led/archfit/internal/relationship"
)

// Edge answers of `archfit policy can-import`.
const (
	// AnswerDenied: a fail-gated rule fires on the edge with an active status.
	AnswerDenied = "denied"
	// AnswerNotDecided: no rule denies the edge, but a fail-gated rule that
	// needs the whole graph (a cycle rule, the seam gate) could still block it.
	AnswerNotDecided = "not_decided"
	// AnswerAllowed: an allowlist entry or the layer order permits the edge.
	AnswerAllowed = "allowed"
	// AnswerUnconstrained: no rule decides the edge. This is not permission.
	AnswerUnconstrained = "unconstrained"
)

// reasonAccepted explains an edge whose only violations are accepted debt.
const reasonAccepted = "the edge breaks fail-gated rules that the baseline or a waiver accepts, so check does not block it"

// Rule types the edge answer reads beyond the rule pass itself.
const (
	ruleTypeCycle      = "cycle"
	ruleTypeLayerOrder = "forbidden_layer_direction"
	goLanguage         = "go"
)

// JudgeInput is one queried edge: the relationship analysis of a one-edge
// graph, and the policy, accepted baseline, and waivers check applies to it.
type JudgeInput struct {
	Relationships relationship.Set
	Policy        policy.PolicySnapshot
	Accepted      status.AcceptedSet
	Now           time.Time
	// UnwalkedSourceProduction says whether the importing file is production,
	// as acquisition says it for an edge source the source walk never visited.
	UnwalkedSourceProduction map[string]bool
}

// EdgeJudgment is the answer for one queried edge.
type EdgeJudgment struct {
	Answer string
	// Reasons say what allowed the edge or what is not decided.
	Reasons []string
	// Denials are the active fail-gated findings on the edge.
	Denials []EdgeFinding
	// Accepted are the fail-gated findings the baseline or a waiver accepts.
	Accepted []EdgeFinding
	// Advisories are the findings of gate: warn rules on the edge.
	Advisories []EdgeFinding
}

// EdgeFinding is one finding on a queried edge, with its repair text.
type EdgeFinding struct {
	FindingID   string
	RuleID      string
	Status      string
	Why         string
	RepairKind  string
	Goal        string
	Constraints []string
}

// JudgeEdge runs the rule pass check runs, over one edge, and assigns
// lifecycle status against the same baseline and waivers. A fail-gated rule
// that fires with an active status denies the edge, with the finding ID check
// reports for the same import. It is the one evaluator behind
// `archfit policy can-import`.
func JudgeEdge(in JudgeInput) (EdgeJudgment, error) {
	ruleset, err := NewRuleset(in.Policy.Gates.Rules)
	if err != nil {
		return EdgeJudgment{}, err
	}
	if in.Accepted == nil {
		in.Accepted = status.Empty{}
	}
	raw := checkRules(ruleset, in.Relationships, RuleEvidence{UnwalkedSourceProduction: in.UnwalkedSourceProduction})
	tagged := status.Assign(raw, in.Accepted, in.Policy.Assessment.Waivers, in.Now, finding.KindGate)
	tagged = slices.DeleteFunc(tagged, func(f finding.Finding) bool { return f.Status == finding.StatusFixed })
	tagged = resolveEvidence(in.Relationships, in.Policy.Topology.ModuleMap, tagged)

	ruleTypes := ruleTypesOf(in.Policy)
	tasks := map[string]EdgeFinding{}
	for _, task := range agenttask.Build(activeAsGate(tagged), ruleTypes, modulePublicOf(in.Policy), nil, nil, nil,
		agenttask.NewPathResolver(nil, nil, nil, func(string) bool { return false })) {
		tasks[task.FindingID] = EdgeFinding{RepairKind: task.RepairKind, Goal: task.Goal, Constraints: task.Constraints}
	}

	var out EdgeJudgment
	for _, f := range tagged {
		ef := tasks[f.ID]
		ef.FindingID, ef.RuleID, ef.Status, ef.Why = f.ID, f.RuleID, string(f.Status), f.Why
		switch {
		case f.Kind == finding.KindAdvisory:
			out.Advisories = append(out.Advisories, ef)
		case f.Status == finding.StatusNew || f.Status == finding.StatusExpiredWaiver:
			out.Denials = append(out.Denials, ef)
		default:
			out.Accepted = append(out.Accepted, ef)
		}
	}
	for _, list := range [][]EdgeFinding{out.Denials, out.Accepted, out.Advisories} {
		sort.Slice(list, func(i, j int) bool { return list[i].FindingID < list[j].FindingID })
	}

	switch {
	case len(out.Denials) > 0:
		out.Answer = AnswerDenied
	default:
		out.Answer, out.Reasons = undecidedOrAllowed(in)
		// A violation the baseline or a waiver accepts does not block, but an
		// allowlist or the layer order cannot be said to permit it.
		if out.Answer == AnswerAllowed && len(out.Accepted) > 0 {
			out.Answer, out.Reasons = AnswerUnconstrained, []string{reasonAccepted}
		}
	}
	return out, nil
}

// activeAsGate returns every finding as a new gate finding, so the repair-task
// builder writes repair text for advisories and accepted findings too.
func activeAsGate(findings []finding.Finding) []finding.Finding {
	out := slices.Clone(findings)
	for i := range out {
		out[i].Kind, out[i].Status = finding.KindGate, finding.StatusNew
	}
	return out
}

// undecidedOrAllowed answers an edge no rule denies. A fail-gated rule that
// needs the whole graph leaves it not decided. Otherwise an allowlist that
// names the pair, or the layer order, allows it; nothing else decides it.
func undecidedOrAllowed(in JudgeInput) (string, []string) {
	rules := in.Policy.Gates.Rules
	mm := rules.ModuleMap
	var undecided, allowed []string
	for _, e := range in.Relationships.Edges {
		// The module_dependencies rule resolves declared modules with the
		// file fallback; module_cycle reads the edge's own modules, kept only
		// when declared and the importer is production; the seam ledger reads
		// them including synthetic go.work members.
		fromModule, toModule := rulespkg.DeclaredEndpoints(mm, e)
		allowlisted := fromModule != "" && toModule != "" && fromModule != toModule
		moduleCycle := rulespkg.ProductionEdge(e, in.UnwalkedSourceProduction) && mm.Has(e.FromModule) && mm.Has(e.ToModule) && e.FromModule != e.ToModule
		seam := e.FromModule != "" && e.ToModule != "" && e.FromModule != e.ToModule
		for _, def := range rules.Rules {
			if !failGated(def) {
				continue
			}
			switch def.Type {
			case ruleTypeCycle:
				// Go edges run file -> package, so a node cycle is never
				// formed on Go: only TypeScript and Python can close one.
				if e.Language != goLanguage {
					undecided = append(undecided, "rule "+def.ID+" (cycle) needs the whole import graph")
				}
			case ruleTypeModuleCycle:
				if moduleCycle {
					undecided = append(undecided, "rule "+def.ID+" (module_cycle) needs the whole module graph")
				}
			case ruleTypeModuleDependencies:
				if allowlisted && mm.DeniedDependency(fromModule, toModule) == nil {
					if why := allowlistReason(in.Policy.Topology.Modules, fromModule, toModule); why != "" {
						allowed = append(allowed, why)
					}
				}
			case ruleTypeLayerOrder:
				if why := layerReason(mm, rules.Layers, e); why != "" {
					allowed = append(allowed, why)
				}
			}
		}
		if seam && in.Policy.Gates.Coupling.Mode == policy.DistributedMonolithFail {
			undecided = append(undecided, "the distributed-monolith seam gate (mode: fail) needs the whole module graph")
		}
	}
	switch {
	case len(undecided) > 0:
		return AnswerNotDecided, sortedUnique(undecided)
	case len(allowed) > 0:
		return AnswerAllowed, sortedUnique(allowed)
	default:
		return AnswerUnconstrained, nil
	}
}

// failGated reports whether a rule blocks: gate fail, or no gate on a type
// that blocks by default.
func failGated(def policy.RuleDef) bool {
	return def.Gate == string(policy.GateFail) || def.Gate == ""
}

// allowlistReason names the allowlist that permits a dependency, or "" when
// neither module declares one. The caller has already asked DeniedDependency.
func allowlistReason(modules map[string]policy.ModuleDef, from, to string) string {
	switch {
	case modules[from].DependsOn != nil:
		return "module " + from + " lists " + to + " in depends_on"
	case modules[to].VisibleTo != nil:
		return "module " + to + " lists " + from + " in visible_to"
	default:
		return ""
	}
}

// layerReason names the layer order that permits an edge, or "" when either
// endpoint has no ranked layer. The layer rule already denied an inverted
// edge, so a ranked pair here flows in the allowed direction.
func layerReason(mm policy.ModuleMap, layers []string, e relationship.Edge) string {
	fromLayer, ok := mm.LayerFor(e.FromPath, e.Language)
	if !ok || !slices.Contains(layers, fromLayer) {
		return ""
	}
	toLayer, ok := mm.LayerFor(e.ToPath, e.Language)
	if !ok || !slices.Contains(layers, toLayer) {
		return ""
	}
	return "layer " + fromLayer + " may depend on layer " + toLayer
}

// ruleTypesOf maps each declared rule ID to its type.
func ruleTypesOf(p policy.PolicySnapshot) map[string]string {
	out := make(map[string]string, len(p.Gates.Rules.Rules))
	for _, def := range p.Gates.Rules.Rules {
		out[def.ID] = def.Type
	}
	return out
}

// modulePublicOf maps each module that declares a public surface to it.
func modulePublicOf(p policy.PolicySnapshot) map[string][]string {
	out := make(map[string][]string, len(p.Topology.Modules))
	for name, def := range p.Topology.Modules {
		if len(def.Public) > 0 {
			out[name] = def.Public
		}
	}
	return out
}

func sortedUnique(in []string) []string {
	out := slices.Clone(in)
	slices.Sort(out)
	return slices.Compact(out)
}
