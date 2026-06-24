package engine

import (
	"context"
	"errors"
	"time"

	"github.com/alexei-led/archfit/internal/classify"
	"github.com/alexei-led/archfit/internal/config"
	"github.com/alexei-led/archfit/internal/facts"
	"github.com/alexei-led/archfit/internal/labels"
	"github.com/alexei-led/archfit/internal/metrics"
	"github.com/alexei-led/archfit/internal/model/coupling"
	"github.com/alexei-led/archfit/internal/model/diagnostic"
	"github.com/alexei-led/archfit/internal/model/finding"
	"github.com/alexei-led/archfit/internal/model/graph"
	"github.com/alexei-led/archfit/internal/model/signal"
	"github.com/alexei-led/archfit/internal/model/symbol"
	"github.com/alexei-led/archfit/internal/ports"
	"github.com/alexei-led/archfit/internal/rules"
	"github.com/alexei-led/archfit/internal/scope"
	"github.com/alexei-led/archfit/internal/status"
	"github.com/alexei-led/archfit/internal/syntax"
)

// Mode controls how the engine run behaves.
type Mode struct {
	Base       string   // git ref to diff against (empty = none)
	Head       string   // git ref of the current HEAD (empty = working tree)
	Full       bool     // if true, full-repo mode (no diff filter)
	Advisory   bool     // if true, include advisory findings (bc/imbalanced_coupling, map/staleness) in Findings and Summary.Warnings
	ReportOnly bool     // report-only: metric regressions are non-blocking
	Formats    []string // output formats to render (e.g. ["json", "console"])
}

// RunInput carries everything a pipeline run needs. Fields mirror the
// pipeline stages; all are required unless noted.
type RunInput struct {
	Mode        Mode
	Scope       scope.Scope
	Classify    config.ClassifyConfig
	Staleness   config.StalenessConfig
	Exceptions  config.ExceptionSet
	Extractors  []ports.Extractor
	Patterns    ports.PatternProvider
	PatternCfg  config.PatternConfig
	Resolver    ports.SymbolResolver
	Syntax      ports.SyntaxProvider // syntactic declaration/route provider; nil = Nop
	SyntaxCfg   config.SyntaxConfig  // derived from ForSyntax(); Enabled gates the call
	Rules       []rules.Rule
	Metrics     []metrics.Metric
	Accepted    status.AcceptedSet
	BaseMetrics diagnostic.MetricSnapshot // baseline metric snapshot; nil = no baseline
	Labels      []labels.Label            // pinned coupling labels; nil = none
	Signals     signal.RunSignals
	Now         time.Time
	// PrimaryExtractorTools names the per-language file extractors whose coverage
	// the scorecard treats as load-bearing (see diagnostic.Diagnostic). Supplied by
	// the composition root from the language registry; attached to the Diagnostic so
	// the score package needs no hardcoded tool list. Empty = score's default set.
	PrimaryExtractorTools []string
	// ConfigHash is the sha256 hex digest of the raw .archfit.yaml bytes,
	// computed by the caller before parsing. Empty when no config file was loaded.
	// Attached to the Diagnostic for reproducibility: same config + same repo → same hash.
	ConfigHash string
}

// extractResult holds the outputs of the extract stage.
type extractResult struct {
	g           *graph.Graph
	coverages   []diagnostic.Coverage
	scipSymbols symbol.Graph
}

// evidenceResult holds the outputs of the resolveEvidence stage.
type evidenceResult struct {
	resolvedFindings []finding.Finding
}

