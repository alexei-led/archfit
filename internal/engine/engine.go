package engine

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"time"

	"github.com/alexei-led/archfit/internal/baseline"
	"github.com/alexei-led/archfit/internal/classify"
	"github.com/alexei-led/archfit/internal/config"
	"github.com/alexei-led/archfit/internal/metrics"
	"github.com/alexei-led/archfit/internal/model/coupling"
	"github.com/alexei-led/archfit/internal/model/diagnostic"
	"github.com/alexei-led/archfit/internal/model/finding"
	"github.com/alexei-led/archfit/internal/model/graph"
	"github.com/alexei-led/archfit/internal/rules"
	"github.com/alexei-led/archfit/internal/scope"
	"github.com/alexei-led/archfit/internal/staleness"
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

// Run executes the full archfit pipeline and returns the assembled Diagnostic.
//
// Pipeline stages:
//  1. Run each extractor sequentially; apply symbol resolution to edges; merge Facts; build graph.
//  2. Run PatternProvider to gather structural matches; build per-file evidence index.
//  3. Classify edges: classify.Run → coupling.Index.
//  4. Apply rules: rule.Check(g, Evidence{PatternMatches}) per rule → raw findings (flattened).
//  5. Assign statuses: status.Assign → lifecycle-tagged findings.
//  6. Compute metrics: build MetricInput, run each metric → MetricResult slice.
//  7. Collect advisory findings: coupling edges with Severity != "" + staleness.Check.
//  8. Assemble Diagnostic: resolve EdgeEvidence {module, path}, join severity,
//     fill Summary, compute verdict. Advisory findings included only when mode.Advisory.
//  9. Return (Diagnostic, nil) on success; (Diagnostic, error) on hard error.
//
// Rendering is the caller's responsibility (cmd renders to deps.Stdout).
func Run(
	ctx context.Context,
	mode Mode,
	s scope.Scope,
	classifyCfg config.ClassifyConfig,
	stalenessCfg config.StalenessConfig,
	exceptions config.ExceptionSet,
	extractors []Extractor,
	pp PatternProvider,
	sr SymbolResolver,
	patternCfg config.PatternConfig,
	rs []rules.Rule,
	ms []metrics.Metric,
	base baseline.Baseline,
	change metrics.ChangeHistory,
	now time.Time,
) (diagnostic.Diagnostic, error) {
	// --- Stage 1: Extract ---
	// Run each extractor; apply symbol resolution and SCIP strength to edges before merging.
	var allFacts []graph.Facts
	var coverages []diagnostic.Coverage

	// Symbol-level integration strength (SCIP), keyed by "fromPath\x00toPath".
	// Best-effort: an empty map when no indexer is available leaves edges to the
	// config-glob and extractor-hint strength classification.
	scipStrength, scipCov, _ := sr.Strengths(ctx, s)
	coverages = append(coverages, scipCov)

	// Symbol graph (SCIP) — per-symbol ownership, fan-in, and cross-module refs.
	// Empty when SCIP is off/absent; metrics that need it report n/a in that case.
	scipSymbols, scipSymCov, _ := sr.Symbols(ctx, s)
	coverages = append(coverages, scipSymCov)

	for _, ex := range extractors {
		facts, cov, err := ex.Extract(ctx, s)
		if err != nil {
			return diagnostic.New(), err
		}
		enrichEdges(ctx, sr, scipStrength, facts)
		allFacts = append(allFacts, facts)
		coverages = append(coverages, cov)
	}
	g := graph.Build(allFacts)

	// --- Stage 2: Pattern matching ---
	// Gather structural matches from the PatternProvider and build a per-file evidence index.
	// Matches are keyed by file path so rules can filter by the edge's from-file.
	patternMatches, ppCov, ppErr := pp.Find(ctx, s, patternCfg)
	if ppErr != nil {
		return diagnostic.New(), ppErr
	}
	coverages = append(coverages, ppCov)
	// Convert engine.PatternMatch → rules.PatternMatch for the evidence type.
	rulesMatches := toRulesPatternMatches(patternMatches)

	// --- Stage 3: Classify ---
	couplingIdx := classify.Run(g, classifyCfg)

	// --- Stage 4: Rules ---
	// Call each rule once with the full evidence set. Rules iterate edges internally;
	// the Evidence carries all pattern matches so each rule can filter by edge's from-file.
	var rawFindings []finding.Finding
	allPatternMatches := rules.Evidence{PatternMatches: rulesMatches}
	for _, r := range rs {
		rawFindings = append(rawFindings, r.Check(g, allPatternMatches)...)
	}

	// --- Stage 5: Status ---
	taggedFindings := status.Assign(rawFindings, base, exceptions, now, "gate")

	// --- Stage 5: Metrics ---
	mi := metrics.MetricInput{
		Graph:           g,
		Classifications: couplingIdx,
		Findings:        taggedFindings,
		Baseline:        base.Metrics,
		ToolCoverage:    coverages,
		FileChurn:       change.FileChurn,
		CoChange:        change.CoChange,
		FileLOC:         change.FileLOC,
		Complexity:      change.Complexity,
		SymbolGraph:     scipSymbols,
		FitnessSignals:  change.FitnessSignals,
		CloneClusters:   change.CloneClusters,
	}
	metricResults := make([]diagnostic.MetricResult, 0, len(ms))
	for _, m := range ms {
		metricResults = append(metricResults, m.Calculate(mi))
	}

	// --- Stage 7: Assemble Diagnostic ---

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

	// --- Stage 6: Advisory findings ---
	// Collect coupling advisories by walking edges in graph order (deterministic).
	// Edges with Severity != "" and at or above the configured minimum severity are included.
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
			Kind:     "advisory",
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
	advisoryFindings = append(advisoryFindings, staleness.Check(g, stalenessCfg, now)...)

	// Apply baseline and exception status to advisory findings.
	// status.Assign also emits fixed gate findings; suppress those for the advisory pass
	// by discarding any synthetic kind=="gate" entries it appends.
	tagged := status.Assign(advisoryFindings, base, exceptions, now, "advisory")
	advisoryFindings = advisoryFindings[:0]
	for _, f := range tagged {
		if f.Kind == "advisory" {
			advisoryFindings = append(advisoryFindings, f)
		}
	}

	// Gate findings: kind=="gate" and not already resolved (fixed findings don't block verdict or inflate count).
	var gateFindings []finding.Finding
	for _, f := range resolvedFindings {
		if f.Kind == "gate" && f.Status != finding.StatusFixed {
			gateFindings = append(gateFindings, f)
		}
	}

	// Summary counts.
	var exceptionsUsed int
	for _, f := range resolvedFindings {
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

	// Include advisory findings in Findings only when mode.Advisory is set.
	// Advisory findings never affect the verdict or gate counts.
	warnings := 0
	if mode.Advisory {
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
	if coverages == nil {
		coverages = []diagnostic.Coverage{}
	}

	d := diagnostic.Diagnostic{
		SchemaVersion: diagnostic.SchemaVersion,
		Verdict:       verdict,
		Base:          mode.Base,
		Head:          mode.Head,
		Metrics:       metricResults,
		Findings:      resolvedFindings,
		AgentTasks:    []diagnostic.AgentTask{},
		ToolCoverage:  coverages,
		Summary: diagnostic.Summary{
			GateFindings:   gateNew,
			Warnings:       warnings,
			ExceptionsUsed: exceptionsUsed,
		},
	}

	return d, nil
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
// extractor's edges in place (facts.Edges is a slice header, so mutations persist).
// Resolution rewrites barrel-file targets to real paths; SCIP strength sets a
// per-edge StrengthHint (config public/internal globs still win in classify).
func enrichEdges(ctx context.Context, sr SymbolResolver, scipStrength map[string]string, facts graph.Facts) {
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

// toRulesPatternMatches converts engine.PatternMatch values to rules.PatternMatch values.
// The rules package defines its own PatternMatch type to avoid an import cycle
// (rules cannot import engine). The conversion maps the common fields.
func toRulesPatternMatches(ms []PatternMatch) []rules.PatternMatch {
	if len(ms) == 0 {
		return nil
	}
	out := make([]rules.PatternMatch, len(ms))
	for i, m := range ms {
		out[i] = rules.PatternMatch{
			File:  m.File,
			Line:  m.Line,
			Match: m.Text,
		}
	}
	return out
}
