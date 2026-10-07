// Package brief is the one view model the text and Markdown renderers share:
// the blockers with their location, goal, and check command; the next steps an
// architect takes; the diagnostics in reading order; and the step that closes
// each fact the run could not measure. It reads the published report contract
// only, so the two formats cannot derive different answers from one run.
package brief

import (
	"sort"
	"strconv"
	"strings"

	"github.com/alexei-led/archfit/internal/model/report"
)

// MaxNextSteps bounds the NEXT STEPS list: a reader acts on the first few.
const MaxNextSteps = 5

// ShortIDLen is how many characters of a finding ID a blocker shows; the
// finding index keeps the full ID.
const ShortIDLen = 8

// deadSelectorPrefix marks an unevaluated required rule whose selector matches
// nothing: a policy defect for the owner, not missing evidence. agentout reads
// the same prefix, so the human step and the agent's next_action agree.
const deadSelectorPrefix = "selector matches nothing:"

// syntheticRulePrefixes name the rule IDs assessment emits without a declared
// rule: coupling advisories, map review, and pinned-label staleness.
var syntheticRulePrefixes = []string{"bc/", "map/", "labels/"}

// Input is what the brief reads: the architecture state, plus the two facts
// that live beside it in the document.
type Input struct {
	State report.ArchitectureState
	// CoverageGaps are the analyzers a run wanted and could not use.
	CoverageGaps []report.CoverageGap
	// MetricRatchet is true when a metric ratchet blocked the run; the
	// renderer names the worsened metrics itself.
	MetricRatchet bool
}

// Blocker is one active gate finding with what a reader needs to act on it.
type Blocker struct {
	ShortID string
	RuleID  string
	// Subject is "from -> to" for a dependency, one module or path for a
	// finding about one thing, or a map finding's directory.
	Subject string
	// Location is "file:line" ("file" when the line is unknown) of the first
	// location; MoreLocations counts the rest.
	Location      string
	MoreLocations int
	Why           string
	// Goal and Checks come from the finding's agent task; both are empty when
	// the finding has none, never invented.
	Goal   string
	Checks []string
}

// View is the brief of one run.
type View struct {
	// VerdictReason explains a needs-attention verdict with no active finding:
	// which dimensions lack evidence. Empty otherwise.
	VerdictReason string
	Blockers      []Blocker
	// Diagnostics are the active advisory findings: findings of declared
	// (warn-gated) rules first, then the synthetic advisories, each group in
	// the document's own order.
	Diagnostics []report.Finding
	NextSteps   []string
}

// Build derives the brief from in.
func Build(in Input) View {
	s := in.State
	blockers, diagnostics := activeFindings(s)
	v := View{Diagnostics: diagnostics}
	tasks := make(map[string]report.AgentTask, len(s.AgentTasks))
	for _, t := range s.AgentTasks {
		tasks[t.FindingID] = t
	}
	for _, f := range blockers {
		v.Blockers = append(v.Blockers, blockerOf(f, tasks[f.ID]))
	}
	if s.Verdict == report.StateNeedsAttention && len(blockers) == 0 && len(diagnostics) == 0 {
		v.VerdictReason = evidenceReason(s.Dimensions)
	}
	v.NextSteps = nextSteps(in, v.Blockers)
	return v
}

// activeFindings splits the findings the dimension envelopes reference — the
// active ones — into blockers and diagnostics, in the document's order, with
// declared-rule diagnostics before synthetic ones.
func activeFindings(s report.ArchitectureState) (blockers, diagnostics []report.Finding) {
	active := map[string]string{}
	for _, dim := range s.Dimensions.All() {
		for _, ref := range dim.Findings {
			active[ref.ID] = ref.Kind
		}
	}
	var synthetic []report.Finding
	for _, f := range s.Findings {
		kind, ok := active[f.ID]
		switch {
		case !ok:
		case kind == report.FindingKindGate:
			blockers = append(blockers, f)
		case isSynthetic(f.RuleID):
			synthetic = append(synthetic, f)
		default:
			diagnostics = append(diagnostics, f)
		}
	}
	return blockers, append(diagnostics, synthetic...)
}

func isSynthetic(ruleID string) bool {
	for _, prefix := range syntheticRulePrefixes {
		if strings.HasPrefix(ruleID, prefix) {
			return true
		}
	}
	return false
}

func blockerOf(f report.Finding, task report.AgentTask) Blocker {
	b := Blocker{
		ShortID: shortID(f.ID), RuleID: f.RuleID, Subject: subject(f),
		Why: strings.TrimSpace(f.Why), Goal: task.Goal, Checks: task.Validation,
	}
	if len(f.Locations) > 0 {
		loc := f.Locations[0]
		b.Location = loc.File
		if loc.Line > 0 {
			b.Location += ":" + strconv.Itoa(loc.Line)
		}
		b.MoreLocations = len(f.Locations) - 1
	}
	return b
}

