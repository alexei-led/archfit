package scip

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"

	"github.com/alexei-led/archfit/internal/model/diagnostic"
	"github.com/alexei-led/archfit/internal/model/symbol"
	"github.com/alexei-led/archfit/internal/scope"
	"github.com/alexei-led/archfit/internal/toolrun"
)

// Symbols runs a SCIP indexer over the project, reads the index, and returns
// a symbol.Graph with per-symbol module ownership, fan-in, and cross-module
// reference edges. A missing toolchain (no indexer, no uv) or any non-fatal
// failure yields an empty symbol.Graph with an absent/partial coverage record,
// never an error — symbol-graph enrichment is best-effort.
func (a *Adapter) Symbols(ctx context.Context, s scope.Scope) (symbol.Graph, diagnostic.Coverage, error) {
	empty := symbol.Graph{}
	absent := diagnostic.Coverage{Tool: toolNameSymbols, Status: statusAbsent}

	indexer, pkg, lang, ok := a.detectIndexer(ctx, s.Root)
	if !ok {
		return empty, absent, nil
	}
	if _, found := a.runner.Detect(ctx, "uv"); !found {
		return empty, absent, nil
	}

	tmp, err := os.MkdirTemp("", "archfit-scip-sym-")
	if err != nil {
		return empty, absent, nil
	}
	defer os.RemoveAll(tmp) //nolint:errcheck

	protoPath := filepath.Join(tmp, "scip.proto")
	readerPath := filepath.Join(tmp, "scip_reader.py")
	indexPath := filepath.Join(tmp, "index.scip")
	if os.WriteFile(protoPath, scipProtoSrc, 0o600) != nil ||
		os.WriteFile(readerPath, scipReaderSrc, 0o600) != nil {
		return empty, absent, nil
	}

	partial := diagnostic.Coverage{Tool: toolNameSymbols, Version: indexer, Status: statusPartial}

	idxOut, err := a.runner.Run(ctx, toolrun.ToolCmd{
		Name:    indexer,
		Args:    indexArgs(indexer, pkg, s.Root, indexPath),
		WorkDir: s.Root,
		Timeout: indexTimeout,
	})
	if err != nil || idxOut.ExitCode != 0 {
		return empty, partial, nil
	}
	if _, statErr := os.Stat(indexPath); statErr != nil {
		return empty, partial, nil
	}

	rdOut, err := a.runner.Run(ctx, toolrun.ToolCmd{
		Name:    "uv",
		Args:    []string{"run", readerPath, "--proto", protoPath, "--index", indexPath, "--package", pkg, "--lang", lang},
		WorkDir: tmp,
		Timeout: readerTimeout,
	})
	if err != nil || rdOut.ExitCode != 0 {
		return empty, partial, nil
	}

	g, perr := parseReaderSymbols(rdOut.Stdout)
	if perr != nil {
		return empty, partial, nil
	}
	return g, diagnostic.Coverage{
		Tool:            toolNameSymbols,
		Version:         indexer,
		FilesSeen:       len(g.Module),
		FilesApplicable: len(g.Module),
		Status:          statusOK,
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
