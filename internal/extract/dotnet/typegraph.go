package dotnet

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/bmatcuk/doublestar/v4"

	"github.com/alexei-led/archfit/v3/internal/model/evidence"
	"github.com/alexei-led/archfit/v3/internal/model/graph"
	"github.com/alexei-led/archfit/v3/internal/relationship/coupling"
	"github.com/alexei-led/archfit/v3/internal/scope"
	"github.com/alexei-led/archfit/v3/internal/toolrun"
)

// toolRoslyn is the coverage name of the type graph row, as the measurement
// contract spells it.
const toolRoslyn = "roslyn"

const (
	helperEnv               = "ARCHFIT_DOTNET_ROSLYN"
	helperDLL               = "archfit-roslyn.dll"
	helperSchema            = 1
	helperStderrPrefix      = "archfit-roslyn: "
	defaultTypeGraphTimeout = 20 * time.Minute
	helperVersionTimeout    = time.Minute

	reasonHelperAbsent   = helperEnv + " is not set; build the helper with `make dotnet-roslyn` and set " + helperEnv + " to its directory"
	reasonHelperDisabled = "analyzers.roslyn.enabled is false; project-level graph only"
	reasonHelperTimedOut = "archfit-roslyn timed out; raise analyzers.roslyn.timeout or run with a warm NuGet cache"
	reasonFallbackAll    = "every project falls back to its ProjectReference edges"
	reasonFallbackNone   = "no project falls back to ProjectReference edges: MSBuild evaluation failed for every project"
)

// Type graph statuses per project (D5), as the helper reports them.
const (
	// TypeGraphComplete means the project loaded with no unresolved symbols.
	TypeGraphComplete = "complete"
	// TypeGraphPartial means the project loaded but some symbols did not
	// resolve: its file edges may miss dependencies.
	TypeGraphPartial = "partial"
	// TypeGraphFailed means the project did not load in the helper.
	TypeGraphFailed = "failed"
)

var statusRank = map[string]int{TypeGraphComplete: 0, TypeGraphPartial: 1, TypeGraphFailed: 2}

// hintRank orders strength hints weakest first; the strongest use wins per file
// pair in each channel. It is the Go extractor's goStrengthRank order, which the
// helper's Hint.Rank also follows: dto sits between contract and model.
var hintRank = map[string]int{
	string(coupling.StrengthContract):   1,
	graph.StrengthHintDTO:               2,
	string(coupling.StrengthModel):      3,
	string(coupling.StrengthFunctional): 4,
	string(coupling.StrengthIntrusive):  5,
}

// dataHintAllowed are the values of the data channel: only non-callable uses (D3).
var dataHintAllowed = map[string]bool{"": true, string(coupling.StrengthModel): true, graph.StrengthHintDTO: true}

func strongerHint(a, b string) string {
	if hintRank[b] > hintRank[a] {
		return b
	}
	return a
}

// TypeGraphOptions configure the Roslyn type graph step of Extract. The zero
// value runs it when ARCHFIT_DOTNET_ROSLYN is set, under the default timeout.
type TypeGraphOptions struct {
	// Disabled forces the project-only graph (analyzers.roslyn.enabled: false).
	Disabled bool
	// Timeout bounds the snapshot copy and the helper run; 0 selects the
	// default of 20 minutes (a large repository loads for over 10 minutes
	// offline).
	Timeout time.Duration
}

// TypeGraphProject is the helper result for one project, folded across its
// target frameworks (D7): loaded when any framework loaded (complete only when
// every entry is complete, else partial), failed only when every entry failed;
// the edges are the union of the loaded frameworks'.
type TypeGraphProject struct {
	// Path is the repo-relative, slash-separated .csproj path.
	Path string
	// TargetFrameworks are the frameworks the helper loaded; empty for a
	// project that did not load.
	TargetFrameworks []string
	// Status is TypeGraphComplete, TypeGraphPartial, or TypeGraphFailed.
	Status string
	// Reason names the cause and the fix; per framework ("tfm: reason") when
	// more than one framework reports one.
	Reason string
	// UnresolvedSymbols counts symbols the compilation could not bind, summed
	// across frameworks. Each may hide a dependency edge: missing candidates,
	// not lower precision.
	UnresolvedSymbols int
	// UnusedReferences is the smallest per-framework count of ProjectReferences
	// with no type use; nil unless every framework is complete.
	UnusedReferences *int
	// IVTDeclarations counts distinct InternalsVisibleTo declarations. A
	// declaration is not a usage edge.
	IVTDeclarations int
	// LinkedFiles are compiled files outside the project directory.
	LinkedFiles []string
}

