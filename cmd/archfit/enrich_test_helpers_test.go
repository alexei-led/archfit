package main

import (
	apppipeline "github.com/alexei-led/archfit/internal/analysispipeline"
	"github.com/alexei-led/archfit/internal/labels/labelsio"
	"github.com/alexei-led/archfit/internal/relationship/labels"
)

func mergeDrafts(existing, drafts []labels.Label, evidence map[string]string) []labels.Label {
	merged := apppipeline.MergeEnrichmentDrafts(
		apppipeline.RelationshipLabelsToApplication(existing),
		apppipeline.RelationshipLabelsToApplication(drafts), evidence,
	)
	return apppipeline.ApplicationLabelsToRelationship(merged)
}

func writeLabels(path string, in []labels.Label) error { return labelsio.Write(path, in) }
