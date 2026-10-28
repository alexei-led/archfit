package dotnet_test

import (
	"context"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"testing"

	evidenceports "github.com/alexei-led/archfit/v3/internal/evidence/ports"
	"github.com/alexei-led/archfit/v3/internal/extract/dotnet"
	"github.com/alexei-led/archfit/v3/internal/model/evidence"
	"github.com/alexei-led/archfit/v3/internal/scope"
	"github.com/alexei-led/archfit/v3/internal/toolrun"
)

// realRunner returns the real runner, or skips when the SDK is not installed.
// Evaluation gets a private CLI home and package folder so the test touches
// nothing of the developer's.
func realRunner(t *testing.T) toolrun.Runner {
	t.Helper()
	runner := toolrun.New()
	if _, ok := runner.Detect(context.Background(), toolDotnet); !ok {
		t.Skip("dotnet is not on PATH")
	}
	t.Setenv("DOTNET_CLI_HOME", t.TempDir())
	t.Setenv("NUGET_PACKAGES", t.TempDir())
	t.Setenv("DOTNET_NOLOGO", "1")
	t.Setenv("DOTNET_CLI_TELEMETRY_OPTOUT", "1")
	return runner
}

var fixtureRoot = filepath.Join("..", "..", "testdata", toolDotnet)

func TestIntegration_LegacyFindsNet10OnlyReference(t *testing.T) {
	root, err := filepath.Abs(fixtureRoot)
	if err != nil {
		t.Fatal(err)
	}
	e := dotnet.New(realRunner(t), evidenceports.ExtractConfig{Mode: evidenceports.ModeOn}, []string{legacyProj})
	facts, cov, err := e.Extract(context.Background(), scope.Scope{Root: root})
	if err != nil {
		t.Fatalf("Extract: %v", err)
	}
	legacy := projectByPath(t, e.LastEvaluation(), legacyProj)
	if !slices.Equal(legacy.TargetFrameworks, []string{targetNet10, targetNet8}) {
		t.Errorf("TFMs = %v", legacy.TargetFrameworks)
	}
	if !slices.Equal(legacy.References, []string{domainProj}) {
		t.Errorf("references = %v, want the net10.0-only Domain reference", legacy.References)
	}
	if !hasEdge(facts, nodeLegacy, nodeDomain) || cov.Status != evidence.StatusOK {
		t.Errorf("edges = %+v, coverage = %+v", facts.Edges, cov)
	}
}

func TestIntegration_FixtureSolution(t *testing.T) {
	if testing.Short() {
		t.Skip("evaluating the unresolvable SDK takes seconds offline")
	}
	root, err := filepath.Abs(fixtureRoot)
	if err != nil {
		t.Fatal(err)
	}
	e := dotnet.New(realRunner(t), evidenceports.ExtractConfig{Mode: evidenceports.ModeOn}, nil)
	_, cov, err := e.Extract(context.Background(), scope.Scope{Root: root})
	if err != nil {
		t.Fatalf("Extract: %v", err)
	}
	ev := e.LastEvaluation()
	if ev.Solution != solutionFix || len(ev.Projects) != 7 {
		t.Fatalf("evaluation = %+v", ev)
	}
	broken := projectByPath(t, ev, "Broken/Broken.csproj")
	wantReason := "MSB4236: SDK 'Archfit.Fixture.Missing.Sdk' not resolvable offline; run with NuGet access and a warm cache"
	if broken.Status != dotnet.ProjectFailed || broken.Reason != wantReason {
		t.Errorf("broken = %+v", broken)
	}
	tests := projectByPath(t, ev, "Tests/Tests.csproj")
	if !tests.IsTestProject || tests.FallbackReferences() != nil {
		t.Errorf("tests = %+v", tests)
	}
	infra := projectByPath(t, ev, "Infrastructure/Infrastructure.csproj")
	if !slices.Equal(infra.References, []string{appProj, domainProj}) {
		t.Errorf("infrastructure references = %v", infra.References)
	}
	if cov.Status != evidence.StatusPartial || !strings.Contains(cov.Reason, "Archfit.Fixture.Missing.Sdk") {
		t.Errorf("coverage = %+v", cov)
	}
}