// TypeGraph is the Roslyn result of one Extract call.
type TypeGraph struct {
	// Projects are the projects the helper reported, sorted by Path.
	Projects []TypeGraphProject
	// SkippedProjects are solution entries the helper skipped as not C#.
	SkippedProjects []string
	// Fallback are the evaluated projects the helper did not load (failed,
	// missing from its report, or the helper did not run). Their
	// ProjectReference edges stand in for file edges (D1); a test project or a
	// failed evaluation supplies none.
	Fallback []string
}

// UnresolvedSymbols sums the projects' unresolved symbols.
func (g TypeGraph) UnresolvedSymbols() int {
	n := 0
	for _, p := range g.Projects {
		n += p.UnresolvedSymbols
	}
	return n
}

// helperReport is the helper's stdout contract (tools/dotnet/archfit-roslyn/src/Output.cs).
type helperReport struct {
	SchemaVersion   int             `json:"schema_version"`
	SkippedProjects []string        `json:"skipped_projects"`
	Projects        []helperProject `json:"projects"`
}

type helperProject struct {
	Path              string       `json:"path"`
	TFM               string       `json:"tfm"`
	Status            string       `json:"status"`
	Reason            string       `json:"reason"`
	UnresolvedSymbols int          `json:"unresolved_symbols"`
	UnusedReferences  *int         `json:"unused_references"`
	IVTDeclarations   []helperIVT  `json:"ivt_declarations"`
	LinkedFiles       []string     `json:"linked_files"`
	Files             []helperFile `json:"files"`
}

type helperIVT struct {
	Assembly   string `json:"assembly"`
	DeclaredIn string `json:"declared_in"`
}

type helperFile struct {
	Path string `json:"path"`
	// Generated has no counterpart in graph.Facts; file classification
	// recognizes generated files downstream.
	Generated bool         `json:"generated"`
	Edges     []helperEdge `json:"edges"`
}

// helperEdge To is a root-relative file, or an assembly name when External.
type helperEdge struct {
	To       string `json:"to"`
	External bool   `json:"external"`
	Hint     string `json:"hint"`
	DataHint string `json:"data_hint"`
}

// typeGraphResult is one type graph step: the folded projects, the files of
// every loaded project, and the coverage row.
type typeGraphResult struct {
	graph TypeGraph
	files map[string][]helperFile // loaded project path -> files across frameworks
	cov   evidence.Coverage
	// unreported are evaluated non-test projects the helper left out of its
	// report: missing inventory, never silently complete.
	unreported []string
	// details are extra coverage detail clauses.
	details []string
}

// loaded reports whether the helper loaded the project (complete or partial),
// so its file edges replace its ProjectReference edges (D1).
func (r typeGraphResult) loaded(project string) bool {
	_, ok := r.files[project]
	return ok
}

func notRun(status, reason string) typeGraphResult {
	return typeGraphResult{cov: evidence.Coverage{Tool: toolRoslyn, Status: status, Reason: reason}}
}

// failedRun is a helper run that produced no report: every project falls back
// to the edges its evaluation supplied, which fallback states.
func failedRun(version, reason, fallback string) typeGraphResult {
	return typeGraphResult{cov: evidence.Coverage{
		Tool:                    toolRoslyn,
		Version:                 version,
		Status:                  evidence.StatusPartial,
		Reason:                  "archfit-roslyn failed: " + reason + "; " + fallback,
		UnresolvedInputsMissing: 1,
	}}
}

func timedOutRun(version, fallback string) typeGraphResult {
	return typeGraphResult{cov: evidence.Coverage{
		Tool:                    toolRoslyn,
		Version:                 version,
		Status:                  evidence.StatusTimedOut,
		Reason:                  reasonHelperTimedOut + "; " + fallback,
		UnresolvedInputsMissing: 1,
	}}
}

