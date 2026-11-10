package main

import (
	"context"
	"fmt"
	"os/exec"
	"strings"

	"github.com/alexei-led/archfit/v3/internal/config"
	"github.com/alexei-led/archfit/v3/internal/extract/registry"
	"github.com/alexei-led/archfit/v3/internal/model/graph"
	"github.com/alexei-led/archfit/v3/internal/scope"
	"github.com/alexei-led/archfit/v3/internal/toolrun"
)

// reference runs today's extractor for lang over root, with the config's
// exclusions and no fact cache, and keeps its first-party edges.
func reference(ctx context.Context, lang, root string, cfg config.Config) (facts, error) {
	cfg = mergedExclusions(cfg)
	var std map[string]bool
	if lang == config.LangGo {
		var err error
		if std, err = goStdlib(ctx, root); err != nil {
			return facts{}, err
		}
	}
	ex := registry.New(lang, toolrun.New(), cfg.ForExtract(lang), nil)
	if ex == nil {
		return facts{}, fmt.Errorf("no extractor for language %q", lang)
	}
	got, cov, err := ex.Extract(ctx, scope.Scope{Root: root, GitRoot: root})
	if err != nil {
		return facts{}, err
	}
	out := facts{Language: lang, Side: "reference", Coverage: &coverage{
		Tool: cov.Tool, Version: cov.Version, Status: cov.Status, Reason: cov.Reason, Unresolved: cov.Unresolved,
	}}
	sourceKind := graph.NodeKindFile
	if lang == config.LangPython {
		sourceKind = graph.NodeKindModule
	}
	firstParty := make(map[string]bool)
	for _, n := range got.Nodes {
		if n.Kind == sourceKind {
			out.Sources = append(out.Sources, n.Path)
		}
		if n.Kind != graph.NodeKindExternal {
			firstParty[n.ID()] = true
		}
	}
	for _, e := range got.Edges {
		from, to := nodePath(e.From), nodePath(e.To)
		switch {
		case lang == config.LangPython && strings.HasPrefix(e.To, "module:"):
			// grimp's own edges are first-party. A low-confidence edge names a
			// first-party module that does not exist: today's unknown.
			if e.Confidence == "low" {
				out.Unknown = append(out.Unknown, unresolved{From: from, Specifier: to})
			} else {
				out.Edges = append(out.Edges, edge{From: from, To: to})
			}
		case firstParty[e.To], lang == config.LangGo && e.To == "package:":
			// The Go load emits no node for the root package; its import is
			// still first-party.
			out.Edges = append(out.Edges, edge{From: from, To: to})
		case lang == config.LangGo && firstPartyGoPath(to, std):
			// A stripped first-party import the load produced no package for.
			out.Unknown = append(out.Unknown, unresolved{From: from, Specifier: to})
		case lang == config.LangTypeScript && e.Confidence == "low":
			// dependency-cruiser could not resolve the specifier.
			out.Unknown = append(out.Unknown, unresolved{From: from, Specifier: to})
		default:
			out.External++
		}
	}
	out.finish()
	return out, nil
}

// nodePath drops the kind prefix of a graph node ID.
func nodePath(id string) string {
	_, p, _ := strings.Cut(id, ":")
	return p
}

// firstPartyGoPath reports whether a package path the Go load left without a
// node is a stripped first-party dir: not "C", not standard library, and not
// a module path (whose first element holds a dot).
func firstPartyGoPath(p string, std map[string]bool) bool {
	first, _, _ := strings.Cut(p, "/")
	return p != "C" && !std[p] && !strings.Contains(first, ".")
}

// goStdlib lists the standard-library packages of the toolchain the typed
// loader uses in root.
func goStdlib(ctx context.Context, root string) (map[string]bool, error) {
	cmd := exec.CommandContext(ctx, "go", "list", "std")
	cmd.Dir = root
	out, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("go list std: %w", err)
	}
	std := make(map[string]bool)
	for _, p := range strings.Fields(string(out)) {
		std[p] = true
	}
	return std, nil
}
