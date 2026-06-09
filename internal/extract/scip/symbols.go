package scip

import (
	"context"
	"encoding/json"

	"github.com/alexei-led/archfit/internal/model/diagnostic"
	"github.com/alexei-led/archfit/internal/model/symbol"
	"github.com/alexei-led/archfit/internal/scope"
)

// Symbols runs a SCIP indexer over the project, reads the index, and returns
// a symbol.Graph with per-symbol module ownership, fan-in, and cross-module
// reference edges. A missing toolchain (no indexer, no uv) or any non-fatal
// failure yields an empty symbol.Graph with an absent/partial coverage record,
// never an error — symbol-graph enrichment is best-effort.
func (a *Adapter) Symbols(ctx context.Context, s scope.Scope) (symbol.Graph, diagnostic.Coverage, error) {
	empty := symbol.Graph{}
	ro, partial, ok := a.runSCIPPipeline(ctx, s.Root, toolNameSymbols)
	if !ok {
		return empty, partial, nil
	}
	g, perr := parseReaderSymbols(ro.raw)
	if perr != nil {
		return empty, partial, nil
	}
	return g, diagnostic.Coverage{
		Tool:            toolNameSymbols,
		Version:         ro.indexer,
		FilesSeen:       len(g.Module),
		FilesApplicable: len(g.Module),
		Status:          diagnostic.StatusOK,
	}, nil
}

// parseReaderSymbols parses scip_reader.py JSON into a symbol.Graph.
// A helper-reported error in the JSON fails the parse.
func parseReaderSymbols(stdout []byte) (symbol.Graph, error) {
	var ro readerOutput
	if err := json.Unmarshal(stdout, &ro); err != nil {
		return symbol.Graph{}, err
	}
	if ro.Error != "" {
		return symbol.Graph{}, errReader(ro.Error)
	}

	module := make(map[string]string, len(ro.Symbols))
	fanIn := make(map[string]int, len(ro.Symbols))
	for _, s := range ro.Symbols {
		module[s.Symbol] = s.Module
		fanIn[s.Symbol] = s.FanIn
	}

	refs := make(map[string]map[string]struct{}, len(ro.SymbolRefs))
	for _, r := range ro.SymbolRefs {
		if refs[r.FromSymbol] == nil {
			refs[r.FromSymbol] = make(map[string]struct{})
		}
		refs[r.FromSymbol][r.ToSymbol] = struct{}{}
	}

	return symbol.Graph{Module: module, FanIn: fanIn, Refs: refs}, nil
}
