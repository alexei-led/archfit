// Package agentout renders archfit.agent-result.v1: the compact digest of one
// run that a coding agent reads after an edit. It carries the verdict, ONE
// next action, and self-contained repairs. It is a projection of the same
// Document every other renderer reads; it never changes the verdict or the exit
// code, and the architecture state gets no agent-only field.
//
// The next action is decided here, once, from the report contract alone. A
// report adapter may import only the report DTOs (internal/arch_test.go), so
// the decision reads the state the pipeline already decided: active gate tasks
// with their origin, the unevaluated required rules, the required-analyzer
// gaps. A tripped metric ratchet is an ordinary repair task.
package agentout

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"slices"
	"sort"
	"strings"

	"github.com/alexei-led/archfit/v3/internal/model/report"
	reportports "github.com/alexei-led/archfit/v3/internal/report/ports"
)

// SchemaVersion identifies the agent-result contract.
const SchemaVersion = "archfit.agent-result.v1"

// FormatName is the --format value that selects this renderer.
const FormatName = "agent"

// NextAction is the one instruction the result gives the agent.
type NextAction string

// Next actions, in precedence order: the first matching one wins.
const (
	// ActionRepair: an in-scope repair needs a code change (a tripped metric
	// ratchet is such a repair).
	ActionRepair NextAction = "repair"
	// ActionAskOwner: every in-scope repair needs an owner decision, or a
	// required rule has a selector that matches nothing (a policy defect).
	ActionAskOwner NextAction = "ask_owner"
	// ActionRestoreEvidence: a required analyzer failed its gate, or a
	// required rule lacks producer evidence.
	ActionRestoreEvidence NextAction = "restore_evidence"
	// ActionReportBlocked: the verdict is blocked, but only by blockers
	// outside the scope of this change.
	ActionReportBlocked NextAction = "report_blocked"
	// ActionNone: nothing for the agent to do. Exit 2 with none is a correct
	// finish.
	ActionNone NextAction = "none"
)

// Repair kinds, as the agent task contract publishes them.
const (
	repairCodeChange         = "code_change"
	repairNeedsOwnerDecision = "needs_owner_decision"
)

// report.OriginPreExisting is the only origin outside the scope of a change.
// introduced, unknown, and an absent origin (no --base) are in scope, so a
// comparison that could not place a finding never hides a blocker.

// deadSelectorPrefix is the unevaluated-rule reason prefix for a selector that
// matches nothing: a policy defect for the owner, not missing evidence.
const deadSelectorPrefix = "selector matches nothing:"

// unmeasuredRatchetPrefix starts the reason of an unevaluated ratchet entry:
// the stored reference does not compare, so only the owner can move it.
const unmeasuredRatchetPrefix = "reference not comparable"

// agentFormatFlag is appended to the replayed validation command, so the
// re-run produces this same digest.
const agentFormatFlag = "--format " + FormatName

// Result is the archfit.agent-result.v1 document.
type Result struct {
	// SchemaVersion is always archfit.agent-result.v1.
	SchemaVersion string `json:"schema_version"`
	// Verdict is the architecture-state verdict of the same run.
	Verdict string `json:"verdict"`
	// NextAction is the one instruction for the agent.
	NextAction NextAction `json:"next_action"`
	// Summary is one deterministic line that counts what the result holds.
	Summary string `json:"summary"`
	// Repairs are the active gate tasks, grouped by edge and sorted in scope
	// first, then code changes, then severity, then lowest finding ID.
	Repairs []Repair `json:"repairs"`
	// EvidenceGaps are the required analyzers that did not run.
	EvidenceGaps []EvidenceGap `json:"evidence_gaps"`
	// UnevaluatedRules are the fail-gated rules the run could not evaluate.
	UnevaluatedRules []UnevaluatedRule `json:"unevaluated_rules"`
	// Omitted counts what the result leaves out.
	Omitted Omitted `json:"omitted"`
	// Truncated is true when the size budget cut text or moved entries into
	// Omitted. The full run is in --format json.
	Truncated bool `json:"truncated,omitempty"`
	// Validate is the command that re-runs this check and prints this digest.
	// It is absent when the run has no repair or advisory task; re-run the
	// command that produced this result instead.
	Validate string `json:"validate,omitempty"`
}

