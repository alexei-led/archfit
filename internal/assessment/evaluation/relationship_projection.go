package evaluation

import (
	"github.com/alexei-led/archfit/internal/assessment/result"
	"github.com/alexei-led/archfit/internal/relationship"
)

func projectRelationshipSummary(in *relationship.ClassifiedEdgeSummary) *result.ClassifiedEdgeSummary {
	if in == nil {
		return nil
	}
	out := &result.ClassifiedEdgeSummary{
		Total: in.Total, Scored: in.Scored, Abstained: in.Abstained, SameModule: in.SameModule,
		DependencyEdges: in.DependencyEdges, InternalDependencies: in.InternalDependencies,
		ClassifiedInternalDependencies: in.ClassifiedInternalDependencies,
		SameModuleDependencies:         in.SameModuleDependencies, DependencyModules: in.DependencyModules,
		FirstPartyNodes: in.FirstPartyNodes, AttributedFirstPartyNodes: in.AttributedFirstPartyNodes,
		MeanBalance: in.MeanBalance, ByStrength: in.ByStrength, ByDistance: in.ByDistance,
		ByDistanceBasis: in.ByDistanceBasis, ByVolatility: in.ByVolatility, BySeverity: in.BySeverity,
		ByBalanceDriver: in.ByBalanceDriver, ByCriticalDriver: in.ByCriticalDriver, ByModulePair: in.ByModulePair,
		DistributedMonolith: in.DistributedMonolith, External: in.External, DeclaredExternal: in.DeclaredExternal,
		ConnectedModules: in.ConnectedModules, CloneOnlyScored: in.CloneOnlyScored, CloneOnlyAdvisory: in.CloneOnlyAdvisory,
		LLMApproved: in.LLMApproved, LabeledLLM: in.LabeledLLM, LLMLowConfidenceEdges: in.LLMLowConfidenceEdges,
	}
	if in.VolatilityProvenance != nil {
		out.VolatilityProvenance = &result.VolatilityProvenance{Declared: in.VolatilityProvenance.Declared, Inherited: in.VolatilityProvenance.Inherited, Cascade: in.VolatilityProvenance.Cascade, Undeclared: in.VolatilityProvenance.Undeclared}
	}
	if in.DistanceCompression != nil {
		d := in.DistanceCompression
		out.DistanceCompression = &result.DistanceCompressionSummary{CompressedMiddleRungs: d.CompressedMiddleRungs, ImplementedRungs: d.ImplementedRungs, OmittedRungs: d.OmittedRungs, DeterministicSplits: d.DeterministicSplits, Rationale: d.Rationale}
		for _, r := range d.OmittedRungReasons {
			out.DistanceCompression.OmittedRungReasons = append(out.DistanceCompression.OmittedRungReasons, result.DistanceOmittedRungReason{Rung: r.Rung, Reason: r.Reason})
		}
		for _, c := range d.CodeStructureBoundaryCounts {
			out.DistanceCompression.CodeStructureBoundaryCounts = append(out.DistanceCompression.CodeStructureBoundaryCounts, result.DistanceCount{Value: c.Value, Count: c.Count})
		}
		for _, c := range d.CodeStructureAncestorDepths {
			out.DistanceCompression.CodeStructureAncestorDepths = append(out.DistanceCompression.CodeStructureAncestorDepths, result.DistanceCount{Value: c.Value, Count: c.Count})
		}
	}
	if in.TailRisk != nil {
		t := in.TailRisk
		out.TailRisk = &result.CouplingTailRiskSummary{WorstBalance: t.WorstBalance, LowerDecileBalance: t.LowerDecileBalance, HighOrWorseEdges: t.HighOrWorseEdges, HighOrWorseSharePct: t.HighOrWorseSharePct, CriticalEdges: t.CriticalEdges, DistributedMonolithEdges: t.DistributedMonolithEdges, CloneOnlyScored: t.CloneOnlyScored, CloneOnlyHighOrWorseEdges: t.CloneOnlyHighOrWorseEdges, CloneOnlyWorstBalance: t.CloneOnlyWorstBalance}
	}
	return out
}

// projectSeams copies the relationship seam ledger into the assessment result.
// It is a pure rename of typed values: assessment never recomputes a seam, and
// nothing here reads a threshold.
func projectSeams(in []relationship.Seam) []result.Seam {
	if len(in) == 0 {
		return nil
	}
	out := make([]result.Seam, 0, len(in))
	for _, s := range in {
		out = append(out, result.Seam{
			ID: s.ID, FromModule: s.FromModule, ToModule: s.ToModule,
			Edges: s.Edges, ScoredEdges: s.ScoredEdges, AbstainedEdges: s.AbstainedEdges,
			Strength: string(s.Strength), Distance: string(s.Distance), Volatility: string(s.Volatility),
			VolatilityProvenance: s.VolatilityProvenance, Severity: string(s.Severity),
			RawDistance: result.SeamDistance{
				Level: string(s.RawDistance.Level), Basis: s.RawDistance.Basis,
				FromOwner: s.RawDistance.FromOwner, ToOwner: s.RawDistance.ToOwner, SameOwner: s.RawDistance.SameOwner,
				FromDeployUnit: s.RawDistance.FromDeployUnit, ToDeployUnit: s.RawDistance.ToDeployUnit,
				SameDeployUnit:    s.RawDistance.SameDeployUnit,
				BoundaryCrossings: s.RawDistance.BoundaryCrossings, SharedAncestor: s.RawDistance.SharedAncestor,
			},
			Quadrant: string(s.Quadrant),
			Scores: result.SeamScoreDistribution{
				N: s.Scores.N, Min: s.Scores.Min, Median: s.Scores.Median, Max: s.Scores.Max,
				P10: s.Scores.P10, P90: s.Scores.P90, Mean: s.Scores.Mean,
			},
			CriticalEdges: s.CriticalEdges, HighOrWorseEdges: s.HighOrWorseEdges,
			CriticalSharePct: s.CriticalSharePct, HighOrWorseSharePct: s.HighOrWorseSharePct,
			Labels: s.Labels, LabelEvidenceHash: s.LabelEvidenceHash, Confidence: string(s.Confidence),
			RoleExpectation: string(s.RoleExpectation), Hypothesis: string(s.Hypothesis),
			DistributedMonolith: s.DistributedMonolith,
		})
	}
	return out
}
