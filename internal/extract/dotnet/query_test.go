package dotnet

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	evidenceports "github.com/alexei-led/archfit/v3/internal/evidence/ports"
	"github.com/alexei-led/archfit/v3/internal/model/graph"
)

const (
	queryDomainFile = "src/Domain/Entities/TodoItem.cs"
	queryInfraFile  = "src/Infrastructure/Data/ApplicationDbContext.cs"
)

// queryTree writes the projects of a small solution under a temp root.
func queryTree(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	for _, project := range []string{
		"src/Domain/Domain.csproj", "src/Infrastructure/Infrastructure.csproj",
		"tests/Domain.UnitTests/Domain.UnitTests.csproj",
	} {
		p := filepath.Join(root, filepath.FromSlash(project))
		if err := os.MkdirAll(filepath.Dir(p), 0o750); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(`<Project Sdk="Microsoft.NET.Sdk" />`), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	for file, body := range map[string]string{
		"src/Infrastructure/Infrastructure.csproj": `<Project Sdk="Microsoft.NET.Sdk"><PropertyGroup><AssemblyName>CleanArchitecture.Infrastructure</AssemblyName></PropertyGroup></Project>`,
		"tests/Directory.Build.props":              `<Project><PropertyGroup><AssemblyName>Company.Tests</AssemblyName></PropertyGroup></Project>`,
	} {
		if err := os.WriteFile(filepath.Join(root, filepath.FromSlash(file)), []byte(body), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	return root
}

// TestQueryEdgeSpellsTheRoslynEdge pins the queried edge to the edge
// appendFileGraph emits for the same reference, so the rule pass keys a
// can-import finding as check keys the extracted one.
func TestQueryEdgeSpellsTheRoslynEdge(t *testing.T) {
	root := queryTree(t)
	internal := []string{"src/Infrastructure/Data/Configurations/**"}
	for _, tc := range []struct {
		name     string
		target   string
		external bool
	}{
		{name: "file in another project", target: queryInfraFile},
		{name: "file under an internal glob", target: "src/Infrastructure/Data/Configurations/TodoListConfiguration.cs"},
		{name: "dotted assembly", target: "Microsoft.EntityFrameworkCore", external: true},
		{name: "undotted assembly", target: "MediatR", external: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var extracted graph.Facts
			appendFileGraph(&extracted, typeGraphResult{files: map[string][]helperFile{
				"src/Domain/Domain.csproj": {{Path: queryDomainFile, Edges: []helperEdge{{To: tc.target, External: tc.external, Hint: "model"}}}},
			}}, nil, internal)
			queried, err := QueryEdge(root, evidenceports.ExtractConfig{Internal: internal}, queryDomainFile, tc.target)
			if err != nil {
				t.Fatalf("QueryEdge: %v", err)
			}
			want, got := extracted.Edges[0], queried.Edges[0]
			want.StrengthHint = ""
			if got.From != want.From || got.To != want.To || got.Kind != want.Kind ||
				got.Language != want.Language || got.Confidence != want.Confidence || len(got.Locations) != 0 {
				t.Fatalf("queried edge = %+v, want %+v", got, want)
			}
			if node := queried.Nodes[1]; node.ID() != want.To {
				t.Fatalf("target node %s, want %s", node.ID(), want.To)
			}
		})
	}
}

// TestQueryEdgeRefusesWhatCheckNeverReads pins the answers where the
// extractor emits no edge (ErrNotExtracted), where only MSBuild evaluation
// can answer (ErrNotDecidable), and malformed input (a plain error).
func TestQueryEdgeRefusesWhatCheckNeverReads(t *testing.T) {
	root := queryTree(t)
	for _, tc := range []struct {
		name         string
		cfg          evidenceports.ExtractConfig
		from, target string
		want         error // nil means a plain error
	}{
		{name: "excluded importer", cfg: evidenceports.ExtractConfig{Exclusions: []string{"src/Domain/Entities/**"}},
			from: queryDomainFile, target: queryInfraFile, want: evidenceports.ErrNotExtracted},
		{name: "excluded target", cfg: evidenceports.ExtractConfig{Exclusions: []string{"src/Infrastructure/**"}},
			from: queryDomainFile, target: queryInfraFile, want: evidenceports.ErrNotExtracted},
		{name: "generated file under obj", from: "src/Domain/obj/Debug/net10.0/GlobalUsings.g.cs", target: queryInfraFile, want: evidenceports.ErrNotExtracted},
		{name: "target outside the root", from: queryDomainFile, target: "../Other/Thing.cs", want: evidenceports.ErrNotExtracted},
		{name: "test project", from: "tests/Domain.UnitTests/TodoItemTests.cs", target: queryDomainFile, want: evidenceports.ErrNotExtracted},
		{name: "roslyn switched off", cfg: evidenceports.ExtractConfig{TypeGraphDisabled: true},
			from: queryDomainFile, target: queryInfraFile, want: evidenceports.ErrNotDecidable},
		{name: "file in no project directory", from: "tools/Script.cs", target: queryInfraFile, want: evidenceports.ErrNotDecidable},
		{name: "first-party project file name", from: queryDomainFile, target: "Infrastructure", want: evidenceports.ErrNotDecidable},
		{name: "first-party literal AssemblyName", from: queryDomainFile, target: "CleanArchitecture.Infrastructure", want: evidenceports.ErrNotDecidable},
		{name: "first-party AssemblyName from Directory.Build.props", from: queryDomainFile, target: "Company.Tests", want: evidenceports.ErrNotDecidable},
		{name: "importer is not C#", from: "src/Domain/README.md", target: queryInfraFile},
		{name: "target is a directory", from: queryDomainFile, target: "src/Infrastructure"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, err := QueryEdge(root, tc.cfg, tc.from, tc.target)
			switch {
			case err == nil:
				t.Fatal("QueryEdge succeeded, want an error")
			case tc.want != nil && !errors.Is(err, tc.want):
				t.Fatalf("err = %v, want %v", err, tc.want)
			case tc.want == nil && (errors.Is(err, evidenceports.ErrNotExtracted) || errors.Is(err, evidenceports.ErrNotDecidable)):
				t.Fatalf("err = %v, want a plain error", err)
			}
		})
	}
}

// TestQueryEdgeAbstainsOnAComputedAssemblyName pins an AssemblyName built
// from a property: any external target could be that first-party assembly.
func TestQueryEdgeAbstainsOnAComputedAssemblyName(t *testing.T) {
	root := queryTree(t)
	props := `<Project><PropertyGroup><AssemblyName>Company.$(MSBuildProjectName)</AssemblyName></PropertyGroup></Project>`
	if err := os.WriteFile(filepath.Join(root, "Directory.Build.props"), []byte(props), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := QueryEdge(root, evidenceports.ExtractConfig{}, queryDomainFile, "Microsoft.EntityFrameworkCore"); !errors.Is(err, evidenceports.ErrNotDecidable) {
		t.Fatalf("err = %v, want ErrNotDecidable", err)
	}
}

// TestOwningProjectPrefersTheNearestDirectory pins nested projects: a file
// belongs to the deepest project directory holding it, and a root project
// owns what no nested one does.
func TestOwningProjectPrefersTheNearestDirectory(t *testing.T) {
	const rootProject, coreProject = "App.csproj", "lib/Core/Core.csproj"
	projects := []string{rootProject, coreProject, "lib/Core/Plugins/Plugins.csproj"}
	for file, want := range map[string]string{
		"Program.cs":                  rootProject,
		"lib/Core/Order.cs":           coreProject,
		"lib/Core/Plugins/Plugin.cs":  "lib/Core/Plugins/Plugins.csproj",
		"lib/CoreExtras/Extension.cs": rootProject,
	} {
		if got := owningProject(projects, file); got != want {
			t.Errorf("owningProject(%s) = %q, want %q", file, got, want)
		}
	}
}
