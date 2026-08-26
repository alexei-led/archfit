package evaluation

import (
	"strconv"

	"github.com/alexei-led/archfit/internal/assessment/finding"
	"github.com/alexei-led/archfit/internal/assessment/result"
	"github.com/alexei-led/archfit/internal/assessment/state"
	modevidence "github.com/alexei-led/archfit/internal/model/evidence"
	"github.com/alexei-led/archfit/internal/model/fileclass"
	"github.com/alexei-led/archfit/internal/policy"
)

// Metric units. They mirror the existing metric Mode vocabulary so a dimension
// metric and the metric it came from cannot disagree about what the number is.
const (
	unitCount = "count"
	unitRatio = "ratio"
)

// Provenance names. Each says which capability observed the fact, so a reader
// can tell a policy declaration from a measured one.
const (
	provPolicy       = "policy"
	provMetrics      = "assessment/metrics"
	provRelationship = "relationship/analysis"
	provFileClass    = "syntax/fileclass"
	provAcquisition  = "evidence/acquisition"
	provGitHistory   = "history/git"
	provAssessment   = "assessment/evaluation"
)

// gateOffPosture is the rule gate that switches a declared rule off entirely.
// It is the same vocabulary the coverage-gap gate uses.
const gateOffPosture = gateOff

// buildDimensions populates the nine architecture-state envelopes from facts
// this run already produced. It measures nothing new: every number here is a
// projection of a policy declaration, a relationship classification, a computed
// metric, a file-class walk, an acquisition coverage row, or git history.
//
// Where v1 has no collector — cognitive complexity, executed test coverage,
// observed runtime topology, a comparable v2 baseline — the envelope reports
// partial or unmeasured and names the missing fact. Reporting those as
// measured-and-empty is exactly the implicit green result the contract exists
// to prevent.
func buildDimensions(diag *result.Result, in stateInput, routed map[string][]state.FindingRef) state.Dimensions {
	dims := state.Dimensions{
		Intent:         intentDimension(diag, in.Policy),
		Structure:      structureDimension(diag),
		Modularity:     modularityDimension(diag, in.Policy),
		Coupling:       couplingDimension(diag),
		ChangeLocality: changeLocalityDimension(diag),
		Complexity:     complexityDimension(in.Facts),
		Testability:    testabilityDimension(in.Facts),
		Operations:     operationsDimension(diag, in.Policy, in.RequiredToolFailure),
		Drift:          driftDimension(diag),
	}
	for _, dim := range dims.Each() {
		if refs := routed[dim.Name]; len(refs) > 0 {
			dim.Findings = refs
		}
		dim.Gate = gateFor(*dim)
	}
	return dims
}

// gateFor is one dimension's gate posture, read from its own classified
// findings. A dimension fails only when a hard-gate finding was routed to it,
// which by construction can only happen where a named hard rule lives; an
// active diagnostic warns; a measured dimension with nothing active passes.
//
// A dimension that already declared a failure keeps it: the required-tool
// policy fails without producing a finding, so its gate cannot be inferred
// from the finding list.
func gateFor(d state.Dimension) state.GateState {
	if d.Gate == state.GateFail {
		return state.GateFail
	}
	if d.Status == state.Unmeasured {
		return state.GateNotApplicable
	}
	for _, ref := range d.Findings {
		if ref.Kind == finding.KindGate {
			return state.GateFail
		}
	}
	if len(d.Findings) > 0 {
		return state.GateWarn
	}
	return state.GatePass
}

