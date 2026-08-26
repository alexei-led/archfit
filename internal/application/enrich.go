package application

import (
	"context"
	"errors"
	"fmt"
	"sort"
)

// EnrichmentEdge is the prompt-ready projection of one classified relationship.
type EnrichmentEdge struct {
	FromPath, ToPath         string
	FromModule, ToModule     string
	Kind, Strength, Distance string
	Locations                []Location
}

// Location is a source location in captured enrichment evidence.
type Location struct {
	File string
	Line int
}

// EnrichmentEvidence contains prompt-ready relationship evidence.
type EnrichmentEvidence struct{ Edges []EnrichmentEdge }

// EnrichmentLabel is the application-owned review DTO for a coupling label.
type EnrichmentLabel struct {
	From, To     string
	Strength     string
	Rationale    string
	EvidenceRefs []string
	Basis        string
	EvidenceHash string
	Status       string
	Confidence   string
	Provenance   string
}

// EnrichmentCandidatePair is a selected semantic-review candidate.
type EnrichmentCandidatePair struct {
	From, To    string
	Strength    string
	EdgeCount   int
	SamplePaths []string
}

// EnrichmentSample is one source location supporting an abstained edge.
type EnrichmentSample struct {
	FromPath, ToPath string
	File             string
	Line             int
	Snippet          string
}

// EnrichmentAbstainedPair groups abstained edges by ordered module pair.
type EnrichmentAbstainedPair struct {
	From, To  string
	EdgeCount int
	Samples   []EnrichmentSample
}

// EnrichmentLabelRequest is the provider-facing judgment request DTO.
type EnrichmentLabelRequest struct {
	From       string `json:"from"`
	To         string `json:"to"`
	Strength   string `json:"strength"`
	Confidence string `json:"confidence,omitempty"`
}

// EnrichmentLabelResponse is the provider-facing judgment response DTO.
type EnrichmentLabelResponse struct {
	From         string   `json:"from"`
	To           string   `json:"to"`
	Strength     string   `json:"strength"`
	Confidence   string   `json:"confidence,omitempty"`
	Rationale    string   `json:"rationale"`
	EvidenceRefs []string `json:"evidence_refs"`
	Basis        string   `json:"basis"`
}

const (
	// EnrichmentLabelStatusDraft marks an unreviewed proposal.
	EnrichmentLabelStatusDraft = "draft"
	// EnrichmentLabelStatusApproved marks a human-approved proposal.
	EnrichmentLabelStatusApproved = "approved"
	// EnrichmentLabelProvenanceHuman marks human-authored labels.
	EnrichmentLabelProvenanceHuman = "human"
	// EnrichmentLabelProvenanceLLM marks LLM-authored labels.
	EnrichmentLabelProvenanceLLM = "llm"
	// EnrichmentLabelProvenanceTool marks deterministic labels.
	EnrichmentLabelProvenanceTool = "tool"
	// EnrichmentLabelConfidenceHigh is high provider confidence.
	EnrichmentLabelConfidenceHigh = "high"
	// EnrichmentLabelConfidenceMedium is medium provider confidence.
	EnrichmentLabelConfidenceMedium = "medium"
	// EnrichmentLabelConfidenceLow is low provider confidence.
	EnrichmentLabelConfidenceLow = "low"
)

// EnrichmentRequest requests one technical enrichment capture and review.
type EnrichmentRequest struct {
	ConfigPath, Root string
	// SnippetRoot is the base directory source snippets are read from. It is
	// SEPARATE from Root on purpose: Root is the analysis boundary and an empty
	// Root means "the whole repository", while snippet reading needs a concrete
	// directory and defaults to the config's own. Folding the two together made
	// `config enrich abstained -c sub/.archfit.yaml` analyse only sub/, whose
	// node paths and module resolution differ from the full tree — every
	// evidence hash it stamped then read permanently stale to the gate.
	SnippetRoot        string
	Refresh            bool
	LabelsPath         string
	Abstained          bool
	EdgeCap            int
	SampleCap          int
	RepositoryEvidence []string
}

// EnrichmentResult summarizes one enrichment workflow execution.
type EnrichmentResult struct {
	Evidence                                                    EnrichmentEvidence
	Candidates, SelectedEdges, TotalEdges, Drafts, ApprovedKept int
	LabelsPath                                                  string
	NoCandidates                                                bool
}

// EnrichmentLabelStore is the persistence port for review labels.
type EnrichmentLabelStore interface {
	Load(context.Context, string) ([]EnrichmentLabel, error)
	Save(context.Context, string, []EnrichmentLabel) error
}

