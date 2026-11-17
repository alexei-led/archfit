package application

import (
	"sort"

	"github.com/alexei-led/archfit/v4/internal/assessment/finding"
	"github.com/alexei-led/archfit/v4/internal/assessment/result"
	"github.com/alexei-led/archfit/v4/internal/assessment/score"
	"github.com/alexei-led/archfit/v4/internal/assessment/state"
	"github.com/alexei-led/archfit/v4/internal/model/evidence"
	"github.com/alexei-led/archfit/v4/internal/model/report"
)

// projectReport converts an assessment result plus its synthesised scorecard
// into the single stable external report contract renderers consume.
func projectReport(r result.Result, sc score.Scorecard) report.Document {
	doc := report.Document{
		SchemaVersion: r.SchemaVersion, Base: r.Base, Head: r.Head,
		ConfigHash: r.ConfigHash, Metrics: projectMetrics(r.Metrics), Findings: projectFindings(r.Findings),
		FileFacts: projectFileFacts(r.FileFacts), DynamicImports: projectDynamicImports(r.DynamicImports), Connascence: projectConnascence(r.Connascence),
		DynamicConnascenceSignals: projectDynamicConnascenceSignals(r.DynamicConnascenceSignals), RuntimeAsync: projectRuntimeAsyncModules(r.RuntimeAsync),
		RuntimeAsyncEdges: projectRuntimeAsyncEdges(r.RuntimeAsyncEdges), DeprecatedDeps: projectDeprecatedDeps(r.DeprecatedDeps),
		SyntaxFacts:  projectSyntaxFacts(r.SyntaxFacts),
		ToolCoverage: projectCoverage(r.ToolCoverage), CoverageGaps: projectCoverageGaps(r.CoverageGaps), OwnerSource: r.OwnerSource,
		PrimaryExtractorTools: r.PrimaryExtractorTools, ConfigWarnings: r.ConfigWarnings,
		ClassifiedEdges: projectClassifiedEdges(r.ClassifiedEdges), DistanceContext: projectDistanceContext(r.DistanceContext),
		DistanceConfigCandidates: projectDistanceConfigCandidates(r.DistanceConfigCandidates),
		LocalCoupling:            projectLocalCoupling(r.LocalCoupling), Summary: report.Summary(r.Summary),
	}
	doc.Score = projectScorecard(sc)
	doc.State = projectArchitectureState(r, doc)
	boundReportText(&doc)
	return doc
}

// IdentityInputs are the identity facts the caller reads from disk and git. An
// empty value means the run could not know it; the report omits it.
type IdentityInputs struct {
	PolicyDigest   string
	PolicyRef      *report.PolicyRef
	BaseSHA        string
	HeadTreeDigest string
	EngineVersion  string
}

// withRunFacts stamps the document's state with what the run was judged under.
func withRunFacts(doc report.Document, in IdentityInputs) report.Document {
	doc.State.Identity = report.ResultIdentity{
		SchemaVersion: report.IdentitySchemaVersion,
		PolicyDigest:  in.PolicyDigest,
		PolicyRef:     in.PolicyRef, BaseSHA: in.BaseSHA, HeadTreeDigest: in.HeadTreeDigest,
		EngineVersion: in.EngineVersion,
	}
	return doc
}

// projectArchitectureState projects the assessment-owned architecture state
// onto the external archfit.architecture-state.v2 contract, and adds the run
// facts assessment deliberately never sees: which tree was measured, which tool
// versions produced the evidence.
//
// Each measurement envelope is copied verbatim. Repository rule errors and
// exceptions are supplied by repostate, not derived by this projection.
func projectArchitectureState(r result.Result, doc report.Document) report.ArchitectureState {
	out := report.NewArchitectureState()
	out.Dimensions = projectStateDimensions(r.State.Dimensions)
	out.Findings = doc.Findings
	out.Measurement.SourceRef = sourceWorktree
	out.Measurement.ToolVersions, out.Coverage.Tools = projectStateToolCoverage(r.ToolCoverage)

	out.Seams = projectStateSeams(r.Seams)
	out.Evidence = projectEvidence(r)

	out.Coverage.Measured, out.Coverage.Partial, out.Coverage.Unmeasured = out.Dimensions.CountStatuses()
	return out
}