// Repair is one self-contained repair: every active gate task on one edge.
type Repair struct {
	// FindingIDs are the stable IDs of the grouped findings, sorted.
	FindingIDs []string `json:"finding_ids"`
	// RuleIDs are the rules the edge breaks, sorted.
	RuleIDs []string `json:"rule_ids"`
	// RepairKind is code_change when any grouped task needs a code change.
	RepairKind string `json:"repair_kind"`
	// Origin is set only for --base runs: introduced, pre_existing, or unknown.
	Origin string `json:"origin,omitempty"`
	// InScope is false only for a pre_existing origin.
	InScope bool `json:"in_scope"`
	// Severity is the highest severity of the grouped findings.
	Severity string `json:"severity"`
	// Edge is the dependency the repair cuts, when the finding has one.
	Edge *Edge `json:"edge,omitempty"`
	// At are the source locations of the grouped findings.
	At []report.Location `json:"at"`
	// Goal is the repair goal of the first grouped code-change task.
	Goal string `json:"goal"`
	// Constraints are the distinct constraints of every grouped task.
	Constraints []string `json:"constraints"`
	// Edit are the files to change: the task files on the source side of the
	// edge (import sites and the importing node), never the target.
	Edit []string `json:"edit"`
	// AtOmitted, EditOmitted, and ConstraintsOmitted count the entries the
	// size budget cut from this repair's lists.
	AtOmitted          int `json:"at_omitted,omitempty"`
	EditOmitted        int `json:"edit_omitted,omitempty"`
	ConstraintsOmitted int `json:"constraints_omitted,omitempty"`
}

// Edge names one dependency by its source and target node or module.
type Edge struct {
	From string `json:"from"`
	To   string `json:"to"`
	Kind string `json:"kind"`
}

// EvidenceGap is one required analyzer that did not run.
type EvidenceGap struct {
	Tool       string `json:"tool"`
	Gate       string `json:"gate"`
	InstallCmd string `json:"install_cmd,omitempty"`
}

// UnevaluatedRule is one fail-gated rule the run could not evaluate.
type UnevaluatedRule struct {
	RuleID string `json:"rule_id"`
	Reason string `json:"reason"`
}

// Omitted counts entries the result does not list.
type Omitted struct {
	// Repairs moved out by the size budget.
	Repairs int `json:"repairs"`
	// UnevaluatedRules moved out by the size budget.
	UnevaluatedRules int `json:"unevaluated_rules"`
	// Advisories are the active advisory findings. The agent result never
	// lists them; --format json does.
	Advisories int `json:"advisories"`
}

// Renderer writes archfit.agent-result.v1.
type Renderer struct{}

var _ reportports.Renderer = (*Renderer)(nil)

// New returns the agent-result renderer.
func New() *Renderer { return &Renderer{} }

// Format returns "agent".
func (r *Renderer) Format() string { return FormatName }

// Render writes the agent result as one line of JSON within the size budget.
func (r *Renderer) Render(d report.Document, w io.Writer) error {
	out, err := fit(Build(d))
	if err != nil {
		return err
	}
	_, err = w.Write(out)
	return err
}

// Build projects the document onto the agent result, before the size budget.
func Build(d report.Document) Result {
	s := d.State
	res := Result{
		SchemaVersion:    SchemaVersion,
		Verdict:          string(s.Verdict),
		Repairs:          buildRepairs(s),
		EvidenceGaps:     evidenceGaps(d.CoverageGaps),
		UnevaluatedRules: unevaluatedRules(s.Decision.UnevaluatedRequiredRules),
		Omitted:          Omitted{Advisories: activeAdvisories(s.Findings)},
		Validate:         validateCommand(s.AgentTasks, d.AdvisoryTasks),
	}
	res.NextAction = decide(res)
	res.Summary = summarize(res)
	return res
}

// decide applies the next-action precedence. A blocked verdict never maps to
// none: when nothing else explains it, the blocker is outside the scope.
func decide(r Result) NextAction {
	inScope, inScopeCode := 0, 0
	for _, rep := range r.Repairs {
		if !rep.InScope {
			continue
		}
		inScope++
		if rep.RepairKind == repairCodeChange {
			inScopeCode++
		}
	}
	deadSelector, missingEvidence := false, false
	for _, rule := range r.UnevaluatedRules {
		if strings.HasPrefix(rule.Reason, deadSelectorPrefix) || strings.HasPrefix(rule.Reason, unmeasuredRatchetPrefix) {
			deadSelector = true
		} else {
			missingEvidence = true
		}
	}
	gateFailed := false
	for _, gap := range r.EvidenceGaps {
		if gap.Gate == string(report.GateFail) {
			gateFailed = true
		}
	}
	switch {
	case inScopeCode > 0:
		return ActionRepair
	case inScope > 0 || deadSelector:
		return ActionAskOwner
	case gateFailed || missingEvidence:
		return ActionRestoreEvidence
	case r.Verdict == string(report.StateBlocked):
		return ActionReportBlocked
	default:
		return ActionNone
	}
}