// Run executes the full archfit pipeline and returns the assembled Diagnostic.
//
// Pipeline stages:
//  1. Run each extractor sequentially; apply symbol resolution to edges; merge Facts; build graph.
//  2. Run PatternProvider to gather structural matches; build per-file evidence index.
//  3. Classify edges: classify.Run → coupling.Index.
//  4. Apply rules: rule.Check(g, Evidence{PatternMatches}) per rule → raw findings (flattened).
//  5. Assign statuses: status.Assign → lifecycle-tagged findings.
//  6. Compute metrics: build CollectedSignals, run each metric → MetricResult slice.
//  7. Resolve evidence: join module labels and severity onto findings.
//  8. Collect advisory findings: coupling edges with Severity != "" + staleness.Check.
//  9. Assemble Diagnostic: fill Summary, compute verdict, attach structural facts.
//     Advisory findings included only when mode.Advisory.
//     Return (Diagnostic, nil) on success; (Diagnostic, error) on hard error.
//
// Rendering is the caller's responsibility (cmd renders to deps.Stdout).
func Run(ctx context.Context, in RunInput) (diagnostic.Diagnostic, error) {
	// --- Stage 1: Extract ---
	// Run each extractor; apply symbol resolution and SCIP strength to edges before merging.
	ex, err := extract(ctx, in)
	if err != nil {
		return diagnostic.New(), err
	}

	// --- Stage 2: Pattern matching ---
	// Gather structural matches from the PatternProvider and build a per-file evidence index.
	// Matches are keyed by file path so rules can filter by the edge's from-file.
	patternMatches, ppCov, ppErr := in.Patterns.Find(ctx, in.Scope, in.PatternCfg)
	if ppErr != nil {
		return diagnostic.New(), ppErr
	}
	ex.coverages = append(ex.coverages, ppCov)

	// --- Stage 2b: Syntax facts ---
	// Collect syntactic declarations and route registrations (ast-grep rules).
	// Derive roles, build NodeRoleIndex for rule consumption — all before the
	// rules stage. Off-gate: facts populate the report but never affect the verdict.
	var syntaxFacts []diagnostic.SyntaxFact
	var nodeRoleIndex *syntax.NodeRoleIndex
	if in.SyntaxCfg.Enabled {
		if in.Syntax == nil {
			return diagnostic.New(), errors.New("engine: SyntaxCfg.Enabled=true but no Syntax provider")
		}
		sf, synCov, synErr := in.Syntax.Syntax(ctx, in.Scope, in.SyntaxCfg.Languages)
		if synErr != nil {
			return diagnostic.New(), synErr
		}
		syntaxFacts = syntax.DeriveRoles(sf)
		nodeRoleIndex = syntax.BuildNodeRoleIndex(ex.g, syntaxFacts)
		ex.coverages = append(ex.coverages, synCov)
	}

	// --- Stage 3: Classify ---
	// Pinned coupling labels first: approved entries refine strength
	// classification (precedence: config globs > approved labels > extractor
	// hint); stale ones surface as labels/stale advisories.
	classifyCfg := in.Classify
	staleLabelFindings, llmApprovedCount := applyPinnedLabels(ex.g, &classifyCfg, in.Mode, in.Labels)
	// Thread clone pairs for CoA (connascence of algorithm) tagging — report-only.
	if len(in.Signals.Duplication.Clusters) > 0 {
		classifyCfg.CrossModuleClonePairs = buildClonePairSet(in.Signals.Duplication.Clusters, classifyCfg.ModuleMap)
	}

	// Register auto-discovered module-graph nodes (Rust "<crate>::<mod>") as modules so
	// classify can resolve their distance/volatility; otherwise their edges are
	// distance-unknown and coupling_balance/encapsulation never see them. No-op for
	// Go/TS/Python (their nodes are already configured; the "::" gate excludes them).
	classifyCfg.Modules = classify.AugmentModulesFromGraph(ex.g, classifyCfg.Modules)

	// Runtime async evidence: build per-module rollup for the diagnostic.
	// Report-only — never changes the gate verdict.
	runtimeAsync := buildRuntimeAsync(in.Signals.RuntimeAsync.Sites, in.Signals.RuntimeAsync.Confidence, classifyCfg.ModuleMap)

	couplingIdx := classify.Run(ex.g, classifyCfg)

	// --- Stage 4: Rules ---
	// Call each rule once with the full evidence set. Rules iterate edges internally;
	// the Evidence carries all pattern matches so each rule can filter by edge's from-file.
	var rawFindings []finding.Finding
	allPatternMatches := rules.Evidence{PatternMatches: patternMatches, Roles: nodeRoleIndex, SyntaxFacts: syntaxFacts}
	for _, r := range in.Rules {
		rawFindings = append(rawFindings, r.Check(ex.g, allPatternMatches)...)
	}

	// --- Stage 5: Status ---
	taggedFindings := status.Assign(rawFindings, in.Accepted, in.Exceptions, in.Now, "gate")

	// --- Stage 6: Metrics ---
	collected := signal.CollectedSignals{
		Common: signal.CommonInput{
			Graph:           ex.g,
			Classifications: couplingIdx,
			Findings:        taggedFindings,
			Baseline:        in.BaseMetrics,
			ToolCoverage:    ex.coverages,
			ChangedFiles:    in.Scope.Changed,
		},
		History:     in.Signals.History,
		Symbol:      signal.SymbolSignals{Graph: ex.scipSymbols, GitnexusImpact: in.Signals.GitnexusImpact},
		Size:        in.Signals.Size,
		Complexity:  in.Signals.Complexity,
		Fitness:     in.Signals.Fitness,
		Duplication: in.Signals.Duplication,
	}
	metricResults := make([]diagnostic.MetricResult, 0, len(in.Metrics))
	for _, m := range in.Metrics {
		metricResults = append(metricResults, m.Calculate(collected))
	}

	// --- Stage 7: Resolve evidence — module labels + severity join ---
	ev := resolveEvidence(ex.g, couplingIdx, classifyCfg, taggedFindings)

	// --- Stage 8: Advisory findings ---
	// Collect coupling advisories by walking edges in graph order (deterministic).
	// Edges with Severity != "" and at or above the configured minimum severity are included.
	advisoryFindings := collectAdvisories(ex.g, couplingIdx, classifyCfg, staleLabelFindings, in)

	// Partition ev.resolvedFindings: rule-advisory findings (Kind="advisory", set by
	// gatedRule for gate:warn rules) go into advisoryFindings; all others go into
	// baseFindings. This prevents double-emission when mode.Advisory is on, and lets
	// computeVerdict count rule-advisories separately from coupling advisories
	// (coupling advisories must never flip the verdict).
	var baseFindings []finding.Finding
	var ruleAdvisoryFindings []finding.Finding
	for _, f := range ev.resolvedFindings {
		if f.Kind == "advisory" {
			ruleAdvisoryFindings = append(ruleAdvisoryFindings, f)
			advisoryFindings = append(advisoryFindings, f)
		} else {
			baseFindings = append(baseFindings, f)
		}
	}

	// Gate findings: kind=="gate" and not fixed.
	var gateFindings []finding.Finding
	for _, f := range baseFindings {
		if f.Kind == "gate" && f.Status != finding.StatusFixed {
			gateFindings = append(gateFindings, f)
		}
	}

	// Summary counts.
	var exceptionsUsed int
	for _, f := range baseFindings {
		if f.Status == finding.StatusExcepted {
			exceptionsUsed++
		}
	}
	gateNew := 0
	for _, f := range gateFindings {
		if f.Status == finding.StatusNew || f.Status == finding.StatusExpiredExcept {
			gateNew++
		}
	}

	// Pass only rule-advisory count to computeVerdict — coupling advisories must
	// not flip verdict regardless of mode.Advisory.
	verdict := computeVerdict(gateFindings, metricResults, countActive(ruleAdvisoryFindings))

	// --- Stage 9: Assemble Diagnostic ---
	// Include advisory findings in Findings only when mode.Advisory is set.
	// Advisory findings never affect gate counts.
	// Active rule-advisory findings (gate: warn) contribute to VerdictWarn
	// even when mode.Advisory is off — warn-rules are intentional non-blocking signals.
	resolvedFindings := baseFindings
	warnings := 0
	if in.Mode.Advisory {
		resolvedFindings = append(resolvedFindings, advisoryFindings...)
		warnings = countActive(advisoryFindings)
	}

	// Ensure non-nil slices.
	if resolvedFindings == nil {
		resolvedFindings = []finding.Finding{}
	}
	if metricResults == nil {
		metricResults = []diagnostic.MetricResult{}
	}
	if ex.coverages == nil {
		ex.coverages = []diagnostic.Coverage{}
	}

	// Neutral structural-facts block (Tranche 1.5): assembled from the symbol
	// graph + change history, attached as report-only evidence. Never read by
	// computeVerdict or any gate logic. Empty when SCIP is off/absent.
	fileFacts := facts.Build(ex.scipSymbols, in.Signals.Size.FileLOC, in.Signals.History.CoChange, in.Signals.GitnexusImpact)

	// Dynamic/lazy-import risk (Task 9): report-only evidence rolled up per module.
	// Dynamic imports are invisible to the static graph, so they hide cycles and
	// undercount coupling. Never read by computeVerdict or any gate, and never
	// alters ex.g or any metric — the sites are scanned in cmd, not from the graph.
	dynamicImports := buildDynamicImports(in.Signals.DynamicImports.Sites, classifyCfg.ModuleMap)

	// Delta bucketing (Task 3c): in delta mode, group findings by how they relate
	// to the baseline and the changed-file set so the report does not read like a
	// full-repo dump. Report-only; never enters the verdict. Nil outside delta mode.
	delta := deltaReport(in.Scope.Mode, resolvedFindings, in.Accepted, in.Scope.Changed)

	classifiedEdges := buildClassifiedEdgeSummary(couplingIdx)
	classifiedEdges.LLMApproved = llmApprovedCount

	d := diagnostic.Diagnostic{
		SchemaVersion:         diagnostic.SchemaVersion,
		Verdict:               verdict,
		Base:                  in.Mode.Base,
		Head:                  in.Mode.Head,
		ConfigHash:            in.ConfigHash,
		PrimaryExtractorTools: in.PrimaryExtractorTools,
		Metrics:               metricResults,
		Findings:              resolvedFindings,
		SyntaxFacts:           syntaxFacts,
		FileFacts:             fileFacts,
		DynamicImports:        dynamicImports,
		RuntimeAsync:          runtimeAsync,
		AgentTasks:            []diagnostic.AgentTask{},
		ToolCoverage:          ex.coverages,
		ClassifiedEdges:       classifiedEdges,
		Delta:                 delta,
		Summary: diagnostic.Summary{
			GateFindings:   gateNew,
			Warnings:       warnings,
			ExceptionsUsed: exceptionsUsed,
		},
	}

	return d, nil
}

