package acquisition_test

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/alexei-led/archfit/internal/evidence/acquisition"
	evidenceports "github.com/alexei-led/archfit/internal/evidence/ports"
	"github.com/alexei-led/archfit/internal/model/evidence"
	"github.com/alexei-led/archfit/internal/model/graph"
	"github.com/alexei-led/archfit/internal/model/symbol"
	"github.com/alexei-led/archfit/internal/scope"
)

const (
	langGo       = "go"
	coverageGo   = "go/packages"
	langTS       = "typescript"
	coverageTS   = "dependency-cruiser"
	extractorErr = "toolchain exploded"
	scipToolName = "scip"
)

// stubExtractor is a language extractor whose Extract outcome the test sets.
type stubExtractor struct {
	name, coverage string
	facts          graph.Facts
	err            error
}

func (e stubExtractor) Name() string         { return e.name }
func (e stubExtractor) CoverageTool() string { return e.coverage }
func (e stubExtractor) Extract(context.Context, scope.Scope) (graph.Facts, evidence.Coverage, error) {
	if e.err != nil {
		return graph.Facts{}, evidence.Coverage{}, e.err
	}
	return e.facts, evidence.Coverage{Tool: e.coverage, Status: evidence.StatusOK}, nil
}

// nopResolver is a symbol resolver with no index: every method abstains, which
// is what an absent SCIP toolchain does.
type nopResolver struct{}

func (nopResolver) Name() string { return scipToolName }
func (nopResolver) Resolve(_ context.Context, _, toPath string) (string, string) {
	return toPath, "high"
}
func (nopResolver) Strengths(context.Context, scope.Scope) (map[string]string, evidence.Coverage, error) {
	return nil, evidence.Coverage{}, nil
}
func (nopResolver) Symbols(context.Context, scope.Scope) (symbol.Graph, evidence.Coverage, error) {
	return symbol.Graph{}, evidence.Coverage{}, nil
}

var _ evidenceports.Extractor = stubExtractor{}
var _ evidenceports.SymbolResolver = nopResolver{}

func goFacts() graph.Facts {
	return graph.Facts{
		Nodes: []graph.Node{{Kind: graph.NodeKindFile, Path: "a/a.go", Language: graph.LangGo}},
	}
}

// TestCollectIsolatesOneExtractorFailure pins that a failing extractor degrades
// only its own language: the surviving extractor's facts still build a graph,
// and the failure is disclosed as a partial coverage row.
//
// The row must be filed under CoverageTool(), not Name(): a failed Extract
// returns a zero Coverage, so acquisition stamps the row itself, and using the
// language name creates a phantom analyzer next to the real family — leaving
// the family with zero rows, which both comparison paths read as unavailable.
func TestCollectIsolatesOneExtractorFailure(t *testing.T) {
	got, err := acquisition.Collect(context.Background(), acquisition.Input{
		Resolver: nopResolver{},
		Extractors: []evidenceports.Extractor{
			stubExtractor{name: langTS, coverage: coverageTS, err: errors.New(extractorErr)},
			stubExtractor{name: langGo, coverage: coverageGo, facts: goFacts()},
		},
	})
	if err != nil {
		t.Fatalf("Collect with one surviving extractor: %v", err)
	}
	if got.Graph == nil || len(got.Graph.Nodes()) != 1 {
		t.Fatalf("graph = %+v, want the surviving extractor's facts", got.Graph)
	}
	byTool := map[string]evidence.Coverage{}
	for _, c := range got.Coverages {
		byTool[c.Tool] = c
	}
	failed, ok := byTool[coverageTS]
	if !ok {
		t.Fatalf("coverage rows = %+v, want a row filed under the coverage name %q", got.Coverages, coverageTS)
	}
	if failed.Status != evidence.StatusPartial || !strings.Contains(failed.Reason, extractorErr) {
		t.Errorf("failed row = %+v, want partial carrying the extractor error", failed)
	}
	if _, phantom := byTool[langTS]; phantom {
		t.Errorf("coverage rows = %+v, want no row under the language name %q", got.Coverages, langTS)
	}
	if byTool[coverageGo].Status != evidence.StatusOK {
		t.Errorf("surviving row = %+v, want the extractor's own coverage", byTool[coverageGo])
	}
}

// TestCollectFailsWhenEveryExtractorFails pins the fatal case: nothing was
// measured, so acquisition must not hand downstream stages an empty graph to
// score as if the repo were clean.
func TestCollectFailsWhenEveryExtractorFails(t *testing.T) {
	_, err := acquisition.Collect(context.Background(), acquisition.Input{
		Resolver: nopResolver{},
		Extractors: []evidenceports.Extractor{
			stubExtractor{name: langTS, coverage: coverageTS, err: errors.New(extractorErr)},
			stubExtractor{name: langGo, coverage: coverageGo, err: errors.New("no go toolchain")},
		},
	})
	if err == nil {
		t.Fatal("Collect with every extractor failing returned no error")
	}
	if !strings.Contains(err.Error(), "all 2 extractor(s) failed") ||
		!strings.Contains(err.Error(), extractorErr) {
		t.Errorf("error = %v, want the count and every wrapped cause", err)
	}
}

// A run with no extractors configured is not a failure — there is nothing to
// fail. It yields an empty graph and whatever extra coverage the caller passed.
func TestCollectWithNoExtractorsIsNotFatal(t *testing.T) {
	extra := evidence.Coverage{Tool: "jscpd", Status: evidence.StatusDisabled}
	got, err := acquisition.Collect(context.Background(), acquisition.Input{
		Resolver: nopResolver{}, ExtraCoverage: []evidence.Coverage{extra},
	})
	if err != nil {
		t.Fatalf("Collect with no extractors: %v", err)
	}
	if got.Graph == nil || len(got.Graph.Nodes()) != 0 {
		t.Errorf("graph = %+v, want an empty graph", got.Graph)
	}
	if len(got.Coverages) != 1 || got.Coverages[0] != extra {
		t.Errorf("coverages = %+v, want only the caller-supplied extra row", got.Coverages)
	}
}
