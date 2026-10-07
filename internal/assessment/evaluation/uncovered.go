package evaluation

import (
	"fmt"
	"path"
	"sort"
	"strconv"

	"github.com/alexei-led/archfit/internal/assessment/finding"
	"github.com/alexei-led/archfit/internal/model/fileclass"
	"github.com/alexei-led/archfit/internal/policy"
	"github.com/alexei-led/archfit/internal/relationship"
)

// maxUncoveredDirs bounds the map/uncovered_path findings of one run. The first
// directories in path order are kept, so a kept ID never moves; a directory
// past the cap surfaces once a reported one is owned. Without a bound, a config
// that declares no module would report every directory of the repository.
const maxUncoveredDirs = 200

// maxUncoveredLocations bounds the files one finding lists: enough for a
// repair task to name the directory's source, never a list that grows with it.
const maxUncoveredLocations = 5

// matched_by keys of a map/uncovered_path finding. Counts only: a list of
// files would grow with the repository.
const (
	matchedBySubject        = "subject"
	matchedByUncoveredFiles = "uncovered_files"
	matchedByUncoveredDirs  = "uncovered_dirs_total"
)

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
// while files are added to the directory.
func uncoveredSource(ev RuleEvidence, p policy.AssessmentPolicy, gate policy.GateMode) []finding.Finding {
	if !p.Staleness.Enabled {
		return nil
	}
	mm := p.Topology.ModuleMap.WithCrateOwners(ev.CrateOwners)
	byDir := make(map[string][]string)
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
		byDir[dir] = append(byDir[dir], file)
	}
	dirs := make([]string, 0, len(byDir))
	for dir := range byDir {
		dirs = append(dirs, dir)
	}
	sort.Strings(dirs)
	total := strconv.Itoa(len(dirs))
	if len(dirs) > maxUncoveredDirs {
		dirs = dirs[:maxUncoveredDirs]
	}
	kind := finding.KindAdvisory
	if gate == policy.GateFail {
		kind = finding.KindGate
	}
	out := make([]finding.Finding, 0, len(dirs))
	for _, dir := range dirs {
		files := byDir[dir]
		sort.Strings(files)
		locs := make([]relationship.Location, 0, min(len(files), maxUncoveredLocations))
		for _, file := range files[:min(len(files), maxUncoveredLocations)] {
			locs = append(locs, relationship.Location{File: file})
		}
		out = append(out, finding.Finding{
			ID:     fingerprint(finding.RuleIDMapUncoveredPath, dir),
			Kind:   kind,
			RuleID: finding.RuleIDMapUncoveredPath,
			Status: finding.StatusNew,
			Why:    fmt.Sprintf("production source in %q is not owned by any declared module", dir),
			MatchedBy: map[string]string{
				matchedBySubject:        dir,
				matchedByUncoveredFiles: strconv.Itoa(len(files)),
				matchedByUncoveredDirs:  total,
			},
			Locations: locs,
		})
	}
	return out
}

// fileOwner is the declared module that owns one walked source file: a path
// glob over the file first, then the file's graph-node selector in its own
// language's vocabulary (a Go package dir, a dotted Python module, a Rust crate
// placed by the crate owners the module map carries). Rule scope and the map
// completeness check share it, so a file is never owned for one and unowned
// for the other.
func fileOwner(mm policy.ModuleMap, file, language, selector string) (string, bool) {
	if module, owned := mm.ModuleFor(file); owned {
		return module, true
	}
	if selector == "" {
		return "", false
	}
	return mm.ModuleForNode(selector, language)
}
