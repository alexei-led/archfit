package application

import (
	"testing"

	"github.com/alexei-led/archfit/internal/assessment/result"
	"github.com/alexei-led/archfit/internal/assessment/score"
	"github.com/alexei-led/archfit/internal/model/evidence"
	"github.com/alexei-led/archfit/internal/model/report"
)

// TestProjectReportCarriesEveryEvidenceBlock walks the report projection with
// one populated item per block. ProjectReport is the single seam between the
// assessment result and the published document every renderer reads, and the
// per-item loops are hand-written field-by-field maps: a dropped field here
// produces an empty or wrong JSON block that no renderer test can catch,
// because the renderer tests build report.Document by hand.
const (
	blockFileA   = "a/a.go"
	blockQueue   = "queue"
	blockNats    = "nats"
	blockBaseRef = "main"
)

// populatedResult is an assessment result with exactly one item in every
// report-only evidence block.
func populatedResult() result.Result {
	r := result.New()
	r.FileFacts = []evidence.FileFact{{Module: "a", Files: []string{blockFileA, "a/b.go"}, InboundModuleFanIn: 1, OutboundDestinations: 2, LOC: 40}}
	r.DynamicImports = []evidence.DynamicImport{{Module: "a", Count: 1, Sites: []evidence.DynamicImportSite{
		{File: "a/a.py", Line: 4, Kind: evidence.DynamicImportKindImportlib, Language: "python"},
	}}}
	r.SyntaxFacts = []evidence.SyntaxFact{{Language: "go", File: blockFileA, Module: "a", Kind: "func", Name: "F", Exported: true, StartLine: 1, EndLine: 9, Count: 1, Framework: "none", FrameworkConfirmed: true}}
	r.DeprecatedDeps = []evidence.DeprecatedDep{{File: "go.mod", Kind: "module", Subject: "old/pkg", Note: "deprecated"}}
	r.SemanticStrengthOverlay = &evidence.SemanticStrengthOverlay{ByLanguage: map[string]evidence.SemanticStrengthOverlayStats{
		"typescript": {CandidateEdges: 5, Applied: 3, Missed: 2, Before: map[string]int{mergeFunctional: 5}, After: map[string]int{mergeContract: 3}},
	}}
	r.DynamicConnascenceSignals = &evidence.DynamicConnascenceSignals{
		Unmeasured: []string{"timing"}, ReportOnlyReason: "no runtime trace",
		Signals: []evidence.DynamicConnascenceSignal{{Kind: "execution", Module: "a", Target: "b", IntegrationKind: blockQueue, Count: 1,
			Sites: []evidence.DynamicConnascenceSite{{File: blockFileA, Line: 2, Kind: "publish", Language: "go", Target: "b"}}}},
	}
	r.RuntimeAsync = []evidence.RuntimeAsyncModule{{Module: "a", IntegrationKind: blockQueue, Count: 2, Confidence: "low"}}
	r.RuntimeAsyncEdges = []evidence.RuntimeAsyncEdge{{FromModule: "a", Target: blockNats, IntegrationKind: blockQueue, Count: 1, Confidence: "low",
		Sites: []evidence.RuntimeAsyncSite{{File: blockFileA, Line: 7, Library: blockNats, IntegrationKind: blockQueue, Language: "go"}}}}
	r.Connascence = &evidence.ConnascenceReport{EdgesWithEvidence: 2, AbstainedEdges: 1, TotalEvidence: 3, StrengthInferredEdges: 1,
		ByKind: map[string]int{"name": 3}, BySource: map[string]int{"go/types": 3}, Unmeasured: []string{"timing"},
		Roadmap: []evidence.ConnascenceRoadmapItem{{Kind: "name", CurrentStatus: "deterministic_static", Sources: []string{"go/types"}, RelatedSignals: []string{"x"}, UpgradeTrigger: "y"}}}
	r.DistanceContext = &evidence.DistanceContext{OwnerModel: "codeowners", DistanceBasis: map[string]int{"code_structure": 3}, DeployUnitDetectedModules: 1,
		DeclaredExternalSystems: 1, RuntimeAsyncRelations: 1, RuntimeAsyncKinds: map[string]int{blockQueue: 1}, Interpretation: "i", RuntimeInterpretation: "ri"}
	r.DistanceConfigCandidates = []evidence.DistanceConfigCandidate{{SourceBlock: "runtime_async", Module: "a", Target: blockNats, IntegrationKind: blockQueue, Count: 1,
		EvidenceSites:         []evidence.DistanceConfigEvidenceSite{{File: blockFileA, Line: 7, Kind: "publish", Language: "go", Target: blockNats}},
		SuggestedReviewAction: "declare external_systems.nats"}}
	r.VolatilityCorroboration = &evidence.VolatilityCorroboration{Source: gitSource, Status: "ok", CommitWindow: 90, FullHistory: true, CommitsScanned: 12, ModulesTouched: 2, Caveat: "c",
		TopTouched: []evidence.VolatilityTouch{{Module: "a", TouchCommits: 9, DeclaredVolatility: stateSevHigh}}}
	r.LocalCoupling = []evidence.LocalCouplingModule{{Module: "a", ScoredEdges: 4, AbstainedEdges: 1, ComplexityEdges: 2, ComplexitySharePct: 50, MeanBalance: 3.5,
		WorstOffenders: []evidence.LocalCouplingEdge{{From: blockFileA, To: "a/b.go", Strength: mergeContract, Balance: 2, Band: "critical", File: blockFileA, Line: 3}}}}
	r.Delta = &result.DeltaReport{New: []string{"n"}, Existing: []string{"e"}, Resolved: []string{"r"}, SeverityChanged: []string{"s"}, TouchedByDelta: []string{"t"}}
	r.GitFindingDelta = &result.GitFindingDelta{BaseRef: blockBaseRef, ComparisonStatus: "comparable", IntroducedFindingIDs: []string{"i"}, PreExistingFindingIDs: []string{"p"}, UnknownOriginFindingIDs: []string{"u"}, ComparisonReasons: []string{"why"}}

	return r
}

