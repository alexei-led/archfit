// Package evidence owns the neutral facts exchanged by analysis stages.
package evidence

import (
	modelclone "github.com/alexei-led/archfit/internal/model/clone"
	modevidence "github.com/alexei-led/archfit/internal/model/evidence"
	"github.com/alexei-led/archfit/internal/model/fileclass"
	"github.com/alexei-led/archfit/internal/model/graph"
	"github.com/alexei-led/archfit/internal/model/pattern"
	"github.com/alexei-led/archfit/internal/model/symbol"
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
}
