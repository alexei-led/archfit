package dotnet

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"slices"
	"strings"
	"sync/atomic"
	"time"

	"github.com/bmatcuk/doublestar/v4"
	"golang.org/x/sync/errgroup"

	evidenceports "github.com/alexei-led/archfit/v3/internal/evidence/ports"
	"github.com/alexei-led/archfit/v3/internal/model/evidence"
	"github.com/alexei-led/archfit/v3/internal/model/graph"
	"github.com/alexei-led/archfit/v3/internal/scope"
	"github.com/alexei-led/archfit/v3/internal/syntax"
	"github.com/alexei-led/archfit/v3/internal/toolrun"
)

// confidenceHigh is the confidence of every .NET edge: MSBuild and the
// compiler resolve them.
const confidenceHigh = "high"

// toolMSBuild is the coverage name of the project graph row, as the
// measurement contract spells it.
const toolMSBuild = "dotnet-msbuild"

// Per-project evaluation statuses.
const (
	// ProjectOK means every evaluation of the project succeeded.
	ProjectOK = "ok"
	// ProjectPartial means the project evaluated, but a per-framework evaluation
	// failed or a reference could not be resolved: some references may be missing.
	ProjectPartial = "partial"
	// ProjectFailed means the project could not be evaluated: it supplied no data.
	ProjectFailed = "failed"
)

const (
	evalParallelism = 4
	reasonListLimit = 5
	reasonTimedOut  = "evaluation timed out; raise the analyzer timeout or run with a warm NuGet cache"
)

// walkSkipDirs are never searched for projects: build output, VCS metadata, and
// package folders.
var walkSkipDirs = map[string]struct{}{"obj": {}, "bin": {}, ".git": {}, "node_modules": {}}

// ProjectEvaluation is the MSBuild evaluation result of one project, across its
// base evaluation and one evaluation per target framework. The type graph step
// reads it to decide, per source project, between file edges and ProjectReference
// edges (design decision D1).
type ProjectEvaluation struct {
	// Path is the repo-relative, slash-separated .csproj path.
	Path string
	// Status is ProjectOK, ProjectPartial, or ProjectFailed.
	Status string
	// Reason names the cause and the fix when Status is not ProjectOK.
	Reason string
	// IsTestProject is true when any evaluation reported IsTestProject=true.
	IsTestProject bool
	// TargetFrameworks are the frameworks evaluated: TargetFrameworks, else the
	// single TargetFramework.
	TargetFrameworks []string
	// References are the repo-relative .csproj paths of existing ProjectReference
	// targets inside the root, deduplicated across frameworks and sorted.
	References []string
	// Packages are the PackageReference ids, deduplicated and sorted.
	Packages []string
	// Missing are ProjectReference targets inside the root with no file on disk.
	Missing []string
	// OutsideRoot counts ProjectReference targets outside the analysed root. They
	// are dropped, not missing.
	OutsideRoot int

	definedIn map[string]string // reference -> file that declares it
}

// FallbackReferences returns the ProjectReference edges to emit when file edges
// are unavailable for this project: nil for a test project (its edges are
// dropped, as the Rust extractor drops dev-dependencies) and for a failed one.
func (p ProjectEvaluation) FallbackReferences() []string {
	if p.isTest() || p.Status == ProjectFailed {
		return nil
	}
	return p.References
}

// isTest reports a test project: IsTestProject=true, or a project path the
// .NET test-file rule classifies (a test/ or tests/ segment). The second guard
// matters offline, where the test SDK props under obj/ may not be imported and
// IsTestProject evaluates empty. A Foo.Tests/ directory is not a test segment.
// Fallback edges and file edges both use it, so the two paths agree.
func (p ProjectEvaluation) isTest() bool {
	return p.IsTestProject || syntax.IsTestFile(graph.LangDotNet, p.Path)
}

