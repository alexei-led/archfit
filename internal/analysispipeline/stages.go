package pipeline

import (
	"context"
	"errors"
	"time"

	"github.com/alexei-led/archfit/internal/assessment/evaluation"
	"github.com/alexei-led/archfit/internal/assessment/result"
	signal "github.com/alexei-led/archfit/internal/assessment/signals"
	"github.com/alexei-led/archfit/internal/baseline"
	"github.com/alexei-led/archfit/internal/evidence/acquisition"
	evidenceports "github.com/alexei-led/archfit/internal/evidence/ports"
	"github.com/alexei-led/archfit/internal/model/evidence"
	"github.com/alexei-led/archfit/internal/model/pattern"
	"github.com/alexei-led/archfit/internal/model/report"
	"github.com/alexei-led/archfit/internal/policy"
	"github.com/alexei-led/archfit/internal/relationship"
	relationshipanalysis "github.com/alexei-led/archfit/internal/relationship/analysis"
	"github.com/alexei-led/archfit/internal/relationship/labels"
	"github.com/alexei-led/archfit/internal/scope"
)

// StageInput carries the inputs shared by evidence, relationship, and assessment stages.
type StageInput struct {
	Mode                  Mode
	Scope                 scope.Scope
	Policy                policy.PolicySnapshot
	Extractors            []evidenceports.Extractor
	Patterns              evidenceports.PatternProvider
	PatternCfg            pattern.Config
	Resolver              evidenceports.SymbolResolver
	Syntax                evidenceports.SyntaxProvider
	SyntaxCfg             evidenceports.SyntaxConfig
	Rules                 evaluation.Ruleset
	Metrics               evaluation.Metricset
	Accepted              baseline.Baseline
	BaseMetrics           report.MetricSnapshot
	CaptureRelationships  bool
	Signals               signal.RunSignals
	Labels                []labels.Label
	Now                   time.Time
	PrimaryExtractorTools []string
	ConfigHash            string
}

// ruleEvidence is the output of the pattern and syntax acquisition stage.
type ruleEvidence struct {
	evidence evaluation.RuleEvidence
	coverage []evidence.Coverage
}

func collectRuleEvidence(ctx context.Context, in StageInput) (ruleEvidence, error) {
	patternMatches, patternCoverage, err := in.Patterns.Find(ctx, in.Scope, in.PatternCfg)
	if err != nil {
		return ruleEvidence{}, err
	}
	result := ruleEvidence{
		evidence: evaluation.RuleEvidence{PatternMatches: patternMatches},
		coverage: []evidence.Coverage{patternCoverage},
	}
	if !in.SyntaxCfg.Enabled {
		return result, nil
	}
	if in.Syntax == nil {
		return ruleEvidence{}, errors.New("engine: SyntaxCfg.Enabled=true but no Syntax provider")
	}
	syntaxFacts, syntaxCoverage, err := in.Syntax.Syntax(ctx, in.Scope, in.SyntaxCfg.Languages)
	if err != nil {
		return ruleEvidence{}, err
	}
	for i := range syntaxFacts {
		syntaxFacts[i].Module, _ = in.Policy.Relationship.Topology.ModuleMap.ModuleForFile(syntaxFacts[i].File)
	}
	result.evidence.SyntaxFacts = syntaxFacts
	result.coverage = append(result.coverage, syntaxCoverage)
	return result, nil
}

type relationshipStage struct {
	classified    relationship.AnalysisResult
	relationships relationship.Set
}

func relationshipStageFromAnalysis(classified relationship.AnalysisResult) relationshipStage {
	return relationshipStage{classified: classified, relationships: classified.Relationships}
}

func classifyRelationships(ex acquisition.Result, in StageInput) relationshipStage {
	classified := relationshipanalysis.Analyze(relationshipanalysis.Input{
		Graph: ex.Graph, Policy: in.Policy.Relationship,
		Mode:              relationshipanalysis.Mode{Base: in.Mode.Base, Full: in.Mode.Full},
		Labels:            in.Labels,
		CloneClusters:     in.Signals.Duplication.Clusters,
		FileClassIndex:    in.Signals.Size.FileClassIndex,
		RuntimeSites:      in.Signals.RuntimeAsync.Sites,
		RuntimeConfidence: in.Signals.RuntimeAsync.Confidence,
	})
	return relationshipStageFromAnalysis(classified)
}

// evaluate coordinates acquisition, relationship analysis, and report projection.
// Rendering remains the caller's responsibility.

// RunStages executes the owner stages for callers that already resolved scope
// and adapters. The application pipeline uses Run for normal production runs.
func RunStages(ctx context.Context, in StageInput) (result.Result, error) {
	return evaluate(ctx, in)
}

func evaluate(ctx context.Context, in StageInput) (result.Result, error) {
	acquired, err := acquireStage(ctx, in)
	if err != nil {
		return result.New(), err
	}
	relationships := classifyRelationships(acquired.acquired, in)
	diag, _ := projectAssessment(in, acquired, relationships)
	return diag, nil
}

type acquiredStage struct {
	acquired     acquisition.Result
	ruleEvidence ruleEvidence
}

func acquireStage(ctx context.Context, in StageInput) (acquiredStage, error) {
	ex, err := acquisition.Collect(ctx, acquisition.Input{
		Scope: in.Scope, Extractors: in.Extractors, Resolver: in.Resolver,
		ExtraCoverage: in.Signals.ExtraCoverage,
	})
	if err != nil {
		return acquiredStage{}, err
	}
	ruleEv, err := collectRuleEvidence(ctx, in)
	if err != nil {
		return acquiredStage{}, err
	}
	ex.Coverages = append(ex.Coverages, ruleEv.coverage...)
	return acquiredStage{acquired: ex, ruleEvidence: ruleEv}, nil
}
