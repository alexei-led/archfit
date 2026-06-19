// Package scorecard renders a Diagnostic as the architect skill's seven-dimension
// banded scorecard: an overall 0-100 value plus one block per dimension with its
// value, band, confidence, evidence references, and a one-line summary. The
// synthesis is delegated to internal/score; this package only formats it. Output
// is deterministic and reads well both as raw text and rendered Markdown.
package scorecard

import (
	"fmt"
	"io"
	"strings"

	"github.com/alexei-led/archfit/internal/model/diagnostic"
	"github.com/alexei-led/archfit/internal/score"
)

// Renderer formats a Diagnostic as a banded scorecard. Satisfies engine.Renderer.
type Renderer struct{}

// New returns a Renderer.
func New() *Renderer { return &Renderer{} }

// Format returns "scorecard".
func (r *Renderer) Format() string { return "scorecard" }

// Render synthesises and writes the banded scorecard for d to w.
func (r *Renderer) Render(d diagnostic.Diagnostic, w io.Writer) error {
	sc := score.Synthesize(d)

	var b strings.Builder
	b.WriteString("# archfit scorecard\n\n")
	fmt.Fprintf(&b, "**Rubric version:** %d\n", sc.RubricVersion)
	fmt.Fprintf(&b, "**Overall:** %d/100 (%s)\n", sc.Overall, sc.OverallBand)
	if d.ConfigHash != "" {
		fmt.Fprintf(&b, "**Config hash:** `%s`\n", d.ConfigHash)
	}

	b.WriteString("\n## Dimensions\n")
	for _, dim := range sc.Dimensions {
		meta := ""
		if dim.Meta {
			meta = " · meta (scores the review, not the architecture)"
		}
		fmt.Fprintf(&b, "\n### %s — %d/100 (%s) · confidence: %s%s\n",
			dim.Name, dim.Value, dim.Band, dim.Confidence, meta)
		if dim.Summary != "" {
			fmt.Fprintf(&b, "%s\n", dim.Summary)
		}
		for _, e := range dim.Evidence {
			fmt.Fprintf(&b, "- %s\n", e)
		}
	}

	_, err := io.WriteString(w, b.String())
	return err
}
