// Package signal defines the carrier types that flow between signal producers
// and the metrics layer: the per-family metric input (CommonInput), the
// CollectedSignals bag the engine assembles and projects per family, and the
// RunSignals bundle the cmd layer produces.
//
// Keeping these types here — rather than in internal/metrics — means adding a
// new signal only churns the new producer package and this package. The package
// joins the model ring: stdlib + internal/model/* imports only.
package signal

import (
	"github.com/alexei-led/archfit/internal/model/clone"
	"github.com/alexei-led/archfit/internal/model/coupling"
	"github.com/alexei-led/archfit/internal/model/diagnostic"
	"github.com/alexei-led/archfit/internal/model/fileclass"
	"github.com/alexei-led/archfit/internal/model/finding"
	"github.com/alexei-led/archfit/internal/model/graph"
	"github.com/alexei-led/archfit/internal/model/symbol"
)

// SymbolSignals carries the SCIP symbol graph. Empty when SCIP is off.
type SymbolSignals struct {
	Graph symbol.Graph
}

// SizeSignals carries per-file line counts (tests excluded) and the full
// file-class index covering ALL visited source files (including test and
// generated). Empty when absent.
type SizeSignals struct {
	FileLOC        map[string]int
	FileClassIndex map[string]fileclass.FileClass // repo-relative slash path → FileClass; nil when loc walk did not run
}

// DuplicationSignals carries duplicated-code clusters from the external clone
// detector. Empty when the tool is off/absent.
type DuplicationSignals struct {
	Clusters []clone.Cluster
}

// DeployUnitSignals carries deterministic deploy-unit detector evidence.
// ModuleUnits is keyed by config module name after deployunit.KeyByModule.
// Report-only except that cmd uses the same evidence to fill missing deploy_unit
// values before classification.
type DeployUnitSignals struct {
	PathUnits   map[string]string
	ModuleUnits map[string]string
}

// RunSignals is the producer-side bundle the cmd layer gathers and hands to the
// engine. The engine folds it (plus its own extract outputs) into a
// CollectedSignals for the metrics — no metric ever sees this type. Each group
// is empty when its source is unavailable.
type RunSignals struct {
	Size        SizeSignals
	Duplication DuplicationSignals
	DeployUnit  DeployUnitSignals
	// ExtraCoverage carries tool-coverage records for opt-in tools that run in cmd
	// (loc, clones) rather than through the engine extractor loop.
	// The engine appends these to the diagnostic ToolCoverage slice.
	ExtraCoverage []diagnostic.Coverage
	// DynamicImports carries the report-only dynamic/lazy-import sites detected by
	// the dynimports adapter. Like ExtraCoverage, the engine reads it straight into
	// the diagnostic (rolled up per module) — no metric ever sees it, and it never
	// touches the dependency graph or the verdict.
	DynamicImports DynamicImportSignals
	// RuntimeAsync carries the async-bridge signals detected by the runtime adapter.
	// The engine rolls the sites up per module and writes them into the diagnostic.
	// Evidence-only — never annotates graph edges, never consumed by classify or
	// score, never changes the gate verdict.
	RuntimeAsync RuntimeAsyncSignals
	// DeprecatedDeps carries the locally-declared deprecation/retraction markers
	// detected by the manifest adapter. Report-only — never reaches a metric or
	// the dependency graph.
	DeprecatedDeps []diagnostic.DeprecatedDep
}

// DynamicImportSignals carries the dynamic/lazy-import sites detected by the
// dynimports adapter. Empty when none were found. Report-only — never reaches a
// metric or the dependency graph.
type DynamicImportSignals struct {
	Sites []diagnostic.DynamicImportSite
}

// RuntimeAsyncSignals carries the async-bridge sites detected by the runtime
// adapter. Empty when none were found. Evidence-only — never annotates graph
// edges, never consumed by classify or score, never alters the verdict.
type RuntimeAsyncSignals struct {
	Sites      []diagnostic.RuntimeAsyncSite
	Confidence string // "low" | "medium"
}

// ---------------------------------------------------------------------------
// Per-family metric inputs (the narrow replacement for the former god input).
//
// Each metric consumes only its family's input, enforced by the compiler: the
// engine assembles a CollectedSignals once and projects it to the per-family
// input each metric declares. Adding a signal touches one group, not every
// metric. Zero-value groups (empty maps/slices) preserve the existing
// "report n/a when the signal is absent" behaviour.
// ---------------------------------------------------------------------------

// CommonInput is the core set every metric can rely on (always populated by the
// pipeline). ChangedFiles is delta-scope, not git history, so it lives here.
type CommonInput struct {
	Graph           *graph.Graph
	Classifications coupling.Index
	Findings        []finding.Finding
	Baseline        diagnostic.MetricSnapshot
	ToolCoverage    []diagnostic.Coverage
	ChangedFiles    []string
	// SyntaxFacts carries the ast-grep syntax facts produced by the syntax provider.
	// Empty when syntax is disabled or sg is absent. The syntax surface renderer
	// and coverage metrics consume this to produce informational counts.
	SyntaxFacts []diagnostic.SyntaxFact
	// DeprecatedDeps carries the locally-declared deprecation/retraction markers
	// detected by the manifest adapter. Empty when none were found. The deprecated-
	// deps renderer consumes this to produce informational counts.
	DeprecatedDeps []diagnostic.DeprecatedDep
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
