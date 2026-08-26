package jsonout

import (
	"encoding/json"
	"io"

	"github.com/alexei-led/archfit/internal/model/report"
	reportports "github.com/alexei-led/archfit/internal/report/ports"
)

// StateRenderer marshals the architecture-state contract as the primary JSON
// output: archfit.architecture-state.v1 AT THE DOCUMENT ROOT, so a consumer
// reads .verdict, .decision, and .dimensions without unwrapping an envelope.
//
// It carries no repository scalar by construction — the state type has none —
// so a CI job cannot go on gating a 0-100 number through the primary format.
type StateRenderer struct{}

var _ reportports.Renderer = (*StateRenderer)(nil)

// NewState returns the primary architecture-state JSON renderer.
func NewState() *StateRenderer { return &StateRenderer{} }

// Format returns "json".
func (r *StateRenderer) Format() string { return "json" }

// Render writes the architecture state. Ordering is fixed by the contract's
// declaration order and by the pipeline that produced each list, so two runs
// over the same tree, config, and tool versions emit identical bytes.
func (r *StateRenderer) Render(d report.Document, w io.Writer) error {
	return json.NewEncoder(w).Encode(d.State)
}

// JSONRenderer marshals a report document plus its projected scorecard to JSON.
//
// This is the pre-cutover envelope, retained for exactly one release under the
// explicit `legacy-json` format name. It is the only output that still carries
// the repository scalar, and nothing in it reaches the verdict or the exit code.
type JSONRenderer struct{}

var _ reportports.Renderer = (*JSONRenderer)(nil)

// New returns the legacy JSON renderer.
func New() *JSONRenderer {
	return &JSONRenderer{}
}

// Format returns "legacy-json".
func (r *JSONRenderer) Format() string {
	return "legacy-json"
}

// envelope flattens the report document at the top level (preserving the existing
// schema) and adds the synthesised scorecard, the hoisted coupling_balance
// dimension (the score's main driver), and — when a base scorecard is supplied —
// a version delta. Before this, an agent/CI consumer reading --json could not see
// the 0-100 score, its driver, or any delta; only text/markdown carried them (F5).
type envelope struct {
	report.Document
	Score report.Scorecard `json:"score"`
	// ScoreVersion pins the BC score formula version (ordinals, severity
	// mapping). Consumers key on it: scores are not comparable across
	// versions. Always present — per-finding matched_by.score_version only
	// appears when a run produces BC advisories.
	ScoreVersion    string            `json:"score_version"`
	CouplingBalance *report.Dimension `json:"coupling_balance,omitempty"`
	// ScoreDelta is the scorecard delta vs --base. Named distinctly from the
	// embedded report document's `delta` (findings lifecycle) to avoid a key collision.
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

// Render writes d plus its projected scorecard (and an optional delta vs base) as JSON.
// schema_version is part of the embedded report document and is always present.
func (r *JSONRenderer) Render(d report.Document, w io.Writer) error {
	env := envelope{
		Document:        d,
		Score:           d.Score,
		ScoreVersion:    report.ScoreVersion,
		CouplingBalance: dimensionByName(d.Score, report.DimCouplingBalance),
	}
	if d.BaseScore != nil {
		env.ScoreDelta = buildDelta(d.Score, *d.BaseScore)
	}
	return json.NewEncoder(w).Encode(env)
}

// dimensionByName returns a copy of the named dimension, or nil when absent.
func dimensionByName(sc report.Scorecard, name string) *report.Dimension {
	for i := range sc.Dimensions {
		if sc.Dimensions[i].Name == name {
			d := sc.Dimensions[i]
			return &d
		}
	}
	return nil
}

// buildDelta pairs head and base dimensions by name and computes value deltas.
func buildDelta(head, base report.Scorecard) *scoreDelta {
	baseDim := make(map[string]report.Dimension, len(base.Dimensions))
	for _, d := range base.Dimensions {
		baseDim[d.Name] = d
	}
	dims := make([]dimensionDelta, 0, len(head.Dimensions))
	for _, d := range head.Dimensions {
		b, ok := baseDim[d.Name]
		if !ok {
			continue
		}
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