// EnrichmentJudgmentRequest is intentionally narrower than the provider API.
type EnrichmentJudgmentRequest struct {
	Candidates         []EnrichmentCandidatePair
	Abstained          []EnrichmentAbstainedPair
	RepositoryEvidence []string
}

// EnrichmentJudge requests judgments without exposing provider or serialization types.
type EnrichmentJudge interface {
	Judge(context.Context, EnrichmentJudgmentRequest) ([]EnrichmentLabel, error)
}

// EnrichmentSnippetSource optionally adds source snippets to abstained samples.
type EnrichmentSnippetSource interface {
	Snippet(root, file string, line int) string
}

// EnrichService owns the complete label enrichment workflow.
type EnrichService struct {
	Stages   StageExecutor
	Labels   EnrichmentLabelStore
	Judge    EnrichmentJudge
	Snippets EnrichmentSnippetSource
}

// capture runs the analysis stages far enough to record the classified
// relationship set, then narrows it to the enrichment contract. The full
// diagnostic never leaves this method: enrichment reviews coupling evidence,
// not findings.
func (s EnrichService) capture(ctx context.Context, req EnrichmentRequest) (EnrichmentResult, error) {
	analysisReq := AnalysisRequest{ConfigSource: req.ConfigPath, Root: req.Root, CaptureRelationships: true, SuppressGateReasons: true}
	acquired, err := s.Stages.Evidence.Acquire(ctx, analysisReq)
	if err != nil {
		return EnrichmentResult{}, err
	}
	base, err := s.Stages.loadBaseline(ctx, analysisReq, acquired.Context)
	if err != nil {
		return EnrichmentResult{}, err
	}
	out, err := s.Stages.assess(ctx, analysisReq, acquired, base)
	if err != nil {
		return EnrichmentResult{}, err
	}
	if out.EnrichmentEvidence == nil {
		return EnrichmentResult{}, nil
	}
	return EnrichmentResult{Evidence: *out.EnrichmentEvidence}, nil
}

// snippetRoot returns the directory source snippets are read from, falling back
// to the analysis root when the caller set none.
func (r EnrichmentRequest) snippetRoot() string {
	if r.SnippetRoot != "" {
		return r.SnippetRoot
	}
	return r.Root
}

// EnrichmentPairKey is the stable ordered module-pair key used by the workflow.
func EnrichmentPairKey(from, to string) string { return from + "\x00" + to }

// effectiveApprovedLabels returns the pairs whose approved label the gate still
// honours, so enrich never re-drafts over one. It must mirror the gate's own
// freshness rule (labels.isEffective): a label goes stale ONLY when a stored
// hash disagrees with the current evidence. An empty EvidenceHash
// (hand-authored) and a pair with no current evidence (edges gone) both stay
// effective — requiring a non-empty match instead would let the LLM overwrite a
// reviewer's hand-pinned strength that the gate is still applying.
func effectiveApprovedLabels(existing []EnrichmentLabel, evidence map[string]string) map[string]struct{} {
	approved := make(map[string]struct{})
	for _, label := range existing {
		if label.Status != EnrichmentLabelStatusApproved {
			continue
		}
		key := EnrichmentPairKey(label.From, label.To)
		hash, measured := evidence[key]
		if !measured || label.EvidenceHash == "" || label.EvidenceHash == hash {
			approved[key] = struct{}{}
		}
	}
	return approved
}

func mergeEnrichmentLabels(existing, drafts []EnrichmentLabel, evidence map[string]string) []EnrichmentLabel {
	byKey := make(map[string]EnrichmentLabel, len(existing)+len(drafts))
	approved := effectiveApprovedLabels(existing, evidence)
	for _, label := range existing {
		byKey[EnrichmentPairKey(label.From, label.To)] = label
	}
	for _, draft := range drafts {
		key := EnrichmentPairKey(draft.From, draft.To)
		if _, keep := approved[key]; !keep {
			byKey[key] = draft
		}
	}
	out := make([]EnrichmentLabel, 0, len(byKey))
	for _, label := range byKey {
		out = append(out, label)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].From != out[j].From {
			return out[i].From < out[j].From
		}
		return out[i].To < out[j].To
	})
	return out
}

