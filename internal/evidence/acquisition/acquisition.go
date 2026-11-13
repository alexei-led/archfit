// Package acquisition owns evidence acquisition and graph construction.
package acquisition

import (
	"context"
	"errors"
	"fmt"

	evidenceports "github.com/alexei-led/archfit/v3/internal/evidence/ports"
	"github.com/alexei-led/archfit/v3/internal/model/evidence"
	"github.com/alexei-led/archfit/v3/internal/model/graph"
	"github.com/alexei-led/archfit/v3/internal/model/symbol"
	"github.com/alexei-led/archfit/v3/internal/scope"
)

// Input is the narrow contract for acquiring relationship evidence before the
// relationship and assessment stages run. Graph is produced here and crosses
// the stage boundary only as the immutable acquisition output.
type Input struct {
	Scope         scope.Scope
	Extractors    []evidenceports.Extractor
	Resolver      evidenceports.SymbolResolver
	ExtraCoverage []evidence.Coverage
}

// Result is the acquired dependency graph plus analyzer coverage and symbol
// evidence needed by later relationship and assessment stages.
type Result struct {
	Graph       *graph.Graph
	Coverages   []evidence.Coverage
	SCIPSymbols symbol.Graph
}

type connascenceResolver interface {
	Connascence(context.Context, scope.Scope) (map[string][]graph.ConnascenceHint, evidence.Coverage, error)
}

type dataStrengthResolver interface {
	DataStrengths(context.Context, scope.Scope) (map[string]string, evidence.Coverage, error)
}

// Collect runs symbol resolution, extractor acquisition, semantic edge
// enrichment, graph build, and coverage collation.
func Collect(ctx context.Context, in Input) (Result, error) {
	var coverages []evidence.Coverage

	scipStrength, scipCov, _ := in.Resolver.Strengths(ctx, in.Scope)
	if scipCov.Tool != "" {
		coverages = append(coverages, scipCov)
	}

	var scipConnascence map[string][]graph.ConnascenceHint
	if cr, ok := in.Resolver.(connascenceResolver); ok {
		scipConnascence, _, _ = cr.Connascence(ctx, in.Scope)
	}

	var scipData map[string]string
	if dr, ok := in.Resolver.(dataStrengthResolver); ok {
		scipData, _, _ = dr.DataStrengths(ctx, in.Scope)
	}

	scipSymbols, scipSymCov, _ := in.Resolver.Symbols(ctx, in.Scope)
	if scipSymCov.Tool != "" {
		coverages = append(coverages, scipSymCov)
	}

	var allFacts []graph.Facts
	var extractErrs []error
	for _, ex := range in.Extractors {
		f, cov, err := ex.Extract(ctx, in.Scope)
		if err != nil {
			coverages = append(coverages, evidence.Coverage{
				Tool:   ex.CoverageTool(),
				Status: evidence.StatusPartial,
				Reason: err.Error(),
			})
			extractErrs = append(extractErrs, err)
			continue
		}
		enrichEdges(ctx, in.Resolver, scipStrength, scipData, scipConnascence, f)
		allFacts = append(allFacts, f)
		coverages = append(coverages, cov)
	}
	if len(in.Extractors) > 0 && len(extractErrs) == len(in.Extractors) {
		return Result{}, fmt.Errorf("evidence acquisition: all %d extractor(s) failed: %w", len(in.Extractors), errors.Join(extractErrs...))
	}
	g := graph.Build(allFacts)
	coverages = append(coverages, in.ExtraCoverage...)
	return Result{Graph: g, Coverages: coverages, SCIPSymbols: scipSymbols}, nil
}

func enrichEdges(ctx context.Context, sr evidenceports.SymbolResolver, scipStrength, scipData map[string]string, scipConnascence map[string][]graph.ConnascenceHint, facts graph.Facts) {
	for i, e := range facts.Edges {
		fromFile := stripPrefix(e.From)
		toPath := stripPrefix(e.To)
		realPath, _ := sr.Resolve(ctx, fromFile, toPath)
		if realPath != toPath {
			prefix := e.To[:len(e.To)-len(toPath)]
			facts.Edges[i].To = prefix + realPath
		}
		key := fromFile + "\x00" + toPath
		if hints := scipConnascence[key]; len(hints) > 0 {
			facts.Edges[i].ConnascenceHints = appendGraphConnascenceHints(facts.Edges[i].ConnascenceHints, hints...)
		}
		if e.Language == graph.LangGo && e.StrengthHint != "" {
			continue
		}
		st, found := scipStrength[key]
		if found {
			facts.Edges[i].StrengthHint = st
		}
		if ds := scipData[key]; ds != "" {
			facts.Edges[i].DataStrengthHint = ds
		}
	}
}

func appendGraphConnascenceHints(dst []graph.ConnascenceHint, hints ...graph.ConnascenceHint) []graph.ConnascenceHint {
	seen := make(map[graph.ConnascenceHint]struct{}, len(dst)+len(hints))
	for _, h := range dst {
		seen[h] = struct{}{}
	}
	for _, h := range hints {
		if _, ok := seen[h]; ok {
			continue
		}
		seen[h] = struct{}{}
		dst = append(dst, h)
	}
	return dst
}

func stripPrefix(id string) string {
	for i := 0; i < len(id); i++ {
		if id[i] == ':' {
			return id[i+1:]
		}
	}
	return id
}
