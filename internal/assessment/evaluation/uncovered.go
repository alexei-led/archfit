package evaluation

import (
	"fmt"
	"path"
	"sort"
	"strconv"
	"strings"

	"github.com/bmatcuk/doublestar/v4"

	"github.com/alexei-led/archfit/v3/internal/assessment/finding"
	"github.com/alexei-led/archfit/v3/internal/assessment/state"
	"github.com/alexei-led/archfit/v3/internal/model/fileclass"
	"github.com/alexei-led/archfit/v3/internal/policy"
	"github.com/alexei-led/archfit/v3/internal/relationship"
)

// maxUncoveredLocations bounds the files one finding lists: enough for a
// repair task to name the directory's source, never a list that grows with it.
const maxUncoveredLocations = 5

// matched_by keys of a map/uncovered_path finding. Counts only: a list of
// files would grow with the repository.
const (
	matchedBySubject        = "subject"
	matchedByUncoveredFiles = "uncovered_files"
	// matchedBySuggestedPath is a paths: glob that owns every unowned file of
	// the directory, in the files' own vocabulary; absent when no single glob
	// is safe to suggest.
	matchedBySuggestedPath = "suggested_path"
)

// languagePython is the Python language id, whose module nodes are dotted.
const languagePython = "python"

// unownedFile is one production source file no declared module owns, with the
// vocabulary its ownership was judged in.
type unownedFile struct {
	path, language, selector string
}

// uncoveredSource returns one map/uncovered_path finding per directory that
// holds production source no declared module owns. It reads the rule-scope
// source inventory, not the dependency graph: a producer that failed to load
// a package must not make that package look owned.
//
// A file counts when it is production (FileClass), inside the declared
// analysis scope, and analysed by a dependency producer: test, generated and
// vendor files, `exclude:` trees, switched-off languages, and source no
// extractor reads (UnanalysedFiles) never make the map incomplete. Ownership
// is the rule-scope ownership (fileOwner); a Rust file whose crate the
// inventory cannot name is undecidable and abstains.
//
// The finding is a gate finding when module_review.gate is fail, otherwise an
// advisory. Its ID hashes the rule and the directory only, so it stays stable
// while files are added to the directory. Each finding is bounded (a directory
// and at most maxUncoveredLocations files); the count is not: a cap would make
// `archfit baseline`, which captures every finding of one run, accept fewer
// directories than the next check reports, and a bound applied after the
// baseline reads accepted debt as fixed.
func uncoveredSource(ev RuleEvidence, p policy.AssessmentPolicy, gate policy.GateMode) []finding.Finding {
	if !p.Staleness.Enabled {
		return nil
	}
	mm := p.Topology.ModuleMap.WithCrateOwners(ev.CrateOwners)
	byDir := make(map[string][]unownedFile)
	for file, class := range ev.FileClasses {
		if class != fileclass.Production {
			continue
		}
		if _, unanalysed := ev.UnanalysedFiles[file]; unanalysed {
			continue
		}
		language, selector, supported := ruleFileSelector(mm, file, ev.SourceSelectors)
		if !supported {
			continue
		}
		if _, owned := fileOwner(mm, file, language, selector); owned {
			continue
		}
		if selector == "" && language == languageRust {
			continue
		}
		dir := path.Dir(file)
		byDir[dir] = append(byDir[dir], unownedFile{path: file, language: language, selector: selector})
	}
	dirs := make([]string, 0, len(byDir))
	for dir := range byDir {
		dirs = append(dirs, dir)
	}
	sort.Strings(dirs)
	kind := finding.KindAdvisory
	if gate == policy.GateFail {
		kind = finding.KindGate
	}
	out := make([]finding.Finding, 0, len(dirs))
	for _, dir := range dirs {
		files := byDir[dir]
		sort.Slice(files, func(i, j int) bool { return files[i].path < files[j].path })
		locs := make([]relationship.Location, 0, min(len(files), maxUncoveredLocations))
		for _, file := range files[:min(len(files), maxUncoveredLocations)] {
			locs = append(locs, relationship.Location{File: file.path})
		}
		matched := map[string]string{
			matchedBySubject:        dir,
			matchedByUncoveredFiles: strconv.Itoa(len(files)),
		}
		if glob := suggestedPath(dir, files); glob != "" {
			matched[matchedBySuggestedPath] = glob
		}
		out = append(out, finding.Finding{
			ID:        fingerprint(finding.RuleIDMapUncoveredPath, dir),
			Kind:      kind,
			RuleID:    finding.RuleIDMapUncoveredPath,
			Status:    finding.StatusNew,
			Why:       fmt.Sprintf("production source in %q is not owned by any declared module", dir),
			MatchedBy: matched,
			Locations: locs,
		})
	}
	return out
}

