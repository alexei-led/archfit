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
	Symbols                 symbol.Graph
	PatternMatches          []pattern.Match
	SyntaxFacts             []modevidence.SyntaxFact
	FileLOC                 map[string]int
	FileClassIndex          map[string]fileclass.FileClass
	Clones                  []modelclone.Cluster
	DynamicImports          []modevidence.DynamicImportSite
	RuntimeAsyncSites       []modevidence.RuntimeAsyncSite
	RuntimeConfidence       string
	DeprecatedDeps          []modevidence.DeprecatedDep
	SemanticStrengthOverlay *modevidence.SemanticStrengthOverlay
}

// AssessmentFacts is the narrow observation projection consumed by assessment.
// It drops the raw pattern/graph acquisition detail assessment never reads back
// through, and carries no run context.
type AssessmentFacts struct {
	Graph                   *graph.Graph
	Coverage                []modevidence.Coverage
	Symbols                 symbol.Graph
	FileLOC                 map[string]int
	FileClassIndex          map[string]fileclass.FileClass
	Clones                  []modelclone.Cluster
	PatternMatches          []pattern.Match
	SyntaxFacts             []modevidence.SyntaxFact
	DynamicImports          []modevidence.DynamicImportSite
	RuntimeAsyncSites       []modevidence.RuntimeAsyncSite
	RuntimeConfidence       string
	DeprecatedDeps          []modevidence.DeprecatedDep
	SemanticStrengthOverlay *modevidence.SemanticStrengthOverlay
}

// ForAssessment returns the assessment-only projection of f.
func (f Facts) ForAssessment() AssessmentFacts {
	return AssessmentFacts{
		Graph: f.Graph, Coverage: f.Coverage, Symbols: f.Symbols,
		FileLOC: f.FileLOC, FileClassIndex: f.FileClassIndex, Clones: f.Clones,
		PatternMatches: f.PatternMatches, SyntaxFacts: f.SyntaxFacts, DynamicImports: f.DynamicImports,
		RuntimeAsyncSites: f.RuntimeAsyncSites, RuntimeConfidence: f.RuntimeConfidence, DeprecatedDeps: f.DeprecatedDeps,
		SemanticStrengthOverlay: f.SemanticStrengthOverlay,
	}
}

// Coverage is retained as a discoverable alias for model evidence coverage.
type Coverage = modevidence.Coverage