// Evaluation is the result of one Extract call.
type Evaluation struct {
	// Solution is the repo-relative solution file the projects came from; empty
	// when they came from configured projects or a directory walk.
	Solution string
	// Projects are the evaluated projects, sorted by Path.
	Projects []ProjectEvaluation
	// SkippedProjects counts solution entries that are not C# projects.
	SkippedProjects int
	// MissingProjects are solution entries or configured targets with no file on disk.
	MissingProjects []string
}

// Extractor is the .NET extractor: the project graph from `dotnet msbuild`
// evaluation and the type-level file graph from the Roslyn helper.
// It satisfies the evidenceports.Extractor interface structurally.
type Extractor struct {
	runner  toolrun.Runner
	cfg     evidenceports.ExtractConfig
	targets []string
	last    Evaluation

	// TypeGraph configures the Roslyn step; the zero value runs it when
	// ARCHFIT_DOTNET_ROSLYN is set.
	TypeGraph TypeGraphOptions

	lastTypeGraph    TypeGraph
	lastTypeGraphCov evidence.Coverage
	// lastReported is true when the helper produced a report; lastMembers maps
	// each file it reported to the projects that compile it; lastAttributable
	// is true when the last Extract finished with every incompleteness pinned
	// to evaluated projects. LastDependencyGaps reads them.
	lastReported     bool
	lastMembers      map[string][]string
	lastAttributable bool
}

// New returns an Extractor. targets are the configured solutions or projects,
// relative to the scan root; none selects the default (see Applicable).
func New(runner toolrun.Runner, cfg evidenceports.ExtractConfig, targets []string) *Extractor {
	return &Extractor{runner: runner, cfg: cfg, targets: targets, lastTypeGraphCov: typeGraphAbsent()}
}

// Name returns the language identifier for this extractor.
func (e *Extractor) Name() string {
	return graph.LangDotNet
}

// CoverageTool returns the name this extractor stamps on its Coverage row.
func (e *Extractor) CoverageTool() string {
	return toolMSBuild
}

// LastEvaluation returns the per-project evaluation from the most recent Extract
// call: the zero value before Extract, and after a call that found nothing to
// evaluate.
func (e *Extractor) LastEvaluation() Evaluation {
	return e.last
}

// LastTypeGraph returns the Roslyn result of the most recent Extract call: the
// zero value before Extract. When the helper did not run or did not report,
// Projects is empty and Fallback lists every evaluated project.
func (e *Extractor) LastTypeGraph() TypeGraph {
	return e.lastTypeGraph
}

// LastTypeGraphCoverage returns the roslyn coverage row of the most recent
// Extract call; absent before Extract and when no project was evaluated.
func (e *Extractor) LastTypeGraphCoverage() evidence.Coverage {
	return e.lastTypeGraphCov
}

// Applicable reports whether root holds a .NET project to analyse: a configured
// target that exists, else a solution at the root, else any .csproj outside
// obj/, bin/ and exclusions. Extract and the registry probe both call it so the
// two cannot disagree about what "present" means.
func Applicable(root string, exclusions, targets []string) bool {
	sel, _ := discover(root, exclusions, targets)
	return sel.solution != "" || len(sel.projects) > 0
}

// Projects returns the repo-relative .csproj paths Extract evaluates under
// root, sorted: the same selection Applicable answers from. Nil when none or
// when the selection cannot be read.
func Projects(root string, exclusions, targets []string) []string {
	sel, err := discover(root, exclusions, targets)
	if err != nil {
		return nil
	}
	return sel.projects
}

