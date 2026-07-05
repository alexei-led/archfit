package jsonout

import (
	"encoding/json"
	"io"

	"github.com/alexei-led/archfit/internal/model/coupling"
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
	Score score.Scorecard `json:"score"`
	// ScoreVersion pins the BC score formula version (ordinals, severity
	// mapping). Consumers key on it: scores are not comparable across
	// versions. Always present — per-finding matched_by.score_version only
	// appears when a run produces BC advisories.
	ScoreVersion    string           `json:"score_version"`
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
		ScoreVersion:    coupling.ScoreVersion,
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
	baseDim := make(map[string]score.Dimension, len(base.Dimensions))
	for _, d := range base.Dimensions {
		baseDim[d.Name] = d
	}
	dims := make([]dimensionDelta, 0, len(head.Dimensions))
	for _, d := range head.Dimensions {
		b := baseDim[d.Name]
		delta := d.Value - b.Value
		// An n/a side (coupling unmeasured) has no real value — suppress the numeric
		// delta so a measurement-status change is not reported as a score regression.
		if d.Band.Unmeasured() || b.Band.Unmeasured() {
			delta = 0
		}
		dims = append(dims, dimensionDelta{Name: d.Name, Base: b.Value, Head: d.Value, Delta: delta})
	}
	overallDelta := head.Overall - base.Overall
	if head.OverallBand.Unmeasured() || base.OverallBand.Unmeasured() {
		overallDelta = 0
	}
	return &scoreDelta{
		BaseOverall:  base.Overall,
		HeadOverall:  head.Overall,
		OverallDelta: overallDelta,
		Dimensions:   dims,
	}
}
