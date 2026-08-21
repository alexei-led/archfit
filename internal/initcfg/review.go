package initcfg

import (
	"fmt"
)

// ConfigReviewSchemaVersion versions the `config update --json` document.
const ConfigReviewSchemaVersion = "archfit.config-review.v1"

// Review statuses in priority order: action_required outranks review_available,
// which outranks no_known_issues. No status ever claims the config is complete
// or healthy — these checks only report what they can see.
const (
	ReviewStatusActionRequired  = "action_required"
	ReviewStatusReviewAvailable = "review_available"
	ReviewStatusNoKnownIssues   = "no_known_issues"
)

// Module issue codes emitted by DiffModules.
const (
	IssueMissingOwner           = "missing_owner"
	IssueMissingVolatilityInput = "missing_volatility_input"
	IssueMissingLayer           = "missing_layer"
)

// SettingCodeRustDeepAnalysis is the settings change `config update --apply`
// writes for a Rust project that lacks the deep-analysis defaults.
const SettingCodeRustDeepAnalysis = "add_rust_deep_analysis_defaults"

// SettingChange is one non-module config setting that `config update --apply`
// would write. It exists so the preview and the JSON document show every edit
// --apply makes, not only the module stanzas.
type SettingChange struct {
	Code   string `json:"code"`
	Reason string `json:"reason"`
}

// RustDeepAnalysisSetting is the settings entry for the Rust deep-analysis
// defaults (`languages.rust.enabled: auto` plus the cargo_modules and scip
// analyzers). The caller decides whether it applies; the code and its reason
// live here so preview and apply cannot drift apart.
func RustDeepAnalysisSetting() SettingChange {
	return SettingChange{
		Code:   SettingCodeRustDeepAnalysis,
		Reason: "rust sources found; --apply sets languages.rust.enabled: auto and enables the cargo_modules + scip analyzers",
	}
}

// ModuleIssue is one config-quality gap on a configured module that needs a
// human decision. Code is one of the Issue* constants.
type ModuleIssue struct {
	Code       string `json:"code"`
	Module     string `json:"module"`
	Reason     string `json:"reason"`
	NextAction string `json:"next_action"`
}

// newModuleIssue builds the issue for one code, keeping reason and next action
// next to the code they describe.
func newModuleIssue(code, moduleName string) ModuleIssue {
	issue := ModuleIssue{Code: code, Module: moduleName}
	switch code {
	case IssueMissingOwner:
		issue.Reason = "no `owner:` — cross-module distance falls back to code structure"
		issue.NextAction = fmt.Sprintf("set `owner:` on module %q", moduleName)
	case IssueMissingVolatilityInput:
		issue.Reason = "neither `subdomain:` nor `volatility:` — volatility stays undeclared"
		issue.NextAction = fmt.Sprintf("set `subdomain:` or `volatility:` on module %q", moduleName)
	case IssueMissingLayer:
		issue.Reason = "no `layer:` while an active forbidden_layer_direction rule checks layer direction"
		issue.NextAction = fmt.Sprintf("set `layer:` on module %q to one of the configured layers", moduleName)
	}
	return issue
}

// ConfigReview is the machine-readable `config update` review document. Every
// list is a non-null array so a consumer never has to distinguish null from
// empty.
type ConfigReview struct {
	SchemaVersion string          `json:"schema_version"`
	Status        string          `json:"status"`
	Structure     ReviewStructure `json:"structure"`
	Issues        []ModuleIssue   `json:"issues"`
	// UncheckedModules names every configured module the per-module field checks
	// did NOT evaluate, so an empty Issues list can never be read as "every
	// module is clean". It exists because that reading was wrong before: a
	// consumer saw `issues: []` over a config whose modules were mostly skipped.
	UncheckedModules  []ReviewUncheckedModule `json:"unchecked_modules"`
	ReviewSuggestions ReviewSuggestions       `json:"review_suggestions"`
}

// ReviewUncheckedModule is one configured module whose fields were not
// evaluated, with the reason it was skipped.
type ReviewUncheckedModule struct {
	Module string `json:"module"`
	Reason string `json:"reason"`
}

// reasonUncheckedUnmatched is why an unmatched configured module goes
// unevaluated: discovery found no code for it, so archfit cannot tell whether
// its fields describe anything that still exists.
const reasonUncheckedUnmatched = "discovery did not emit this module; its fields were not evaluated"

// reasonUncheckedPathless is why a pathless stanza goes unevaluated: it selects
// no source, so the per-module field checks skip it before raising anything —
// which means its missing owner or classification input appears in no issue and
// in no unclassified list.
const reasonUncheckedPathless = "module declares no paths; its fields were not evaluated"

