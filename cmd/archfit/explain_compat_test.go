package main

import (
	"github.com/alexei-led/archfit/internal/assessment/finding"
	"github.com/alexei-led/archfit/internal/assessment/result"
	"github.com/alexei-led/archfit/internal/model/report"
)

// Compatibility helper for focused prompt tests; production explain uses the
// application report DTO and never imports assessment packages.
func buildExplainPrompt(f finding.Finding, diag result.Result) string {
	rf := report.Finding{ID: f.ID, RuleID: f.RuleID, Status: string(f.Status), Severity: string(f.Severity), Confidence: f.Confidence, MatchedBy: f.MatchedBy, Why: f.Why, Constraint: f.Constraint, Alternatives: f.Alternatives,
		Edge: report.FindingEdge{From: report.FindingEndpoint{Path: f.Edge.From.Path, Module: f.Edge.From.Module}, To: report.FindingEndpoint{Path: f.Edge.To.Path, Module: f.Edge.To.Module}, Kind: f.Edge.Kind}}
	for _, loc := range f.Locations {
		rf.Locations = append(rf.Locations, report.Location{File: loc.File, Line: loc.Line})
	}
	rd := report.Document{}
	for _, ff := range diag.FileFacts {
		rd.FileFacts = append(rd.FileFacts, report.FileFact{Module: ff.Module, InboundModuleFanIn: ff.InboundModuleFanIn, OutboundDestinations: ff.OutboundDestinations, LOC: ff.LOC})
	}
	return buildExplainPromptReport(rf, rd)
}