// projectStateSeams copies the seam ledger in the order assessment built it.
// The ledger is evidence, not a decision: no seam is dropped, reordered, or
// summarised here, so a reader sees exactly the pairs the run classified.
func projectStateSeams(in []result.Seam) []report.Seam {
	out := make([]report.Seam, 0, len(in))
	for _, s := range in {
		out = append(out, report.Seam{
			ID: s.ID, FromModule: s.FromModule, ToModule: s.ToModule,
			Edges: s.Edges, ScoredEdges: s.ScoredEdges, AbstainedEdges: s.AbstainedEdges,
			Strength: s.Strength, Distance: s.Distance, Volatility: s.Volatility,
			Severity: s.Severity,
			RawDistance: report.SeamDistance{
				Level: s.RawDistance.Level, Basis: s.RawDistance.Basis,
				FromOwner: s.RawDistance.FromOwner, ToOwner: s.RawDistance.ToOwner, SameOwner: s.RawDistance.SameOwner,
				FromDeployUnit: s.RawDistance.FromDeployUnit, ToDeployUnit: s.RawDistance.ToDeployUnit,
				SameDeployUnit:    s.RawDistance.SameDeployUnit,
				BoundaryCrossings: s.RawDistance.BoundaryCrossings, SharedAncestor: s.RawDistance.SharedAncestor,
			},
			Quadrant: s.Quadrant,
			Scores: report.SeamScoreDistribution{
				N: s.Scores.N, Min: s.Scores.Min, Median: s.Scores.Median, Max: s.Scores.Max,
				P10: s.Scores.P10, P90: s.Scores.P90, Mean: s.Scores.Mean,
			},
			CriticalEdges: s.CriticalEdges, HighOrWorseEdges: s.HighOrWorseEdges,
			CriticalSharePct: s.CriticalSharePct, HighOrWorseSharePct: s.HighOrWorseSharePct,
			Confidence:      s.Confidence,
			RoleExpectation: s.RoleExpectation, Hypothesis: s.Hypothesis,
			DistributedMonolith: s.DistributedMonolith,
		})
	}
	return out
}

// projectStateDimensions copies the nine envelopes in contract order. The
// assessment and report types are separate by architecture rule, not by
// accident: assessment must not import the report DTOs, so this is the one
// place the two vocabularies meet.
func projectStateDimensions(in state.Dimensions) report.Dimensions {
	src := in.All()
	out := report.Dimensions{}
	dst := []*report.DimensionState{
		&out.Intent, &out.Structure, &out.Modularity, &out.Coupling, &out.ChangeLocality,
		&out.Complexity, &out.Testability, &out.Operations, &out.Drift,
	}
	for i, dim := range src {
		*dst[i] = projectStateDimension(dim)
	}
	return out
}

func projectStateDimension(in state.Dimension) report.DimensionState {
	out := report.DimensionState{
		Name: in.Name, Owner: in.Owner,
		Status: report.MeasurementStatus(in.Status), Confidence: report.Confidence(in.Confidence),
		Coverage: report.DimensionCoverage{Basis: in.Coverage.Basis, Observed: in.Coverage.Observed, Total: in.Coverage.Total},
		Metrics:  make([]report.MetricValue, 0, len(in.Metrics)),
		Findings: make([]report.FindingRef, 0, len(in.Findings)),
		Unknown:  make([]report.UnknownFact, 0, len(in.Unknown)),
	}
	for _, m := range in.Metrics {
		metric := report.MetricValue{Name: m.Name, Value: m.Value, Unit: m.Unit, Provenance: m.Provenance}
		if m.Denominator != nil {
			metric.Denominator = &report.MetricDenominator{Observed: m.Denominator.Observed, Total: m.Denominator.Total}
		}
		out.Metrics = append(out.Metrics, metric)
	}
	for _, f := range in.Findings {
		out.Findings = append(out.Findings, report.FindingRef{ID: f.ID, RuleID: f.RuleID, Kind: f.Kind, Severity: f.Severity, Status: f.Status})
	}
	for _, u := range in.Unknown {
		out.Unknown = append(out.Unknown, report.UnknownFact{Fact: u.Fact, Reason: u.Reason, Owner: u.Owner})
	}
	return out
}