// suggestedPath returns a paths: glob that owns every unowned file of one
// directory, or "". A bare directory owns only a Go package; TypeScript and
// Rust files need a glob over their paths, a Python module a dotted glob, and
// a root directory a glob over its file extension. Every candidate is checked
// against the files exactly as fileOwner matches them, so a suggestion never
// leaves a listed file unowned.
func suggestedPath(dir string, files []unownedFile) string {
	for _, glob := range pathCandidates(dir, files) {
		if ownsAll(glob, files) {
			return glob
		}
	}
	return ""
}

func pathCandidates(dir string, files []unownedFile) []string {
	language := files[0].language
	for _, f := range files[1:] {
		if f.language != language {
			return nil
		}
	}
	switch {
	case language == languagePython:
		pkg := pythonPackage(files)
		if pkg == "" {
			return nil
		}
		return []string{pkg + ".**", "{" + pkg + "," + pkg + ".**}"}
	case dir == ".":
		return []string{"*" + path.Ext(files[0].path)}
	default:
		return []string{dir + "/**"}
	}
}

// pythonPackage is the dotted package the files' modules share, or "" when
// they share none (top-level modules).
func pythonPackage(files []unownedFile) string {
	pkg := ""
	for i, f := range files {
		own := f.selector
		if path.Base(f.path) != "__init__.py" {
			cut := strings.LastIndex(own, ".")
			if cut < 0 {
				return ""
			}
			own = own[:cut]
		}
		if i > 0 && own != pkg {
			return ""
		}
		pkg = own
	}
	return pkg
}

func ownsAll(glob string, files []unownedFile) bool {
	for _, f := range files {
		byPath, _ := doublestar.Match(glob, f.path)
		bySelector, _ := doublestar.Match(glob, f.selector)
		if !byPath && !bySelector {
			return false
		}
	}
	return true
}

// fileOwner is the declared module that owns one walked source file: a path
// glob over the file first, then the file's graph-node selector in its own
// language's vocabulary (a Go package dir, a dotted Python module, a Rust crate
// placed by the crate owners the module map carries). Rule scope and the map
// completeness check share it over the same module map — both attach the
// observed crate owners — so a file is never owned for one and unowned for the
// other.
func fileOwner(mm policy.ModuleMap, file, language, selector string) (string, bool) {
	if module, owned := mm.ModuleFor(file); owned {
		return module, true
	}
	if selector == "" {
		return "", false
	}
	return mm.ModuleForNode(selector, language)
}

// Reasons a file the change adds is not covered, in matched_by["reason"].
const (
	uncoveredUnowned          = "unowned"
	uncoveredExcluded         = "excluded"
	uncoveredLanguageDisabled = "language_disabled"
	uncoveredUnanalysed       = "unanalysed"
)

// matchedByReason and matchedByCause name an uncovered-change finding's why:
// the class, and the exclusion or switch behind it.
const (
	matchedByReason = "reason"
	matchedByCause  = "cause"
)

// UncoveredChangePrefix starts the reason of the decision entry the
// change_gate: fail posture adds. The agent result reads the same prefix.
const UncoveredChangePrefix = "uncovered change:"

