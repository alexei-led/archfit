// Package agenttask assembles the structured repair-task block (spec §13) from
// gate findings. One task per ACTIVE gate finding, derived deterministically
// from the finding plus rule/module configuration — a coding agent consumes
// the block mechanically: goal, constraints, files, validation commands.
package agenttask

import (
	"fmt"
	"path"
	"sort"
	"strings"

	"github.com/alexei-led/archfit/internal/assessment/finding"
	"github.com/alexei-led/archfit/internal/assessment/result"
	"github.com/alexei-led/archfit/internal/model/evidence"
)

// matchedByModuleKey mirrors internal/assessment/rules' unexported matchedByModule
// MatchedBy key ("module") — the two packages agree on the key by convention,
// not by import, since agenttask must not depend on the rules package.
const matchedByModuleKey = "module"

// Rule types whose violation survives any route to the target, its public API
// included. They select a repair goal, and forbidsTarget keeps the target's
// public surface out of their constraints.
const (
	ruleTypeForbiddenDependency     = "forbidden_dependency"
	ruleTypeForbiddenLayerDirection = "forbidden_layer_direction"
	ruleTypeCycle                   = "cycle"
	ruleTypeNewCrossModule          = "new_cross_module_dependency"
)

// Rule types with a dedicated goal template, and the MatchedBy keys those
// templates read: a module cycle lists its members, a pattern finding names its
// pattern. Both agree with internal/assessment/rules by convention.
const (
	ruleTypeModuleCycle       = "module_cycle"
	ruleTypeModuleDeps        = "module_dependencies"
	edgeKindModuleDependency  = "module_dependency" // a module-pair finding's edge kind
	matchedByViolatesKey      = "violates"
	matchedByCycleModulesKey  = "cycle_modules"
	matchedByCycleSizeKey     = "cycle_size"
	ruleTypeForbiddenPattern  = "forbidden_pattern"
	matchedByPatternKey       = "pattern"
	matchedBySubjectKey       = "subject"
	matchedBySuggestedPathKey = "suggested_path"
)

// PathResolver carries the filesystem facts filesFor needs to turn a config
// module key, a Rust "crate::mod" module key, or a Python dotted module key
// into a path that actually exists on disk — without agenttask itself ever
// touching the filesystem. Build it once per run from the LOC walk's
// FileClassIndex (KnownFiles) and the Rust extractor's crate roots
// (CrateRootDirs); the composition root (cmd/) owns the I/O.
//
// The zero value disables resolution: every candidate passes through
// unchanged, matching the pre-resolver behavior relied on by existing callers
// and tests that construct a Finding's Edge/Locations with paths they already
// know are real.
type PathResolver struct {
	knownFiles     map[string]struct{}
	knownDirs      map[string]struct{}
	crateRootDirs  map[string]string
	moduleRootDirs map[string]string
	onDisk         func(string) bool
}

// NewPathResolver builds a PathResolver from already-gathered facts:
// knownFiles is every repo-relative file path seen by the LOC walk
// (SizeSignals.FileClassIndex keys); crateRootDirs maps a Rust crate name to
// its repo-relative directory (from graph.CrateRoot); moduleRootDirs maps a
// config module name to its declared Paths root (policy.ModuleRootDirs),
// the last-resort fallback when nothing else resolves. A nil knownFiles
// disables resolution (see PathResolver).
//
// onDisk (optional, nil-safe) reports whether a repo-relative path exists on
// disk — the composition root passes an os.Stat closure. It backstops
// knownFiles misses: the LOC walk skips directories (mocks/, target/, venv/)
// that the extractor exclusions do not, so a real edge endpoint under one of
// them is absent from the index yet must not be dropped — the files[]
// contract is "exists on disk", not "was seen by the LOC walk". The closure
// must itself reject paths that are absolute or escape the scan root after
// OS-path conversion (filepath.IsLocal) — the resolver's slash-only guard
// below cannot see OS-specific separators like `..\` or `C:\`.
func NewPathResolver(knownFiles map[string]struct{}, crateRootDirs, moduleRootDirs map[string]string, onDisk func(string) bool) PathResolver {
	if knownFiles == nil {
		return PathResolver{crateRootDirs: crateRootDirs, moduleRootDirs: moduleRootDirs}
	}
	knownDirs := make(map[string]struct{}, len(knownFiles))
	for f := range knownFiles {
		for dir := f; ; {
			i := strings.LastIndexByte(dir, '/')
			if i < 0 {
				break
			}
			dir = dir[:i]
			if _, seen := knownDirs[dir]; seen {
				break // ancestors already recorded
			}
			knownDirs[dir] = struct{}{}
		}
	}
	return PathResolver{
		knownFiles:     knownFiles,
		knownDirs:      knownDirs,
		crateRootDirs:  crateRootDirs,
		moduleRootDirs: moduleRootDirs,
		onDisk:         onDisk,
	}
}

