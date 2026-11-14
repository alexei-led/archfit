package analysis_test

import (
	"reflect"
	"testing"

	"github.com/alexei-led/archfit/v3/internal/model/fileclass"
	"github.com/alexei-led/archfit/v3/internal/policy"
	"github.com/alexei-led/archfit/v3/internal/relationship"
	"github.com/alexei-led/archfit/v3/internal/relationship/analysis"
)

func TestClonesDoNotChangeImportScoring(t *testing.T) {
	modules := map[string]policy.ModuleDef{
		"a": core("team", "svc", "a/**"),
		"b": core("team", "svc", "b/**"),
	}
	for _, name := range []string{"a", "b"} {
		module := modules[name]
		module.Public = []string{name + "/api/**"}
		modules[name] = module
	}
	in := analysis.Input{
		Graph:  exampleGraph(exampleEdge{fromFile: fileAPIX, toFile: fileAPIY, hint: hintInterface}),
		Policy: relationshipPolicy(modules),
	}
	// Before removal, inject the old producer's facts. Afterwards the input has
	// no clone field and the same import must still produce the same seam.
	fields := reflect.ValueOf(&in).Elem()
	if clusters := fields.FieldByName("CloneClusters"); clusters.IsValid() {
		entry := reflect.New(clusters.Type().Elem()).Elem()
		entry.FieldByName("Files").Set(reflect.ValueOf([]string{"a/dup.go", "b/dup.go"}))
		entry.FieldByName("Lines").SetInt(40)
		clusters.Set(reflect.Append(clusters, entry))
		fields.FieldByName("FileClassIndex").Set(reflect.ValueOf(map[string]fileclass.FileClass{
			"a/dup.go": fileclass.Production, "b/dup.go": fileclass.Production,
		}))
	}
	got := analysis.Analyze(in)
	seam := seamBetween(t, got, "a", "b")
	if seam.Edges != 1 || seam.ScoredEdges != 1 {
		t.Errorf("seam edges=%d scored=%d; want one scored import and no clone contribution", seam.Edges, seam.ScoredEdges)
	}
	if edge := onlyEdge(t, got); edge.Strength != relationship.StrengthContract {
		t.Errorf("import strength = %s, want contract", edge.Strength)
	}

	in.Graph = exampleGraph()
	got = analysis.Analyze(in)
	if got.Assessment.ClassifiedEdges.Scored != 0 || len(got.Assessment.AdvisoryCandidates) != 0 {
		t.Errorf("clone-only evidence produced scored pairs or advisories: %+v", got.Assessment)
	}
}