// sourceWorktree is the measurement source ref of every run. Archfit measures
// the files on disk and never resolves HEAD, so naming a commit here would
// assert that the measured bytes equal that commit — false the moment the tree
// is dirty.
const sourceWorktree = "worktree"

// projectStateToolCoverage splits the existing coverage rows into the two views
// the state contract keeps separate: deterministic tool versions (a measurement
// fact) and per-tool status (an evidence-coverage fact). Rows stay in the order
// acquisition produced them, and a version is recorded only when the tool
// reported one.
func projectStateToolCoverage(in []evidence.Coverage) (map[string]string, []report.StateToolCoverage) {
	versions := make(map[string]string, len(in))
	tools := make([]report.StateToolCoverage, 0, len(in))
	for _, c := range in {
		if c.Version != "" {
			versions[c.Tool] = c.Version
		}
		tools = append(tools, report.StateToolCoverage{Tool: c.Tool, Status: c.Status, Reason: c.Reason, Details: c.Details})
	}
	return versions, tools
}

func projectScorecard(in score.Scorecard) report.Scorecard {
	out := report.Scorecard{RubricVersion: in.RubricVersion, Overall: in.Overall, OverallBand: report.ScoreBand(in.OverallBand), Dimensions: make([]report.Dimension, len(in.Dimensions))}
	for i, d := range in.Dimensions {
		out.Dimensions[i] = report.Dimension{Name: d.Name, Value: d.Value, Band: report.ScoreBand(d.Band), Confidence: report.Confidence(d.Confidence), Evidence: d.Evidence, Summary: d.Summary, RawValue: d.RawValue, CapApplied: d.CapApplied, Meta: d.Meta}
	}
	return out
}

func projectCoverage(in []evidence.Coverage) []report.Coverage {
	out := make([]report.Coverage, len(in))
	for i, v := range in {
		out[i] = report.Coverage{Tool: v.Tool, Version: v.Version, FilesSeen: v.FilesSeen, FilesApplicable: v.FilesApplicable, Unresolved: v.Unresolved, SpecifiersSeen: v.SpecifiersSeen, UnresolvedInputsMissing: v.UnresolvedInputsMissing, UnresolvedPrecisionOnly: v.UnresolvedPrecisionOnly, Status: v.Status, Reason: v.Reason, Details: v.Details}
	}
	return out
}

func projectCoverageGaps(in []evidence.CoverageGap) []report.CoverageGap {
	out := make([]report.CoverageGap, len(in))
	for i, v := range in {
		out[i] = report.CoverageGap{Tool: v.Tool, InstallCmd: v.InstallCmd, AffectedMetrics: v.AffectedMetrics, Gate: v.Gate}
	}
	return out
}

func projectFileFacts(in []evidence.FileFact) []report.FileFact {
	out := make([]report.FileFact, len(in))
	for i, v := range in {
		out[i] = report.FileFact{Module: v.Module, Files: v.Files, InboundModuleFanIn: v.InboundModuleFanIn, OutboundDestinations: v.OutboundDestinations, LOC: v.LOC}
	}
	return out
}