// dimensionForRule routes one finding to the dimension that owns its subject.
//
// Assessment's own rule IDs are matched first because they are not declared in
// config and therefore have no type. Declared rules route by type: dependency
// direction, layering, and cycles are structure facts; every public-surface and
// internal-access rule is a modularity fact.
//
// An unrecognised rule ID lands in intent — a finding from a rule nobody can
// classify is still evidence about configured intent, and dropping it would
// hide it from every dimension at once.
func dimensionForRule(ruleID string, ruleTypes map[string]string) string {
	switch ruleID {
	case finding.RuleIDBCImbalanced, finding.RuleIDDuplicatedKnowledge, finding.RuleIDCouplingGate, ruleIDStaleLabel:
		return state.DimensionCoupling
	}
	switch ruleTypes[ruleID] {
	case "forbidden_dependency", "forbidden_layer_direction", "cycle", "new_cross_module_dependency":
		return state.DimensionStructure
	case "public_api_only", "public_api_max", "public_api_change", "public_api_type_leak", "internal_api_access":
		return state.DimensionModularity
	}
	return state.DimensionIntent
}

// ---------------------------------------------------------------------------
// Dimension collectors
// ---------------------------------------------------------------------------

// intentDimension reports configured intent and gate conformance: what the
// configuration declares and how much of it was actually evaluated. A config
// that declares neither a module nor a rule states no intent, so there is
// nothing to conform to and nothing to measure.
func intentDimension(diag *result.Result, p policy.PolicySnapshot) state.Dimension {
	dim := state.NewDimension(state.DimensionIntent, state.OwnerIntent)
	rules := p.Gates.Rules.Rules
	modules := len(p.Topology.Modules)
	if len(rules) == 0 && modules == 0 {
		return unmeasured(dim, state.UnknownFact{
			Fact:   "configured intent",
			Reason: "the configuration declares no module and no rule, so there is no stated intent to conform to",
			Owner:  state.OwnerIntent,
		})
	}
	evaluated := 0
	for _, r := range rules {
		if r.Gate != gateOffPosture {
			evaluated++
		}
	}
	dim.Status = state.Measured
	dim.Coverage = state.Coverage{Basis: "declared rules evaluated", Observed: evaluated, Total: len(rules)}
	dim.Metrics = []state.MetricValue{
		count("declared_modules", modules, provPolicy),
		count("declared_layers", len(p.Topology.Layers), provPolicy),
		{Name: "declared_rules", Value: float64(len(rules)), Unit: unitCount,
			Denominator: &state.MetricDenominator{Observed: evaluated, Total: len(rules)},
			Provenance:  []string{provPolicy}},
		count("declared_waivers", len(p.Assessment.Waivers.Waivers), provPolicy),
		count("waivers_used", diag.Summary.WaiversUsed, provAssessment),
		count("expired_waivers", countStatus(diag.Findings, finding.StatusExpiredWaiver), provAssessment),
	}
	if off := len(rules) - evaluated; off > 0 {
		dim.Unknown = append(dim.Unknown, state.UnknownFact{
			Fact:   "conformance to " + strconv.Itoa(off) + " declared rules",
			Reason: "the rules declare gate: off, so they were compiled out and never evaluated",
			Owner:  state.OwnerIntent,
		})
	}
	dim.Confidence = state.ConfidenceFor(dim.Status)
	return dim
}

// structureDimension reports dependency structure and direction. Its
// denominator is how much of the discovered edge set resolved to a declared
// module: an edge whose target is outside the module map was discovered but
// says nothing about this repository's internal structure.
func structureDimension(diag *result.Result) state.Dimension {
	dim := state.NewDimension(state.DimensionStructure, state.OwnerStructure)
	ce := diag.ClassifiedEdges
	if ce == nil || ce.Total == 0 {
		return unmeasured(dim, state.UnknownFact{
			Fact:   "dependency structure",
			Reason: "relationship analysis classified no edge; no extractor produced a dependency graph over this tree",
			Owner:  state.OwnerStructure,
		})
	}
	internal := ce.Total - ce.External
	dim.Status = state.Measured
	dim.Coverage = state.Coverage{Basis: "discovered edges resolved to a declared module", Observed: internal, Total: ce.Total}
	dim.Metrics = []state.MetricValue{
		count("internal_edges", internal, provRelationship),
		count("external_edges", ce.External, provRelationship),
		count("same_module_edges", ce.SameModule, provRelationship),
		count("connected_modules", ce.ConnectedModules, provRelationship),
	}
	dim.Metrics = append(dim.Metrics, metricValues(diag.Metrics, "cycle")...)
	dim.Confidence = weakest(state.ConfidenceFor(dim.Status), metricConfidence(diag.Metrics, "cycle"))
	if ce.External > 0 {
		dim.Unknown = append(dim.Unknown, state.UnknownFact{
			Fact:   "structure of " + strconv.Itoa(ce.External) + " edges leaving the module map",
			Reason: "the target is not a declared module, so its direction and layer cannot be judged",
			Owner:  state.OwnerStructure,
		})
	}
	return dim
}