// escapesScanRoot reports whether p points outside the analyzed tree:
// absolute, or cleaning to a ".."-prefixed path. This slash guard is the
// platform-independent first line; the onDisk closure owns the OS-aware
// locality check (see NewPathResolver).
func escapesScanRoot(p string) bool {
	clean := path.Clean(p)
	return strings.HasPrefix(clean, "/") || clean == ".." || strings.HasPrefix(clean, "../")
}

// exists reports whether p is in the LOC-walk index (file or ancestor dir) or,
// failing that, exists on disk per the onDisk callback — the index is a fast
// under-approximation of the disk (its walk skips mocks/, target/, venv/).
// The escape guard runs here, not only in resolve, so derived candidates
// (Rust crate::mod probes built from crateRootDirs) can never leak an
// out-of-tree path through the onDisk closure.
func (r PathResolver) exists(p string) bool {
	if escapesScanRoot(p) {
		return false
	}
	if _, ok := r.knownFiles[p]; ok {
		return true
	}
	if _, ok := r.knownDirs[p]; ok {
		return true
	}
	return r.onDisk != nil && r.onDisk(p)
}

// resolve turns a candidate path/key into one that exists on disk, or reports
// false when it cannot be resolved. Resolution order: literal file or
// directory (index first, then disk), Rust "crate::mod" (module file under
// the crate's src/, then the crate dir), Python dotted module (the local
// pythonModuleFileCandidates candidate list, then the dots-to-slashes
// directory). Disabled (knownFiles nil) trusts every non-empty, non-escaping
// candidate, matching pre-resolver behavior. Candidates that escape the scan
// root (absolute, or cleaning to a ".."-prefixed path — e.g. a module Paths
// glob like "../outside/**" feeding the policy.ModuleRootDirs fallback) are always
// rejected, both here and on every derived probe in exists (escapesScanRoot):
// files[] must never point outside the analyzed tree.
func (r PathResolver) resolve(candidate string) (string, bool) {
	if candidate == "" {
		return "", false
	}
	if escapesScanRoot(candidate) {
		return "", false
	}
	if r.knownFiles == nil {
		return candidate, true
	}
	if r.exists(candidate) {
		return candidate, true
	}
	if dir, ok := r.crateRootDirs[candidate]; ok {
		// A crate-level node or module root: a package name or crate
		// identifier, never a path. Root crates (Dir "") resolve to src.
		if dir != "" && r.exists(dir) {
			return dir, true
		}
		if src := path.Join(dir, "src"); r.exists(src) {
			return src, true
		}
	}
	if crate, modPath, ok := strings.Cut(candidate, "::"); ok {
		if dir, ok := r.crateRootDirs[crate]; ok {
			// Root crates carry Dir "" — path.Join drops the empty segment, so
			// their module files probe as "src/<mod>.rs" and their dir
			// fallback as "src".
			rel := strings.ReplaceAll(modPath, "::", "/")
			base := path.Join(dir, "src", rel)
			for _, cand := range []string{base + ".rs", path.Join(base, "mod.rs")} {
				if r.exists(cand) {
					return cand, true
				}
			}
			if dir != "" && r.exists(dir) {
				return dir, true
			}
			if src := path.Join(dir, "src"); r.exists(src) {
				return src, true
			}
		}
	}
	if strings.Contains(candidate, ".") && !strings.Contains(candidate, "/") {
		for _, cand := range pythonModuleFileCandidates(candidate) {
			if r.exists(cand) {
				return cand, true
			}
		}
		if dir := strings.ReplaceAll(candidate, ".", "/"); r.exists(dir) {
			return dir, true
		}
	}
	return "", false
}

// pythonModuleFileCandidates maps a dotted Python module path to its candidate
// source files (mirrors the module-node convention used by the graph extractor).
// Includes both flat-layout and "src/"-layout candidates. Kept inline here so
// the assessment agenttask package need not import the raw graph convention.
func pythonModuleFileCandidates(modulePath string) []string {
	slashed := strings.ReplaceAll(modulePath, ".", "/")
	return []string{
		slashed + ".py", slashed + ".pyi", slashed + "/__init__.py",
		"src/" + slashed + ".py", "src/" + slashed + ".pyi", "src/" + slashed + "/__init__.py",
	}
}

