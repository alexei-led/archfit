package engine

import (
	"testing"
	"time"

	"github.com/alexei-led/archfit/internal/assessment/finding"
	"github.com/alexei-led/archfit/internal/baseline"
	"github.com/alexei-led/archfit/internal/model/graph"
	"github.com/alexei-led/archfit/internal/model/module"
	"github.com/alexei-led/archfit/internal/relationship/classify"
	"github.com/alexei-led/archfit/internal/view"
)

// TestGroupBCAdvisories_StatusSplitsTheGroup verifies that Status is part of
// groupBCAdvisories' grouping key: two bc/imbalanced_coupling advisories that
// share the same (fromModule, toModule, strength, distance, volatility) shape
// but differ in Status (new vs baseline) must NOT be folded into one rollup —
// a baseline-suppressed edge must never inflate a "new" rollup's count, and a
// new edge must never hide inside an already-accepted baseline rollup.
func TestGroupBCAdvisories_StatusSplitsTheGroup(t *testing.T) {
	modules := map[string]module.ModuleDef{
		"a": {Paths: []string{"pkg/x/**"}},
		"b": {Paths: []string{"pkg/y/**"}},
	}
	cfg := view.ClassifyConfig{Modules: modules, ModuleMap: module.BuildMap(modules)}
	edge := graph.Edge{
		From: "file:pkg/x/f.go", To: "file:pkg/y/g.go",
		Kind: graph.EdgeKindImports, Language: graph.LangGo,
		StrengthHint: "functional",
	}
	g := graph.Build([]graph.Facts{{Edges: []graph.Edge{edge}, Language: graph.LangGo}})
	idx := classify.Run(g, cfg)

	fnds := collectAdvisories(g, idx, cfg, nil, RunInput{Now: time.Now(), Accepted: baseline.Baseline{}, Waivers: view.WaiverSet{}})
	var newFinding finding.Finding
	for _, f := range fnds {
		if f.RuleID == RuleIDBCImbalanced {
			newFinding = f
			break
		}
	}
	if newFinding.ID == "" {
		t.Fatalf("test setup: no bc/imbalanced_coupling advisory produced: %+v", fnds)
	}

	baselineFinding := newFinding
	baselineFinding.ID = newFinding.ID + "-baseline-variant"
	baselineFinding.Status = finding.StatusBaseline

	got := groupBCAdvisories([]finding.Finding{newFinding, baselineFinding})
	if len(got) != 2 {
		t.Fatalf("groupBCAdvisories returned %d findings, want 2 (status must split the group): %+v", len(got), got)
	}
	statuses := map[finding.Status]bool{got[0].Status: true, got[1].Status: true}
	if !statuses[finding.StatusNew] || !statuses[finding.StatusBaseline] {
		t.Errorf("statuses = %v, want both new and baseline present, unmerged", statuses)
	}
}
