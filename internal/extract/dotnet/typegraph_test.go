package dotnet_test

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	evidenceports "github.com/alexei-led/archfit/v3/internal/evidence/ports"
	"github.com/alexei-led/archfit/v3/internal/extract/dotnet"
	"github.com/alexei-led/archfit/v3/internal/model/evidence"
	"github.com/alexei-led/archfit/v3/internal/model/graph"
	"github.com/alexei-led/archfit/v3/internal/policy"
	"github.com/alexei-led/archfit/v3/internal/relationship/classify"
	"github.com/alexei-led/archfit/v3/internal/relationship/coupling"
	"github.com/alexei-led/archfit/v3/internal/scope"
	"github.com/alexei-led/archfit/v3/internal/toolrun"
)

const (
	helperEnv     = "ARCHFIT_DOTNET_ROSLYN"
	helperDLL     = "archfit-roslyn.dll"
	toolRoslyn    = "roslyn"
	helperVersion = "archfit-roslyn 1.0.0; Microsoft.CodeAnalysis 5.6.0; SDK 10.0.402"

	infraProj      = "Infrastructure/Infrastructure.csproj"
	testsProj      = "Tests/Tests.csproj"
	unresolvedProj = "Unresolved/Unresolved.csproj"

	repositoryCS = "Infrastructure/Repository.cs"
	contractsCS  = "Application/Contracts.cs"
	domainCS     = "Domain/Domain.cs"
	legacyCS     = "Legacy/Legacy.cs"
	mapperCS     = "Application/Generated/Mapper.g.cs"

	hContract   = "contract"
	hDTO        = "dto"
	hModel      = "model"
	hFunctional = "functional"
	hIntrusive  = "intrusive"
)

// realHelper is the helper directory from ARCHFIT_DOTNET_ROSLYN, read before
// the variable is cleared: every other test sets it, or leaves it unset, itself.
var realHelper string

func TestMain(m *testing.M) {
	realHelper = os.Getenv(helperEnv)
	_ = os.Unsetenv(helperEnv)
	os.Exit(m.Run())
}

// copyFixture copies the shared .NET fixture into a temp root.
func copyFixture(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	err := filepath.WalkDir(fixtureRoot, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, _ := filepath.Rel(fixtureRoot, p)
		target := filepath.Join(root, rel)
		if d.IsDir() {
			return os.MkdirAll(target, 0o750)
		}
		data, err := os.ReadFile(p) //nolint:gosec // test fixture path
		if err != nil {
			return err
		}
		return os.WriteFile(target, data, 0o600) //nolint:gosec // target is under the test's temp root
	})
	if err != nil {
		t.Fatal(err)
	}
	return root
}

// fixtureReplies are the recorded MSBuild evaluations of every fixture project.
func fixtureReplies(t *testing.T, root string) map[string]reply {
	return map[string]reply{
		appProj:        ok(t, root, "application.json"),
		domainProj:     ok(t, root, "domain.json"),
		infraProj:      ok(t, root, "infrastructure.json"),
		legacyProj:     ok(t, root, "legacy_base.json"),
		legacyNet10:    ok(t, root, "legacy_net10.json"),
		legacyNet8:     ok(t, root, "legacy_net8.json"),
		brokenProj:     {stderr: fixture(t, root, "msb4236.stderr"), exit: 1},
		testsProj:      ok(t, root, "tests.json"),
		unresolvedProj: ok(t, root, "unresolved.json"),
	}
}

// installHelper points ARCHFIT_DOTNET_ROSLYN at a directory holding a stand-in
// helper assembly; the fake runner answers its calls.
func installHelper(t *testing.T) {
	t.Helper()
	dir := t.TempDir()
	writeFile(t, dir, helperDLL, "")
	t.Setenv(helperEnv, dir)
}

// runFunc answers the helper's analysis run; snapshot is the root it was given.
type runFunc func(ctx context.Context, snapshot string) (toolrun.Output, error)

// fakeHelper answers `--version` with helperVersion and the analysis run with run.
func fakeHelper(run runFunc) func(context.Context, toolrun.ToolCmd) (toolrun.Output, error) {
	return func(ctx context.Context, cmd toolrun.ToolCmd) (toolrun.Output, error) {
		if slices.Equal(cmd.Args[1:], []string{argVersion}) {
			return toolrun.Output{Stdout: []byte(helperVersion + "\n")}, nil
		}
		return run(ctx, cmd.Args[1])
	}
}

func replying(stdout string) runFunc {
	return func(context.Context, string) (toolrun.Output, error) {
		return toolrun.Output{Stdout: []byte(stdout)}, nil
	}
}

func recordedReport(t *testing.T) string {
	t.Helper()
	return fixture(t, "", "roslyn_fixture.json")
}

// keyPath is the helper JSON key of a project or file path.
const keyPath = "path"

// editReport rewrites the recorded report's project entries.
func editReport(t *testing.T, edit func([]map[string]any) []map[string]any) string {
	t.Helper()
	var doc map[string]any
	if err := json.Unmarshal([]byte(recordedReport(t)), &doc); err != nil {
		t.Fatal(err)
	}
	raw := doc["projects"].([]any)
	projects := make([]map[string]any, 0, len(raw))
	for _, p := range raw {
		projects = append(projects, p.(map[string]any))
	}
	doc["projects"] = edit(projects)
	out, err := json.Marshal(doc)
	if err != nil {
		t.Fatal(err)
	}
	return string(out)
}

// legacyLoadFailure is the workspace failure of a Legacy framework that did not load.
const legacyLoadFailure = "Msbuild failed when processing the file 'Legacy/Legacy.csproj'"

// failedLegacy is a helper entry for a Legacy framework that did not load.
func failedLegacy() map[string]any {
	return map[string]any{
		keyPath: legacyProj, "tfm": "", "status": "failed", "reason": legacyLoadFailure, "unresolved_symbols": 0,
		"used_projects": []any{}, "ivt_declarations": []any{}, "linked_files": []any{}, "files": []any{},
	}
}

// fixtureRun extracts a fixture copy with the fake helper answering run.
func fixtureRun(t *testing.T, cfg evidenceports.ExtractConfig, run runFunc) (*dotnet.Extractor, *fakeDotnet, graph.Facts, evidence.Coverage) {
	t.Helper()
	root := copyFixture(t)
	installHelper(t)
	fake := newFake(t, root, fixtureReplies(t, root))
	fake.helper = fakeHelper(run)
	e := dotnet.New(fake, cfg, nil)
	facts, cov, err := e.Extract(context.Background(), scope.Scope{Root: root})
	if err != nil {
		t.Fatalf("Extract: %v", err)
	}
	return e, fake, facts, cov
}

type hints struct{ hint, data string }

// fileEdges are the file-level edges keyed "from -> to".
func fileEdges(facts graph.Facts) map[string]hints {
	out := map[string]hints{}
	for _, e := range facts.Edges {
		if strings.HasPrefix(e.From, "file:") {
			out[graph.NodePath(e.From)+" -> "+e.To] = hints{e.StrengthHint, e.DataStrengthHint}
		}
	}
	return out
}

func projectEdges(facts graph.Facts) []string {
	var out []string
	for _, e := range facts.Edges {
		if e.Kind == graph.EdgeKindDependsOn {
			out = append(out, graph.NodePath(e.From)+" -> "+graph.NodePath(e.To))
		}
	}
	slices.Sort(out)
	return out
}

func typeProject(t *testing.T, tg dotnet.TypeGraph, path string) dotnet.TypeGraphProject {
	t.Helper()
	for _, p := range tg.Projects {
		if p.Path == path {
			return p
		}
	}
	t.Fatalf("no project %s in %+v", path, tg.Projects)
	return dotnet.TypeGraphProject{}
}

var fixtureFallbackEdges = []string{
	"Application/Application.csproj -> Domain/Domain.csproj",
	"Infrastructure/Infrastructure.csproj -> Application/Application.csproj",
	"Infrastructure/Infrastructure.csproj -> Domain/Domain.csproj",
	"Legacy/Legacy.csproj -> Domain/Domain.csproj",
}

