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
	if len(cmd.Args) > 1 && strings.HasSuffix(cmd.Args[0], helperDLL) && cmd.Args[1] != "--version" {
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
