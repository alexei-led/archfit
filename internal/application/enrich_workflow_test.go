package application

import (
	"context"
	"errors"
	"io"
	"reflect"
	"testing"

	"github.com/alexei-led/archfit/internal/evidence"
	"github.com/alexei-led/archfit/internal/model/graph"
	"github.com/alexei-led/archfit/internal/policy"
)

const (
	workflowConfig = "config"
	workflowLabels = "labels"
	workflowModA   = "a"
	workflowModB   = "b"
	workflowModC   = "c"
	workflowFileA  = "a/a.go"
	workflowFileB  = "b/b.go"
	workflowFileC  = "c/c.go"
)

// workflowEvidence is a real evidence stage over a two-file fixture graph: one
// a→b import edge whose strength hint decides whether the pair is a refinable
// candidate ("functional") or an abstained one (""). Selection then runs for
// real, so these tests pin the workflow against the actual relationship
// contract rather than a hand-written candidate list.
type workflowEvidence struct {
	order *[]string
	hint  string
	empty bool
}

func (e workflowEvidence) Acquire(context.Context, AnalysisRequest) (Acquired, error) {
	*e.order = append(*e.order, "capture")
	modules := map[string]policy.ModuleDef{
		workflowModA: {Paths: []string{"a/**"}, Subdomain: "core"},
		workflowModB: {Paths: []string{"b/**"}, Subdomain: subdomainSupporting},
		workflowModC: {Paths: []string{"c/**"}, Subdomain: subdomainSupporting},
	}
	topology := policy.TopologyView{Modules: modules, ModuleMap: policy.BuildModuleMap(modules)}
	acquired := Acquired{Context: AnalysisContext{
		Policy: policy.New(topology, policy.RelationshipPolicy{}, policy.AssessmentPolicy{Topology: topology}, policy.GatePolicy{}, nil, nil),
	}}
	if e.empty {
		return acquired, nil
	}
	acquired.Facts = evidence.Facts{Graph: graph.Build([]graph.Facts{{
		Nodes: []graph.Node{
			{Kind: graph.NodeKindFile, Path: workflowFileA, Language: graph.LangGo},
			{Kind: graph.NodeKindFile, Path: workflowFileB, Language: graph.LangGo},
			{Kind: graph.NodeKindFile, Path: workflowFileC, Language: graph.LangGo},
		},
		Edges: []graph.Edge{
			{
				From: "file:" + workflowFileA, To: "file:" + workflowFileB,
				Kind: graph.EdgeKindImports, Language: graph.LangGo, StrengthHint: e.hint,
				Locations: []graph.Location{{File: workflowFileA, Line: 7}},
			},
			{
				From: "file:" + workflowFileA, To: "file:" + workflowFileC,
				Kind: graph.EdgeKindImports, Language: graph.LangGo, StrengthHint: e.hint,
				Locations: []graph.Location{{File: workflowFileA, Line: 8}},
			},
		},
	}})}
	return acquired, nil
}

type workflowStore struct {
	labels  []EnrichmentLabel
	order   *[]string
	saveErr error
	saved   []EnrichmentLabel
}

func (s *workflowStore) Load(context.Context, string) ([]EnrichmentLabel, error) {
	*s.order = append(*s.order, "load")
	return s.labels, nil
}

func (s *workflowStore) Save(_ context.Context, _ string, in []EnrichmentLabel) error {
	*s.order = append(*s.order, "save")
	s.saved = append([]EnrichmentLabel(nil), in...)
	return s.saveErr
}

type workflowJudge struct {
	order  *[]string
	drafts []EnrichmentLabel
}

func (j workflowJudge) Judge(context.Context, EnrichmentJudgmentRequest) ([]EnrichmentLabel, error) {
	*j.order = append(*j.order, "judge")
	return j.drafts, nil
}

func workflowService(ev workflowEvidence, store *workflowStore, judge workflowJudge) EnrichService {
	return EnrichService{
		Stages: StageExecutor{Preparer: noopPrepare{}, Evidence: ev, Stderr: io.Discard},
		Labels: store, Judge: judge,
	}
}

type noopPrepare struct{}

func (noopPrepare) Prepare(context.Context) error { return nil }

