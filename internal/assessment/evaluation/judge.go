package evaluation

import "github.com/alexei-led/archfit/v3/internal/policy"

const (
	ruleTypeLayerOrder = "forbidden_layer_direction"
	goLanguage         = "go"
)

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

func ruleTypesOf(p policy.PolicySnapshot) map[string]string {
	out := make(map[string]string, len(p.Gates.Rules.Rules))
	for _, def := range p.Gates.Rules.Rules {
		out[def.ID] = def.Type
	}
	return out
}

func modulePublicOf(p policy.PolicySnapshot) map[string][]string {
	out := make(map[string][]string, len(p.Topology.Modules))
	for name, def := range p.Topology.Modules {
		if len(def.Public) > 0 {
			out[name] = def.Public
		}
	}
	return out
}