func TestTypeGraph_RecordedFixtureEdges(t *testing.T) {
	_, _, facts, _ := fixtureRun(t, autoCfg(), replying(recordedReport(t)))

	want := map[string]hints{
		repositoryCS + " -> file:" + contractsCS: {hDTO, hDTO},
		repositoryCS + " -> file:" + domainCS:    {hIntrusive, hModel},
		contractsCS + " -> file:" + domainCS:     {hModel, hModel},
		legacyCS + " -> file:" + domainCS:        {hFunctional, hModel},
		mapperCS + " -> file:" + contractsCS:     {hFunctional, hDTO},
	}
	if got := fileEdges(facts); !mapsEqual(got, want) {
		t.Errorf("file edges = %v, want %v", got, want)
	}
	if got := projectEdges(facts); len(got) != 0 {
		t.Errorf("project edges = %v, want none: every evaluated project the helper loaded scores through its files", got)
	}
	for _, n := range []string{"file:Shared/Marker.cs", "file:Tests/ThingTests.cs", "project:" + testsProj, "project:" + brokenProj} {
		if !slices.Contains(nodeIDs(facts), n) {
			t.Errorf("missing node %s", n)
		}
	}
	if facts.Unresolved != 35 {
		t.Errorf("facts unresolved = %d, want 35", facts.Unresolved)
	}
}

func TestTypeGraph_FileEdgeLocatesTheFirstUse(t *testing.T) {
	_, _, facts, _ := fixtureRun(t, autoCfg(), replying(recordedReport(t)))

	want := map[string]graph.Location{
		repositoryCS + " -> file:" + contractsCS: {File: repositoryCS, Line: 6},
		contractsCS + " -> file:" + domainCS:     {File: contractsCS, Line: 5},
		legacyCS + " -> file:" + domainCS:        {File: legacyCS, Line: 6},
	}
	for _, e := range facts.Edges {
		key := graph.NodePath(e.From) + " -> " + e.To
		if loc, ok := want[key]; ok {
			if !slices.Equal(e.Locations, []graph.Location{loc}) {
				t.Errorf("%s locations = %+v, want %+v", key, e.Locations, loc)
			}
			delete(want, key)
		}
	}
	if len(want) > 0 {
		t.Errorf("edges not found: %v", want)
	}
}

func TestTypeGraph_RecordedFixtureCoverage(t *testing.T) {
	e, _, _, _ := fixtureRun(t, autoCfg(), replying(recordedReport(t)))
	cov := e.LastTypeGraphCoverage()
	if cov.Tool != toolRoslyn || cov.Version != helperVersion || cov.Status != evidence.StatusPartial {
		t.Errorf("roslyn coverage = %+v", cov)
	}
	if cov.Unresolved != 35 || cov.UnresolvedInputsMissing != 36 || cov.FilesSeen != 9 {
		t.Errorf("roslyn counts = %+v, want 35 unresolved, 36 missing inputs (35 symbols + 1 failed project), 9 files", cov)
	}
	for _, part := range []string{
		"3 of 7 projects partial, 1 failed, 35 unresolved symbols",
		"Broken/Broken.csproj failed: MSB4236: SDK 'Archfit.Fixture.Missing.Sdk'",
		"Legacy/Legacy.csproj partial: net8.0: 31 unresolved symbols",
		"Unresolved/Unresolved.csproj partial: 3 unresolved symbols",
		"detail: 1 InternalsVisibleTo declaration, 1 linked file outside its project",
	} {
		if !strings.Contains(cov.Reason, part) {
			t.Errorf("roslyn reason %q lacks %q", cov.Reason, part)
		}
	}
}

// The dotnet-msbuild row carries the per-project table too: status and the
// frameworks MSBuild evaluated, so the evaluation result is readable without a
// second run.
func TestCoverage_EvaluationCarriesThePerProjectTable(t *testing.T) {
	root := copyFixture(t)
	installHelper(t)
	fake := newFake(t, root, fixtureReplies(t, root))
	fake.helper = fakeHelper(replying(recordedReport(t)))
	e := dotnet.New(fake, autoCfg(), nil)
	_, msbuild, err := e.Extract(context.Background(), scope.Scope{Root: root})
	if err != nil {
		t.Fatal(err)
	}
	details := msbuild.Details
	if len(details) != 7 {
		t.Fatalf("dotnet-msbuild details = %v, want one line per project", details)
	}
	for _, want := range []string{
		"Application/Application.csproj ok frameworks=net10.0",
		"Legacy/Legacy.csproj ok frameworks=net10.0,net8.0",
	} {
		if !slices.Contains(details, want) && !strings.HasPrefix(details[1], want) {
			t.Errorf("dotnet-msbuild details = %v, want %q", details, want)
		}
	}
	broken := details[slices.IndexFunc(details, func(d string) bool { return strings.HasPrefix(d, "Broken/") })]
	if !strings.Contains(broken, "failed frameworks=") || !strings.Contains(broken, "reason: ") {
		t.Errorf("broken detail = %q, want failed with its reason", broken)
	}
}

// The roslyn row carries the per-project table behind its shortened reason, so
// a consumer reads each project's status, frameworks, files and unresolved count
// without running the helper by hand.
func TestTypeGraph_CoverageCarriesThePerProjectTable(t *testing.T) {
	e, _, _, _ := fixtureRun(t, autoCfg(), replying(recordedReport(t)))
	want := []string{
		"Application/Application.csproj complete frameworks=net10.0 files=4 unresolved=0",
		"Broken/Broken.csproj failed frameworks= files=0 unresolved=0 reason: MSB4236: SDK 'Archfit.Fixture.Missing.Sdk' not resolvable; run with NuGet access and a warm cache, or install the SDK",
		"Domain/Domain.csproj complete frameworks=net10.0 files=1 unresolved=0",
		"Infrastructure/Infrastructure.csproj complete frameworks=net10.0 files=1 unresolved=0",
		"Legacy/Legacy.csproj partial frameworks=net10.0,net8.0 files=1 unresolved=31 reason: net8.0: 31 unresolved symbols (CS0246 x18, CS0400 x1, CS0518 x12); reference pack Microsoft.NETCore.App.Ref 8.0 is not installed: install the .NET 8.0 targeting pack or restore with NuGet access",
		"Tests/Tests.csproj partial frameworks=net10.0 files=1 unresolved=1 reason: MSB4236: SDK 'Archfit.Fixture.Missing.Sdk' not resolvable; 1 unresolved symbol (CS0234 x1); run with NuGet access and a warm cache, or install the SDK; restore the project with NuGet access (`dotnet restore`) or install the missing references",
		"Unresolved/Unresolved.csproj partial frameworks=net10.0 files=1 unresolved=3 reason: 3 unresolved symbols (CS0246 x3); restore the project with NuGet access (`dotnet restore`) or install the missing references",
	}
	if got := e.LastTypeGraphCoverage().Details; !slices.Equal(got, want) {
		t.Errorf("roslyn details = %#v, want %#v", got, want)
	}
}

func TestTypeGraph_RecordedFixtureProjects(t *testing.T) {
	e, _, _, _ := fixtureRun(t, autoCfg(), replying(recordedReport(t)))
	tg := e.LastTypeGraph()
	legacy := typeProject(t, tg, legacyProj)
	if legacy.Status != dotnet.TypeGraphPartial || !slices.Equal(legacy.TargetFrameworks, []string{targetNet10, targetNet8}) || legacy.UnresolvedSymbols != 31 || legacy.UnusedReferences != nil {
		t.Errorf("legacy = %+v, want partial: one framework is partial", legacy)
	}
	if d := typeProject(t, tg, domainProj); d.Status != dotnet.TypeGraphComplete || d.UnusedReferences == nil || *d.UnusedReferences != 0 || d.IVTDeclarations != 1 {
		t.Errorf("domain = %+v", d)
	}
	if a := typeProject(t, tg, appProj); !slices.Equal(a.LinkedFiles, []string{"Shared/Marker.cs"}) {
		t.Errorf("application = %+v", a)
	}
	if !slices.Equal(tg.Fallback, []string{brokenProj}) {
		t.Errorf("fallback = %v, want only Broken", tg.Fallback)
	}
}

