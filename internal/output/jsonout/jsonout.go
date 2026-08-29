package jsonout

import (
	"encoding/json"
	"io"

	"github.com/alexei-led/archfit/internal/model/report"
	reportports "github.com/alexei-led/archfit/internal/report/ports"
)

// Renderer marshals the architecture-state contract as JSON at the document
// root, so consumers read .verdict, .decision, and .dimensions directly.
type Renderer struct{}

var _ reportports.Renderer = (*Renderer)(nil)

// New returns the architecture-state JSON renderer.
func New() *Renderer { return &Renderer{} }

// Format returns "json".
func (r *Renderer) Format() string { return "json" }

// Render writes the architecture state. Ordering is fixed by the contract and
// by the pipeline that produced each list.
func (r *Renderer) Render(d report.Document, w io.Writer) error {
	return json.NewEncoder(w).Encode(d.State)
}
