package acquisition

import (
	"testing"

	evidencecontract "github.com/alexei-led/archfit/internal/evidence"
	"github.com/alexei-led/archfit/internal/model/fileclass"
	"github.com/alexei-led/archfit/internal/model/graph"
)

func TestSourceSelectorsUseProducerRootsWithoutEdges(t *testing.T) {
	f := evidencecontract.Facts{
		Graph:          graph.Build([]graph.Facts{{CrateRoots: []graph.CrateRoot{{Dir: "core", Name: "smoke-core"}, {Dir: "adapter", Name: "smoke-adapter"}}}}),
		FileLOC:        map[string]int{"core/src/lib.rs": 5},
		FileClassIndex: map[string]fileclass.FileClass{"adapter/src/lib.rs": fileclass.Production},
	}
	got := sourceSelectorsOf(f)
	if got["core/src/lib.rs"] != "smoke-core" || got["adapter/src/lib.rs"] != "smoke-adapter" {
		t.Fatalf("selectors = %+v", got)
	}
	f.Graph = nil
	got = sourceSelectorsOf(f)
	if got["core/src/lib.rs"] != "" || got["adapter/src/lib.rs"] != "" {
		t.Fatalf("guessed crates without producer: %+v", got)
	}
}