// uncoveredChange returns one map/uncovered_change finding per production
// source file the change adds that no analysis covers. A file is covered when
// a declared module owns it and a dependency producer reads it; it is not when
// it is unowned, matched by an `exclude:` glob, in a language the config turns
// off, or in a language no producer analyses here. The finding names which, so
// an owner-approved exclusion stays distinguishable from a hole.
//
// Without a base nothing is "added", so nothing is reported: the files that
// were always unowned are map/uncovered_path's, not the change's.
func uncoveredChange(ev RuleEvidence, p policy.AssessmentPolicy) []finding.Finding {
	if len(ev.AddedFiles) == 0 {
		return nil
	}
	mm := p.Topology.ModuleMap.WithCrateOwners(ev.CrateOwners)
	added := make([]string, 0, len(ev.AddedFiles))
	for file := range ev.AddedFiles {
		added = append(added, file)
	}
	sort.Strings(added)

	type verdict struct{ reason, cause string }
	why := make(map[string]verdict)
	unownedByDir := make(map[string][]unownedFile)
	for _, file := range added {
		if cause, out := ev.UncoveredCauses[file]; out {
			reason := uncoveredLanguageDisabled
			if strings.HasPrefix(cause, "exclude:") {
				reason = uncoveredExcluded
			}
			why[file] = verdict{reason, cause}
			continue
		}
		if ev.FileClasses[file] != fileclass.Production {
			continue
		}
		if _, unanalysed := ev.UnanalysedFiles[file]; unanalysed {
			why[file] = verdict{uncoveredUnanalysed, "no dependency producer reads this file"}
			continue
		}
		language, selector, supported := ruleFileSelector(mm, file, ev.SourceSelectors)
		if !supported {
			continue
		}
		if _, owned := fileOwner(mm, file, language, selector); owned {
			continue
		}
		if selector == "" && language == languageRust {
			continue
		}
		why[file] = verdict{reason: uncoveredUnowned}
		dir := path.Dir(file)
		unownedByDir[dir] = append(unownedByDir[dir], unownedFile{path: file, language: language, selector: selector})
	}

	out := make([]finding.Finding, 0, len(why))
	for _, file := range added {
		v, uncovered := why[file]
		if !uncovered {
			continue
		}
		matched := map[string]string{matchedBySubject: file, matchedByReason: v.reason}
		explanation := "no declared module owns it"
		switch v.reason {
		case uncoveredUnowned:
			dir := path.Dir(file)
			if glob := suggestedPath(dir, unownedByDir[dir]); glob != "" {
				matched[matchedBySuggestedPath] = glob
			}
		case uncoveredExcluded:
			matched[matchedByCause] = v.cause
			explanation = "it matches " + v.cause
		case uncoveredLanguageDisabled:
			matched[matchedByCause] = v.cause
			explanation = "its language is switched off (" + v.cause + ")"
		case uncoveredUnanalysed:
			matched[matchedByCause] = v.cause
			explanation = v.cause
		}
		out = append(out, finding.Finding{
			ID:        fingerprint(finding.RuleIDMapUncoveredChange, file),
			Kind:      finding.KindAdvisory,
			RuleID:    finding.RuleIDMapUncoveredChange,
			Status:    finding.StatusNew,
			Why:       fmt.Sprintf("the change adds production source %q that no analysis covers: %s", file, explanation),
			MatchedBy: matched,
			Locations: []relationship.Location{{File: file}},
		})
	}
	return out
}

// uncoveredChangeUnevaluated is the decision entry the change_gate: fail
// posture adds when the change adds uncovered code: the change cannot pass
// because part of it was not read. It lists the files and the glob to add, and
// ignores waivers and the baseline: an accepted hole is still a hole.
func uncoveredChangeUnevaluated(findings []finding.Finding) (state.UnevaluatedRule, bool) {
	if len(findings) == 0 {
		return state.UnevaluatedRule{}, false
	}
	files := make([]string, 0, min(len(findings), maxUncoveredLocations))
	globs := map[string]struct{}{}
	reasons := map[string]struct{}{}
	for i, f := range findings {
		if i < maxUncoveredLocations {
			files = append(files, f.MatchedBy[matchedBySubject])
		}
		reasons[f.MatchedBy[matchedByReason]] = struct{}{}
		if glob := f.MatchedBy[matchedBySuggestedPath]; glob != "" {
			globs[glob] = struct{}{}
		}
	}
	list := strings.Join(files, ", ")
	if len(findings) > len(files) {
		list += fmt.Sprintf(" (+%d more)", len(findings)-len(files))
	}
	reason := fmt.Sprintf("%s %d added production file(s) are not covered (%s): %s", UncoveredChangePrefix, len(findings), joinSorted(reasons), list)
	if len(globs) > 0 {
		reason += "; add to a module's paths: " + joinSorted(globs)
	}
	return state.UnevaluatedRule{RuleID: finding.RuleIDMapUncoveredChange, Reason: reason}, true
}

func joinSorted(set map[string]struct{}) string {
	out := make([]string, 0, len(set))
	for k := range set {
		out = append(out, k)
	}
	sort.Strings(out)
	return strings.Join(out, ", ")
}
