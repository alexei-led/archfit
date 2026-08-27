package application

import (
	"context"
	"errors"
	"fmt"

	"github.com/alexei-led/archfit/internal/assessment/finding"
	"github.com/alexei-led/archfit/internal/assessment/result"
	"github.com/alexei-led/archfit/internal/assessment/state"
	"github.com/alexei-led/archfit/internal/assessment/status"
	"github.com/alexei-led/archfit/internal/model/report"
)

// BaselineFinding is the application-owned persistence view of an accepted finding.
type BaselineFinding struct {
	Fingerprint string
	RuleID      string
	Kind        string
	Severity    string
}

// BaselineMetric is one stored dimension metric.
type BaselineMetric struct {
	Name  string
	Value float64
	Unit  string
}

// BaselineDimension is one architecture-state dimension as the reference
// recorded it.
type BaselineDimension struct {
	Name             string
	Status           string
	Gate             string
	CoverageBasis    string
	CoverageObserved int
	CoverageTotal    int
	Metrics          []BaselineMetric
}

// BaselineStateSnapshot is the architecture-state reference a baseline stores.
//
// The four fingerprints travel with the facts they qualify. A dimension or seam
// delta may be claimed only when all four still match this run, so a policy
// change can never be reported as a code change.
type BaselineStateSnapshot struct {
	ConfigHash    string
	ModelHash     string
	LabelsHash    string
	RubricVersion string
	// HardGateFindingIDs are the reference's active blockers.
	HardGateFindingIDs []string
	// QualifyingSeamIDs are the reference's distributed-monolith seams.
	QualifyingSeamIDs []string
	Dimensions        []BaselineDimension
}

// BaselineSnapshot is the application contract passed to baseline persistence.
//
// It carries no repository score. Schema v2 retired the scalar gate, so a
// stored score would anchor nothing and would only invite a consumer to gate on
// it again.
type BaselineSnapshot struct {
	Accepted []BaselineFinding
	Metrics  report.MetricSnapshot
	State    *BaselineStateSnapshot
}

// BaselineLoader reads the accepted-findings baseline for a bundle. Persistence
// is an adapter concern; the application decides only whether to consult it.
type BaselineLoader interface {
	Load(ctx context.Context, bundleDir string) (Baseline, error)
}

// Baseline is the application-owned view of a persisted baseline: the accepted
// set lifecycle status assigns against, the metric anchor, and the
// architecture-state reference a comparison may use.
type Baseline struct {
	Accepted status.AcceptedSet
	Metrics  report.MetricSnapshot
	// Legacy marks a pre-state baseline. Its accepted fingerprints stay usable;
	// its retired scalar snapshot is ignored, and it can never support a
	// state/dimension/seam comparison.
	Legacy bool
	// State is the stored architecture-state reference. Nil for a legacy file
	// or when no baseline exists.
	State *BaselineStateSnapshot
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
	Stages StageExecutor
	Writer BaselineWriter
}

// Execute measures the configured tree and persists the accepted findings, the
// metric anchor, and the architecture-state reference.
//
// The capture runs against an EMPTY accepted set on purpose, so the file it
// writes is a function of the tree and the config alone. Reading the existing
// baseline made the capture self-referential: Balanced-Coupling advisories are
// rolled up per (module pair, strength, distance, volatility, STATUS), so
// accepting a group's representative split the group on the next run, exposed
// its siblings as new representatives, and wrote a different file every time.
// Two captures over an unchanged tree never settled.
func (s BaselineService) Execute(ctx context.Context, req BaselineRequest) (BaselineResponse, error) {
	if s.Stages.Preparer == nil || s.Stages.Evidence == nil || s.Writer == nil {
		return BaselineResponse{}, errors.New("baseline stages are required")
	}
	if req.Path == "" {
		return BaselineResponse{}, errors.New("baseline path is required")
	}
	out, err := s.Stages.Execute(ctx, AnalysisRequest{
		ConfigSource: req.ConfigPath, Root: req.Root, NoAdvisories: req.NoAdvisories,
		SuppressGateReasons: true, EmptyBaseline: true,
	})
	if err != nil {
		return BaselineResponse{}, fmt.Errorf("baseline analysis: %w", err)
	}
	doc := ProjectReport(out.Diagnostic, out.Score, out.BaseScore, out.HardGate)
	snapshot := BaselineSnapshot{Metrics: documentMetrics(doc), State: baselineState(out.Diagnostic)}
	for _, f := range doc.Findings {
		if f.Status == report.FindingStatusFixed || f.RuleID == finding.RuleIDCouplingGate {
			continue
		}
		kind := f.Kind
		if f.RuleID == finding.RuleIDBCImbalanced {
			kind = report.FindingKindAdvisory
		}
		snapshot.Accepted = append(snapshot.Accepted, BaselineFinding{Fingerprint: f.ID, RuleID: f.RuleID, Kind: kind, Severity: f.Severity})
	}
	if err := s.Writer.Save(ctx, req.Path, snapshot); err != nil {
		return BaselineResponse{}, fmt.Errorf("save baseline: %w", err)
	}
	return BaselineResponse{Path: req.Path}, nil
}

// baselineState projects the run into the architecture-state reference a later
// comparison reads. The fingerprints come from the same run that produced the
// facts, so a snapshot can never pair one run's seams with another run's hashes.
func baselineState(r result.Result) *BaselineStateSnapshot {
	out := &BaselineStateSnapshot{
		ConfigHash: r.ConfigHash, ModelHash: r.ModelHash, LabelsHash: r.LabelsHash,
		RubricVersion:      report.ScoreVersion,
		HardGateFindingIDs: []string{},
		QualifyingSeamIDs:  []string{},
		Dimensions:         make([]BaselineDimension, 0, state.DimensionCount),
	}
	for _, b := range r.State.Blockers {
		out.HardGateFindingIDs = append(out.HardGateFindingIDs, b.ID)
	}
	for _, seam := range r.Seams {
		if seam.DistributedMonolith {
			out.QualifyingSeamIDs = append(out.QualifyingSeamIDs, seam.ID)
		}
	}
	for _, dim := range r.State.Dimensions.All() {
		snap := BaselineDimension{
			Name: dim.Name, Status: string(dim.Status), Gate: string(dim.Gate),
			CoverageBasis: dim.Coverage.Basis, CoverageObserved: dim.Coverage.Observed, CoverageTotal: dim.Coverage.Total,
			Metrics: make([]BaselineMetric, 0, len(dim.Metrics)),
		}
		for _, m := range dim.Metrics {
			snap.Metrics = append(snap.Metrics, BaselineMetric{Name: m.Name, Value: m.Value, Unit: m.Unit})
		}
		out.Dimensions = append(out.Dimensions, snap)
	}
	return out
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
