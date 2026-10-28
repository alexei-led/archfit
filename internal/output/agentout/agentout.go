// Package agentout renders archfit.agent-result.v1: the compact digest of one
// run that a coding agent reads after an edit. It carries the verdict, ONE
// next action, and self-contained repairs. It is a projection of the same
// Document every other renderer reads; it never changes the verdict or the exit
// code, and the architecture state gets no agent-only field.
//
// The change status and the next action are decided here, once, in judge, from
// the report contract alone. A report adapter may import only the report DTOs
// (internal/arch_test.go), so the decision reads the state the pipeline already
// decided: active gate tasks with their origin, the unevaluated required
// rules, the required-analyzer gaps. A tripped metric ratchet is an ordinary
// repair task. The hook and the CLI read the same Change; neither re-judges.
//
// The result separates two questions. change: may this change land (pass,
// fail, unknown). repository: what is wrong with the repository that this
// change did not cause (attention). The next action follows change only.
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

// Next actions. Each follows from the change status alone.
const (
	// ActionRepair: the change fails and an in-scope repair needs a code change
	// (a tripped metric ratchet is such a repair).
	ActionRepair NextAction = "repair"
	// ActionAskOwner: the change fails and every in-scope repair needs an owner
	// decision.
	ActionAskOwner NextAction = "ask_owner"
	// ActionRestoreEvidence: the change is unknown: a required analyzer failed
	// its gate, or a required rule lacks producer evidence.
	ActionRestoreEvidence NextAction = "restore_evidence"
	// ActionNone: nothing for the agent to do. Exit 2 with none is a correct
	// finish: the repository attention is not the change's.
	ActionNone NextAction = "none"
)

// ChangeStatus is whether the change may land.
type ChangeStatus string

// Change statuses.
const (
	// ChangePass: nothing in scope blocks and no evidence is missing.
	ChangePass ChangeStatus = "pass"
	// ChangeFail: an in-scope repair exists.
	ChangeFail ChangeStatus = "fail"
	// ChangeUnknown: evidence is missing, so the change cannot be judged. It
	// never passes.
	ChangeUnknown ChangeStatus = "unknown"
)

// Reasons a change is unknown beyond missing evidence. They are a closed set.
const (
	// ReasonPolicyChanged: the branch changed a protected policy file. The
	// branch cannot grade itself under rules it wrote, so an owner decides.
	ReasonPolicyChanged = "policy_changed_on_branch"
	// ReasonPolicyStale: the target changed a protected policy file after this
	// branch forked and the branch did not touch it. Update the branch.
	ReasonPolicyStale = "policy_stale_on_branch"
	// ReasonScopeUnresolved: the target branch or the base could not be
	// resolved, so what the change is stays unknown.
	ReasonScopeUnresolved = "scope_unresolved"
)