// modularityDimension reports cohesion, hubs, and public surface over the
// declared module set. Its denominator is the declared modules: modularity is a
// statement about boundaries somebody drew, so an undeclared tree has none.
func modularityDimension(diag *result.Result, p policy.PolicySnapshot) state.Dimension {
	dim := state.NewDimension(state.DimensionModularity, state.OwnerModularity)
	modules := len(p.Topology.Modules)
	if modules == 0 {
		return unmeasured(dim, state.UnknownFact{
			Fact:   "module boundaries",
			Reason: "the configuration declares no module, so there is no boundary to measure cohesion or public surface against",
			Owner:  state.OwnerModularity,
		})
	}
	withPublic, publicEntries := 0, 0
	for _, def := range p.Topology.Modules {
		if len(def.Public) > 0 {
			withPublic++
			publicEntries += len(def.Public)
		}
	}
	dim.Status = state.Measured
	dim.Coverage = state.Coverage{Basis: "declared modules with a declared public surface", Observed: withPublic, Total: modules}
	dim.Metrics = []state.MetricValue{
		count("declared_modules", modules, provPolicy),
		{Name: "public_surface_entries", Value: float64(publicEntries), Unit: unitCount,
			Denominator: &state.MetricDenominator{Observed: withPublic, Total: modules},
			Provenance:  []string{provPolicy}},
		count("local_coupling_modules", len(diag.LocalCoupling), provRelationship),
	}
	dim.Metrics = append(dim.Metrics, metricValues(diag.Metrics, "blast_radius", "encapsulation")...)
	dim.Confidence = weakest(state.ConfidenceFor(dim.Status), metricConfidence(diag.Metrics, "blast_radius", "encapsulation"))
	if withPublic == 0 {
		dim.Unknown = append(dim.Unknown, state.UnknownFact{
			Fact:   "public surface",
			Reason: "no declared module states a public surface, so every symbol reads as equally public",
			Owner:  state.OwnerModularity,
		})
	}
	return dim
}

