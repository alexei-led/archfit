package application

import (
	"maps"

	"github.com/alexei-led/archfit/internal/assessment/result"
	modevidence "github.com/alexei-led/archfit/internal/model/evidence"
	"github.com/alexei-led/archfit/internal/policy"
	"github.com/alexei-led/archfit/internal/relationship"
)

const runtimeDistanceInterpretation = "async runtime bridges reduce lifecycle coupling and therefore increase perceived distance (book Ch8), but remain report-only because archfit does not yet measure synchronous first-party runtime peers deterministically"

const (
	ownerModelNoOwner               = "no_owner_signal"
	ownerModelSingleOwnerDegenerate = "single_owner_degenerate"
	ownerModelMultiOwner            = "multi_owner"
)

func attachRelationshipEvidence(diag *result.Result, in relationship.AnalysisEvidence) {
	if diag == nil {
		return
	}
	diag.DynamicImports = in.DynamicImports
	diag.RuntimeAsync = in.RuntimeModules
	diag.RuntimeAsyncEdges = in.RuntimeEdges
	diag.DynamicConnascenceSignals = in.DynamicConnascenceSignals
	diag.DistanceConfigCandidates = in.DistanceConfigCandidates
	diag.LocalCoupling = in.LocalCoupling
	if in.Connascence != nil {
		diag.Connascence = in.Connascence
	} else {
		diag.Connascence = &modevidence.ConnascenceReport{}
	}
	if diag.ClassifiedEdges != nil {
		diag.ClassifiedEdges.LLMApproved = in.LLMApprovedCount
		if in.VolatilityProvenance != nil {
			vp := in.VolatilityProvenance
			diag.ClassifiedEdges.VolatilityProvenance = &result.VolatilityProvenance{
				Declared: vp.Declared, Inherited: vp.Inherited, Cascade: vp.Cascade, Undeclared: vp.Undeclared,
			}
		}
	}
}

func buildDistanceContext(d result.Result, p policy.PolicySnapshot, deployUnitDetectedModules int) *modevidence.DistanceContext {
	ctx := &modevidence.DistanceContext{
		OwnerModel:                ownerModel(p),
		DeployUnitDetectedModules: deployUnitDetectedModules,
		DeclaredExternalSystems:   len(p.Topology.ExternalSystems),
	}
	if d.ClassifiedEdges != nil && len(d.ClassifiedEdges.ByDistanceBasis) > 0 {
		ctx.DistanceBasis = maps.Clone(d.ClassifiedEdges.ByDistanceBasis)
	}
	if len(d.RuntimeAsyncEdges) > 0 {
		ctx.RuntimeAsyncRelations = len(d.RuntimeAsyncEdges)
		ctx.RuntimeAsyncKinds = countRuntimeAsyncKinds(d.RuntimeAsyncEdges)
		ctx.RuntimeInterpretation = runtimeDistanceInterpretation
	}
	ctx.Interpretation = distanceInterpretation(ctx.OwnerModel, deployUnitDetectedModules, len(p.Topology.ExternalSystems))
	return ctx
}

func ownerModel(p policy.PolicySnapshot) string {
	owners := make(map[string]struct{})
	for _, def := range p.Topology.Modules {
		if def.Owner == "" {
			continue
		}
		owners[def.Owner] = struct{}{}
	}
	switch len(owners) {
	case 0:
		return ownerModelNoOwner
	case 1:
		return ownerModelSingleOwnerDegenerate
	default:
		return ownerModelMultiOwner
	}
}

func countRuntimeAsyncKinds(edges []modevidence.RuntimeAsyncEdge) map[string]int {
	kinds := make(map[string]int)
	for _, edge := range edges {
		if edge.IntegrationKind == "" {
			continue
		}
		kinds[edge.IntegrationKind]++
	}
	if len(kinds) == 0 {
		return nil
	}
	return kinds
}

func distanceInterpretation(model string, deployUnitDetectedModules, declaredExternalSystems int) string {
	suffix := ""
	if deployUnitDetectedModules > 0 || declaredExternalSystems > 0 {
		suffix = "; deploy_unit and declared external_systems evidence can still raise distance when configured/detected"
	}
	switch model {
	case ownerModelSingleOwnerDegenerate:
		return "same-owner is the lowest cross-module distance; this is a low socio-technical distance signal, not missing ownership" + suffix
	case ownerModelMultiOwner:
		return "ownership has multiple distinct owners, so owner distance can distinguish same-owner and different-owner module edges" + suffix
	default:
		return "ownership is absent or unresolved, so distance uses code structure plus deterministic deploy_unit and declared external_systems evidence" + suffix
	}
}
