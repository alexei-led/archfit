package engine

import (
	"context"
	"errors"
	"fmt"
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
	Waivers     config.WaiverSet
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
	// MetricGates maps metric name → its metrics.<name> config entry (gate
	// posture off|warn|fail plus max_new/min_delta thresholds); the caller
	// passes cfg.Metrics. nil/missing entries mean the defaults: blocking
	// gate, zero tolerated regression.
	MetricGates map[string]config.MetricConfig
	Labels      []labels.Label // pinned coupling labels; nil = none
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

// AugmentClassifyConfig returns cfg with the same synthetic-module augmentation
// and ModuleMap rebuild that Run applies before label freshness, classification,
// advisories, and diagnostics.
func AugmentClassifyConfig(g *graph.Graph, cfg config.ClassifyConfig) config.ClassifyConfig {
	// Register auto-discovered module-graph nodes (Rust "<crate>::<mod>") as modules so
	// classify can resolve their distance/volatility; otherwise their edges are
	// distance-unknown and coupling_balance/encapsulation never see them. No-op for
	// Go/TS/Python (their nodes are already configured; the "::" gate excludes them).
	cfg.Modules = classify.AugmentModulesFromGraph(g, cfg.Modules)
	// Register Go workspace members (≥2-member gate) as synthetic modules so
	// cross-member edges classify with a real Distance for coupling_balance. No-op for
	// single-module repos and archfit's own self-scan (1 surviving member after exclusion).
	cfg.Modules = classify.AugmentGoWorkspaceModules(g, cfg.Modules)
	// Bind crate-level Rust nodes (bare `package:<crate>` names) to the module whose
	// path glob covers the crate's directory, so multi-crate workspaces configured with
	// "crates/<crate>/**" globs measure coupling instead of classifying every cross-crate
	// edge as external. No-op for bare-name configs (tokio/yazi) and single-crate repos.
	cfg.Modules = classify.AugmentCargoCrateNodes(g, cfg.Modules)

	// Rebuild the ModuleMap from the augmented Modules slice so that all secondary
	// consumers see auto-registered members. The Augment* calls above mutate
	// cfg.Modules but NOT cfg.ModuleMap, which was built at config-view construction
	// time.
	cfg.ModuleMap = config.BuildModuleMap(cfg.Modules)
	return cfg
}

// extractResult holds the outputs of the extract stage.
type extractResult struct {
	g                       *graph.Graph
	coverages               []diagnostic.Coverage
	scipSymbols             symbol.Graph
	semanticStrengthOverlay *diagnostic.SemanticStrengthOverlay
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
	// Collect syntactic declarations and route registrations (ast-grep rules)
	// before the rules stage. Off-gate: facts populate the report (consumed by
	// public_api_*) but never affect the verdict.
	var syntaxFacts []diagnostic.SyntaxFact
	if in.SyntaxCfg.Enabled {
		if in.Syntax == nil {
			return diagnostic.New(), errors.New("engine: SyntaxCfg.Enabled=true but no Syntax provider")
		}
		sf, synCov, synErr := in.Syntax.Syntax(ctx, in.Scope, in.SyntaxCfg.Languages)
		if synErr != nil {
			return diagnostic.New(), synErr
		}
		syntaxFacts = sf
		// Backfill Module from ModuleMap — uniform across all languages,
		// no per-language branching. Files outside declared modules get an
		// empty Module (legitimate: a script in the repo root, for example).
		for i := range syntaxFacts {
			syntaxFacts[i].Module, _ = in.Classify.ModuleMap.ModuleForFile(syntaxFacts[i].File)
		}
		ex.coverages = append(ex.coverages, synCov)
	}

	// --- Stage 3: Classify ---
	classifyCfg := AugmentClassifyConfig(ex.g, in.Classify)

	// Pinned coupling labels refine strength classification (human/tool: config
	// globs > approved labels > extractor hint; llm: fills only cells all static
	// sources left unknown). Freshness must use the augmented ModuleMap above, or
	// labels for synthetic Go/Rust module pairs bypass the stale-evidence check.
	staleLabelFindings, llmApprovedCount := applyPinnedLabels(ex.g, &classifyCfg, in.Mode, in.Labels)

	// Thread clone pairs for the clone-driven classify paths: Symmetric-strength
	// upgrade, volatility-cascade exclusion, duplicated-knowledge pairing.
	// Placed after the ModuleMap rebuild so auto-registered members participate
	// in cross-module clone-pair detection.
	if len(in.Signals.Duplication.Clusters) > 0 {
		classifyCfg.CrossModuleClonePairs, classifyCfg.CloneEvidence = buildClonePairSet(in.Signals.Duplication.Clusters, classifyCfg.ModuleMap, in.Signals.Size.FileClassIndex)
	}

	// Runtime async evidence: build per-module and relationship-level rollups for
	// the diagnostic. Report-only — never changes the graph, score, or verdict.
	runtimeAsync := buildRuntimeAsync(in.Signals.RuntimeAsync.Sites, in.Signals.RuntimeAsync.Confidence, classifyCfg.ModuleMap)
	runtimeAsyncEdges := buildRuntimeAsyncEdges(in.Signals.RuntimeAsync.Sites, in.Signals.RuntimeAsync.Confidence, classifyCfg.ModuleMap)

	couplingIdx := classify.Run(ex.g, classifyCfg)
	cloneOnlyPairs := classify.CloneOnlyPairs(ex.g, classifyCfg)

	// --- Stage 4: Rules ---
	// Call each rule once with the full evidence set. Rules iterate edges internally;
	// the Evidence carries all pattern matches so each rule can filter by edge's from-file.
	var rawFindings []finding.Finding
	allPatternMatches := rules.Evidence{PatternMatches: patternMatches, SyntaxFacts: syntaxFacts}
	for _, r := range in.Rules {
		rawFindings = append(rawFindings, r.Check(ex.g, allPatternMatches)...)
	}

	// --- Stage 5: Status ---
	taggedFindings := status.Assign(rawFindings, in.Accepted, in.Waivers, in.Now, finding.KindGate)

	// --- Stage 6: Metrics ---
	collected := signal.CollectedSignals{
		Common: signal.CommonInput{
			Graph:           ex.g,
			Classifications: couplingIdx,
			Findings:        taggedFindings,
			Baseline:        in.BaseMetrics,
			ToolCoverage:    ex.coverages,
			ChangedFiles:    in.Scope.Changed,
			SyntaxFacts:     syntaxFacts,
			DeprecatedDeps:  in.Signals.DeprecatedDeps,
		},
		Symbol:      signal.SymbolSignals{Graph: ex.scipSymbols},
		Size:        in.Signals.Size,
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
		if f.Kind == finding.KindAdvisory {
			ruleAdvisoryFindings = append(ruleAdvisoryFindings, f)
			advisoryFindings = append(advisoryFindings, f)
		} else {
			baseFindings = append(baseFindings, f)
		}
	}

	// Gate findings: kind=="gate" and not fixed.
	var gateFindings []finding.Finding
	for _, f := range baseFindings {
		if f.Kind == finding.KindGate && f.Status != finding.StatusFixed {
			gateFindings = append(gateFindings, f)
		}
	}

	// Summary counts.
	var waiversUsed int
	for _, f := range baseFindings {
		if f.Status == finding.StatusWaived {
			waiversUsed++
		}
	}
	gateNew := 0
	for _, f := range gateFindings {
		if f.Status == finding.StatusNew || f.Status == finding.StatusExpiredWaiver {
			gateNew++
		}
	}

	// Pass only rule-advisory count to computeVerdict — coupling advisories must
	// not flip verdict regardless of mode.Advisory.
	verdict := computeVerdict(gateFindings, metricResults, in.MetricGates, countActive(ruleAdvisoryFindings))

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
	// graph and file LOC, attached as report-only evidence. Never read by
	// computeVerdict or any gate logic. Empty when SCIP is off/absent.
	fileFacts := facts.Build(ex.scipSymbols, in.Signals.Size.FileLOC)

	// Dynamic/lazy-import risk (Task 9): report-only evidence rolled up per module.
	// Dynamic imports are invisible to the static graph, so they hide cycles and
	// undercount coupling. Never read by computeVerdict or any gate, and never
	// alters ex.g or any metric — the sites are scanned in cmd, not from the graph.
	dynamicImports := buildDynamicImports(in.Signals.DynamicImports.Sites, classifyCfg.ModuleMap)

	// Delta bucketing (Task 3c): in delta mode, group findings by how they relate
	// to the baseline and the changed-file set so the report does not read like a
	// full-repo dump. Report-only; never enters the verdict. Nil outside delta mode.
	delta := deltaReport(in.Scope.Mode, resolvedFindings, in.Accepted, in.Scope.Changed)

	classifiedEdges := buildClassifiedEdgeSummaryForRun(couplingIdx, cloneOnlyPairs, classifyCfg.DuplicatedKnowledgePolicy, classifyCfg.ModuleMap)
	classifiedEdges.LLMApproved = llmApprovedCount
	connascenceReport := buildConnascenceReport(couplingIdx)
	dynamicConnascenceSignals := buildDynamicConnascenceSignals(dynamicImports, runtimeAsyncEdges, connascenceReport.Unmeasured)
	// Volatility triage disclosure: count modules by volatility source (declared /
	// inherited / cascade / undeclared) so coupling_balance can say whether a
	// uniform-volatility repo is measured or uniform-by-inheritance. in.Classify
	// still holds the PRE-augmentation module map (Augment* are copy-on-write).
	classifiedEdges.VolatilityProvenance = classify.ComputeVolatilityProvenance(ex.g, in.Classify.Modules, classifyCfg)

	// Local complexity (book Ch10): per-module rollup of scored same-module
	// edges. Report-only — never enters coupling_balance or the verdict.
	localCoupling := buildLocalCoupling(ex.g, couplingIdx, classifyCfg.ModuleMap)

	d := diagnostic.Diagnostic{
		SchemaVersion:             diagnostic.SchemaVersion,
		Verdict:                   verdict,
		Base:                      in.Mode.Base,
		Head:                      in.Mode.Head,
		ConfigHash:                in.ConfigHash,
		PrimaryExtractorTools:     in.PrimaryExtractorTools,
		Metrics:                   metricResults,
		Findings:                  resolvedFindings,
		SyntaxFacts:               syntaxFacts,
		FileFacts:                 fileFacts,
		DynamicImports:            dynamicImports,
		Connascence:               connascenceReport,
		DynamicConnascenceSignals: dynamicConnascenceSignals,
		RuntimeAsync:              runtimeAsync,
		RuntimeAsyncEdges:         runtimeAsyncEdges,
		DeprecatedDeps:            in.Signals.DeprecatedDeps,
		SemanticStrengthOverlay:   ex.semanticStrengthOverlay,
		AgentTasks:                []diagnostic.AgentTask{},
		ToolCoverage:              ex.coverages,
		ClassifiedEdges:           classifiedEdges,
		LocalCoupling:             localCoupling,
		Delta:                     delta,
		Summary: diagnostic.Summary{
			GateFindings: gateNew,
			Warnings:     warnings,
			WaiversUsed:  waiversUsed,
		},
	}

	return d, nil
}

type connascenceResolver interface {
	Connascence(context.Context, scope.Scope) (map[string][]graph.ConnascenceHint, diagnostic.Coverage, error)
}

// extract runs stage 1: symbol resolution, extractor loop, graph build.
// Returns the import graph, all coverage records, and the SCIP symbol graph.
func extract(ctx context.Context, in RunInput) (extractResult, error) {
	var coverages []diagnostic.Coverage

	// Symbol-level integration strength (SCIP), keyed by "fromPath\x00toPath".
	// Best-effort: an empty map when no indexer is available leaves edges to the
	// config-glob and extractor-hint strength classification. A zero-Tool coverage
	// (from NopSymbolResolver when SCIP is disabled) is dropped — the pipeline
	// appends an explicit StatusDisabled row via ExtraCoverage instead.
	scipStrength, scipCov, _ := in.Resolver.Strengths(ctx, in.Scope)
	if scipCov.Tool != "" {
		coverages = append(coverages, scipCov)
	}

	// Symbol-level connascence evidence (SCIP), keyed by "from\x00to". Best-effort;
	// empty when no resolver exposes deterministic connascence facts.
	var scipConnascence map[string][]graph.ConnascenceHint
	if cr, ok := in.Resolver.(connascenceResolver); ok {
		scipConnascence, _, _ = cr.Connascence(ctx, in.Scope)
	}

	// Symbol graph (SCIP) — per-symbol ownership, fan-in, and cross-module refs.
	// Empty when SCIP is off/absent; metrics that need it report n/a in that case.
	scipSymbols, scipSymCov, _ := in.Resolver.Symbols(ctx, in.Scope)
	if scipSymCov.Tool != "" {
		coverages = append(coverages, scipSymCov)
	}

	var allFacts []graph.Facts
	var extractErrs []error
	overlay := newSemanticStrengthOverlay()
	for _, ex := range in.Extractors {
		f, cov, err := ex.Extract(ctx, in.Scope)
		if err != nil {
			// One extractor's failure must not discard facts already produced
			// (or still to be produced) by the others — record it as a coverage
			// gap and keep going, mirroring the empty-SCIP-index StatusPartial
			// pattern (internal/extract/scip/scip_strength.go) instead of
			// aborting the whole run. Only the degenerate case where every
			// extractor fails (nothing to preserve) is still fatal, below.
			coverages = append(coverages, diagnostic.Coverage{
				Tool:   ex.Name(),
				Status: diagnostic.StatusPartial,
				Reason: err.Error(),
			})
			extractErrs = append(extractErrs, err)
			continue
		}
		overlay.merge(enrichEdges(ctx, in.Resolver, scipStrength, scipConnascence, f))
		allFacts = append(allFacts, f)
		coverages = append(coverages, cov)
	}
	if len(in.Extractors) > 0 && len(extractErrs) == len(in.Extractors) {
		return extractResult{}, fmt.Errorf("engine: all %d extractor(s) failed: %w", len(in.Extractors), errors.Join(extractErrs...))
	}
	g := graph.Build(allFacts)

	// Append opt-in tool coverage (clones, complexity) collected in cmd rather than
	// through the extractor loop. These have no path into the diagnostic otherwise.
	coverages = append(coverages, in.Signals.ExtraCoverage...)

	return extractResult{g: g, coverages: coverages, scipSymbols: scipSymbols, semanticStrengthOverlay: overlay.report()}, nil
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
// per-metric gate config, and active rule-advisory findings (gate: warn).
//   - Any gate finding with status new or expired_waiver → fail
//   - Any metric whose delta breaches its threshold in the worsening direction
//     trips its gate: DirectionHigherIsWorse breaches on *delta > max_new,
//     everything else (including the unset zero Direction) breaches on
//     *delta < -min_delta (ratio semantics). Unset knobs default to 0 — any
//     worsening move trips. The gate posture then decides:
//     off skips the check, warn caps at warn, fail/unset fails — the same
//     convention as rule gates (unset = blocking).
//   - Any active rule-advisory finding (activeRuleAdvisories > 0) → warn (if not already fail)
//   - Otherwise → pass
//
// Coupling advisories are intentionally excluded from activeRuleAdvisories — they
// must not flip the verdict.
func computeVerdict(gateFindings []finding.Finding, ms []diagnostic.MetricResult, gates map[string]config.MetricConfig, activeRuleAdvisories int) diagnostic.Verdict {
	for _, f := range gateFindings {
		if f.Status == finding.StatusNew || f.Status == finding.StatusExpiredWaiver {
			return diagnostic.VerdictFail
		}
	}
	verdict := diagnostic.VerdictPass
	for _, m := range ms {
		if m.Delta == nil {
			continue
		}
		mc := gates[m.Name]
		if mc.Gate == string(config.GateOff) {
			continue
		}
		var minDelta float64
		if mc.MinDelta != nil {
			minDelta = *mc.MinDelta
		}
		breached := *m.Delta < -minDelta
		if m.Direction == diagnostic.DirectionHigherIsWorse {
			var maxNew int
			if mc.MaxNew != nil {
				maxNew = *mc.MaxNew
			}
			breached = *m.Delta > float64(maxNew)
		}
		if !breached {
			continue
		}
		if mc.Gate == string(config.GateWarn) {
			verdict = diagnostic.VerdictWarn
			continue
		}
		return diagnostic.VerdictFail
	}
	if verdict == diagnostic.VerdictPass && activeRuleAdvisories > 0 {
		return diagnostic.VerdictWarn
	}
	return verdict
}

// enrichEdges applies symbol resolution and SCIP integration strength to an
// extractor's edges in place (element writes go through the slice's shared
// backing array, so they are visible to the caller).
// Resolution rewrites barrel-file targets to real paths; SCIP strength sets a
// per-edge StrengthHint (config public/internal globs still win in classify).
func enrichEdges(ctx context.Context, sr ports.SymbolResolver, scipStrength map[string]string, scipConnascence map[string][]graph.ConnascenceHint, facts graph.Facts) *semanticStrengthOverlay {
	overlay := newSemanticStrengthOverlay()
	for i, e := range facts.Edges {
		fromFile := stripPrefix(e.From)
		toPath := stripPrefix(e.To)
		realPath, _ := sr.Resolve(ctx, fromFile, toPath)
		if realPath != toPath {
			prefix := e.To[:len(e.To)-len(toPath)]
			facts.Edges[i].To = prefix + realPath
		}
		// Strength: SCIP refines the edge UNLESS the Go extractor already supplied a
		// compiler-grade type-info hint. Go strength is derived from go/types — the
		// actual resolved object (interface→contract, concrete type→model,
		// func→functional) — so it is ground truth. SCIP-go is a coarser subprocess
		// re-derivation that collapses package imports to a blanket "functional"; it
		// must not overwrite the authoritative hint or coupling_balance loses its
		// strength signal. SCIP remains the strength source for TS/Py/Rust (whose
		// extractor hints are heuristics) and for Go edges type-info left unresolved.
		key := fromFile + "\x00" + toPath
		if hints := scipConnascence[key]; len(hints) > 0 {
			facts.Edges[i].ConnascenceHints = appendGraphConnascenceHints(facts.Edges[i].ConnascenceHints, hints...)
		}
		if e.Language == graph.LangGo && e.StrengthHint != "" {
			continue
		}
		trackOverlay := len(scipStrength) > 0 && isSemanticOverlayLanguage(e.Language)
		if trackOverlay {
			overlay.addCandidate(e.Language, e.StrengthHint)
		}
		st, found := scipStrength[key]
		if found {
			facts.Edges[i].StrengthHint = st
		}
		if trackOverlay {
			overlay.finishCandidate(e.Language, facts.Edges[i].StrengthHint, found)
		}
	}
	return overlay
}

func appendGraphConnascenceHints(dst []graph.ConnascenceHint, hints ...graph.ConnascenceHint) []graph.ConnascenceHint {
	seen := make(map[graph.ConnascenceHint]struct{}, len(dst)+len(hints))
	for _, h := range dst {
		seen[h] = struct{}{}
	}
	for _, h := range hints {
		if _, ok := seen[h]; ok {
			continue
		}
		seen[h] = struct{}{}
		dst = append(dst, h)
	}
	return dst
}
