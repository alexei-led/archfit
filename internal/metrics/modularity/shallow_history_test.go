package modularity_test

import (
	"strings"
	"testing"

	"github.com/alexei-led/archfit/internal/metrics/metricstest"
	"github.com/alexei-led/archfit/internal/metrics/modularity"
	"github.com/alexei-led/archfit/internal/model/graph"
	"github.com/alexei-led/archfit/internal/model/signal"
)

// shallowHistInput builds a HistoryInput with non-empty churn/co-change but the
// given CommitCount — used to exercise the shallow-history guard.
func shallowHistInput(commitCount int) signal.HistoryInput {
	a := graph.Node{Kind: graph.NodeKindPackage, Path: ccModA}
	b := graph.Node{Kind: graph.NodeKindPackage, Path: ccModB}
	g := metricstest.BuildGraph([]graph.Node{a, b}, []graph.Edge{
		{From: a.ID(), To: b.ID(), Kind: graph.EdgeKindImports, Language: "go"},
	})
	return signal.HistoryInput{
		CommonInput: signal.CommonInput{Graph: g},
		History: signal.HistorySignals{
			FileChurn: map[string]int{ccGoFile(ccModA): 1, ccGoFile(ccModB): 1},
			CoChange: map[[2]string]int{
				{ccGoFile(ccModA), ccGoFile(ccModB)}: 1,
			},
			CommitCount: commitCount,
		},
	}
}

// TestChangeCoupling_ShallowHistory verifies that CommitCount=1 returns n/a (no history)
// even when CoChange is non-empty (single-commit window → 0 pairs vacuously).
func TestChangeCoupling_ShallowHistory(t *testing.T) {
	res := modularity.ChangeCouplingMetric{}.Calculate(shallowHistInput(1))
	if res.Band != bandNAStr {
		t.Errorf("band=%q want %q for CommitCount=1", res.Band, bandNAStr)
	}
	if !strings.Contains(res.Display, "no history") {
		t.Errorf("display=%q want \"no history\" substring for CommitCount=1", res.Display)
	}
}

// TestChangeCoupling_PopulatedHistory verifies that CommitCount≥2 computes normally.
func TestChangeCoupling_PopulatedHistory(t *testing.T) {
	in := buildCCInput(
		[]string{ccModA, ccModB},
		map[string]int{ccModA: 10, ccModB: 10},
		map[[2]string]int{{ccModA, ccModB}: 8},
	)
	in.History.CommitCount = 10
	res := modularity.ChangeCouplingMetric{}.Calculate(in)
	if res.Band == bandNAStr {
		t.Errorf("band=%q: CommitCount=10 must not trigger shallow-history guard", res.Band)
	}
	if strings.Contains(res.Display, "no history") {
		t.Errorf("display=%q: CommitCount=10 must not show \"no history\"", res.Display)
	}
}

// TestChangeAmplification_ShallowHistory verifies that CommitCount=1 returns
// n/a (no history) even when FileChurn is non-empty.
func TestChangeAmplification_ShallowHistory(t *testing.T) {
	res := modularity.ChangeAmplificationMetric{}.Calculate(shallowHistInput(1))
	if res.Band != bandNAStr {
		t.Errorf("band=%q want %q for CommitCount=1", res.Band, bandNAStr)
	}
	if !strings.Contains(res.Display, "no history") {
		t.Errorf("display=%q want \"no history\" substring for CommitCount=1", res.Display)
	}
}

// TestChangeAmplification_PopulatedHistory verifies that CommitCount≥2 computes normally.
func TestChangeAmplification_PopulatedHistory(t *testing.T) {
	in := shallowHistInput(10)
	// Raise churn so at least one hub exceeds the amp threshold.
	in.History.FileChurn[ccGoFile(ccModA)] = 10
	in.History.FileChurn[ccGoFile(ccModB)] = 10
	res := modularity.ChangeAmplificationMetric{}.Calculate(in)
	if res.Band == bandNAStr {
		t.Errorf("band=%q: CommitCount=10 must not trigger shallow-history guard", res.Band)
	}
	if strings.Contains(res.Display, "no history") {
		t.Errorf("display=%q: CommitCount=10 must not show \"no history\"", res.Display)
	}
}

// TestHiddenCoupling_ShallowHistory verifies that CommitCount=1 returns
// n/a (no history) even when CoChange is non-empty.
func TestHiddenCoupling_ShallowHistory(t *testing.T) {
	res := modularity.HiddenCouplingMetric{}.Calculate(shallowHistInput(1))
	if res.Band != bandNAStr {
		t.Errorf("band=%q want %q for CommitCount=1", res.Band, bandNAStr)
	}
	if !strings.Contains(res.Display, "no history") {
		t.Errorf("display=%q want \"no history\" substring for CommitCount=1", res.Display)
	}
}

// TestHiddenCoupling_PopulatedHistory verifies that CommitCount≥2 computes normally.
func TestHiddenCoupling_PopulatedHistory(t *testing.T) {
	res := modularity.HiddenCouplingMetric{}.Calculate(shallowHistInput(10))
	if res.Band == bandNAStr {
		t.Errorf("band=%q: CommitCount=10 must not trigger shallow-history guard", res.Band)
	}
	if strings.Contains(res.Display, "no history") {
		t.Errorf("display=%q: CommitCount=10 must not show \"no history\"", res.Display)
	}
}

// TestChangeCoupling_CommitCountZeroUnaffected verifies that CommitCount=0 (zero
// value, not set by fixture) does NOT trigger the shallow-history guard — fixtures
// without CommitCount set remain backward-compatible.
func TestChangeCoupling_CommitCountZeroUnaffected(t *testing.T) {
	in := buildCCInput(
		[]string{ccModA, ccModB},
		map[string]int{ccModA: 10, ccModB: 10},
		map[[2]string]int{{ccModA, ccModB}: 8},
	)
	// CommitCount is intentionally not set (zero value).
	res := modularity.ChangeCouplingMetric{}.Calculate(in)
	if strings.Contains(res.Display, "no history") {
		t.Errorf("display=%q: CommitCount=0 (unset) must not show \"no history\"", res.Display)
	}
}

// TestChangeAmplification_CommitCountZeroUnaffected verifies CommitCount=0 does not
// trigger the guard.
func TestChangeAmplification_CommitCountZeroUnaffected(t *testing.T) {
	in := shallowHistInput(0)
	in.History.FileChurn[ccGoFile(ccModA)] = 10
	in.History.FileChurn[ccGoFile(ccModB)] = 10
	res := modularity.ChangeAmplificationMetric{}.Calculate(in)
	if strings.Contains(res.Display, "no history") {
		t.Errorf("display=%q: CommitCount=0 (unset) must not show \"no history\"", res.Display)
	}
}
