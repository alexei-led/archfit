package application

import (
	"context"
	"errors"
	"reflect"
	"testing"
)

const (
	workflowConfig = "config"
	workflowLabels = "labels"
	workflowFresh  = "fresh"
	workflowHash   = "hash"
)

type workflowAnalyzer struct {
	evidence EnrichmentEvidence
	order    *[]string
}

func (a workflowAnalyzer) AnalyzeEnrichment(context.Context, EnrichmentRequest) (EnrichmentResult, error) {
	*a.order = append(*a.order, "capture")
	return EnrichmentResult{Evidence: a.evidence}, nil
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
	if s.saveErr != nil {
		return s.saveErr
	}
	return nil
}

type workflowPolicy struct {
	order      *[]string
	hash       string
	candidates []EnrichmentCandidatePair
	abstained  []EnrichmentAbstainedPair
}

func (p workflowPolicy) PairEvidence(EnrichmentEvidence, map[string]struct{}) map[string]string {
	*p.order = append(*p.order, "hash")
	return map[string]string{EnrichmentPairKey("a", "b"): p.hash}
}
func (p workflowPolicy) SelectCandidates(EnrichmentEvidence, map[string]struct{}) []EnrichmentCandidatePair {
	*p.order = append(*p.order, "select")
	return p.candidates
}
func (p workflowPolicy) SelectAbstained(EnrichmentEvidence, map[string]struct{}, int, int) ([]EnrichmentAbstainedPair, int) {
	*p.order = append(*p.order, "select-abstained")
	return p.abstained, 1
}

type workflowJudge struct {
	order  *[]string
	drafts []EnrichmentLabel
}

func (j workflowJudge) Judge(context.Context, EnrichmentJudgmentRequest) ([]EnrichmentLabel, error) {
	*j.order = append(*j.order, "judge")
	return j.drafts, nil
}

func TestEnrichServiceWorkflowOrderPreservesApprovedAndStampsEvidence(t *testing.T) {
	order := []string{}
	store := &workflowStore{order: &order, labels: []EnrichmentLabel{{From: "a", To: "b", Strength: "contract", Status: EnrichmentLabelStatusApproved, EvidenceHash: workflowFresh}}}
	svc := EnrichService{
		Analyzer: workflowAnalyzer{order: &order, evidence: EnrichmentEvidence{Edges: []EnrichmentEdge{{FromModule: "a", ToModule: "b"}}}},
		Labels:   store, Policy: workflowPolicy{order: &order, hash: "fresh", candidates: []EnrichmentCandidatePair{{From: "a", To: "b"}}},
		Judge: workflowJudge{order: &order, drafts: []EnrichmentLabel{{From: "a", To: "b", Strength: "intrusive", Status: EnrichmentLabelStatusDraft}}},
	}
	out, err := svc.Execute(context.Background(), EnrichmentRequest{ConfigPath: workflowConfig, LabelsPath: workflowLabels})
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(order, []string{"capture", "load", workflowHash, "select", "judge", workflowHash, "save"}) {
		t.Fatalf("order = %v", order)
	}
	if out.Drafts != 1 || store.saved[0].Strength != "contract" || store.saved[0].EvidenceHash != "fresh" {
		t.Fatalf("result = %+v saved = %+v", out, store.saved)
	}
}

func TestEnrichServiceNoCandidateSkipsJudgeAndSave(t *testing.T) {
	order := []string{}
	store := &workflowStore{order: &order}
	svc := EnrichService{Analyzer: workflowAnalyzer{order: &order}, Labels: store, Policy: workflowPolicy{order: &order}, Judge: workflowJudge{order: &order}}
	out, err := svc.Execute(context.Background(), EnrichmentRequest{ConfigPath: workflowConfig, LabelsPath: workflowLabels})
	if err != nil || !out.NoCandidates {
		t.Fatalf("out=%+v err=%v", out, err)
	}
	if !reflect.DeepEqual(order, []string{"capture", "load", workflowHash, "select"}) || len(store.saved) != 0 {
		t.Fatalf("order=%v saved=%v", order, store.saved)
	}
}

func TestEnrichServiceSaveError(t *testing.T) {
	order := []string{}
	store := &workflowStore{order: &order, saveErr: errors.New("disk full")}
	svc := EnrichService{Analyzer: workflowAnalyzer{order: &order, evidence: EnrichmentEvidence{Edges: []EnrichmentEdge{{FromModule: "a", ToModule: "b"}}}}, Labels: store, Policy: workflowPolicy{order: &order, candidates: []EnrichmentCandidatePair{{From: "a", To: "b"}}}, Judge: workflowJudge{order: &order, drafts: []EnrichmentLabel{{From: "a", To: "b"}}}}
	if _, err := svc.Execute(context.Background(), EnrichmentRequest{ConfigPath: workflowConfig, LabelsPath: workflowLabels}); err == nil || !errors.Is(err, store.saveErr) {
		t.Fatalf("err=%v", err)
	}
}

func TestEnrichServiceAbstainedSelection(t *testing.T) {
	order := []string{}
	store := &workflowStore{order: &order}
	svc := EnrichService{Analyzer: workflowAnalyzer{order: &order}, Labels: store, Policy: workflowPolicy{order: &order, abstained: []EnrichmentAbstainedPair{{From: "a", To: "b", EdgeCount: 2}}}, Judge: workflowJudge{order: &order}}
	out, err := svc.Execute(context.Background(), EnrichmentRequest{ConfigPath: workflowConfig, LabelsPath: workflowLabels, Abstained: true, EdgeCap: 3, SampleCap: 2})
	if err != nil || out.Candidates != 1 || out.SelectedEdges != 2 {
		t.Fatalf("out=%+v err=%v", out, err)
	}
}