// fallbackClause states which projects a run without a report falls back for:
// a project whose MSBuild evaluation failed supplies no ProjectReference edges
// (D1), and a test project's are dropped either way, so neither is counted.
func fallbackClause(projects []ProjectEvaluation) string {
	fallback, failed := 0, 0
	for _, p := range projects {
		switch {
		case p.isTest():
		case p.Status == ProjectFailed:
			failed++
		default:
			fallback++
		}
	}
	switch {
	case failed == 0:
		return reasonFallbackAll
	case fallback == 0:
		return reasonFallbackNone
	}
	verb := "have"
	if failed == 1 {
		verb = "has"
	}
	return fmt.Sprintf("%d of %d projects fall back to their ProjectReference edges; %d failed MSBuild evaluation and %s none",
		fallback, fallback+failed, failed, verb)
}

func isTimeout(ctx context.Context, err error) bool {
	return errors.Is(err, context.DeadlineExceeded) || errors.Is(ctx.Err(), context.DeadlineExceeded)
}

// HelperDir returns the helper directory from ARCHFIT_DOTNET_ROSLYN. An unset
// variable is not an error (the type graph is absent). A set one is
// authoritative: an invalid value fails the run instead of silently dropping to
// the project-only graph, as ARCHFIT_TOOL_PYTHON does. root is the analysis
// root the helper must lie outside; doctor, which has no target, passes "".
func HelperDir(root string) (dir string, configured bool, err error) {
	dir, configured = os.LookupEnv(helperEnv)
	if !configured {
		return "", false, nil
	}
	if !filepath.IsAbs(dir) {
		return "", true, fmt.Errorf("%s must name the absolute helper directory outside the target", helperEnv)
	}
	if root != "" {
		inside, err := toolrun.ContainsPath(root, dir)
		if err != nil {
			return "", true, fmt.Errorf("%s: validate helper location: %w", helperEnv, err)
		}
		if inside {
			return "", true, fmt.Errorf("%s must not select a helper inside the target", helperEnv)
		}
	}
	if info, err := os.Stat(filepath.Join(dir, helperDLL)); err != nil || !info.Mode().IsRegular() {
		return "", true, fmt.Errorf("%s must name a directory holding %s; build it with `make dotnet-roslyn`", helperEnv, helperDLL)
	}
	return dir, true, nil
}

