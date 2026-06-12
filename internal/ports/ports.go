// Package ports defines the hexagonal port interfaces that separate the engine
// orchestrator from its adapters. Both the engine (consumer) and adapters
// (producers/implementors) import this neutral package — neither imports the
// other.
package ports

import (
	"context"
	"io"

	"github.com/alexei-led/archfit/internal/config"
	"github.com/alexei-led/archfit/internal/model/diagnostic"
	"github.com/alexei-led/archfit/internal/model/graph"
	"github.com/alexei-led/archfit/internal/model/pattern"
	"github.com/alexei-led/archfit/internal/model/symbol"
	"github.com/alexei-led/archfit/internal/scope"
)

// statusAbsent is the coverage status returned by no-op ports when the tool is not present.
const statusAbsent = "absent"

//go:generate moq -out extractor_moq.go . Extractor
//go:generate moq -out pattern_provider_moq.go . PatternProvider
//go:generate moq -out symbol_resolver_moq.go . SymbolResolver

// Extractor is the port that language-specific adapters satisfy.
// Each adapter runs a single-language extraction pipeline and returns
// raw facts plus a coverage record. The engine consumes this interface;
// adapters in extract/* implement it.
type Extractor interface {
	// Name returns the language/tool name (e.g. "go", "typescript", "python").
	Name() string

	// Extract runs the extractor for the given scope and returns raw graph facts,
	// a coverage record, and any hard error. A missing toolchain must not return
	// an error — it returns an empty Facts and a coverage record with status="unavailable".
	Extract(ctx context.Context, s scope.Scope) (graph.Facts, diagnostic.Coverage, error)
}

// PatternProvider is the port for structural pattern search (Phase 3: ast-grep).
// The engine consumes this interface; adapters in extract/astgrep implement it.
//
//go:generate moq -out pattern_provider_moq.go . PatternProvider
type PatternProvider interface {
	// Name returns the tool name (e.g. "ast-grep").
	Name() string

	// Find runs all patterns in c against the given scope and returns matches,
	// a coverage record, and any hard error. A missing tool must not return an
	// error — it returns empty matches and a coverage record with status="absent".
	Find(ctx context.Context, s scope.Scope, c config.PatternConfig) ([]pattern.Match, diagnostic.Coverage, error)
}

// SymbolResolver is the port for barrel-file / re-export resolution (Phase 3: SCIP).
// The engine consumes this interface; adapters in extract/scip implement it.
//
//go:generate moq -out symbol_resolver_moq.go . SymbolResolver
type SymbolResolver interface {
	// Name returns the tool name (e.g. "scip").
	Name() string

	// Resolve maps a logical import path (toPath) seen from fromFile to the real
	// source file path. Returns realPath and a confidence string ("high"/"medium").
	// When no re-export map is available it returns toPath unchanged with confidence "high".
	Resolve(ctx context.Context, fromFile, toPath string) (realPath, confidence string)

	// Strengths returns symbol-level Balanced-Coupling integration strength for
	// cross-module edges, keyed by "<fromModulePath>\x00<toModulePath>" (paths in
	// the same dotted/slash form as graph node paths, kind prefix stripped). Values
	// are coupling strengths ("contract"/"model"/"functional"/"intrusive"). A missing
	// tool returns an empty map and status="absent" coverage — never an error.
	Strengths(ctx context.Context, s scope.Scope) (map[string]string, diagnostic.Coverage, error)

	// Symbols returns per-symbol module ownership, fan-in, and cross-module
	// reference edges from a SCIP index. A missing tool or any non-fatal failure
	// returns an empty Graph and absent/partial coverage — never an error.
	//
	// TODO(perf): Strengths and Symbols each run the indexer separately. Merge into
	// a single indexer+reader pass so enabling scip does not double the index time.
	Symbols(ctx context.Context, s scope.Scope) (symbol.Graph, diagnostic.Coverage, error)
}

// NopPatternProvider is a no-op PatternProvider used when no Phase 3 tools are
// present. Find returns empty matches and a coverage record with status="absent".
type NopPatternProvider struct{}

var _ PatternProvider = NopPatternProvider{}

// Name returns "nop-pattern".
func (NopPatternProvider) Name() string { return "nop-pattern" }

// Find returns empty matches and an absent coverage record.
func (NopPatternProvider) Find(_ context.Context, _ scope.Scope, _ config.PatternConfig) ([]pattern.Match, diagnostic.Coverage, error) {
	return nil, diagnostic.Coverage{Tool: "ast-grep", Status: statusAbsent}, nil
}

// NopSymbolResolver is a no-op SymbolResolver used when no Phase 3 tools are
// present. Resolve returns the input path unchanged with confidence "high".
type NopSymbolResolver struct{}

var _ SymbolResolver = NopSymbolResolver{}

// Name returns "nop-resolver".
func (NopSymbolResolver) Name() string { return "nop-resolver" }

// Resolve returns toPath unchanged with confidence "high".
func (NopSymbolResolver) Resolve(_ context.Context, _, toPath string) (string, string) {
	return toPath, "high"
}

// Strengths returns an empty map and an absent coverage record.
func (NopSymbolResolver) Strengths(_ context.Context, _ scope.Scope) (map[string]string, diagnostic.Coverage, error) {
	return nil, diagnostic.Coverage{Tool: "scip", Status: statusAbsent}, nil
}

// Symbols returns an empty Graph and an absent coverage record.
func (NopSymbolResolver) Symbols(_ context.Context, _ scope.Scope) (symbol.Graph, diagnostic.Coverage, error) {
	return symbol.Graph{}, diagnostic.Coverage{Tool: "scip-symbols", Status: statusAbsent}, nil
}

// Renderer is the port that output adapters satisfy.
// Each adapter formats and writes a completed Diagnostic to a writer.
type Renderer interface {
	// Format returns the format name (e.g. "json", "console").
	Format() string

	// Render writes d to w in the adapter's format.
	Render(d diagnostic.Diagnostic, w io.Writer) error
}
