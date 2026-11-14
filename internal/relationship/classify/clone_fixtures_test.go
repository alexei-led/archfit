package classify_test

import (
	"testing"

	"github.com/alexei-led/archfit/v3/internal/model/graph"
	"github.com/alexei-led/archfit/v3/internal/policy"
	"github.com/alexei-led/archfit/v3/internal/relationship/classify"
	"github.com/alexei-led/archfit/v3/internal/relationship/coupling"
)

const (
	cfgVolLow      = "low"
	fileFromA      = "file:services/a/a.go"
	fileToB        = "file:services/b/b.go"
	hintFunctional = "functional"
	hintIntrusive  = "intrusive"
	modNameA       = "modA"
	modNameB       = "modB"
)

// A flat-named edge across a bare module boundary scores at D=9: functional
// coupling into a core module is critical whatever the key spelling.
func TestFlatNameEdgeScoresAtTheBoundaryRung(t *testing.T) {
	t.Parallel()

	cfg := classify.Config{Modules: map[string]policy.ModuleDef{
		"core": {Paths: []string{"core/**"}, Subdomain: subdomainCore},
		"api":  {Paths: []string{"api/**"}},
	}}
	edge := graph.Edge{From: "file:core/x.go", To: "file:api/y.go", Kind: graph.EdgeKindImports, StrengthHint: hintFunctional}
	cl := classify.Run(makeGraph([]graph.Edge{edge}), cfg)[edgeKey(edge)]

	if cl.Distance != coupling.DistanceCrossModule {
		t.Errorf("Distance = %q, want cross_module", cl.Distance)
	}
	// V = worse(core high, api undeclared) = high: S=8, D=9, V=10 → 2.
	if cl.Score.Balance != 2 || cl.Score.Band != coupling.SeverityCritical {
		t.Errorf("score = %d %q, want 2 critical", cl.Score.Balance, cl.Score.Band)
	}
}
