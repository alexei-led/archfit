package evaluation

import (
	"time"

	"github.com/alexei-led/archfit/internal/assessment/result"
	"github.com/alexei-led/archfit/internal/assessment/score"
	signal "github.com/alexei-led/archfit/internal/assessment/signals"
	"github.com/alexei-led/archfit/internal/assessment/status"
	"github.com/alexei-led/archfit/internal/model/clone"
	modevidence "github.com/alexei-led/archfit/internal/model/evidence"
	"github.com/alexei-led/archfit/internal/model/fileclass"
	"github.com/alexei-led/archfit/internal/model/pattern"
	"github.com/alexei-led/archfit/internal/model/symbol"
	"github.com/alexei-led/archfit/internal/policy"
	"github.com/alexei-led/archfit/internal/relationship"
	"github.com/alexei-led/archfit/internal/scope"
)

// Observations is the assessment-owned view of what the tools reported. It is
// deliberately narrower than the acquisition snapshot: the dependency graph and
// the classifier index are absent, because every relationship question is
// already answered by the relationship contract. Assessment cannot re-derive a
// relationship even by accident.
type Observations struct {
	Coverage         []modevidence.Coverage
	SuppliedCoverage []modevidence.CoverageIngest
	Symbols          symbol.Graph
	PatternMatches   []pattern.Match
	SyntaxFacts      []modevidence.SyntaxFact
	FileLOC          map[string]int
	FileClassIndex   map[string]fileclass.FileClass
	SourceSelectors  map[string]string
	// OutOfScopeFiles are walked source files the configuration declared
	// outside the analysis scope: an `exclude:` glob matches them, or their
	// language is switched off. Rule scope skips them, so a stray tooling script
	// cannot hold a rule unevaluated for an analyzer the config turned off.
	// Metrics and file classes still count them; nil excludes nothing.
	OutOfScopeFiles map[string]struct{}
	// UnanalysedFiles are in-scope walked source files no dependency producer
	// analyses: their language's extractor applicability probe finds no
	// project under the root (TypeScript below a directory with no root
	// package.json, Go files with no go.mod), which is exactly when the
	// language's primary coverage row is gapless absent, or they are Rust files
	// outside every cargo workspace member. Dependency and module rule scope
	// skips them; forbidden_pattern, which reads the ast-grep pattern pass,
	// keeps them. Nil excludes nothing.
	UnanalysedFiles map[string]struct{}
	// RustModuleNodes are the crate::mod node IDs of the Rust module graph
	// (cargo-modules), sorted. A crate::mod selector under a crate the module
	// graph covers (RustModuleGraphCrates, or a crate with nodes here) is
	// judged against them; under a loaded crate it does not cover it stays
	// undecidable, so a missing module graph never reads as an empty one.
	RustModuleNodes []string
	// RustCrates are the loaded Rust crates (cargo metadata): the crate
	// identifier crate::mod node IDs start with, the library spelling of the
	// package name, and the crate name of every other target of the package.
	// A loaded crate is first-party even when no file selector spells it
	// (binary target yazi of package yazi-fm, binary tool_cli beside lib tool).
	RustCrates []string
	// RustModuleGraphCrates are the crate identifiers cargo-modules graphed,
	// a crate with no submodule included: its module graph is empty, so a
	// crate::mod selector under it provably matches nothing.
	RustModuleGraphCrates []string
	// UnwalkedSourceProduction says, for each dependency-edge source file the
	// LOC walk never visited (a dot directory, target/), whether it is
	// production: a path-only FileClass with the configured globs, false when
	// declared out of scope. module_cycle reads it.
	UnwalkedSourceProduction map[string]bool
	// GoModulePaths are the first-party Go module paths. Go node IDs drop them,
	// so a rule selector spelled with one can never match.
	GoModulePaths []string
	// GoStdlibPackages are the importable Go standard-library package paths of
	// the toolchain that analyses the tree (internal and vendor packages
	// dropped). A to: selector naming one is an external ban even when a
	// first-party directory shares its first segment (database/sql beside a
	// top-level database/). Nil when Go is absent or the toolchain probe failed.
	GoStdlibPackages []string
	// CrateOwners maps each Rust crate spelling (package name and crate
	// identifier) to the declared module that owns the crate, resolved by
	// acquisition (policy.ModuleMap.CrateOwners). The rules use it to place a
	// Rust node no path glob claims, such as a cargo-modules crate::mod node,
	// in its crate's module. Nil when Rust is absent.
	CrateOwners             map[string]string
	FileFacts               []modevidence.FileFact
	Clones                  []clone.Cluster
	DynamicImports          []modevidence.DynamicImportSite
	RuntimeAsyncSites       []modevidence.RuntimeAsyncSite
	RuntimeConfidence       string
	DeprecatedDeps          []modevidence.DeprecatedDep
	SemanticStrengthOverlay *modevidence.SemanticStrengthOverlay

	// Declared and corroborated deploy units remain separate. The corroborated
	// map is keyed by declared module only after acquisition has attributed the
	// detector path; it never overwrites the declaration map.
	DeclaredDeployUnits     map[string]string
	CorroboratedDeployUnits map[string]modevidence.CorroboratedDeployUnit
	OwnerProvenance         map[string]modevidence.OwnerProvenance
}

