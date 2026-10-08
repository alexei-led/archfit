// Package evidence owns the neutral facts exchanged by analysis stages.
package evidence

import (
	modelclone "github.com/alexei-led/archfit/v3/internal/model/clone"
	modevidence "github.com/alexei-led/archfit/v3/internal/model/evidence"
	"github.com/alexei-led/archfit/v3/internal/model/fileclass"
	"github.com/alexei-led/archfit/v3/internal/model/graph"
	"github.com/alexei-led/archfit/v3/internal/model/pattern"
	"github.com/alexei-led/archfit/v3/internal/model/symbol"
)

// Facts is the neutral, immutable observation bundle produced by acquisition:
// what the source tree and the external tools reported, and nothing else. Run
// context (scope, instant, config identity, bundle paths) belongs to the
// application's analysis context; policy, assessment rules and metrics,
// lifecycle status, baselines, and report models deliberately never cross this
// boundary.
type Facts struct {
	Graph                   *graph.Graph
	Coverage                []modevidence.Coverage
	SuppliedCoverage        []modevidence.CoverageIngest
	Symbols                 symbol.Graph
	PatternMatches          []pattern.Match
	SyntaxFacts             []modevidence.SyntaxFact
	FileLOC                 map[string]int
	FileClassIndex          map[string]fileclass.FileClass
	FileFacts               []modevidence.FileFact
	Clones                  []modelclone.Cluster
	DynamicImports          []modevidence.DynamicImportSite
	RuntimeAsyncSites       []modevidence.RuntimeAsyncSite
	RuntimeConfidence       string
	DeprecatedDeps          []modevidence.DeprecatedDep
	SemanticStrengthOverlay *modevidence.SemanticStrengthOverlay
	// RustModuleGraphCrates are the crate identifiers cargo-modules graphed,
	// a crate with no submodule included: its module graph is empty, not
	// missing.
	RustModuleGraphCrates []string
	// RustTargetCrates are the crate names of every target cargo metadata
	// lists for a loaded workspace member (library, binaries, and the rest), in
	// crate spelling. A binary target of a lib+bin package is a crate of its own
	// that CrateRoot.Crate, the library, does not name.
	RustTargetCrates []string
}
