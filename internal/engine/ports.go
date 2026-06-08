package engine

import (
	"context"
	"io"

	"github.com/alexei-led/archfit/internal/model/diagnostic"
	"github.com/alexei-led/archfit/internal/model/graph"
	"github.com/alexei-led/archfit/internal/scope"
)

//go:generate moq -out extractor_moq.go . Extractor

// Extractor is the port that language-specific adapters satisfy.
// Each adapter runs a single-language extraction pipeline and returns
// raw facts plus a coverage record. The engine defines this interface
// (consumer-owns-interface principle); adapters in extract/* implement it.
type Extractor interface {
	// Name returns the language/tool name (e.g. "go", "typescript", "python").
	Name() string

	// Extract runs the extractor for the given scope and returns raw graph facts,
	// a coverage record, and any hard error. A missing toolchain must not return
	// an error — it returns an empty Facts and a coverage record with status="unavailable".
	Extract(ctx context.Context, s scope.Scope) (graph.Facts, diagnostic.Coverage, error)
}

// Renderer is the port that output adapters satisfy.
// Each adapter formats and writes a completed Diagnostic to a writer.
type Renderer interface {
	// Format returns the format name (e.g. "json", "console").
	Format() string

	// Render writes d to w in the adapter's format.
	Render(d diagnostic.Diagnostic, w io.Writer) error
}
