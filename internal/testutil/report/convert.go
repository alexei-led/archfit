// Package reporttest provides test-only conversion helpers between assessment and report findings.
package reporttest

import (
	"github.com/alexei-led/archfit/internal/assessment/finding"
	"github.com/alexei-led/archfit/internal/model/report"
	"github.com/alexei-led/archfit/internal/relationship"
)

// Findings converts assessment findings into the stable report view for renderer tests.
func Findings(in ...finding.Finding) []report.Finding {
	out := make([]report.Finding, 0, len(in))
	for _, f := range in {
		locations := make([]report.Location, 0, len(f.Locations))
		for _, loc := range f.Locations {
			locations = append(locations, report.Location{File: loc.File, Line: loc.Line})
		}
		out = append(out, report.Finding{
			ID: f.ID, Kind: f.Kind, RuleID: f.RuleID, Status: string(f.Status), Severity: string(f.Severity), Confidence: f.Confidence,
			Edge: report.FindingEdge{
				From: report.FindingEndpoint{Module: f.Edge.From.Module, Path: f.Edge.From.Path},
				To:   report.FindingEndpoint{Module: f.Edge.To.Module, Path: f.Edge.To.Path}, Kind: f.Edge.Kind,
			},
			MatchedBy: f.MatchedBy, Locations: locations, Why: f.Why, Constraint: f.Constraint, Alternatives: f.Alternatives,
		})
	}
	return out
}

// Finding converts one report finding back to the assessment view for scorecard tests.
func Finding(in report.Finding) finding.Finding {
	locations := make([]relationship.Location, 0, len(in.Locations))
	for _, loc := range in.Locations {
		locations = append(locations, relationship.Location{File: loc.File, Line: loc.Line})
	}
	return finding.Finding{
		ID: in.ID, Kind: in.Kind, RuleID: in.RuleID, Status: finding.Status(in.Status), Severity: finding.Severity(in.Severity), Confidence: in.Confidence,
		Edge: finding.EdgeEvidence{
			From: finding.Endpoint{Module: in.Edge.From.Module, Path: in.Edge.From.Path},
			To:   finding.Endpoint{Module: in.Edge.To.Module, Path: in.Edge.To.Path}, Kind: in.Edge.Kind,
		},
		MatchedBy: in.MatchedBy, Locations: locations, Why: in.Why, Constraint: in.Constraint, Alternatives: in.Alternatives,
	}
}
