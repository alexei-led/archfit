package application

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"

	"github.com/alexei-led/archfit/internal/model/report"
)

// ExplainRequest identifies a finding to explain.
type ExplainRequest struct {
	ConfigPath  string
	Root        string
	Fingerprint string
}

// ExplainResponse contains the matched finding and report.
type ExplainResponse struct {
	Finding  report.Finding
	Document report.Document
}

// ExplainService owns the explain use case.
type ExplainService struct {
	Stages StageExecutor
}

// Execute reruns the same technical measurement as the gate and resolves a
// fingerprint prefix deterministically. Multiple matches are ordered by ID.
func (s ExplainService) Execute(ctx context.Context, req ExplainRequest) (ExplainResponse, error) {
	if s.Stages.Preparer == nil || s.Stages.Evidence == nil {
		return ExplainResponse{}, errors.New("explain stages are required")
	}
	if strings.TrimSpace(req.Fingerprint) == "" {
		return ExplainResponse{}, errors.New("finding fingerprint prefix is required")
	}
	out, err := s.Stages.Execute(ctx, AnalysisRequest{ConfigSource: req.ConfigPath, Root: req.Root, SuppressGateReasons: true})
	if err != nil {
		return ExplainResponse{}, fmt.Errorf("explain analysis: %w", err)
	}
	doc := ProjectReport(out.Diagnostic, out.Score, out.BaseScore, out.HardGate)
	matches := make([]report.Finding, 0, 1)
	for _, f := range doc.Findings {
		if strings.HasPrefix(f.ID, req.Fingerprint) {
			matches = append(matches, f)
		}
	}
	if len(matches) == 0 {
		return ExplainResponse{}, fmt.Errorf("no finding with fingerprint prefix %q", req.Fingerprint)
	}
	sort.Slice(matches, func(i, j int) bool { return matches[i].ID < matches[j].ID })
	return ExplainResponse{Finding: matches[0], Document: doc}, nil
}
