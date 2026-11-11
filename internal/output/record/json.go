package record

import (
	"encoding/json"
	"io"

	"github.com/alexei-led/archfit/v3/internal/model/result"
)

// ResultJSON writes a check result as the archfit.result.v1 document of
// archfit.result.schema.json. A nil list is written as an empty one: the
// contract has no null lists.
func ResultJSON(w io.Writer, r result.Result) error {
	if r.Errors == nil {
		r.Errors = []result.Error{}
	}
	if r.Unknown == nil {
		r.Unknown = []result.Unknown{}
	}
	return writeJSON(w, r)
}

// AnswerJSON writes the answer of `archfit can` as the archfit.can.v1
// document of archfit.result.schema.json.
func AnswerJSON(w io.Writer, a result.CanAnswer) error {
	return writeJSON(w, a)
}

// writeJSON writes v with two-space indent and a final newline. It does not
// escape <, > and &: a next step such as `archfit request -m "<why>"` stays
// readable in the file a script or the App reads.
func writeJSON(w io.Writer, v any) error {
	enc := json.NewEncoder(w)
	enc.SetEscapeHTML(false)
	enc.SetIndent("", "  ")
	return enc.Encode(v)
}