// ReviewStructure holds the structural difference between the config and
// discovery. AddedModules, PathDrift, and Settings are the pending edits
// `--apply` would write; RemovedModules and NameDrift are review-only, because
// resolving either means rewriting or deleting a stanza and discarding the
// owner, subdomain, volatility, layer, and public settings it carries.
type ReviewStructure struct {
	AddedModules   []string          `json:"added_modules"`
	RemovedModules []string          `json:"removed_modules"`
	NameDrift      []ReviewNameDrift `json:"name_drift"`
	PathDrift      []ReviewPathDrift `json:"path_drift"`
	Settings       []SettingChange   `json:"settings"`
}

// ReviewNameDrift is one configured module that discovery re-emitted under a
// different name over exactly the same paths.
type ReviewNameDrift struct {
	ConfigModule     string   `json:"config_module"`
	DiscoveredModule string   `json:"discovered_module"`
	Paths            []string `json:"paths"`
}

// ReviewPathDrift is one module whose configured paths differ from discovery.
type ReviewPathDrift struct {
	Module          string   `json:"module"`
	ConfigPaths     []string `json:"config_paths"`
	DiscoveredPaths []string `json:"discovered_paths"`
}

// ReviewSuggestions holds the deterministic review-only proposals. `config
// update` never writes them; deploy topology and distance config are
// architecture decisions.
type ReviewSuggestions struct {
	DeployUnits    []DeployUnitSuggestion    `json:"deploy_units"`
	DistanceConfig []DistanceConfigCandidate `json:"distance_config"`
}

// BuildConfigReview projects an UpdateReport into the review document. It is a
// pure projection: every list keeps the order DiffModules and the suggestion
// builders already established, and nothing is recomputed or re-sorted.
func BuildConfigReview(r UpdateReport) ConfigReview {
	return ConfigReview{
		SchemaVersion: ConfigReviewSchemaVersion,
		Status:        reviewStatus(r),
		Structure: ReviewStructure{
			AddedModules:   moduleDefNames(r.Added),
			RemovedModules: existingModuleNames(r.Removed),
			NameDrift:      reviewNameDrift(r.NameDrift),
			PathDrift:      reviewPathDrift(r.PathDrift),
			Settings:       append(make([]SettingChange, 0, len(r.Settings)), r.Settings...),
		},
		Issues:           append(make([]ModuleIssue, 0, len(r.Issues)), r.Issues...),
		UncheckedModules: uncheckedModules(r.Removed, r.Pathless),
		ReviewSuggestions: ReviewSuggestions{
			DeployUnits:    append(make([]DeployUnitSuggestion, 0, len(r.DeployUnitSuggestions)), r.DeployUnitSuggestions...),
			DistanceConfig: append(make([]DistanceConfigCandidate, 0, len(r.DistanceConfigCandidates)), r.DistanceConfigCandidates...),
		},
	}
}

// RenderReviewStatus renders the one-line status header printed above the text
// report. It never states that the config is complete or healthy, and it names
// the unchecked modules so an issue count can never be read as a full audit.
func RenderReviewStatus(rev ConfigReview) string {
	unchecked := ""
	if n := len(rev.UncheckedModules); n > 0 {
		// Reason-neutral: unchecked now covers unmatched AND pathless stanzas, and
		// naming only one of them would misreport the other.
		unchecked = fmt.Sprintf(" (%d module(s) unchecked — see unchecked_modules)", n)
	}
	switch rev.Status {
	case ReviewStatusActionRequired:
		return fmt.Sprintf("status: %s — %d module issue(s), %d pending edit(s)%s\n",
			rev.Status, len(rev.Issues), pendingEditCount(rev.Structure), unchecked)
	case ReviewStatusReviewAvailable:
		return fmt.Sprintf("status: %s — nothing to apply; review items only%s\n", rev.Status, unchecked)
	default:
		// The unchecked clause belongs here too: "nothing detected" over skipped
		// modules is the exact misreading this line exists to prevent. Defensive —
		// BuildConfigReview cannot reach it, because both sources of
		// UncheckedModules (Removed, Pathless) also make HasReviewItems true and so
		// lift the status to review_available. A hand-built ConfigReview can.
		return fmt.Sprintf("status: %s — nothing detected by these checks%s\n", rev.Status, unchecked)
	}
}

// reviewStatus applies the fixed status priority.
//
// action_required means a human has to decide something OR `--apply` has a real
// edit to write. It deliberately excludes the two review-only buckets:
//
//   - NameDrift is a naming artifact of DiffModules' name-only matching (config
//     `internal/agenttask` vs discovered `agenttask` over one path set). Deriving
//     action_required from it made this repo's own reference config permanently
//     action_required while the recommended remedy destroyed it.
//   - Removed is never applied — deleting a stanza discards its settings — so it
//     is a review item, not a pending edit.
//
// Unclassified and Pathless sit in the same review-only tier. They are gaps, not
// edits, and Issues already carries the ones a human must decide, so they lift
// the status to review_available without reaching action_required.
func reviewStatus(r UpdateReport) string {
	if len(r.Issues) > 0 || HasPendingEdits(r) {
		return ReviewStatusActionRequired
	}
	if HasReviewItems(r) {
		return ReviewStatusReviewAvailable
	}
	return ReviewStatusNoKnownIssues
}

