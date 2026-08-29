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
	Coverage                []modevidence.Coverage
	SuppliedCoverage        []modevidence.CoverageIngest
	Symbols                 symbol.Graph
	PatternMatches          []pattern.Match
	SyntaxFacts             []modevidence.SyntaxFact
	FileLOC                 map[string]int
	FileClassIndex          map[string]fileclass.FileClass
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
	ruleset, err := NewRuleset(in.Policy.Gates.Rules)
	if err != nil {
		return Assessed{}, err
	}
	if in.Accepted == nil {
		// No persisted baseline: nothing was accepted, so every finding is new.
		in.Accepted = status.Empty{}
	}
	diag := project(in, ruleset, newMetricset(in.Policy.Gates.Metrics))
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
	Policy        policy.PolicySnapshot
	Facts         Observations
	Anchor        BaselineAnchor
	ConfigSource  string
	ScanRoot      string
	Root          string
	CrateRootDirs map[string]string
	RequireTools  bool
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
	ruleTypes := make(map[string]string, len(in.Policy.Gates.Rules.Rules))
	for _, def := range in.Policy.Gates.Rules.Rules {
		ruleTypes[def.ID] = def.Type
	}
	modulePublic := make(map[string][]string, len(in.Policy.Topology.Modules))
	for name, def := range in.Policy.Topology.Modules {
		if len(def.Public) > 0 {
			modulePublic[name] = def.Public
		}
	}
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
		ValidationCommands: []string{validationCommand(in.ConfigSource, in.ScanRoot)},
		KnownFiles:         knownFiles, CrateRootDirs: in.CrateRootDirs,
		ModuleRootDirs: policy.ModuleRootDirs(in.Policy.Topology.Modules),
		OnDisk:         scope.OnDiskWithin(in.Root),
	})
	diag.CoverageGaps = in.CoverageGaps
	diag.ConfigWarnings = in.ConfigWarnings
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
