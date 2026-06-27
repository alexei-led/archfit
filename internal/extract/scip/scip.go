// Package scip implements the ports.SymbolResolver port using SCIP indexers.
//
// # SCIP Go bindings availability
//
// The github.com/sourcegraph/scip module (now github.com/scip-code/scip) ships
// its Go protobuf bindings as a workspace sub-module with a local replace
// directive, making them non-importable from external modules. As a result this
// adapter implements tool detection and subprocess invocation but does not parse
// the raw .scip protobuf output.
//
// # Resolution behaviour
//
//   - If none of scip-typescript, scip-python, scip-go, or rust-analyzer are present on PATH,
//     Resolve returns toPath unchanged with confidence "medium" (identity
//     resolver — safe for gate evaluation but signals missing toolchain).
//   - If a SCIP indexer is detected, Resolve still returns toPath unchanged
//     with confidence "medium". Full barrel-file resolution (confidence "high")
//     requires importable scip Go bindings; this will be wired once the
//     upstream sub-module is published as a standalone Go module.
//
// The adapter is safe to use as the SymbolResolver in all command call sites.
// It never returns an error for a missing tool.
package scip

import (
	"context"
	"sync"
	"time"

	"github.com/alexei-led/archfit/internal/ports"
	"github.com/alexei-led/archfit/internal/toolrun"
)

// toolName is the coverage/name identifier for this adapter (strength rows).
const toolName = "scip"

// toolNameSymbols is the coverage/name identifier for the symbol-graph row,
// distinct from "scip" (strength) so the two rows are distinguishable in output.
const toolNameSymbols = "scip-symbols"

// scipTools are the SCIP indexers checked in preference order.
var scipTools = []string{indexerTS, indexerPython, indexerGo, indexerRust}

// Adapter satisfies ports.SymbolResolver using SCIP indexers.
// It detects available SCIP tools once (via sync.Once) and, when any are
// present, is ready to run an indexer on first Resolve call.
type Adapter struct {
	runner  toolrun.Runner
	timeout time.Duration // per-analyzer outer watchdog; 0 = defaultTimeout in scip_strength.go

	once      sync.Once
	toolFound string // empty when no SCIP indexer detected

	// pipeCache memoizes the index+read pipeline per project root so Strengths
	// and Symbols share one indexer pass per run (see runSCIPPipeline).
	pipeMu    sync.Mutex
	pipeCache map[string]pipeCacheEntry
}

var _ ports.SymbolResolver = (*Adapter)(nil)

// New returns an Adapter backed by the given runner.
// timeout is the per-analyzer outer watchdog; 0 uses the built-in default (20 min).
func New(runner toolrun.Runner, timeout time.Duration) *Adapter {
	return &Adapter{runner: runner, timeout: timeout}
}

// Name returns the tool identifier.
func (a *Adapter) Name() string { return toolName }

// detect runs tool discovery once and caches the result.
func (a *Adapter) detect(ctx context.Context) {
	a.once.Do(func() {
		for _, t := range scipTools {
			if _, ok := a.runner.Detect(ctx, t); ok {
				a.toolFound = t
				return
			}
		}
	})
}

// Resolve maps toPath to a real source path.
//
// Three confidence states so callers can tell the situations apart:
//   - "low"    — no SCIP indexer on PATH: identity, no resolution capability.
//   - "medium" — indexer detected but barrel-file resolution is not yet
//     implemented (pending importable scip Go bindings): identity, documented
//     limitation.
//   - "high"   — reserved for actually resolved paths (future).
//
// Future: once the scip Go bindings are importable, this method should:
//  1. Lazily run the detected indexer with WorkDir derived from fromFile.
//  2. Parse the .scip output protobuf to build a re-export map.
//  3. Look up toPath in the map; return realPath+"high" when found.
func (a *Adapter) Resolve(ctx context.Context, _ /* fromFile */, toPath string) (realPath, confidence string) {
	a.detect(ctx)
	if a.toolFound == "" {
		return toPath, "low"
	}
	return toPath, "medium"
}
