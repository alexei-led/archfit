// Package markdown implements the Renderer port for Markdown output.
// It uses text/template for the report skeleton and go-pretty/v6/table
// for tabular sections. When stdout is a terminal, tables are rendered with
// color; otherwise GFM markdown tables are used.
package markdown

import (
	_ "embed"
	"io"
	"os"
	"sort"
	"strings"
	"text/template"

	"github.com/jedib0t/go-pretty/v6/table"
	"github.com/jedib0t/go-pretty/v6/text"
	"golang.org/x/term"

	"github.com/alexei-led/archfit/internal/model/diagnostic"
	"github.com/alexei-led/archfit/internal/model/finding"
)

//go:embed report.tmpl
var reportTmpl string

// severityRank maps severity strings to an integer rank (higher = more severe).
var severityRank = map[finding.Severity]int{
	finding.SeverityCritical: 4,
	finding.SeverityHigh:     3,
	finding.SeverityMedium:   2,
	finding.SeverityLow:      1,
}

// Renderer formats a Diagnostic as Markdown. Satisfies engine.Renderer.
type Renderer struct {
	isTTY bool
}

// New returns a Renderer. Terminal detection uses os.Stdout.
func New() *Renderer {
	return &Renderer{
		isTTY: term.IsTerminal(int(os.Stdout.Fd())),
	}
}

// Format returns "markdown".
func (r *Renderer) Format() string { return "markdown" }

// Render writes the Markdown report for d to w.
func (r *Renderer) Render(d diagnostic.Diagnostic, w io.Writer) error {
	tmpl, err := template.New("report").Parse(reportTmpl)
	if err != nil {
		return err
	}

	data := r.buildTemplateData(d)
	return tmpl.Execute(w, data)
}

// templateData holds all pre-rendered strings passed into the template.
type templateData struct {
	Verdict string
	Summary diagnostic.Summary
	Metrics []diagnostic.MetricResult

	// Pre-rendered table strings (empty string = section omitted).
	GateTable        string
	AdvisoryTable    string
	MetricsTable     string
	StalenessTable   string
	ExceptionTable   string
	AllFindingsTable string

	// Slices used for conditional checks in template.
	AllFindings []finding.Finding
}

func (r *Renderer) buildTemplateData(d diagnostic.Diagnostic) templateData {
	// Partition findings.
	gate := make([]finding.Finding, 0, len(d.Findings))
	advisory := make([]finding.Finding, 0, len(d.Findings))
	staleness := make([]finding.Finding, 0, len(d.Findings))
	exceptions := make([]finding.Finding, 0, len(d.Findings))
	all := make([]finding.Finding, 0, len(d.Findings))
	for _, f := range d.Findings {
		all = append(all, f)
		switch {
		case f.Kind == "advisory" && strings.HasPrefix(f.RuleID, "map/"):
			staleness = append(staleness, f)
		case f.Kind == "advisory":
			advisory = append(advisory, f)
		case f.Status == finding.StatusExcepted || f.Status == finding.StatusExpiredExcept:
			exceptions = append(exceptions, f)
		default:
			gate = append(gate, f)
		}
	}

	// Sort gate findings by severity (desc), stable by ID.
	sort.SliceStable(gate, func(i, j int) bool {
		ri, rj := severityRank[gate[i].Severity], severityRank[gate[j].Severity]
		if ri != rj {
			return ri > rj
		}
		return gate[i].ID < gate[j].ID
	})
	top10 := gate
	if len(top10) > 10 {
		top10 = top10[:10]
	}

	td := templateData{
		Verdict:     string(d.Verdict),
		Summary:     d.Summary,
		Metrics:     d.Metrics,
		AllFindings: all,
	}

	if len(top10) > 0 {
		td.GateTable = r.renderFindingsTable(top10)
	}
	if len(advisory) > 0 {
		td.AdvisoryTable = r.renderFindingsTable(advisory)
	}
	if len(d.Metrics) > 0 {
		td.MetricsTable = r.renderMetricsTable(d.Metrics)
	}
	if len(staleness) > 0 {
		td.StalenessTable = r.renderFindingsTable(staleness)
	}
	if len(exceptions) > 0 {
		td.ExceptionTable = r.renderFindingsTable(exceptions)
	}
	if len(all) > 0 {
		td.AllFindingsTable = r.renderFindingsTable(all)
	}

	return td
}

// renderFindingsTable builds a table for a slice of findings.
func (r *Renderer) renderFindingsTable(findings []finding.Finding) string {
	t := newTable(r.isTTY)
	t.AppendHeader(table.Row{"ID", "Rule", "Severity", "From", "To", "Status"})
	for _, f := range findings {
		t.AppendRow(table.Row{
			f.ID[:8], // short ID (first 8 chars)
			f.RuleID,
			string(f.Severity),
			f.Edge.From.Path,
			f.Edge.To.Path,
			string(f.Status),
		})
	}
	return renderTable(t, r.isTTY)
}

// renderMetricsTable builds a table for metric results.
func (r *Renderer) renderMetricsTable(metrics []diagnostic.MetricResult) string {
	t := newTable(r.isTTY)
	t.AppendHeader(table.Row{"Metric", "Value", "Band", "Confidence"})
	for _, m := range metrics {
		t.AppendRow(table.Row{m.Name, m.Display, m.Band, m.Confidence})
	}
	return renderTable(t, r.isTTY)
}

// newTable creates a table.Writer with consistent style.
func newTable(isTTY bool) table.Writer {
	t := table.NewWriter()
	if isTTY {
		t.SetStyle(table.StyleColoredBright)
	} else {
		t.SetStyle(table.StyleDefault)
	}
	t.Style().Options.SeparateRows = false
	t.Style().Options.DrawBorder = !isTTY // borders in terminal; GFM doesn't want them
	// Suppress color codes in non-TTY mode.
	if !isTTY {
		t.SetStyle(table.Style{
			Name: "markdown",
			Box:  table.StyleBoxDefault,
			Color: table.ColorOptions{
				Header: text.Colors{},
				Row:    text.Colors{},
			},
			Format: table.FormatOptions{
				Header: text.FormatDefault,
				Row:    text.FormatDefault,
			},
			Options: table.Options{
				DrawBorder:      false,
				SeparateColumns: true,
				SeparateRows:    false,
			},
		})
	}
	return t
}

// renderTable produces the final string: colored for TTY, GFM for non-TTY.
func renderTable(t table.Writer, isTTY bool) string {
	if isTTY {
		return t.Render()
	}
	return t.RenderMarkdown()
}
