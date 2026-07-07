package main

import (
	"maps"

	"github.com/alexei-led/archfit/internal/config"
	"github.com/alexei-led/archfit/internal/model/diagnostic"
)

const (
	ownerModelNoOwner               = "no_owner_signal"
	ownerModelSingleOwnerDegenerate = "single_owner_degenerate"
	ownerModelMultiOwner            = "multi_owner"
)

func buildDistanceContext(d diagnostic.Diagnostic, cfg config.Config, deployUnitDetectedModules int) *diagnostic.DistanceContext {
	ctx := &diagnostic.DistanceContext{
		OwnerModel:                ownerModel(cfg),
		DeployUnitDetectedModules: deployUnitDetectedModules,
		DeclaredExternalSystems:   len(cfg.ExternalSystems),
	}
	if d.ClassifiedEdges != nil && len(d.ClassifiedEdges.ByDistanceBasis) > 0 {
		ctx.DistanceBasis = maps.Clone(d.ClassifiedEdges.ByDistanceBasis)
	}
	ctx.Interpretation = distanceInterpretation(ctx.OwnerModel, deployUnitDetectedModules, len(cfg.ExternalSystems))
	return ctx
}

func ownerModel(cfg config.Config) string {
	owners := make(map[string]struct{})
	for _, def := range cfg.Modules {
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
