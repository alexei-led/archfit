// Package signal defines the carrier types that flow between signal producers
// (fitness scanner, git history, complexity tool) and the metrics layer: the
// per-family metric inputs (CommonInput, HistoryInput, SymbolInput, SizeInput,
// ComplexityInput, FitnessInput, DuplicationInput), the CollectedSignals bag the
// engine assembles and projects per family, and the RunSignals bundle the cmd
// layer produces.
//
// Keeping these types here — rather than in internal/metrics or internal/fitness
// — means adding a new signal only churns the new producer package and this
// package. The package joins the model ring: stdlib + internal/model/* imports only.
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

// Signals carries the result of an architecture-fitness scan.
// Each boolean indicates whether a category of enforcement is present.
// EvidencePaths records the matched file paths for explainability.
type Signals struct {
	// ArchTestFiles is true when test files indicate architecture rule
	// enforcement (name contains arch/import-cycle patterns, or content
	// imports a known arch-test library).
	ArchTestFiles bool

	// ImportLinterConfig is true when an import-linter configuration file
	// is present (.importlinter, setup.cfg with [importlinter] section, or
	// pyproject.toml with [tool.importlinter] section).
	ImportLinterConfig bool

	// ArchLinterInCI is true when a CI workflow file references a known
	// architecture-linting tool (archfit, import-linter, deptry,
	// dependency-cruiser, or goda).
	ArchLinterInCI bool

	// EvidencePaths is the list of matched file paths, grouped by category.
	EvidencePaths EvidenceMap
}

// EvidenceMap maps signal category names to the matching file paths.
type EvidenceMap map[string][]string

// ComplexityFunc is one function's cyclomatic complexity, as measured by an
// external multi-language tool (lizard). File is repo-relative.
type ComplexityFunc struct {
	File string
	Name string
	CCN  int
	NLOC int
	Line int
}

// RunSignals is the producer-side bundle the cmd layer gathers and hands to the
// engine: git history, size, complexity, fitness, duplication, and opt-in tool
// coverage. The engine folds it (plus its own extract outputs) into a
// CollectedSignals for the metrics — no metric ever sees this type. Each group
// is empty when its source is unavailable.
type RunSignals struct {
	History     HistorySignals
	Size        SizeSignals
	Complexity  ComplexitySignals
	Fitness     Signals
	Duplication DuplicationSignals
	// ExtraCoverage carries tool-coverage records for opt-in tools that run in cmd
	// (loc, complexity, clones) rather than through the engine extractor loop.
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
	// Empty when syntax is disabled or sg is absent. Report-only metrics (e.g.
	// unsafe_density) consume this to produce informational counts.
	SyntaxFacts []diagnostic.SyntaxFact
	// DeprecatedDeps carries the locally-declared deprecation/retraction markers
	// detected by the manifest adapter. Empty when none were found. Report-only
	// metrics (e.g. deprecated_dep_count) consume this to produce informational counts.
	DeprecatedDeps []diagnostic.DeprecatedDep
}

// HistorySignals carries the git-history signals (empty when no history).
type HistorySignals struct {
	FileChurn   map[string]int    // file -> recent commit count
	CoChange    map[[2]string]int // sorted file pair -> commits touching both
	CommitCount int               // number of commits in the analysed window (0 = not counted / empty)
}

// SymbolSignals carries the SCIP symbol graph and the SCIP-derived per-file
// dependant counts. Empty when SCIP is off.
type SymbolSignals struct {
	Graph            symbol.Graph
	SymbolDependants map[string]int // file -> distinct dependant-file count (SCIP-derived)
}

// SizeSignals carries per-file line counts (tests excluded) and the full
// file-class index covering ALL visited source files (including test and
// generated). Empty when absent.
type SizeSignals struct {
	FileLOC        map[string]int
	FileClassIndex map[string]fileclass.FileClass // repo-relative slash path → FileClass; nil when loc walk did not run
}

// ComplexitySignals carries per-function cyclomatic complexity from the external
// tool. Empty when the opt-in tool is off/absent.
type ComplexitySignals struct {
	Funcs []ComplexityFunc
}

// DuplicationSignals carries duplicated-code clusters from the external clone
// detector. Empty when the tool is off/absent.
type DuplicationSignals struct {
	Clusters []clone.Cluster
}

// HistoryInput is CommonInput plus the git-history signals.
type HistoryInput struct {
	CommonInput
	History HistorySignals
}

// SymbolInput is CommonInput plus the SCIP symbol signals.
type SymbolInput struct {
	CommonInput
	Symbol SymbolSignals
}

// SizeInput is CommonInput plus the size signals.
type SizeInput struct {
	CommonInput
	Size SizeSignals
}

// ComplexityInput is CommonInput plus the complexity signals.
type ComplexityInput struct {
	CommonInput
	Complexity ComplexitySignals
}

// FitnessInput is CommonInput plus the architecture-enforcement scan result.
type FitnessInput struct {
	CommonInput
	Fitness Signals
}

// DuplicationInput is CommonInput plus the clone clusters; it also carries the
// history signals because functional_candidates joins clones with co-change.
// Size carries the FileClassIndex so the metric can filter test/generated
// clone pairs without re-reading files.
type DuplicationInput struct {
	CommonInput
	Duplication DuplicationSignals
	History     HistorySignals
	Size        SizeSignals
}

// CollectedSignals is the engine's producer-side bag of everything gathered for
// a run. It never reaches metric logic directly — the engine projects it to the
// narrow per-family input each metric declares via the As* methods below.
type CollectedSignals struct {
	Common      CommonInput
	History     HistorySignals
	Symbol      SymbolSignals
	Size        SizeSignals
	Complexity  ComplexitySignals
	Fitness     Signals
	Duplication DuplicationSignals
}

// AsCommon projects to the common input.
func (s CollectedSignals) AsCommon() CommonInput { return s.Common }

// AsHistory projects to the history-family input.
func (s CollectedSignals) AsHistory() HistoryInput {
	return HistoryInput{CommonInput: s.Common, History: s.History}
}

// AsSymbol projects to the symbol-family input.
func (s CollectedSignals) AsSymbol() SymbolInput {
	return SymbolInput{CommonInput: s.Common, Symbol: s.Symbol}
}

// AsSize projects to the size-family input.
func (s CollectedSignals) AsSize() SizeInput {
	return SizeInput{CommonInput: s.Common, Size: s.Size}
}

// AsComplexity projects to the complexity-family input.
func (s CollectedSignals) AsComplexity() ComplexityInput {
	return ComplexityInput{CommonInput: s.Common, Complexity: s.Complexity}
}

// AsFitness projects to the fitness-family input.
func (s CollectedSignals) AsFitness() FitnessInput {
	return FitnessInput{CommonInput: s.Common, Fitness: s.Fitness}
}

// AsDuplication projects to the duplication-family input.
func (s CollectedSignals) AsDuplication() DuplicationInput {
	return DuplicationInput{CommonInput: s.Common, Duplication: s.Duplication, History: s.History, Size: s.Size}
}