// helperForIntegration returns the built helper directory: ARCHFIT_DOTNET_ROSLYN
// when set, else the `make dotnet-roslyn` output. It skips when neither exists.
func helperForIntegration(t *testing.T) string {
	t.Helper()
	dir := realHelper
	if dir == "" {
		abs, err := filepath.Abs(filepath.Join("..", "..", "..", ".bin", "dotnet-roslyn"))
		if err != nil {
			t.Fatal(err)
		}
		dir = abs
	}
	if _, err := os.Stat(filepath.Join(dir, helperDLL)); err != nil {
		t.Skipf("no built helper at %s; run make dotnet-roslyn", dir)
	}
	return dir
}

// recordingRunner records the snapshot root of each helper analysis run.
type recordingRunner struct {
	toolrun.Runner
	mu        sync.Mutex
	snapshots []string
}

func (r *recordingRunner) Run(ctx context.Context, cmd toolrun.ToolCmd) (toolrun.Output, error) {
	if len(cmd.Args) > 1 && strings.HasSuffix(cmd.Args[0], helperDLL) && cmd.Args[1] != argVersion {
		r.mu.Lock()
		r.snapshots = append(r.snapshots, cmd.Args[1])
		r.mu.Unlock()
	}
	return r.Runner.Run(ctx, cmd)
}

func TestIntegration_TypeGraphOnFixture(t *testing.T) {
	if testing.Short() {
		t.Skip("loading the fixture in the helper takes seconds offline")
	}
	runner := &recordingRunner{Runner: realRunner(t)}
	t.Setenv(helperEnv, helperForIntegration(t))
	root := copyFixture(t)
	before := treeState(t, root)
	cfg := evidenceports.ExtractConfig{Mode: evidenceports.ModeOn, Exclusions: []string{contractsCS}}
	e := dotnet.New(runner, cfg, nil)
	facts, _, err := e.Extract(context.Background(), scope.Scope{Root: root})
	if err != nil {
		t.Fatalf("Extract: %v", err)
	}

	want := map[string]hints{
		repositoryCS + " -> file:" + domainCS: {hIntrusive, hModel},
		legacyCS + " -> file:" + domainCS:     {hFunctional, hModel},
	}
	if got := fileEdges(facts); !mapsEqual(got, want) {
		t.Errorf("file edges = %v, want %v", got, want)
	}
	tg := e.LastTypeGraph()
	if p := typeProject(t, tg, infraProj); p.Status != dotnet.TypeGraphComplete || p.UnresolvedSymbols != 0 {
		t.Errorf("infrastructure = %+v, want its use of the excluded %s resolved", p, contractsCS)
	}
	legacy := typeProject(t, tg, legacyProj)
	missingPack := legacy.Status == dotnet.TypeGraphPartial && strings.Contains(legacy.Reason, "Microsoft.NETCore.App.Ref 8.")
	if legacy.Status != dotnet.TypeGraphComplete && !missingPack {
		t.Errorf("legacy = %+v, want complete, or partial naming the 8.0 reference pack", legacy)
	}
	if b := typeProject(t, tg, brokenProj); b.Status != dotnet.TypeGraphFailed || !strings.Contains(b.Reason, "Archfit.Fixture.Missing.Sdk") {
		t.Errorf("broken = %+v", b)
	}
	if !slices.Equal(tg.Fallback, []string{brokenProj}) || len(projectEdges(facts)) != 0 {
		t.Errorf("fallback = %v, project edges = %v, want only Broken and no edge", tg.Fallback, projectEdges(facts))
	}
	cov := e.LastTypeGraphCoverage()
	if !strings.HasPrefix(cov.Version, "archfit-roslyn ") || cov.Status != evidence.StatusPartial || cov.UnresolvedInputsMissing == 0 {
		t.Errorf("roslyn coverage = %+v", cov)
	}

	if len(runner.snapshots) != 1 {
		t.Fatalf("helper runs = %v, want one", runner.snapshots)
	}
	if _, err := os.Stat(runner.snapshots[0]); !errors.Is(err, fs.ErrNotExist) {
		t.Errorf("snapshot %s still exists (err %v)", runner.snapshots[0], err)
	}
	if after := treeState(t, root); !mapsEqual(after, before) {
		t.Errorf("the original root changed:\nbefore %v\nafter  %v", before, after)
	}
}

