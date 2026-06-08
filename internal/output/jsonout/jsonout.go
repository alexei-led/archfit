package jsonout

import (
	"encoding/json"
	"io"

	"github.com/alexei-led/archfit/internal/model/diagnostic"
)

// JSONRenderer marshals a Diagnostic to JSON.
type JSONRenderer struct{}

// New returns a new JSONRenderer.
func New() *JSONRenderer {
	return &JSONRenderer{}
}

// Format returns "json".
func (r *JSONRenderer) Format() string {
	return "json"
}

// Render writes d to w as JSON. schema_version is part of the Diagnostic struct
// and is always present (set by diagnostic.New).
func (r *JSONRenderer) Render(d diagnostic.Diagnostic, w io.Writer) error {
	return json.NewEncoder(w).Encode(d)
}
