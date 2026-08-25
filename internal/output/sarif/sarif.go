// Package sarif renders a report document as a SARIF 2.1.0 log (spec §12) so CI
// systems (GitHub code scanning et al.) can surface findings inline on PRs —
// the agent/CI half of the feedback loop.
//
// Mapping:
//   - one run; one driver rule per distinct finding RuleID;
//   - one result per finding — level: error (active gate), warning (advisory),
//     note (baselined/waived/fixed);
//   - metric results and the verdict ride in run.properties (SARIF has no
//     metric concept);
//   - byte-deterministic: stable ordering, no timestamps; base/head refs go in
//     automationDetails.id.
package sarif

import (
	"encoding/json"
	"io"
	"sort"

	"github.com/alexei-led/archfit/internal/model/report"
	reportports "github.com/alexei-led/archfit/internal/report/ports"
)

// Renderer formats a report document as SARIF 2.1.0.
type Renderer struct{}

var _ reportports.Renderer = (*Renderer)(nil)

// New returns a Renderer.
func New() *Renderer { return &Renderer{} }

// Format returns "sarif".
func (r *Renderer) Format() string { return "sarif" }

const (
	schemaURI    = "https://json.schemastore.org/sarif-2.1.0.json"
	sarifVersion = "2.1.0"
	driverInfo   = "https://github.com/alexei-led/archfit"
)

// log is the top-level SARIF document.
type log struct {
	Schema  string `json:"$schema"`
	Version string `json:"version"`
	Runs    []run  `json:"runs"`
}

type run struct {
	Tool              tool           `json:"tool"`
	AutomationDetails automationID   `json:"automationDetails"`
	Results           []result       `json:"results"`
	Properties        map[string]any `json:"properties,omitempty"`
}

type tool struct {
	Driver driver `json:"driver"`
}

type driver struct {
	Name           string `json:"name"`
	InformationURI string `json:"informationUri"`
	Rules          []rule `json:"rules"`
}

type rule struct {
	ID               string  `json:"id"`
	ShortDescription message `json:"shortDescription"`
}

type automationID struct {
	ID string `json:"id"`
}

type result struct {
	RuleID       string            `json:"ruleId"`
	RuleIndex    int               `json:"ruleIndex"`
	Level        string            `json:"level"`
	Message      message           `json:"message"`
	Locations    []location        `json:"locations,omitempty"`
	Fingerprints map[string]string `json:"fingerprints"`
	Properties   map[string]any    `json:"properties,omitempty"`
}

type message struct {
	Text string `json:"text"`
}

type location struct {
	PhysicalLocation physicalLocation `json:"physicalLocation"`
}

type physicalLocation struct {
	ArtifactLocation artifactLocation `json:"artifactLocation"`
	Region           *region          `json:"region,omitempty"`
}

type artifactLocation struct {
	URI string `json:"uri"`
}

type region struct {
	StartLine int `json:"startLine"`
}

// Render writes d to w as a SARIF 2.1.0 log.
func (r *Renderer) Render(d report.Document, w io.Writer) error {
	// Distinct rule IDs, sorted, with index lookup.
	ruleIDs := map[string]struct{}{}
	for _, f := range d.Findings {
		ruleIDs[f.RuleID] = struct{}{}
	}
	sorted := make([]string, 0, len(ruleIDs))
	for id := range ruleIDs {
		sorted = append(sorted, id)
	}
	sort.Strings(sorted)
	ruleIndex := make(map[string]int, len(sorted))
	rules := make([]rule, len(sorted))
	for i, id := range sorted {
		ruleIndex[id] = i
		rules[i] = rule{ID: id, ShortDescription: message{Text: id}}
	}

	results := make([]result, 0, len(d.Findings))
	for _, f := range d.Findings {
		results = append(results, toResult(f, ruleIndex[f.RuleID]))
	}

	doc := log{
		Schema:  schemaURI,
		Version: sarifVersion,
		Runs: []run{{
			Tool: tool{Driver: driver{
				Name:           "archfit",
				InformationURI: driverInfo,
				Rules:          rules,
			}},
			AutomationDetails: automationID{ID: "archfit/check/base=" + d.Base + "/head=" + d.Head},
			Results:           results,
			Properties: map[string]any{
				"verdict": d.Verdict,
				"metrics": d.Metrics,
				"summary": d.Summary,
			},
		}},
	}

	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	return enc.Encode(doc)
}

// toResult maps one finding to a SARIF result.
func toResult(f report.Finding, ruleIdx int) result {
	text := f.Why
	if text == "" {
		text = f.RuleID + " violation: " + f.Edge.From.Path + " -> " + f.Edge.To.Path
	}

	var locs []location
	for _, l := range f.Locations {
		if l.File == "" {
			continue
		}
		pl := physicalLocation{ArtifactLocation: artifactLocation{URI: l.File}}
		if l.Line > 0 {
			pl.Region = &region{StartLine: l.Line}
		}
		locs = append(locs, location{PhysicalLocation: pl})
	}
	if len(locs) == 0 && f.Edge.From.Path != "" {
		locs = append(locs, location{PhysicalLocation: physicalLocation{
			ArtifactLocation: artifactLocation{URI: f.Edge.From.Path},
		}})
	}

	return result{
		RuleID:       f.RuleID,
		RuleIndex:    ruleIdx,
		Level:        levelFor(f),
		Message:      message{Text: text},
		Locations:    locs,
		Fingerprints: map[string]string{"archfit/v1": f.ID},
		Properties: map[string]any{
			"status":   f.Status,
			"severity": f.Severity,
			"kind":     f.Kind,
		},
	}
}

// levelFor maps finding kind+status to a SARIF level: active gate findings are
// errors, advisories are warnings, everything resolved/accepted is a note.
func levelFor(f report.Finding) string {
	if f.Kind == report.FindingKindGate && (f.Status == report.FindingStatusNew || f.Status == report.FindingStatusExpiredWaiver) {
		return "error"
	}
	if f.Kind == report.FindingKindAdvisory && f.Status == report.FindingStatusNew {
		return "warning"
	}
	return "note"
}