// couplingDimension reports the coupling seam evidence. Its denominator is the
// existing scored-plus-abstained split: an abstained edge is a seam whose
// strength or distance is unknown, which lowers confidence rather than
// disappearing from the count.
//
// V1 reports the underlying edge classification only. The logical seam ledger,
// its distribution, and the distributed-monolith policy are a separate task;
// this envelope carries their facts when they land.
func couplingDimension(diag *result.Result) state.Dimension {
	dim := state.NewDimension(state.DimensionCoupling, state.OwnerCoupling)
	ce := diag.ClassifiedEdges
	if ce == nil || ce.Scored+ce.Abstained == 0 {
		return unmeasured(dim, state.UnknownFact{
			Fact:   "coupling seams",
			Reason: "no cross-boundary edge was classified: the module map matched nothing, or every edge left the declared modules",
			Owner:  state.OwnerCoupling,
		})
	}
	denominator := ce.Scored + ce.Abstained
	dim.Status = state.Measured
	if ce.Abstained > 0 {
		// An abstained edge is a measured seam with an unknown rung. The
		// dimension is not fully measured while any remain, and saying so is the
		// difference between "no critical coupling" and "we could not tell".
		dim.Status = state.Partial
		dim.Unknown = append(dim.Unknown, state.UnknownFact{
			Fact:   "balance of " + strconv.Itoa(ce.Abstained) + " cross-boundary edges",
			Reason: "integration strength or distance is unknown, so the edge is deliberately unscored rather than given an invented rung",
			Owner:  state.OwnerCoupling,
		})
	}
	dim.Coverage = state.Coverage{Basis: "cross-boundary edges scored", Observed: ce.Scored, Total: denominator}
	dim.Metrics = []state.MetricValue{
		{Name: "scored_edges", Value: float64(ce.Scored), Unit: unitCount,
			Denominator: &state.MetricDenominator{Observed: ce.Scored, Total: denominator},
			Provenance:  []string{provRelationship}},
		count("abstained_edges", ce.Abstained, provRelationship),
		count("declared_external_edges", ce.DeclaredExternal, provRelationship),
		count("clone_only_seams", ce.CloneOnlyScored, provRelationship),
	}
	if tail := ce.TailRisk; tail != nil {
		dim.Metrics = append(dim.Metrics,
			count("critical_band_edges", tail.CriticalEdges, provRelationship),
			count("high_or_worse_edges", tail.HighOrWorseEdges, provRelationship),
			// The existing edge-level distributed-monolith count is a diagnostic
			// fact about edges. It is not the seam policy, which counts logical
			// module pairs and is owned by the coupling gate, not this envelope.
			count("critical_high_distance_edges", tail.DistributedMonolithEdges, provRelationship))
	}
	dim.Metrics = append(dim.Metrics, metricValues(diag.Metrics, "unbalanced_edge")...)
	dim.Confidence = weakest(state.ConfidenceFor(dim.Status), metricConfidence(diag.Metrics, "unbalanced_edge"))
	return dim
}

// changeLocalityDimension reports the bounded git-history corroboration.
// Zero observed commits is unmeasured, never a quiet zero: an unscanned history
// and a genuinely stable repository produce the same number and must not
// produce the same status.
func changeLocalityDimension(diag *result.Result) state.Dimension {
	dim := state.NewDimension(state.DimensionChangeLocality, state.OwnerChangeLocality)
	h := diag.VolatilityCorroboration
	switch {
	case h == nil:
		return unmeasured(dim, state.UnknownFact{
			Fact:   "change locality",
			Reason: "git history is unavailable: the tree is not a repository, or no module was declared to attribute commits to",
			Owner:  state.OwnerChangeLocality,
		})
	case h.CommitsScanned == 0:
		return unmeasured(dim, state.UnknownFact{
			Fact:   "change locality",
			Reason: "the history scan returned no eligible commit (" + h.Status + "), so co-change cannot be distinguished from a stable tree",
			Owner:  state.OwnerChangeLocality,
		})
	}
	dim.Status = state.Measured
	if h.Status != modevidence.StatusOK {
		dim.Status = state.Partial
		dim.Unknown = append(dim.Unknown, state.UnknownFact{
			Fact:   "the remainder of the history window",
			Reason: "the history scan reported " + h.Status + ", so the commit sample is truncated",
			Owner:  state.OwnerChangeLocality,
		})
	}
	dim.Coverage = state.Coverage{Basis: "modules touched in the scanned history window",
		Observed: h.ModulesTouched, Total: h.ModulesTouched}
	dim.Metrics = []state.MetricValue{
		count("commits_scanned", h.CommitsScanned, provGitHistory),
		count("modules_touched", h.ModulesTouched, provGitHistory),
	}
	dim.Confidence = state.ConfidenceFor(dim.Status)
	// Git history reflects both essential and accidental volatility, so it
	// corroborates a declared volatility and never replaces it.
	dim.Unknown = append(dim.Unknown, state.UnknownFact{
		Fact:   "essential vs accidental volatility",
		Reason: "commit frequency corroborates a declared volatility; it cannot establish one",
		Owner:  state.OwnerChangeLocality,
	})
	return dim
}

