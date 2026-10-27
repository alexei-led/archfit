package dotnet

import (
	"fmt"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"slices"
	"strings"

	evidenceports "github.com/alexei-led/archfit/v3/internal/evidence/ports"
	"github.com/alexei-led/archfit/v3/internal/model/graph"
	"github.com/alexei-led/archfit/v3/internal/syntax"
)

// csExt is the C# source extension, the only file type the helper reports.
const csExt = ".cs"

// QueryEdge builds the facts `archfit policy can-import` judges: one use by
// the C# file from of target, spelled exactly as appendFileGraph spells a
// Roslyn edge — file node to file node with the same edge kind, or file node
// to an external:<assembly> node, and no location — so the rule pass keys its
// findings as it does on the extracted edge. target is a root-relative .cs
// file or an assembly name (MediatR.Contracts). It reads the project selection
// Extract uses; it runs no dotnet process.
//
// It wraps evidenceports.ErrNotExtracted where the extractor emits no edge: an
// excluded file or target, a file under obj/ or bin/ or outside the root, or
// a file of a test project. It wraps evidenceports.ErrNotDecidable where only
// MSBuild evaluation can answer: Roslyn switched off (check then reads
// ProjectReference edges), a file in no selected project's directory (it is
// compiled only through a linked Compile item, if at all), and a target that
// names a first-party project's assembly (Roslyn resolves those symbols to
// files, never to an external node) or might, because an AssemblyName is
// computed.
func QueryEdge(root string, cfg evidenceports.ExtractConfig, from, target string) (graph.Facts, error) {
	if !strings.EqualFold(path.Ext(from), csExt) {
		return graph.Facts{}, fmt.Errorf("dotnet: %q is not a .cs file", from)
	}
	if cfg.TypeGraphDisabled {
		return graph.Facts{}, fmt.Errorf("dotnet: analyzers.roslyn is off, so check reads ProjectReference edges, whose project membership needs MSBuild evaluation: %w", evidenceports.ErrNotDecidable)
	}
	external := !strings.Contains(target, "/") && !strings.EqualFold(path.Ext(target), csExt)
	if !external && !strings.EqualFold(path.Ext(target), csExt) {
		return graph.Facts{}, fmt.Errorf("dotnet: %q is neither a .cs file nor an assembly name", target)
	}
	for _, p := range [...]string{from, target} {
		if p == target && external {
			continue
		}
		switch {
		case escapesRoot(p):
			return graph.Facts{}, fmt.Errorf("dotnet: %s is outside the analysis root: %w", p, evidenceports.ErrNotExtracted)
		case excluded(cfg.Exclusions, p):
			return graph.Facts{}, fmt.Errorf("dotnet: %s matches an exclude: glob: %w", p, evidenceports.ErrNotExtracted)
		case syntax.InDotNetBuildDir(p):
			return graph.Facts{}, fmt.Errorf("dotnet: %s is build output under obj/ or bin/: %w", p, evidenceports.ErrNotExtracted)
		}
	}
	projects := Projects(root, cfg.Exclusions, cfg.DotNetProjects)
	project := owningProject(projects, from)
	switch {
	case project == "":
		return graph.Facts{}, fmt.Errorf("dotnet: %s is in no selected project's directory, so only a linked Compile item can compile it and MSBuild evaluation decides that: %w", from, evidenceports.ErrNotDecidable)
	case syntax.IsTestFile(graph.LangDotNet, project):
		return graph.Facts{}, fmt.Errorf("dotnet: %s belongs to the test project %s, whose edges are dropped: %w", from, project, evidenceports.ErrNotExtracted)
	}
	to := graph.Node{Kind: graph.NodeKindFile, Path: target, Language: graph.LangDotNet}
	kind := importEdgeKind(cfg.Internal, target)
	if external {
		if reason := firstPartyAssembly(root, projects, target); reason != "" {
			return graph.Facts{}, fmt.Errorf("dotnet: %s: %w", reason, evidenceports.ErrNotDecidable)
		}
		to.Kind, kind = graph.NodeKindExternal, graph.EdgeKindImports
	}
	return graph.Facts{
		Language: graph.LangDotNet,
		Nodes: []graph.Node{
			{Kind: graph.NodeKindFile, Path: from, Language: graph.LangDotNet},
			to,
		},
		Edges: []graph.Edge{{
			From:       graph.Node{Kind: graph.NodeKindFile, Path: from}.ID(),
			To:         to.ID(),
			Kind:       kind,
			Language:   graph.LangDotNet,
			Confidence: confidenceHigh,
		}},
	}, nil
}

// owningProject returns the selected project whose directory most closely
// contains file: the project the SDK's default Compile glob compiles it into.
func owningProject(projects []string, file string) string {
	best, depth := "", -1
	for _, project := range projects {
		dir := path.Dir(project)
		switch {
		case dir == "." && depth < 0:
			best, depth = project, 0
		case strings.HasPrefix(file, dir+"/") && len(dir) > depth:
			best, depth = project, len(dir)
		}
	}
	return best
}

// assemblyNameElement matches a literal <AssemblyName> property.
var assemblyNameElement = regexp.MustCompile(`<AssemblyName>([^<]*)</AssemblyName>`)

// firstPartyAssembly names why an external target cannot be answered without
// MSBuild evaluation: it is the assembly of a selected project, whose symbols
// Roslyn resolves to files, or a project's assembly name is computed so the
// target could be one. A project's assembly name is a literal <AssemblyName>
// in its project file or a Directory.Build.props above it, else its file name.
// "" means target names no first-party assembly.
func firstPartyAssembly(root string, projects []string, target string) string {
	for _, project := range projects {
		names := []string{strings.TrimSuffix(path.Base(project), path.Ext(project))}
		for _, file := range assemblyNameSources(root, project) {
			data, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(file))) // #nosec G304 -- a project file or Directory.Build.props under the root
			if err != nil {
				continue
			}
			for _, m := range assemblyNameElement.FindAllSubmatch(data, -1) {
				name := strings.TrimSpace(string(m[1]))
				if strings.Contains(name, "$(") {
					return file + " computes the AssemblyName of " + project + ", so whether " + target + " is first-party needs MSBuild evaluation"
				}
				names = append(names, name)
			}
		}
		if slices.ContainsFunc(names, func(name string) bool { return strings.EqualFold(name, target) }) {
			return target + " is the assembly of the first-party project " + project + "; Roslyn resolves its symbols to files, so name a .cs file of that project"
		}
	}
	return ""
}

// assemblyNameSources are the files that can set a project's AssemblyName
// without an import: the project file and every Directory.Build.props from
// its directory up to the root.
func assemblyNameSources(root, project string) []string {
	sources := []string{project}
	for dir := path.Dir(project); ; dir = path.Dir(dir) {
		props := path.Join(dir, "Directory.Build.props")
		if _, err := os.Stat(filepath.Join(root, filepath.FromSlash(props))); err == nil {
			sources = append(sources, props)
		}
		if dir == "." {
			return sources
		}
	}
}
