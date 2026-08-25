// Package evidence owns the immutable facts exchanged by analysis stages.
package evidence

import (
	"time"

	modelclone "github.com/alexei-led/archfit/internal/model/clone"
	modevidence "github.com/alexei-led/archfit/internal/model/evidence"
	"github.com/alexei-led/archfit/internal/model/fileclass"
	"github.com/alexei-led/archfit/internal/model/graph"
	"github.com/alexei-led/archfit/internal/model/pattern"
	"github.com/alexei-led/archfit/internal/model/symbol"
	"github.com/alexei-led/archfit/internal/relationship/labels"
	"github.com/alexei-led/archfit/internal/scope"
)

// Snapshot is the neutral, immutable fact bundle exchanged by the technical
// stages. It contains acquisition output and run context only. Policy,
// assessment rules and metrics, lifecycle status, baselines, and report models
// deliberately do not cross this boundary.
type Snapshot struct {
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
	DeployUnitsByModule     map[string]string
	SemanticStrengthOverlay *modevidence.SemanticStrengthOverlay

	Scope                 scope.Scope
	BaseRef               string
	Full                  bool
	PinnedLabels          []labels.Label
	Now                   time.Time
	ConfigHash            string
	PrimaryExtractorTools []string
	ConfigSource          string
	BundleDir             string
}

// AssessmentView is the narrow evidence projection consumed by assessment.
type AssessmentView struct {
	Graph                   *graph.Graph
	Coverage                []modevidence.Coverage
	ChangedFiles            []string
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
	Scope                   scope.Scope
	Now                     time.Time
	ConfigHash              string
	PrimaryExtractorTools   []string
	ConfigSource            string
	BundleDir               string
	DeployUnitsByModule     map[string]string
}

// AssessmentView returns the assessment-only projection of s.
func (s Snapshot) AssessmentView() AssessmentView {
	return AssessmentView{
		Graph: s.Graph, Coverage: s.Coverage, ChangedFiles: s.Scope.Changed, Symbols: s.Symbols,
		FileLOC: s.FileLOC, FileClassIndex: s.FileClassIndex, Clones: s.Clones,
		PatternMatches: s.PatternMatches, SyntaxFacts: s.SyntaxFacts, DynamicImports: s.DynamicImports,
		RuntimeAsyncSites: s.RuntimeAsyncSites, RuntimeConfidence: s.RuntimeConfidence, DeprecatedDeps: s.DeprecatedDeps,
		SemanticStrengthOverlay: s.SemanticStrengthOverlay, Scope: s.Scope,
		Now: s.Now, ConfigHash: s.ConfigHash, PrimaryExtractorTools: s.PrimaryExtractorTools,
		ConfigSource: s.ConfigSource, BundleDir: s.BundleDir, DeployUnitsByModule: s.DeployUnitsByModule,
	}
}

// Coverage is retained as a discoverable alias for model evidence coverage.
type Coverage = modevidence.Coverage