// runTypeGraph copies root to a snapshot, runs the helper on it, and folds the
// report. The snapshot is removed before it returns, on every path. Only an
// invalid helper setting, an unusable report, an exec failure, or cancellation
// is an error; a failed or timed-out helper run is a coverage gap with the
// project-only fallback.
func (e *Extractor) runTypeGraph(ctx context.Context, s scope.Scope, targets []string, projects []ProjectEvaluation) (res typeGraphResult, err error) {
	root := s.Root
	if e.TypeGraph.Disabled {
		return notRun(evidence.StatusDisabled, reasonHelperDisabled), nil
	}
	dir, configured, err := HelperDir(root)
	if err != nil {
		return typeGraphResult{}, err
	}
	if !configured {
		return notRun(evidence.StatusAbsent, reasonHelperAbsent), nil
	}
	timeout := e.TypeGraph.Timeout
	if timeout <= 0 {
		timeout = defaultTypeGraphTimeout
	}
	fallback := fallbackClause(projects)
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	snap, err := makeSnapshot(ctx, root, s.GitRoot)
	if err != nil {
		if isTimeout(ctx, err) {
			return timedOutRun("", fallback), nil
		}
		if ctx.Err() != nil {
			return typeGraphResult{}, err
		}
		return failedRun("", err.Error(), fallback), nil
	}
	defer func() {
		if rerr := os.RemoveAll(snap.dir); rerr != nil && err == nil {
			res.cov.Reason = joinReason(res.cov.Reason, "snapshot cleanup failed: "+rerr.Error())
		}
	}()
	// The helper reports paths relative to the root it is given; MSBuild spells
	// files by their real path (macOS /var is /private/var), so pass that.
	helperRoot := snap.root
	if resolved, rerr := filepath.EvalSymlinks(snap.root); rerr == nil {
		helperRoot = resolved
	}
	mapper := newSnapshotMapper(snap, s.GitRoot)
	dll := filepath.Join(dir, helperDLL)
	run := func(args []string, limit time.Duration) (toolrun.Output, error) {
		return e.runner.Run(ctx, toolrun.ToolCmd{
			Name:    toolDotnet,
			Args:    append([]string{dll}, args...),
			Env:     msbuildEnv,
			WorkDir: helperRoot, // a global.json in the target selects the SDK
			Timeout: limit,
		})
	}

	out, err := run([]string{"--version"}, min(helperVersionTimeout, timeout))
	switch {
	case err != nil && isTimeout(ctx, err):
		return timedOutRun("", fallback), nil
	case err != nil:
		return typeGraphResult{}, fmt.Errorf("run archfit-roslyn --version: %w", err)
	case out.ExitCode != 0:
		return failedRun("", helperFailure(out, mapper), fallback), nil
	}
	version := helperIdentity(out.Stdout)

	out, err = run(append([]string{helperRoot}, targets...), timeout)
	switch {
	case err != nil && isTimeout(ctx, err):
		return timedOutRun(version, fallback), nil
	case err != nil:
		return typeGraphResult{}, fmt.Errorf("run archfit-roslyn: %w", err)
	case out.ExitCode != 0:
		return failedRun(version, helperFailure(out, mapper), fallback), nil
	}
	report, err := parseReport(out.Stdout, mapper)
	if err != nil {
		return typeGraphResult{}, err
	}
	res = foldReport(report, projects)
	if len(snap.aboveRepo) > 0 {
		res.details = append(res.details, "build inputs above the copied tree are not in the snapshot: "+strings.Join(snap.aboveRepo, ", "))
	}
	res.cov = typeGraphCoverage(version, res)
	return res, nil
}

// helperIdentity is the producer identity line `--version` prints
// ("archfit-roslyn <v>; Microsoft.CodeAnalysis <v>; SDK <v>"). The SDK resolver
// can print other lines on stdout, so the line is found by its prefix.
func helperIdentity(stdout []byte) string {
	for line := range strings.SplitSeq(string(stdout), "\n") {
		if line = strings.TrimSpace(line); strings.HasPrefix(line, "archfit-roslyn ") {
			return line
		}
	}
	return ""
}

// helperFailure names why the helper exited non-zero, root-relative: the first
// error line it wrote (an unresolvable SDK or global.json pin is one such line).
// Workspace diagnostics before it are not the cause.
func helperFailure(out toolrun.Output, mapper snapshotMapper) string {
	for line := range strings.SplitSeq(string(out.Stderr), "\n") {
		if msg, ok := strings.CutPrefix(strings.TrimSpace(line), helperStderrPrefix); ok && !strings.HasPrefix(msg, "workspace failure:") {
			return mapper.text(msg)
		}
	}
	return fmt.Sprintf("exited %d", out.ExitCode)
}

// parseReport decodes the helper JSON, rejects a schema it does not know, and
// maps snapshot paths back to the root.
func parseReport(data []byte, mapper snapshotMapper) (helperReport, error) {
	data = bytes.TrimSpace(data)
	var head struct {
		SchemaVersion int `json:"schema_version"`
	}
	if err := json.Unmarshal(data, &head); err != nil {
		return helperReport{}, fmt.Errorf("parse archfit-roslyn output: %w", err)
	}
	if head.SchemaVersion != helperSchema {
		return helperReport{}, fmt.Errorf("archfit-roslyn schema_version %d is not supported (want %d); rebuild the helper with `make dotnet-roslyn`", head.SchemaVersion, helperSchema)
	}
	var r helperReport
	if err := json.Unmarshal(data, &r); err != nil {
		return helperReport{}, fmt.Errorf("parse archfit-roslyn output: %w", err)
	}
	for i := range r.Projects {
		p := &r.Projects[i]
		if _, ok := statusRank[p.Status]; !ok {
			return helperReport{}, fmt.Errorf("archfit-roslyn: project %s has unknown status %q", p.Path, p.Status)
		}
		p.Path, p.Reason = mapper.path(p.Path), mapper.text(p.Reason)
		for j := range p.LinkedFiles {
			p.LinkedFiles[j] = mapper.path(p.LinkedFiles[j])
		}
		for j := range p.Files {
			f := &p.Files[j]
			f.Path = mapper.path(f.Path)
			for k := range f.Edges {
				edge := &f.Edges[k]
				if _, ok := hintRank[edge.Hint]; !ok || !dataHintAllowed[edge.DataHint] {
					return helperReport{}, fmt.Errorf("archfit-roslyn: edge %s -> %s has unknown hints %q/%q", f.Path, edge.To, edge.Hint, edge.DataHint)
				}
				if !edge.External {
					edge.To = mapper.path(edge.To)
				}
			}
		}
	}
	return r, nil
}

