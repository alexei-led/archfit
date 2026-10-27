package initcfg

import (
	"context"
	"reflect"
	"strings"
	"testing"

	"github.com/alexei-led/archfit/v3/internal/toolrun"
)

const (
	dotnetCoreProject = "src/Core/Core.csproj"
	dotnetAPIProject  = "src/Api/Api.csproj"
	dotnetTestProject = "test/Core.Test/Core.Test.csproj"
)

func TestDiscoverDotNet_OneModulePerProjectDirectory(t *testing.T) {
	tests := []struct {
		name     string
		projects []string
		want     []ModuleDef
	}{
		{name: "none", want: []ModuleDef{}},
		{
			name:     "one module per directory, sorted",
			projects: []string{dotnetCoreProject, dotnetAPIProject},
			want: []ModuleDef{
				{Name: "Api", Paths: []string{"src/Api/**"}, Sources: []string{dotnetAPIProject}},
				{Name: "Core", Paths: []string{"src/Core/**"}, Sources: []string{dotnetCoreProject}},
			},
		},
		{
			name:     "two projects in one directory share a module",
			projects: []string{"src/Tool/Tool.csproj", "src/Tool/Tool.Legacy.csproj"},
			want: []ModuleDef{
				{Name: "Tool.Legacy", Paths: []string{"src/Tool/**"}, Sources: []string{"src/Tool/Tool.Legacy.csproj", "src/Tool/Tool.csproj"}},
			},
		},
		{
			name:     "a root project owns the tree",
			projects: []string{"App.csproj"},
			want:     []ModuleDef{{Name: "App", Paths: []string{"**"}, Sources: []string{"App.csproj"}}},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := discoverDotNet(tt.projects); !reflect.DeepEqual(got, tt.want) {
				t.Errorf("discoverDotNet = %+v, want %+v", got, tt.want)
			}
		})
	}
}

// TestDiscover_DotNetRunsNoToolAndDropsTestProjects: discovery reads the
// extractor's project list, runs no subprocess, and proposes no module for a
// test project, which owns no production source.
func TestDiscover_DotNetRunsNoToolAndDropsTestProjects(t *testing.T) {
	runner := &toolrun.RunnerMock{RunFunc: func(_ context.Context, cmd toolrun.ToolCmd) (toolrun.Output, error) {
		t.Errorf("unexpected subprocess %s %v", cmd.Name, cmd.Args)
		return toolrun.Output{}, nil
	}}
	presence := Presence{
		DotNet:         true,
		DotNetProjects: []string{dotnetCoreProject, dotnetAPIProject, dotnetTestProject},
		Sources: []SourceFile{
			{Path: "src/Core/User.cs", Language: langDotNet, Selector: "src/Core/User.cs", Production: true},
			{Path: "src/Api/Program.cs", Language: langDotNet, Selector: "src/Api/Program.cs", Production: true},
			{Path: "test/Core.Test/UserTests.cs", Language: langDotNet, Selector: "test/Core.Test/UserTests.cs"},
		},
	}
	cfg, err := Discover(context.Background(), t.TempDir(), runner, presence)
	if err != nil {
		t.Fatalf("Discover: %v", err)
	}
	names := make([]string, 0, len(cfg.Modules))
	for _, m := range cfg.Modules {
		names = append(names, m.Name)
	}
	if strings.Join(names, ",") != "Api,Core" {
		t.Errorf("modules = %v, want [Api Core]", names)
	}
	if !cfg.HasDotNet || cfg.ImportGraphComplete || !strings.Contains(cfg.GraphGap, gapDotNet) {
		t.Errorf("HasDotNet = %v, ImportGraphComplete = %v, GraphGap = %q; want present, incomplete, naming the .NET gap",
			cfg.HasDotNet, cfg.ImportGraphComplete, cfg.GraphGap)
	}
}

// TestRender_DotNetToolMode: a present .NET project stays at enabled: auto (a
// missing SDK reports a coverage gap) and writes the Roslyn analyzer
// explicitly, sharing one analyzers: block with Rust.
func TestRender_DotNetToolMode(t *testing.T) {
	on := Render(DiscoveredConfig{HasDotNet: true}, nil, false)
	for _, want := range []string{"  dotnet:\n    enabled: auto\n", "analyzers:\n  roslyn:\n    enabled: true\n"} {
		if !strings.Contains(on, want) {
			t.Errorf("HasDotNet=true should emit %q; got:\n%s", want, on)
		}
	}
	off := Render(DiscoveredConfig{}, nil, false)
	if !strings.Contains(off, "  dotnet:\n    enabled: auto\n") {
		t.Errorf("HasDotNet=false should emit languages.dotnet enabled auto; got:\n%s", off)
	}
	if strings.Contains(off, "roslyn:") {
		t.Errorf("HasDotNet=false should not write analyzers.roslyn; got:\n%s", off)
	}
	both := Render(DiscoveredConfig{HasRust: true, HasDotNet: true}, nil, false)
	if n := strings.Count(both, "\nanalyzers:\n"); n != 1 {
		t.Errorf("Rust and .NET wrote %d analyzers: blocks, want 1:\n%s", n, both)
	}
	if !strings.Contains(both, "    enabled: true\n  roslyn:\n    enabled: true\n") {
		t.Errorf("Rust and .NET should share the analyzers block; got:\n%s", both)
	}
}