// summarize counts what the result holds, in one fixed sentence.
func summarize(r Result) string {
	inScope, outScope := 0, 0
	for _, rep := range r.Repairs {
		if rep.InScope {
			inScope++
		} else {
			outScope++
		}
	}
	return fmt.Sprintf("%s: %d repairs in scope, %d outside scope; %d unevaluated required rules; %d evidence gaps",
		r.Verdict, inScope, outScope, len(r.UnevaluatedRules), len(r.EvidenceGaps))
}

// edgeKey groups the tasks of one dependency. A finding that names no
// dependency groups only with itself.
type edgeKey struct{ from, to, kind string }

// isDependency reports whether a finding edge names a dependency between two
// different endpoints. A public_api_* finding names its module on both sides
// with no kind: one finding per declaration, each its own repair.
func isDependency(e report.FindingEdge) bool {
	from, to := endpoint(e.From), endpoint(e.To)
	return from != "" && to != "" && from != to && e.Kind != ""
}

// buildRepairs groups the active gate tasks by edge: one import that breaks
// two rules is one repair.
func buildRepairs(s report.ArchitectureState) []Repair {
	findings := make(map[string]report.Finding, len(s.Findings))
	for _, f := range s.Findings {
		findings[f.ID] = f
	}
	var order []edgeKey
	groups := map[edgeKey][]report.AgentTask{}
	for _, task := range s.AgentTasks {
		key := edgeKey{from: task.FindingID}
		if f, ok := findings[task.FindingID]; ok && isDependency(f.Edge) {
			key = edgeKey{from: endpoint(f.Edge.From), to: endpoint(f.Edge.To), kind: f.Edge.Kind}
		}
		if _, seen := groups[key]; !seen {
			order = append(order, key)
		}
		groups[key] = append(groups[key], task)
	}
	repairs := make([]Repair, 0, len(order))
	for _, key := range order {
		repairs = append(repairs, buildRepair(groups[key], findings))
	}
	sort.SliceStable(repairs, func(i, j int) bool { return repairLess(repairs[i], repairs[j]) })
	return repairs
}

// endpoint names a finding endpoint: its node path, else its module.
func endpoint(e report.FindingEndpoint) string {
	if e.Path != "" {
		return e.Path
	}
	return e.Module
}

func buildRepair(tasks []report.AgentTask, findings map[string]report.Finding) Repair {
	slices.SortFunc(tasks, func(a, b report.AgentTask) int { return strings.Compare(a.FindingID, b.FindingID) })
	rep := Repair{RepairKind: repairNeedsOwnerDecision, InScope: true}
	var goalTask *report.AgentTask
	var files, origins []string
	sources := map[string]struct{}{}
	nodeEdge := false
	locations := map[report.Location]struct{}{}
	for i := range tasks {
		task := &tasks[i]
		rep.FindingIDs = append(rep.FindingIDs, task.FindingID)
		rep.RuleIDs = append(rep.RuleIDs, task.RuleID)
		if task.RepairKind == repairCodeChange {
			rep.RepairKind = repairCodeChange
			if goalTask == nil {
				goalTask = task
			}
		}
		origins = append(origins, task.Origin)
		for _, c := range task.Constraints {
			if !slices.Contains(rep.Constraints, c) {
				rep.Constraints = append(rep.Constraints, c)
			}
		}
		files = append(files, task.Files...)
		f, ok := findings[task.FindingID]
		if !ok {
			continue
		}
		if severityRank(f.Severity) > severityRank(rep.Severity) {
			rep.Severity = f.Severity
		}
		if rep.Edge == nil && (f.Edge.From.Path != "" || f.Edge.From.Module != "") {
			rep.Edge = &Edge{From: endpoint(f.Edge.From), To: endpoint(f.Edge.To), Kind: f.Edge.Kind}
		}
		if f.Edge.From.Path != "" || f.Edge.To.Path != "" {
			nodeEdge = true
			sources[f.Edge.From.Path] = struct{}{}
		}
		for _, loc := range f.Locations {
			locations[loc] = struct{}{}
			sources[loc.File] = struct{}{}
		}
	}
	if goalTask == nil {
		goalTask = &tasks[0]
	}
	rep.Goal = goalTask.Goal
	rep.Origin = mergeOrigins(origins)
	rep.InScope = rep.Origin != report.OriginPreExisting
	rep.FindingIDs = sortedUnique(rep.FindingIDs)
	rep.RuleIDs = sortedUnique(rep.RuleIDs)
	if rep.Constraints == nil {
		rep.Constraints = []string{}
	}
	rep.At = make([]report.Location, 0, len(locations))
	for loc := range locations {
		rep.At = append(rep.At, loc)
	}
	slices.SortFunc(rep.At, func(a, b report.Location) int {
		if c := strings.Compare(a.File, b.File); c != 0 {
			return c
		}
		return a.Line - b.Line
	})
	rep.Edit = editFiles(files, sources, nodeEdge)
	return rep
}