// Build returns one AgentTask per active gate finding (status new or
// expired_waiver). Advisory findings never produce tasks — they are
// signals, not orders.
//
// ruleTypes maps rule ID → rule type (drives the goal template).
// modulePublic maps module name → its public path globs (becomes a constraint
// for the edge's target module when present).
// validation lists the exact commands that re-verify the gate; passed through
// verbatim to every task.
// syntaxFacts is the Diagnostic.SyntaxFacts slice (may be nil/empty when syntax
// is disabled). When non-empty, each task is enriched with the declarations
// found in its referenced files (compact agent context). When empty the output
// is structurally identical to pre-enrichment builds.
// seams is the seam ledger. A coupling-gate finding names only a module pair,
// so its task takes its files from that seam's qualifying edges.
//
// Output is sorted by FindingID; all nested slices carry a total order.
func Build(
	findings []finding.Finding,
	ruleTypes map[string]string,
	modulePublic map[string][]string,
	validation []string,
	syntaxFacts []evidence.SyntaxFact,
	seams []result.Seam,
	resolver PathResolver,
) []result.AgentTask {
	// Build a file→facts index once so the per-task lookup is O(1).
	var factsByFile map[string][]evidence.SyntaxFact
	if len(syntaxFacts) > 0 {
		factsByFile = make(map[string][]evidence.SyntaxFact, len(syntaxFacts))
		for _, sf := range syntaxFacts {
			factsByFile[sf.File] = append(factsByFile[sf.File], sf)
		}
	}
	seamPaths := make(map[[2]string][]string, len(seams))
	for _, s := range seams {
		seamPaths[[2]string{s.FromModule, s.ToModule}] = s.QualifyingPaths
	}

	tasks := []result.AgentTask{}
	for _, f := range findings {
		if f.Kind != "gate" {
			continue
		}
		if f.Status != finding.StatusNew && f.Status != finding.StatusExpiredWaiver {
			continue
		}
		seamGate := f.RuleID == finding.RuleIDCouplingGate
		var seamEvidence []string
		if seamGate {
			seamEvidence = seamPaths[[2]string{f.Edge.From.Module, f.Edge.To.Module}]
		}
		files := filesFor(f, seamEvidence, resolver)
		ruleType := ruleTypes[f.RuleID]
		task := result.AgentTask{
			FindingID:   f.ID,
			RuleID:      f.RuleID,
			RepairKind:  repairKind(ruleType, f.RuleID),
			Goal:        goalFor(ruleType, f),
			Constraints: constraintsFor(f, ruleType, modulePublic),
			Files:       files,
			Validation:  append([]string{}, validation...),
		}
		// A seam task's files span both modules — up to forty paths of
		// evidence — and a module-pair task's (module_cycle,
		// module_dependencies, a module-selector forbidden_dependency) span up
		// to fifty import sites of one module, repeated for every pair; their
		// declarations would bury the import to cut and dominate the report.
		modulePair := ruleType == ruleTypeModuleCycle || ruleType == ruleTypeModuleDeps || f.Edge.Kind == edgeKindModuleDependency
		if factsByFile != nil && !seamGate && !modulePair {
			task.Declarations = declarationsFor(files, factsByFile)
		}
		tasks = append(tasks, task)
	}
	sort.Slice(tasks, func(i, j int) bool { return tasks[i].FindingID < tasks[j].FindingID })
	return tasks
}

