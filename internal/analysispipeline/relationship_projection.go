package pipeline

import (
	"github.com/alexei-led/archfit/internal/assessment/result"
	"github.com/alexei-led/archfit/internal/relationship"
)

func projectRelationshipSummary(in *relationship.ClassifiedEdgeSummary) *result.ClassifiedEdgeSummary {
	if in == nil {
		return nil
	}
	out := &result.ClassifiedEdgeSummary{Total: in.Total, Scored: in.Scored, Abstained: in.Abstained, SameModule: in.SameModule, MeanBalance: in.MeanBalance, ByStrength: in.ByStrength, ByDistance: in.ByDistance, ByDistanceBasis: in.ByDistanceBasis, ByVolatility: in.ByVolatility, BySeverity: in.BySeverity, ByBalanceDriver: in.ByBalanceDriver, ByCriticalDriver: in.ByCriticalDriver, ByModulePair: in.ByModulePair, DistributedMonolith: in.DistributedMonolith, External: in.External, DeclaredExternal: in.DeclaredExternal, ConnectedModules: in.ConnectedModules, CloneOnlyScored: in.CloneOnlyScored, CloneOnlyAdvisory: in.CloneOnlyAdvisory, LLMApproved: in.LLMApproved, LabeledLLM: in.LabeledLLM, LLMLowConfidenceEdges: in.LLMLowConfidenceEdges}
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