// TestIntegration_SubtreeInheritsAncestorBuildProps scans src/ of a repository
// whose root Directory.Build.props defines the constant that compiles App's
// only use of Lib. Without the ancestor file in the snapshot the use is not
// compiled and the edge is missing.
func TestIntegration_SubtreeInheritsAncestorBuildProps(t *testing.T) {
	if testing.Short() {
		t.Skip("loads two projects in the helper")
	}
	runner := &recordingRunner{Runner: realRunner(t)}
	t.Setenv(helperEnv, helperForIntegration(t))
	repo := t.TempDir()
	const sdkProject = `<Project Sdk="Microsoft.NET.Sdk"><PropertyGroup><TargetFramework>net10.0</TargetFramework></PropertyGroup>%s</Project>`
	writeFile(t, repo, "Directory.Build.props", `<Project><PropertyGroup><DefineConstants>$(DefineConstants);ARCHFIT_ANCESTOR</DefineConstants></PropertyGroup></Project>`)
	writeFile(t, repo, "src/Lib/Lib.csproj", strings.Replace(sdkProject, "%s", "", 1))
	writeFile(t, repo, "src/Lib/Thing.cs", "namespace Lib;\npublic class Thing { public void Run() { } }\n")
	writeFile(t, repo, "src/App/App.csproj", strings.Replace(sdkProject, "%s", `<ItemGroup><ProjectReference Include="../Lib/Lib.csproj" /></ItemGroup>`, 1))
	writeFile(t, repo, "src/App/Use.cs", "namespace App;\n#if ARCHFIT_ANCESTOR\npublic class U { public void M(Lib.Thing t) => t.Run(); }\n#endif\n")
	root := filepath.Join(repo, "src")
	before := treeState(t, repo)

	e := dotnet.New(runner, evidenceports.ExtractConfig{Mode: evidenceports.ModeOn}, nil)
	facts, _, err := e.Extract(context.Background(), scope.Scope{Root: root, GitRoot: repo, SubtreePrefix: "src"})
	if err != nil {
		t.Fatalf("Extract: %v", err)
	}
	if got := fileEdges(facts)["App/Use.cs -> file:Lib/Thing.cs"]; got.hint != hFunctional {
		t.Errorf("file edges = %v, want App/Use.cs -> Lib/Thing.cs (functional) compiled under the ancestor's constant", fileEdges(facts))
	}
	if cov := e.LastTypeGraphCoverage(); cov.Status != evidence.StatusOK {
		t.Errorf("roslyn coverage = %+v", cov)
	}
	if len(runner.snapshots) != 1 {
		t.Fatalf("helper runs = %v, want one", runner.snapshots)
	}
	if top := filepath.Dir(runner.snapshots[0]); exists(top) {
		t.Errorf("snapshot %s still exists", top)
	}
	if after := treeState(t, repo); !mapsEqual(after, before) {
		t.Errorf("the original tree changed:\nbefore %v\nafter  %v", before, after)
	}
}

