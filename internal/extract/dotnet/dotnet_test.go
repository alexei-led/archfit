package dotnet_test

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"testing"

	evidenceports "github.com/alexei-led/archfit/v3/internal/evidence/ports"
	"github.com/alexei-led/archfit/v3/internal/extract/dotnet"
	"github.com/alexei-led/archfit/v3/internal/model/evidence"
	"github.com/alexei-led/archfit/v3/internal/model/graph"
	"github.com/alexei-led/archfit/v3/internal/scope"
	"github.com/alexei-led/archfit/v3/internal/toolrun"
)

const (
	dotnetVersion = "10.0.402"
	rootToken     = "{{ROOT}}"
	nodeApp       = "project:Application/Application.csproj"
	nodeDomain    = "project:Domain/Domain.csproj"
	nodeLegacy    = "project:Legacy/Legacy.csproj"

	appProj     = "Application/Application.csproj"
	domainProj  = "Domain/Domain.csproj"
	legacyProj  = "Legacy/Legacy.csproj"
	legacyNet10 = "Legacy/Legacy.csproj@net10.0"
	legacyNet8  = "Legacy/Legacy.csproj@net8.0"
	brokenProj  = "Broken/Broken.csproj"
	targetNet10 = "net10.0"
	targetNet8  = "net8.0"
	solutionFix = "Fixture.slnx"
	projA       = "A/A.csproj"
	toolDotnet  = "dotnet"
	targetB     = "x/B.csproj"
	argVersion  = "--version"
	argMSBuild  = "msbuild"
	mauiProj    = "Maui/Maui.csproj"
)

// reply is a recorded dotnet msbuild result.
type reply struct {
	stdout, stderr string
	exit           int
	err            error
}

// fixture loads a recorded file with its root token filled in.
func fixture(t *testing.T, root, name string) string {
	t.Helper()
	data, err := os.ReadFile(filepath.Join("testdata", name)) //nolint:gosec // test fixture path
	if err != nil {
		t.Fatalf("load %s: %v", name, err)
	}
	return strings.ReplaceAll(string(data), rootToken, root)
}

func ok(t *testing.T, root, name string) reply {
	return reply{stdout: fixture(t, root, name)}
}

// fakeDotnet answers `dotnet --version` and `dotnet msbuild`. Replies are keyed
// by the project path relative to root, plus "@<tfm>" for a per-framework run.
// Calls to the Roslyn helper go to helper; with none set they fail the test.
type fakeDotnet struct {
	*toolrun.RunnerMock
	mu          sync.Mutex
	all         []toolrun.ToolCmd // every dotnet call, helper and --version included
	calls       []toolrun.ToolCmd
	replies     map[string]reply
	helper      func(ctx context.Context, cmd toolrun.ToolCmd) (toolrun.Output, error)
	helperCalls []toolrun.ToolCmd
}

func newFake(t *testing.T, root string, replies map[string]reply) *fakeDotnet {
	f := &fakeDotnet{replies: replies}
	f.RunnerMock = &toolrun.RunnerMock{
		DetectFunc: func(_ context.Context, tool string) (toolrun.ToolInfo, bool) {
			return toolrun.ToolInfo{Name: tool, Path: "/usr/bin/" + tool}, tool == toolDotnet
		},
		RunFunc: func(ctx context.Context, cmd toolrun.ToolCmd) (toolrun.Output, error) {
			f.mu.Lock()
			f.all = append(f.all, cmd)
			f.mu.Unlock()
			if len(cmd.Args) > 0 && strings.HasSuffix(cmd.Args[0], helperDLL) {
				f.mu.Lock()
				f.helperCalls = append(f.helperCalls, cmd)
				h := f.helper
				f.mu.Unlock()
				if h == nil {
					t.Errorf("unexpected helper call %v", cmd.Args)
					return toolrun.Output{ExitCode: 1}, nil
				}
				return h(ctx, cmd)
			}
			if slices.Contains(cmd.Args, argVersion) {
				return toolrun.Output{Stdout: []byte(dotnetVersion + "\n")}, nil
			}
			key := cmdKey(t, root, cmd)
			f.mu.Lock()
			f.calls = append(f.calls, cmd)
			r, found := f.replies[key]
			f.mu.Unlock()
			if !found {
				t.Errorf("unexpected dotnet call %q", key)
				return toolrun.Output{ExitCode: 1}, nil
			}
			return toolrun.Output{Stdout: []byte(r.stdout), Stderr: []byte(r.stderr), ExitCode: r.exit}, r.err
		},
	}
	return f
}