// AssessInput carries every value the assessment stage decides over. All of it
// is already resolved: assessment reads no configuration file, runs no
// subprocess, and touches no repository. It never receives the full evidence
// snapshot — only the narrow observation projection and the public relationship
// contract.
type AssessInput struct {
	Facts               Observations
	Relationships       relationship.Set
	RelationshipSignals relationship.AssessmentSignals
	Policy              policy.PolicySnapshot
	Accepted            status.AcceptedSet
	BaseMetrics         result.MetricSnapshot
	Scope               scope.Scope
	Now                 time.Time
	BaseRef             string
	Head                string
	// Advisory mirrors the caller's --no-advisories posture.
	Advisory bool

	ConfigSource string
	// ScanRoot is the analysis boundary as the CALLER gave it. Warning hints echo
	// it verbatim; Scope.Root is its canonical form and would not copy-paste back.
	ScanRoot              string
	ConfigHash            string
	ModelHash             string
	ClassificationHash    string
	LabelsHash            string
	PrimaryExtractorTools []string
	OwnerSource           string
	// ConfigWarnings, MarkedCoverage, CoverageGaps, and VolatilityCorroboration
	// are acquisition-resolved run evidence. Assessment attaches them; it never
	// re-derives them, because each depends on the source tree or the config
	// file, which are outside this ring.
	ConfigWarnings          []string
	MarkedCoverage          []modevidence.Coverage
	CoverageGaps            []modevidence.CoverageGap
	VolatilityCorroboration *modevidence.VolatilityCorroboration
	// DeployUnitDetectedModules counts the modules deploy-unit DETECTION mapped,
	// which is not len(Policy.DeployUnits): the snapshot is seeded from declared
	// units and resolution only fills gaps, so a module that both declares a unit
	// and was detected appears in one count and not the other.
	DeployUnitDetectedModules int
}

// Assessed is the pre-score assessment outcome: the diagnostic and the health
// warnings the caller must disclose before scoring.
type Assessed struct {
	Diagnostic result.Result
	Warnings   []string
}

// Assess evaluates rules, metrics, lifecycle status, and the verdict, then
// assembles the report-only evidence blocks into one diagnostic. Scoring, the
// coupling gate, and repair tasks follow in Score.
func Assess(in AssessInput) (Assessed, error) {
	ruleConfig := in.Policy.Gates.Rules
	ruleConfig.ModuleMap = ruleConfig.ModuleMap.WithCrateOwners(in.Facts.CrateOwners)
	ruleset, err := NewRuleset(ruleConfig)
	if err != nil {
		return Assessed{}, err
	}
	if in.Accepted == nil {
		// No persisted baseline: nothing was accepted, so every finding is new.
		in.Accepted = status.Empty{}
	}
	diag := project(in, ruleset, newMetricset(in.Policy.Gates.Metrics))
	diag.SeamEndpointModules = seamEndpointModules(in.Relationships)
	diag.OwnerSource = in.OwnerSource
	diag.VolatilityCorroboration = in.VolatilityCorroboration
	return Assessed{
		Diagnostic: diag,
		Warnings:   healthWarnings(diag, in.CoverageGaps, in.Policy.Topology, in.Facts.FileLOC, in.ScanRoot, in.ConfigSource),
	}, nil
}