// The helper omits the framework-less evaluation of a multi-targeted project.
// A helper that still reports it as failed names no framework, so when every
// framework the project evaluates loaded, the entry measured nothing: it must
// not hold the project. When an evaluated framework has no loaded entry the
// failure may name it and stays.
func TestTypeGraph_FrameworklessFailureDoesNotHoldTheLoadedFrameworks(t *testing.T) {
	withOuterFailure := func(dropNet8 bool) string {
		return editReport(t, func(ps []map[string]any) []map[string]any {
			out := make([]map[string]any, 0, len(ps)+1)
			for _, p := range ps {
				if p[keyPath] == legacyProj {
					if dropNet8 && p["tfm"] == "net8.0" {
						continue
					}
					p["status"], p["reason"], p["unresolved_symbols"] = "complete", "", 0
				}
				out = append(out, p)
			}
			return append(out, map[string]any{
				keyPath: legacyProj, "tfm": "", "status": "failed", "unresolved_symbols": 0,
				"reason": "Project does not contain 'Compile' target.; fix the project so `dotnet build` loads it",
				"files":  []any{},
			})
		})
	}

	e, _, _, _ := fixtureRun(t, autoCfg(), replying(withOuterFailure(false)))
	legacy := typeProject(t, e.LastTypeGraph(), legacyProj)
	if legacy.Status != dotnet.TypeGraphComplete || legacy.Reason != "" {
		t.Errorf("legacy = %+v, want complete: the framework-less evaluation measured nothing", legacy)
	}
	if !slices.Equal(legacy.TargetFrameworks, []string{targetNet10, targetNet8}) {
		t.Errorf("legacy frameworks = %v", legacy.TargetFrameworks)
	}

	e, _, _, _ = fixtureRun(t, autoCfg(), replying(withOuterFailure(true)))
	legacy = typeProject(t, e.LastTypeGraph(), legacyProj)
	if legacy.Status != dotnet.TypeGraphPartial || !strings.Contains(legacy.Reason, targetNet8) {
		t.Errorf("legacy = %+v, want partial naming %s: its framework did not load", legacy, targetNet8)
	}
}

func TestTypeGraph_HelperInvocation(t *testing.T) {
	_, fake, _, _ := fixtureRun(t, autoCfg(), replying(recordedReport(t)))
	if len(fake.helperCalls) != 2 {
		t.Fatalf("helper calls = %d, want --version and one run", len(fake.helperCalls))
	}
	run := fake.helperCalls[1]
	if run.Name != toolDotnet || run.WorkDir != run.Args[1] || !slices.Equal(run.Args[2:], []string{solutionFix}) || run.Timeout <= 0 {
		t.Errorf("helper run = %+v, want dotnet <dll> <snapshot> Fixture.slnx in the snapshot", run)
	}
}

// Every dotnet call, not only the helper run, must keep the SDK's first run
// from writing telemetry, logo, dev-certificate, or tool-path state.
func TestTypeGraph_EveryDotnetCallIsQuietAndStateless(t *testing.T) {
	_, fake, _, _ := fixtureRun(t, autoCfg(), replying(recordedReport(t)))
	want := []string{
		"DOTNET_CLI_TELEMETRY_OPTOUT=1", "DOTNET_NOLOGO=1", "DOTNET_SKIP_FIRST_TIME_EXPERIENCE=1",
		"MSBUILDDISABLENODEREUSE=1", "DOTNET_GENERATE_ASPNET_CERTIFICATE=false", "DOTNET_ADD_GLOBAL_TOOLS_TO_PATH=false",
	}
	kinds := map[string]bool{}
	for _, call := range fake.all {
		kind := call.Args[0]
		if strings.HasSuffix(kind, helperDLL) {
			kind = "helper " + call.Args[1]
		}
		if kind != argVersion && kind != argMSBuild && kind != "helper "+argVersion {
			kind = "helper run"
		}
		kinds[kind] = true
		for _, env := range want {
			if !slices.Contains(call.Env, env) {
				t.Errorf("%s call env %v lacks %s", kind, call.Env, env)
			}
		}
	}
	for _, kind := range []string{argVersion, argMSBuild, "helper " + argVersion, "helper run"} {
		if !kinds[kind] {
			t.Errorf("no %s call seen in %v", kind, kinds)
		}
	}
}

func TestTypeGraph_TestProjectEmitsNoEdges(t *testing.T) {
	useDomain := []any{map[string]any{"to": domainCS, "hint": hFunctional, "data_hint": hModel}}
	report := editReport(t, func(ps []map[string]any) []map[string]any {
		for _, p := range ps {
			switch p[keyPath] {
			case testsProj:
				p["files"] = []any{map[string]any{keyPath: "Tests/ThingTests.cs", "edges": useDomain}}
			case unresolvedProj:
				p["files"] = []any{map[string]any{keyPath: "Unresolved/Use.cs", "edges": useDomain}}
			}
		}
		return ps
	})
	e, _, facts, _ := fixtureRun(t, autoCfg(), replying(report))

	for _, edge := range facts.Edges {
		if strings.HasPrefix(graph.NodePath(edge.From), "Tests/") {
			t.Errorf("edge from the test project: %+v", edge)
		}
	}
	if _, ok := fileEdges(facts)["Unresolved/Use.cs -> file:"+domainCS]; !ok {
		t.Error("the same use from a production project is missing")
	}
	if !slices.Contains(nodeIDs(facts), "file:Tests/ThingTests.cs") {
		t.Error("the test file is no longer a node")
	}
	if cov := e.LastTypeGraphCoverage(); !strings.Contains(cov.Reason, "1 test-project edge dropped") {
		t.Errorf("roslyn reason %q does not count the dropped edge", cov.Reason)
	}
}

func TestTypeGraph_UnreportedProjectIsMissingInventory(t *testing.T) {
	report := editReport(t, func(ps []map[string]any) []map[string]any {
		return slices.DeleteFunc(ps, func(p map[string]any) bool { return p[keyPath] == domainProj || p[keyPath] == testsProj })
	})
	e, _, _, _ := fixtureRun(t, autoCfg(), replying(report))

	cov := e.LastTypeGraphCoverage()
	if cov.Status != evidence.StatusPartial || cov.UnresolvedInputsMissing != 34+1+1 { // symbols (Tests dropped), Broken failed, Domain unreported
		t.Errorf("roslyn coverage = %+v, want partial with the unreported project counted as missing input", cov)
	}
	if !strings.Contains(cov.Reason, domainProj+" not reported by archfit-roslyn") || !strings.Contains(cov.Reason, ", 1 not reported") {
		t.Errorf("roslyn reason %q does not name the unreported project", cov.Reason)
	}
	if strings.Contains(cov.Reason, testsProj+" not reported") {
		t.Errorf("roslyn reason %q names the unreported test project", cov.Reason)
	}
	if !slices.Contains(e.LastTypeGraph().Fallback, domainProj) {
		t.Errorf("fallback = %v, want the unreported project", e.LastTypeGraph().Fallback)
	}
}

