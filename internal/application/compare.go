package application

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"time"

	"github.com/alexei-led/archfit/v3/internal/assessment/decision"
	"github.com/alexei-led/archfit/v3/internal/model/report"
	"github.com/alexei-led/archfit/v3/internal/policy"
)

// CompareRequest describes the current and candidate configurations.
type CompareRequest struct {
	CurrentConfig   string
	CandidateConfig string
	Root            string
	EvaluatedAt     time.Time
	// CurrentPolicy and CandidatePolicy are the two configurations'
	// declarations; the policy change is classified from them, not measured.
	CurrentPolicy, CandidatePolicy PolicyDeclarations
}

// PolicyDeclarations are the declarations that decide what blocks and what the
// analysis reads; the composition root reads them from a configuration.
type PolicyDeclarations struct {
	Rules       []policy.RuleDef
	Waivers     []policy.WaiverDef
	Modules     map[string]policy.ModuleDef
	MetricGates map[string]string
	Exclude     []string
	Languages   map[string]string
	Analyzers   map[string]string
	// ModuleReviewGate is module_review.gate.
	ModuleReviewGate string
}

// PolicyChange is the classified difference between two policies
// (decision.ComparePolicies), projected for the composition root.
type PolicyChange struct {
	Direction                             string
	RulesRemoved, RulesAdded              []PolicyRule
	RulesLoosened, RulesTightened         []PolicyRuleChange
	WaiversAdded, WaiversRemoved          []policy.WaiverDef
	AllowlistsWidened, AllowlistsNarrowed []PolicyAllowlist
	CoverageLost, CoverageGained          []PolicyCoverage
}

// PolicyRule names a rule and its effective gate.
type PolicyRule struct{ ID, Type, Gate string }

// PolicyRuleChange is one changed declaration of a rule.
type PolicyRuleChange struct{ ID, Field, From, To string }

// PolicyAllowlist is one changed allowlist.
type PolicyAllowlist struct{ Module, Field, From, To string }

// PolicyCoverage is one change in what the analysis reads.
type PolicyCoverage struct{ Kind, Detail string }

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
	PolicyChange       PolicyChange
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
		Current:    CompareSide{Document: ProjectReport(cur.Diagnostic, cur.Score)},
		Candidate:  CompareSide{Document: ProjectReport(cand.Diagnostic, cand.Score)},
		ScoreDelta: pure.ScoreDelta,
		Findings:   CompareFindings{CurrentOnlyIDs: pure.Findings.CurrentOnlyIDs, CandidateOnlyIDs: pure.Findings.CandidateOnlyIDs, BothIDs: pure.Findings.BothIDs},
		Coverage:   compareCoverage(pure.Coverage), Warnings: compareWarnings(pure.Warnings),
		PolicyChange: projectPolicyChange(decision.ComparePolicies(policyInput(req.CurrentPolicy), policyInput(req.CandidatePolicy))),
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

func policyInput(d PolicyDeclarations) decision.PolicyInput {
	return decision.PolicyInput{
		Rules: d.Rules, Waivers: d.Waivers, Modules: d.Modules, MetricGates: d.MetricGates,
		Exclude: d.Exclude, Languages: d.Languages, Analyzers: d.Analyzers, ModuleReviewGate: d.ModuleReviewGate,
	}
}

func projectPolicyChange(c decision.PolicyChange) PolicyChange {
	rules := func(in []decision.RuleRef) []PolicyRule {
		out := make([]PolicyRule, 0, len(in))
		for _, r := range in {
			out = append(out, PolicyRule{ID: r.ID, Type: r.Type, Gate: r.Gate})
		}
		return out
	}
	changes := func(in []decision.RuleChange) []PolicyRuleChange {
		out := make([]PolicyRuleChange, 0, len(in))
		for _, r := range in {
			out = append(out, PolicyRuleChange{ID: r.ID, Field: r.Field, From: r.From, To: r.To})
		}
		return out
	}
	allow := func(in []decision.AllowlistChange) []PolicyAllowlist {
		out := make([]PolicyAllowlist, 0, len(in))
		for _, a := range in {
			out = append(out, PolicyAllowlist{Module: a.Module, Field: a.Field, From: a.From, To: a.To})
		}
		return out
	}
	coverage := func(in []decision.CoverageChange) []PolicyCoverage {
		out := make([]PolicyCoverage, 0, len(in))
		for _, c := range in {
			out = append(out, PolicyCoverage{Kind: c.Kind, Detail: c.Detail})
		}
		return out
	}
	return PolicyChange{
		Direction:    string(c.Direction),
		RulesRemoved: rules(c.RulesRemoved), RulesAdded: rules(c.RulesAdded),
		RulesLoosened: changes(c.RulesLoosened), RulesTightened: changes(c.RulesTightened),
		WaiversAdded: append([]policy.WaiverDef{}, c.WaiversAdded...), WaiversRemoved: append([]policy.WaiverDef{}, c.WaiversRemoved...),
		AllowlistsWidened: allow(c.AllowlistsWidened), AllowlistsNarrowed: allow(c.AllowlistsNarrowed),
		CoverageLost: coverage(c.CoverageLost), CoverageGained: coverage(c.CoverageGained),
	}
}
