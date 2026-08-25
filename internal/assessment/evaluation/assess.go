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
}

// AssessInput carries every value the assessment stage decides over. All of it
// is already resolved: assessment reads no configuration file, runs no
// subprocess, and touches no repository. It never receives the full evidence
// snapshot — only the narrow observation projection and the public relationship
// contract.
type AssessInput struct {
	Facts         Observations
	Relationships relationship.AnalysisResult
	Policy        policy.PolicySnapshot
	Accepted      status.AcceptedSet
	BaseMetrics   result.MetricSnapshot
	Scope         scope.Scope
	Now           time.Time
	BaseRef       string
	Head          string
	// Advisory mirrors the caller's --no-advisories posture; CaptureRelationships
	// records the classified set for the enrichment use case.
	Advisory             bool
	CaptureRelationships bool

	ConfigSource          string
	ConfigHash            string
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
}

// Assessed is the pre-score assessment outcome: the diagnostic, the relationship
// set the metric pass observed (enrichment only), and the health warnings the
// caller must disclose before scoring.
type Assessed struct {
	Diagnostic result.Result
	Captured   relationship.Set
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
	diag, captured := project(in, ruleset, newMetricset(in.Policy.Gates.Metrics))
	diag.OwnerSource = in.OwnerSource
	diag.DistanceContext = buildDistanceContext(diag, in.Policy, len(in.Policy.DeployUnits))
	diag.VolatilityCorroboration = in.VolatilityCorroboration
	return Assessed{
		Diagnostic: diag,
		Captured:   captured,
		Warnings:   healthWarnings(diag, in.Policy.Topology.Modules, in.Scope.Root, in.ConfigSource),
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

	ConfigWarnings []string
	MarkedCoverage []modevidence.Coverage
	CoverageGaps   []modevidence.CoverageGap
}

// Scored is the scoring outcome. GateReasons explain a tripped coupling gate
// and AnchorStale reports that max_drop was skipped because the stored score
// snapshot is incompatible with this binary; the caller decides whether to
// disclose either (only `analyze` does). HardGate reports that a required
// analyzer gap must fail the run.
type Scored struct {
	Score       score.Scorecard
	GateReasons []string
	AnchorStale bool
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
	knownFiles := make(map[string]struct{}, len(in.Facts.FileClassIndex))
	for file := range in.Facts.FileClassIndex {
		knownFiles[file] = struct{}{}
	}
	gate := in.Policy.Gates.Coupling
	finalized := finalize(diag, FinalizeInput{
		Gate: gate, Baseline: in.Anchor, RuleTypes: ruleTypes, ModulePublic: modulePublic,
		ValidationCommands: []string{validationCommand(in.ConfigSource, in.ScanRoot)},
		KnownFiles:         knownFiles, CrateRootDirs: in.CrateRootDirs,
		ModuleRootDirs: policy.ModuleRootDirs(in.Policy.Topology.Modules),
		OnDisk:         scope.OnDiskWithin(in.Root),
	})
	diag.ToolCoverage = in.MarkedCoverage
	diag.CoverageGaps = in.CoverageGaps
	diag.ConfigWarnings = in.ConfigWarnings
	return Scored{
		Score: finalized.Score, GateReasons: finalized.GateReasons,
		AnchorStale: couplingGateAnchorStale(gate, in.Anchor),
		HardGate:    applyToolGate(diag, in.RequireTools),
	}
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