func TestTypeGraph_FoldsFrameworksAndProjects(t *testing.T) {
	const report = `{"schema_version":2,"skipped_projects":["Db/Db.sqlproj"],"projects":[
	 {"path":"A/A.csproj","tfm":"net10.0","status":"complete","used_projects":[],"files":[
	  {"path":"A/X.cs","edges":[
	   {"to":"B/Y.cs","hint":"model","data_hint":"model","line":9},
	   {"to":"System.Runtime","external":true,"hint":"model","data_hint":"model"}]}]},
	 {"path":"A/A.csproj","tfm":"net8.0","status":"partial","reason":"2 unresolved symbols","unresolved_symbols":2,"files":[
	  {"path":"A/X.cs","edges":[{"to":"B/Y.cs","hint":"functional","data_hint":"dto","line":4}]}]},
	 {"path":"C/C.csproj","tfm":"net10.0","status":"complete","used_projects":[],"files":[
	  {"path":"A/X.cs","edges":[{"to":"B/Y.cs","hint":"contract","data_hint":""},{"to":"B/Hidden.cs","hint":"intrusive","data_hint":""}]}]}]}`
	root := tree(t, projA)
	installHelper(t)
	fake := newFake(t, root, map[string]reply{projA: ok(t, root, "domain.json")})
	fake.helper = fakeHelper(replying(report))
	cfg := autoCfg()
	cfg.Internal = []string{"B/Hidden.cs"}
	e := dotnet.New(fake, cfg, nil)
	facts, _, err := e.Extract(context.Background(), scope.Scope{Root: root})
	if err != nil {
		t.Fatal(err)
	}

	want := map[string]hints{
		"A/X.cs -> file:B/Y.cs":             {hFunctional, hModel},
		"A/X.cs -> file:B/Hidden.cs":        {hIntrusive, ""},
		"A/X.cs -> external:System.Runtime": {hModel, hModel},
	}
	if got := fileEdges(facts); !mapsEqual(got, want) {
		t.Errorf("file edges = %v, want %v (strongest per channel, independently)", got, want)
	}
	kinds := map[string]graph.EdgeKind{}
	for _, edge := range facts.Edges {
		kinds[edge.To] = edge.Kind
		if edge.To == "file:B/Y.cs" && !slices.Equal(edge.Locations, []graph.Location{{File: "A/X.cs", Line: 4}}) {
			t.Errorf("A/X.cs -> B/Y.cs locations = %+v, want the earliest first use across frameworks", edge.Locations)
		}
	}
	if kinds["file:B/Hidden.cs"] != graph.EdgeKindUsesInternal || kinds["file:B/Y.cs"] != graph.EdgeKindImports {
		t.Errorf("edge kinds = %v", kinds)
	}
	if !slices.Contains(nodeIDs(facts), "external:System.Runtime") {
		t.Errorf("nodes = %v, want the assembly as an external node", nodeIDs(facts))
	}

	tg := e.LastTypeGraph()
	a := typeProject(t, tg, projA)
	if a.Status != dotnet.TypeGraphPartial || a.Reason != "net8.0: 2 unresolved symbols" || a.UnusedReferences != nil {
		t.Errorf("A = %+v", a)
	}
	if !slices.Equal(tg.SkippedProjects, []string{"Db/Db.sqlproj"}) {
		t.Errorf("skipped = %v", tg.SkippedProjects)
	}
	if cov := e.LastTypeGraphCoverage(); !strings.Contains(cov.Reason, "1 non-C# project skipped") || cov.UnresolvedInputsMissing != 2 {
		t.Errorf("coverage = %+v", cov)
	}
	if got := fake.helperCalls[1].Args[2:]; !slices.Equal(got, []string{projA}) {
		t.Errorf("helper targets = %v, want the walked projects", got)
	}
}

func TestTypeGraph_RejectsUnusableReports(t *testing.T) {
	tests := []struct {
		name, report, want string
	}{
		{"unsupported schema version", `{"schema_version":1,"projects":[]}`, "schema_version 1 is not supported"},
		{"missing schema version", `{"projects":[]}`, "schema_version 0 is not supported"},
		{"not JSON", `boom`, "parse archfit-roslyn output"},
		{"unknown status", `{"schema_version":2,"projects":[{"path":"A/A.csproj","status":"loaded"}]}`, `unknown status "loaded"`},
		{"unknown hint", `{"schema_version":2,"projects":[{"path":"A/A.csproj","status":"complete","files":[{"path":"A/X.cs","edges":[{"to":"B/Y.cs","hint":"strong"}]}]}]}`, "unknown hints"},
		{"callable data hint", `{"schema_version":2,"projects":[{"path":"A/A.csproj","status":"complete","files":[{"path":"A/X.cs","edges":[{"to":"B/Y.cs","hint":"functional","data_hint":"functional"}]}]}]}`, "unknown hints"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			root := tree(t, projA)
			installHelper(t)
			fake := newFake(t, root, map[string]reply{projA: ok(t, root, "domain.json")})
			fake.helper = fakeHelper(replying(tt.report))
			_, _, err := dotnet.New(fake, autoCfg(), nil).Extract(context.Background(), scope.Scope{Root: root})
			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Errorf("err = %v, want %q", err, tt.want)
			}
		})
	}
}

func TestTypeGraph_FallbackWhenHelperFailsButEvaluationSucceeds(t *testing.T) {
	report := editReport(t, func(ps []map[string]any) []map[string]any {
		out := slices.DeleteFunc(ps, func(p map[string]any) bool { return p[keyPath] == legacyProj })
		return append(out, failedLegacy())
	})
	e, _, facts, _ := fixtureRun(t, autoCfg(), replying(report))

	if got := projectEdges(facts); !slices.Equal(got, []string{"Legacy/Legacy.csproj -> Domain/Domain.csproj"}) {
		t.Errorf("project edges = %v, want Legacy's ProjectReference from the net10.0 evaluation", got)
	}
	for edge := range fileEdges(facts) {
		if strings.HasPrefix(edge, legacyCS) {
			t.Errorf("file edge %s beside the fallback edge", edge)
		}
	}
	for _, edge := range facts.Edges {
		if edge.Kind != graph.EdgeKindDependsOn {
			continue
		}
		if edge.StrengthHint != "" || edge.DataStrengthHint != "" {
			t.Errorf("fallback edge carries a hint: %+v", edge)
		}
		// Legacy.csproj line 7 holds the conditional ProjectReference.
		if want := []graph.Location{{File: legacyProj, Line: 7}}; !slices.Equal(edge.Locations, want) {
			t.Errorf("fallback edge locations = %+v, want %+v", edge.Locations, want)
		}
	}
	if got := e.LastTypeGraph().Fallback; !slices.Equal(got, []string{brokenProj, legacyProj}) {
		t.Errorf("fallback = %v", got)
	}
}

func TestTypeGraph_ProjectLoadedIfAnyFrameworkLoaded(t *testing.T) {
	tests := []struct {
		name       string
		entries    func(net10, net8 map[string]any) []map[string]any
		wantStatus string
		wantReason string
		wantFiles  bool
	}{
		{
			name: "complete and failed",
			entries: func(net10, _ map[string]any) []map[string]any {
				return []map[string]any{net10, failedLegacy()}
			},
			wantStatus: dotnet.TypeGraphPartial,
			wantReason: "net8.0 failed: " + legacyLoadFailure,
			wantFiles:  true,
		},
		{
			name: "failed and failed",
			entries: func(map[string]any, map[string]any) []map[string]any {
				return []map[string]any{failedLegacy(), failedLegacy()}
			},
			wantStatus: dotnet.TypeGraphFailed,
			wantReason: "net10.0, net8.0 failed: " + legacyLoadFailure,
		},
		{
			name:       "partial and complete",
			entries:    func(net10, net8 map[string]any) []map[string]any { return []map[string]any{net8, net10} },
			wantStatus: dotnet.TypeGraphPartial,
			wantReason: "net8.0: 31 unresolved symbols",
			wantFiles:  true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			report := editReport(t, func(ps []map[string]any) []map[string]any {
				var net10, net8 map[string]any
				for _, p := range ps {
					if p[keyPath] == legacyProj {
						if p["tfm"] == targetNet10 {
							net10 = p
						} else {
							net8 = p
						}
					}
				}
				out := slices.DeleteFunc(ps, func(p map[string]any) bool { return p[keyPath] == legacyProj })
				return append(out, tt.entries(net10, net8)...)
			})
			e, _, facts, _ := fixtureRun(t, autoCfg(), replying(report))

			legacy := typeProject(t, e.LastTypeGraph(), legacyProj)
			if legacy.Status != tt.wantStatus || !strings.HasPrefix(legacy.Reason, tt.wantReason) || strings.Count(legacy.Reason, legacyLoadFailure) > 1 {
				t.Errorf("legacy = %+v, want %s with reason %q", legacy, tt.wantStatus, tt.wantReason)
			}
			_, hasFileEdge := fileEdges(facts)[legacyCS+" -> file:"+domainCS]
			hasProjectEdge := slices.Contains(projectEdges(facts), "Legacy/Legacy.csproj -> Domain/Domain.csproj")
			if hasFileEdge != tt.wantFiles || hasProjectEdge == tt.wantFiles {
				t.Errorf("file edge = %v, project edge = %v; want exactly one, the file edge only when a framework loaded", hasFileEdge, hasProjectEdge)
			}
			if fellBack := slices.Contains(e.LastTypeGraph().Fallback, legacyProj); fellBack == tt.wantFiles {
				t.Errorf("fallback includes Legacy = %v", fellBack)
			}
			if cov := e.LastTypeGraphCoverage(); cov.Status != evidence.StatusPartial || cov.UnresolvedInputsMissing == 0 {
				t.Errorf("roslyn coverage = %+v, want partial and non-comparable", cov)
			}
		})
	}
}