func cmdKey(t *testing.T, root string, cmd toolrun.ToolCmd) string {
	t.Helper()
	if len(cmd.Args) < 2 || cmd.Args[0] != argMSBuild {
		t.Fatalf("not an msbuild call: %v", cmd.Args)
	}
	rel, err := filepath.Rel(root, filepath.Join(cmd.WorkDir, cmd.Args[1]))
	if err != nil {
		t.Fatal(err)
	}
	for _, a := range cmd.Args {
		if tfm, found := strings.CutPrefix(a, "-p:TargetFramework="); found {
			return filepath.ToSlash(rel) + "@" + tfm
		}
	}
	return filepath.ToSlash(rel)
}

func (f *fakeDotnet) keys(t *testing.T, root string) []string {
	out := make([]string, 0, len(f.calls))
	for _, c := range f.calls {
		out = append(out, cmdKey(t, root, c))
	}
	slices.Sort(out)
	return out
}

// tree writes empty files under a temp root and returns it.
func tree(t *testing.T, files ...string) string {
	t.Helper()
	root := t.TempDir()
	for _, f := range files {
		writeFile(t, root, f, "")
	}
	return root
}

func writeFile(t *testing.T, root, rel, content string) {
	t.Helper()
	p := filepath.Join(root, filepath.FromSlash(rel))
	if err := os.MkdirAll(filepath.Dir(p), 0o750); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
}

func autoCfg() evidenceports.ExtractConfig {
	return evidenceports.ExtractConfig{Mode: evidenceports.ModeAuto}
}

func hasEdge(facts graph.Facts, from, to string) bool { //nolint:unparam // generic helper; callers happen to share a target
	return slices.ContainsFunc(facts.Edges, func(e graph.Edge) bool {
		return e.From == from && e.To == to && e.Kind == graph.EdgeKindDependsOn
	})
}

func nodeIDs(facts graph.Facts) []string {
	ids := make([]string, 0, len(facts.Nodes))
	for _, n := range facts.Nodes {
		ids = append(ids, n.ID())
	}
	slices.Sort(ids)
	return ids
}

func projectByPath(t *testing.T, ev dotnet.Evaluation, path string) dotnet.ProjectEvaluation {
	t.Helper()
	for _, p := range ev.Projects {
		if p.Path == path {
			return p
		}
	}
	t.Fatalf("no project %s in %v", path, ev.Projects)
	return dotnet.ProjectEvaluation{}
}

// evalCase is one recorded-evaluation scenario.
type evalCase struct {
	name  string
	files []string
	// replies builds the recorded replies once the root is known.
	replies func(t *testing.T, root string) map[string]reply
	check   func(t *testing.T, facts graph.Facts, ev dotnet.Evaluation, cov evidence.Coverage)
}

