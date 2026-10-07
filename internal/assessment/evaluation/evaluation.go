// Package evaluation owns assessment decisions after relationship analysis.
// It consumes only the relationship contract and gathered signals; raw graph
// and coupling classifier internals never cross this boundary.
package evaluation

import (
	"time"

	"github.com/alexei-led/archfit/internal/assessment/finding"
	"github.com/alexei-led/archfit/internal/assessment/result"
	"github.com/alexei-led/archfit/internal/assessment/rules"
	signal "github.com/alexei-led/archfit/internal/assessment/signals"
	"github.com/alexei-led/archfit/internal/assessment/staleness"
	"github.com/alexei-led/archfit/internal/assessment/status"
	"github.com/alexei-led/archfit/internal/model/evidence"
	"github.com/alexei-led/archfit/internal/model/symbol"
	"github.com/alexei-led/archfit/internal/policy"
	"github.com/alexei-led/archfit/internal/relationship"
)

// Input is the assessment stage boundary. Every value is an assessment or
// relationship contract; adapters and graph internals stay outside this package.
type Input struct {
	Relationships relationship.Set
	Evidence      RuleEvidence
	Rules         Ruleset
	Metrics       Metricset
	Signals       signal.RunSignals
	Symbols       symbol.Graph
	Coverage      []evidence.Coverage
	ChangedFiles  []string
	Baseline      result.MetricSnapshot
	Accepted      status.AcceptedSet
	Policy        policy.AssessmentPolicy
	Gates         map[string]policy.MetricConfig
	// ModuleReview is the module_review gate mode: fail makes production
	// source no declared module owns a gate finding.
	ModuleReview       policy.GateMode
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

// evaluate applies rules, statuses, and metrics in their domain order.
func evaluate(in Input) Result {
	raw := checkRules(in.Rules, in.Relationships, in.Evidence)
	adv := candidateFindings(in.AdvisoryCandidates)
	// An uncovered-source gate finding joins the rule findings, so baseline
	// acceptance and waivers treat it as any other blocker; the advisory form
	// stays with the staleness advisories.
	for _, f := range uncoveredSource(in.Evidence, in.Policy, in.ModuleReview) {
		if f.Kind == finding.KindGate {
			raw = append(raw, f)
		} else {
			adv = append(adv, f)
		}
	}
	adv = append(adv, staleness.Check(in.Relationships, in.Policy, in.Now)...)
	adv = append(adv, staleLabelFindings(in.StaleLabelKeys)...)
	tagged := status.Assign(raw, in.Accepted, in.Policy.Waivers, in.Now, finding.KindGate, adv...)
	collected := signal.CollectedSignals{
		Common: signal.CommonInput{Relationships: in.Relationships, Findings: tagged, Baseline: in.Baseline, Coverage: signal.NewCoverageView(in.Coverage), ChangedFiles: in.ChangedFiles, Symbols: signal.SymbolSignals{Graph: in.Symbols}},
		Symbol: signal.SymbolSignals{Graph: in.Symbols}, Size: in.Signals.Size, Duplication: in.Signals.Duplication,
	}
	calculated := make([]result.MetricResult, 0, in.Metrics.Len())
	for _, metric := range in.Metrics.metrics {
		calculated = append(calculated, metric.Calculate(collected))
	}
	// A tripped ratchet is a finding like any other: it takes its status from
	// the waivers (never from the accepted set, which a capture cannot fill —
	// it runs with no baseline, so it has no delta to trip), and then joins the
	// gate and advisory populations below.
	tagged = append(tagged, status.Assign(ratchetFindings(calculated, in.Gates), status.Empty{}, in.Policy.Waivers, in.Now, finding.KindGate)...)
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
	taggedAdvisories := status.Assign(adv, in.Accepted, in.Policy.Waivers, in.Now, finding.KindAdvisory, raw...)
	adv = adv[:0]
	for _, f := range taggedAdvisories {
		if f.Kind == finding.KindAdvisory {
			adv = append(adv, f)
		}
	}
	adv = groupBCAdvisories(adv)
	tagged = resolveEvidence(in.Relationships, in.Policy.Topology.ModuleMap, tagged)
	// Split the rule pass: gatedRule stamps KindAdvisory on findings from a
	// `gate: warn` rule. Those are advisories, not gate findings — they are
	// hidden with --no-advisories and counted in summary.warnings, exactly like
	// the coupling/staleness advisories collected above.
	base := make([]finding.Finding, 0, len(tagged))
	ruleAdv := make([]finding.Finding, 0)
	for _, f := range tagged {
		if f.Kind == finding.KindAdvisory {
			ruleAdv = append(ruleAdv, f)
			continue
		}
		base = append(base, f)
	}
	gateNew, waiversUsed := 0, 0
	for _, f := range base {
		if f.Status == finding.StatusWaived {
			waiversUsed++
		}
		if f.Kind == finding.KindGate && f.Status != finding.StatusFixed && (f.Status == finding.StatusNew || f.Status == finding.StatusExpiredWaiver) {
			gateNew++
		}
	}
	visible := base
	warnings := 0
	if in.IncludeAdvisories {
		visible = append(append(append([]finding.Finding(nil), base...), adv...), ruleAdv...)
		warnings = countActive(adv) + countActive(ruleAdv)
	}
	var delta *result.DeltaReport
	if in.Delta {
		buckets := status.DeltaBuckets(visible, in.Accepted, in.ChangedFiles)
		if !buckets.Empty() {
			delta = &result.DeltaReport{New: buckets.New, Existing: buckets.Existing, Resolved: buckets.Resolved, SeverityChanged: buckets.SeverityChanged, TouchedByDelta: buckets.TouchedByDelta}
		}
	}
	return Result{Findings: visible, Metrics: calculated, Verdict: computeVerdict(gates, advisories), GateFindings: gateNew, Warnings: warnings, WaiversUsed: waiversUsed, Delta: delta}
}

// checkRules runs every compiled rule over the relationships. It is the one
// rule pass: check and `archfit policy can-import` both call it.
func checkRules(rs Ruleset, s relationship.Set, ev RuleEvidence) []finding.Finding {
	raw := make([]finding.Finding, 0, rs.Len())
	for _, rule := range rs.rules {
		raw = append(raw, rule.Check(s, rules.Evidence{
			PatternMatches: ev.PatternMatches, SyntaxFacts: ev.SyntaxFacts, FileClasses: ev.FileClasses,
			OutOfScopeFiles: ev.OutOfScopeFiles, UnwalkedSourceProduction: ev.UnwalkedSourceProduction,
		})...)
	}
	return raw
}

func countActive(in []finding.Finding) int {
	n := 0
	for _, f := range in {
		if f.Status != finding.StatusFixed {
			n++
		}
	}
	return n
}

// computeVerdict is the assessment verdict. A tripped metric ratchet is a gate
// (or advisory) finding like any other, so the verdict reads findings only.
func computeVerdict(gates []finding.Finding, advisories int) result.Verdict {
	for _, f := range gates {
		if f.Status == finding.StatusNew || f.Status == finding.StatusExpiredWaiver {
			return result.VerdictFail
		}
	}
	if advisories > 0 {
		return result.VerdictWarn
	}
	return result.VerdictPass
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