func TestTypeGraph_HelperAndEvaluationBothFailed(t *testing.T) {
	e, _, facts, msbuildCov := fixtureRun(t, autoCfg(), replying(recordedReport(t)))

	for _, edge := range facts.Edges {
		if strings.HasPrefix(graph.NodePath(edge.From), "Broken/") {
			t.Errorf("edge from Broken: %+v", edge)
		}
	}
	if !slices.Contains(nodeIDs(facts), "project:"+brokenProj) {
		t.Error("Broken's project node is gone; it must stay for topology")
	}
	const sdk = "Archfit.Fixture.Missing.Sdk"
	if !strings.Contains(msbuildCov.Reason, brokenProj) || !strings.Contains(msbuildCov.Reason, sdk) || msbuildCov.Status != evidence.StatusPartial {
		t.Errorf("dotnet-msbuild coverage = %+v, want Broken reported missing", msbuildCov)
	}
	cov := e.LastTypeGraphCoverage()
	if cov.Status != evidence.StatusPartial || !strings.Contains(cov.Reason, brokenProj+" failed") || !strings.Contains(cov.Reason, sdk) {
		t.Errorf("roslyn coverage = %+v, want Broken reported failed", cov)
	}
	if b := projectByPath(t, e.LastEvaluation(), brokenProj); b.Status != dotnet.ProjectFailed || b.FallbackReferences() != nil {
		t.Errorf("broken evaluation = %+v", b)
	}
}

func TestTypeGraph_RunWithoutReportFallsBackForEveryProject(t *testing.T) {
	tests := []struct {
		name       string
		setup      func(t *testing.T, fake *fakeDotnet, e *dotnet.Extractor)
		wantStatus string
		wantReason string
	}{
		{
			name: "helper not installed",
			setup: func(t *testing.T, _ *fakeDotnet, _ *dotnet.Extractor) {
				t.Setenv(helperEnv, "")
				_ = os.Unsetenv(helperEnv)
			},
			wantStatus: evidence.StatusAbsent,
			wantReason: "ARCHFIT_DOTNET_ROSLYN is not set; build the helper with `make dotnet-roslyn`",
		},
		{
			name:       "type graph disabled",
			setup:      func(_ *testing.T, _ *fakeDotnet, e *dotnet.Extractor) { e.TypeGraph.Disabled = true },
			wantStatus: evidence.StatusDisabled,
			wantReason: "analyzers.roslyn.enabled is false",
		},
		{
			name: "unsatisfiable global.json",
			setup: func(t *testing.T, fake *fakeDotnet, _ *dotnet.Extractor) {
				fake.helper = func(_ context.Context, cmd toolrun.ToolCmd) (toolrun.Output, error) {
					return toolrun.Output{Stderr: []byte(fixture(t, cmd.WorkDir, "roslyn_globaljson.stderr")), ExitCode: 1}, nil
				}
			},
			wantStatus: evidence.StatusPartial,
			wantReason: "archfit-roslyn failed: global.json requests SDK 9.0.100, which is not installed; install a matching SDK or relax the pin; 5 of 6 projects fall back",
		},
		{
			name: "helper run exits non-zero",
			setup: func(_ *testing.T, fake *fakeDotnet, _ *dotnet.Extractor) {
				fake.helper = fakeHelper(func(_ context.Context, snap string) (toolrun.Output, error) {
					return toolrun.Output{Stderr: []byte("archfit-roslyn: workspace failure: x\narchfit-roslyn: " + snap + "/Fixture.slnx does not exist\n"), ExitCode: 1}, nil
				})
			},
			wantStatus: evidence.StatusPartial,
			wantReason: "archfit-roslyn failed: Fixture.slnx does not exist; 5 of 6 projects fall back",
		},
		{
			name: "helper hits an unexpected error",
			setup: func(_ *testing.T, fake *fakeDotnet, _ *dotnet.Extractor) {
				fake.helper = fakeHelper(func(context.Context, string) (toolrun.Output, error) {
					return toolrun.Output{Stderr: []byte("archfit-roslyn: System.Xml.XmlException: Unexpected end of file has occurred. Line 1, position 37.\n"), ExitCode: 2}, nil
				})
			},
			wantStatus: evidence.StatusPartial,
			wantReason: "archfit-roslyn failed: System.Xml.XmlException: Unexpected end of file has occurred. Line 1, position 37.; 5 of 6 projects fall back",
		},
		{
			name: "helper timed out",
			setup: func(_ *testing.T, fake *fakeDotnet, _ *dotnet.Extractor) {
				fake.helper = fakeHelper(func(context.Context, string) (toolrun.Output, error) {
					return toolrun.Output{}, context.DeadlineExceeded
				})
			},
			wantStatus: evidence.StatusTimedOut,
			wantReason: "archfit-roslyn timed out; raise analyzers.roslyn.timeout",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			root := copyFixture(t)
			installHelper(t)
			fake := newFake(t, root, fixtureReplies(t, root))
			fake.helper = fakeHelper(replying(recordedReport(t)))
			e := dotnet.New(fake, autoCfg(), nil)
			tt.setup(t, fake, e)
			facts, _, err := e.Extract(context.Background(), scope.Scope{Root: root})
			if err != nil {
				t.Fatal(err)
			}
			if got := projectEdges(facts); !slices.Equal(got, fixtureFallbackEdges) {
				t.Errorf("project edges = %v, want every evaluated reference except the test project's", got)
			}
			if got := fileEdges(facts); len(got) != 0 {
				t.Errorf("file edges = %v, want none", got)
			}
			cov := e.LastTypeGraphCoverage()
			if cov.Tool != toolRoslyn || cov.Status != tt.wantStatus || !strings.Contains(cov.Reason, tt.wantReason) {
				t.Errorf("roslyn coverage = %+v, want %s with %q", cov, tt.wantStatus, tt.wantReason)
			}
			if got := e.LastTypeGraph().Fallback; len(got) != 7 {
				t.Errorf("fallback = %v, want all 7 projects", got)
			}
		})
	}
}

// TestTypeGraph_FailedRunNamesOnlyRealFallback pins D17: a helper run that
// produced no report states the fallback the evaluation actually supplied. A
// project whose MSBuild evaluation failed has no ProjectReference edges to fall
// back to, so a run where every evaluation failed promises none.
func TestTypeGraph_FailedRunNamesOnlyRealFallback(t *testing.T) {
	globalJSONFailure := func(t *testing.T, root string) reply {
		return reply{stdout: fixture(t, root, "globaljson.stdout"), stderr: fixture(t, root, "globaljson.stderr"), exit: 155}
	}
	tests := []struct {
		name     string
		replies  func(t *testing.T, root string) map[string]reply
		want     string
		wantNone []string
	}{
		{
			name: "every evaluation failed",
			replies: func(t *testing.T, root string) map[string]reply {
				replies := map[string]reply{}
				for key := range fixtureReplies(t, root) {
					if !strings.Contains(key, "@") {
						replies[key] = globalJSONFailure(t, root)
					}
				}
				return replies
			},
			want:     "no project falls back to ProjectReference edges: MSBuild evaluation failed for every project",
			wantNone: []string{"every project falls back", "projects fall back"},
		},
		{
			name:     "one evaluation failed",
			replies:  fixtureReplies,
			want:     "5 of 6 projects fall back to their ProjectReference edges; 1 failed MSBuild evaluation and has none",
			wantNone: []string{"every project falls back"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			root := copyFixture(t)
			installHelper(t)
			fake := newFake(t, root, tt.replies(t, root))
			fake.helper = func(_ context.Context, cmd toolrun.ToolCmd) (toolrun.Output, error) {
				return toolrun.Output{Stderr: []byte(fixture(t, cmd.WorkDir, "roslyn_globaljson.stderr")), ExitCode: 1}, nil
			}
			e := dotnet.New(fake, autoCfg(), nil)
			if _, _, err := e.Extract(context.Background(), scope.Scope{Root: root}); err != nil {
				t.Fatal(err)
			}
			cov := e.LastTypeGraphCoverage()
			if !strings.Contains(cov.Reason, tt.want) {
				t.Errorf("roslyn reason %q lacks %q", cov.Reason, tt.want)
			}
			for _, bad := range tt.wantNone {
				if strings.Contains(cov.Reason, bad) {
					t.Errorf("roslyn reason %q claims %q", cov.Reason, bad)
				}
			}
		})
	}
}

