package main

import (
	apppipeline "github.com/alexei-led/archfit/internal/analysispipeline"
	"github.com/alexei-led/archfit/internal/config"
	"github.com/alexei-led/archfit/internal/labels/labelsio"
	"github.com/alexei-led/archfit/internal/model/graph"
	"github.com/alexei-led/archfit/internal/relationship/coupling"
	"github.com/alexei-led/archfit/internal/relationship/labels"
)

func selectRefinablePairs(g *graph.Graph, idx coupling.Index, mm config.ModuleMap, existing []labels.Label, evidence map[string]string) []refinablePair {
	selected := apppipeline.SelectRefinablePairs(g, idx, mm, effectiveApprovedPairs(existing, evidence))
	out := make([]refinablePair, len(selected))
	for i, p := range selected {
		out[i] = refinablePair{From: p.From, To: p.To, Strength: p.Strength, EdgeCount: p.EdgeCount, SamplePaths: append([]string(nil), p.SamplePaths...)}
	}
	return out
}

func selectAbstainedPairs(g *graph.Graph, idx coupling.Index, mm config.ModuleMap, existing []labels.Label, evidence map[string]string) (pairs []abstainedPair, total int) {
	selected, total := apppipeline.SelectAbstainedPairs(g, idx, mm, effectiveApprovedPairs(existing, evidence), abstainedEdgeCap, abstainedSampleLocs)
	for _, p := range selected {
		pair := abstainedPair{From: p.From, To: p.To, EdgeCount: p.EdgeCount}
		for _, s := range p.Samples {
			pair.Samples = append(pair.Samples, abstainedSample{FromPath: s.FromPath, ToPath: s.ToPath, File: s.File, Line: s.Line})
		}
		pairs = append(pairs, pair)
	}
	return pairs, total
}

func enrichModuleMap(cfg config.Config, g *graph.Graph) config.ModuleMap {
	return apppipeline.AugmentClassifyConfig(g, cfg.ForClassify()).ModuleMap
}

func effectiveApprovedPairs(existing []labels.Label, evidence map[string]string) map[string]struct{} {
	return apppipeline.EffectiveApprovedEnrichmentPairs(apppipeline.RelationshipLabelsToApplication(existing), evidence)
}

func mergeDrafts(existing, drafts []labels.Label, evidence map[string]string) []labels.Label {
	merged := apppipeline.MergeEnrichmentDrafts(
		apppipeline.RelationshipLabelsToApplication(existing),
		apppipeline.RelationshipLabelsToApplication(drafts), evidence,
	)
	return apppipeline.ApplicationLabelsToRelationship(merged)
}

func writeLabels(path string, in []labels.Label) error { return labelsio.Write(path, in) }
