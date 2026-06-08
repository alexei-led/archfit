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
//  1. Run each extractor sequentially; merge Facts and coverage records; build graph.
//  2. Classify edges: classify.Run → coupling.Index.
//  3. Apply rules: rule.Check per rule → raw findings (flattened).
//  4. Assign statuses: status.Assign → lifecycle-tagged findings.
//  5. Compute metrics: build MetricInput, run each metric → MetricResult slice.
//  6. Collect advisory findings: coupling edges with Severity != "" + staleness.Check.
//  7. Assemble Diagnostic: resolve EdgeEvidence {module, path}, join severity,
//     fill Summary, compute verdict. Advisory findings included only when mode.Advisory.
//  8. Return (Diagnostic, nil) on success; (Diagnostic, error) on hard error.
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
	rs []rules.Rule,
	ms []metrics.Metric,
	base baseline.Baseline,
	now time.Time,
) (diagnostic.Diagnostic, error) {
	_ = pp
	_ = sr
	// --- Stage 1: Extract ---
	var allFacts []graph.Facts
	var coverages []diagnostic.Coverage
	for _, ex := range extractors {
		facts, cov, err := ex.Extract(ctx, s)
		if err != nil {
			return diagnostic.New(), err
		}
		allFacts = append(allFacts, facts)
		coverages = append(coverages, cov)
	}
	g := graph.Build(allFacts)

	// --- Stage 2: Classify ---
	couplingIdx := classify.Run(g, classifyCfg)

	// --- Stage 3: Rules ---
	var rawFindings []finding.Finding
	for _, r := range rs {
		rawFindings = append(rawFindings, r.Check(g, rules.Evidence{})...)
	}

	// --- Stage 4: Status ---
	taggedFindings := status.Assign(rawFindings, base, exceptions, now)

	// --- Stage 5: Metrics ---
	mi := metrics.MetricInput{
		Graph:           g,
		Classifications: couplingIdx,
		Findings:        taggedFindings,
		Baseline:        base.Metrics,
		ToolCoverage:    coverages,
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
		pp := pathPair{
			from: stripPrefix(e.From),
			to:   stripPrefix(e.To),
			kind: string(e.Kind),
		}
		pairClass[pp] = classEntry{
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
		pp := pathPair{from: f.Edge.From.Path, to: f.Edge.To.Path, kind: f.Edge.Kind}
		if ce, ok := pairClass[pp]; ok {
			f.Severity = severityFor(ce.strength, ce.distance)
		}

		resolvedFindings = append(resolvedFindings, f)
	}

	// --- Stage 6: Advisory findings ---
	// Collect coupling advisories by walking edges in graph order (deterministic).
	// Edges with Severity != "" represent imbalanced or intrusive coupling.
	var advisoryFindings []finding.Finding
	for _, e := range g.Edges() {
		key := e.From + "\x00" + e.To + "\x00" + string(e.Kind)
		cl, ok := couplingIdx[key]
		if !ok || cl.Severity == coupling.SeverityNone {
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
		warnings = len(advisoryFindings)
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

// stripPrefix removes the "kind:" prefix from a node ID (e.g. "file:pkg/a" → "pkg/a").
func stripPrefix(id string) string {
	for i := 0; i < len(id); i++ {
		if id[i] == ':' {
			return id[i+1:]
		}
	}
	return id
}

// couplingAdvisoryID returns a stable 32-character hex fingerprint for a coupling advisory
// finding, derived from (from, to, kind) — same scheme as finding.fingerprint.
func couplingAdvisoryID(from, to, kind string) string {
	h := sha256.Sum256([]byte("bc/imbalanced_coupling\x00" + from + "\x00" + to + "\x00" + kind))
	return hex.EncodeToString(h[:16])
}
