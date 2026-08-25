// Package signal defines the narrow assessment inputs projected from neutral
// evidence facts for each metric family.
package signal

import (
	"github.com/alexei-led/archfit/internal/assessment/finding"
	assessmentresult "github.com/alexei-led/archfit/internal/assessment/result"
	neutral "github.com/alexei-led/archfit/internal/evidence"
	"github.com/alexei-led/archfit/internal/model/evidence"
	"github.com/alexei-led/archfit/internal/model/symbol"
	"github.com/alexei-led/archfit/internal/relationship"
)

// SymbolSignals carries the SCIP symbol graph. Empty when SCIP is off.
type SymbolSignals struct {
	Graph symbol.Graph
}

// SizeSignals aliases neutral source-size evidence for metric compatibility.
type SizeSignals = neutral.SizeSignals

// DuplicationSignals aliases neutral clone evidence for metric compatibility.
type DuplicationSignals = neutral.DuplicationSignals

// RunSignals aliases the neutral evidence bundle for metric compatibility.
type RunSignals = neutral.RunSignals

// DynamicImportSignals aliases neutral dynamic-import evidence.
type DynamicImportSignals = neutral.DynamicImportSignals

// RuntimeAsyncSignals aliases neutral runtime-async evidence.
type RuntimeAsyncSignals = neutral.RuntimeAsyncSignals

// ---------------------------------------------------------------------------
// Per-family metric inputs (the narrow replacement for the former god input).
//
// Each metric consumes only its family's input, enforced by the compiler: the
// engine assembles a CollectedSignals once and projects it to the per-family
// input each metric declares. Adding a signal touches one group, not every
// metric. Zero-value groups (empty maps/slices) preserve the existing
// "report n/a when the signal is absent" behaviour.
// ---------------------------------------------------------------------------

// CoverageView is the coverage fact subset metrics need. It narrows the raw
// evidence.Coverage model to assessment-owned values before metric calculation.
type CoverageView []CoverageRecord

// CoverageRecord is one tool coverage row for assessment metrics.
type CoverageRecord struct {
	Tool            string
	Status          string
	FilesSeen       int
	FilesApplicable int
	Unresolved      int
	SpecifiersSeen  int
}

// NewCoverageView projects raw evidence coverage records into the assessment
// metric contract.
func NewCoverageView(rows []evidence.Coverage) CoverageView {
	out := make(CoverageView, 0, len(rows))
	for _, row := range rows {
		out = append(out, CoverageRecord{
			Tool:            row.Tool,
			Status:          row.Status,
			FilesSeen:       row.FilesSeen,
			FilesApplicable: row.FilesApplicable,
			Unresolved:      row.Unresolved,
			SpecifiersSeen:  row.SpecifiersSeen,
		})
	}
	return out
}

// CommonInput is the narrow assessment metric contract every metric can rely on.
// It carries relationship-owned classified facts, a coverage view, changed files,
// symbol input, statuses, and baselines — not the raw extractor graph.
type CommonInput struct {
	Relationships relationship.Set
	Findings      []finding.Finding
	Baseline      assessmentresult.MetricSnapshot
	Coverage      CoverageView
	ChangedFiles  []string
	Symbols       SymbolSignals
}

// CollectedSignals is the engine's producer-side bag of everything gathered for
// a run. It never reaches metric logic directly — the engine projects it to the
// common input each metric declares via AsCommon below.
type CollectedSignals struct {
	Common      CommonInput
	Symbol      SymbolSignals
	Size        SizeSignals
	Duplication DuplicationSignals
}

// AsCommon projects to the common input.
func (s CollectedSignals) AsCommon() CommonInput { return s.Common }
