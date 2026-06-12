// Package signal defines the carrier types that flow between signal producers
// (fitness scanner, git history, complexity tool) and the metrics layer.
//
// Keeping these types here — rather than in internal/metrics or internal/fitness
// — means adding a new signal only churns the new producer package and this
// package. Neither metrics nor engine need to change for a new signal field.
// The package joins the model ring: stdlib + internal/model/* imports only.
package signal

import (
	"github.com/alexei-led/archfit/internal/model/clone"
	"github.com/alexei-led/archfit/internal/model/coupling"
	"github.com/alexei-led/archfit/internal/model/diagnostic"
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

// ChangeHistory carries git-derived volatility signals into the engine for the
// modularity metrics. Both maps are empty when no git history is available.
type ChangeHistory struct {
	FileChurn      map[string]int    // file -> recent commit count
	CoChange       map[[2]string]int // sorted file pair -> commits touching both
	FileLOC        map[string]int    // source file -> lines of code (tests excluded)
	Complexity     []ComplexityFunc  // per-function cyclomatic complexity (external tool)
	FitnessSignals Signals           // architecture-intent enforcement signals (filesystem scan)
	CloneClusters  []clone.Cluster   // duplicated code blocks across files (clone detector)
	// GitnexusImpact maps repo-relative file path → distinct dependant-file count from the
	// gitnexus CLI. Nil/empty when gitnexus is disabled or absent; risk_hub uses it
	// as an optional multiplicative factor (never alters surface-breadth computation).
	GitnexusImpact map[string]int
	// ExtraCoverage carries tool-coverage records for opt-in tools that run in cmd
	// (clones, gitnexus) rather than through the engine extractor loop. The engine
	// appends these to the diagnostic ToolCoverage slice after building its own records.
	ExtraCoverage []diagnostic.Coverage
}

// MetricInput is the complete input set for all metrics.
type MetricInput struct {
	Graph           *graph.Graph
	Classifications coupling.Index
	Findings        []finding.Finding // status-tagged findings from status stage
	Baseline        diagnostic.MetricSnapshot
	ToolCoverage    []diagnostic.Coverage // from extractors
	// FileChurn maps a repo-relative source file to its recent commit count (git
	// history). Empty when no git history is available; modularity metrics that
	// depend on volatility then report n/a.
	FileChurn map[string]int
	// CoChange maps a sorted file pair to the number of commits that touched both —
	// the logical-coupling signal for hidden_coupling. Empty when unavailable.
	CoChange map[[2]string]int
	// FileLOC maps a source file to its line count (tests excluded), for the
	// structural_weight (size-skew / god-module) metric. Empty when unavailable.
	FileLOC map[string]int
	// Complexity is per-function cyclomatic complexity from an external tool
	// (lizard), for the complexity metric. Empty when the opt-in tool is off/absent.
	Complexity []ComplexityFunc
	// SymbolGraph is per-symbol module ownership, fan-in, and cross-module reference
	// edges from a SCIP index. Empty when SCIP is off or the indexer is absent;
	// metrics that need it must report n/a when SymbolGraph.Empty() is true.
	SymbolGraph symbol.Graph
	// FitnessSignals carries the results of the architecture-intent enforcement scan
	// (arch tests, import-linter config, arch-linter in CI). Zero-value Signals
	// (EvidencePaths == nil) means the scan was never run; metrics must report n/a.
	FitnessSignals Signals
	// CloneClusters holds duplicated-code blocks detected by an external clone
	// detector (e.g. jscpd). Empty when the tool is disabled or absent; metrics
	// that need it must report n/a when CloneClusters is nil/empty.
	CloneClusters []clone.Cluster
	// GitnexusImpact maps repo-relative file path → distinct dependant-file count from the
	// gitnexus CLI (tools.gitnexus.enabled: on). Nil/empty (the default) leaves
	// risk_hub behaviour exactly as today (surface-breadth × volatility only).
	// When non-empty, risk_hub incorporates it as a bounded additional factor.
	GitnexusImpact map[string]int
	// ChangedFiles is the sorted repo-relative diff file set (scope.Scope.Changed)
	// in delta mode; empty in full mode. change_locality reports n/a without it.
	ChangedFiles []string
}
