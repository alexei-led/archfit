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

	"github.com/alexei-led/archfit/internal/model/diagnostic"
	"github.com/alexei-led/archfit/internal/model/finding"
	"github.com/alexei-led/archfit/internal/model/graph"
)

// matchedByModuleKey mirrors internal/rules' unexported matchedByModule
// MatchedBy key ("module") — the two packages agree on the key by convention,
// not by import, since agenttask must not depend on the rules package.
const matchedByModuleKey = "module"

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
// config module name to its declared Paths root (config.ModuleRootDirs),
// the last-resort fallback when nothing else resolves. A nil knownFiles
// disables resolution (see PathResolver).
//
// onDisk (optional, nil-safe) reports whether a repo-relative path exists on
// disk — the composition root passes an os.Stat closure. It backstops
// knownFiles misses: the LOC walk skips directories (mocks/, target/, venv/)
// that the extractor exclusions do not, so a real edge endpoint under one of
// them is absent from the index yet must not be dropped — the files[]
// contract is "exists on disk", not "was seen by the LOC walk".
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

// exists reports whether p is in the LOC-walk index (file or ancestor dir) or,
// failing that, exists on disk per the onDisk callback — the index is a fast
// under-approximation of the disk (its walk skips mocks/, target/, venv/).
func (r PathResolver) exists(p string) bool {
	if _, ok := r.knownFiles[p]; ok {
		return true
	}
	if _, ok := r.knownDirs[p]; ok {
		return true
	}
	return r.onDisk != nil && r.onDisk(p)
}

// dirExists reports whether dir is a known or on-disk directory, or resolution
// is disabled (knownFiles nil), in which case every directory is trusted.
func (r PathResolver) dirExists(dir string) bool {
	if r.knownFiles == nil {
		return true
	}
	if _, ok := r.knownDirs[dir]; ok {
		return true
	}
	return r.onDisk != nil && r.onDisk(dir)
}

