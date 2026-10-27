package dotnet_test

import (
	"context"
	"maps"
	"os"
	"strings"
	"testing"

	"github.com/alexei-led/archfit/v3/internal/extract/dotnet"
	"github.com/alexei-led/archfit/v3/internal/scope"
	"github.com/alexei-led/archfit/v3/internal/toolrun"
)

const (
	globalUsingsCS = "Application/GlobalUsings.cs"
	markerCS       = "Shared/Marker.cs"
	brokenCS       = "Broken/Broken.cs"
	useCS          = "Unresolved/Use.cs"
)

// gapFiles are the fixture's walked files, plus the solution and a C# file no
// project compiles.
var gapFiles = []string{
	contractsCS, mapperCS, globalUsingsCS, brokenCS, domainCS,
	repositoryCS, legacyCS, markerCS, "Tests/ThingTests.cs", useCS,
	"Fixture.slnx", "Stray/Loose.cs", // a file no project compiles
}

// TestLastDependencyGaps pins which files lack complete Roslyn file evidence,
// the project gap each carries and the fix of each gap: a partial project's
// files, a project on ProjectReference fallback, and a project whose
// evaluation failed. Complete projects, test projects and files no project
// compiles carry no gap.
func TestLastDependencyGaps(t *testing.T) {
	const (
		sdkFix     = "MSB4236: SDK 'Archfit.Fixture.Missing.Sdk' not resolvable offline; run with NuGet access and a warm cache"
		restoreFix = "restore the projects with NuGet access (`dotnet restore`); the roslyn row names each project's cause"
	)
	brokenGap := brokenProj + " failed MSBuild evaluation"
	appGap, legacyGap, useGap := appProj+" partial", legacyProj+" partial", unresolvedProj+" partial"
	tests := []struct {
		name      string
		setup     func(t *testing.T, fake *fakeDotnet)
		wantFiles map[string]string
		wantFixes map[string]string // gap -> fix prefix
	}{
		{
			name:      "recorded report",
			setup:     func(*testing.T, *fakeDotnet) {},
			wantFiles: map[string]string{brokenCS: brokenGap, legacyCS: legacyGap, useCS: useGap},
			wantFixes: map[string]string{brokenGap: sdkFix, legacyGap: restoreFix, useGap: restoreFix},
		},
		{
			name: "a linked file belongs to the project that compiles it",
			setup: func(t *testing.T, fake *fakeDotnet) {
				fake.helper = fakeHelper(replying(editReport(t, func(ps []map[string]any) []map[string]any {
					for _, p := range ps {
						if p[keyPath] == appProj {
							p["status"], p["reason"], p["unresolved_symbols"] = "partial", "2 unresolved symbols", 2
						}
					}
					return ps
				})))
			},
			wantFiles: map[string]string{
				contractsCS: appGap, mapperCS: appGap, globalUsingsCS: appGap, markerCS: appGap,
				brokenCS: brokenGap, legacyCS: legacyGap, useCS: useGap,
			},
			wantFixes: map[string]string{appGap: restoreFix},
		},
		{
			name: "helper run without a report",
			setup: func(t *testing.T, fake *fakeDotnet) {
				fake.helper = func(_ context.Context, cmd toolrun.ToolCmd) (toolrun.Output, error) {
					return toolrun.Output{Stderr: []byte(fixture(t, cmd.WorkDir, "roslyn_globaljson.stderr")), ExitCode: 1}, nil
				}
			},
			wantFiles: fallbackGaps(brokenGap),
			wantFixes: map[string]string{
				domainProj + edgesOnly: "archfit-roslyn failed: global.json requests SDK 9.0.100", brokenGap: sdkFix,
			},
		},
		{
			name: "helper not installed",
			setup: func(*testing.T, *fakeDotnet) {
				_ = os.Unsetenv(helperEnv)
			},
			wantFiles: fallbackGaps(brokenGap),
			wantFixes: map[string]string{domainProj + edgesOnly: "ARCHFIT_DOTNET_ROSLYN is not set"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			root := copyFixture(t)
			installHelper(t)
			fake := newFake(t, root, fixtureReplies(t, root))
			fake.helper = fakeHelper(replying(recordedReport(t)))
			tt.setup(t, fake)
			e := dotnet.New(fake, autoCfg(), nil)
			if _, _, err := e.Extract(context.Background(), scope.Scope{Root: root}); err != nil {
				t.Fatal(err)
			}
			gaps, attributed := e.LastDependencyGaps(gapFiles)
			if !attributed {
				t.Fatal("gaps not attributed to projects")
			}
			if !maps.Equal(gaps.Files, tt.wantFiles) {
				t.Errorf("gap files = %v, want %v", gaps.Files, tt.wantFiles)
			}
			for gap, prefix := range tt.wantFixes {
				if !strings.HasPrefix(gaps.Fixes[gap], prefix) {
					t.Errorf("fix of %q = %q, want prefix %q", gap, gaps.Fixes[gap], prefix)
				}
			}
		})
	}
}

// edgesOnly ends the gap of a project a run without a helper report left on
// its ProjectReference edges.
const edgesOnly = " has project edges only"

// fallbackGaps are the gaps of a run without a helper report: every non-test
// project with an evaluation has project edges only; Broken failed evaluation.
func fallbackGaps(brokenGap string) map[string]string {
	out := map[string]string{brokenCS: brokenGap}
	for file, project := range map[string]string{
		contractsCS: appProj, mapperCS: appProj, globalUsingsCS: appProj, domainCS: domainProj,
		repositoryCS: infraProj, legacyCS: legacyProj, useCS: unresolvedProj,
	} {
		out[file] = project + edgesOnly
	}
	return out
}

func TestLastDependencyGapsNotAttributed(t *testing.T) {
	t.Run("before Extract", func(t *testing.T) {
		if _, attributed := dotnet.New(newFake(t, t.TempDir(), nil), autoCfg(), nil).LastDependencyGaps(gapFiles); attributed {
			t.Error("attributed before any Extract")
		}
	})
	t.Run("a listed project is missing on disk", func(t *testing.T) {
		root := copyFixture(t)
		installHelper(t)
		if err := os.Remove(root + "/" + domainProj); err != nil {
			t.Fatal(err)
		}
		replies := fixtureReplies(t, root)
		delete(replies, domainProj)
		fake := newFake(t, root, replies)
		fake.helper = fakeHelper(replying(recordedReport(t)))
		e := dotnet.New(fake, autoCfg(), nil)
		if _, _, err := e.Extract(context.Background(), scope.Scope{Root: root}); err != nil {
			t.Fatal(err)
		}
		if gaps, attributed := e.LastDependencyGaps(gapFiles); attributed {
			t.Errorf("gaps %v attributed, want the coverage rows read whole: a listed project is missing", gaps)
		}
	})
}