// extract runs stage 1: symbol resolution, extractor loop, graph build.
// Returns the import graph, all coverage records, and the SCIP symbol graph.
func extract(ctx context.Context, in RunInput) (extractResult, error) {
	var coverages []diagnostic.Coverage

	// Symbol-level integration strength (SCIP), keyed by "fromPath\x00toPath".
	// Best-effort: an empty map when no indexer is available leaves edges to the
	// config-glob and extractor-hint strength classification.
	scipStrength, scipCov, _ := in.Resolver.Strengths(ctx, in.Scope)
	coverages = append(coverages, scipCov)

	// Symbol graph (SCIP) — per-symbol ownership, fan-in, and cross-module refs.
	// Empty when SCIP is off/absent; metrics that need it report n/a in that case.
	scipSymbols, scipSymCov, _ := in.Resolver.Symbols(ctx, in.Scope)
	coverages = append(coverages, scipSymCov)

	var allFacts []graph.Facts
	for _, ex := range in.Extractors {
		f, cov, err := ex.Extract(ctx, in.Scope)
		if err != nil {
			return extractResult{}, err
		}
		enrichEdges(ctx, in.Resolver, scipStrength, f)
		allFacts = append(allFacts, f)
		coverages = append(coverages, cov)
	}
	g := graph.Build(allFacts)

	// Append opt-in tool coverage (clones, gitnexus) collected in cmd rather than
	// through the extractor loop. These have no path into the diagnostic otherwise.
	coverages = append(coverages, in.Signals.ExtraCoverage...)

	return extractResult{g: g, coverages: coverages, scipSymbols: scipSymbols}, nil
}