func TestTypeGraph_InvalidHelperSettingFailsVisibly(t *testing.T) {
	root := tree(t, projA)
	writeFile(t, root, "tools/"+helperDLL, "")
	empty := t.TempDir()
	tests := []struct {
		name, value, want string
	}{
		{"empty", "", "must name the absolute helper directory"},
		{"relative", "dotnet-roslyn", "must name the absolute helper directory"},
		{"no helper assembly", empty, "must name a directory holding archfit-roslyn.dll"},
		{"missing directory", filepath.Join(empty, "gone"), "must name a directory holding archfit-roslyn.dll"},
		{"inside the target", filepath.Join(root, "tools"), "must not select a helper inside the target"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv(helperEnv, tt.value)
			fake := newFake(t, root, map[string]reply{projA: ok(t, root, "domain.json")})
			_, _, err := dotnet.New(fake, autoCfg(), nil).Extract(context.Background(), scope.Scope{Root: root})
			if err == nil || !strings.Contains(err.Error(), "ARCHFIT_DOTNET_ROSLYN") || !strings.Contains(err.Error(), tt.want) {
				t.Errorf("err = %v, want %q", err, tt.want)
			}
			if len(fake.helperCalls) != 0 {
				t.Errorf("helper ran %d times", len(fake.helperCalls))
			}
		})
	}
}

func TestTypeGraph_ExcludedFileStillBindsButEmitsNothing(t *testing.T) {
	cfg := autoCfg()
	cfg.Exclusions = []string{contractsCS}
	var present bool
	e, _, facts, _ := fixtureRun(t, cfg, func(_ context.Context, snap string) (toolrun.Output, error) {
		_, err := os.Stat(filepath.Join(snap, contractsCS))
		present = err == nil
		return toolrun.Output{Stdout: []byte(recordedReport(t))}, nil
	})

	if !present {
		t.Fatal("the excluded file was not in the snapshot; code that binds to it would not resolve")
	}
	if p := typeProject(t, e.LastTypeGraph(), infraProj); p.Status != dotnet.TypeGraphComplete || p.UnresolvedSymbols != 0 {
		t.Errorf("infrastructure = %+v, want its use of the excluded file resolved", p)
	}
	want := map[string]hints{
		repositoryCS + " -> file:" + domainCS: {hIntrusive, hModel},
		legacyCS + " -> file:" + domainCS:     {hFunctional, hModel},
	}
	if got := fileEdges(facts); !mapsEqual(got, want) {
		t.Errorf("file edges = %v, want none from or to %s", got, contractsCS)
	}
	if slices.Contains(nodeIDs(facts), "file:"+contractsCS) {
		t.Error("the excluded file is a node")
	}
}

// dataOnlyReport is a helper report where Application/Contracts.cs reads a
// field of a concrete Domain class and calls nothing.
const dataOnlyReport = `{"schema_version":2,"skipped_projects":[],"projects":[
 {"path":"Application/Application.csproj","tfm":"net10.0","status":"complete","used_projects":[],
  "files":[{"path":"Application/Contracts.cs","edges":[{"to":"Domain/Domain.cs","hint":"model","data_hint":"model","reference":"data_use"}]}]},
 {"path":"Domain/Domain.csproj","tfm":"net10.0","status":"complete","used_projects":[],
  "files":[{"path":"Domain/Domain.cs","edges":[]}]}]}`

func dataOnlyFacts(t *testing.T) graph.Facts {
	t.Helper()
	root := tree(t, appProj, domainProj)
	installHelper(t)
	fake := newFake(t, root, map[string]reply{appProj: ok(t, root, "application.json"), domainProj: ok(t, root, "domain.json")})
	fake.helper = fakeHelper(replying(dataOnlyReport))
	facts, _, err := dotnet.New(fake, autoCfg(), nil).Extract(context.Background(), scope.Scope{Root: root})
	if err != nil {
		t.Fatal(err)
	}
	return facts
}

var fixtureModules = map[string]policy.ModuleDef{
	"application":    {Paths: []string{"Application/**"}},
	"domain":         {Paths: []string{"Domain/**"}},
	"infrastructure": {Paths: []string{"Infrastructure/**"}},
	"legacy":         {Paths: []string{"Legacy/**"}},
}

func TestTypeGraph_DataOnlyConcreteUseClassifiesAsModel(t *testing.T) {
	g := graph.Build([]graph.Facts{dataOnlyFacts(t)})
	idx := classify.Run(g, classify.Config{Modules: fixtureModules})
	key := "file:" + contractsCS + "\x00file:" + domainCS + "\x00" + string(graph.EdgeKindImports)
	if got := idx[key].Strength; got != coupling.StrengthModel {
		t.Errorf("strength = %q, want model: no public selector, so the hint decides", got)
	}
}

func TestTypeGraph_OneDependencyScoredOnce(t *testing.T) {
	facts := dataOnlyFacts(t)
	if got := projectEdges(facts); len(got) != 0 {
		t.Errorf("project edges = %v, want none: Application's file edges replace its ProjectReference to Domain", got)
	}
	idx := classify.Run(graph.Build([]graph.Facts{facts}), classify.Config{Modules: fixtureModules})
	scored := 0
	for key, cl := range idx {
		from, rest, _ := strings.Cut(key, "\x00")
		to, _, _ := strings.Cut(rest, "\x00")
		if strings.HasPrefix(graph.NodePath(from), "Application/") && strings.HasPrefix(graph.NodePath(to), "Domain/") && cl.Distance != coupling.DistanceUnknown {
			scored++
		}
	}
	if scored != 1 {
		t.Errorf("Application -> Domain classified %d times, want once", scored)
	}
}

func TestTypeGraph_PublicSelectorMakesFileEdgeContract(t *testing.T) {
	_, _, facts, _ := fixtureRun(t, autoCfg(), replying(recordedReport(t)))
	modules := map[string]policy.ModuleDef{
		"application":    {Paths: []string{"Application/**"}, Public: []string{contractsCS}},
		"domain":         {Paths: []string{"Domain/**"}},
		"infrastructure": {Paths: []string{"Infrastructure/**"}},
	}
	idx := classify.Run(graph.Build([]graph.Facts{facts}), classify.Config{Modules: modules})

	key := "file:" + repositoryCS + "\x00file:" + contractsCS + "\x00" + string(graph.EdgeKindImports)
	if got := idx[key].Strength; got != coupling.StrengthContract {
		t.Errorf("strength = %q, want contract: a DTO across a declared public boundary is the contract", got)
	}
	for key, cl := range idx {
		if strings.HasPrefix(key, "project:") {
			t.Errorf("project edge classified beside the file edge: %q -> %s", key, cl.Strength)
		}
	}
}

