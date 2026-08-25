package pipeline

import (
	"github.com/alexei-led/archfit/internal/model/graph"
	"github.com/alexei-led/archfit/internal/relationship/classify"
	"github.com/alexei-led/archfit/internal/relationship/coupling"
)

// RuleIDBCImbalanced is the stable Balanced-Coupling advisory rule ID.
const RuleIDBCImbalanced = "bc/imbalanced_coupling"

// ClassifyGraph augments module boundaries and runs relationship classification
// for review-only CLI flows. Staged analysis uses relationship/analysis.Analyze.
func ClassifyGraph(g *graph.Graph, cfg classify.Config) (classify.Config, coupling.Index) {
	cfg = AugmentClassifyConfig(g, cfg)
	return cfg, classify.Run(g, cfg)
}