// Extract detects dotnet, selects projects (solution, configured targets, or a
// directory walk), evaluates each with `dotnet msbuild -getItem/-getProperty`,
// runs the Roslyn helper on a snapshot of the root, and returns project nodes,
// file nodes, and one scored edge set per source project (D1): file edges when
// the helper loaded the project, else its ProjectReference edges.
//
// Mode off, no project to evaluate, or dotnet missing in auto mode yield empty
// Facts and an absent Coverage, never an error. A project that does not evaluate
// makes the row partial with the reason; a timeout makes it "timed out". The
// roslyn row is reported by LastTypeGraphCoverage.
func (e *Extractor) Extract(ctx context.Context, s scope.Scope) (graph.Facts, evidence.Coverage, error) {
	e.last = Evaluation{}
	e.lastTypeGraph, e.lastTypeGraphCov = TypeGraph{}, typeGraphAbsent()
	e.lastReported, e.lastMembers, e.lastAttributable = false, nil, false
	if e.cfg.Mode == evidenceports.ModeOff {
		return graph.Facts{}, absentCoverage(""), nil
	}

	sel, derr := discover(s.Root, e.cfg.Exclusions, e.targets)
	if sel.solution == "" && len(sel.projects) == 0 && derr == nil {
		if e.cfg.Mode == evidenceports.ModeOn && len(e.targets) > 0 {
			return graph.Facts{}, evidence.Coverage{}, fmt.Errorf("extract/dotnet: configured target not found: %s", strings.Join(sel.missing, ", "))
		}
		return graph.Facts{}, absentCoverage(""), nil
	}
	// A project is selected, or its selection failed: an analyzers.roslyn opt-out
	// reads as disabled on every return from here, not only once the helper
	// would have run.
	if e.TypeGraph.Disabled {
		e.lastTypeGraphCov = notRun(evidence.StatusDisabled, reasonHelperDisabled).cov
	}

	if _, ok := e.runner.Detect(ctx, toolDotnet); !ok {
		if e.cfg.Mode == evidenceports.ModeOn {
			return graph.Facts{}, evidence.Coverage{}, errors.New("extract/dotnet: dotnet not found; install the .NET SDK from https://dotnet.microsoft.com/download")
		}
		return graph.Facts{}, absentCoverage(""), nil
	}
	version := e.toolVersion(ctx, s.Root)

	if derr != nil {
		reason := fmt.Sprintf("project selection failed: %v", derr)
		if e.cfg.Mode == evidenceports.ModeOn {
			return graph.Facts{}, evidence.Coverage{}, fmt.Errorf("extract/dotnet: %s", reason)
		}
		return graph.Facts{}, evidence.Coverage{Tool: toolMSBuild, Version: version, Status: evidence.StatusPartial, Reason: reason}, nil
	}

	projects, timedOut, err := e.evaluateAll(ctx, s.Root, sel.projects)
	if err != nil {
		return graph.Facts{}, evidence.Coverage{}, fmt.Errorf("extract/dotnet: %w", err)
	}
	e.last = Evaluation{Solution: sel.solution, Projects: projects, SkippedProjects: sel.skipped, MissingProjects: sel.missing}

	tg, err := e.runTypeGraph(ctx, s, helperTargets(sel, e.targets), projects)
	if err != nil {
		return graph.Facts{}, evidence.Coverage{}, fmt.Errorf("extract/dotnet: %w", err)
	}
	facts, fallback := buildFacts(projects, tg)
	appendFileGraph(&facts, tg, e.cfg.Exclusions, e.cfg.Internal)
	facts.Unresolved = tg.graph.UnresolvedSymbols()
	tg.graph.Fallback = fallback
	e.lastTypeGraph, e.lastTypeGraphCov = tg.graph, tg.cov
	e.lastReported, e.lastMembers = tg.reported, tg.members()
	e.lastAttributable = len(projects) > 0 && len(sel.missing) == 0
	return facts, coverageFor(e.last, version, timedOut), nil
}

// helperTargets is what the helper opens: the solution when the selection came
// from exactly one (the root solution or one configured), else the projects.
func helperTargets(sel selection, configured []string) []string {
	if sel.solution != "" && len(configured) <= 1 {
		return []string{sel.solution}
	}
	return sel.projects
}

func (e *Extractor) toolVersion(ctx context.Context, root string) string {
	out, err := e.runner.Run(ctx, toolrun.ToolCmd{
		Name:    toolDotnet,
		Args:    []string{"--version"},
		Env:     msbuildEnv,
		WorkDir: root,
		Timeout: 30 * time.Second,
	})
	if err != nil || out.ExitCode != 0 {
		return ""
	}
	return strings.TrimSpace(strings.SplitN(string(out.Stdout), "\n", 2)[0])
}

