package evaluation

import (
	"time"

	"github.com/alexei-led/archfit/v3/internal/assessment/result"
	"github.com/alexei-led/archfit/v3/internal/assessment/score"
	signal "github.com/alexei-led/archfit/v3/internal/assessment/signals"
	modevidence "github.com/alexei-led/archfit/v3/internal/model/evidence"
	"github.com/alexei-led/archfit/v3/internal/model/fileclass"
	"github.com/alexei-led/archfit/v3/internal/model/symbol"
	"github.com/alexei-led/archfit/v3/internal/policy"
	"github.com/alexei-led/archfit/v3/internal/relationship"
	"github.com/alexei-led/archfit/v3/internal/scope"
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
	SyntaxFacts      []modevidence.SyntaxFact
	FileLOC          map[string]int
	FileClassIndex   map[string]fileclass.FileClass
	OutOfScopeFiles  map[string]struct{}
	// ScopedDependencyEvidence names per-file incompleteness, so a completed
	// project-level producer cannot falsely complete a report dimension.
	ScopedDependencyEvidence map[string]ScopedDependencyEvidence
	FileFacts                []modevidence.FileFact
	DynamicImports           []modevidence.DynamicImportSite
	RuntimeAsyncSites        []modevidence.RuntimeAsyncSite
	RuntimeConfidence        string
	DeprecatedDeps           []modevidence.DeprecatedDep

	// Declared and corroborated deploy units remain separate. The corroborated
	// map is keyed by declared module only after acquisition has attributed the
	// detector path; it never overwrites the declaration map.
	DeclaredDeployUnits     map[string]string
	CorroboratedDeployUnits map[string]modevidence.CorroboratedDeployUnit
	OwnerProvenance         map[string]modevidence.OwnerProvenance
}

// ScopedDependencyEvidence is one language's dependency-evidence completeness
// per source file, for producers that judge completeness per source unit (a
// .NET project) rather than per run.
type ScopedDependencyEvidence struct {
	// Tool is the producer the gaps are about, as its coverage row names it.
	Tool string
	// Incomplete maps each in-scope source file whose unit lacks complete
	// evidence to that unit's gap, "<unit> <status>". Files of one unit share
	// one gap.
	Incomplete map[string]string
	// Fixes maps each gap to what restores it.
	Fixes map[string]string
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
	PrimaryExtractorTools []string
	OwnerSource           string
	// ConfigWarnings, MarkedCoverage, and CoverageGaps
	// are acquisition-resolved run evidence. Assessment attaches them; it never
	// re-derives them, because each depends on the source tree or the config
	// file, which are outside this ring.
	ConfigWarnings []string
	MarkedCoverage []modevidence.Coverage
	CoverageGaps   []modevidence.CoverageGap
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

// Assess calculates metrics and relationship advisories, then assembles the
// report evidence. Repository errors and exceptions are evaluated separately.
func Assess(in AssessInput) Assessed {
	diag := project(in, newMetricset(in.Policy.Gates.Metrics))
	diag.OwnerSource = in.OwnerSource
	return Assessed{
		Diagnostic: diag,
		Warnings:   healthWarnings(diag, in.CoverageGaps, in.Policy.Topology, in.Facts.FileLOC, in.ScanRoot, in.ConfigSource),
	}
}

// ScoreInput carries measurement and acquisition context for report scoring.
type ScoreInput struct {
	Policy         policy.PolicySnapshot
	Facts          Observations
	ConfigWarnings []string
	MarkedCoverage []modevidence.Coverage
	CoverageGaps   []modevidence.CoverageGap
}

// Scored holds the diagnostic scorecard.
type Scored struct {
	Score score.Scorecard
}

// Score synthesises the scorecard and measurement envelopes using the
// acquisition-resolved coverage evidence. It produces no verdict.
func Score(diag *result.Result, in ScoreInput) Scored {
	// Stamp the acquisition-resolved coverage BEFORE synthesis. The scorecard's
	// confidence caps read ToolCoverage — the cargo-modules partial-module-graph
	// row only ever exists on the marked copy, so scoring off the raw rows makes
	// that cap dead and reports high confidence over an incomplete Rust graph.
	// Metric evaluation already ran in Assess against the raw rows, so
	// the marked copy cannot move a measured metric.
	diag.ToolCoverage = in.MarkedCoverage
	card := score.Synthesize(*diag)
	diag.CoverageGaps = in.CoverageGaps
	diag.ConfigWarnings = in.ConfigWarnings
	diag.State = buildState(diag, stateInput{Policy: in.Policy, Facts: in.Facts})
	return Scored{Score: card}
}

// runSignals projects the acquired facts into the change signals rule and
// metric evaluation read.
func runSignals(f Observations) signal.RunSignals {
	return signal.RunSignals{
		Size:           signal.SizeSignals{FileLOC: f.FileLOC, FileClassIndex: f.FileClassIndex},
		DynamicImports: signal.DynamicImportSignals{Sites: f.DynamicImports},
		RuntimeAsync:   signal.RuntimeAsyncSignals{Sites: f.RuntimeAsyncSites, Confidence: f.RuntimeConfidence},
		DeprecatedDeps: f.DeprecatedDeps,
	}
}