// treeState lists every file under root with its mode, size, mtime and hash.
func treeState(t *testing.T, root string) map[string]string {
	t.Helper()
	out := map[string]string{}
	err := filepath.WalkDir(root, func(p string, _ fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		info, err := os.Lstat(p)
		if err != nil {
			return err
		}
		sum := ""
		if info.Mode().IsRegular() {
			data, err := os.ReadFile(p) //nolint:gosec // test tree
			if err != nil {
				return err
			}
			h := sha256.Sum256(data)
			sum = hex.EncodeToString(h[:])
		}
		rel, _ := filepath.Rel(root, p)
		out[rel] = fmt.Sprintf("%v %d %d %s", info.Mode(), info.Size(), info.ModTime().UnixNano(), sum)
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return out
}

func TestTypeGraph_SnapshotIsRemovedAndOriginalNeverWritten(t *testing.T) {
	tests := []struct {
		name    string
		timeout time.Duration
		answer  func(ctx context.Context) (toolrun.Output, error)
		panics  bool
	}{
		{name: "success", answer: func(context.Context) (toolrun.Output, error) { return toolrun.Output{}, nil }},
		{name: "helper error", answer: func(context.Context) (toolrun.Output, error) {
			return toolrun.Output{Stderr: []byte("archfit-roslyn: boom\n"), ExitCode: 1}, nil
		}},
		{name: "timeout", timeout: 300 * time.Millisecond, answer: func(ctx context.Context) (toolrun.Output, error) {
			<-ctx.Done()
			return toolrun.Output{}, ctx.Err()
		}},
		{name: "exec failure", answer: func(context.Context) (toolrun.Output, error) { return toolrun.Output{}, errors.New("fork failed") }},
		{name: "panic", panics: true, answer: func(context.Context) (toolrun.Output, error) { panic("helper crashed") }},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			root := copyFixture(t)
			writeFile(t, root, ".git/HEAD", "ref: refs/heads/main\n")
			writeFile(t, root, "web/node_modules/x/index.js", "")
			writeFile(t, root, "Domain/obj/project.assets.json", "{}")
			writeFile(t, root, "Domain/bin/Lib.dll", "")
			// UseArtifactsOutput moves restore and build output to artifacts/.
			writeFile(t, root, "artifacts/obj/Domain/project.assets.json", "{}")
			if err := os.Symlink(filepath.Join(root, "Domain"), filepath.Join(root, "DomainLink")); err != nil {
				t.Fatal(err)
			}
			before := treeState(t, root)
			installHelper(t)
			fake := newFake(t, root, fixtureReplies(t, root))
			var snapshot string
			fake.helper = fakeHelper(func(ctx context.Context, snap string) (toolrun.Output, error) {
				snapshot = snap
				for rel, want := range map[string]bool{
					".git": false, "web/node_modules": false, "web": true,
					"Domain/obj/project.assets.json": true, "Domain/bin/Lib.dll": true, solutionFix: true,
					"artifacts/obj/Domain/project.assets.json": true,
				} {
					if _, err := os.Stat(filepath.Join(snap, rel)); (err == nil) != want {
						t.Errorf("snapshot has %s = %v, want %v", rel, err == nil, want)
					}
				}
				if link, err := os.Readlink(filepath.Join(snap, "DomainLink")); err != nil || !strings.Contains(link, filepath.Base(snap)) {
					t.Errorf("snapshot link = %q (%v), want it inside the snapshot", link, err)
				}
				// The design-time build writes generated files under obj/.
				writeFile(t, snap, "Domain/obj/Debug/net10.0/Domain.GlobalUsings.g.cs", "// generated")
				writeFile(t, filepath.Join(snap, "DomainLink"), "obj/through-link.cs", "// via link")
				return tt.answer(ctx)
			})
			e := dotnet.New(fake, autoCfg(), nil)
			e.TypeGraph.Timeout = tt.timeout

			func() {
				defer func() {
					if r := recover(); (r != nil) != tt.panics {
						t.Errorf("recovered %v, want panic %v", r, tt.panics)
					}
				}()
				_, _, _ = e.Extract(context.Background(), scope.Scope{Root: root})
			}()

			if snapshot == "" {
				t.Fatal("helper never ran")
			}
			if _, err := os.Stat(snapshot); !errors.Is(err, fs.ErrNotExist) {
				t.Errorf("snapshot %s still exists (err %v)", snapshot, err)
			}
			if after := treeState(t, root); !mapsEqual(after, before) {
				t.Errorf("original root changed:\nbefore %v\nafter  %v", before, after)
			}
			if tt.name == "timeout" {
				if cov := e.LastTypeGraphCoverage(); cov.Status != evidence.StatusTimedOut {
					t.Errorf("coverage = %+v, want timed out", cov)
				}
			}
		})
	}
}

// TestTypeGraph_AncestorAboveRepoInputMakesProjectsPartial: MSBuild reads build
// inputs above the repository root, which the snapshot does not have. A project
// that would inherit one is partial and names it, and its files are dependency
// gaps, so a required rule stays unevaluated instead of judging a snapshot
// compiled without the inherited constant. A file in the repository tree that
// MSBuild would use instead shadows the ancestor one, so nothing changes.
func TestTypeGraph_AncestorAboveRepoInputMakesProjectsPartial(t *testing.T) {
	const props = `<Project><PropertyGroup><DefineConstants>$(DefineConstants);ARCHFIT_ANCESTOR</DefineConstants></PropertyGroup></Project>`
	for _, tt := range []struct {
		name        string
		repoProps   bool
		wantPartial bool
	}{
		{name: "inherited from above the repository root", wantPartial: true},
		{name: "shadowed by a repository-root file", repoProps: true},
	} {
		t.Run(tt.name, func(t *testing.T) {
			outer := t.TempDir()
			writeFile(t, outer, "Directory.Build.props", props)
			repo := filepath.Join(outer, "repo")
			root := filepath.Join(repo, "src")
			writeFile(t, root, domainProj, "")
			writeFile(t, root, "Domain/Domain.cs", "namespace Domain; public class Thing { }")
			if tt.repoProps {
				writeFile(t, repo, "Directory.Build.props", props)
			}
			before := treeState(t, outer)
			installHelper(t)
			fake := newFake(t, root, map[string]reply{domainProj: ok(t, root, "domain.json")})
			fake.helper = fakeHelper(func(context.Context, string) (toolrun.Output, error) {
				return toolrun.Output{Stdout: []byte(domainOnlyReport)}, nil
			})
			e := dotnet.New(fake, autoCfg(), nil)
			if _, _, err := e.Extract(context.Background(), scope.Scope{Root: root, GitRoot: repo, SubtreePrefix: subtreeDir}); err != nil {
				t.Fatal(err)
			}
			ancestor := filepath.Join(outer, "Directory.Build.props")
			p := typeProject(t, e.LastTypeGraph(), domainProj)
			if !tt.wantPartial {
				if p.Status != dotnet.TypeGraphComplete || strings.Contains(p.Reason, ancestor) {
					t.Errorf("project = %+v, want complete: the repository's own Directory.Build.props shadows the ancestor one", p)
				}
			} else {
				if p.Status != dotnet.TypeGraphPartial || !strings.Contains(p.Reason, ancestor) {
					t.Errorf("project = %+v, want partial naming %s", p, ancestor)
				}
				gaps, attributed := e.LastDependencyGaps([]string{"Domain/Domain.cs"})
				if !attributed || gaps.Files["Domain/Domain.cs"] == "" {
					t.Errorf("dependency gaps = %+v attributed %v, want Domain/Domain.cs incomplete", gaps, attributed)
				}
				if cov := e.LastTypeGraphCoverage(); cov.Status != evidence.StatusPartial {
					t.Errorf("roslyn coverage = %+v, want partial", cov)
				}
			}
			if after := treeState(t, outer); !mapsEqual(after, before) {
				t.Errorf("the original tree changed:\nbefore %v\nafter  %v", before, after)
			}
		})
	}
}

func mapsEqual[V comparable](a, b map[string]V) bool {
	if len(a) != len(b) {
		return false
	}
	for k, v := range a {
		if w, ok := b[k]; !ok || w != v {
			return false
		}
	}
	return true
}

// evalJSON is a synthetic MSBuild evaluation of project with IsTestProject
// unset (as offline, when the test SDK props under obj/ are not imported) and
// one ProjectReference to ref.
func evalJSON(root, project, ref string) string {
	return fmt.Sprintf(`{"Properties":{"TargetFramework":"net10.0","TargetFrameworks":"","IsTestProject":""},
 "Items":{"ProjectReference":[{"Identity":"x","FullPath":"%s/%s","DefiningProjectFullPath":"%s/%s"}],"PackageReference":[]}}`,
		filepath.ToSlash(root), ref, filepath.ToSlash(root), project)
}

func TestTypeGraph_TestClassifiedPathDropsEdgesWithoutIsTestProject(t *testing.T) {
	const (
		core      = "src/Core/Core.csproj"
		testDir   = "test/Foo.Test/Foo.Test.csproj"
		testsName = "src/Foo.Tests/Foo.Tests.csproj"
		app       = "src/App/App.csproj"
	)
	projects := []string{testDir, testsName, app}
	fileOf := func(project string) string { return filepath.ToSlash(filepath.Join(filepath.Dir(project), "Use.cs")) }
	entries := make([]string, 0, 1+len(projects))
	for _, p := range append([]string{core}, projects...) {
		edges := ""
		if p != core {
			edges = `{"to":"src/Core/Core.cs","hint":"functional","data_hint":""}`
		}
		entries = append(entries, fmt.Sprintf(`{"path":%q,"tfm":"net10.0","status":"complete","used_projects":[],"files":[{"path":%q,"edges":[%s]}]}`, p, fileOf(p), edges))
	}
	report := `{"schema_version":2,"skipped_projects":[],"projects":[` + strings.Join(entries, ",") + `]}`

	for _, helper := range []bool{false, true} {
		t.Run(fmt.Sprintf("helper loaded %v", helper), func(t *testing.T) {
			root := tree(t, core, testDir, testsName, app)
			replies := map[string]reply{core: {stdout: `{"Properties":{"TargetFramework":"net10.0"},"Items":{}}`}}
			for _, p := range projects {
				replies[p] = reply{stdout: evalJSON(root, p, core)}
			}
			fake := newFake(t, root, replies)
			if helper {
				installHelper(t)
				fake.helper = fakeHelper(replying(report))
			}
			facts, _, err := dotnet.New(fake, autoCfg(), nil).Extract(context.Background(), scope.Scope{Root: root})
			if err != nil {
				t.Fatal(err)
			}
			from := map[string]bool{}
			for _, e := range facts.Edges {
				from[graph.NodePath(e.From)] = true
			}
			kept := map[string]bool{testDir: false, testsName: true, app: true}
			for project, want := range kept {
				source := project
				if helper {
					source = fileOf(project)
				}
				if from[source] != want {
					t.Errorf("edges from %s = %v, want %v (edges %+v)", source, from[source], want, facts.Edges)
				}
			}
			if helper && !slices.Contains(nodeIDs(facts), "file:"+fileOf(testDir)) {
				t.Error("the test project's file is no longer a node")
			}
		})
	}
}