// blockCheck asserts that one report-only evidence block survived projection.
type blockCheck struct {
	block string
	ok    func() bool
}

func reportBlockChecks(doc report.Document) []blockCheck {
	return []blockCheck{
		{"file facts", func() bool {
			return len(doc.FileFacts) == 1 && doc.FileFacts[0].LOC == 40 &&
				doc.FileFacts[0].OutboundDestinations == 2 && len(doc.FileFacts[0].Files) == 2
		}},
		{"dynamic imports", func() bool {
			return len(doc.DynamicImports) == 1 && len(doc.DynamicImports[0].Sites) == 1 &&
				doc.DynamicImports[0].Sites[0].Line == 4
		}},
		{"syntax facts", func() bool {
			return len(doc.SyntaxFacts) == 1 && doc.SyntaxFacts[0].Exported && doc.SyntaxFacts[0].EndLine == 9
		}},
		{"deprecated deps", func() bool {
			return len(doc.DeprecatedDeps) == 1 && doc.DeprecatedDeps[0].Subject == "old/pkg"
		}},
		{"semantic overlay", func() bool {
			return doc.SemanticStrengthOverlay != nil && doc.SemanticStrengthOverlay.ByLanguage["typescript"].Missed == 2
		}},
		{"dynamic connascence", func() bool {
			return doc.DynamicConnascenceSignals != nil && len(doc.DynamicConnascenceSignals.Signals) == 1 &&
				len(doc.DynamicConnascenceSignals.Signals[0].Sites) == 1 &&
				doc.DynamicConnascenceSignals.Signals[0].Sites[0].Target == "b"
		}},
		{"runtime async modules", func() bool {
			return len(doc.RuntimeAsync) == 1 && doc.RuntimeAsync[0].Count == 2
		}},
		{"runtime async edges", func() bool {
			return len(doc.RuntimeAsyncEdges) == 1 && len(doc.RuntimeAsyncEdges[0].Sites) == 1 &&
				doc.RuntimeAsyncEdges[0].Sites[0].Library == blockNats
		}},
		{"connascence", func() bool {
			return doc.Connascence != nil && len(doc.Connascence.Roadmap) == 1 &&
				doc.Connascence.Roadmap[0].UpgradeTrigger == "y"
		}},
		{"distance context", func() bool {
			return doc.DistanceContext != nil && doc.DistanceContext.RuntimeInterpretation == "ri" &&
				doc.DistanceContext.DistanceBasis["code_structure"] == 3
		}},
		{"distance candidates", func() bool {
			return len(doc.DistanceConfigCandidates) == 1 && len(doc.DistanceConfigCandidates[0].EvidenceSites) == 1 &&
				doc.DistanceConfigCandidates[0].EvidenceSites[0].Target == blockNats
		}},
		{"volatility corroboration", func() bool {
			return doc.VolatilityCorroboration != nil && len(doc.VolatilityCorroboration.TopTouched) == 1 &&
				doc.VolatilityCorroboration.TopTouched[0].TouchCommits == 9
		}},
		{"local coupling", func() bool {
			return len(doc.LocalCoupling) == 1 && len(doc.LocalCoupling[0].WorstOffenders) == 1 &&
				doc.LocalCoupling[0].WorstOffenders[0].Band == "critical"
		}},
		{"delta", func() bool {
			return doc.Delta != nil && len(doc.Delta.TouchedByDelta) == 1 && len(doc.Delta.SeverityChanged) == 1
		}},
		{"git finding delta", func() bool {
			return doc.GitFindingDelta != nil && len(doc.GitFindingDelta.ComparisonReasons) == 1
		}},
	}
}

func TestProjectReportCarriesEveryEvidenceBlock(t *testing.T) {
	doc := ProjectReport(populatedResult(), score.Scorecard{RubricVersion: score.RubricVersion}, nil, false)
	for _, test := range reportBlockChecks(doc) {
		if !test.ok() {
			t.Errorf("%s block lost data in projection", test.block)
		}
	}
}
