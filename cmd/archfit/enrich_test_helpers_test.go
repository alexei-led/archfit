package main

import (
	"github.com/alexei-led/archfit/internal/config"
	"github.com/alexei-led/archfit/internal/engine"
	"github.com/alexei-led/archfit/internal/model/graph"
	"github.com/alexei-led/archfit/internal/relationship/coupling"
	"github.com/alexei-led/archfit/internal/relationship/labels"
)

func selectRefinablePairs(g *graph.Graph, idx coupling.Index, mm config.ModuleMap, existing []labels.Label, evidence map[string]string) []refinablePair {
	return engine.SelectRefinablePairs(g, idx, mm, effectiveApprovedPairs(existing, evidence))
}

func enrichModuleMap(cfg config.Config, g *graph.Graph) config.ModuleMap {
	return engine.AugmentClassifyConfig(g, cfg.ForClassify()).ModuleMap
}