// complexityDimension reports code size over production files. It is always
// partial in v1: cognitive complexity has no collector, so the envelope carries
// the size tail and names what is missing rather than implying the tail is the
// whole answer.
func complexityDimension(f Observations) state.Dimension {
	dim := state.NewDimension(state.DimensionComplexity, state.OwnerComplexity)
	if len(f.FileLOC) == 0 {
		return unmeasured(dim, state.UnknownFact{
			Fact:   "code size and complexity",
			Reason: "the source walk produced no file, so there is nothing to measure size or complexity over",
			Owner:  state.OwnerComplexity,
		})
	}
	production, productionLOC, largest := 0, 0, 0
	for file, loc := range f.FileLOC {
		if !fileclass.IsProduction(classOf(f.FileClassIndex, file)) {
			continue
		}
		production++
		productionLOC += loc
		if loc > largest {
			largest = loc
		}
	}
	dim.Status = state.Partial
	dim.Coverage = state.Coverage{Basis: "production files in the source walk", Observed: production, Total: len(f.FileLOC)}
	dim.Metrics = []state.MetricValue{
		count("production_files", production, provFileClass),
		count("production_loc", productionLOC, provFileClass),
		count("largest_production_file_loc", largest, provFileClass),
	}
	dim.Confidence = state.ConfidenceFor(dim.Status)
	dim.Unknown = []state.UnknownFact{{
		Fact:   "cognitive complexity",
		Reason: "v1 ships no cognitive-complexity analyzer; only the size tail is measured",
		Owner:  state.OwnerComplexity,
	}}
	return dim
}

// testabilityDimension reports the static test-to-production file split. It is
// always partial in v1: archfit does not execute a target repository's test
// suite, so executed coverage is absent by design and is named rather than
// approximated by the file counts.
func testabilityDimension(f Observations) state.Dimension {
	dim := state.NewDimension(state.DimensionTestability, state.OwnerTestability)
	if len(f.FileClassIndex) == 0 {
		return unmeasured(dim, state.UnknownFact{
			Fact:   "testability",
			Reason: "no file was classified, so test and production files cannot be told apart",
			Owner:  state.OwnerTestability,
		})
	}
	tests, production := 0, 0
	for _, class := range f.FileClassIndex {
		switch {
		case class == fileclass.Test:
			tests++
		case fileclass.IsProduction(class):
			production++
		}
	}
	dim.Status = state.Partial
	dim.Coverage = state.Coverage{Basis: "classified source files", Observed: tests + production, Total: len(f.FileClassIndex)}
	dim.Metrics = []state.MetricValue{
		count("test_files", tests, provFileClass),
		count("production_files", production, provFileClass),
		ratio("test_to_production_files", tests, production, provFileClass),
	}
	dim.Confidence = state.ConfidenceFor(dim.Status)
	dim.Unknown = []state.UnknownFact{{
		Fact:   "executed test coverage",
		Reason: "v1 does not run a target repository's test suite; supplied coverage is not yet an input",
		Owner:  state.OwnerTestability,
	}, {
		Fact:   "boundary test coverage",
		Reason: "which module boundaries a test actually exercises needs test-to-production import resolution, which v1 does not collect",
		Owner:  state.OwnerTestability,
	}}
	return dim
}