// Execute captures evidence, loads labels, selects candidates, obtains judgments,
// stamps current evidence, merges, and saves the review file in that order.
// Execute runs the complete enrichment workflow.
func (s EnrichService) Execute(ctx context.Context, req EnrichmentRequest) (EnrichmentResult, error) {
	if s.Stages.Evidence == nil {
		return EnrichmentResult{}, errors.New("enrichment stage is required")
	}
	if req.ConfigPath == "" {
		return EnrichmentResult{}, errors.New("enrichment config path is required")
	}
	store := s.Labels
	if store == nil || s.Judge == nil {
		return EnrichmentResult{}, errors.New("enrichment workflow is not fully configured")
	}
	if req.LabelsPath == "" {
		return EnrichmentResult{}, errors.New("enrichment labels path is required")
	}
	// Enrichment uses the same explicit preparer boundary as Analyze/Check,
	// once per full workflow. AnalyzeEnrichment then delegates to the prepared
	// analyzer and does not prepare a second time.
	if s.Stages.Preparer != nil {
		if err := s.Stages.Preparer.Prepare(ctx); err != nil {
			return EnrichmentResult{}, fmt.Errorf("enrichment preparation: %w", err)
		}
	}
	captured, err := s.capture(ctx, req)
	if err != nil {
		return EnrichmentResult{}, fmt.Errorf("enrichment analysis: %w", err)
	}
	existing, err := store.Load(ctx, req.LabelsPath)
	if err != nil {
		return EnrichmentResult{}, fmt.Errorf("enrichment labels: %w", err)
	}
	wanted := make(map[string]struct{}, len(existing))
	for _, label := range existing {
		wanted[EnrichmentPairKey(label.From, label.To)] = struct{}{}
	}
	evidence := pairEvidence(captured.Evidence, wanted)
	approved := effectiveApprovedLabels(existing, evidence)
	var candidates []EnrichmentCandidatePair
	var abstained []EnrichmentAbstainedPair
	total := 0
	if req.Abstained {
		abstained, total = selectAbstained(captured.Evidence, approved, req.EdgeCap, req.SampleCap)
	} else {
		candidates = selectCandidates(captured.Evidence, approved)
	}
	selectedEdges := len(candidates)
	for _, pair := range abstained {
		selectedEdges += pair.EdgeCount
	}
	result := EnrichmentResult{Evidence: captured.Evidence, Candidates: len(candidates) + len(abstained), SelectedEdges: selectedEdges, TotalEdges: total, LabelsPath: req.LabelsPath, NoCandidates: len(candidates) == 0 && len(abstained) == 0}
	if result.NoCandidates {
		return result, nil
	}
	if s.Snippets != nil {
		for i := range abstained {
			for j := range abstained[i].Samples {
				sample := &abstained[i].Samples[j]
				if sample.Snippet == "" && sample.File != "" {
					sample.Snippet = s.Snippets.Snippet(req.snippetRoot(), sample.File, sample.Line)
				}
			}
		}
	}
	drafts, err := s.Judge.Judge(ctx, EnrichmentJudgmentRequest{Candidates: candidates, Abstained: abstained, RepositoryEvidence: req.RepositoryEvidence})
	if err != nil {
		return EnrichmentResult{}, fmt.Errorf("enrichment judgment: %w", err)
	}
	draftWanted := make(map[string]struct{}, len(drafts))
	for _, draft := range drafts {
		draftWanted[EnrichmentPairKey(draft.From, draft.To)] = struct{}{}
	}
	draftEvidence := pairEvidence(captured.Evidence, draftWanted)
	for i := range drafts {
		drafts[i].EvidenceHash = draftEvidence[EnrichmentPairKey(drafts[i].From, drafts[i].To)]
	}
	// Keep the first evidence set so approved labels not selected this run are
	// still evaluated against their current hash during merge. PairEvidence
	// returns nil for an empty want set, which is the steady state when the
	// judge kept zero drafts — the merge must still run, so materialise the map.
	if draftEvidence == nil {
		draftEvidence = make(map[string]string, len(evidence))
	}
	for key, hash := range evidence {
		if _, ok := draftEvidence[key]; !ok {
			draftEvidence[key] = hash
		}
	}
	merged := mergeEnrichmentLabels(existing, drafts, draftEvidence)
	if err := store.Save(ctx, req.LabelsPath, merged); err != nil {
		return EnrichmentResult{}, fmt.Errorf("enrichment labels: %w", err)
	}
	for _, label := range merged {
		if label.Status == EnrichmentLabelStatusApproved {
			result.ApprovedKept++
		}
	}
	result.Drafts = len(drafts)
	return result, nil
}

const (
	enrichmentStrengthContract   = "contract"
	enrichmentStrengthFunctional = "functional"
	enrichmentStrengthModel      = "model"
	enrichmentStrengthIntrusive  = "intrusive"
)

// ValidEnrichmentStrength validates a provider-proposed strength.
func ValidEnrichmentStrength(s string) bool {
	switch s {
	case enrichmentStrengthContract, enrichmentStrengthFunctional, enrichmentStrengthModel, enrichmentStrengthIntrusive:
		return true
	default:
		return false
	}
}