// evaluateAll evaluates every project with a bounded worker pool. Results keep
// the input order. timedOut reports that at least one evaluation hit its
// deadline; those projects are failed, not dropped.
func (e *Extractor) evaluateAll(ctx context.Context, root string, rels []string) ([]ProjectEvaluation, bool, error) {
	out := make([]ProjectEvaluation, len(rels))
	var timedOut atomic.Bool
	g, gctx := errgroup.WithContext(ctx)
	g.SetLimit(evalParallelism)
	for i, rel := range rels {
		g.Go(func() error {
			pe, err := evaluateProject(gctx, e.runner, root, rel)
			if errors.Is(err, errTimedOut) {
				timedOut.Store(true)
				pe, err = ProjectEvaluation{Path: rel, Status: ProjectFailed, Reason: reasonTimedOut}, nil
			}
			out[i] = pe
			return err
		})
	}
	if err := g.Wait(); err != nil {
		return nil, false, err
	}
	return out, timedOut.Load(), nil
}

// evaluateProject runs the base evaluation, then one evaluation per target
// framework: a conditional ProjectReference shows up only in the per-framework
// run. The base evaluation failing fails the project; a framework evaluation
// failing leaves it partial.
func evaluateProject(ctx context.Context, runner toolrun.Runner, root, rel string) (ProjectEvaluation, error) {
	abs := filepath.Join(root, filepath.FromSlash(rel))
	pe := ProjectEvaluation{Path: rel, Status: ProjectOK}

	base, err := evaluate(ctx, runner, root, abs, "")
	if err != nil {
		return pe, err
	}
	if base.failure != "" {
		pe.Status, pe.Reason = ProjectFailed, base.failure
		return pe, nil
	}
	acc := newAccumulator(root)
	acc.add(base)

	tfms := splitTFMs(base.props[propTargetFWs])
	switch {
	case len(tfms) > 0:
		pe.TargetFrameworks = tfms
	case base.props[propTargetFW] != "":
		pe.TargetFrameworks = []string{base.props[propTargetFW]}
	}
	for _, tfm := range tfms {
		res, err := evaluate(ctx, runner, root, abs, tfm)
		if err != nil {
			return pe, err
		}
		if res.failure != "" {
			if pe.Status == ProjectOK {
				pe.Status, pe.Reason = ProjectPartial, fmt.Sprintf("target framework %s: %s", tfm, res.failure)
			}
			continue
		}
		acc.add(res)
	}
	acc.finish(&pe)
	return pe, nil
}

// accumulator merges evaluations of one project: the union of references and
// packages, classified against the root.
type accumulator struct {
	root     string
	isTest   bool
	refs     map[string]string // reference -> file that declares it
	missing  map[string]struct{}
	outside  map[string]struct{}
	packages map[string]struct{}
}

func newAccumulator(root string) *accumulator {
	return &accumulator{
		root:     root,
		refs:     map[string]string{},
		missing:  map[string]struct{}{},
		outside:  map[string]struct{}{},
		packages: map[string]struct{}{},
	}
}

func (a *accumulator) add(r evalResult) {
	if strings.EqualFold(r.props[propIsTest], "true") {
		a.isTest = true
	}
	for _, it := range r.refs {
		full := filepath.Clean(it.FullPath)
		rel, inside := relToRoot(a.root, full)
		switch {
		case !inside || rel == "":
			a.outside[filepath.ToSlash(full)] = struct{}{}
		case !fileExists(filepath.Join(a.root, filepath.FromSlash(rel))):
			a.missing[rel] = struct{}{}
		default:
			if _, seen := a.refs[rel]; !seen {
				a.refs[rel] = a.definingFile(it.DefiningProjectFullPath)
			}
		}
	}
	for _, it := range r.pkgs {
		a.packages[it.Identity] = struct{}{}
	}
}

// definingFile spells the file that declared a reference (the project or an
// imported props file) relative to the root; empty when it lies outside.
func (a *accumulator) definingFile(abs string) string {
	rel, _ := relToRoot(a.root, filepath.Clean(abs))
	return rel
}

