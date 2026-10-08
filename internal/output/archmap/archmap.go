// Package archmap renders the architecture map: the declared modules in layer
// order and the observed seams between them, each with its policy status and
// integration strength. It reads the run's seam ledger and the configured
// layers; it adds no wire document of its own.
package archmap

import (
	"errors"
	"fmt"
	"io"
	"slices"
	"sort"
	"strings"

	"github.com/alexei-led/archfit/v3/internal/model/report"
)

// Output formats.
const (
	FormatMermaid = "mermaid"
	FormatText    = "text"
)

// Seam policy statuses, as seams[].policy publishes them.
const (
	policyViolation = "violation"
	policyAccepted  = "accepted"
)

// Labels for a module with no layer, and for a seam endpoint no module
// declares (a go.work member).
const (
	noLayer    = "(no layer)"
	undeclared = "(undeclared)"
)

// ErrUnknownFocus is returned when a --focus module is neither declared nor an
// endpoint of any seam.
var ErrUnknownFocus = errors.New("unknown --focus module")

// Module is one declared module and its configured layer ("" for none).
type Module struct {
	Name  string
	Layer string
}

// Input is everything the map reads.
type Input struct {
	// Layers is the configured layer order, innermost first.
	Layers []string
	// Modules are the declared modules.
	Modules []Module
	// Seams is the run's seam ledger, in ledger order.
	Seams []report.Seam
	// Focus keeps the named modules, their direct neighbours, and the seams
	// that touch a named module. Empty keeps everything.
	Focus []string
}

// node is one module drawn on the map.
type node struct {
	name, layer string
	declared    bool
	rank        int // position in drawing order: outermost ranked layer first
}

// view is the map after --focus.
type view struct {
	nodes   []node
	seams   []report.Seam
	omitted []report.Seam
}

// Render writes the map of in in format to w.
func Render(in Input, format string, w io.Writer) error {
	v, err := build(in)
	if err != nil {
		return err
	}
	var b strings.Builder
	switch format {
	case FormatMermaid:
		writeMermaid(&b, v)
	case FormatText:
		writeText(&b, v)
	default:
		return fmt.Errorf("unknown map format %q", format)
	}
	_, err = io.WriteString(w, b.String())
	return err
}

func build(in Input) (view, error) {
	layerOf, declared := map[string]string{}, map[string]bool{}
	for _, m := range in.Modules {
		layerOf[m.Name], declared[m.Name] = m.Layer, true
	}
	for _, s := range in.Seams {
		for _, name := range []string{s.FromModule, s.ToModule} {
			if _, ok := layerOf[name]; !ok {
				layerOf[name] = ""
			}
		}
	}
	for _, name := range in.Focus {
		if _, ok := layerOf[name]; !ok {
			return view{}, fmt.Errorf("%w %q: it is not a declared module or a seam endpoint", ErrUnknownFocus, name)
		}
	}
	keep := map[string]bool{}
	var v view
	for _, s := range in.Seams {
		if len(in.Focus) > 0 && !slices.Contains(in.Focus, s.FromModule) && !slices.Contains(in.Focus, s.ToModule) {
			v.omitted = append(v.omitted, s)
			continue
		}
		v.seams = append(v.seams, s)
		keep[s.FromModule], keep[s.ToModule] = true, true
	}
	for _, name := range in.Focus {
		keep[name] = true
	}
	for name, layer := range layerOf {
		if len(in.Focus) > 0 && !keep[name] {
			continue
		}
		rank := drawRank(in.Layers, layer)
		if !declared[name] {
			rank = len(in.Layers) + 2
		}
		v.nodes = append(v.nodes, node{name: name, layer: layer, declared: declared[name], rank: rank})
	}
	sort.Slice(v.nodes, func(i, j int) bool {
		if v.nodes[i].rank != v.nodes[j].rank {
			return v.nodes[i].rank < v.nodes[j].rank
		}
		return v.nodes[i].name < v.nodes[j].name
	})
	return v, nil
}

// drawRank orders layers outermost first, so a permitted dependency points
// down: the configured order is innermost first. A layer the order does not
// rank, then no layer, come last; build puts undeclared endpoints after them.
func drawRank(layers []string, layer string) int {
	if i := slices.Index(layers, layer); i >= 0 {
		return len(layers) - 1 - i
	}
	if layer != "" {
		return len(layers)
	}
	return len(layers) + 1
}

func writeMermaid(b *strings.Builder, v view) {
	b.WriteString("flowchart TB\n")
	ids := make(map[string]string, len(v.nodes))
	for i, n := range v.nodes {
		ids[n.name] = fmt.Sprintf("m%d", i)
		label := n.name
		if n.layer != "" {
			label += " · " + n.layer
		}
		fmt.Fprintf(b, "  %s[\"%s\"]\n", ids[n.name], mermaidText(label))
	}
	for _, s := range v.seams {
		fmt.Fprintf(b, "  %s %s|\"%s\"| %s\n", ids[s.FromModule], mermaidArrow(s.Policy), mermaidText(edgeLabel(s)), ids[s.ToModule])
	}
	if len(v.omitted) > 0 {
		fmt.Fprintf(b, "  %%%% omitted by --focus: %s\n", omittedSummary(v.omitted))
	}
}

// mermaidArrow draws a violation thick and accepted debt dotted.
func mermaidArrow(policy string) string {
	switch policy {
	case policyViolation:
		return "==>"
	case policyAccepted:
		return "-.->"
	default:
		return "-->"
	}
}

// mermaidText escapes the characters a quoted Mermaid label cannot hold.
func mermaidText(s string) string {
	return strings.NewReplacer(`"`, "#quot;", "\n", " ").Replace(s)
}

func edgeLabel(s report.Seam) string {
	return orDash(s.Policy) + " · " + orDash(s.Strength)
}

func orDash(s string) string {
	if s == "" {
		return "-"
	}
	return s
}

func writeText(b *strings.Builder, v view) {
	b.WriteString("ARCHITECTURE MAP\n\nMODULES (outermost layer first)\n\n")
	for _, n := range v.nodes {
		layer := n.layer
		switch {
		case !n.declared:
			layer = undeclared
		case layer == "":
			layer = noLayer
		}
		fmt.Fprintf(b, "  %-16s %s\n", layer, n.name)
	}
	fmt.Fprintf(b, "\nSEAMS (%d)\n\n", len(v.seams))
	if len(v.seams) == 0 {
		b.WriteString("  none\n")
	}
	for _, s := range v.seams {
		fmt.Fprintf(b, "  %-10s %s -> %s  (%s)\n", orDash(s.Policy), s.FromModule, s.ToModule, orDash(s.Strength))
	}
	if len(v.omitted) > 0 {
		fmt.Fprintf(b, "\nOMITTED BY --focus: %s\n", omittedSummary(v.omitted))
	}
}

// omittedSummary counts the seams --focus left out, by status, so a hidden
// violation always leaves a trace.
func omittedSummary(seams []report.Seam) string {
	counts := map[string]int{}
	for _, s := range seams {
		counts[orDash(s.Policy)]++
	}
	statuses := make([]string, 0, len(counts))
	for status := range counts {
		statuses = append(statuses, status)
	}
	sort.Strings(statuses)
	parts := make([]string, 0, len(statuses))
	for _, status := range statuses {
		parts = append(parts, fmt.Sprintf("%d %s", counts[status], status))
	}
	noun := "seams"
	if len(seams) == 1 {
		noun = "seam"
	}
	return fmt.Sprintf("%d %s (%s)", len(seams), noun, strings.Join(parts, ", "))
}
