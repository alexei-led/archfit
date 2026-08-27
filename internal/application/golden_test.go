package application_test

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	"github.com/alexei-led/archfit/internal/application"
	"github.com/alexei-led/archfit/internal/assessment/evaluation"
	"github.com/alexei-led/archfit/internal/evidence"
	"github.com/alexei-led/archfit/internal/evidence/acquisition"
	evidenceports "github.com/alexei-led/archfit/internal/evidence/ports"
	goextract "github.com/alexei-led/archfit/internal/extract/golang"
	"github.com/alexei-led/archfit/internal/policy"
	"github.com/alexei-led/archfit/internal/scope"
)

const (
	globModuleA         = "pkg/a/**"
	globModuleB         = "pkg/b/**"
	globModuleBInternal = "pkg/b/internal/**"
)

// goldenFixtureRoot returns the absolute path to internal/testdata/golang, which
// contains real .go files go/packages can load (it shells out to go list).
func goldenFixtureRoot(t *testing.T) string {
	t.Helper()
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller failed")
	}
	abs, err := filepath.Abs(filepath.Join(filepath.Dir(file), "..", "testdata", "golang"))
	if err != nil {
		t.Fatalf("abs path: %v", err)
	}
	return abs
}

// goldenPolicy declares the two-module fixture (a, b) with a forbidden
// dependency from a into b's internal package.
func goldenPolicy() policy.PolicySnapshot {
	modules := map[string]policy.ModuleDef{
		"a": {Paths: []string{globModuleA}, Public: []string{globModuleA}, Internal: []string{}},
		"b": {Paths: []string{globModuleB}, Public: []string{globModuleB}, Internal: []string{globModuleBInternal}},
	}
	topology := policy.TopologyView{Modules: modules, ModuleMap: policy.BuildModuleMap(modules)}
	gates := policy.GatePolicy{Rules: policy.RuleConfig{
		Rules: []policy.RuleDef{{
			ID: "no_internal_access", Type: "forbidden_dependency", Gate: "fail",
			From: globModuleA, To: globModuleBInternal,
		}},
		ModuleMap: topology.ModuleMap,
	}}
	return policy.New(topology, policy.RelationshipPolicy{}, policy.AssessmentPolicy{Topology: topology}, gates, nil, nil)
}

// goldenEvidence is a fixed-context evidence stage: it runs the real Go
// extractor over the fixture but pins scope, instant, and policy so the only
// variable left in the comparison is the pipeline itself.
type goldenEvidence struct {
	root   string
	now    time.Time
	policy policy.PolicySnapshot
}

func (g goldenEvidence) Acquire(ctx context.Context, _ application.AnalysisRequest) (application.Acquired, error) {
	sc := scope.Scope{Root: g.root, Mode: scope.ModeFull}
	collected, err := acquisition.Collect(ctx, acquisition.Input{
		Scope:      sc,
		Extractors: []evidenceports.Extractor{goextract.New(evidenceports.ExtractConfig{})},
		Resolver:   evidenceports.NopSymbolResolver{},
	})
	if err != nil {
		return application.Acquired{}, err
	}
	facts := evidence.Facts{Graph: collected.Graph, Coverage: collected.Coverages, Symbols: collected.SCIPSymbols}
	return application.Acquired{
		Facts:        facts,
		Observations: evaluation.Observations{Coverage: facts.Coverage, Symbols: facts.Symbols},
		Context:      application.AnalysisContext{Scope: sc, Full: true, Now: g.now, Policy: g.policy},
	}, nil
}

type noopPreparer struct{}

func (noopPreparer) Prepare(context.Context) error { return nil }

// TestGolden_DoubleRun runs the application stage sequence twice with identical
// inputs against the testdata/golang fixture and asserts byte-identical
// JSON-encoded output. This is CI gate 2: determinism of the full pipeline.
func TestGolden_DoubleRun(t *testing.T) {
	// Fixed timestamp — any wall-clock source would break determinism.
	stages := application.StageExecutor{
		Preparer: noopPreparer{},
		Evidence: goldenEvidence{root: goldenFixtureRoot(t), now: time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC), policy: goldenPolicy()},
		Stderr:   io.Discard,
	}

	runOnce := func() []byte {
		t.Helper()
		out, err := stages.Execute(context.Background(), application.AnalysisRequest{NoAdvisories: false})
		if err != nil {
			t.Fatalf("Execute: %v", err)
		}
		// Encode the PROJECTED wire document, not the pre-projection assessment
		// result: report projection is the last stage before output, so a
		// non-deterministic projector must fail this gate.
		doc := application.ProjectReport(out.Diagnostic, out.Score, out.BaseScore, out.HardGate)
		var buf bytes.Buffer
		if encErr := json.NewEncoder(&buf).Encode(doc); encErr != nil {
			t.Fatalf("encode: %v", encErr)
		}
		return buf.Bytes()
	}

	first, second := runOnce(), runOnce()
	if !bytes.Equal(first, second) {
		t.Errorf("double-run outputs differ:\nfirst:  %s\nsecond: %s", first, second)
	}
	if len(first) == 0 {
		t.Error("output is empty")
	}
	var check map[string]any
	if err := json.Unmarshal(first, &check); err != nil {
		t.Errorf("output is not valid JSON: %v", err)
	}
}
