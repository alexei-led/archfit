package modularity_test

import (
	"testing"

	"github.com/alexei-led/archfit/internal/metrics/metricstest"
	"github.com/alexei-led/archfit/internal/metrics/modularity"
	"github.com/alexei-led/archfit/internal/model/clone"
	"github.com/alexei-led/archfit/internal/model/graph"
	"github.com/alexei-led/archfit/internal/model/signal"
)

const (
	fcPathA  = "pkg/a/a.go"
	fcPathB  = "pkg/b/b.go"
	fcPathC  = "pkg/c/c.go"
	bandInfo = "info"
)

// twoGoNodes returns a minimal graph with two Go-file nodes and one import edge
// so dominantLanguage returns "go" and file→module mapping works correctly.
func twoGoNodes() *graph.Graph {
	nodes := []graph.Node{
		{Kind: graph.NodeKindFile, Path: fcPathA},
		{Kind: graph.NodeKindFile, Path: fcPathB},
	}
	edges := []graph.Edge{
		{From: "file:" + fcPathA, To: "file:" + fcPathB, Kind: graph.EdgeKindImports, Language: "go"},
	}
	return metricstest.BuildGraph(nodes, edges)
}

func TestFunctionalCandidates_Calculate(t *testing.T) {
	m := modularity.FunctionalCandidatesMetric{}

	// fcPathA = "pkg/a/a.go" → module "pkg/a"
	// fcPathB = "pkg/b/b.go" → module "pkg/b"
	// fcPathC = "pkg/c/c.go" → module "pkg/c"

	tests := []struct {
		name        string
		clusters    []clone.Cluster
		coChange    map[[2]string]int
		g           *graph.Graph
		wantBand    string
		wantValue   float64
		wantDisplay string // empty means "don't check"
	}{
		{
			name:      "no clone data → n/a",
			clusters:  nil,
			g:         twoGoNodes(),
			wantBand:  bandNAStr,
			wantValue: 0,
		},
		{
			name:      "nil graph → n/a",
			clusters:  []clone.Cluster{{Files: []string{fcPathA, fcPathB}, Lines: 10}},
			g:         nil,
			wantBand:  bandNAStr,
			wantValue: 0,
		},
		{
			name: "one cross-module cluster → one pair",
			clusters: []clone.Cluster{
				{Files: []string{fcPathA, fcPathB}, Lines: 20},
			},
			g:           twoGoNodes(),
			wantBand:    bandInfo,
			wantValue:   1,
			wantDisplay: "1 clone-duplicated cross-module pair(s)",
		},
		{
			name: "two clusters same module pair → deduped to one",
			clusters: []clone.Cluster{
				{Files: []string{fcPathA, fcPathB}, Lines: 10},
				{Files: []string{fcPathA, fcPathB}, Lines: 15},
			},
			g:         twoGoNodes(),
			wantBand:  bandInfo,
			wantValue: 1,
		},
		{
			name: "two distinct cross-module pairs",
			clusters: []clone.Cluster{
				{Files: []string{fcPathA, fcPathB}, Lines: 10},
				{Files: []string{fcPathA, fcPathC}, Lines: 8},
			},
			g: metricstest.BuildGraph(
				[]graph.Node{
					{Kind: graph.NodeKindFile, Path: fcPathA},
					{Kind: graph.NodeKindFile, Path: fcPathB},
					{Kind: graph.NodeKindFile, Path: fcPathC},
				},
				[]graph.Edge{
					{From: "file:" + fcPathA, To: "file:" + fcPathB, Kind: graph.EdgeKindImports, Language: "go"},
					{From: "file:" + fcPathA, To: "file:" + fcPathC, Kind: graph.EdgeKindImports, Language: "go"},
				},
			),
			wantBand:  bandInfo,
			wantValue: 2,
		},
		{
			name: "same-module cluster → no cross-module pair → n/a",
			clusters: []clone.Cluster{
				// Both files resolve to module "pkg/a"
				{Files: []string{"pkg/a/a.go", "pkg/a/b.go"}, Lines: 10},
			},
			g:         twoGoNodes(),
			wantBand:  bandNAStr,
			wantValue: 0,
		},
		{
			name: "cross-module pair also co-changes",
			clusters: []clone.Cluster{
				{Files: []string{fcPathA, fcPathB}, Lines: 20},
			},
			coChange: map[[2]string]int{
				{fcPathA, fcPathB}: 5,
			},
			g:           twoGoNodes(),
			wantBand:    bandInfo,
			wantValue:   1,
			wantDisplay: "1 clone-duplicated cross-module pair(s) (1 also co-change)",
		},
		{
			name: "co-change on different pair → clone pair unaffected",
			clusters: []clone.Cluster{
				{Files: []string{fcPathA, fcPathB}, Lines: 20},
			},
			coChange: map[[2]string]int{
				{fcPathA, fcPathC}: 3,
			},
			g:           twoGoNodes(),
			wantBand:    bandInfo,
			wantValue:   1,
			wantDisplay: "1 clone-duplicated cross-module pair(s)",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			in := signal.DuplicationInput{
				CommonInput: signal.CommonInput{Graph: tc.g},
				Duplication: signal.DuplicationSignals{Clusters: tc.clusters},
				History:     signal.HistorySignals{CoChange: tc.coChange},
			}
			got := m.Calculate(in)

			if got.Band != tc.wantBand {
				t.Errorf("band=%q want %q", got.Band, tc.wantBand)
			}
			if got.Value != tc.wantValue {
				t.Errorf("value=%v want %v", got.Value, tc.wantValue)
			}
			if tc.wantDisplay != "" && got.Display != tc.wantDisplay {
				t.Errorf("display=%q want %q", got.Display, tc.wantDisplay)
			}
			if got.Name != "functional_candidates" {
				t.Errorf("name=%q want %q", got.Name, "functional_candidates")
			}
			if got.Version != "functional_candidates.v1" {
				t.Errorf("version=%q want %q", got.Version, "functional_candidates.v1")
			}
			// Delta must be nil — report-only metric must never flip the verdict.
			if got.Delta != nil {
				t.Errorf("delta want nil got %v", *got.Delta)
			}
		})
	}
}
