package golang

import (
	"context"
	"go/build"
	"slices"
	"strings"
	"testing"

	evidenceports "github.com/alexei-led/archfit/v3/internal/evidence/ports"
	"github.com/alexei-led/archfit/v3/internal/model/graph"
	"github.com/alexei-led/archfit/v3/internal/scope"
)

// goFacts is one side of the parser-versus-typed-loader comparison, as the
// milestone 0 equivalence test compared them: file-level first-party
// dependencies, sources, and first-party imports neither side places.
type goFacts struct {
	sources, firstParty, unresolved []string
}

func parserFacts(p ParsedImports) goFacts {
	return goFacts{
		sources:    p.Sources,
		firstParty: pairs(p, ImportFirstParty),
		unresolved: pairs(p, ImportUnresolved),
	}
}

// typedLoaderFacts reads the same facts from the typed loader's graph. A
// package node is first-party; the root package has no node. An edge target
// with no node is external when it is "C", the standard library, or a module
// path (a dot in its first element), and unresolved otherwise.
func typedLoaderFacts(t *testing.T, facts graph.Facts) goFacts {
	t.Helper()
	nodes := map[string]bool{"package:": true}
	var out goFacts
	for _, n := range facts.Nodes {
		nodes[n.ID()] = true
		if n.Kind == graph.NodeKindFile {
			out.sources = append(out.sources, n.Path)
		}
	}
	for _, e := range facts.Edges {
		from := strings.TrimPrefix(e.From, "file:")
		to := strings.TrimPrefix(e.To, "package:")
		first, _, _ := strings.Cut(to, "/")
		switch {
		case nodes[e.To]:
			out.firstParty = append(out.firstParty, from+" -> "+to)
		case to == "C" || strings.Contains(first, ".") || standardLibrary(to):
		default:
			out.unresolved = append(out.unresolved, from+" -> "+to)
		}
	}
	for _, list := range []*[]string{&out.sources, &out.firstParty, &out.unresolved} {
		slices.Sort(*list)
		*list = slices.Compact(*list)
	}
	return out
}

func standardLibrary(importPath string) bool {
	pkg, err := build.Default.Import(importPath, "", build.FindOnly)
	return err == nil && pkg.Goroot
}

func typedLoad(t *testing.T, root string, cfg evidenceports.ExtractConfig) graph.Facts {
	t.Helper()
	facts, cov, err := New(cfg).Extract(context.Background(), scope.Scope{Root: root, GitRoot: root})
	if err != nil {
		t.Fatal(err)
	}
	if cov.Status == statusAbsent {
		t.Fatalf("typed loader did not run: %+v", cov)
	}
	return facts
}

// TestParserMatchesTypedLoader keeps the equivalence the corpus measured in
// milestone 0 as a regression check. The one difference is explained: the
// typed loader gives the standard library log the node of the first-party
// <root>/log package; the parser path calls it external.
func TestParserMatchesTypedLoader(t *testing.T) {
	if testing.Short() {
		t.Skip("loads packages with the Go toolchain")
	}
	for _, tc := range []struct {
		name          string
		files         map[string]string
		cfg           evidenceports.ExtractConfig
		onlyTypedEdge []string
	}{
		{name: "rules", files: parseFixture, cfg: parseConfig},
		{name: "standard log beside a first-party log", files: logFixture, onlyTypedEdge: []string{"a/a.go -> log"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			setGoEnv(t, "0")
			root := writeTree(t, tc.files)
			parsed := parserFacts(mustParse(t, root, tc.cfg))
			typed := typedLoaderFacts(t, typedLoad(t, root, tc.cfg))

			if !slices.Equal(parsed.sources, typed.sources) {
				t.Errorf("sources: parser %q, typed loader %q", parsed.sources, typed.sources)
			}
			if !slices.Equal(parsed.unresolved, typed.unresolved) {
				t.Errorf("unresolved: parser %q, typed loader %q", parsed.unresolved, typed.unresolved)
			}
			wantTyped := slices.Sorted(slices.Values(append(slices.Clone(parsed.firstParty), tc.onlyTypedEdge...)))
			if !slices.Equal(typed.firstParty, wantTyped) {
				t.Errorf("first-party: typed loader %q, want the parser's %q plus %q", typed.firstParty, parsed.firstParty, tc.onlyTypedEdge)
			}
		})
	}
}