func successCases() []evalCase {
	return []evalCase{
		{
			name:  "one target framework",
			files: []string{appProj, domainProj},
			replies: func(t *testing.T, root string) map[string]reply {
				return map[string]reply{
					appProj:    ok(t, root, "application.json"),
					domainProj: ok(t, root, "domain.json"),
				}
			},
			check: func(t *testing.T, facts graph.Facts, ev dotnet.Evaluation, cov evidence.Coverage) {
				if cov.Status != evidence.StatusOK || cov.Version != dotnetVersion || cov.Tool != "dotnet-msbuild" {
					t.Errorf("coverage = %+v", cov)
				}
				if !hasEdge(facts, nodeApp, nodeDomain) || len(facts.Edges) != 1 {
					t.Errorf("edges = %+v", facts.Edges)
				}
				if got := facts.Edges[0].Locations; len(got) != 1 || got[0].File != appProj {
					t.Errorf("edge locations = %+v, want the defining project file", got)
				}
				app := projectByPath(t, ev, appProj)
				if app.Status != dotnet.ProjectOK || !slices.Equal(app.TargetFrameworks, []string{targetNet10}) || app.IsTestProject {
					t.Errorf("application = %+v", app)
				}
			},
		},
		{
			name:  "two target frameworks find the conditional reference per framework",
			files: []string{legacyProj, domainProj},
			replies: func(t *testing.T, root string) map[string]reply {
				return map[string]reply{
					legacyProj:  ok(t, root, "legacy_base.json"),
					legacyNet10: ok(t, root, "legacy_net10.json"),
					legacyNet8:  ok(t, root, "legacy_net8.json"),
					domainProj:  ok(t, root, "domain.json"),
				}
			},
			check: func(t *testing.T, facts graph.Facts, ev dotnet.Evaluation, cov evidence.Coverage) {
				if !hasEdge(facts, nodeLegacy, nodeDomain) {
					t.Errorf("missing Legacy -> Domain edge: %+v", facts.Edges)
				}
				legacy := projectByPath(t, ev, legacyProj)
				if !slices.Equal(legacy.TargetFrameworks, []string{targetNet10, targetNet8}) {
					t.Errorf("TFMs = %v", legacy.TargetFrameworks)
				}
				if !slices.Equal(legacy.References, []string{domainProj}) || legacy.Status != dotnet.ProjectOK {
					t.Errorf("legacy = %+v", legacy)
				}
				if cov.Status != evidence.StatusOK {
					t.Errorf("coverage = %+v", cov)
				}
			},
		},
		{
			name:  "reference to a project with no file is missing, not an edge",
			files: []string{appProj},
			replies: func(t *testing.T, root string) map[string]reply {
				return map[string]reply{appProj: ok(t, root, "missing_reference.json")}
			},
			check: func(t *testing.T, facts graph.Facts, ev dotnet.Evaluation, cov evidence.Coverage) {
				app := projectByPath(t, ev, appProj)
				if app.Status != dotnet.ProjectPartial || !slices.Equal(app.Missing, []string{"Ghost/Ghost.csproj"}) {
					t.Errorf("application = %+v", app)
				}
				if len(facts.Edges) != 0 {
					t.Errorf("edges = %+v, want none", facts.Edges)
				}
				if cov.Status != evidence.StatusPartial || cov.Unresolved != 1 || !strings.Contains(cov.Reason, "Ghost/Ghost.csproj") {
					t.Errorf("coverage = %+v", cov)
				}
			},
		},
		{
			name:  "package references are collected",
			files: []string{"Unresolved/Unresolved.csproj"},
			replies: func(t *testing.T, root string) map[string]reply {
				return map[string]reply{"Unresolved/Unresolved.csproj": ok(t, root, "unresolved.json")}
			},
			check: func(t *testing.T, _ graph.Facts, ev dotnet.Evaluation, cov evidence.Coverage) {
				p := projectByPath(t, ev, "Unresolved/Unresolved.csproj")
				if !slices.Equal(p.Packages, []string{"Archfit.Fixture.Missing.Package"}) || cov.Status != evidence.StatusOK {
					t.Errorf("project = %+v, coverage = %+v", p, cov)
				}
			},
		},
	}
}