// resolveEvidence runs stage 7: join module labels and severity onto tagged findings.
func resolveEvidence(
	g *graph.Graph,
	couplingIdx coupling.Index,
	classifyCfg config.ClassifyConfig,
	taggedFindings []finding.Finding,
) evidenceResult {
	// Build a path-pair → coupling.Classification lookup so we can join
	// severity and module labels onto findings without re-importing coupling keys.
	type pathPair struct{ from, to, kind string }
	type classEntry struct {
		strength string
		distance string
	}
	pairClass := make(map[pathPair]classEntry, len(g.Edges()))
	for _, e := range g.Edges() {
		key := e.From + "\x00" + e.To + "\x00" + string(e.Kind)
		cl, ok := couplingIdx[key]
		if !ok {
			continue
		}
		epair := pathPair{
			from: stripPrefix(e.From),
			to:   stripPrefix(e.To),
			kind: string(e.Kind),
		}
		pairClass[epair] = classEntry{
			strength: string(cl.Strength),
			distance: string(cl.Distance),
		}
	}

	mm := classifyCfg.ModuleMap
	resolvedFindings := make([]finding.Finding, 0, len(taggedFindings))
	for _, f := range taggedFindings {
		// Resolve module labels from ModuleMap.
		fromModule, _ := mm.ModuleFor(f.Edge.From.Path)
		toModule, _ := mm.ModuleFor(f.Edge.To.Path)
		f.Edge.From.Module = fromModule
		f.Edge.To.Module = toModule

		// Join severity from coupling classification.
		epair := pathPair{from: f.Edge.From.Path, to: f.Edge.To.Path, kind: f.Edge.Kind}
		if ce, ok := pairClass[epair]; ok {
			f.Severity = severityFor(ce.strength, ce.distance)
		}

		resolvedFindings = append(resolvedFindings, f)
	}

	return evidenceResult{resolvedFindings: resolvedFindings}
}

