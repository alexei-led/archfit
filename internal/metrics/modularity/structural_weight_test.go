package modularity_test

import (
	"strings"
	"testing"

	"github.com/alexei-led/archfit/internal/metrics/metricstest"
	"github.com/alexei-led/archfit/internal/metrics/modularity"
	"github.com/alexei-led/archfit/internal/model/fileclass"
	"github.com/alexei-led/archfit/internal/model/graph"
	"github.com/alexei-led/archfit/internal/model/signal"
)

const (
	swFileGenerated = ccModA + "/big_generated.go"
	swFileReal      = ccModB + "/real.go"
	swFileGod       = ccModA + "/god_module.go"
	swFileSmall     = ccModB + "/util.go"
)

// swModC is a third module used in the god-file test (needs ≥3 modules so the
// median is not dominated by the large module itself).
const swModC = "pkg/c"

// makeSwGraph builds a minimal two-module Go graph for structural_weight tests.
// A Go-language edge ensures DominantLanguage returns "go" so FileToModuleKey
// strips filenames to directory-level module keys.
func makeSwGraph() *graph.Graph {
	a := graph.Node{Kind: graph.NodeKindPackage, Path: ccModA}
	b := graph.Node{Kind: graph.NodeKindPackage, Path: ccModB}
	edge := graph.Edge{From: a.ID(), To: b.ID(), Kind: graph.EdgeKindImports, Language: langGo}
	return metricstest.BuildGraph([]graph.Node{a, b}, []graph.Edge{edge})
}

// makeSwGraph3 builds a three-module Go graph so that one large module produces
// a low median from the other two, making the god-module threshold reachable.
// A Go-language edge is required so DominantLanguage returns "go" and
// FileToModuleKey strips filenames to directory keys correctly.
func makeSwGraph3() *graph.Graph {
	a := graph.Node{Kind: graph.NodeKindPackage, Path: ccModA}
	b := graph.Node{Kind: graph.NodeKindPackage, Path: ccModB}
	c := graph.Node{Kind: graph.NodeKindPackage, Path: swModC}
	edge := graph.Edge{From: a.ID(), To: b.ID(), Kind: graph.EdgeKindImports, Language: langGo}
	return metricstest.BuildGraph([]graph.Node{a, b, c}, []graph.Edge{edge})
}

// TestStructuralWeight_GeneratedFileExcluded verifies T5: a file whose LOC
// entry is in FileLOC but whose FileClassIndex entry is Generated is excluded
// from the god-module count — the index-override path.
//
// Design note: FileLOC normally contains only production files (the loc walk
// skips test/generated at classification time). The index-override case arises
// when a caller constructs SizeInput directly (e.g. tests, or a future pipeline
// change). structural_weight must honour the index over the FileLOC presence.
func TestStructuralWeight_GeneratedFileExcluded(t *testing.T) {
	t.Parallel()
	g := makeSwGraph()

	// swFileGenerated is a large file present in FileLOC, but the FileClassIndex
	// marks it Generated — it must NOT inflate ccModA's LOC.
	fileLOC := map[string]int{
		swFileGenerated: 10000, // would be a god-module if not excluded
		swFileReal:      100,
	}
	idx := map[string]fileclass.FileClass{
		swFileGenerated: fileclass.Generated,
		swFileReal:      fileclass.Production,
	}

	in := signal.SizeInput{
		Size: signal.SizeSignals{FileLOC: fileLOC, FileClassIndex: idx},
	}
	in.Graph = g

	res := modularity.StructuralWeightMetric{}.Calculate(in)

	// With the generated file excluded, ccModA contributes 0 LOC.
	// Only ccModB (100 LOC) remains — len(modLOC) < 2 → n/a.
	if res.Band != bandNAStr {
		t.Errorf("band = %q, want n/a (generated file excluded, < 2 modules with LOC)", res.Band)
	}
}

// TestStructuralWeight_RealGodFileStillFlagged verifies the complementary
// case: a genuinely large production file is flagged as a god-module.
// Uses 3 modules so the median is driven by the two small modules (50 LOC each),
// making ccModA's 5000 LOC well above the threshold (median=50, threshold=400).
func TestStructuralWeight_RealGodFileStillFlagged(t *testing.T) {
	t.Parallel()
	g := makeSwGraph3()

	fileLOC := map[string]int{
		swFileGod:             5000,
		swFileSmall:           50,
		swModC + "/helper.go": 50,
	}
	idx := map[string]fileclass.FileClass{
		swFileGod:             fileclass.Production,
		swFileSmall:           fileclass.Production,
		swModC + "/helper.go": fileclass.Production,
	}

	in := signal.SizeInput{
		Size: signal.SizeSignals{FileLOC: fileLOC, FileClassIndex: idx},
	}
	in.Graph = g

	res := modularity.StructuralWeightMetric{}.Calculate(in)

	// median([50,50,5000]) = 50; threshold = max(50*4=200, 400) = 400;
	// ccModA at 5000 >= 400 → should be flagged.
	if res.Band == bandNAStr {
		t.Errorf("band = n/a, want informational (real god-file should be flagged)")
	}
	if res.Value == 0 {
		t.Errorf("value = 0, want ≥1 god-module flagged; display: %s", res.Display)
	}
	if !strings.Contains(res.Display, "pkg/a") {
		t.Errorf("display should mention %s god-module; got: %s", ccModA, res.Display)
	}
}