func failureCases() []evalCase {
	const (
		msb4236 = "MSB4236: SDK 'Archfit.Fixture.Missing.Sdk' not resolvable offline; run with NuGet access and a warm cache"
		globalJ = "global.json requests SDK 9.0.100, which is not installed; install a matching SDK or relax the pin"
	)
	return []evalCase{
		{
			name:  "MSB4236 names the SDK",
			files: []string{brokenProj},
			replies: func(t *testing.T, root string) map[string]reply {
				return map[string]reply{brokenProj: {stderr: fixture(t, root, "msb4236.stderr"), exit: 1}}
			},
			check: func(t *testing.T, facts graph.Facts, ev dotnet.Evaluation, cov evidence.Coverage) {
				p := projectByPath(t, ev, brokenProj)
				if p.Status != dotnet.ProjectFailed || p.Reason != msb4236 {
					t.Errorf("project = %+v", p)
				}
				if got := nodeIDs(facts); !slices.Equal(got, []string{"project:Broken/Broken.csproj"}) {
					t.Errorf("nodes = %v, want the failed project kept for topology", got)
				}
				if cov.Status != evidence.StatusPartial || !strings.Contains(cov.Reason, msb4236) {
					t.Errorf("coverage = %+v", cov)
				}
			},
		},
		{
			name:  "unsatisfied SDK pin names global.json",
			files: []string{domainProj, "global.json"},
			replies: func(t *testing.T, root string) map[string]reply {
				return map[string]reply{domainProj: {
					stdout: fixture(t, root, "globaljson.stdout"),
					stderr: fixture(t, root, "globaljson.stderr"),
					exit:   155,
				}}
			},
			check: func(t *testing.T, _ graph.Facts, ev dotnet.Evaluation, cov evidence.Coverage) {
				p := projectByPath(t, ev, domainProj)
				if p.Status != dotnet.ProjectFailed || p.Reason != globalJ {
					t.Errorf("project = %+v, want reason %q", p, globalJ)
				}
				if cov.Status != evidence.StatusPartial {
					t.Errorf("coverage = %+v", cov)
				}
			},
		},
		{
			name:  "a missing workload names the workloads and the install command",
			files: []string{mauiProj},
			replies: func(_ *testing.T, root string) map[string]reply {
				return map[string]reply{mauiProj: {
					stdout: "/usr/local/share/dotnet/sdk/10.0.403/Sdks/Microsoft.NET.Sdk/targets/Microsoft.NET.Sdk.ImportWorkloads.targets(38,5): error NETSDK1147: To build this project, the following workloads must be installed: maui-android maui-ios [" + root + "/Maui/Maui.csproj]\n" +
						"/usr/local/share/dotnet/sdk/10.0.403/Sdks/Microsoft.NET.Sdk/targets/Microsoft.NET.Sdk.ImportWorkloads.targets(38,5): error NETSDK1147: To install these workloads, run the following command: dotnet workload restore [" + root + "/Maui/Maui.csproj]\n",
					exit: 1,
				}}
			},
			check: func(t *testing.T, _ graph.Facts, ev dotnet.Evaluation, _ evidence.Coverage) {
				const want = "NETSDK1147: workload maui-android maui-ios not installed; run `dotnet workload install maui-android maui-ios` or exclude the project"
				if got := projectByPath(t, ev, mauiProj); got.Status != dotnet.ProjectFailed || got.Reason != want {
					t.Errorf("project = %+v, want reason %q", got, want)
				}
			},
		},
		{
			name:  "workload packs missing on this host name the packs",
			files: []string{mauiProj},
			replies: func(*testing.T, string) map[string]reply {
				return map[string]reply{mauiProj: {
					stdout: "Microsoft.NET.Sdk.ImportWorkloads.targets(38,5): error NETSDK1178: The project depends on the following workload packs that do not exist in any of the workloads available in this installation: Microsoft.iOS.Sdk.net10.0_26.0 [Maui.csproj]\n",
					exit:   1,
				}}
			},
			check: func(t *testing.T, _ graph.Facts, ev dotnet.Evaluation, _ evidence.Coverage) {
				if got := projectByPath(t, ev, mauiProj).Reason; !strings.HasPrefix(got, "NETSDK1178: workload packs Microsoft.iOS.Sdk.net10.0_26.0 are not available") {
					t.Errorf("reason = %q", got)
				}
			},
		},
		{
			name:  "other errors keep the first error line, root-relative",
			files: []string{domainProj},
			replies: func(_ *testing.T, root string) map[string]reply {
				return map[string]reply{domainProj: {
					stderr: "Welcome\n" + root + "/Domain/Domain.csproj : error MSB4025: The project file could not be loaded.\n" + root + "/x : error : later\n",
					exit:   1,
				}}
			},
			check: func(t *testing.T, _ graph.Facts, ev dotnet.Evaluation, _ evidence.Coverage) {
				got := projectByPath(t, ev, domainProj)
				const want = "Domain/Domain.csproj : error MSB4025: The project file could not be loaded."
				if got.Status != dotnet.ProjectFailed || got.Reason != want {
					t.Errorf("project = %+v, want reason %q", got, want)
				}
			},
		},
		{
			name:  "failed evaluation with no error line reports the exit code",
			files: []string{domainProj},
			replies: func(*testing.T, string) map[string]reply {
				return map[string]reply{domainProj: {exit: 3}}
			},
			check: func(t *testing.T, _ graph.Facts, ev dotnet.Evaluation, _ evidence.Coverage) {
				if got := projectByPath(t, ev, domainProj).Reason; got != "dotnet msbuild exited 3" {
					t.Errorf("reason = %q", got)
				}
			},
		},
		{
			name:  "unparsable output on exit 0 is a failed evaluation",
			files: []string{domainProj},
			replies: func(*testing.T, string) map[string]reply {
				return map[string]reply{domainProj: {stdout: "not json"}}
			},
			check: func(t *testing.T, _ graph.Facts, ev dotnet.Evaluation, _ evidence.Coverage) {
				p := projectByPath(t, ev, domainProj)
				if p.Status != dotnet.ProjectFailed || !strings.Contains(p.Reason, "unparsable") {
					t.Errorf("project = %+v", p)
				}
			},
		},
		{
			name:  "test project keeps its references but contributes no fallback edges",
			files: []string{"Tests/Tests.csproj", brokenProj},
			replies: func(t *testing.T, root string) map[string]reply {
				return map[string]reply{
					"Tests/Tests.csproj": ok(t, root, "tests.json"),
					brokenProj:           {stderr: fixture(t, root, "msb4236.stderr"), exit: 1},
				}
			},
			check: func(t *testing.T, facts graph.Facts, ev dotnet.Evaluation, _ evidence.Coverage) {
				tp := projectByPath(t, ev, "Tests/Tests.csproj")
				if !tp.IsTestProject || !slices.Equal(tp.References, []string{brokenProj}) {
					t.Errorf("tests = %+v", tp)
				}
				if tp.FallbackReferences() != nil || len(facts.Edges) != 0 {
					t.Errorf("fallback = %v, edges = %+v, want none", tp.FallbackReferences(), facts.Edges)
				}
			},
		},
	}
}