// goalFor instantiates the rule type's repair-goal template with the finding's
// edge. Unknown rule types fall back to the finding's Why text — never empty.
func goalFor(ruleType string, f finding.Finding) string {
	if f.RuleID == finding.RuleIDMapUncoveredPath {
		return uncoveredGoal(f)
	}
	from, to := f.Edge.From.Path, f.Edge.To.Path
	toMod := f.Edge.To.Module
	if toMod == "" {
		toMod = to
	}
	switch ruleType {
	case ruleTypeForbiddenDependency:
		return fmt.Sprintf("Remove the forbidden dependency from %s on %s; move shared behavior to a location permitted by the existing dependency rules.",
			endpointName(f.Edge.From), endpointName(f.Edge.To))
	case "public_api_only", "internal_api_access":
		return fmt.Sprintf("Replace the internal-API access from %s to %s with %s's public API.", from, to, toMod)
	case ruleTypeForbiddenLayerDirection:
		return fmt.Sprintf("Remove the layer-inverting dependency from %s to %s: inner layers must not import outer layers — introduce an abstraction in the inner layer instead.", from, to)
	case ruleTypeNewCrossModule:
		return fmt.Sprintf("Remove the new cross-module dependency from %s to %s. If the dependency is intentional, request an architecture-owner decision before changing policy or accepted debt.", from, to)
	case ruleTypeCycle:
		return "Break the import cycle: " + f.Why
	case ruleTypeModuleCycle:
		fromMod, toMod := f.Edge.From.Module, f.Edge.To.Module
		return fmt.Sprintf("Break the dependency cycle among declared modules %s by removing one direction of it: drop the %s -> %s dependency, or the dependency path from %s back to %s. Routing it through another module keeps the cycle.",
			cycleMembers(f), fromMod, toMod, toMod, fromMod)
	case ruleTypeModuleDeps:
		from := f.Edge.From.Module
		if from == "" {
			from = f.Edge.From.Path + " (owned by no declared module)"
		}
		return fmt.Sprintf("Remove the dependency of %s on module %s, which the module allowlist (%s) denies: drop the imports at the listed sites. Routing them through %s's public API keeps the violation. If the dependency is intended, ask the architecture owner to change the allowlist; do not edit it yourself.",
			from, f.Edge.To.Module, f.MatchedBy[matchedByViolatesKey], f.Edge.To.Module)
	case ruleTypeForbiddenPattern:
		return fmt.Sprintf("Remove the code in %s that matches forbidden pattern %q at the listed lines: replace it, or move that behavior to code the rule's scope does not cover.",
			from, f.MatchedBy[matchedByPatternKey])
	default:
		if f.Why != "" {
			return f.Why
		}
		return fmt.Sprintf("Resolve the %s violation on the edge %s -> %s.", f.RuleID, from, to)
	}
}

// uncoveredGoal asks the owner to place a directory of unowned production
// source in a module. A bare directory in paths: owns only a Go package, so the
// goal names the glob the finding verified to own every unowned file, or says
// what a glob must match when no single one is safe.
func uncoveredGoal(f finding.Finding) string {
	dir := f.MatchedBy[matchedBySubjectKey]
	how := "add a paths: glob that matches each file's path or its module node (Go package directory, dotted Python module, Rust crate)"
	if glob := f.MatchedBy[matchedBySuggestedPathKey]; glob != "" {
		how = fmt.Sprintf("add the paths: glob %q, which owns every unowned file there", glob)
	}
	return fmt.Sprintf("Ask the architecture owner which declared module owns the production source in %s. Then %s to that module in the archfit config, or declare a new module with it. Do not move the code to satisfy the check.",
		dir, how)
}

// endpointName names a finding endpoint in a goal: its path, or "module <m>"
// for a module-selector endpoint, which has no path.
func endpointName(e finding.Endpoint) string {
	if e.Path == "" && e.Module != "" {
		return "module " + e.Module
	}
	return e.Path
}

// maxGoalMemberBytes bounds the cycle member list a goal quotes; the App caps
// goal text, and a large strongly connected component can name every module.
const maxGoalMemberBytes = 300

// cycleMembers is the member list a module-cycle goal quotes: the whole list
// when it is short, otherwise its size and where the full list lives.
func cycleMembers(f finding.Finding) string {
	members := f.MatchedBy[matchedByCycleModulesKey]
	if len(members) <= maxGoalMemberBytes {
		return members
	}
	return fmt.Sprintf("(%s modules, listed in matched_by.cycle_modules)", f.MatchedBy[matchedByCycleSizeKey])
}

func repairKind(ruleType, ruleID string) string {
	if ruleType == ruleTypeNewCrossModule || ruleID == finding.RuleIDMapUncoveredPath {
		return "needs_owner_decision"
	}
	return "code_change"
}

// constraintsFor joins the finding's constraint text, the rule's rationale, its
// allowed alternatives, and — unless the rule forbids the target route — the
// target module's public surface. A forbidden dependency, an inverted layer, a
// node or module cycle, a dependency outside a module allowlist, or a new
// cross-module dependency is still a violation through the target's public
// API, so naming that surface would route the agent straight back into the
// violation.
func constraintsFor(f finding.Finding, ruleType string, modulePublic map[string][]string) []string {
	out := []string{}
	if f.Constraint != "" {
		out = append(out, f.Constraint)
	}
	if f.Rationale != "" {
		out = append(out, "rationale: "+f.Rationale)
	}
	for _, alt := range f.Alternatives {
		out = append(out, "allowed alternative: "+alt)
	}
	if forbidsTarget(ruleType) {
		return out
	}
	if pub := modulePublic[f.Edge.To.Module]; len(pub) > 0 {
		out = append(out, fmt.Sprintf("public surface of module %q: %v", f.Edge.To.Module, pub))
	}
	return out
}