// HasReviewItems reports whether the report carries anything a human should
// look at even though `--apply` has nothing to write: a review suggestion, a
// naming difference, a configured module discovery did not emit, a module gap
// needing a decision, a module archfit cannot classify, or a stanza whose field
// checks were skipped. Callers use it to decide whether "nothing to apply" may
// be printed on its own.
//
// The three field-check buckets are deliberately in the list. They are exactly
// what makes a structurally in-sync config still action_required, so omitting
// them let `--apply` print a clean result over the same findings the run without
// --apply reports.
func HasReviewItems(r UpdateReport) bool {
	return HasReviewSuggestions(r) || len(r.NameDrift) > 0 || len(r.Removed) > 0 ||
		len(r.Issues) > 0 || len(r.Unclassified) > 0 || len(r.Pathless) > 0
}

// HasModuleEdits reports whether `config update --apply` has a MODULE stanza to
// write — the subset ApplyEdits handles.
//
// Removed is deliberately absent: commenting out a configured stanza discards
// its owner, subdomain, volatility, layer, and public settings, and modules are
// matched by name only, so a stanza discovery merely names differently would be
// destroyed. NameDrift is excluded for the same reason. Both surface in the
// report as review items instead.
//
// LLM role/volatility output is review-only too: it is rendered as a diff but
// never written by `config update --apply`.
func HasModuleEdits(r UpdateReport) bool {
	return len(r.Added) > 0 || len(r.PathDrift) > 0
}

// HasPendingEdits reports whether `config update --apply` would write anything
// at all: a module stanza, or a non-module setting the line-based editor writes
// separately. It is the single source for that question — a status that names
// edits apply would not make is the defect this replaces.
func HasPendingEdits(r UpdateReport) bool {
	return HasModuleEdits(r) || len(r.Settings) > 0
}

// HasReviewSuggestions reports whether any review-only proposal exists. It
// covers all five kinds so the text status stays honest under --ai-classify;
// the JSON document exposes only the two deterministic lists, which is
// sufficient because --json rejects --ai-classify and the other three kinds
// come from that pass alone.
func HasReviewSuggestions(r UpdateReport) bool {
	return len(r.Suggested) > 0 || len(r.DeployUnitSuggestions) > 0 ||
		len(r.DistanceConfigCandidates) > 0 || len(r.RuleSuggestions) > 0 ||
		len(r.ExternalSystemSuggestions) > 0
}

// pendingEditCount counts only what `--apply` would write, so the header can
// never name a change apply refuses to make.
func pendingEditCount(s ReviewStructure) int {
	return len(s.AddedModules) + len(s.PathDrift) + len(s.Settings)
}

func moduleDefNames(defs []ModuleDef) []string {
	out := make([]string, 0, len(defs))
	for _, d := range defs {
		out = append(out, d.Name)
	}
	return out
}

// uncheckedModules projects every configured module whose fields were not
// evaluated into the disclosure list, each with the reason it was skipped:
// discovery never emitted it, or the stanza declares no paths so checkModuleFields
// skipped it before raising anything. Name-drifted modules are NOT here:
// DiffModules' name-drift pass rescues them and their fields are checked like
// any other stanza.
//
// The two sets are disjoint by construction — checkModuleFields runs only over
// modules that are NOT in Removed — so no module can be listed twice.
func uncheckedModules(mods []ExistingModule, pathless []string) []ReviewUncheckedModule {
	out := make([]ReviewUncheckedModule, 0, len(mods)+len(pathless))
	for _, m := range mods {
		out = append(out, ReviewUncheckedModule{Module: m.Name, Reason: reasonUncheckedUnmatched})
	}
	for _, name := range pathless {
		out = append(out, ReviewUncheckedModule{Module: name, Reason: reasonUncheckedPathless})
	}
	return out
}

func existingModuleNames(mods []ExistingModule) []string {
	out := make([]string, 0, len(mods))
	for _, m := range mods {
		out = append(out, m.Name)
	}
	return out
}

func reviewNameDrift(drift []NameDrift) []ReviewNameDrift {
	out := make([]ReviewNameDrift, 0, len(drift))
	for _, d := range drift {
		out = append(out, ReviewNameDrift{
			ConfigModule:     d.ConfigName,
			DiscoveredModule: d.DiscoveredName,
			Paths:            append(make([]string, 0, len(d.Paths)), d.Paths...),
		})
	}
	return out
}

func reviewPathDrift(drift []PathDelta) []ReviewPathDrift {
	out := make([]ReviewPathDrift, 0, len(drift))
	for _, d := range drift {
		out = append(out, ReviewPathDrift{
			Module:          d.Name,
			ConfigPaths:     append(make([]string, 0, len(d.ConfigPaths)), d.ConfigPaths...),
			DiscoveredPaths: append(make([]string, 0, len(d.DiscoveredPaths)), d.DiscoveredPaths...),
		})
	}
	return out
}