// ScoreInput carries the explicit values scoring, the coupling gate, and repair
// tasks need on top of the assessed diagnostic.
type ScoreInput struct {
	Policy         policy.PolicySnapshot
	Facts          Observations
	Anchor         BaselineAnchor
	ConfigSource   string
	ScanRoot       string
	Root           string
	CrateRootDirs  map[string]string
	RequireTools   bool
	ValidationArgs []string
	// ApplyToolGate enables the coverage-gap hard gate. Only the analyze/check
	// use case sets it: every other stage renders a verdict nothing consumes as
	// an exit code, so a required-analyzer gap must not rewrite it there.
	ApplyToolGate bool

	ConfigWarnings []string
	MarkedCoverage []modevidence.Coverage
	CoverageGaps   []modevidence.CoverageGap
}

// Scored is the scoring outcome. GateReasons explain the coupling seam gate —
// a trip when it blocked, an abstention when no comparable reference existed;
// the caller decides whether to disclose them (only `analyze` does). HardGate
// reports that a required analyzer gap must fail the run.
type Scored struct {
	Score       score.Scorecard
	GateReasons []string
	HardGate    bool
}

// Score synthesises the scorecard, applies the coupling gate, attaches repair
// tasks, and stamps the acquisition-resolved coverage evidence onto diag.
func Score(diag *result.Result, in ScoreInput) Scored {
	ruleTypes := ruleTypesOf(in.Policy)
	modulePublic := modulePublicOf(in.Policy)
	// A nil FileClassIndex (the LOC walk did not run) leaves knownFiles nil,
	// which disables agent-task path resolution rather than resolving every
	// candidate against os.Stat alone. Allocating unconditionally would make
	// PathResolver's documented nil contract unreachable.
	var knownFiles map[string]struct{}
	if in.Facts.FileClassIndex != nil {
		knownFiles = make(map[string]struct{}, len(in.Facts.FileClassIndex))
		for file := range in.Facts.FileClassIndex {
			knownFiles[file] = struct{}{}
		}
	}
	gate := in.Policy.Gates.Coupling
	// Stamp the acquisition-resolved coverage BEFORE synthesis. The scorecard's
	// confidence caps read ToolCoverage — the cargo-modules partial-module-graph
	// row only ever exists on the marked copy, so scoring off the raw rows makes
	// that cap dead and reports high confidence over an incomplete Rust graph.
	// Rule and metric evaluation already ran in Assess against the raw rows, so
	// the marked copy cannot move a measured metric.
	diag.ToolCoverage = in.MarkedCoverage
	finalized := finalize(diag, FinalizeInput{
		Gate: gate, Baseline: in.Anchor, RuleTypes: ruleTypes, ModulePublic: modulePublic,
		ValidationCommands: []string{validationCommand(in.ConfigSource, in.ScanRoot, in.ValidationArgs...)},
		KnownFiles:         knownFiles, CrateRootDirs: in.CrateRootDirs,
		ModuleRootDirs: policy.ModuleRootDirs(in.Policy.Topology.Modules),
		OnDisk:         scope.OnDiskWithin(in.Root),
	})
	diag.CoverageGaps = in.CoverageGaps
	diag.ConfigWarnings = in.ConfigWarnings
	attachSeamPolicy(diag, in.Policy)
	// The tool gate runs before the state is built, not inside the returned
	// literal: it can rewrite the verdict, and the state must classify the
	// finalized run, not the one halfway through it.
	hardGate := in.ApplyToolGate && applyToolGate(diag, in.RequireTools)
	diag.State = buildState(diag, stateInput{
		Policy: in.Policy, Facts: in.Facts, RuleTypes: ruleTypes, RequiredToolFailure: hardGate,
		MetricRegressions: blockingMetricRegressions(diag.Metrics, in.Policy.Gates.Metrics),
		Drift:             in.Anchor,
	})
	return Scored{Score: finalized.Score, GateReasons: finalized.GateReasons, HardGate: hardGate}
}

// runSignals projects the acquired facts into the change signals rule and
// metric evaluation read.
func runSignals(f Observations) signal.RunSignals {
	return signal.RunSignals{
		Size:           signal.SizeSignals{FileLOC: f.FileLOC, FileClassIndex: f.FileClassIndex},
		Duplication:    signal.DuplicationSignals{Clusters: f.Clones},
		DynamicImports: signal.DynamicImportSignals{Sites: f.DynamicImports},
		RuntimeAsync:   signal.RuntimeAsyncSignals{Sites: f.RuntimeAsyncSites, Confidence: f.RuntimeConfidence},
		DeprecatedDeps: f.DeprecatedDeps,
	}
}
