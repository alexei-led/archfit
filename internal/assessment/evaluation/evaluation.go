// Package evaluation owns assessment decisions after relationship analysis.
// It consumes only the relationship contract and gathered signals; raw graph
// and coupling classifier internals never cross this boundary.
package evaluation

import (
	"time"

	"github.com/alexei-led/archfit/internal/assessment/finding"
	"github.com/alexei-led/archfit/internal/assessment/metrics"
	"github.com/alexei-led/archfit/internal/assessment/result"
	"github.com/alexei-led/archfit/internal/assessment/rules"
	signal "github.com/alexei-led/archfit/internal/assessment/signals"
	"github.com/alexei-led/archfit/internal/assessment/staleness"
	"github.com/alexei-led/archfit/internal/assessment/status"
	"github.com/alexei-led/archfit/internal/model/symbol"
	"github.com/alexei-led/archfit/internal/policy"
	"github.com/alexei-led/archfit/internal/relationship"
)

// Input is the assessment stage boundary. Every value is an assessment or
// relationship contract; adapters and graph internals stay outside this package.
type Input struct {
	Relationships      relationship.Set
	Evidence           rules.Evidence
	Rules              []rules.Rule
	Metrics            []metrics.Metric
	Signals            signal.RunSignals
	Symbols            symbol.Graph
	Coverage           []result.Coverage
	ChangedFiles       []string
	Baseline           result.MetricSnapshot
	Accepted           status.AcceptedSet
	Policy             policy.AssessmentPolicy
	Gates              map[string]policy.MetricConfig
	Now                time.Time
	AdvisoryCandidates []relationship.AdvisoryCandidate
	StaleLabelKeys     []string
	IncludeAdvisories  bool
	Delta              bool
}

// Result contains gate findings, metric values, and the verdict inputs produced
// by assessment. Report assembly is deliberately left to the pipeline.
type Result struct {
	Findings     []finding.Finding
	Metrics      []result.MetricResult
	Verdict      result.Verdict
	GateFindings int
	Warnings     int
	WaiversUsed  int
	Delta        *result.DeltaReport
}

// Evaluate applies rules, statuses, and metrics in their domain order.
func Evaluate(in Input) Result {
	raw := make([]finding.Finding, 0, len(in.Rules))
	for _, rule := range in.Rules {
		raw = append(raw, rule.Check(in.Relationships, in.Evidence)...)
	}
	tagged := status.Assign(raw, in.Accepted, in.Policy.Waivers, in.Now, finding.KindGate)
	collected := signal.CollectedSignals{
		Common: signal.CommonInput{Relationships: in.Relationships, Findings: tagged, Baseline: in.Baseline, Coverage: signal.NewCoverageView(in.Coverage), ChangedFiles: in.ChangedFiles, Symbols: signal.SymbolSignals{Graph: in.Symbols}},
		Symbol: signal.SymbolSignals{Graph: in.Symbols}, Size: in.Signals.Size, Duplication: in.Signals.Duplication,
	}
	calculated := make([]result.MetricResult, 0, len(in.Metrics))
	for _, metric := range in.Metrics {
		calculated = append(calculated, metric.Calculate(collected))
	}
	gates := make([]finding.Finding, 0, len(tagged))
	advisories := 0
	for _, f := range tagged {
		if f.Kind == finding.KindGate && f.Status != finding.StatusFixed {
			gates = append(gates, f)
		}
		if f.Kind == finding.KindAdvisory && f.Status != finding.StatusFixed {
			advisories++
		}
	}
	adv := candidateFindings(in.AdvisoryCandidates)
	adv = append(adv, staleness.Check(in.Relationships, in.Policy, in.Now)...)
	adv = append(adv, staleLabelFindings(in.StaleLabelKeys)...)
	taggedAdvisories := status.Assign(adv, in.Accepted, in.Policy.Waivers, in.Now, finding.KindAdvisory)
	adv = adv[:0]
	for _, f := range taggedAdvisories {
		if f.Kind == finding.KindAdvisory {
			adv = append(adv, f)
		}
	}
	adv = groupBCAdvisories(adv)
	tagged = resolveEvidence(in.Relationships, tagged)
	gateNew, waiversUsed := 0, 0
	for _, f := range tagged {
		if f.Status == finding.StatusWaived {
			waiversUsed++
		}
		if f.Kind == finding.KindGate && f.Status != finding.StatusFixed && (f.Status == finding.StatusNew || f.Status == finding.StatusExpiredWaiver) {
			gateNew++
		}
	}
	visible := tagged
	warnings := 0
	if in.IncludeAdvisories {
		visible = append(append([]finding.Finding(nil), tagged...), adv...)
		for _, f := range adv {
			if f.Status != finding.StatusFixed {
				warnings++
			}
		}
	}
	var delta *result.DeltaReport
	if in.Delta {
		buckets := status.DeltaBuckets(visible, in.Accepted, in.ChangedFiles)
		if !buckets.Empty() {
			delta = &result.DeltaReport{New: buckets.New, Existing: buckets.Existing, Resolved: buckets.Resolved, SeverityChanged: buckets.SeverityChanged, TouchedByDelta: buckets.TouchedByDelta}
		}
	}
	return Result{Findings: visible, Metrics: calculated, Verdict: computeVerdict(gates, calculated, in.Gates, advisories), GateFindings: gateNew, Warnings: warnings, WaiversUsed: waiversUsed, Delta: delta}
}

func computeVerdict(gates []finding.Finding, ms []result.MetricResult, cfg map[string]policy.MetricConfig, advisories int) result.Verdict {
	for _, f := range gates {
		if f.Status == finding.StatusNew || f.Status == finding.StatusExpiredWaiver {
			return result.VerdictFail
		}
	}
	verdict := result.VerdictPass
	for _, m := range ms {
		if m.Delta == nil {
			continue
		}
		c := cfg[m.Name]
		if c.Gate == string(policy.GateOff) {
			continue
		}
		breached := *m.Delta < -metricMinDelta(c)
		if m.Direction == result.DirectionHigherIsWorse {
			breached = *m.Delta > float64(metricMaxNew(c))
		}
		if !breached {
			continue
		}
		if c.Gate == string(policy.GateWarn) {
			verdict = result.VerdictWarn
			continue
		}
		return result.VerdictFail
	}
	if verdict == result.VerdictPass && advisories > 0 {
		return result.VerdictWarn
	}
	return verdict
}
func metricMinDelta(c policy.MetricConfig) float64 {
	if c.MinDelta != nil {
		return *c.MinDelta
	}
	return 0
}
func metricMaxNew(c policy.MetricConfig) int {
	if c.MaxNew != nil {
		return *c.MaxNew
	}
	return 0
}