func projectDynamicImports(in []evidence.DynamicImport) []report.DynamicImport {
	out := make([]report.DynamicImport, len(in))
	for i, v := range in {
		sites := make([]report.DynamicImportSite, len(v.Sites))
		for j, s := range v.Sites {
			sites[j] = report.DynamicImportSite{File: s.File, Line: s.Line, Kind: s.Kind, Language: s.Language}
		}
		out[i] = report.DynamicImport{Module: v.Module, Count: v.Count, Sites: sites}
	}
	return out
}

func projectSyntaxFacts(in []evidence.SyntaxFact) []report.SyntaxFact {
	out := make([]report.SyntaxFact, len(in))
	for i, v := range in {
		out[i] = report.SyntaxFact{Language: v.Language, File: v.File, Module: v.Module, Kind: v.Kind, Name: v.Name, Exported: v.Exported, StartLine: v.StartLine, EndLine: v.EndLine, Count: v.Count, Framework: v.Framework, FrameworkConfirmed: v.FrameworkConfirmed}
	}
	return out
}

func projectDeprecatedDeps(in []evidence.DeprecatedDep) []report.DeprecatedDep {
	out := make([]report.DeprecatedDep, len(in))
	for i, v := range in {
		out[i] = report.DeprecatedDep{File: v.File, Kind: v.Kind, Subject: v.Subject, Note: v.Note}
	}
	return out
}

func projectDynamicConnascenceSignals(in *evidence.DynamicConnascenceSignals) *report.DynamicConnascenceSignals {
	if in == nil {
		return nil
	}
	out := &report.DynamicConnascenceSignals{Unmeasured: in.Unmeasured, ReportOnlyReason: in.ReportOnlyReason, Signals: make([]report.DynamicConnascenceSignal, len(in.Signals))}
	for i, v := range in.Signals {
		sites := make([]report.DynamicConnascenceSite, len(v.Sites))
		for j, s := range v.Sites {
			sites[j] = report.DynamicConnascenceSite{File: s.File, Line: s.Line, Kind: s.Kind, Language: s.Language, Target: s.Target}
		}
		out.Signals[i] = report.DynamicConnascenceSignal{Kind: v.Kind, RelatedConnascence: v.RelatedConnascence, Measured: v.Measured, ReportOnlyReason: v.ReportOnlyReason, Module: v.Module, Target: v.Target, IntegrationKind: v.IntegrationKind, Count: v.Count, Sites: sites}
	}
	return out
}

func projectRuntimeAsyncModules(in []evidence.RuntimeAsyncModule) []report.RuntimeAsyncModule {
	out := make([]report.RuntimeAsyncModule, len(in))
	for i, v := range in {
		out[i] = report.RuntimeAsyncModule{Module: v.Module, IntegrationKind: v.IntegrationKind, Count: v.Count, Confidence: v.Confidence}
	}
	return out
}

func projectRuntimeAsyncEdges(in []evidence.RuntimeAsyncEdge) []report.RuntimeAsyncEdge {
	out := make([]report.RuntimeAsyncEdge, len(in))
	for i, v := range in {
		sites := make([]report.RuntimeAsyncSite, len(v.Sites))
		for j, s := range v.Sites {
			sites[j] = report.RuntimeAsyncSite{File: s.File, Line: s.Line, Library: s.Library, IntegrationKind: s.IntegrationKind, Language: s.Language}
		}
		out[i] = report.RuntimeAsyncEdge{FromModule: v.FromModule, Target: v.Target, IntegrationKind: v.IntegrationKind, Count: v.Count, Confidence: v.Confidence, Sites: sites}
	}
	return out
}

func projectConnascence(in *evidence.ConnascenceReport) *report.ConnascenceReport {
	if in == nil {
		return nil
	}
	out := &report.ConnascenceReport{EdgesWithEvidence: in.EdgesWithEvidence, AbstainedEdges: in.AbstainedEdges, TotalEvidence: in.TotalEvidence, ByKind: in.ByKind, BySource: in.BySource, Unmeasured: in.Unmeasured, Roadmap: make([]report.ConnascenceRoadmapItem, len(in.Roadmap))}
	for i, v := range in.Roadmap {
		out.Roadmap[i] = report.ConnascenceRoadmapItem{Kind: v.Kind, CurrentStatus: v.CurrentStatus, Sources: v.Sources, RelatedSignals: v.RelatedSignals, UpgradeTrigger: v.UpgradeTrigger}
	}
	return out
}