func (a *accumulator) finish(pe *ProjectEvaluation) {
	pe.IsTestProject = a.isTest
	pe.References = sortedKeys(a.refs)
	pe.Packages = sortedKeys(a.packages)
	pe.Missing = sortedKeys(a.missing)
	pe.OutsideRoot = len(a.outside)
	pe.definedIn = a.refs
	if len(pe.Missing) > 0 && pe.Status == ProjectOK {
		pe.Status = ProjectPartial
		pe.Reason = "ProjectReference target not found: " + strings.Join(pe.Missing, ", ")
	}
}

func sortedKeys[V any](m map[string]V) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	slices.Sort(keys)
	return keys
}

func fileExists(p string) bool {
	info, err := os.Stat(p)
	return err == nil && !info.IsDir()
}

// buildFacts emits one project node per evaluated project, one per existing
// referenced project outside the evaluated set (so no edge dangles), and a
// depends_on edge per kept ProjectReference of every project the helper did not
// load. A loaded project's dependencies are its file edges instead, never both
// (D1). It returns the projects that fell back.
func buildFacts(projects []ProjectEvaluation, tg typeGraphResult) (graph.Facts, []string) {
	facts := graph.Facts{Language: graph.LangDotNet}
	seen := map[string]struct{}{}
	node := func(p string) string {
		n := graph.Node{Kind: graph.NodeKindProject, Path: p, Language: graph.LangDotNet}
		if _, ok := seen[n.ID()]; !ok {
			seen[n.ID()] = struct{}{}
			facts.Nodes = append(facts.Nodes, n)
		}
		return n.ID()
	}
	for _, p := range projects {
		node(p.Path)
	}
	var fallback []string
	for _, p := range projects {
		from := node(p.Path)
		if tg.loaded(p.Path) {
			continue
		}
		fallback = append(fallback, p.Path)
		for _, ref := range p.FallbackReferences() {
			edge := graph.Edge{
				From:       from,
				To:         node(ref),
				Kind:       graph.EdgeKindDependsOn,
				Language:   graph.LangDotNet,
				Confidence: confidenceHigh,
			}
			if file := p.definedIn[ref]; file != "" {
				edge.Locations = []graph.Location{{File: file}}
			}
			facts.Edges = append(facts.Edges, edge)
		}
	}
	return facts, fallback
}

// coverageFor builds the coverage row: partial when any project did not fully
// evaluate or a listed project is missing, "timed out" when a deadline hit.
func coverageFor(ev Evaluation, version string, timedOut bool) evidence.Coverage {
	cov := evidence.Coverage{
		Tool:            toolMSBuild,
		Version:         version,
		FilesSeen:       len(ev.Projects),
		FilesApplicable: len(ev.Projects),
		Status:          evidence.StatusOK,
	}
	var problems []string
	for _, p := range ev.Projects {
		cov.Unresolved += len(p.Missing)
		if p.Status != ProjectOK {
			problems = append(problems, p.Path+": "+p.Reason)
		}
	}
	if len(ev.MissingProjects) > 0 {
		problems = append(problems, "not found on disk: "+strings.Join(ev.MissingProjects, ", "))
	}
	if len(problems) == 0 {
		return cov
	}
	cov.Status = evidence.StatusPartial
	if timedOut {
		cov.Status = evidence.StatusTimedOut
	}
	if len(problems) > reasonListLimit {
		problems = append(problems[:reasonListLimit], fmt.Sprintf("and %d more", len(problems)-reasonListLimit))
	}
	cov.Reason = strings.Join(problems, "; ")
	return cov
}

// absentCoverage returns a Coverage record indicating no .NET project or no dotnet.
func absentCoverage(version string) evidence.Coverage {
	return evidence.Coverage{Tool: toolMSBuild, Version: version, Status: evidence.StatusAbsent}
}

// typeGraphAbsent is the roslyn row when the helper never ran.
func typeGraphAbsent() evidence.Coverage {
	return evidence.Coverage{Tool: toolRoslyn, Status: evidence.StatusAbsent}
}

