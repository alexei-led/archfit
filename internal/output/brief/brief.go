// Package brief is the one view model the text and Markdown renderers share:
// the blockers with their location, goal, and check command; the next steps an
// architect takes; the diagnostics in reading order; and the step that closes
// each fact the run could not measure. It reads the published report contract
// only, so the two formats cannot derive different answers from one run.
package brief

import (
	"slices"
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

// unmeasuredRatchetPrefix starts the reason of an unevaluated ratchet entry: the stored
// reference does not compare, which the reference step below already answers.
const unmeasuredRatchetPrefix = "reference not comparable"

// repairNeedsOwnerDecision is the agent-task repair kind of a blocker an owner
// decides; agentout reads the same value.
const repairNeedsOwnerDecision = "needs_owner_decision"

// syntheticRulePrefixes name the rule IDs assessment emits without a declared
// rule: coupling advisories, map review, and pinned-label staleness.
var syntheticRulePrefixes = []string{"bc/", "map/", "labels/"}

// Input is what the brief reads: the architecture state, plus the two facts
// that live beside it in the document.
type Input struct {
	State report.ArchitectureState
	// CoverageGaps are the analyzers a run wanted and could not use.
	CoverageGaps []report.CoverageGap
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
	// OwnerDecision is true when the task's repair is an architecture-owner
	// decision (repair_kind needs_owner_decision), not a code change.
	OwnerDecision bool
	// Origin is the finding's origin against the --base ref; empty without one.
	Origin string
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
	// referenceStep closes the gate-reference facts in this run's context:
	// review a drifted reference, fix the blockers first, or record one.
	referenceStep string
}

// StepFor is the NOT MEASURED suffix for one unknown fact of this run. The
// gate-reference facts read the run's reference and blockers, so NOT MEASURED
// and NEXT STEPS never disagree about `archfit baseline`.
func (v View) StepFor(fact string) string {
	if fs, ok := factSteps[fact]; ok && fs.category == categoryReference && v.referenceStep != "" {
		return "→ " + v.referenceStep
	}
	return Step(fact)
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
	v.referenceStep = referenceStep(s.GateReference, len(blockers))
	v.NextSteps = nextSteps(in, v.Blockers, v.referenceStep)
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
		OwnerDecision: task.RepairKind == repairNeedsOwnerDecision, Origin: f.Origin,
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

// shortID shortens a finding ID for a reader. A synthetic ID with a prefix
// (coupling-gate/<seam ID>) keeps the prefix, so two seams never share one
// short ID and the short ID is still a prefix of the full one.
func shortID(id string) string {
	prefix, rest := "", id
	if i := strings.LastIndex(id, "/"); i >= 0 {
		prefix, rest = id[:i+1], id[i+1:]
	}
	if len(rest) > ShortIDLen {
		rest = rest[:ShortIDLen]
	}
	return prefix + rest
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

// nextSteps orders what to do: blockers, rules and
// analyzers that need evidence, the gate reference, module decisions, then
// the evidence that closes a measured gap. Steps repeat nothing and stop at
// MaxNextSteps.
func nextSteps(in Input, blockers []Blocker, reference string) []string {
	var steps []string
	add := func(step string) {
		if len(steps) < MaxNextSteps && !slices.Contains(steps, step) {
			steps = append(steps, step)
		}
	}
	// Code repairs come before owner decisions, and owner decisions before
	// missing evidence: the order agentout.decide picks next_action in.
	ordered := make([]Blocker, 0, len(blockers))
	for _, owner := range []bool{false, true} {
		for _, b := range blockers {
			if b.OwnerDecision == owner {
				ordered = append(ordered, b)
			}
		}
	}
	for i, b := range ordered {
		if i == MaxNextSteps-1 && len(ordered) > MaxNextSteps {
			add("Fix the other " + strconv.Itoa(len(ordered)-i) + " blockers listed under BLOCKERS.")
			break
		}
		if b.OwnerDecision {
			add("Ask the owner to decide blocker " + b.ShortID + " (" + b.RuleID + ").")
			continue
		}
		add("Fix blocker " + b.ShortID + " (" + b.RuleID + ").")
	}
	rules := in.State.Decision.UnevaluatedRequiredRules
	for _, rule := range rules {
		if strings.HasPrefix(rule.Reason, deadSelectorPrefix) {
			add("Ask the owner to fix rule " + rule.RuleID + ": its selector matches nothing (archfit config lint).")
		}
	}
	for _, rule := range rules {
		if !strings.HasPrefix(rule.Reason, deadSelectorPrefix) && !strings.HasPrefix(rule.Reason, unmeasuredRatchetPrefix) {
			add("Restore the evidence rule " + rule.RuleID + " needs: archfit doctor --fix.")
		}
	}
	if tools := gapTools(in.CoverageGaps); tools != "" {
		add("Install or fix the missing analyzers (" + tools + "): archfit doctor --fix.")
	}
	addFactSteps(in.State.Dimensions, categoryTools, add)
	if in.State.GateReference != nil && reference != "" && reference != stepBlockersFirst {
		add(capitalize(reference) + ".")
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
		if !slices.Contains(tools, gap.Tool) {
			tools = append(tools, gap.Tool)
		}
	}
	sort.Strings(tools)
	return strings.Join(tools, ", ")
}

// storedReasonPrefixes start every reason a stored reference gives when it
// cannot be compared: the four comparison fingerprints and the measurement
// profile, named by their wire keys, and a stored baseline whose state or seam
// snapshot is incomplete. Only a missing baseline file gives none of them.
var storedReasonPrefixes = []string{"classification_hash", "model_hash", "labels_hash", "rubric_version", "measurement_profile", "stored baseline"}

// Gate-reference steps.
const (
	stepReviewReference = "review why the gate reference does not compare (GATE REFERENCE) before you record a new one"
	stepRecordReference = "record a gate reference once the findings are reviewed: archfit baseline, with the same -c and --root as this run"
	stepBlockersFirst   = "fix the blockers, then record a gate reference: archfit baseline, with the same -c and --root as this run"
)

// referenceStep closes the gate-reference gap of one run, or "" when the
// reference compares. A stored reference that does not compare asks for a
// review first: a blanket re-baseline would accept debt nobody reviewed.
// `archfit baseline` is the step only when no reference is stored, and only
// after the blockers are fixed.
func referenceStep(ref *report.StateComparison, blockers int) string {
	switch {
	case ref != nil && ref.Status == report.ComparisonComparable:
		return ""
	case ref != nil && storedReference(ref.Reasons):
		return stepReviewReference
	case blockers > 0:
		return stepBlockersFirst
	default:
		return stepRecordReference
	}
}

func storedReference(reasons []string) bool {
	for _, reason := range reasons {
		for _, prefix := range storedReasonPrefixes {
			if strings.HasPrefix(reason, prefix) {
				return true
			}
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