func projectDistanceContext(in *evidence.DistanceContext) *report.DistanceContext {
	if in == nil {
		return nil
	}
	return &report.DistanceContext{OwnerModel: in.OwnerModel, DistanceBasis: in.DistanceBasis, DeployUnitDetectedModules: in.DeployUnitDetectedModules, DeclaredExternalSystems: in.DeclaredExternalSystems, RuntimeAsyncRelations: in.RuntimeAsyncRelations, RuntimeAsyncKinds: in.RuntimeAsyncKinds, Interpretation: in.Interpretation, RuntimeInterpretation: in.RuntimeInterpretation}
}

func projectDistanceConfigCandidates(in []evidence.DistanceConfigCandidate) []report.DistanceConfigCandidate {
	out := make([]report.DistanceConfigCandidate, len(in))
	for i, v := range in {
		sites := make([]report.DistanceConfigEvidenceSite, len(v.EvidenceSites))
		for j, s := range v.EvidenceSites {
			sites[j] = report.DistanceConfigEvidenceSite{File: s.File, Line: s.Line, Kind: s.Kind, Language: s.Language, Target: s.Target}
		}
		out[i] = report.DistanceConfigCandidate{SourceBlock: v.SourceBlock, Module: v.Module, Target: v.Target, IntegrationKind: v.IntegrationKind, Count: v.Count, EvidenceSites: sites, SuggestedReviewAction: v.SuggestedReviewAction}
	}
	return out
}

func projectLocalCoupling(in []evidence.LocalCouplingModule) []report.LocalCouplingModule {
	out := make([]report.LocalCouplingModule, len(in))
	for i, v := range in {
		edges := make([]report.LocalCouplingEdge, len(v.WorstOffenders))
		for j, e := range v.WorstOffenders {
			edges[j] = report.LocalCouplingEdge{From: e.From, To: e.To, Strength: e.Strength, Balance: e.Balance, Band: e.Band, File: e.File, Line: e.Line}
		}
		out[i] = report.LocalCouplingModule{Module: v.Module, ScoredEdges: v.ScoredEdges, AbstainedEdges: v.AbstainedEdges, ComplexityEdges: v.ComplexityEdges, ComplexitySharePct: v.ComplexitySharePct, MeanBalance: v.MeanBalance, WorstOffenders: edges}
	}
	return out
}

func projectMetrics(in []result.MetricResult) []report.MetricResult {
	out := make([]report.MetricResult, 0, len(in))
	for _, m := range in {
		out = append(out, report.MetricResult{Name: m.Name, Value: m.Value, Display: m.Display, Band: m.Band, Confidence: m.Confidence, Version: m.Version, Mode: m.Mode, Definition: m.Definition, Direction: report.Direction(m.Direction)})
	}
	return out
}