func TestExtract_Evaluations(t *testing.T) {
	tests := slices.Concat(successCases(), failureCases())
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			root := tree(t, tt.files...)
			e := dotnet.New(newFake(t, root, tt.replies(t, root)), autoCfg(), nil)
			facts, cov, err := e.Extract(context.Background(), scope.Scope{Root: root})
			if err != nil {
				t.Fatalf("Extract: %v", err)
			}
			tt.check(t, facts, e.LastEvaluation(), cov)
		})
	}
}

func TestExtract_FrameworkEvaluationFailureIsPartial(t *testing.T) {
	root := tree(t, legacyProj, domainProj)
	fake := newFake(t, root, map[string]reply{
		legacyProj:  ok(t, root, "legacy_base.json"),
		legacyNet10: ok(t, root, "legacy_net10.json"),
		legacyNet8:  {stderr: "Legacy.csproj : error NU1100: boom\n", exit: 1},
		domainProj:  ok(t, root, "domain.json"),
	})
	e := dotnet.New(fake, autoCfg(), nil)
	facts, cov, err := e.Extract(context.Background(), scope.Scope{Root: root})
	if err != nil {
		t.Fatal(err)
	}
	p := projectByPath(t, e.LastEvaluation(), legacyProj)
	if p.Status != dotnet.ProjectPartial || !strings.HasPrefix(p.Reason, "target framework net8.0: ") {
		t.Errorf("project = %+v", p)
	}
	if !hasEdge(facts, nodeLegacy, nodeDomain) || cov.Status != evidence.StatusPartial {
		t.Errorf("edges = %+v, coverage = %+v", facts.Edges, cov)
	}
}

func TestExtract_RunsEachProjectAndFramework(t *testing.T) {
	root := tree(t, legacyProj, domainProj)
	fake := newFake(t, root, map[string]reply{
		legacyProj:  ok(t, root, "legacy_base.json"),
		legacyNet10: ok(t, root, "legacy_net10.json"),
		legacyNet8:  ok(t, root, "legacy_net8.json"),
		domainProj:  ok(t, root, "domain.json"),
	})
	if _, _, err := dotnet.New(fake, autoCfg(), nil).Extract(context.Background(), scope.Scope{Root: root}); err != nil {
		t.Fatal(err)
	}
	want := []string{domainProj, legacyProj, legacyNet10, legacyNet8}
	if got := fake.keys(t, root); !slices.Equal(got, want) {
		t.Errorf("calls = %v, want %v", got, want)
	}
	call := fake.calls[0]
	for _, flag := range []string{"-getItem:ProjectReference", "-getItem:PackageReference", "-getProperty:TargetFramework", "-getProperty:TargetFrameworks", "-getProperty:IsTestProject"} {
		if !slices.Contains(call.Args, flag) {
			t.Errorf("args %v lack %s", call.Args, flag)
		}
	}
	for _, env := range []string{"DOTNET_CLI_TELEMETRY_OPTOUT=1", "DOTNET_NOLOGO=1", "DOTNET_SKIP_FIRST_TIME_EXPERIENCE=1", "MSBUILDDISABLENODEREUSE=1"} {
		if !slices.Contains(call.Env, env) {
			t.Errorf("env %v lacks %s", call.Env, env)
		}
	}
	if call.Timeout <= 0 {
		t.Error("evaluation has no timeout")
	}
}