// selection is the project set chosen for evaluation.
type selection struct {
	solution string   // repo-relative solution path, "" when none
	projects []string // repo-relative .csproj paths, sorted
	missing  []string // listed or configured paths with no file on disk
	skipped  int      // non-C# solution entries
}

// discover selects projects: configured targets if any, else the first
// .slnx/.sln at the root, else every .csproj outside obj/, bin/ and exclusions.
// On a solution that cannot be read or parsed it returns the selection so far
// with the solution set, and the error.
func discover(root string, exclusions, targets []string) (selection, error) {
	var sel selection
	var err error
	switch {
	case len(targets) > 0:
		err = sel.addTargets(root, targets)
	default:
		if sol := rootSolution(root); sol != "" {
			err = sel.addSolution(root, sol)
		} else {
			sel.projects, err = walkProjects(root, exclusions)
		}
	}
	sel.projects = dedupSorted(sel.projects)
	slices.Sort(sel.missing)
	return sel, err
}

func (s *selection) addTargets(root string, targets []string) error {
	for _, t := range targets {
		rel := path.Clean(filepath.ToSlash(t))
		if !fileExists(filepath.Join(root, filepath.FromSlash(rel))) {
			s.missing = append(s.missing, rel)
			continue
		}
		switch strings.ToLower(path.Ext(rel)) {
		case csprojExt:
			s.projects = append(s.projects, rel)
		case slnxExt, slnExt:
			if err := s.addSolution(root, rel); err != nil {
				return err
			}
		default:
			return fmt.Errorf("target %s is not a solution or .csproj", rel)
		}
	}
	return nil
}

// addSolution adds the projects a solution lists; entries that escape the root or
// have no file are recorded as missing.
func (s *selection) addSolution(root, rel string) error {
	if s.solution == "" {
		s.solution = rel
	}
	data, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(rel))) //nolint:gosec // solution file under the scan root
	if err != nil {
		return fmt.Errorf("read %s: %w", rel, err)
	}
	parsed, err := parseSolution(rel, data)
	if err != nil {
		return fmt.Errorf("%s: %w", rel, err)
	}
	s.skipped += parsed.Skipped
	dir := path.Dir(rel)
	for _, p := range parsed.Projects {
		full := path.Join(dir, p)
		if full == ".." || strings.HasPrefix(full, "../") || !fileExists(filepath.Join(root, filepath.FromSlash(full))) {
			s.missing = append(s.missing, full)
			continue
		}
		s.projects = append(s.projects, full)
	}
	return nil
}

// rootSolution returns the first .slnx or .sln file directly under root, by name.
func rootSolution(root string) string {
	entries, err := os.ReadDir(root)
	if err != nil {
		return ""
	}
	for _, e := range entries {
		ext := strings.ToLower(filepath.Ext(e.Name()))
		if !e.IsDir() && (ext == slnxExt || ext == slnExt) {
			return e.Name()
		}
	}
	return ""
}

// walkProjects returns every .csproj under root outside obj/, bin/, .git/,
// node_modules/ and exclusions.
func walkProjects(root string, exclusions []string) ([]string, error) {
	var out []string
	err := filepath.WalkDir(root, func(p string, d fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return nil // unreadable entries do not abort the walk
		}
		rel, err := filepath.Rel(root, p)
		if err != nil || rel == "." {
			return nil
		}
		rel = filepath.ToSlash(rel)
		if d.IsDir() {
			if _, skip := walkSkipDirs[d.Name()]; skip || excluded(exclusions, rel) {
				return filepath.SkipDir
			}
			return nil
		}
		if strings.EqualFold(path.Ext(rel), csprojExt) && !excluded(exclusions, rel) {
			out = append(out, rel)
		}
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("walk %s: %w", root, err)
	}
	return out, nil
}

func excluded(exclusions []string, rel string) bool {
	for _, pattern := range exclusions {
		if ok, _ := doublestar.Match(pattern, rel); ok {
			return true
		}
	}
	return false
}

func dedupSorted(in []string) []string {
	slices.Sort(in)
	return slices.Compact(in)
}