// operationsDimension reports declared operational topology and analyzer
// coverage. It is always partial in v1: observed runtime topology, SBOM, and
// vulnerability facts have no collector, so declarations are all there is.
//
// It is the one dimension that can fail without a finding — the required-tool
// policy is a gate over evidence, not over a violation.
func operationsDimension(diag *result.Result, p policy.PolicySnapshot, requiredToolFailure bool) state.Dimension {
	dim := state.NewDimension(state.DimensionOperations, state.OwnerOperations)
	modules := len(p.Topology.Modules)
	if modules == 0 && len(diag.ToolCoverage) == 0 {
		return unmeasured(dim, state.UnknownFact{
			Fact:   "operational topology",
			Reason: "nothing declares an owner or a deploy unit and no analyzer reported coverage",
			Owner:  state.OwnerOperations,
		})
	}
	owners, deployUnits := map[string]struct{}{}, map[string]struct{}{}
	withOwner := 0
	for _, def := range p.Topology.Modules {
		if def.Owner != "" {
			owners[def.Owner] = struct{}{}
			withOwner++
		}
		if def.DeployUnit != "" {
			deployUnits[def.DeployUnit] = struct{}{}
		}
	}
	reporting := 0
	for _, c := range diag.ToolCoverage {
		if c.Status == modevidence.StatusOK || c.Status == modevidence.StatusPartial {
			reporting++
		}
	}
	dim.Status = state.Partial
	dim.Coverage = state.Coverage{Basis: "analyzers reporting coverage", Observed: reporting, Total: len(diag.ToolCoverage)}
	dim.Metrics = []state.MetricValue{
		{Name: "modules_with_owner", Value: float64(withOwner), Unit: unitCount,
			Denominator: &state.MetricDenominator{Observed: withOwner, Total: modules},
			Provenance:  []string{provPolicy}},
		count("distinct_owners", len(owners), provPolicy),
		count("deploy_units", len(deployUnits), provPolicy),
		count("declared_external_systems", len(p.Topology.ExternalSystems), provPolicy),
		count("coverage_gaps", len(diag.CoverageGaps), provAcquisition),
	}
	dim.Metrics = append(dim.Metrics, metricValues(diag.Metrics, "coverage")...)
	dim.Confidence = weakest(state.ConfidenceFor(dim.Status), metricConfidence(diag.Metrics, "coverage"))
	dim.Unknown = []state.UnknownFact{{
		Fact:   "observed runtime topology",
		Reason: "v1 reports declared owners and deploy units only; nothing observes what actually runs",
		Owner:  state.OwnerOperations,
	}, {
		Fact:   "supply-chain inventory",
		Reason: "SBOM and vulnerability facts have no collector in v1",
		Owner:  state.OwnerOperations,
	}}
	for _, gap := range diag.CoverageGaps {
		dim.Unknown = append(dim.Unknown, state.UnknownFact{
			Fact:   "analyzer coverage for " + gap.Tool,
			Reason: "the analyzer did not run over this tree (gate: " + gap.Gate + "); install it with: " + gap.InstallCmd,
			Owner:  state.OwnerOperations,
		})
	}
	if requiredToolFailure {
		dim.Gate = state.GateFail
	}
	return dim
}

// driftDimension reports erosion against a comparable reference. It is
// unmeasured in v1 by contract, not by omission: a pre-state baseline carries a
// scalar snapshot this binary ignores, so no stored reference can support a
// comparable dimension or seam delta yet.
func driftDimension(diag *result.Result) state.Dimension {
	dim := state.NewDimension(state.DimensionDrift, state.OwnerDrift)
	reasons := []string{"legacy_score_snapshot_ignored", "no_comparable_state_baseline"}
	dim.Delta = &state.Delta{Status: state.ComparisonNonComparable, Reasons: reasons}
	if d := diag.Delta; d != nil {
		// Accepted finding fingerprints stay usable across the migration, so the
		// finding-level buckets are real evidence. They are attached as evidence
		// only: the dimension still reports unmeasured because no metric or seam
		// on either side is comparable.
		dim.Delta.NewFindings = d.New
		dim.Delta.ResolvedFindings = d.Resolved
	}
	return unmeasured(dim, state.UnknownFact{
		Fact:   "architecture drift",
		Reason: "no comparable architecture-state baseline exists: a stored baseline records a scalar snapshot this binary ignores",
		Owner:  state.OwnerDrift,
	})
}

// ---------------------------------------------------------------------------
// Envelope helpers
// ---------------------------------------------------------------------------

