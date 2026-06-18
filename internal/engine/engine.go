package engine

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"time"

	"github.com/alexei-led/archfit/internal/classify"
	"github.com/alexei-led/archfit/internal/config"
	"github.com/alexei-led/archfit/internal/facts"
	"github.com/alexei-led/archfit/internal/labels"
	"github.com/alexei-led/archfit/internal/metrics"
	"github.com/alexei-led/archfit/internal/model/clone"
	"github.com/alexei-led/archfit/internal/model/coupling"
	"github.com/alexei-led/archfit/internal/model/diagnostic"
	"github.com/alexei-led/archfit/internal/model/finding"
	"github.com/alexei-led/archfit/internal/model/graph"
	"github.com/alexei-led/archfit/internal/model/signal"
	"github.com/alexei-led/archfit/internal/model/symbol"
	"github.com/alexei-led/archfit/internal/ports"
	"github.com/alexei-led/archfit/internal/rules"
	"github.com/alexei-led/archfit/internal/scope"
	"github.com/alexei-led/archfit/internal/staleness"
	"github.com/alexei-led/archfit/internal/status"
)

// kindAdvisory is the finding kind for non-gating advisory findings.
const kindAdvisory = "advisory"

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
	Rules       []rules.Rule
	Metrics     []metrics.Metric
	Accepted    status.AcceptedSet
	BaseMetrics diagnostic.MetricSnapshot // baseline metric snapshot; nil = no baseline
	Labels      []labels.Label            // pinned coupling labels; nil = none
	Signals     signal.RunSignals
	Now         time.Time
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

	// --- Stage 3: Classify ---
	// Pinned coupling labels first: approved entries refine strength
	// classification (precedence: config globs > approved labels > extractor
	// hint); stale ones surface as labels/stale advisories.
	classifyCfg := in.Classify
	staleLabelFindings := applyPinnedLabels(ex.g, &classifyCfg, in.Mode, in.Labels)
	// Thread clone pairs for CoA (connascence of algorithm) tagging — report-only.
	if len(in.Signals.Duplication.Clusters) > 0 {
		classifyCfg.CrossModuleClonePairs = buildClonePairSet(in.Signals.Duplication.Clusters, classifyCfg.ModuleMap)
	}

	couplingIdx := classify.Run(ex.g, classifyCfg)

	// --- Stage 4: Rules ---
	// Call each rule once with the full evidence set. Rules iterate edges internally;
	// the Evidence carries all pattern matches so each rule can filter by edge's from-file.
	var rawFindings []finding.Finding
	allPatternMatches := rules.Evidence{PatternMatches: patternMatches}
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

	// Gate findings: kind=="gate" and not already resolved (fixed findings don't block verdict or inflate count).
	var gateFindings []finding.Finding
	for _, f := range ev.resolvedFindings {
		if f.Kind == "gate" && f.Status != finding.StatusFixed {
			gateFindings = append(gateFindings, f)
		}
	}

	// Summary counts.
	var exceptionsUsed int
	for _, f := range ev.resolvedFindings {
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

	verdict := computeVerdict(gateFindings, metricResults)

	// --- Stage 9: Assemble Diagnostic ---
	// Include advisory findings in Findings only when mode.Advisory is set.
	// Advisory findings never affect the verdict or gate counts.
	resolvedFindings := ev.resolvedFindings
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

	d := diagnostic.Diagnostic{
		SchemaVersion: diagnostic.SchemaVersion,
		Verdict:       verdict,
		Base:          in.Mode.Base,
		Head:          in.Mode.Head,
		Metrics:       metricResults,
		Findings:      resolvedFindings,
		FileFacts:     fileFacts,
		AgentTasks:    []diagnostic.AgentTask{},
		ToolCoverage:  ex.coverages,
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

// collectAdvisories runs stage 8: coupling advisories, staleness advisories,
// stale label advisories, and the advisory status pass.
// staleLabelFnds is the slice produced by stage 3 (applyPinnedLabels).
func collectAdvisories(g *graph.Graph, couplingIdx coupling.Index, classifyCfg config.ClassifyConfig, staleLabelFnds []finding.Finding, in RunInput) []finding.Finding {
	mm := classifyCfg.ModuleMap

	var advisoryFindings []finding.Finding
	for _, e := range g.Edges() {
		key := e.From + "\x00" + e.To + "\x00" + string(e.Kind)
		cl, ok := couplingIdx[key]
		if !ok || cl.Severity == coupling.SeverityNone {
			continue
		}
		if !severityAtLeast(cl.Severity, classifyCfg.BCAdvisoryMinSeverity) {
			continue
		}
		fromPath := stripPrefix(e.From)
		toPath := stripPrefix(e.To)
		fromModule, _ := mm.ModuleFor(fromPath)
		toModule, _ := mm.ModuleFor(toPath)
		af := finding.Finding{
			ID:       couplingAdvisoryID(fromPath, toPath, string(e.Kind)),
			Kind:     kindAdvisory,
			RuleID:   "bc/imbalanced_coupling",
			Status:   finding.StatusNew,
			Severity: finding.Severity(cl.Severity),
			Edge: finding.EdgeEvidence{
				From: finding.Endpoint{Module: fromModule, Path: fromPath},
				To:   finding.Endpoint{Module: toModule, Path: toPath},
				Kind: string(e.Kind),
			},
			Locations: e.Locations,
			Why:       "balanced coupling violation: " + string(cl.Severity) + " severity",
			MatchedBy: map[string]string{
				"strength": string(cl.Strength),
				"distance": string(cl.Distance),
			},
		}
		advisoryFindings = append(advisoryFindings, af)
	}
	// Append staleness advisories.
	advisoryFindings = append(advisoryFindings, staleness.Check(g, in.Staleness, in.Now)...)

	// Stale pinned labels were ignored during classification; advise re-enrich.
	advisoryFindings = append(advisoryFindings, staleLabelFnds...)

	// Apply baseline and exception status to advisory findings.
	// status.Assign also emits fixed gate findings; suppress those for the advisory pass
	// by discarding any synthetic kind=="gate" entries it appends.
	tagged := status.Assign(advisoryFindings, in.Accepted, in.Exceptions, in.Now, "advisory")
	advisoryFindings = advisoryFindings[:0]
	for _, f := range tagged {
		if f.Kind == kindAdvisory {
			advisoryFindings = append(advisoryFindings, f)
		}
	}
	return advisoryFindings
}

// computeVerdict derives the overall verdict from gate findings and metric results.
//   - Any gate finding with status new or expired_exception → fail
//   - Any metric with delta != nil && *delta < 0 → warn (if not already fail)
//   - Otherwise → pass
func computeVerdict(gateFindings []finding.Finding, ms []diagnostic.MetricResult) diagnostic.Verdict {
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
	return diagnostic.VerdictPass
}

// severityFor maps coupling classification to finding severity.
// Intrusive coupling that crosses module boundaries → high; otherwise medium.
func severityFor(strength, distance string) finding.Severity {
	if strength == "intrusive" &&
		distance != "same_module" &&
		distance != "unknown" &&
		distance != "" {
		return finding.SeverityHigh
	}
	return finding.SeverityMedium
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

// stripPrefix removes the "kind:" prefix from a node ID (e.g. "file:pkg/a" → "pkg/a").
func stripPrefix(id string) string {
	for i := 0; i < len(id); i++ {
		if id[i] == ':' {
			return id[i+1:]
		}
	}
	return id
}

// countActive returns the number of findings whose status is not fixed.
func countActive(findings []finding.Finding) int {
	n := 0
	for _, f := range findings {
		if f.Status != finding.StatusFixed {
			n++
		}
	}
	return n
}

// severityAtLeast reports whether got meets or exceeds the threshold.
// Empty threshold means no filter (all severities pass). Order: low < medium < high < critical.
func severityAtLeast(got coupling.Severity, threshold string) bool {
	if threshold == "" {
		return true
	}
	rank := map[string]int{"low": 1, "medium": 2, "high": 3, "critical": 4}
	return rank[string(got)] >= rank[threshold]
}

// couplingAdvisoryID returns a stable 32-character hex fingerprint for a coupling advisory
// finding, derived from (from, to, kind) — same scheme as finding.fingerprint.
func couplingAdvisoryID(from, to, kind string) string {
	h := sha256.Sum256([]byte("bc/imbalanced_coupling\x00" + from + "\x00" + to + "\x00" + kind))
	return hex.EncodeToString(h[:16])
}

// staleLabelID returns a stable fingerprint for a labels/stale advisory.
func staleLabelID(from, to string) string {
	h := sha256.Sum256([]byte("labels/stale\x00" + from + "\x00" + to))
	return hex.EncodeToString(h[:16])
}

// applyPinnedLabels validates pinned labels and injects the approved ones into
// the classify config (precedence: config globs > approved labels > extractor
// hint). Freshness is checked against the full import graph — on full runs
// only (a delta graph is partial and would false-stale every label). Returns
// one labels/stale advisory per ignored stale label. Deterministic — the gate
// never calls an LLM; labels are reviewed YAML.
func applyPinnedLabels(g *graph.Graph, classifyCfg *config.ClassifyConfig, mode Mode, lbls []labels.Label) []finding.Finding {
	var evidence map[string]string
	if mode.Full || mode.Base == "" {
		wanted := make(map[string]struct{}, len(lbls))
		for _, l := range lbls {
			wanted[labels.Key(l.From, l.To)] = struct{}{}
		}
		evidence = PairEvidence(g, classifyCfg.ModuleMap, wanted)
	}
	approved, stale := labels.Approved(lbls, evidence)
	classifyCfg.ApprovedLabels = approved

	out := make([]finding.Finding, 0, len(stale))
	for _, sl := range stale {
		out = append(out, finding.Finding{
			ID:       staleLabelID(sl.From, sl.To),
			Kind:     kindAdvisory,
			RuleID:   "labels/stale",
			Status:   finding.StatusNew,
			Severity: finding.SeverityLow,
			Edge: finding.EdgeEvidence{
				From: finding.Endpoint{Module: sl.From},
				To:   finding.Endpoint{Module: sl.To},
			},
			Why: "pinned label evidence is stale: the " + sl.From + " -> " + sl.To +
				" dependency surface changed since approval; label ignored — re-run `archfit enrich` and re-review",
		})
	}
	return out
}

// PairEvidence computes the current evidence hash per module pair (keyed by
// labels.Key): HashItems over "fromPath\x00toPath\x00kind" for every
// import-graph edge whose endpoints resolve to that ordered pair. Only pairs
// in wanted are hashed (pairs of interest are few; the graph can be large).
//
// Exported because enrich (cmd) must stamp drafts with EXACTLY the hash the
// engine will later verify — one computation, two callers.
func PairEvidence(g *graph.Graph, mm config.ModuleMap, wanted map[string]struct{}) map[string]string {
	if len(wanted) == 0 {
		return nil
	}

	items := map[string][]string{}
	for _, e := range g.Edges() {
		fromPath := stripPrefix(e.From)
		toPath := stripPrefix(e.To)
		fromMod, okF := mm.ModuleFor(fromPath)
		toMod, okT := mm.ModuleFor(toPath)
		if !okF || !okT || fromMod == toMod {
			continue
		}
		key := labels.Key(fromMod, toMod)
		if _, ok := wanted[key]; !ok {
			continue
		}
		items[key] = append(items[key], fromPath+"\x00"+toPath+"\x00"+string(e.Kind))
	}

	evidence := make(map[string]string, len(items))
	for key, its := range items {
		evidence[key] = labels.HashItems(its)
	}
	return evidence
}

// buildClonePairSet converts clone clusters to a canonical module-pair key set
// for CoA (connascence of algorithm) tagging in classify.
// Keys are "[a]\x00[b]" with a≤b (canonical sorted pair, from clone.ModulePairs).
func buildClonePairSet(clusters []clone.Cluster, mm config.ModuleMap) map[string]struct{} {
	pairs := clone.ModulePairs(clusters, func(f string) string {
		mod, ok := mm.ModuleFor(f)
		if !ok {
			return ""
		}
		return mod
	})
	set := make(map[string]struct{}, len(pairs))
	for _, p := range pairs {
		// clone.ModulePairs already returns sorted pairs [a,b] with a≤b.
		set[p[0]+"\x00"+p[1]] = struct{}{}
	}
	return set
}