func projectClassifiedEdges(in *result.ClassifiedEdgeSummary) *report.ClassifiedEdgeSummary {
	if in == nil {
		return nil
	}
	out := &report.ClassifiedEdgeSummary{
		Total: in.Total, Scored: in.Scored, Abstained: in.Abstained, SameModule: in.SameModule, MeanBalance: in.MeanBalance,
		ByStrength: in.ByStrength, ByDistance: in.ByDistance, ByDistanceBasis: in.ByDistanceBasis, ByVolatility: in.ByVolatility,
		BySeverity: in.BySeverity, ByBalanceDriver: in.ByBalanceDriver, ByCriticalDriver: in.ByCriticalDriver, ByModulePair: in.ByModulePair,
		DistributedMonolith: in.DistributedMonolith, External: in.External, DeclaredExternal: in.DeclaredExternal,
		ConnectedModules:     in.ConnectedModules,
		VolatilityProvenance: projectVolatilityProvenance(in.VolatilityProvenance), DistanceCompression: projectDistanceCompression(in.DistanceCompression),
	}
	if in.TailRisk != nil {
		out.TailRisk = &report.CouplingTailRiskSummary{WorstBalance: in.TailRisk.WorstBalance, LowerDecileBalance: in.TailRisk.LowerDecileBalance, HighOrWorseEdges: in.TailRisk.HighOrWorseEdges, HighOrWorseSharePct: in.TailRisk.HighOrWorseSharePct, CriticalEdges: in.TailRisk.CriticalEdges, DistributedMonolithEdges: in.TailRisk.DistributedMonolithEdges}
	}
	return out
}

func projectVolatilityProvenance(in *result.VolatilityProvenance) *report.VolatilityProvenance {
	if in == nil {
		return nil
	}
	return &report.VolatilityProvenance{Declared: in.Declared, Inherited: in.Inherited, Cascade: in.Cascade, Undeclared: in.Undeclared}
}

func projectDistanceCompression(in *result.DistanceCompressionSummary) *report.DistanceCompressionSummary {
	if in == nil {
		return nil
	}
	out := &report.DistanceCompressionSummary{CompressedMiddleRungs: in.CompressedMiddleRungs, ImplementedRungs: in.ImplementedRungs, OmittedRungs: in.OmittedRungs, Rationale: in.Rationale}
	for _, r := range in.OmittedRungReasons {
		out.OmittedRungReasons = append(out.OmittedRungReasons, report.DistanceOmittedRungReason{Rung: r.Rung, Reason: r.Reason})
	}
	out.DeterministicSplits = append(out.DeterministicSplits, in.DeterministicSplits...)
	for _, c := range in.ContainmentBoundaryCounts {
		out.ContainmentBoundaryCounts = append(out.ContainmentBoundaryCounts, report.DistanceCount{Value: c.Value, Count: c.Count})
	}
	for _, c := range in.ContainmentAncestorDepths {
		out.ContainmentAncestorDepths = append(out.ContainmentAncestorDepths, report.DistanceCount{Value: c.Value, Count: c.Count})
	}
	return out
}

func projectFindings(in []finding.Finding) []report.Finding {
	out := make([]report.Finding, 0, len(in))
	for _, f := range in {
		locations := make([]report.Location, 0, len(f.Locations))
		for _, loc := range f.Locations {
			locations = append(locations, report.Location{File: loc.File, Line: loc.Line})
		}
		matchedBy := f.MatchedBy
		if matchedBy == nil {
			matchedBy = map[string]string{}
		}
		out = append(out, report.Finding{
			ID: f.ID, Kind: f.Kind, RuleID: f.RuleID, Status: string(f.Status), Severity: string(f.Severity),
			Confidence: f.Confidence,
			Edge: report.FindingEdge{
				From: report.FindingEndpoint{Module: f.Edge.From.Module, Path: f.Edge.From.Path},
				To:   report.FindingEndpoint{Module: f.Edge.To.Module, Path: f.Edge.To.Path}, Kind: f.Edge.Kind,
			},
			MatchedBy: matchedBy, Locations: locations, Why: f.Why, Constraint: f.Constraint, Alternatives: f.Alternatives,
		})
	}
	return out
}

const (
	reportEvidenceUnknown = "unknown"
	reportEvidenceChanged = "changed"
)

