package dotnet_test

import (
	"context"
	"path/filepath"
	"slices"
	"strings"
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
	if _, ok := runner.Detect(context.Background(), "dotnet"); !ok {
		t.Skip("dotnet is not on PATH")
	}
	t.Setenv("DOTNET_CLI_HOME", t.TempDir())
	t.Setenv("NUGET_PACKAGES", t.TempDir())
	t.Setenv("DOTNET_NOLOGO", "1")
	t.Setenv("DOTNET_CLI_TELEMETRY_OPTOUT", "1")
	return runner
}

var fixtureRoot = filepath.Join("..", "..", "testdata", "dotnet")

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
	if !slices.Equal(legacy.TargetFrameworks, []string{targetNet10, "net8.0"}) {
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
	if ev.Solution != "Fixture.slnx" || len(ev.Projects) != 7 {
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
