package dotnet

import (
	"path"
	"slices"
	"strings"

	"github.com/alexei-led/archfit/v3/internal/model/graph"
)

// Fixes of a project gap whose cause the roslyn row already details.
const (
	fixRestore    = "restore the projects with NuGet access (`dotnet restore`); the roslyn row names each project's cause"
	fixUnreported = "the roslyn row names the projects archfit-roslyn did not report"
)

// DependencyGaps are the files whose project lacks complete file-level
// dependency evidence.
type DependencyGaps struct {
	// Files maps each such file to its project's gap: "<project> <status>".
	Files map[string]string
	// Fixes maps each gap to what restores it. Gaps with one cause share one
	// fix text.
	Fixes map[string]string
}

// LastDependencyGaps returns the gaps of files (root-relative,
// slash-separated) that a non-test project of the most recent Extract call
// compiles without complete file evidence. A file whose every compiling
// project the helper loaded complete is left out, as are the files of test
// projects (their edges are dropped by design) and files no project compiles.
//
// A project's file evidence is complete only when the helper loaded it
// complete. A partial project's unresolved symbols are missing candidates. A
// project on ProjectReference fallback (failed in the helper, unreported, a
// helper run without a report, or no helper) has project edges only: they
// match no selector below the project directory, and they omit the projects
// it uses through a transitive ProjectReference, which C# allows by default.
// A project whose MSBuild evaluation failed has no edges at all.
//
// A file belongs to the projects the helper reported it for (linked files
// included), else to the project with the deepest directory that holds it.
//
// attributed is false when the run's incompleteness cannot be pinned to
// projects: Extract did not finish, no project was evaluated, or a listed
// project is missing on disk. The coverage rows must then be read whole.
func (e *Extractor) LastDependencyGaps(files []string) (gaps DependencyGaps, attributed bool) {
	if !e.lastAttributable {
		return DependencyGaps{}, false
	}
	projectGaps, fixes := e.projectGaps()
	if len(projectGaps) == 0 {
		return DependencyGaps{}, true
	}
	dirs := make([]string, 0, len(e.last.Projects))
	for _, p := range e.last.Projects {
		dirs = append(dirs, path.Dir(p.Path))
	}
	extensions := graph.BuiltinConventions.Lookup(graph.LangDotNet).FileExtensions
	for _, file := range files {
		if !slices.Contains(extensions, path.Ext(file)) {
			continue
		}
		owners := e.lastMembers[file]
		if len(owners) == 0 {
			if owner, ok := deepestProject(e.last.Projects, dirs, file); ok {
				owners = []string{owner}
			}
		}
		for _, owner := range owners {
			if gap, incomplete := projectGaps[owner]; incomplete {
				if gaps.Files == nil {
					gaps.Files, gaps.Fixes = map[string]string{}, map[string]string{}
				}
				gaps.Files[file], gaps.Fixes[gap] = gap, fixes[gap]
				break
			}
		}
	}
	return gaps, true
}

// projectGaps names the gap of every evaluated non-test project whose file
// evidence is incomplete, keyed by project, and the fix of each gap. A run
// with no helper report shares the roslyn row's reason as its fix, since the
// cause is the run and not the project.
func (e *Extractor) projectGaps() (gaps, fixes map[string]string) {
	reported := make(map[string]TypeGraphProject, len(e.lastTypeGraph.Projects))
	for _, p := range e.lastTypeGraph.Projects {
		reported[p.Path] = p
	}
	gaps, fixes = map[string]string{}, map[string]string{}
	add := func(project, gap, fix string) {
		gaps[project], fixes[gap] = gap, fix
	}
	for _, p := range e.last.Projects {
		if p.isTest() {
			continue
		}
		tp, inReport := reported[p.Path]
		switch {
		case p.Status == ProjectFailed && (!inReport || tp.Status == TypeGraphFailed):
			add(p.Path, p.Path+" failed MSBuild evaluation", p.Reason)
		case !e.lastReported:
			add(p.Path, p.Path+" has project edges only", e.lastTypeGraphCov.Reason)
		case !inReport:
			add(p.Path, p.Path+" not reported", fixUnreported)
		case tp.Status != TypeGraphComplete:
			add(p.Path, p.Path+" "+tp.Status, fixRestore)
		}
	}
	return gaps, fixes
}

// deepestProject returns the evaluated project whose directory most closely
// holds file. dirs are the projects' directories, in project order.
func deepestProject(projects []ProjectEvaluation, dirs []string, file string) (string, bool) {
	best := -1
	for i, dir := range dirs {
		if dir != "." && !strings.HasPrefix(file, dir+"/") {
			continue
		}
		if best < 0 || len(dir) > len(dirs[best]) || dirs[best] == "." {
			best = i
		}
	}
	if best < 0 {
		return "", false
	}
	return projects[best].Path, true
}