func projectEvidence(r result.Result) report.ReportEvidence {
	in := r.ReportEvidence
	out := report.NewReportEvidence()
	out.Connascence.TotalEdges = in.Relationships.TotalEdges
	out.Connascence.Summary = projectConnascence(r.Connascence)
	out.PairPopulation.TotalEdges = in.Relationships.PairEdges
	var before, after evidence.ReportFileSnapshot
	if in.Context != nil {
		before, after = in.Context.Before, in.After
		sc := in.Context.Scope
		out.Scope = report.EvidenceScope{Abstraction: sc.Abstraction, SourceRoot: sc.SourceRoot, PolicyRef: projectEvidenceRef(sc.PolicyRef, before, after),
			ModuleInventoryHash: in.Relationships.ModuleInventoryHash, ClassificationHash: r.ClassificationHash,
			Exclude: append([]string{}, sc.Exclude...), Tests: append([]string{}, sc.Tests...), Generated: append([]string{}, sc.Generated...), Languages: []report.EvidenceLanguage{}}
		for _, l := range sc.Languages {
			out.Scope.Languages = append(out.Scope.Languages, report.EvidenceLanguage{Name: l.Name, Mode: l.Mode})
		}
		out.Snapshot = projectEvidenceSnapshot(before, after, sc.PolicyRef.SHA256)
	}
	components := map[string]evidence.ReportComponent{}
	for _, c := range in.Relationships.Components {
		components[c.Name] = c
	}
	if in.Context != nil {
		for _, c := range in.Context.Components {
			components[c.Name] = c
		}
	}
	names := make([]string, 0, len(components))
	for name := range components {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		c := components[name]
		out.Components = append(out.Components, report.EvidenceComponent{Name: c.Name, Declared: c.Declared,
			Public:     report.EvidencePathsFact{Raw: c.Public.Raw, Effective: c.Public.Effective, Origin: string(c.Public.Origin), Derivation: reportText(c.Public.Derivation, maxReportTextRunes), Refs: projectEvidenceRefs(c.Public.Refs, before, after, out.Scope.PolicyRef.Path)},
			Subdomain:  projectEvidenceString(c.Subdomain, before, after, out.Scope.PolicyRef.Path),
			Volatility: projectEvidenceString(c.Volatility, before, after, out.Scope.PolicyRef.Path), Unknowns: projectEvidenceUnknowns(c.Unknowns)})
	}
	for _, p := range in.Relationships.Pairs {
		row := report.EvidencePair{PairID: p.PairID, Observations: []report.EvidenceObservation{}, InterfaceInventory: report.InterfaceInventory{Status: reportEvidenceUnknown, Reason: "no integration-interface collector; ordered module pair only"}, Unknowns: projectEvidenceUnknowns(p.Unknowns)}
		for _, o := range p.Observations {
			obs := report.EvidenceObservation{FromNode: o.FromNode, ToNode: o.ToNode, EdgeKind: o.EdgeKind, Language: o.Language, Sites: []report.EvidenceRef{}, Connascence: []report.EvidenceConnascenceItem{}}
			for _, s := range o.Sites {
				ref := evidence.ReportRef{Path: s.Path}
				if s.Line > 0 {
					ref.Line = new(s.Line)
				} else {
					row.Unknowns = append(row.Unknowns, report.UnknownFact{Fact: "site_line", Reason: "retained site has no positive source line", Owner: "extractor"})
				}
				site := projectEvidenceRef(ref, before, after)
				if site.Status != "captured" {
					row.Unknowns = append(row.Unknowns, report.UnknownFact{Fact: "site_bytes", Reason: reportText("site unavailable or changed in captured subset: "+s.Path, maxReportTextRunes), Owner: "evidence/acquisition"})
				}
				obs.Sites = append(obs.Sites, site)
			}
			for _, c := range o.Connascence {
				obs.Connascence = append(obs.Connascence, report.EvidenceConnascenceItem{Kind: c.Kind, Source: c.Source, Detail: reportText(c.Detail, maxReportTextRunes)})
			}
			row.Observations = append(row.Observations, obs)
		}
		out.Pairs = append(out.Pairs, row)
	}
	return out
}