func TestExtract_TimeoutIsFailedNotTruncated(t *testing.T) {
	root := tree(t, domainProj, appProj)
	fake := newFake(t, root, map[string]reply{
		domainProj: {err: context.DeadlineExceeded},
		appProj:    ok(t, root, "application.json"),
	})
	e := dotnet.New(fake, autoCfg(), nil)
	_, cov, err := e.Extract(context.Background(), scope.Scope{Root: root})
	if err != nil {
		t.Fatal(err)
	}
	ev := e.LastEvaluation()
	if len(ev.Projects) != 2 {
		t.Fatalf("projects = %+v, want both reported", ev.Projects)
	}
	p := projectByPath(t, ev, domainProj)
	if p.Status != dotnet.ProjectFailed || !strings.Contains(p.Reason, "timed out") {
		t.Errorf("project = %+v", p)
	}
	if cov.Status != evidence.StatusTimedOut || !strings.Contains(cov.Reason, "timed out") {
		t.Errorf("coverage = %+v", cov)
	}
}

func TestExtract_ExecFailureIsAnError(t *testing.T) {
	root := tree(t, domainProj)
	fake := newFake(t, root, map[string]reply{domainProj: {err: errors.New("fork failed")}})
	if _, _, err := dotnet.New(fake, autoCfg(), nil).Extract(context.Background(), scope.Scope{Root: root}); err == nil {
		t.Fatal("want an error for an exec-level failure")
	}
}

func TestExtract_SolutionSelection(t *testing.T) {
	const slnx = `<Solution>
  <Folder Name="/src/"><Project Path="Domain/Domain.csproj" /></Folder>
  <Project Path="Gone/Gone.csproj" />
  <Project Path="Db/Db.sqlproj" />
</Solution>`
	root := tree(t, domainProj, "Stray/Stray.csproj", "Db/Db.sqlproj")
	writeFile(t, root, "A.slnx", slnx)
	fake := newFake(t, root, map[string]reply{domainProj: ok(t, root, "domain.json")})
	e := dotnet.New(fake, autoCfg(), nil)
	facts, cov, err := e.Extract(context.Background(), scope.Scope{Root: root})
	if err != nil {
		t.Fatal(err)
	}
	ev := e.LastEvaluation()
	if ev.Solution != "A.slnx" || ev.SkippedProjects != 1 || !slices.Equal(ev.MissingProjects, []string{"Gone/Gone.csproj"}) {
		t.Errorf("evaluation = %+v", ev)
	}
	if got := nodeIDs(facts); !slices.Equal(got, []string{nodeDomain}) {
		t.Errorf("nodes = %v, want only the solution's project", got)
	}
	if cov.Status != evidence.StatusPartial || !strings.Contains(cov.Reason, "Gone/Gone.csproj") {
		t.Errorf("coverage = %+v, want partial naming the missing project", cov)
	}
}

func TestExtract_ConfiguredTargets(t *testing.T) {
	root := tree(t, domainProj, appProj)
	fake := newFake(t, root, map[string]reply{domainProj: ok(t, root, "domain.json")})
	e := dotnet.New(fake, autoCfg(), []string{domainProj, "Nope/Nope.csproj"})
	_, cov, err := e.Extract(context.Background(), scope.Scope{Root: root})
	if err != nil {
		t.Fatal(err)
	}
	if got := e.LastEvaluation().MissingProjects; !slices.Equal(got, []string{"Nope/Nope.csproj"}) {
		t.Errorf("missing = %v", got)
	}
	if cov.Status != evidence.StatusPartial {
		t.Errorf("coverage = %+v", cov)
	}
}