// mergeOrigins combines the origins of the tasks on one edge. A group is
// pre_existing only when every task in it is; any introduced task makes it
// introduced; equal origins (none, without --base) stay as they are; any
// other mix is unknown, which stays in scope.
func mergeOrigins(origins []string) string {
	all := func(want string) bool {
		for _, o := range origins {
			if o != want {
				return false
			}
		}
		return true
	}
	switch {
	case all(origins[0]):
		return origins[0]
	case slices.Contains(origins, report.OriginIntroduced):
		return report.OriginIntroduced
	default:
		return report.OriginUnknown
	}
}

// editFiles lists the files to change: the task files on the source side of
// the edge, which are the import sites and the importing node. Node IDs and
// resolved file paths differ for Python (dotted) and Rust (crate names,
// crate::mod), so the source side is read from the finding locations and the
// source node, never by comparing a resolved path with the target node. When
// the edge has a source node but neither it nor a location is a task file (a
// location-less crate::mod edge), the side of each file is unknown and edit is
// empty: edge.from names the source. Only a finding with no node on either
// side (a seam-gate or module-pair finding) lists every task file, since
// either module can be changed.
func editFiles(files []string, sources map[string]struct{}, nodeEdge bool) []string {
	all := sortedUnique(files)
	if !nodeEdge {
		return append([]string{}, all...)
	}
	out := []string{}
	for _, file := range all {
		if _, ok := sources[file]; ok {
			out = append(out, file)
		}
	}
	return out
}

// repairLess orders repairs: in scope first, then code changes, then higher
// severity, then the lowest finding ID.
func repairLess(a, b Repair) bool {
	if a.InScope != b.InScope {
		return a.InScope
	}
	if ac, bc := a.RepairKind == repairCodeChange, b.RepairKind == repairCodeChange; ac != bc {
		return ac
	}
	if ar, br := severityRank(a.Severity), severityRank(b.Severity); ar != br {
		return ar > br
	}
	return a.FindingIDs[0] < b.FindingIDs[0]
}

func severityRank(s string) int {
	switch s {
	case report.FindingSeverityCritical:
		return 4
	case report.FindingSeverityHigh:
		return 3
	case report.FindingSeverityMedium:
		return 2
	case report.FindingSeverityLow:
		return 1
	default:
		return 0
	}
}

func evidenceGaps(gaps []report.CoverageGap) []EvidenceGap {
	out := make([]EvidenceGap, 0, len(gaps))
	for _, g := range gaps {
		out = append(out, EvidenceGap{Tool: g.Tool, Gate: g.Gate, InstallCmd: g.InstallCmd})
	}
	slices.SortFunc(out, func(a, b EvidenceGap) int { return strings.Compare(a.Tool, b.Tool) })
	return out
}

func unevaluatedRules(rules []report.UnevaluatedRule) []UnevaluatedRule {
	out := make([]UnevaluatedRule, 0, len(rules))
	for _, r := range rules {
		out = append(out, UnevaluatedRule{RuleID: r.RuleID, Reason: r.Reason})
	}
	return out
}

func activeAdvisories(findings []report.Finding) int {
	n := 0
	for _, f := range findings {
		if f.Kind == report.FindingKindAdvisory && (f.Status == report.FindingStatusNew || f.Status == report.FindingStatusExpiredWaiver) {
			n++
		}
	}
	return n
}

// validateCommand replays the run's own validation command with the agent
// format. Every repair task and every advisory task carries the same command;
// a run with neither has none in the report contract.
func validateCommand(tasks []report.AgentTask, advisories []report.AdvisoryTask) string {
	for _, task := range tasks {
		if len(task.Validation) > 0 {
			return task.Validation[0] + " " + agentFormatFlag
		}
	}
	for _, task := range advisories {
		if len(task.Validation) > 0 {
			return task.Validation[0] + " " + agentFormatFlag
		}
	}
	return ""
}

func sortedUnique(in []string) []string {
	out := slices.Clone(in)
	slices.Sort(out)
	return slices.Compact(out)
}

// encode writes the result as one line of JSON. HTML escaping is off: goals
// quote edges as "a -> b", and an agent reads the text, not a browser.
func encode(r Result) ([]byte, error) {
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	if err := enc.Encode(r); err != nil {
		return nil, fmt.Errorf("agentout: encode: %w", err)
	}
	return buf.Bytes(), nil
}
