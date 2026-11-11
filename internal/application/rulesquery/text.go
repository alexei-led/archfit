package rulesquery

import (
	"fmt"
	"strings"
	"time"

	"github.com/alexei-led/archfit/v3/internal/policy/resolved"
)

// builtinTexts say each built-in rule in the words of design 6.2. coupling
// takes min_balance.
var builtinTexts = map[string]string{
	resolved.RuleGraph:       "a dependency between two modules must be an allowed dependency",
	resolved.RuleCoupling:    "new knowledge across a module boundary needs a balance of at least %d",
	resolved.RuleDuplication: "knowledge that another module already owns is not copied",
	resolved.RuleCycle:       "no new cycle between modules",
	resolved.RuleLayers:      "dependencies follow the layer order",
	resolved.RuleUnowned:     "new production code belongs to a module",
}

// couplingInWords is the coupling rule in words, from design 6.2.
const couplingInWords = "a module may use the public part of a module it is allowed to depend on; " +
	"it may reach past the public part only into a module declared low"

// modeTexts say what each mode does with an error.
var modeTexts = map[resolved.Mode]string{
	resolved.ModeDeny: "fix the code",
	resolved.ModeAsk:  "an owner may approve",
	resolved.ModeOff:  "not checked",
}

// builtinLine writes a built-in rule with its mode in parentheses.
func (w *writer) builtinLine(indent int, p resolved.Policy, id string) {
	mode := builtinMode(p.Builtin, id)
	w.line(indent, "%s: %s (%s: %s)", id, builtinText(p, id), mode, modeTexts[mode])
}

func builtinText(p resolved.Policy, id string) string {
	if id == resolved.RuleCoupling {
		return fmt.Sprintf(builtinTexts[id], p.Builtin.MinBalance)
	}
	return builtinTexts[id]
}

func builtinMode(b resolved.Builtin, id string) resolved.Mode {
	switch id {
	case resolved.RuleGraph:
		return b.Graph
	case resolved.RuleCoupling:
		return b.Coupling
	case resolved.RuleDuplication:
		return b.Duplication
	case resolved.RuleCycle:
		return b.Cycle
	case resolved.RuleLayers:
		return b.Layers
	default:
		return b.Unowned
	}
}

// customText says what a custom rule asks, after its id.
func customText(r resolved.Rule) string {
	switch r.Form {
	case resolved.FormDeny:
		return strings.Join(r.From, ", ") + " must not depend on " + strings.Join(r.To, ", ")
	case resolved.FormOnly:
		return r.Use + " is used only in " + strings.Join(r.In, ", ")
	default:
		return r.Note
	}
}

// writer indents lines.
type writer struct {
	b strings.Builder
}

func (w *writer) line(indent int, format string, args ...any) {
	w.b.WriteString(strings.Repeat("  ", indent))
	fmt.Fprintf(&w.b, format, args...)
	w.b.WriteString("\n")
}

// field writes "name: text" when text is not empty.
func (w *writer) field(indent int, name, text string) {
	if text != "" {
		w.line(indent, "%s: %s", name, text)
	}
}

// customRule writes a custom rule: its id and what it asks, then why and
// instead.
func (w *writer) customRule(indent int, r resolved.Rule) {
	w.line(indent, "%s: %s", r.ID, customText(r))
	w.field(indent+1, "why", r.Why)
	w.field(indent+1, "instead", r.Instead)
}

// exception writes an exception: its rule and place, its date, its reason.
func (w *writer) exception(indent int, e resolved.Exception, date time.Time) {
	place := e.From
	if e.To != "" {
		place += " -> " + e.To
	}
	expires := resolved.FormatExpires(e.Expires, date)
	if expires == "" {
		expires = "no date"
	}
	w.line(indent, "%s %s: %s", e.Rule, place, expires)
	w.field(indent+1, "why", e.Why)
}

func listOrNone(list []string) string {
	return orNone(strings.Join(list, ", "))
}

func orNone(s string) string {
	if s == "" {
		return "none"
	}
	return s
}