// A failed evaluation reports the error that failed it, not a warning printed
// before it: eShop's functional tests print the NU1902 vulnerable-package
// warning first and fail on ASPIRE009.
func TestFailedEvaluationNamesTheErrorNotAWarning(t *testing.T) {
	const app = "App/App.csproj"
	root := tree(t, app)
	fake := newFake(t, root, map[string]reply{app: {exit: 1, stderr: strings.Join([]string{
		app + " : warning NU1902: Package 'MessagePack' 2.5.302 has a known moderate severity vulnerability, https://github.com/advisories/GHSA-qhrr-8q5h-9q3h",
		"",
		app + " : error ASPIRE009: The Aspire CLI bundle could not be resolved.",
		"",
		"Build FAILED.",
	}, "\n")}})
	e := dotnet.New(fake, autoCfg(), nil)
	if _, _, err := e.Extract(context.Background(), scope.Scope{Root: root}); err != nil {
		t.Fatal(err)
	}
	projects := e.LastEvaluation().Projects
	if len(projects) != 1 {
		t.Fatalf("projects = %+v, want one", projects)
	}
	if reason := projects[0].Reason; !strings.Contains(reason, "ASPIRE009") || strings.Contains(reason, "NU1902") {
		t.Errorf("reason = %q, want the ASPIRE009 error, not the NU1902 warning", reason)
	}
}

func TestCoverage_NamesEveryProjectWithAProblem(t *testing.T) {
	const projects = 7
	paths := make([]string, 0, projects)
	entries := make([]string, 0, projects)
	for i := range projects {
		p := fmt.Sprintf("P%d/P%d.csproj", i, i)
		paths = append(paths, p)
		entries = append(entries, fmt.Sprintf(`{"path":%q,"tfm":"net10.0","status":"partial","reason":"1 unresolved symbol","unresolved_symbols":1,"files":[]}`, p))
	}
	root := tree(t, paths...)
	replies := map[string]reply{}
	for _, p := range paths {
		replies[p] = reply{stderr: p + " : error MSB4025: broken " + p, exit: 1}
	}
	installHelper(t)
	fake := newFake(t, root, replies)
	fake.helper = fakeHelper(replying(`{"schema_version":2,"projects":[` + strings.Join(entries, ",") + `]}`))
	e := dotnet.New(fake, autoCfg(), nil)
	_, msbuild, err := e.Extract(context.Background(), scope.Scope{Root: root})
	if err != nil {
		t.Fatal(err)
	}

	for _, row := range []evidence.Coverage{msbuild, e.LastTypeGraphCoverage()} {
		for _, p := range paths {
			if !strings.Contains(row.Reason, p) {
				t.Errorf("%s reason %q does not name %s", row.Tool, row.Reason, p)
			}
		}
		if strings.Contains(row.Reason, "more") {
			t.Errorf("%s reason %q is truncated", row.Tool, row.Reason)
		}
	}
}

// The helper's workspace adds transitive references to a project, so it
// reports the projects used; the unused count comes from the direct
// references MSBuild evaluated, without assembly-less ones.
func TestTypeGraph_UnusedReferencesAreDirectAssemblyReferencesOnly(t *testing.T) {
	const (
		app   = "App/App.csproj"
		lib   = "Lib/Lib.csproj"
		other = "Other/Other.csproj"
		core  = "Core/Core.csproj"
		res   = "Res/Res.csproj"
		web   = "Web/Web.csproj"
	)
	root := tree(t, app, lib, other, core, res, web)
	item := func(ref, meta string) string {
		return fmt.Sprintf(`{"Identity":"x","FullPath":"%s/%s","DefiningProjectFullPath":"%s/%s"%s}`, filepath.ToSlash(root), ref, filepath.ToSlash(root), app, meta)
	}
	appEval := `{"Properties":{"TargetFramework":"net10.0"},"Items":{"ProjectReference":[` + strings.Join([]string{
		item(lib, ""),
		item(other, ""),
		item(res, `,"ReferenceOutputAssembly":"false"`),
		item(web, `,"IsAspireProjectResource":"true"`),
	}, ",") + `],"PackageReference":[]}}`
	leaf := reply{stdout: `{"Properties":{"TargetFramework":"net10.0"},"Items":{}}`}
	replies := map[string]reply{app: {stdout: appEval}, lib: leaf, other: leaf, core: leaf, res: leaf, web: leaf}

	entry := func(path, tfm, status, used string) string {
		return fmt.Sprintf(`{"path":%q,"tfm":%q,"status":%q,"used_projects":[%s],"files":[]}`, path, tfm, status, used)
	}
	// Core is transitive (Lib references it) and used; Other is direct and unused.
	const usedByApp = `"Lib/Lib.csproj","Core/Core.csproj"`
	one := 1
	tests := []struct {
		name       string
		appEntries []string
		want       *int
	}{
		{name: "complete", appEntries: []string{entry(app, "net10.0", "complete", usedByApp)}, want: &one},
		{name: "a partial framework may hide uses", appEntries: []string{entry(app, "net10.0", "complete", usedByApp), entry(app, "net8.0", "partial", "")}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			installHelper(t)
			fake := newFake(t, root, replies)
			entries := append([]string{
				entry(lib, "net10.0", "complete", `"Core/Core.csproj"`), entry(other, "net10.0", "complete", ""),
				entry(core, "net10.0", "complete", ""), entry(res, "net10.0", "complete", ""), entry(web, "net10.0", "complete", ""),
			}, tt.appEntries...)
			fake.helper = fakeHelper(replying(`{"schema_version":2,"projects":[` + strings.Join(entries, ",") + `]}`))
			e := dotnet.New(fake, autoCfg(), nil)
			if _, _, err := e.Extract(context.Background(), scope.Scope{Root: root}); err != nil {
				t.Fatal(err)
			}
			got := typeProject(t, e.LastTypeGraph(), app).UnusedReferences
			if (got == nil) != (tt.want == nil) || (got != nil && *got != *tt.want) {
				t.Errorf("unused = %v, want %v", deref(got), deref(tt.want))
			}
		})
	}
}

func deref(p *int) any {
	if p == nil {
		return nil
	}
	return *p
}

// A snapshot that cannot be removed is named in the roslyn reason, which the
// JSON and Markdown coverage tables print even on an ok row: the copy left in
// the temp directory is never silent.
func TestTypeGraph_SnapshotCleanupFailureIsReported(t *testing.T) {
	var locked string
	t.Cleanup(func() {
		if locked != "" {
			_ = os.Chmod(locked, 0o700)                                        //nolint:gosec // test cleanup of its own directory
			_ = os.RemoveAll(filepath.Dir(filepath.Dir(filepath.Dir(locked)))) // the snapshot
		}
	})
	e, _, _, _ := fixtureRun(t, autoCfg(), func(_ context.Context, snap string) (toolrun.Output, error) {
		// A build step that leaves a read-only directory behind.
		writeFile(t, snap, "Domain/obj/locked/file.txt", "x")
		locked = filepath.Join(snap, "Domain", "obj", "locked")
		if err := os.Chmod(locked, 0o500); err != nil { //nolint:gosec // read-only on purpose
			t.Fatal(err)
		}
		return toolrun.Output{Stdout: []byte(recordedReport(t))}, nil
	})

	if cov := e.LastTypeGraphCoverage(); !strings.Contains(cov.Reason, "snapshot cleanup failed") || !strings.Contains(cov.Reason, "locked") {
		t.Errorf("roslyn reason %q does not report the snapshot left behind", cov.Reason)
	}
}
