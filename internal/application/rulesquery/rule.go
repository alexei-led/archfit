package rulesquery

import (
	"maps"
	"slices"
	"strings"
	"time"

	"github.com/alexei-led/archfit/v3/internal/policy/resolved"
)

// rule writes one rule in full, then its exceptions. id is a built-in rule
// or a rule of the policy.
func rule(p resolved.Policy, date time.Time, id string) string {
	var w writer
	if r, ok := customRule(p, id); ok {
		w.customRule(0, r)
		if r.Form == resolved.FormNote {
			w.line(1, "not checked")
		}
	} else {
		w.line(0, "%s: %s", id, builtinText(p, id))
		mode := builtinMode(p.Builtin, id)
		w.line(1, "mode: %s (%s)", mode, modeTexts[mode])
		if id == resolved.RuleCoupling {
			w.line(1, "in words: %s", couplingInWords)
		}
	}
	exceptions(&w, p, date, func(e resolved.Exception) bool { return e.Rule == id })
	return w.b.String()
}

// graph writes the modules by layer, inner first, each with its allowed
// dependencies. Modules with no layer come last.
func graph(p resolved.Policy) string {
	byLayer := map[string][]string{}
	for _, name := range slices.Sorted(maps.Keys(p.Modules)) {
		layer := p.Modules[name].Layer
		byLayer[layer] = append(byLayer[layer], name)
	}
	var w writer
	if len(p.Layers) == 0 {
		w.line(0, "layers: none")
	} else {
		w.line(0, "layers: %s (inner first)", strings.Join(p.Layers, ", "))
	}
	for _, layer := range p.Layers {
		w.line(0, "%s:", layer)
		graphModules(&w, p, byLayer[layer])
	}
	if names := byLayer[""]; len(names) > 0 {
		w.line(0, "no layer:")
		graphModules(&w, p, names)
	}
	return w.b.String()
}

func graphModules(w *writer, p resolved.Policy, names []string) {
	if len(names) == 0 {
		w.line(1, "no module")
		return
	}
	for _, name := range names {
		w.line(1, "%s -> %s", name, listOrNone(p.Modules[name].DependsOn))
	}
}
