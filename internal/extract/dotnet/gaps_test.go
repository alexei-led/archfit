package dotnet_test

import (
	"context"
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
// evaluation failed. A partial project's gap carries the reason the roslyn row
// states, so the rule reason names the real cause. Complete projects, test
// projects and files no project compiles carry no gap.
func TestLastDependencyGaps(t *testing.T) {
	const (
		sdkFix    = "MSB4236: SDK 'Archfit.Fixture.Missing.Sdk' not resolvable offline; run with NuGet access and a warm cache"
		legacyFix = "net8.0: 31 unresolved symbols (CS0246 x18, CS0400 x1, CS0518 x12); reference pack Microsoft.NETCore.App.Ref 8.0 is not installed"
		useFix    = "3 unresolved symbols (CS0246 x3); restore the project with NuGet access"
		appFix    = "2 unresolved symbols"
	)
	brokenGap := brokenProj + " failed MSBuild evaluation"
	appGap, legacyGap, useGap := appProj+" partial: ", legacyProj+" partial: ", unresolvedProj+" partial: "
	tests := []struct {
		name      string
		setup     func(t *testing.T, fake *fakeDotnet)
		wantFiles map[string]string // file -> gap prefix
		wantFixes map[string]string // gap prefix -> fix prefix
	}{
		{
			name:      "recorded report",
			setup:     func(*testing.T, *fakeDotnet) {},
			wantFiles: map[string]string{brokenCS: brokenGap, legacyCS: legacyGap + legacyFix, useCS: useGap + useFix},
			wantFixes: map[string]string{brokenGap: sdkFix},
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
				contractsCS: appGap + appFix, mapperCS: appGap + appFix, globalUsingsCS: appGap + appFix,
				markerCS: appGap + appFix, brokenCS: brokenGap, legacyCS: legacyGap + legacyFix, useCS: useGap + useFix,
			},
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
			if len(gaps.Files) != len(tt.wantFiles) {
				t.Errorf("gap files = %v, want %v", gaps.Files, tt.wantFiles)
			}
			for file, prefix := range tt.wantFiles {
				if !strings.HasPrefix(gaps.Files[file], prefix) {
					t.Errorf("gap of %q = %q, want prefix %q", file, gaps.Files[file], prefix)
				}
			}
			for gap, prefix := range tt.wantFixes {
				var matched bool
				for got, fix := range gaps.Fixes {
					if strings.HasPrefix(got, gap) {
						matched = strings.HasPrefix(fix, prefix)
						break
					}
				}
				if !matched {
					t.Errorf("fix of %q = %v, want prefix %q", gap, gaps.Fixes, prefix)
				}
			}
		})
	}
}

// restoreFix is the fix of a gap whose project stated no cause of its own.
const restoreFix = "restore the projects with NuGet access (`dotnet restore`); the roslyn row names each project's cause"

// Keys of a roslyn report project entry; keyPath is its file or project path.
const (
	keyTFM        = "tfm"
	keyStatus     = "status"
	keyReason     = "reason"
	keyUnresolved = "unresolved_symbols"
	keyIVT        = "ivt_declarations"
	keyFiles      = "files"
	keyEdges      = "edges"
)

// TestLastDependencyGapsHelperOnlyProject pins the gaps of a project the helper
// loads through ProjectReference and the MSBuild evaluation did not select: it
// has file edges like any evaluated project, so an incomplete one carries a gap
// on its files, and a required rule over them reads as unevaluated instead of
// passing on evidence nobody checked. The gap names the project's own cause, and
// a project whose file membership is unavailable is still attributed by its
// directory. A complete helper-only project, and a file any complete project
// holds, carry no gap.
func TestLastDependencyGapsHelperOnlyProject(t *testing.T) {
	const (
		helperProj    = "Helper/Helper.csproj"
		helperCS      = "Helper/Helper.cs"
		helperLooseCS = "Helper/Loose.cs" // under the project's directory, reported by no project
		helperReason  = "1 unresolved symbol (CS0246); restore the project with NuGet access"
		helperCause   = "Msbuild failed when processing the file 'Helper/Helper.csproj'"
	)
	// entry is a roslyn report entry for the helper-only project.
	entry := func(status, reason string, files ...string) map[string]any {
		reported := make([]any, 0, len(files))
		for _, f := range files {
			reported = append(reported, map[string]any{keyPath: f, keyEdges: []any{}})
		}
		return map[string]any{
			keyPath: helperProj, keyTFM: "net10.0", keyStatus: status, keyReason: reason,
			keyUnresolved: 1, keyIVT: []any{}, keyFiles: reported,
		}
	}
	tests := []struct {
		name      string
		project   map[string]any
		files     []string
		wantFiles map[string]string // file -> gap prefix
		wantFixes map[string]string // gap prefix -> fix prefix
	}{
		{
			name:    "partial project its files carry its cause",
			project: entry("partial", helperReason, helperCS),
			// The evaluated partial project's file keeps its own gap in the same run.
			files:     []string{helperCS, useCS},
			wantFiles: map[string]string{helperCS: helperProj + " partial: " + helperReason, useCS: unresolvedProj + " partial: "},
		},
		{
			name:      "partial project with no cause takes the restore fix",
			project:   entry("partial", "", helperCS),
			files:     []string{helperCS},
			wantFiles: map[string]string{helperCS: helperProj + " partial"},
			wantFixes: map[string]string{helperProj + " partial": restoreFix},
		},
		{
			name:      "failed project without file membership is attributed by directory",
			project:   entry("failed", helperCause),
			files:     []string{helperLooseCS},
			wantFiles: map[string]string{helperLooseCS: helperProj + " failed: " + helperCause},
		},
		{
			name:      "complete project carries no gap",
			project:   entry("complete", "", helperCS),
			files:     []string{helperCS, helperLooseCS},
			wantFiles: map[string]string{},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			root := copyFixture(t)
			installHelper(t)
			fake := newFake(t, root, fixtureReplies(t, root))
			fake.helper = fakeHelper(replying(editReport(t, func(ps []map[string]any) []map[string]any {
				return append(ps, tt.project)
			})))
			e := dotnet.New(fake, autoCfg(), nil)
			if _, _, err := e.Extract(context.Background(), scope.Scope{Root: root}); err != nil {
				t.Fatal(err)
			}
			gaps, attributed := e.LastDependencyGaps(tt.files)
			if !attributed {
				t.Fatal("gaps not attributed to projects")
			}
			if len(gaps.Files) != len(tt.wantFiles) {
				t.Errorf("gap files = %v, want %v", gaps.Files, tt.wantFiles)
			}
			for file, prefix := range tt.wantFiles {
				if !strings.HasPrefix(gaps.Files[file], prefix) {
					t.Errorf("gap of %q = %q, want prefix %q", file, gaps.Files[file], prefix)
				}
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