// foldReport folds the per-(project, framework) entries into one result per
// project (D7 with D1): summed unresolved symbols and the union of files, IVT
// declarations and linked files. projects are the MSBuild evaluations: their
// frameworks name a framework the helper failed to load (its failed entry
// carries none), a test project's file edges are dropped as its fallback edges
// are (its files stay as nodes), and an evaluated non-test project missing
// from the report is recorded as unreported.
func foldReport(r helperReport, projects []ProjectEvaluation) typeGraphResult {
	byPath := map[string][]helperProject{}
	for _, p := range r.Projects {
		byPath[p.Path] = append(byPath[p.Path], p)
	}
	evaluated := make(map[string]ProjectEvaluation, len(projects))
	for _, p := range projects {
		evaluated[p.Path] = p
	}
	res := typeGraphResult{files: map[string][]helperFile{}}
	res.graph.SkippedProjects = r.SkippedProjects
	droppedTestEdges := 0
	for _, path := range sortedKeys(byPath) {
		eval := evaluated[path]
		tp, files := foldProject(path, byPath[path], eval.TargetFrameworks)
		res.graph.Projects = append(res.graph.Projects, tp)
		if tp.Status == TypeGraphFailed {
			continue
		}
		if eval.isTest() {
			var n int
			files, n = withoutEdges(files)
			droppedTestEdges += n
		}
		res.files[path] = files
	}
	for _, p := range projects {
		if _, reported := byPath[p.Path]; !reported && !p.isTest() {
			res.unreported = append(res.unreported, p.Path)
		}
	}
	if droppedTestEdges > 0 {
		res.details = append(res.details, plural(droppedTestEdges, "test-project edge dropped", "test-project edges dropped"))
	}
	return res
}

// withoutEdges returns files with their edges removed and the number of
// distinct file pairs removed.
func withoutEdges(files []helperFile) ([]helperFile, int) {
	out := make([]helperFile, len(files))
	pairs := map[[2]string]struct{}{}
	for i, f := range files {
		for _, e := range f.Edges {
			pairs[[2]string{f.Path, e.To}] = struct{}{}
		}
		f.Edges = nil
		out[i] = f
	}
	return out, len(pairs)
}

// foldProject decides one project's status. A project with any loaded
// framework (complete or partial: it had source documents) counts as loaded:
// complete when every entry is complete, else partial, and its file edges
// replace its ProjectReference edges. It is failed only when every entry
// failed, so the fallback applies only to a project the helper did not load.
func foldProject(path string, entries []helperProject, evaluated []string) (TypeGraphProject, []helperFile) {
	tp := TypeGraphProject{Path: path, Status: TypeGraphFailed}
	ivts := map[helperIVT]struct{}{}
	linked := map[string]struct{}{}
	var reasons []string
	var files []helperFile
	allComplete, loaded := true, false
	for _, p := range entries {
		if p.Status != TypeGraphFailed {
			loaded = true
			if p.TFM != "" {
				tp.TargetFrameworks = append(tp.TargetFrameworks, p.TFM)
			}
		}
		if p.Status != TypeGraphComplete || p.UnusedReferences == nil {
			allComplete = false
		} else if tp.UnusedReferences == nil || *p.UnusedReferences < *tp.UnusedReferences {
			n := *p.UnusedReferences
			tp.UnusedReferences = &n
		}
		tp.UnresolvedSymbols += p.UnresolvedSymbols
		for _, d := range p.IVTDeclarations {
			ivts[d] = struct{}{}
		}
		for _, f := range p.LinkedFiles {
			linked[f] = struct{}{}
		}
		files = append(files, p.Files...)
	}
	slices.Sort(tp.TargetFrameworks)
	switch {
	case !loaded: // every entry failed: the status stays failed
	case allComplete:
		tp.Status = TypeGraphComplete
	default:
		tp.Status = TypeGraphPartial
	}
	for _, p := range entries {
		if p.Reason == "" && p.Status != TypeGraphFailed {
			continue
		}
		switch {
		case len(entries) == 1:
			reasons = append(reasons, p.Reason)
		case p.Status == TypeGraphFailed:
			reasons = append(reasons, failedFrameworks(p.TFM, evaluated, tp.TargetFrameworks)+" failed: "+p.Reason)
		default:
			reasons = append(reasons, p.TFM+": "+p.Reason)
		}
	}
	if !allComplete {
		tp.UnusedReferences = nil
	}
	tp.Reason = strings.Join(slices.Compact(reasons), "; ")
	tp.IVTDeclarations = len(ivts)
	tp.LinkedFiles = sortedKeys(linked)
	return tp, files
}

