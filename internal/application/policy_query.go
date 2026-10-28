package application

import (
	"context"
	"regexp"
	"slices"
	"strings"
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
	// Instead is what the owners of the denying rules declared to do instead.
	Instead []string
	// Candidates are the declared alternatives that resolve to a module or
	// package, each judged again by the rule pass from the same file.
	Candidates []Candidate
}

// Candidate is one owner-declared alternative that names a place the policy can
// judge. A permitted placement is not proof that the alternative does the same
// thing: the agent still has to read it.
type Candidate struct {
	// Path is the package or directory the alternative names.
	Path string
	// Module owns Path.
	Module string
	// PublicAPI says whether Path is part of Module's declared public surface.
	PublicAPI bool
	// PolicyCheck is the rule pass's answer for the import from the same file:
	// allowed, denied, not_decided, or unconstrained.
	PolicyCheck string
}

// pathToken finds the path-like words in an alternative's text.
var pathToken = regexp.MustCompile(`[A-Za-z0-9_.\-]+(?:/[A-Za-z0-9_.\-]+)+`)

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
		answer, err := s.judge(ctx, req, base, target)
		if err != nil {
			return nil, err
		}
		answer.Instead = instead(answer.Denials)
		if answer.Candidates, err = s.candidates(ctx, req, base, answer); err != nil {
			return nil, err
		}
		answers = append(answers, answer)
	}
	return answers, nil
}

// instead lists, once each, what the denying rules declare to do instead.
func instead(denials []evaluation.EdgeFinding) []string {
	var out []string
	for _, d := range denials {
		for _, alt := range d.Alternatives {
			if !slices.Contains(out, alt) {
				out = append(out, alt)
			}
		}
	}
	return out
}

// candidates resolves each declared alternative to the packages it names that
// a module owns, and asks the rule pass about the import of each from the same
// file. An alternative that names no owned package yields no candidate: advice
// is not a destination.
func (s PolicyQueryService) candidates(ctx context.Context, req CanImportRequest, base Baseline, answer CanImportAnswer) ([]Candidate, error) {
	moduleMap := req.Policy.Topology.ModuleMap
	var out []Candidate
	seen := map[string]bool{}
	for _, alt := range answer.Instead {
		for _, path := range pathToken.FindAllString(alt, -1) {
			path = strings.TrimSuffix(path, "/")
			module, owned := moduleMap.ModuleFor(path)
			if !owned {
				module, owned = moduleMap.ModuleFor(path + "/x")
			}
			if !owned || seen[path] || path == answer.Target || strings.HasSuffix(path, ".go") {
				continue
			}
			seen[path] = true
			// A candidate the extractor cannot spell (a directory for a language
			// that reads files, say) is not decided. It must not turn the
			// denial that named it into an error.
			check := evaluation.AnswerNotDecided
			judged, err := s.judge(ctx, req, base, path)
			switch {
			case err == nil:
				check = judged.Answer
			case ctx.Err() != nil:
				return nil, err
			}
			out = append(out, Candidate{Path: path, Module: module, PublicAPI: moduleMap.Exposes(module, path), PolicyCheck: check})
		}
	}
	return out, nil
}

// judge answers the import of one target from req.From.
func (s PolicyQueryService) judge(ctx context.Context, req CanImportRequest, base Baseline, target string) (CanImportAnswer, error) {
	edge, err := s.Edges.Edge(ctx, req.From, target)
	if err != nil {
		return CanImportAnswer{}, err
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
			return CanImportAnswer{}, err
		}
		answer.EdgeJudgment = boundJudgmentText(judged)
	}
	answer.Reasons = reportTexts(answer.Reasons, maxReportTextRunes)
	return answer, nil
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
		f.Alternatives = reportTexts(f.Alternatives, maxReportTaskTextRunes)
		out[i] = f
	}
	return out
}
