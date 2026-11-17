package application_test

import (
	"context"
	"io"
	"reflect"
	"testing"

	"github.com/alexei-led/archfit/v4/internal/application"
	"github.com/alexei-led/archfit/v4/internal/config"
	evidencecontract "github.com/alexei-led/archfit/v4/internal/evidence"
	"github.com/alexei-led/archfit/v4/internal/model/evidence"
	"github.com/alexei-led/archfit/v4/internal/model/graph"
	"github.com/alexei-led/archfit/v4/internal/model/report"
)

type syntheticEvidenceStage struct{ acquired application.Acquired }

func (s syntheticEvidenceStage) Prepare(context.Context) error { return nil }
func (s syntheticEvidenceStage) Acquire(context.Context, application.AnalysisRequest) (application.Acquired, error) {
	return s.acquired, nil
}

const (
	syntheticAsserted = "asserted"
	syntheticUnknown  = "unknown"
)

func TestReportSyntheticComponentRetainsUnknownLineage(t *testing.T) {
	t.Parallel()
	in, err := config.NewReportInput([]byte("version: 3\nowners: [owner]\nmodules:\n  crate:\n    paths: [crate]\n    public: [crate]\n    volatility: low\n    subdomain: supporting\n"))
	if err != nil {
		t.Fatal(err)
	}
	g := graph.Build([]graph.Facts{{
		Language: graph.LangRust,
		Nodes:    []graph.Node{{Kind: graph.NodeKindPackage, Path: "crate::a", Language: graph.LangRust}, {Kind: graph.NodeKindPackage, Path: "crate::b", Language: graph.LangRust}},
		Edges:    []graph.Edge{{From: "package:crate::a", To: "package:crate::b", Kind: graph.EdgeKindImports, Language: graph.LangRust, StrengthHint: "functional"}},
	}})
	stage := syntheticEvidenceStage{acquired: application.Acquired{Facts: evidencecontract.Facts{Graph: g}, Context: application.AnalysisContext{Policy: in.Config.PolicySnapshot()}}}
	response, err := (application.Service{Stages: application.StageExecutor{Preparer: stage, Evidence: stage, Stderr: io.Discard}}).Execute(t.Context(), application.Request{
		ReportContext: &evidence.ReportContext{Scope: evidence.ReportScope{Abstraction: "module", SourceRoot: "."}, Components: in.Components},
	})
	if err != nil {
		t.Fatal(err)
	}
	components := map[string]report.EvidenceComponent{}
	for _, c := range response.Document.State.Evidence.Components {
		components[c.Name] = c
	}
	declared := components["crate"]
	if !declared.Declared || declared.Public.Origin != syntheticAsserted || declared.Volatility.Origin != syntheticAsserted || declared.Subdomain.Origin != syntheticAsserted {
		t.Fatalf("declared overlay: %+v", declared)
	}
	if declared.Public.Raw == nil || declared.Public.Effective == nil || !reflect.DeepEqual(*declared.Public.Raw, []string{"crate"}) || !reflect.DeepEqual(*declared.Public.Effective, []string{"crate"}) {
		t.Fatalf("declared public: %+v", declared.Public)
	}
	for _, name := range []string{"crate::a", "crate::b"} {
		c, ok := components[name]
		if !ok || c.Declared {
			t.Fatalf("synthetic component missing/declared: %s %+v", name, c)
		}
		if c.Public.Raw != nil || c.Volatility.Raw != nil || c.Subdomain.Raw != nil {
			t.Fatalf("invented raw synthetic assertion: %+v", c)
		}
		if c.Volatility.Effective == nil || *c.Volatility.Effective != "low" || c.Subdomain.Effective == nil || *c.Subdomain.Effective != "supporting" {
			t.Fatalf("effective synthetic values lost: %+v", c)
		}
		if c.Public.Effective != nil {
			t.Fatalf("augmentation has no public value; must not invent one: %+v", c.Public)
		}
		if c.Public.Origin != syntheticUnknown || c.Volatility.Origin != syntheticUnknown || c.Subdomain.Origin != syntheticUnknown {
			t.Fatalf("invented synthetic ancestry: %+v", c)
		}
		if len(c.Public.Refs)+len(c.Volatility.Refs)+len(c.Subdomain.Refs) != 0 {
			t.Fatalf("invented lineage references: %+v", c)
		}
		if len(c.Unknowns) != 1 || c.Unknowns[0].Fact != "component_origin" || c.Unknowns[0].Reason == "" {
			t.Fatalf("missing lineage gap: %+v", c.Unknowns)
		}
	}
}
