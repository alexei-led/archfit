package application

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"

	"github.com/alexei-led/archfit/internal/assessment/decision"
	"github.com/alexei-led/archfit/internal/assessment/evaluation"
	"github.com/alexei-led/archfit/internal/assessment/result"
	"github.com/alexei-led/archfit/internal/assessment/score"
	modevidence "github.com/alexei-led/archfit/internal/model/evidence"
	"github.com/alexei-led/archfit/internal/model/report"
)

// BaseEvidence is the ONLY data allowed to cross from the base sub-run into
// head output: observed finding IDs, analyzer coverage, and the run
// fingerprints.
// Base task paths, locations, validation commands, and declarations are dropped
// at the source — the base run is scored inside a temporary worktree that no
// longer exists when the head report is read, so any path from it is a lie.
type BaseEvidence struct {
	// FindingIDs are the base run's observed finding IDs, sorted, with fixed
	// entries removed.
	FindingIDs []string
	// Coverage and CoverageGaps are the base run's analyzer coverage evidence.
	Coverage     []modevidence.Coverage
	CoverageGaps []modevidence.CoverageGap
	// ConfigHash, ModelHash, and LabelsHash are the base run's fingerprints.
	// They cross with the finding IDs and coverage rows because they are the
	// only way the head side can tell a code change from a policy change.
	ConfigHash string
	ModelHash  string
	LabelsHash string
}

// attachBaseComparison checks the base ref out into a clean detached worktree,
// scores it with the same stages as head, and attaches the origin delta. The
// user's working tree is never mutated and the worktree is always removed.
//
// The base side measures the caller's EFFECTIVE head config, so config drift can
// never masquerade as code drift, and its diagnostic never leaves this function.
func (s StageExecutor) attachBaseComparison(ctx context.Context, req AnalysisRequest, runCtx AnalysisContext, diag *result.Result) (*score.Scorecard, error) {
	// A leading-dash ref would be parsed as a flag by rev-parse/worktree-add;
	// `git worktree add --detach <dir> --force` silently checks out HEAD and the
	// delta becomes HEAD-vs-HEAD. Reject rather than pass through.
	if strings.HasPrefix(req.BaseRef, "-") {
		return nil, &ExecutionError{Message: fmt.Sprintf("invalid --base ref %q", req.BaseRef)}
	}
	if s.Worktree == nil || s.NewBaseEvidence == nil {
		return nil, &ExecutionError{Message: "--base is not available in this build"}
	}
	if s.Progress != nil {
		s.reportPhase("Comparing against base")
	}
	bundleDir := runCtx.BundleDir
	if bundleDir == "" {
		bundleDir = filepath.Dir(runCtx.ConfigSource)
	}
	// The repository anchor is the RESOLVED scan root: it is inside the repo for
	// both the whole-repo and the --root-subtree case, so rev-parse finds the
	// same git root the head run analysed.
	baseRoot, cleanup, err := s.Worktree.Checkout(ctx, req.BaseRef, runCtx.Scope.Root, runCtx.ScanRoot)
	if cleanup != nil {
		defer cleanup()
	}
	if err != nil {
		return nil, &ExecutionError{Message: err.Error()}
	}
	card, evidence, err := s.scoreBaseTree(ctx, req, runCtx, bundleDir, baseRoot)
	if err != nil {
		return nil, &ExecutionError{Message: fmt.Sprintf("score base (%s): %v", req.BaseRef, err)}
	}
	diag.Comparison = decision.CompareFingerprints(req.BaseRef,
		decision.Fingerprints{ConfigHash: diag.ConfigHash, ModelHash: diag.ModelHash,
			LabelsHash: diag.LabelsHash, RubricVersion: report.ScoreVersion},
		decision.Fingerprints{ConfigHash: evidence.ConfigHash, ModelHash: evidence.ModelHash,
			LabelsHash: evidence.LabelsHash, RubricVersion: report.ScoreVersion})
	evaluation.AttachGitOrigin(diag, evaluation.GitOriginInput{
		BaseRef: req.BaseRef, BaseFindingIDs: evidence.FindingIDs,
		HeadCoverage: diag.ToolCoverage, HeadGaps: diag.CoverageGaps, HeadConfigHash: diag.ConfigHash,
		BaseCoverage: evidence.Coverage, BaseGaps: evidence.CoverageGaps, BaseConfigHash: evidence.ConfigHash,
		PrimaryTools: runCtx.PrimaryExtractorTools, Patterns: s.Analyzers.Patterns, Syntax: s.Analyzers.Syntax,
		SCIP: s.Analyzers.SCIP, Clones: s.Analyzers.Clones, CargoModules: s.Analyzers.CargoModules,
	})
	return &card, nil
}

// scoreBaseTree runs the full stage sequence over the base worktree and returns
// only the scorecard and the narrow base evidence. The base diagnostic is
// projected here and dropped: that is the structural guarantee that no base
// path, location, or validation command reaches head output.
//
// Progress is silenced — the head run already announced "Comparing against base"
// and re-emitting phases through the same reporter overflows its counter — and
// warnings are labelled so a base-side degradation is not misread as a head one.
func (s StageExecutor) scoreBaseTree(ctx context.Context, req AnalysisRequest, runCtx AnalysisContext, bundleDir, baseRoot string) (score.Scorecard, BaseEvidence, error) {
	sub := StageExecutor{Preparer: s.Preparer, Evidence: s.NewBaseEvidence(baseRoot), Stderr: s.Stderr}
	baseReq := AnalysisRequest{
		ConfigSource: runCtx.ConfigSource, BundleDir: bundleDir, Root: baseRoot,
		EvaluatedAt: runCtx.Now, EmptyBaseline: true,
		NoAdvisories: req.NoAdvisories, SuppressGateReasons: true, WarnLabel: baseWarnLabel,
	}
	acquired, err := sub.Evidence.Acquire(ctx, baseReq)
	if err != nil {
		return score.Scorecard{}, BaseEvidence{}, err
	}
	out, err := sub.assess(ctx, baseReq, acquired, Baseline{})
	if err != nil {
		return score.Scorecard{}, BaseEvidence{}, err
	}
	diag := out.Diagnostic
	return out.Score, BaseEvidence{
		FindingIDs:   evaluation.BaseFindingIDs(diag.Findings),
		Coverage:     diag.ToolCoverage,
		CoverageGaps: diag.CoverageGaps,
		ConfigHash:   diag.ConfigHash,
		ModelHash:    diag.ModelHash,
		LabelsHash:   diag.LabelsHash,
	}, nil
}

// baseWarnLabel prefixes every base-side stderr warning.
const baseWarnLabel = "[base] "
