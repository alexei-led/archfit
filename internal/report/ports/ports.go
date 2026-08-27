// Package ports defines the rendering port that output adapters satisfy.
//
// This package owns only the Renderer port. The evidence ports (Extractor,
// PatternProvider, SymbolResolver, SyntaxProvider) live in
// internal/evidence/ports.
package ports

import (
	"io"

	"github.com/alexei-led/archfit/internal/model/report"
)

// Renderer is the port that output adapters satisfy.
// Each adapter formats and writes a completed report document to a writer.
type Renderer interface {
	// Format returns the format name (e.g. "json", "console").
	Format() string

	// Render writes d to w in the adapter's format.
	Render(d report.Document, w io.Writer) error
}
