package jsonout

import (
	"encoding/json"
	"io"

	"github.com/alexei-led/archfit/internal/model/diagnostic"
	"github.com/alexei-led/archfit/internal/score"
)

// JSONRenderer marshals a Diagnostic plus its synthesised scorecard to JSON.
type JSONRenderer struct{}

// New returns a new JSONRenderer.
func New() *JSONRenderer {
	return &JSONRenderer{}
}

// Format returns "json".
func (r *JSONRenderer) Format() string {
	return "json"
}

// envelope flattens the Diagnostic at the top level (preserving the existing
// schema) and adds the synthesised scorecard, the hoisted coupling_balance
// dimension (the score's main driver), and — when a base scorecard is supplied —
// a version delta. Before this, an agent/CI consumer reading --json could not see
// the 0-100 score, its driver, or any delta; only text/markdown carried them (F5).
type envelope struct {
	diagnostic.Diagnostic
	Score           score.Scorecard  `json:"score"`
	CouplingBalance *score.Dimension `json:"coupling_balance,omitempty"`
	// ScoreDelta is the scorecard delta vs --base. Named distinctly from the
	// embedded Diagnostic's `delta` (findings lifecycle) to avoid a key collision.
	ScoreDelta *scoreDelta `json:"score_delta,omitempty"`
}

// scoreDelta is the per-dimension head-vs-base comparison emitted with --base.
type scoreDelta struct {
	BaseOverall  int              `json:"base_overall"`
	HeadOverall  int              `json:"head_overall"`
	OverallDelta int              `json:"overall_delta"`
	Dimensions   []dimensionDelta `json:"dimensions"`
}

type dimensionDelta struct {
	Name  string `json:"name"`
	Base  int    `json:"base"`
	Head  int    `json:"head"`
	Delta int    `json:"delta"`
}

// Render writes d plus its scorecard (and an optional delta vs base) as JSON.
// schema_version is part of the embedded Diagnostic and is always present.
func (r *JSONRenderer) Render(d diagnostic.Diagnostic, sc score.Scorecard, base *score.Scorecard, w io.Writer) error {
	env := envelope{
		Diagnostic:      d,
		Score:           sc,
		CouplingBalance: dimensionByName(sc, score.DimCouplingBalance),
	}
	if base != nil {
		env.ScoreDelta = buildDelta(sc, *base)
	}
	return json.NewEncoder(w).Encode(env)
}

// dimensionByName returns a copy of the named dimension, or nil when absent.
func dimensionByName(sc score.Scorecard, name string) *score.Dimension {
	for i := range sc.Dimensions {
		if sc.Dimensions[i].Name == name {
			d := sc.Dimensions[i]
			return &d
		}
	}
	return nil
}

// buildDelta pairs head and base dimensions by name and computes value deltas.
func buildDelta(head, base score.Scorecard) *scoreDelta {
	baseVal := make(map[string]int, len(base.Dimensions))
	for _, d := range base.Dimensions {
		baseVal[d.Name] = d.Value
	}
	dims := make([]dimensionDelta, 0, len(head.Dimensions))
	for _, d := range head.Dimensions {
		b := baseVal[d.Name]
		dims = append(dims, dimensionDelta{Name: d.Name, Base: b, Head: d.Value, Delta: d.Value - b})
	}
	return &scoreDelta{
		BaseOverall:  base.Overall,
		HeadOverall:  head.Overall,
		OverallDelta: head.Overall - base.Overall,
		Dimensions:   dims,
	}
}