// TestIntegration_CheckoutReusesWorkingTreeRestoreOutputs analyses a checkout
// of tracked files (the --base worktree) of a restored working tree. App's only
// use of Lib compiles under a constant that App's restore outputs define, as a
// package's build props would. Without the working tree's restore outputs the
// use is not compiled and the edge is missing. A checkout whose App references
// a package the working tree did not restore is told apart from it by NuGet's
// own restore graph, offline.
func TestIntegration_CheckoutReusesWorkingTreeRestoreOutputs(t *testing.T) {
	if testing.Short() {
		t.Skip("restores one project and loads two in the helper, three times")
	}
	runner := realRunner(t)
	t.Setenv(helperEnv, helperForIntegration(t))
	const (
		sdkProject = `<Project Sdk="Microsoft.NET.Sdk"><PropertyGroup><TargetFramework>net10.0</TargetFramework></PropertyGroup>%s</Project>`
		libRef     = `<ItemGroup><ProjectReference Include="../Lib/Lib.csproj" /></ItemGroup>`
		appProj    = "App/App.csproj"
	)
	tracked := map[string]string{
		"Lib/Lib.csproj": strings.Replace(sdkProject, "%s", "", 1),
		"Lib/Thing.cs":   "namespace Lib;\npublic class Thing { public void Run() { } }\n",
		appProj:          strings.Replace(sdkProject, "%s", libRef, 1),
		"App/Use.cs":     "namespace App;\n#if ARCHFIT_RESTORED\npublic class U { public void M(Lib.Thing t) => t.Run(); }\n#endif\n",
	}
	checkoutOf := func(changes map[string]string) string {
		dir := t.TempDir()
		for rel, content := range tracked {
			if changed, ok := changes[rel]; ok {
				content = changed
			}
			writeFile(t, dir, rel, content)
		}
		return dir
	}
	work := checkoutOf(nil)
	out, err := runner.Run(context.Background(), toolrun.ToolCmd{Name: toolDotnet, Args: []string{"restore", "App.csproj"}, WorkDir: filepath.Join(work, "App")})
	if err != nil || out.ExitCode != 0 {
		t.Fatalf("restore: %v %s %s", err, out.Stdout, out.Stderr)
	}
	writeFile(t, work, "App/obj/App.csproj.archfit.props",
		`<Project><PropertyGroup><DefineConstants>$(DefineConstants);ARCHFIT_RESTORED</DefineConstants></PropertyGroup></Project>`)
	workBefore := treeState(t, work)

	for _, tt := range []struct {
		name           string
		checkout       string
		dependencyRoot string
		wantEdge       bool
		wantStatus     string
		wantReason     string
	}{
		{
			name: "same restore inputs", checkout: checkoutOf(nil), dependencyRoot: work, wantEdge: true,
			wantStatus: evidence.StatusOK, wantReason: "restore outputs reused from the working tree for 2 projects",
		},
		{
			name: "package added in the checkout", dependencyRoot: work, wantEdge: true, wantStatus: evidence.StatusPartial,
			checkout: checkoutOf(map[string]string{appProj: strings.Replace(sdkProject, "%s",
				libRef+`<ItemGroup><PackageReference Include="Archfit.Fixture.Unrestored" Version="1.0.0" /></ItemGroup>`, 1)}),
			wantReason: "restore inputs (package references, central versions, sources, global.json, NuGet.config, " +
				"Directory.Build.props/.targets, Directory.Packages.props) differ from the working tree's " +
				"for 1 project, so they are partial: App/App.csproj; restore outputs reused from the working tree for 2 projects",
		},
		{name: "checkout alone", checkout: checkoutOf(nil), wantStatus: evidence.StatusOK},
	} {
		t.Run(tt.name, func(t *testing.T) {
			checkoutBefore := treeState(t, tt.checkout)
			e := dotnet.New(runner, evidenceports.ExtractConfig{Mode: evidenceports.ModeOn}, nil)
			facts, _, err := e.Extract(context.Background(), scope.Scope{Root: tt.checkout, DependencyRoot: tt.dependencyRoot})
			if err != nil {
				t.Fatalf("Extract: %v", err)
			}
			if got := fileEdges(facts)["App/Use.cs -> file:Lib/Thing.cs"]; (got.hint == hFunctional) != tt.wantEdge {
				t.Errorf("file edges = %v, want App/Use.cs -> Lib/Thing.cs %v", fileEdges(facts), tt.wantEdge)
			}
			if cov := e.LastTypeGraphCoverage(); cov.Status != tt.wantStatus || !strings.HasPrefix(cov.Reason, tt.wantReason) {
				t.Errorf("roslyn coverage = %s %q, want %s starting %q", cov.Status, cov.Reason, tt.wantStatus, tt.wantReason)
			}
			if after := treeState(t, tt.checkout); !mapsEqual(after, checkoutBefore) {
				t.Errorf("the checkout changed:\nbefore %v\nafter  %v", checkoutBefore, after)
			}
		})
	}
	if after := treeState(t, work); !mapsEqual(after, workBefore) {
		t.Errorf("the working tree changed:\nbefore %v\nafter  %v", workBefore, after)
	}
}
