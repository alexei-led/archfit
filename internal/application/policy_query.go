package application

import (
	"context"
	"time"

	"github.com/alexei-led/archfit/v3/internal/assessment/evaluation"
	"github.com/alexei-led/archfit/v3/internal/model/graph"
	"github.com/alexei-led/archfit/v3/internal/policy"
	"github.com/alexei-led/archfit/v3/internal/relationship/analysis"
)

// QueriedEdge is one import an agent asks about, spelled as the language's
// extractor spells it, before any tool runs.
type QueriedEdge struct {
	// From is the importing file, ScanRoot-relative.
	From string
	// Language is the language that owns From, or "" when none does.
	Language string
	// Graph holds the one queried edge. Nil when NotExtracted or Undecided.
	Graph *graph.Graph
	// NotExtracted says why the language's extractor would never emit the
	// edge, so check never reads it.
	NotExtracted string
	// Undecided says why the edge cannot be spelled without a tool.
	Undecided string
	// Production says whether From is a production file.
	Production map[string]bool
}

// EdgeQuery builds the queried edge. The adapter reads the config and the
// files it names; it runs no tool and never opens the fact cache.
type EdgeQuery interface {
	Edge(ctx context.Context, from, target string) (QueriedEdge, error)
}

// CanImportRequest asks whether the file From may import each of Targets.
type CanImportRequest struct {
	Policy    policy.PolicySnapshot
	BundleDir string
	From      string
	Targets   []string
	Now       time.Time
}

// CanImportAnswer is the judgment for one target.
type CanImportAnswer struct {
	Target   string
	From     string
	Language string
	evaluation.EdgeJudgment
}

// PolicyQueryService answers pre-edit questions with the evaluator check
// uses: the queried edge goes through the same relationship analysis, rule
// pass, baseline, and waivers. It runs no extractor and no tool.
type PolicyQueryService struct {
	Edges    EdgeQuery
	Baseline BaselineLoader
}

// CanImport judges each target against the policy, the accepted baseline,
// and the waivers.
func (s PolicyQueryService) CanImport(ctx context.Context, req CanImportRequest) ([]CanImportAnswer, error) {
	var base Baseline
	if s.Baseline != nil {
		loaded, err := s.Baseline.Load(ctx, req.BundleDir)
		if err != nil {
			return nil, err
		}
		base = loaded
	}
	answers := make([]CanImportAnswer, 0, len(req.Targets))
	for _, target := range req.Targets {
		edge, err := s.Edges.Edge(ctx, req.From, target)
		if err != nil {
			return nil, err
		}
		answer := CanImportAnswer{Target: target, From: edge.From, Language: edge.Language}
		switch {
		case edge.NotExtracted != "":
			answer.Answer, answer.Reasons = evaluation.AnswerUnconstrained, []string{edge.NotExtracted}
		case edge.Undecided != "":
			answer.Answer, answer.Reasons = evaluation.AnswerNotDecided, []string{edge.Undecided}
		default:
			related := analysis.Analyze(analysis.Input{
				Graph: edge.Graph, Policy: req.Policy.Relationship,
			})
			judged, err := evaluation.JudgeEdge(evaluation.JudgeInput{
				Relationships: related.Relationships, Policy: req.Policy,
				Accepted: base.Accepted, Now: req.Now, UnwalkedSourceProduction: edge.Production,
			})
			if err != nil {
				return nil, err
			}
			answer.EdgeJudgment = boundJudgmentText(judged)
		}
		answer.Reasons = reportTexts(answer.Reasons, maxReportTextRunes)
		answers = append(answers, answer)
	}
	return answers, nil
}

// boundJudgmentText applies the report text bounds to the repair text of an
// edge judgment. The answer never passes through ProjectReport, yet it carries
// the same rule-derived text (a rationale, alternatives, and docs written as
// YAML block scalars), so it is bounded here, at the one place it is built.
// Finding and rule IDs are identity and stay as they are.
func boundJudgmentText(j evaluation.EdgeJudgment) evaluation.EdgeJudgment {
	j.Denials = boundEdgeFindings(j.Denials)
	j.Accepted = boundEdgeFindings(j.Accepted)
	j.Advisories = boundEdgeFindings(j.Advisories)
	return j
}

func boundEdgeFindings(in []evaluation.EdgeFinding) []evaluation.EdgeFinding {
	if in == nil {
		return nil
	}
	out := make([]evaluation.EdgeFinding, len(in))
	for i, f := range in {
		f.Why = reportText(f.Why, maxReportTextRunes)
		f.Goal = reportText(f.Goal, maxReportTaskTextRunes)
		f.Constraints = reportTexts(f.Constraints, maxReportTaskTextRunes)
		out[i] = f
	}
	return out
}