// resolve turns a candidate path/key into one that exists on disk, or reports
// false when it cannot be resolved. Resolution order: literal file or
// directory (index first, then disk), Rust "crate::mod" (module file under
// the crate's src/, then the crate dir), Python dotted module (the shared
// graph.BuiltinConventions candidate list, then the dots-to-slashes
// directory). Disabled (knownFiles nil) trusts every non-empty, non-escaping
// candidate, matching pre-resolver behavior. Candidates that escape the scan
// root (absolute, or cleaning to a ".."-prefixed path — e.g. a module Paths
// glob like "../outside/**" feeding the ModuleRootDirs fallback) are always
// rejected: the onDisk backstop joins candidates to ScanRoot unchecked, and
// files[] must never point outside the analyzed tree.
func (r PathResolver) resolve(candidate string) (string, bool) {
	if candidate == "" {
		return "", false
	}
	if clean := path.Clean(candidate); strings.HasPrefix(clean, "/") ||
		clean == ".." || strings.HasPrefix(clean, "../") {
		return "", false
	}
	if r.knownFiles == nil {
		return candidate, true
	}
	if r.exists(candidate) {
		return candidate, true
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
			if dir != "" && r.dirExists(dir) {
				return dir, true
			}
			if src := path.Join(dir, "src"); r.dirExists(src) {
				return src, true
			}
		}
	}
	if strings.Contains(candidate, ".") && !strings.Contains(candidate, "/") {
		for _, cand := range graph.BuiltinConventions.Lookup(graph.LangPython).ModuleFileCandidates(candidate) {
			if r.exists(cand) {
				return cand, true
			}
		}
		if dir := strings.ReplaceAll(candidate, ".", "/"); r.dirExists(dir) {
			return dir, true
		}
	}
	return "", false
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
//
// Output is sorted by FindingID; all nested slices carry a total order.
func Build(
	findings []finding.Finding,
	ruleTypes map[string]string,
	modulePublic map[string][]string,
	validation []string,
	syntaxFacts []diagnostic.SyntaxFact,
	resolver PathResolver,
) []diagnostic.AgentTask {
	// Build a file→facts index once so the per-task lookup is O(1).
	var factsByFile map[string][]diagnostic.SyntaxFact
	if len(syntaxFacts) > 0 {
		factsByFile = make(map[string][]diagnostic.SyntaxFact, len(syntaxFacts))
		for _, sf := range syntaxFacts {
			factsByFile[sf.File] = append(factsByFile[sf.File], sf)
		}
	}

	tasks := []diagnostic.AgentTask{}
	for _, f := range findings {
		if f.Kind != "gate" {
			continue
		}
		if f.Status != finding.StatusNew && f.Status != finding.StatusExpiredWaiver {
			continue
		}
		files := filesFor(f, resolver)
		task := diagnostic.AgentTask{
			FindingID:   f.ID,
			RuleID:      f.RuleID,
			Goal:        goalFor(ruleTypes[f.RuleID], f),
			Constraints: constraintsFor(f, modulePublic),
			Files:       files,
			Validation:  append([]string{}, validation...),
		}
		if factsByFile != nil {
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
	from, to := f.Edge.From.Path, f.Edge.To.Path
	toMod := f.Edge.To.Module
	if toMod == "" {
		toMod = to
	}
	switch ruleType {
	case "forbidden_dependency":
		return fmt.Sprintf("Remove the forbidden dependency from %s on %s; depend on %s's public API or move the shared code to an allowed location.", from, to, toMod)
	case "public_api_only", "internal_api_access":
		return fmt.Sprintf("Replace the internal-API access from %s to %s with %s's public API.", from, to, toMod)
	case "forbidden_layer_direction":
		return fmt.Sprintf("Remove the layer-inverting dependency from %s to %s: inner layers must not import outer layers — introduce an abstraction in the inner layer instead.", from, to)
	case "new_cross_module_dependency":
		return fmt.Sprintf("Review the new cross-module dependency from %s to %s: either remove it, route it through %s's public API, or accept it explicitly with `archfit baseline`.", from, to, toMod)
	case "cycle":
		return "Break the import cycle: " + f.Why
	default:
		if f.Why != "" {
			return f.Why
		}
		return fmt.Sprintf("Resolve the %s violation on the edge %s -> %s.", f.RuleID, from, to)
	}
}

// constraintsFor joins the finding's constraint text, its allowed
// alternatives, and the target module's public surface.
func constraintsFor(f finding.Finding, modulePublic map[string][]string) []string {
	out := []string{}
	if f.Constraint != "" {
		out = append(out, f.Constraint)
	}
	for _, alt := range f.Alternatives {
		out = append(out, "allowed alternative: "+alt)
	}
	if pub := modulePublic[f.Edge.To.Module]; len(pub) > 0 {
		out = append(out, fmt.Sprintf("public surface of module %q: %v", f.Edge.To.Module, pub))
	}
	return out
}

// declarationsFor returns the SyntaxFacts for the given files, in file + start-line
// order. Returns nil (not an empty slice) when no facts match any file, so the
// AgentTask.Declarations field stays absent from JSON output (omitempty).
func declarationsFor(files []string, factsByFile map[string][]diagnostic.SyntaxFact) []diagnostic.SyntaxFact {
	var out []diagnostic.SyntaxFact
	for _, f := range files { // files is already sorted
		out = append(out, factsByFile[f]...)
	}
	return out // nil when nothing matched
}

// filesFor returns the deduplicated, sorted repo-relative files involved: edge
// endpoints plus every finding location, each resolved to a path that exists
// on disk. An entry that cannot be resolved (e.g. a bare config module key or
// a dotted/"::" module id copied verbatim onto Edge.From/To.Path) is dropped
// rather than emitted — this is the contract agents trust blindly. When
// dropping leaves the set empty, the finding's module root dir (config
// paths:) is used as a last resort; if that isn't resolvable either, Files is
// legitimately empty.
func filesFor(f finding.Finding, r PathResolver) []string {
	set := map[string]struct{}{}
	add := func(candidate string) {
		if resolved, ok := r.resolve(candidate); ok {
			set[resolved] = struct{}{}
		}
	}
	add(f.Edge.From.Path)
	add(f.Edge.To.Path)
	for _, loc := range f.Locations {
		add(loc.File)
	}

	if len(set) == 0 {
		if mod := f.MatchedBy[matchedByModuleKey]; mod != "" {
			// The root goes through resolve, not a bare dir check: a Python
			// module's root is a dotted module-ID prefix that only the
			// dotted-candidate probe can turn into a real path.
			if root, ok := r.moduleRootDirs[mod]; ok && root != "" {
				if resolved, rok := r.resolve(root); rok {
					set[resolved] = struct{}{}
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
