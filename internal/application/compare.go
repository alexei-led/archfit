package application

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"time"

	"github.com/alexei-led/archfit/internal/assessment/decision"
	"github.com/alexei-led/archfit/internal/model/report"
)

// CompareRequest describes the current and candidate configurations.
type CompareRequest struct {
	CurrentConfig   string
	CandidateConfig string
	Root            string
	EvaluatedAt     time.Time
}

// CompareSide contains one projected comparison document.
type CompareSide struct {
	Document report.Document
}

// CompareFindings partitions finding IDs by side.
type CompareFindings struct {
	CurrentOnlyIDs   []string
	CandidateOnlyIDs []string
	BothIDs          []string
}

// CompareCoverageDetail describes one analyzer's paired coverage.
type CompareCoverageDetail struct {
	Tool, Current, Candidate, Status, Reason string
}

// CompareCoverage summarizes analyzer comparability.
type CompareCoverage struct {
	Status  string
	Details []CompareCoverageDetail
}

// CompareWarning is a non-fatal comparison warning.
type CompareWarning struct{ Code, Text string }

// CompareResult is the application comparison response.
type CompareResult struct {
	Current, Candidate CompareSide
	ScoreDelta         *int
	Findings           CompareFindings
	Coverage           CompareCoverage
	Warnings           []CompareWarning
}

// CompareService owns the config comparison use case.
type CompareService struct {
	Current   StageExecutor
	Candidate StageExecutor
}

// Execute prepares and runs both staged measurements against the same
// evaluation instant, then projects the pure comparison into application-owned
// report DTOs.
func (s CompareService) Execute(ctx context.Context, req CompareRequest) (CompareResult, error) {
	if s.Current.Preparer == nil || s.Current.Evidence == nil ||
		s.Candidate.Preparer == nil || s.Candidate.Evidence == nil {
		return CompareResult{}, errors.New("comparison stages are required")
	}
	bundleDir := filepath.Dir(req.CurrentConfig)
	cur, err := s.Current.Execute(ctx, AnalysisRequest{ConfigSource: req.CurrentConfig, BundleDir: bundleDir, Root: req.Root, EvaluatedAt: req.EvaluatedAt, EmptyBaseline: true, SuppressGateReasons: true, WarnLabel: "[current] "})
	if err != nil {
		return CompareResult{}, fmt.Errorf("measure current config: %w", err)
	}
	cand, err := s.Candidate.Execute(ctx, AnalysisRequest{ConfigSource: req.CandidateConfig, BundleDir: bundleDir, Root: req.Root, EvaluatedAt: req.EvaluatedAt, EmptyBaseline: true, SuppressGateReasons: true, WarnLabel: "[candidate] "})
	if err != nil {
		return CompareResult{}, fmt.Errorf("measure candidate config: %w", err)
	}
	pure := decision.CompareConfigs(decision.ConfigCompareInput{
		Current:   decision.ConfigCompareSide{Diag: cur.Diagnostic, Score: cur.Score},
		Candidate: decision.ConfigCompareSide{Diag: cand.Diagnostic, Score: cand.Score},
	})
	return CompareResult{
		Current:    CompareSide{Document: ProjectReport(cur.Diagnostic, cur.Score, nil, cur.HardGate)},
		Candidate:  CompareSide{Document: ProjectReport(cand.Diagnostic, cand.Score, nil, cand.HardGate)},
		ScoreDelta: pure.ScoreDelta,
		Findings:   CompareFindings{CurrentOnlyIDs: pure.Findings.CurrentOnlyIDs, CandidateOnlyIDs: pure.Findings.CandidateOnlyIDs, BothIDs: pure.Findings.BothIDs},
		Coverage:   compareCoverage(pure.Coverage), Warnings: compareWarnings(pure.Warnings),
	}, nil
}

func compareCoverage(in decision.ConfigCompareCoverage) CompareCoverage {
	out := CompareCoverage{Status: string(in.Status), Details: make([]CompareCoverageDetail, 0, len(in.Details))}
	for _, d := range in.Details {
		out.Details = append(out.Details, CompareCoverageDetail{Tool: d.Tool, Current: d.Current, Candidate: d.Candidate, Status: string(d.Status), Reason: d.Reason})
	}
	return out
}
func compareWarnings(in []decision.ConfigCompareWarning) []CompareWarning {
	out := make([]CompareWarning, 0, len(in))
	for _, w := range in {
		out = append(out, CompareWarning{Code: w.Code, Text: w.Text})
	}
	return out
}