func shortID(id string) string {
	if len(id) <= ShortIDLen {
		return id
	}
	return id[:ShortIDLen]
}

// subject names what a finding is about: the dependency it forbids, the one
// module or path it concerns, or the directory a map finding names.
func subject(f report.Finding) string {
	from, to := endpoint(f.Edge.From), endpoint(f.Edge.To)
	switch {
	case from != "" && to != "" && from != to:
		return from + " -> " + to
	case from != "":
		return from
	case to != "":
		return to
	default:
		return f.MatchedBy["subject"]
	}
}

func endpoint(e report.FindingEndpoint) string {
	if e.Path != "" {
		return e.Path
	}
	return e.Module
}

// evidenceReason lists the dimensions whose evidence is incomplete.
func evidenceReason(dims report.Dimensions) string {
	var names []string
	for _, dim := range dims.All() {
		if dim.Status != report.MeasurementMeasured {
			names = append(names, dim.Name)
		}
	}
	if len(names) == 0 {
		return ""
	}
	return "evidence incomplete: " + strings.Join(names, ", ")
}

// nextSteps orders what to do: blockers, the metric ratchet, rules and
// analyzers that need evidence, the gate reference, module decisions, then
// the evidence that closes a measured gap. Steps repeat nothing and stop at
// MaxNextSteps.
func nextSteps(in Input, blockers []Blocker) []string {
	var steps []string
	add := func(step string) {
		if len(steps) < MaxNextSteps && !contains(steps, step) {
			steps = append(steps, step)
		}
	}
	for i, b := range blockers {
		if i == MaxNextSteps-1 && len(blockers) > MaxNextSteps {
			add("Fix the other " + strconv.Itoa(len(blockers)-i) + " blockers listed under BLOCKERS.")
			break
		}
		add("Fix blocker " + b.ShortID + " (" + b.RuleID + ").")
	}
	if in.MetricRatchet {
		add("Restore the worsened metrics listed under METRIC RATCHET.")
	}
	for _, rule := range in.State.Decision.UnevaluatedRequiredRules {
		if strings.HasPrefix(rule.Reason, deadSelectorPrefix) {
			add("Ask the owner to fix rule " + rule.RuleID + ": its selector matches nothing (archfit config lint).")
			continue
		}
		add("Restore the evidence rule " + rule.RuleID + " needs: archfit doctor --fix.")
	}
	if tools := gapTools(in.CoverageGaps); tools != "" {
		add("Install or fix the missing analyzers (" + tools + "): archfit doctor --fix.")
	}
	addFactSteps(in.State.Dimensions, categoryTools, add)
	if step := baselineStep(in.State, len(blockers)); step != "" {
		add(step)
	}
	addFactSteps(in.State.Dimensions, categoryModules, add)
	addFactSteps(in.State.Dimensions, categoryEvidence, add)
	return steps
}

// addFactSteps adds the step of every unknown fact of one category, in
// dimension order.
func addFactSteps(dims report.Dimensions, cat category, add func(string)) {
	for _, dim := range dims.All() {
		for _, u := range dim.Unknown {
			if fs, ok := factSteps[u.Fact]; ok && fs.category == cat {
				add(capitalize(fs.step) + ".")
			}
		}
	}
}

func gapTools(gaps []report.CoverageGap) string {
	tools := make([]string, 0, len(gaps))
	for _, gap := range gaps {
		if !contains(tools, gap.Tool) {
			tools = append(tools, gap.Tool)
		}
	}
	sort.Strings(tools)
	return strings.Join(tools, ", ")
}

// driftReasonPrefixes start every reason a stored reference gives when it was
// written under other inputs: the four comparison fingerprints and the
// measurement profile, named by their wire keys. A missing or pre-state
// baseline gives none of them.
var driftReasonPrefixes = []string{"config_hash", "model_hash", "labels_hash", "rubric_version", "measurement_profile"}

// baselineStep offers `archfit baseline` only when no blocker is active and no
// usable reference is stored: a blanket re-baseline would accept debt nobody
// reviewed. A stored reference written under other inputs (config, model,
// labels, rubric, or measurement profile drift) asks for a review first.
func baselineStep(s report.ArchitectureState, blockers int) string {
	ref := s.GateReference
	if ref == nil || ref.Status == report.ComparisonComparable {
		return ""
	}
	for _, reason := range ref.Reasons {
		for _, prefix := range driftReasonPrefixes {
			if strings.HasPrefix(reason, prefix) {
				return "Review why the gate reference does not compare (GATE REFERENCE) before you record a new one."
			}
		}
	}
	if blockers > 0 {
		return ""
	}
	return "Record a gate reference once the findings are reviewed: archfit baseline."
}

func contains(list []string, s string) bool {
	for _, v := range list {
		if v == s {
			return true
		}
	}
	return false
}

func capitalize(s string) string {
	if s == "" {
		return s
	}
	return strings.ToUpper(s[:1]) + s[1:]
}