// Repository attention kinds.
const (
	// AttentionUnmeasuredRatchet: a metric ratchet cannot be evaluated because
	// the stored reference does not compare (profile drift, missing baseline).
	AttentionUnmeasuredRatchet = "unmeasured_ratchet"
	// AttentionDeadSelector: a required rule has a selector that matches
	// nothing, a policy defect for the owner.
	AttentionDeadSelector = "dead_selector"
	// AttentionExistingViolation: blockers that exist at the base and are not
	// this change's.
	AttentionExistingViolation = "existing_violation"
	// AttentionCouplingGate: the coupling gate's own explanation, for example
	// that seams are present and no comparable reference exists.
	AttentionCouplingGate = "coupling_gate"
	// AttentionBlockedWithoutFinding: the verdict is blocked and nothing in
	// the result names why.
	AttentionBlockedWithoutFinding = "blocked_without_finding"
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
	// NextAction is the one instruction for the agent. It follows Change only.
	NextAction NextAction `json:"next_action"`
	// Summary is one deterministic line that counts what the result holds.
	Summary string `json:"summary"`
	// Identity names what the run was judged under.
	Identity report.ResultIdentity `json:"identity"`
	// Scope says how the change and its base were resolved. It is absent when
	// the run resolved none.
	Scope *ScopeInfo `json:"scope,omitempty"`
	// Change is whether this change may land, and why not.
	Change Change `json:"change"`
	// Repository is what is wrong with the repository outside this change.
	Repository Repository `json:"repository"`
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

// ScopeInfo is how a check resolved the change.
type ScopeInfo struct {
	// Mode is none, explicit, target_branch, or merge_base.
	Mode string `json:"mode"`
	// Target and TargetSHA name the target branch and its tip.
	Target    string `json:"target,omitempty"`
	TargetSHA string `json:"target_sha,omitempty"`
	// Base is what the change was compared against; absent means the whole tree.
	Base string `json:"base,omitempty"`
}

// PolicyChange is the second result a changed policy needs: the same tree
// judged by the policy files the gate will use.
type PolicyChange struct {
	// Files are the protected policy files that differ from the target tip.
	Files []string `json:"files"`
	// UnderTargetPolicy is the change judged under the target tip's policy.
	UnderTargetPolicy PolicyView `json:"under_target_policy"`
}

// PolicyView is a status and its blockers under one policy.
type PolicyView struct {
	Status   ChangeStatus `json:"status"`
	Blockers []Repair     `json:"blockers"`
}

// Change is the part of the result that decides whether the change lands.
type Change struct {
	// Status is pass, fail, or unknown.
	Status ChangeStatus `json:"status"`
	// Reason says why the change is unknown when evidence is not the cause: a
	// Reason* constant.
	Reason string `json:"reason,omitempty"`
	// ReasonDetail is the one-sentence explanation of Reason.
	ReasonDetail string `json:"reason_detail,omitempty"`
	// PolicyChange is set with ReasonPolicyChanged.
	PolicyChange *PolicyChange `json:"policy_change,omitempty"`
	// Blockers are the in-scope repairs, grouped by edge and sorted code
	// changes first, then severity, then lowest finding ID.
	Blockers []Repair `json:"blockers"`
	// EvidenceGaps are the required analyzers that did not run.
	EvidenceGaps []EvidenceGap `json:"evidence_gaps"`
	// UnevaluatedRules are the fail-gated rules that lack producer evidence.
	UnevaluatedRules []UnevaluatedRule `json:"unevaluated_rules"`
}

// Blocks reports whether a hook or a gate must stop the change: the one
// judgement every consumer of the result shares.
func (c Change) Blocks() bool { return c.Status == ChangeFail }

// Repository is the repository-level state the change did not cause.
type Repository struct {
	// Attention lists what the owner should look at. The agent does not act on it.
	Attention []Attention `json:"attention"`
}

// Attention is one repository-level item.
type Attention struct {
	// Kind is one of the Attention* constants.
	Kind string `json:"kind"`
	// RuleIDs are the rules involved, sorted.
	RuleIDs []string `json:"rule_ids,omitempty"`
	// Count is how many findings the item stands for, when it groups findings.
	Count int `json:"count,omitempty"`
	// Reason says what is wrong, in one sentence.
	Reason string `json:"reason"`
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
	// Blockers moved out by the size budget.
	Blockers int `json:"blockers"`
	// UnevaluatedRules moved out by the size budget.
	UnevaluatedRules int `json:"unevaluated_rules"`
	// Attention moved out by the size budget.
	Attention int `json:"attention"`
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
	var blockers, existing []Repair
	for _, rep := range buildRepairs(s) {
		if rep.InScope {
			blockers = append(blockers, rep)
		} else {
			existing = append(existing, rep)
		}
	}
	change := Change{Blockers: nonNil(blockers), EvidenceGaps: evidenceGaps(d.CoverageGaps), UnevaluatedRules: []UnevaluatedRule{}}
	attention := []Attention{}
	for _, rule := range unevaluatedRules(s.Decision.UnevaluatedRequiredRules) {
		switch {
		case strings.HasPrefix(rule.Reason, deadSelectorPrefix):
			attention = append(attention, Attention{Kind: AttentionDeadSelector, RuleIDs: []string{rule.RuleID}, Reason: rule.Reason})
		case strings.HasPrefix(rule.Reason, unmeasuredRatchetPrefix):
			attention = append(attention, Attention{Kind: AttentionUnmeasuredRatchet, RuleIDs: []string{rule.RuleID}, Reason: rule.Reason})
		default:
			change.UnevaluatedRules = append(change.UnevaluatedRules, rule)
		}
	}
	for _, reason := range s.GateReasons {
		attention = append(attention, Attention{Kind: AttentionCouplingGate, Reason: reason})
	}
	var next NextAction
	change.Status, next = judge(change)
	if len(existing) > 0 {
		var rules []string
		count := 0
		for _, rep := range existing {
			rules = append(rules, rep.RuleIDs...)
			count += len(rep.FindingIDs)
		}
		attention = append(attention, Attention{
			Kind: AttentionExistingViolation, RuleIDs: sortedUnique(rules), Count: count,
			Reason: "blockers that exist at the base; this change did not add them",
		})
	}
	if s.Verdict == report.StateBlocked && change.Status == ChangePass && len(existing) == 0 {
		attention = append(attention, Attention{Kind: AttentionBlockedWithoutFinding, Reason: "the verdict is blocked and the result names no blocker; read --format json"})
	}
	scope := applyScope(&change, &next, d.Scope)
	res := Result{
		SchemaVersion: SchemaVersion,
		Scope:         scope,
		Verdict:       string(s.Verdict),
		NextAction:    next,
		Identity:      s.Identity,
		Change:        change,
		Repository:    Repository{Attention: attention},
		Omitted:       Omitted{Advisories: activeAdvisories(s.Findings)},
		Validate:      validateCommand(s.AgentTasks, d.AdvisoryTasks),
	}
	res.Summary = summarize(res, len(existing))
	return res
}

// applyScope folds the scope resolution into the change. A changed policy makes
// the change unknown whatever it finds: the branch cannot grade itself under
// rules it wrote. An unresolved scope never passes; it fails only when blockers
// are already in hand, because with no base every blocker is in scope.
func applyScope(change *Change, next *NextAction, sc *report.CheckScope) *ScopeInfo {
	if sc == nil {
		return nil
	}
	switch {
	case len(sc.PolicyChanged) > 0:
		change.Status, *next = ChangeUnknown, ActionAskOwner
		change.Reason = ReasonPolicyChanged
		change.ReasonDetail = fmt.Sprintf("%s differ from %s: an owner approves the policy change", strings.Join(sc.PolicyChanged, ", "), sc.Target)
		change.PolicyChange = policyChange(sc.PolicyChanged, sc.TargetPolicy)
	case len(sc.PolicyStale) > 0:
		change.Status, *next = ChangeUnknown, ActionRestoreEvidence
		change.Reason = ReasonPolicyStale
		change.ReasonDetail = fmt.Sprintf("%s changed on %s after this branch forked: merge or rebase %s", strings.Join(sc.PolicyStale, ", "), sc.Target, sc.Target)
		change.PolicyChange = policyChange(sc.PolicyStale, sc.TargetPolicy)
	case sc.Unresolved != "":
		change.Reason, change.ReasonDetail = ReasonScopeUnresolved, sc.Unresolved
		if change.Status == ChangePass {
			change.Status, *next = ChangeUnknown, ActionRestoreEvidence
		}
	}
	return &ScopeInfo{Mode: sc.Mode, Target: sc.Target, TargetSHA: sc.TargetSHA, Base: sc.Base}
}

// policyChange builds the second result: the files and the change judged under
// the target tip's policy, unknown when the target has no policy to judge with.
func policyChange(files []string, underTarget *report.Document) *PolicyChange {
	pc := &PolicyChange{Files: slices.Clone(files), UnderTargetPolicy: PolicyView{Status: ChangeUnknown, Blockers: []Repair{}}}
	if underTarget != nil {
		t := Build(*underTarget)
		pc.UnderTargetPolicy = PolicyView{Status: t.Change.Status, Blockers: t.Change.Blockers}
	}
	return pc
}

// judge is the one table that turns a change into a status and a next action.
// A blocker fails the change; missing evidence makes it unknown; anything else
// passes. Repository attention never enters the table.
func judge(c Change) (ChangeStatus, NextAction) {
	for _, rep := range c.Blockers {
		if rep.RepairKind == repairCodeChange {
			return ChangeFail, ActionRepair
		}
	}
	if len(c.Blockers) > 0 {
		return ChangeFail, ActionAskOwner
	}
	for _, gap := range c.EvidenceGaps {
		if gap.Gate == string(report.GateFail) {
			return ChangeUnknown, ActionRestoreEvidence
		}
	}
	if len(c.UnevaluatedRules) > 0 {
		return ChangeUnknown, ActionRestoreEvidence
	}
	return ChangePass, ActionNone
}

func nonNil[T any](in []T) []T {
	if in == nil {
		return []T{}
	}
	return in
}

// summarize counts what the result holds, in one fixed sentence.
func summarize(r Result, existing int) string {
	return fmt.Sprintf("%s: change %s, %d blockers, %d existing; %d unknown rules, %d evidence gaps; %d attention",
		r.Verdict, r.Change.Status, len(r.Change.Blockers), existing,
		len(r.Change.UnevaluatedRules), len(r.Change.EvidenceGaps), len(r.Repository.Attention))
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