func projectEvidenceSnapshot(before, after evidence.ReportFileSnapshot, consumed *string) report.EvidenceSnapshot {
	out := report.EvidenceSnapshot{HeadCommit: before.HeadCommit, HeadTree: before.HeadTree, BeforeDigest: before.Digest, AfterDigest: after.Digest,
		PolicyBeforeDigest: before.PolicyDigest, PolicyAfterDigest: after.PolicyDigest, Cleanliness: before.Cleanliness, Stability: reportEvidenceUnknown, PolicyBinding: reportEvidenceUnknown,
		Unknowns: append(projectEvidenceUnknowns(before.Unknowns), projectEvidenceUnknowns(after.Unknowns)...)}
	if out.Cleanliness == "" {
		out.Cleanliness = reportEvidenceUnknown
	}
	switch {
	case consumed != nil && ((before.PolicyDigest != nil && *consumed != *before.PolicyDigest) || (after.PolicyDigest != nil && *consumed != *after.PolicyDigest)):
		out.PolicyBinding = "mismatch"
		out.Stability = reportEvidenceChanged
	case consumed != nil && before.PolicyDigest != nil && after.PolicyDigest != nil:
		out.PolicyBinding = "match"
	default:
		out.Unknowns = append(out.Unknowns, report.UnknownFact{Fact: "policy_binding", Reason: "consumed policy or boundary bytes unavailable", Owner: "evidence/acquisition"})
	}
	if evidenceStringChanged(before.HeadCommit, after.HeadCommit) || evidenceStringChanged(before.HeadTree, after.HeadTree) || evidenceStringChanged(before.Digest, after.Digest) {
		out.Stability = reportEvidenceChanged
	}
	if out.Stability != reportEvidenceChanged && before.Digest != nil && after.Digest != nil && *before.Digest == *after.Digest && out.PolicyBinding == "match" {
		out.Stability = "stable_by_pre_post_check"
	}
	return out
}

func evidenceStringChanged(a, b *string) bool { return a != nil && b != nil && *a != *b }

func projectEvidenceRef(in evidence.ReportRef, before, after evidence.ReportFileSnapshot) report.EvidenceRef {
	out := report.EvidenceRef{Path: in.Path, SHA256: in.SHA256, Line: in.Line, Selector: in.Selector, Status: reportEvidenceUnknown}
	a, aok := before.Files[in.Path]
	b, bok := after.Files[in.Path]
	switch {
	case aok && bok:
		if out.SHA256 == nil {
			out.SHA256 = new(a)
		}
		if a == b && *out.SHA256 == a {
			out.Status = "captured"
		} else {
			out.Status = reportEvidenceChanged
		}
	case (aok || bok) && before.Digest != nil && after.Digest != nil:
		out.Status = reportEvidenceChanged
	}
	return out
}

func projectEvidenceRefs(in []evidence.ReportRef, before, after evidence.ReportFileSnapshot, policyPath string) []report.EvidenceRef {
	out := make([]report.EvidenceRef, 0, len(in))
	for _, ref := range in {
		if ref.Path == "" {
			ref.Path = policyPath
		}
		out = append(out, projectEvidenceRef(ref, before, after))
	}
	return out
}

func projectEvidenceString(in evidence.ReportStringFact, before, after evidence.ReportFileSnapshot, policyPath string) report.EvidenceStringFact {
	return report.EvidenceStringFact{Raw: in.Raw, Effective: in.Effective, Origin: string(in.Origin), Derivation: reportText(in.Derivation, maxReportTextRunes), Refs: projectEvidenceRefs(in.Refs, before, after, policyPath)}
}

func projectEvidenceUnknowns(in []evidence.ReportUnknownFact) []report.UnknownFact {
	out := make([]report.UnknownFact, 0, len(in))
	for _, u := range in {
		out = append(out, report.UnknownFact{Fact: reportText(u.Fact, maxReportTextRunes), Reason: reportText(u.Reason, maxReportTextRunes), Owner: u.Owner})
	}
	return out
}