// forbidsTarget reports whether a rule type's violation survives any route to
// the target, the target's public API included: the dependency is forbidden or
// outside the module allowlist, the layer stays inverted, the cycle stays
// closed, or the cross-module dependency stays new.
func forbidsTarget(ruleType string) bool {
	switch ruleType {
	case ruleTypeForbiddenDependency, ruleTypeForbiddenLayerDirection,
		ruleTypeCycle, ruleTypeModuleCycle, ruleTypeModuleDeps, ruleTypeNewCrossModule:
		return true
	}
	return false
}

// declarationsFor returns the SyntaxFacts for the given files, in file + start-line
// order. Returns nil (not an empty slice) when no facts match any file, so the
// AgentTask.Declarations field stays absent from JSON output (omitempty).
func declarationsFor(files []string, factsByFile map[string][]evidence.SyntaxFact) []evidence.SyntaxFact {
	var out []evidence.SyntaxFact
	for _, f := range files { // files is already sorted
		out = append(out, factsByFile[f]...)
	}
	return out // nil when nothing matched
}

// filesFor returns the deduplicated, sorted repo-relative files involved: edge
// endpoints plus every finding location, plus seamEvidence (a coupling-gate
// finding's qualifying-edge paths), each resolved to a path that exists on
// disk. An entry that cannot be resolved (e.g. a bare config module key or a
// dotted/"::" module id copied verbatim onto Edge.From/To.Path) is dropped
// rather than emitted — this is the contract agents trust blindly. When
// dropping leaves the set empty, a module root dir (config paths:) is used as
// a last resort; if that isn't resolvable either, Files is legitimately empty.
func filesFor(f finding.Finding, seamEvidence []string, r PathResolver) []string {
	set := map[string]struct{}{}
	add := func(candidate string) {
		if resolved, ok := r.resolve(candidate); ok {
			set[resolved] = struct{}{}
		}
	}
	for _, loc := range f.Locations {
		add(loc.File)
	}
	// Module-key endpoints (the public_api_* rules stamp Edge.From/To.Path
	// with the bare config module key, recorded in MatchedBy) are resolution
	// hints, not file evidence. Once a Location resolved, skip them: a key
	// that collides with an unrelated real path (module "docs" owning
	// src/domain/** next to a real docs/ dir) must not leak into files[] as
	// false evidence. With no resolved Location they remain the best-effort
	// probe (dotted Python id, crate::mod, dir named after the module).
	locResolved := len(set) > 0
	modKey := f.MatchedBy[matchedByModuleKey]
	for _, p := range []string{f.Edge.From.Path, f.Edge.To.Path} {
		if p == modKey && locResolved {
			continue
		}
		add(p)
	}
	for _, p := range seamEvidence {
		add(p)
	}

	if len(set) == 0 {
		for _, mod := range rootFallbackModules(f) {
			// The root goes through resolve, not a bare dir check: a Python
			// module's root is a dotted module-ID prefix that only the
			// dotted-candidate probe can turn into a real path.
			if root, ok := r.moduleRootDirs[mod]; ok && root != "" {
				if resolved, rok := r.resolve(root); rok {
					set[resolved] = struct{}{}
					break
				}
			}
		}
	}

	files := make([]string, 0, len(set))
	for p := range set {
		files = append(files, p)
	}
	sort.Strings(files)
	return files
}

// rootFallbackModules names the modules whose declared root stands in when no
// evidence resolved: the module a public_api_* finding is about, or the two
// ends of a coupling-gate seam or a module_cycle pair, source first because the
// repair starts there. A module_cycle pair over cargo-modules crate::mod
// modules has no import site to locate.
func rootFallbackModules(f finding.Finding) []string {
	if mod := f.MatchedBy[matchedByModuleKey]; mod != "" {
		return []string{mod}
	}
	if f.RuleID == finding.RuleIDCouplingGate || f.Edge.Kind == edgeKindModuleDependency {
		return []string{f.Edge.From.Module, f.Edge.To.Module}
	}
	return nil
}
