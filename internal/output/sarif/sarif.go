// Package sarif renders a report document as a SARIF 2.1.0 log (spec §12) so CI
// systems (GitHub code scanning et al.) can surface findings inline on PRs —
// the agent/CI half of the feedback loop.
//
// Mapping:
//   - one run; one driver rule per distinct finding RuleID;
//   - one result per finding — level: error (active gate), warning (advisory),
//     note (baselined/waived/fixed);
//   - the architecture state rides in run.properties (SARIF has no dimension or
//     verdict concept); finding identity — ruleId, ruleIndex, and the
//     archfit/v1 fingerprint — is unchanged by the state cutover, so an existing
//     code-scanning consumer keeps resolving the same alerts;
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

	dimensionOf := findingDimensions(d.State)
	seamOf := seamIDsByModulePair(d.State.Seams)
	results := make([]result, 0, len(d.Findings))
	for _, f := range d.Findings {
		results = append(results, toResult(f, ruleIndex[f.RuleID], dimensionOf[f.ID], seamOf))
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
			Properties:        runProperties(d),
		}},
	}

	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	return enc.Encode(doc)
}

// runProperties carries the architecture state alongside the diagnostic facts
// SARIF has no native slot for. SARIF is exempt from human-layout parity, not
// from fact parity: a consumer reading only the SARIF log still sees the same
// verdict, the same nine dimensions, the same coverage split, and the same
// comparability answer the other formats report.
func runProperties(d report.Document) map[string]any {
	s := d.State
	dims := make([]map[string]any, 0, report.DimensionCount)
	for _, dim := range s.Dimensions.All() {
		dims = append(dims, map[string]any{
			"name": dim.Name, "owner": dim.Owner, "status": dim.Status,
			"confidence": dim.Confidence, "gate": dim.Gate,
			"coverage": map[string]any{"basis": dim.Coverage.Basis, "observed": dim.Coverage.Observed, "total": dim.Coverage.Total},
			"unknown":  len(dim.Unknown),
		})
	}
	return map[string]any{
		"schema_version": s.SchemaVersion,
		"verdict":        s.Verdict,
		"decision":       s.Decision,
		"dimensions":     dims,
		"coverage":       s.Coverage,
		"comparison":     s.Comparison,
		"measurement":    s.Measurement,
		"seams":          s.Seams,
		"metrics":        d.Metrics,
		"summary":        d.Summary,
	}
}

// findingDimensions maps each active finding to the dimension that owns its
// subject, so a SARIF consumer can group alerts the way the report does. A
// finding no dimension references (baselined, waived, fixed) is simply absent.
func findingDimensions(s report.ArchitectureState) map[string]string {
	out := map[string]string{}
	for _, dim := range s.Dimensions.All() {
		for _, ref := range dim.Findings {
			out[ref.ID] = dim.Name
		}
	}
	return out
}

// seamIDsByModulePair indexes the ledger by ordered module pair so a finding on
// a cross-boundary edge can carry its stable seam ID.
func seamIDsByModulePair(seams []report.Seam) map[string]string {
	out := make(map[string]string, len(seams))
	for _, s := range seams {
		out[s.FromModule+"\x00"+s.ToModule] = s.ID
	}
	return out
}

// toResult maps one finding to a SARIF result. RuleID, ruleIndex, and the
// fingerprint are untouched by the state cutover: the architecture state adds
// grouping metadata, it never re-identifies an alert.
func toResult(f report.Finding, ruleIdx int, dimension string, seamOf map[string]string) result {
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
		Properties:   resultProperties(f, dimension, seamOf),
	}
}

// resultProperties carries the finding's lifecycle plus the state grouping.
// `gate` separates a blocker from a diagnostic explicitly rather than making a
// consumer re-derive it from kind and status.
func resultProperties(f report.Finding, dimension string, seamOf map[string]string) map[string]any {
	props := map[string]any{
		"status":   f.Status,
		"severity": f.Severity,
		"kind":     f.Kind,
		"gate":     f.Kind == report.FindingKindGate,
	}
	if dimension != "" {
		props["dimension"] = dimension
	}
	if f.Edge.From.Module != "" && f.Edge.To.Module != "" {
		props["module_pair"] = f.Edge.From.Module + " -> " + f.Edge.To.Module
		if id, ok := seamOf[f.Edge.From.Module+"\x00"+f.Edge.To.Module]; ok {
			props["seam_id"] = id
		}
	}
	return props
}

// levelFor maps finding kind+status to a SARIF level: active gate findings are
// errors, active advisories are warnings, everything resolved/accepted is a note.
//
// Both kinds read activity through one predicate. An expired waiver is active on
// either kind — the status assignment is kind-blind, and the run counts an
// expired advisory as a live diagnostic that sets its dimension to warn — so
// grading it a note here would contradict the state block in the same document.
func levelFor(f report.Finding) string {
	if !findingActive(f) {
		return "note"
	}
	if f.Kind == report.FindingKindGate {
		return "error"
	}
	return "warning"
}

// findingActive reports whether a finding still counts against this run: newly
// observed, or accepted under a waiver that has since expired.
func findingActive(f report.Finding) bool {
	return f.Status == report.FindingStatusNew || f.Status == report.FindingStatusExpiredWaiver
}
