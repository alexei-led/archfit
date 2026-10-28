package registry_test

import (
	"context"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/alexei-led/archfit/v3/internal/config"
	"github.com/alexei-led/archfit/v3/internal/extract/dotnet"
	"github.com/alexei-led/archfit/v3/internal/extract/registry"
	"github.com/alexei-led/archfit/v3/internal/model/evidence"
	"github.com/alexei-led/archfit/v3/internal/model/graph"
	"github.com/alexei-led/archfit/v3/internal/scope"
	"github.com/alexei-led/archfit/v3/internal/toolrun"
)

const (
	helperEnv  = "ARCHFIT_DOTNET_ROSLYN"
	brokenProj = "Broken/Broken.csproj"
	testsProj  = "Tests/Tests.csproj"
	hDTO       = "dto"
	hModel     = "model"
)

// The .NET lane end to end through the composition path analyze uses: a config
// file, its projected extract view, and the registry-built extractor running
// the real SDK and the built helper on a copy of the shared fixture.
func TestDotNetLaneOnFixture(t *testing.T) {
	if testing.Short() {
		t.Skip("evaluating and loading the fixture takes seconds offline")
	}
	runner := toolrun.New()
	if _, ok := runner.Detect(context.Background(), "dotnet"); !ok {
		t.Skip("dotnet is not on PATH")
	}
	helper := builtHelper(t)
	t.Setenv(helperEnv, helper)
	t.Setenv("DOTNET_CLI_HOME", t.TempDir())
	t.Setenv("NUGET_PACKAGES", t.TempDir())
	t.Setenv("DOTNET_NOLOGO", "1")
	t.Setenv("DOTNET_CLI_TELEMETRY_OPTOUT", "1")

	root := copyTree(t, filepath.Join("..", "..", "testdata", "dotnet"))
	cfgPath := filepath.Join(root, ".archfit.yaml")
	if err := os.WriteFile(cfgPath, []byte("version: 2\nlanguages:\n  dotnet:\n    enabled: true\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	cfg, err := config.Load(context.Background(), cfgPath)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	exs := registry.Build(runner, cfg.ExtractConfigs(), nil)
	e := registry.DotNetExtractor(exs)
	if e == nil {
		t.Fatal("registry.Build built no .NET extractor")
	}
	facts, cov, err := e.Extract(context.Background(), scope.Scope{Root: root})
	if err != nil {
		t.Fatalf("Extract: %v", err)
	}

	if cov.Tool != registry.ToolDotNetMSBuild || cov.Status != evidence.StatusPartial || !strings.Contains(cov.Reason, "Archfit.Fixture.Missing.Sdk") {
		t.Errorf("dotnet-msbuild coverage = %+v, want partial naming Broken's SDK", cov)
	}
	rcov := e.LastTypeGraphCoverage()
	if rcov.Tool != registry.ToolRoslyn || rcov.Status != evidence.StatusPartial || rcov.UnresolvedInputsMissing == 0 {
		t.Errorf("roslyn coverage = %+v, want partial with missing inputs", rcov)
	}

	ev, tg := e.LastEvaluation(), e.LastTypeGraph()
	evStatus := map[string]string{}
	for _, p := range ev.Projects {
		evStatus[p.Path] = p.Status
	}
	tgStatus := map[string]string{}
	for _, p := range tg.Projects {
		tgStatus[p.Path] = p.Status
	}
	for path, want := range map[string]string{
		"Domain/Domain.csproj":                 dotnet.TypeGraphComplete,
		"Application/Application.csproj":       dotnet.TypeGraphComplete,
		"Infrastructure/Infrastructure.csproj": dotnet.TypeGraphComplete,
		brokenProj:                             dotnet.TypeGraphFailed,
		testsProj:                              dotnet.TypeGraphPartial,
		"Unresolved/Unresolved.csproj":         dotnet.TypeGraphPartial,
	} {
		if tgStatus[path] != want {
			t.Errorf("type graph %s = %q, want %q", path, tgStatus[path], want)
		}
	}
	if evStatus[brokenProj] != dotnet.ProjectFailed {
		t.Errorf("evaluation %s = %q, want failed", brokenProj, evStatus[brokenProj])
	}

	type hints struct{ hint, data string }
	got := map[string]hints{}
	for _, edge := range facts.Edges {
		if strings.HasPrefix(edge.From, "file:") {
			got[graph.NodePath(edge.From)+" -> "+graph.NodePath(edge.To)] = hints{edge.StrengthHint, edge.DataStrengthHint}
		}
	}
	for pair, want := range map[string]hints{
		"Infrastructure/Repository.cs -> Application/Contracts.cs": {hDTO, hDTO},
		"Infrastructure/Repository.cs -> Domain/Domain.cs":         {"intrusive", hModel},
		"Application/Contracts.cs -> Domain/Domain.cs":             {hModel, hModel},
	} {
		if got[pair] != want {
			t.Errorf("file edge %s = %+v, want %+v (all file edges: %v)", pair, got[pair], want, got)
		}
	}

	// Broken loads in neither pass: its project stays in the inventory, but no
	// file of it is, and it supplies no edge.
	for _, n := range facts.Nodes {
		if strings.HasPrefix(n.ID(), "file:Broken/") {
			t.Errorf("node %s: a failed project inventories no file", n.ID())
		}
	}
	for _, edge := range facts.Edges {
		from := graph.NodePath(edge.From)
		if strings.HasPrefix(from, "Broken/") || from == testsProj {
			t.Errorf("edge %s -> %s: Broken and Tests supply no edge", edge.From, edge.To)
		}
	}
	if !slices.Equal(tg.Fallback, []string{brokenProj}) {
		t.Errorf("fallback = %v, want only Broken (Tests is a test project)", tg.Fallback)
	}
}

// builtHelper returns ARCHFIT_DOTNET_ROSLYN when set, else the `make
// dotnet-roslyn` output, and skips when neither holds the helper.
func builtHelper(t *testing.T) string {
	t.Helper()
	dir := os.Getenv(helperEnv)
	if dir == "" {
		abs, err := filepath.Abs(filepath.Join("..", "..", "..", ".bin", "dotnet-roslyn"))
		if err != nil {
			t.Fatal(err)
		}
		dir = abs
	}
	if _, err := os.Stat(filepath.Join(dir, "archfit-roslyn.dll")); err != nil { //nolint:gosec // helper directory from the test environment
		t.Skipf("no built helper at %s; run make dotnet-roslyn", dir)
	}
	return dir
}

// copyTree copies src into a temp root so no run can write into the committed
// fixture.
func copyTree(t *testing.T, src string) string {
	t.Helper()
	root := t.TempDir()
	err := filepath.WalkDir(src, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, _ := filepath.Rel(src, p)
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
