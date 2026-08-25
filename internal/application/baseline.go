package application

import (
	"context"
	"errors"
	"fmt"

	"github.com/alexei-led/archfit/internal/model/report"
)

// BaselineFinding is the application-owned persistence view of an accepted finding.
type BaselineFinding struct {
	Fingerprint string
	RuleID      string
	Kind        string
	Severity    string
}

// BaselineScore is the application-owned score snapshot view.
type BaselineScore struct {
	CouplingBalance int
	Band            string
	ScoreVersion    string
	RubricVersion   int
}

// BaselineSnapshot is the application contract passed to baseline persistence.
type BaselineSnapshot struct {
	Accepted []BaselineFinding
	Metrics  report.MetricSnapshot
	Score    *BaselineScore
}

// BaselineWriter persists an application baseline snapshot.
type BaselineWriter interface {
	Save(context.Context, string, BaselineSnapshot) error
}

// BaselineRequest describes a baseline capture.
type BaselineRequest struct {
	ConfigPath   string
	Root         string
	Path         string
	NoAdvisories bool
}

// BaselineResponse identifies the persisted baseline.
type BaselineResponse struct {
	Path string
}

// BaselineService owns the baseline use case.
type BaselineService struct {
	Preparer     PolicyPreparer
	Evidence     EvidenceStage
	Relationship RelationshipStage
	Assessment   AssessmentStage
	Writer       BaselineWriter
}

// Execute measures the configured tree, then persists the native finding kind
// and score snapshot used by later gate checks.
func (s BaselineService) Execute(ctx context.Context, req BaselineRequest) (BaselineResponse, error) {
	if s.Preparer == nil || s.Evidence == nil || s.Relationship == nil || s.Assessment == nil || s.Writer == nil {
		return BaselineResponse{}, errors.New("baseline stages are required")
	}
	if req.Path == "" {
		return BaselineResponse{}, errors.New("baseline path is required")
	}
	out, err := (StageExecutor{Preparer: s.Preparer, Evidence: s.Evidence, Relationship: s.Relationship, Assessment: s.Assessment}).Execute(ctx, AnalysisRequest{ConfigSource: req.ConfigPath, Root: req.Root, NoAdvisories: req.NoAdvisories, SuppressGateReasons: true})
	if err != nil {
		return BaselineResponse{}, fmt.Errorf("baseline analysis: %w", err)
	}
	doc := ProjectReport(out.Diagnostic, out.Score, out.BaseScore, out.HardGate)
	snapshot := BaselineSnapshot{Metrics: documentMetrics(doc)}
	for _, f := range doc.Findings {
		if f.Status == report.FindingStatusFixed || f.RuleID == "bc/coupling_gate" {
			continue
		}
		kind := f.Kind
		if f.RuleID == "bc/imbalanced_coupling" {
			kind = report.FindingKindAdvisory
		}
		snapshot.Accepted = append(snapshot.Accepted, BaselineFinding{Fingerprint: f.ID, RuleID: f.RuleID, Kind: kind, Severity: f.Severity})
	}
	if !out.Score.OverallBand.Unmeasured() {
		snapshot.Score = &BaselineScore{CouplingBalance: out.Score.Overall, Band: string(out.Score.OverallBand), ScoreVersion: report.ScoreVersion, RubricVersion: out.Score.RubricVersion}
	}
	if err := s.Writer.Save(ctx, req.Path, snapshot); err != nil {
		return BaselineResponse{}, fmt.Errorf("save baseline: %w", err)
	}
	return BaselineResponse{Path: req.Path}, nil
}

func documentMetrics(doc report.Document) report.MetricSnapshot {
	out := make(report.MetricSnapshot, len(doc.Metrics))
	for _, m := range doc.Metrics {
		out[m.Name] = struct {
			Value   float64 `json:"value"`
			Version string  `json:"version"`
		}{Value: m.Value, Version: m.Version}
	}
	return out
}
