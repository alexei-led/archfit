package pipeline

import (
	"github.com/alexei-led/archfit/internal/application"
	assessmentresult "github.com/alexei-led/archfit/internal/assessment/result"
	signal "github.com/alexei-led/archfit/internal/assessment/signals"
)

type relationshipCapture struct{ in *signal.CommonInput }

func (m *relationshipCapture) Name() string    { return "application_enrichment_capture" }
func (m *relationshipCapture) Version() string { return "application_enrichment_capture.v1" }
func (m *relationshipCapture) Calculate(in signal.CollectedSignals) assessmentresult.MetricResult {
	*m.in = in.Common
	return assessmentresult.MetricResult{Name: m.Name(), Version: m.Version(), Band: "info", Display: "internal capture"}
}

func projectEnrichmentEvidence(in signal.CommonInput) *application.EnrichmentEvidence {
	out := &application.EnrichmentEvidence{Edges: make([]application.EnrichmentEdge, 0, len(in.Relationships.Edges))}
	for _, e := range in.Relationships.Edges {
		p := application.EnrichmentEdge{FromPath: e.FromPath, ToPath: e.ToPath, FromModule: e.FromModule, ToModule: e.ToModule, Kind: e.Kind, Strength: string(e.Strength), Distance: string(e.Distance)}
		for _, l := range e.Locations {
			p.Locations = append(p.Locations, application.Location{File: l.File, Line: l.Line})
		}
		out.Edges = append(out.Edges, p)
	}
	return out
}