// failedFrameworks names the framework of a failed entry: its own, else the
// evaluated frameworks the helper loaded no entry for.
func failedFrameworks(tfm string, evaluated, loaded []string) string {
	if tfm != "" {
		return tfm
	}
	var missing []string
	for _, f := range evaluated {
		if !slices.Contains(loaded, f) {
			missing = append(missing, f)
		}
	}
	if len(missing) == 0 {
		return "a target framework"
	}
	return strings.Join(missing, ", ")
}

// typeGraphCoverage builds the roslyn row: partial when any project is partial
// or failed, with counts and the first causes. Unused references, IVT
// declarations, linked files and skipped projects are detail text only.
func typeGraphCoverage(version string, res typeGraphResult) evidence.Coverage {
	files := map[string]struct{}{}
	for _, fs := range res.files {
		for _, f := range fs {
			files[f.Path] = struct{}{}
		}
	}
	cov := evidence.Coverage{
		Tool:            toolRoslyn,
		Version:         version,
		FilesSeen:       len(files),
		FilesApplicable: len(files),
		Unresolved:      res.graph.UnresolvedSymbols(),
		Status:          evidence.StatusOK,
	}
	var problems []string
	var partial, failed, unused, ivts, linked int
	for _, p := range res.graph.Projects {
		switch p.Status {
		case TypeGraphPartial:
			partial++
		case TypeGraphFailed:
			failed++
		}
		if p.Status != TypeGraphComplete {
			problem := p.Path + " " + p.Status
			if p.Reason != "" {
				problem += ": " + p.Reason
			}
			problems = append(problems, problem)
		}
		if p.UnusedReferences != nil {
			unused += *p.UnusedReferences
		}
		ivts += p.IVTDeclarations
		linked += len(p.LinkedFiles)
	}
	for _, path := range res.unreported {
		problems = append(problems, path+" not reported by archfit-roslyn")
	}
	var parts []string
	if len(problems) > 0 {
		cov.Status = evidence.StatusPartial
		// Unresolved symbols and unloaded or unreported projects are missing
		// inputs, never a precision-only gap: the run must not read as comparable.
		cov.UnresolvedInputsMissing = max(1, cov.Unresolved+failed+len(res.unreported))
		summary := fmt.Sprintf("%d of %d projects partial, %d failed, %s",
			partial, len(res.graph.Projects), failed, plural(cov.Unresolved, "unresolved symbol", "unresolved symbols"))
		if len(res.unreported) > 0 {
			summary += fmt.Sprintf(", %d not reported", len(res.unreported))
		}
		parts = append(parts, summary)
		if len(problems) > reasonListLimit {
			problems = append(problems[:reasonListLimit], fmt.Sprintf("and %d more", len(problems)-reasonListLimit))
		}
		parts = append(parts, problems...)
	}
	var detail []string
	for _, d := range []struct {
		n                int
		singular, plural string
	}{
		{unused, "unused ProjectReference", "unused ProjectReferences"},
		{ivts, "InternalsVisibleTo declaration", "InternalsVisibleTo declarations"},
		{linked, "linked file outside its project", "linked files outside their projects"},
		{len(res.graph.SkippedProjects), "non-C# project skipped", "non-C# projects skipped"},
	} {
		if d.n > 0 {
			detail = append(detail, plural(d.n, d.singular, d.plural))
		}
	}
	detail = append(detail, res.details...)
	if len(detail) > 0 {
		parts = append(parts, "detail: "+strings.Join(detail, ", "))
	}
	cov.Reason = strings.Join(parts, "; ")
	return cov
}