// freshHashFor returns the evidence hash the workflow itself would compute for
// the fixture pair, so "approved and fresh" is expressed in the same terms the
// production code uses instead of a magic string.
func freshHashFor(t *testing.T, ev workflowEvidence) string {
	t.Helper()
	order := []string{}
	ev.order = &order
	acquired, err := ev.Acquire(context.Background(), AnalysisRequest{})
	if err != nil {
		t.Fatal(err)
	}
	captured := relate(acquired).Relationships
	hashes := pairEvidence(*projectEnrichmentEvidence(captured), map[string]struct{}{EnrichmentPairKey(workflowModA, workflowModB): {}})
	return hashes[EnrichmentPairKey(workflowModA, workflowModB)]
}

func TestEnrichServiceWorkflowOrderPreservesApprovedAndStampsEvidence(t *testing.T) {
	order := []string{}
	ev := workflowEvidence{order: &order, hint: "functional"}
	store := &workflowStore{order: &order, labels: []EnrichmentLabel{{
		From: workflowModA, To: workflowModB, Strength: "contract",
		Status: EnrichmentLabelStatusApproved, EvidenceHash: freshHashFor(t, ev),
	}}}
	// The judge proposes a draft for BOTH pairs; only the un-approved one may land.
	judge := workflowJudge{order: &order, drafts: []EnrichmentLabel{
		{From: workflowModA, To: workflowModB, Strength: "intrusive", Status: EnrichmentLabelStatusDraft},
		{From: workflowModA, To: workflowModC, Strength: "model", Status: EnrichmentLabelStatusDraft},
	}}

	out, err := workflowService(ev, store, judge).
		Execute(context.Background(), EnrichmentRequest{ConfigPath: workflowConfig, LabelsPath: workflowLabels})
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(order, []string{"capture", "load", "judge", "save"}) {
		t.Fatalf("order = %v", order)
	}
	if out.Drafts != 2 {
		t.Fatalf("drafts = %d, want 2", out.Drafts)
	}
	saved := map[string]EnrichmentLabel{}
	for _, l := range store.saved {
		saved[EnrichmentPairKey(l.From, l.To)] = l
	}
	if got := saved[EnrichmentPairKey(workflowModA, workflowModB)]; got.Strength != "contract" || got.Status != EnrichmentLabelStatusApproved {
		t.Fatalf("a fresh approved label was clobbered by a draft: %+v", got)
	}
	if got := saved[EnrichmentPairKey(workflowModA, workflowModC)]; got.Strength != "model" || got.EvidenceHash == "" {
		t.Fatalf("new draft = %+v, want the judged strength stamped with current evidence", got)
	}
}

func TestEnrichServiceNoCandidateSkipsJudgeAndSave(t *testing.T) {
	order := []string{}
	store := &workflowStore{order: &order}
	out, err := workflowService(workflowEvidence{order: &order, empty: true}, store, workflowJudge{order: &order}).
		Execute(context.Background(), EnrichmentRequest{ConfigPath: workflowConfig, LabelsPath: workflowLabels})
	if err != nil || !out.NoCandidates {
		t.Fatalf("out=%+v err=%v", out, err)
	}
	if !reflect.DeepEqual(order, []string{"capture", "load"}) || len(store.saved) != 0 {
		t.Fatalf("order=%v saved=%v", order, store.saved)
	}
}

func TestEnrichServiceSaveError(t *testing.T) {
	order := []string{}
	store := &workflowStore{order: &order, saveErr: errors.New("disk full")}
	judge := workflowJudge{order: &order, drafts: []EnrichmentLabel{{From: workflowModA, To: workflowModB}}}
	_, err := workflowService(workflowEvidence{order: &order, hint: "functional"}, store, judge).
		Execute(context.Background(), EnrichmentRequest{ConfigPath: workflowConfig, LabelsPath: workflowLabels})
	if err == nil || !errors.Is(err, store.saveErr) {
		t.Fatalf("err=%v", err)
	}
}

func TestEnrichServiceAbstainedSelection(t *testing.T) {
	order := []string{}
	store := &workflowStore{order: &order}
	out, err := workflowService(workflowEvidence{order: &order}, store, workflowJudge{order: &order}).
		Execute(context.Background(), EnrichmentRequest{
			ConfigPath: workflowConfig, LabelsPath: workflowLabels,
			Abstained: true, EdgeCap: 3, SampleCap: 2,
		})
	if err != nil {
		t.Fatal(err)
	}
	if out.Candidates != 2 || out.SelectedEdges != 2 {
		t.Fatalf("abstained selection = %d pair(s), %d edge(s); want both unknown-strength pairs", out.Candidates, out.SelectedEdges)
	}
}