func TestExtract_SolutionFilterTarget(t *testing.T) {
	const webFilter = "Web.slnf"
	const filter = `{"solution":{"path":"All.slnx","projects":["Domain\\Domain.csproj","Gone\\Gone.csproj"]}}`
	tests := []struct {
		name    string
		files   []string
		wantErr string
	}{
		{name: "filter selects its projects, not the root solution", files: []string{"All.slnx"}},
		{name: "filter whose base solution is missing", wantErr: "Web.slnf: solution All.slnx not found under the root"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			root := tree(t, append([]string{domainProj, appProj}, tt.files...)...)
			writeFile(t, root, webFilter, filter)
			fake := newFake(t, root, map[string]reply{domainProj: ok(t, root, "domain.json")})
			installHelper(t)
			fake.helper = fakeHelper(replying(`{"schema_version":2,"projects":[]}`))
			e := dotnet.New(fake, autoCfg(), []string{webFilter})
			facts, cov, err := e.Extract(context.Background(), scope.Scope{Root: root})
			if err != nil {
				t.Fatal(err)
			}
			if tt.wantErr != "" {
				if cov.Status != evidence.StatusPartial || !strings.Contains(cov.Reason, tt.wantErr) {
					t.Errorf("coverage = %+v, want partial naming %q", cov, tt.wantErr)
				}
				return
			}
			ev := e.LastEvaluation()
			if ev.Solution != webFilter || !slices.Equal(ev.MissingProjects, []string{"Gone/Gone.csproj"}) {
				t.Errorf("evaluation = %+v", ev)
			}
			if got := nodeIDs(facts); !slices.Equal(got, []string{nodeDomain}) {
				t.Errorf("nodes = %v, want only the filter's project", got)
			}
			if got := fake.helperCalls[1].Args[2:]; !slices.Equal(got, []string{webFilter}) {
				t.Errorf("helper targets = %v, want the solution filter", got)
			}
		})
	}
}

func TestExtract_WalkRespectsExclusionsAndBuildDirs(t *testing.T) {
	root := tree(t, domainProj, "Domain/obj/Gen.csproj", "bin/Old.csproj", "Skip/Skip.csproj", "node_modules/x/X.csproj")
	fake := newFake(t, root, map[string]reply{domainProj: ok(t, root, "domain.json")})
	cfg := autoCfg()
	cfg.Exclusions = []string{"**/Skip/**"}
	facts, _, err := dotnet.New(fake, cfg, nil).Extract(context.Background(), scope.Scope{Root: root})
	if err != nil {
		t.Fatal(err)
	}
	if got := nodeIDs(facts); !slices.Equal(got, []string{nodeDomain}) {
		t.Errorf("nodes = %v", got)
	}
}

func TestExtract_NoProjectIsAbsentNotPartial(t *testing.T) {
	tests := []struct {
		name  string
		files []string
		mode  evidenceports.ToolMode
	}{
		{"empty root", nil, evidenceports.ModeAuto},
		{"empty root, mode on", nil, evidenceports.ModeOn},
		{"only build output", []string{"obj/A.csproj", "bin/B.csproj", "src/main.cs"}, evidenceports.ModeAuto},
		{"mode off with a project", []string{projA}, evidenceports.ModeOff},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			root := tree(t, tt.files...)
			fake := newFake(t, root, nil)
			facts, cov, err := dotnet.New(fake, evidenceports.ExtractConfig{Mode: tt.mode}, nil).Extract(context.Background(), scope.Scope{Root: root})
			if err != nil {
				t.Fatal(err)
			}
			if cov.Status != evidence.StatusAbsent || len(facts.Nodes) != 0 || len(fake.calls) != 0 {
				t.Errorf("coverage = %+v, facts = %+v, calls = %d", cov, facts, len(fake.calls))
			}
		})
	}
}

func TestExtract_DotnetMissing(t *testing.T) {
	root := tree(t, projA)
	missing := &toolrun.RunnerMock{DetectFunc: func(context.Context, string) (toolrun.ToolInfo, bool) { return toolrun.ToolInfo{}, false }}

	_, cov, err := dotnet.New(missing, autoCfg(), nil).Extract(context.Background(), scope.Scope{Root: root})
	if err != nil || cov.Status != evidence.StatusAbsent {
		t.Errorf("auto: coverage = %+v, err = %v, want absent", cov, err)
	}
	_, _, err = dotnet.New(missing, evidenceports.ExtractConfig{Mode: evidenceports.ModeOn}, nil).Extract(context.Background(), scope.Scope{Root: root})
	if err == nil {
		t.Error("mode on: want an error when dotnet is missing")
	}
}