// unmeasured stamps the reason a dimension has no measurement and returns it.
// An unmeasured envelope with no stated reason is indistinguishable from one
// nobody bothered to explain.
func unmeasured(dim state.Dimension, why state.UnknownFact) state.Dimension {
	dim.Status = state.Unmeasured
	dim.Confidence = state.ConfidenceUnrated
	dim.Unknown = append(dim.Unknown, why)
	return dim
}

func count(name string, value int, provenance string) state.MetricValue {
	return state.MetricValue{Name: name, Value: float64(value), Unit: unitCount, Provenance: []string{provenance}}
}

// ratio reports numerator/denominator, carrying both sides in the denominator
// block so a 0 over 0 stays distinguishable from a measured zero.
func ratio(name string, numerator, denominator int, provenance string) state.MetricValue {
	value := 0.0
	if denominator > 0 {
		value = float64(numerator) / float64(denominator)
	}
	return state.MetricValue{Name: name, Value: value, Unit: unitRatio,
		Denominator: &state.MetricDenominator{Observed: numerator, Total: denominator},
		Provenance:  []string{provenance}}
}

// metricValues projects already-computed metric results into dimension metrics,
// in the requested order. A metric that reported n/a is skipped: it measured
// nothing, and copying its zero would turn an abstention into a value.
func metricValues(computed []result.MetricResult, names ...string) []state.MetricValue {
	out := make([]state.MetricValue, 0, len(names))
	for _, name := range names {
		m, ok := metricByName(computed, name)
		if !ok || m.Band == bandNA {
			continue
		}
		unit := m.Mode
		if unit == "" {
			unit = unitCount
		}
		out = append(out, state.MetricValue{Name: m.Name, Value: m.Value, Unit: unit,
			Provenance: []string{provMetrics + "/" + m.Version}})
	}
	return out
}

// metricConfidence is the weakest confidence among the named computed metrics.
// A dimension cannot be more confident than the evidence it reports.
func metricConfidence(computed []result.MetricResult, names ...string) state.Confidence {
	out := state.ConfidenceHigh
	for _, name := range names {
		m, ok := metricByName(computed, name)
		if !ok || m.Band == bandNA {
			continue
		}
		out = weakest(out, confidenceOf(m.Confidence))
	}
	return out
}

func metricByName(computed []result.MetricResult, name string) (result.MetricResult, bool) {
	for _, m := range computed {
		if m.Name == name {
			return m, true
		}
	}
	return result.MetricResult{}, false
}

// bandNA is the metric band that marks an unmeasured metric. It mirrors the
// metrics package's own constant, which is behind an internal package.
const bandNA = "n/a"

func confidenceOf(s string) state.Confidence {
	switch s {
	case string(state.ConfidenceHigh):
		return state.ConfidenceHigh
	case string(state.ConfidenceMedium):
		return state.ConfidenceMedium
	case string(state.ConfidenceLow):
		return state.ConfidenceLow
	default:
		return state.ConfidenceUnrated
	}
}

// weakest returns the lower of two confidences on the high > medium > low >
// unrated ladder.
func weakest(a, b state.Confidence) state.Confidence {
	if confidenceRank(b) < confidenceRank(a) {
		return b
	}
	return a
}

func confidenceRank(c state.Confidence) int {
	switch c {
	case state.ConfidenceHigh:
		return 3
	case state.ConfidenceMedium:
		return 2
	case state.ConfidenceLow:
		return 1
	default:
		return 0
	}
}

func countStatus(findings []finding.Finding, want finding.Status) int {
	n := 0
	for _, f := range findings {
		if f.Status == want {
			n++
		}
	}
	return n
}

// classOf resolves a file's class, defaulting to production when the walk did
// not classify it — the same fallback the metric filters use.
func classOf(index map[string]fileclass.FileClass, file string) fileclass.FileClass {
	if class, ok := index[file]; ok {
		return class
	}
	return fileclass.Production
}