// computeVerdict derives the overall verdict from gate findings, metric results,
// and active rule-advisory findings (gate: warn).
//   - Any gate finding with status new or expired_exception → fail
//   - Any metric with delta != nil && *delta < 0 → warn (if not already fail)
//   - Any active rule-advisory finding (activeRuleAdvisories > 0) → warn (if not already fail)
//   - Otherwise → pass
//
// Coupling advisories are intentionally excluded from activeRuleAdvisories — they
// must not flip the verdict.
func computeVerdict(gateFindings []finding.Finding, ms []diagnostic.MetricResult, activeRuleAdvisories int) diagnostic.Verdict {
	for _, f := range gateFindings {
		if f.Status == finding.StatusNew || f.Status == finding.StatusExpiredExcept {
			return diagnostic.VerdictFail
		}
	}
	for _, m := range ms {
		if m.Delta != nil && *m.Delta < 0 {
			return diagnostic.VerdictWarn
		}
	}
	if activeRuleAdvisories > 0 {
		return diagnostic.VerdictWarn
	}
	return diagnostic.VerdictPass
}

// enrichEdges applies symbol resolution and SCIP integration strength to an
// extractor's edges in place (element writes go through the slice's shared
// backing array, so they are visible to the caller).
// Resolution rewrites barrel-file targets to real paths; SCIP strength sets a
// per-edge StrengthHint (config public/internal globs still win in classify).
func enrichEdges(ctx context.Context, sr ports.SymbolResolver, scipStrength map[string]string, facts graph.Facts) {
	for i, e := range facts.Edges {
		fromFile := stripPrefix(e.From)
		toPath := stripPrefix(e.To)
		realPath, _ := sr.Resolve(ctx, fromFile, toPath)
		if realPath != toPath {
			prefix := e.To[:len(e.To)-len(toPath)]
			facts.Edges[i].To = prefix + realPath
		}
		if st, found := scipStrength[fromFile+"\x00"+toPath]; found {
			facts.Edges[i].StrengthHint = st
		}
	}
}