func TestExtract_MalformedSolutionIsPartial(t *testing.T) {
	root := tree(t, projA)
	writeFile(t, root, "A.slnx", "<Solution><Project")
	cov := func(mode evidenceports.ToolMode) (evidence.Coverage, error) {
		_, c, err := dotnet.New(newFake(t, root, nil), evidenceports.ExtractConfig{Mode: mode}, nil).Extract(context.Background(), scope.Scope{Root: root})
		return c, err
	}
	c, err := cov(evidenceports.ModeAuto)
	if err != nil || c.Status != evidence.StatusPartial || !strings.Contains(c.Reason, "A.slnx") {
		t.Errorf("auto: coverage = %+v, err = %v", c, err)
	}
	if _, err := cov(evidenceports.ModeOn); err == nil {
		t.Error("mode on: want an error")
	}
}

// analyzers.roslyn.enabled: false is a deliberate opt-out: the roslyn row reads
// disabled on every return once a project is selected, not only when the
// helper would have run. With no project it stays absent: nothing to analyze.
func TestExtract_TypeGraphOptOutIsDisabledOnEveryPath(t *testing.T) {
	missing := &toolrun.RunnerMock{DetectFunc: func(context.Context, string) (toolrun.ToolInfo, bool) { return toolrun.ToolInfo{}, false }}
	tests := []struct {
		name   string
		files  []string
		extra  map[string]string
		runner func(t *testing.T, root string) toolrun.Runner
		want   string
	}{
		{
			name:   "SDK missing",
			files:  []string{projA},
			runner: func(*testing.T, string) toolrun.Runner { return missing },
			want:   evidence.StatusDisabled,
		},
		{
			name:   "project selection error",
			files:  []string{projA},
			extra:  map[string]string{"A.slnx": "<Solution><Project"},
			runner: func(t *testing.T, root string) toolrun.Runner { return newFake(t, root, nil) },
			want:   evidence.StatusDisabled,
		},
		{
			name:   "no .NET project",
			runner: func(t *testing.T, root string) toolrun.Runner { return newFake(t, root, nil) },
			want:   evidence.StatusAbsent,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			root := tree(t, tt.files...)
			for rel, content := range tt.extra {
				writeFile(t, root, rel, content)
			}
			e := dotnet.New(tt.runner(t, root), autoCfg(), nil)
			e.TypeGraph.Disabled = true
			if _, _, err := e.Extract(context.Background(), scope.Scope{Root: root}); err != nil {
				t.Fatal(err)
			}
			if got := e.LastTypeGraphCoverage(); got.Tool != toolRoslyn || got.Status != tt.want {
				t.Errorf("roslyn coverage = %+v, want status %q", got, tt.want)
			}
		})
	}
}

func TestApplicable(t *testing.T) {
	tests := []struct {
		name       string
		files      []string
		exclusions []string
		targets    []string
		want       bool
	}{
		{"nothing", nil, nil, nil, false},
		{"root solution", []string{"A.sln"}, nil, nil, true},
		{"nested csproj", []string{"src/A/A.csproj"}, nil, nil, true},
		{"csproj under obj", []string{"obj/A.csproj"}, nil, nil, false},
		{"excluded csproj", []string{"src/A/A.csproj"}, []string{"src/**"}, nil, false},
		{"configured target exists", []string{targetB}, nil, []string{targetB}, true},
		{"configured target missing", []string{"A.csproj"}, nil, []string{targetB}, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			root := tree(t, tt.files...)
			if got := dotnet.Applicable(root, tt.exclusions, tt.targets); got != tt.want {
				t.Errorf("Applicable = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestNameAndCoverageTool(t *testing.T) {
	e := dotnet.New(nil, autoCfg(), nil)
	if e.Name() != toolDotnet || e.CoverageTool() != "dotnet-msbuild" {
		t.Errorf("Name = %q, CoverageTool = %q", e.Name(), e.CoverageTool())
	}
	if ev := e.LastEvaluation(); len(ev.Projects) != 0 {
		t.Errorf("LastEvaluation before Extract = %+v", ev)
	}
}
