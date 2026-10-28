package application

import (
	"context"
	"errors"
	"testing"

	"github.com/alexei-led/archfit/v3/internal/assessment/evaluation"
	"github.com/alexei-led/archfit/v3/internal/policy"
)

// edgeQueryFunc adapts a function to EdgeQuery.
type edgeQueryFunc func(ctx context.Context, from, target string) (QueriedEdge, error)

func (f edgeQueryFunc) Edge(ctx context.Context, from, target string) (QueriedEdge, error) {
	return f(ctx, from, target)
}

func candidatePolicy() policy.PolicySnapshot {
	modules := map[string]policy.ModuleDef{
		"store": {Paths: []string{"src/store/**"}, Public: []string{"src/store/api"}},
	}
	topology := policy.TopologyView{Modules: modules, ModuleMap: policy.BuildModuleMap(modules)}
	return policy.New(topology, policy.RelationshipPolicy{}, policy.AssessmentPolicy{Topology: topology}, policy.GatePolicy{}, nil, nil)
}

// TestCandidateThatCannotBeJudgedIsNotDecided: an extractor that cannot spell a
// directory target (TypeScript reads files) must not turn the denial that named
// the directory into an error. The candidate is reported as not decided.
func TestCandidateThatCannotBeJudgedIsNotDecided(t *testing.T) {
	t.Parallel()
	unspellable := errors.New("typescript: both must be source files")
	svc := PolicyQueryService{Edges: edgeQueryFunc(func(context.Context, string, string) (QueriedEdge, error) {
		return QueriedEdge{}, unspellable
	})}
	answer := CanImportAnswer{Target: "src/store/impl/db.ts", Instead: []string{"Import from src/store/api instead"}}

	got, err := svc.candidates(context.Background(), CanImportRequest{Policy: candidatePolicy(), From: "src/web/app.ts"}, Baseline{}, answer)
	if err != nil {
		t.Fatalf("candidates: %v, want the denial to survive an unjudgeable candidate", err)
	}
	if len(got) != 1 || got[0].Path != "src/store/api" || got[0].PolicyCheck != evaluation.AnswerNotDecided || !got[0].PublicAPI {
		t.Errorf("candidates = %+v, want src/store/api, public, not_decided", got)
	}
}

// TestCandidateErrorStillStopsOnCancel: a cancelled run is an error, not a
// quiet not_decided.
func TestCandidateErrorStillStopsOnCancel(t *testing.T) {
	t.Parallel()
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	svc := PolicyQueryService{Edges: edgeQueryFunc(func(context.Context, string, string) (QueriedEdge, error) {
		return QueriedEdge{}, ctx.Err()
	})}
	answer := CanImportAnswer{Target: "src/store/impl/db.ts", Instead: []string{"Import from src/store/api instead"}}
	if _, err := svc.candidates(ctx, CanImportRequest{Policy: candidatePolicy(), From: "src/web/app.ts"}, Baseline{}, answer); err == nil {
		t.Fatal("a cancelled context must stop the query")
	}
}