func plural(n int, singular, pluralForm string) string {
	if n == 1 {
		return "1 " + singular
	}
	return fmt.Sprintf("%d %s", n, pluralForm)
}

func joinReason(a, b string) string {
	if a == "" {
		return b
	}
	return a + "; " + b
}

// fileEdgeKey identifies one aggregated file edge.
type fileEdgeKey struct {
	from, to string
	external bool
}

// fileHints are the two hint channels of one file pair, aggregated
// independently: the strongest use wins in each (D3).
type fileHints struct{ hint, data string }

// appendFileGraph adds the loaded projects' file nodes and file -> file (or
// file -> assembly) edges to facts. A file compiled into several projects or
// frameworks is one node; its edges are the union, strongest hint per channel.
// exclude applies here, after analysis: an excluded file contributes no node
// and no edge, and no edge points at it.
func appendFileGraph(facts *graph.Facts, res typeGraphResult, exclusions, internal []string) {
	seen := map[string]struct{}{}
	node := func(n graph.Node) string {
		if _, ok := seen[n.ID()]; !ok {
			seen[n.ID()] = struct{}{}
			facts.Nodes = append(facts.Nodes, n)
		}
		return n.ID()
	}
	dropped := func(p string) bool {
		return escapesRoot(p) || excluded(exclusions, p)
	}
	agg := map[fileEdgeKey]fileHints{}
	for _, project := range sortedKeys(res.files) {
		for _, f := range res.files[project] {
			if dropped(f.Path) {
				continue
			}
			node(graph.Node{Kind: graph.NodeKindFile, Path: f.Path, Language: graph.LangDotNet})
			for _, edge := range f.Edges {
				if !edge.External && dropped(edge.To) {
					continue
				}
				k := fileEdgeKey{from: f.Path, to: edge.To, external: edge.External}
				h := agg[k]
				agg[k] = fileHints{hint: strongerHint(h.hint, edge.Hint), data: strongerHint(h.data, edge.DataHint)}
			}
		}
	}
	keys := make([]fileEdgeKey, 0, len(agg))
	for k := range agg {
		keys = append(keys, k)
	}
	slices.SortFunc(keys, func(a, b fileEdgeKey) int {
		return strings.Compare(a.from+"\x00"+a.to, b.from+"\x00"+b.to)
	})
	for _, k := range keys {
		to, kind := graph.Node{Kind: graph.NodeKindFile, Path: k.to, Language: graph.LangDotNet}, graph.EdgeKindImports
		if k.external {
			to.Kind = graph.NodeKindExternal
		} else {
			kind = importEdgeKind(internal, k.to)
		}
		facts.Edges = append(facts.Edges, graph.Edge{
			From:             graph.Node{Kind: graph.NodeKindFile, Path: k.from}.ID(),
			To:               node(to),
			Kind:             kind,
			Language:         graph.LangDotNet,
			Confidence:       "high",
			StrengthHint:     agg[k].hint,
			DataStrengthHint: agg[k].data,
		})
	}
}

// importEdgeKind is uses_internal for a target matching an internal glob, as
// for TypeScript and Python file edges.
func importEdgeKind(internal []string, to string) graph.EdgeKind {
	for _, pattern := range internal {
		if ok, _ := doublestar.Match(pattern, to); ok {
			return graph.EdgeKindUsesInternal
		}
	}
	return graph.EdgeKindImports
}

// escapesRoot reports a helper path that is not root-relative.
func escapesRoot(p string) bool {
	return p == "" || p == ".." || strings.HasPrefix(p, "../") || filepath.IsAbs(p)
}
