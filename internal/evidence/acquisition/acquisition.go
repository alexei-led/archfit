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

// Collect runs extractors, builds the graph, and collates coverage.
func Collect(ctx context.Context, in Input) (Result, error) {
	var coverages []evidence.Coverage

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
		allFacts = append(allFacts, f)
		coverages = append(coverages, cov)
	}
	if len(in.Extractors) > 0 && len(extractErrs) == len(in.Extractors) {
		return Result{}, fmt.Errorf("evidence acquisition: all %d extractor(s) failed: %w", len(in.Extractors), errors.Join(extractErrs...))
	}
	g := graph.Build(allFacts)
	coverages = append(coverages, in.ExtraCoverage...)
	return Result{Graph: g, Coverages: coverages}, nil
}
