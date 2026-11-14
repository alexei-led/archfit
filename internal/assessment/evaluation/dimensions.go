package evaluation

import (
	"fmt"
	"maps"
	"slices"
	"sort"
	"strconv"
	"strings"

	"github.com/alexei-led/archfit/v3/internal/assessment/finding"
	"github.com/alexei-led/archfit/v3/internal/assessment/result"
	"github.com/alexei-led/archfit/v3/internal/assessment/score"
	"github.com/alexei-led/archfit/v3/internal/assessment/state"
	modevidence "github.com/alexei-led/archfit/v3/internal/model/evidence"
	"github.com/alexei-led/archfit/v3/internal/model/fileclass"
	"github.com/alexei-led/archfit/v3/internal/policy"
	"github.com/alexei-led/archfit/v3/internal/relationship"
)

// Metric units. They mirror the existing metric Mode vocabulary so a dimension
// metric and the metric it came from cannot disagree about what the number is.
const (
	unitCount  = "count"
	goLanguage = "go"
	unitRatio  = "ratio"
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

// buildDimensions populates the nine architecture-state envelopes from facts
// this run already produced. It measures nothing new: every number here is a
// projection of a policy declaration, a relationship classification, a computed
// metric, a file-class walk, an acquisition coverage row, or git history.
//
// Where an in-claim fact has no collector — supplied test coverage or
// historical seam identity — the envelope reports partial or unmeasured
// and names the missing fact. Out-of-claim facts,
// such as observed runtime topology, remain explicit disclosures without
// blocking a complete declared-topology claim.
func buildDimensions(diag *result.Result, in stateInput, routed map[string][]state.FindingRef) state.Dimensions {
	dims := state.Dimensions{
		Intent:         intentDimension(diag, in.Policy, in.Facts),
		Structure:      structureDimension(diag, in.Facts),
		Modularity:     modularityDimension(diag, in.Policy, in.Facts),
		Coupling:       couplingDimension(diag),
		ChangeLocality: changeLocalityDimension(),
		Complexity:     complexityDimension(diag, in.Policy, in.Facts),
		Testability:    testabilityDimension(in.Policy, in.Facts),
		Operations:     operationsDimension(diag, in.Policy, in.Facts),
		Drift:          driftDimension(),
	}
	for _, dim := range dims.Each() {
		if refs := routed[dim.Name]; len(refs) > 0 {
			dim.Findings = refs
		}
	}
	return dims
}

// dimensionForRule routes report advisories to the measurement they describe.
func dimensionForRule(ruleID string) string {
	switch ruleID {
	case finding.RuleIDBCImbalanced, finding.RuleIDDuplicatedKnowledge:
		return state.DimensionCoupling
	default:
		return state.DimensionIntent
	}
}

// ---------------------------------------------------------------------------
// Dimension collectors
// ---------------------------------------------------------------------------

type primaryInventory struct {
	known      bool
	applicable int
	completed  int
	// gaps name the incomplete files of a language with scoped evidence
	// (Observations.ScopedDependencyEvidence), as "<tool>: <gap list>".
	gaps []string
}

// primaryDependencyInventory counts the applicable primary extractors and those
// that completed. A primary whose row is ok still has not completed when its
// language's scoped evidence names incomplete files: the row counts the
// projects it evaluated, not the file edges the graph needs.
func primaryDependencyInventory(diag *result.Result, f Observations) primaryInventory {
	out := primaryInventory{known: len(diag.PrimaryExtractorTools) > 0}
	incompleteTools := map[string]struct{}{}
	for _, language := range slices.Sorted(maps.Keys(f.ScopedDependencyEvidence)) {
		gaps := scopedIncomplete(f, language)
		tool, ok := primaryToolForLanguage(diag, language)
		if len(gaps) == 0 || !ok {
			continue
		}
		incompleteTools[tool] = struct{}{}
		ev := f.ScopedDependencyEvidence[language]
		out.gaps = append(out.gaps, ev.Tool+": "+gapSummary(ev, gaps))
	}
	gapped := make(map[string]struct{}, len(diag.CoverageGaps))
	for _, gap := range diag.CoverageGaps {
		gapped[gap.Tool] = struct{}{}
	}
	seenTools := make(map[string]struct{}, len(diag.PrimaryExtractorTools))
	for _, tool := range diag.PrimaryExtractorTools {
		if _, duplicateTool := seenTools[tool]; duplicateTool {
			continue
		}
		seenTools[tool] = struct{}{}
		rows := coverageRows(diag.ToolCoverage, tool)
		if len(rows) == 1 && rows[0].Status == modevidence.StatusAbsent {
			if _, hasGap := gapped[tool]; !hasGap {
				// A gapless absent primary is the extractor's own proof that its
				// language is not present in this tree.
				continue
			}
		}
		out.applicable++
		if _, incomplete := incompleteTools[tool]; !incomplete && len(rows) == 1 && rows[0].Status == modevidence.StatusOK {
			out.completed++
			continue
		}
	}
	return out
}

func coverageRows(rows []modevidence.Coverage, tool string) []modevidence.Coverage {
	out := make([]modevidence.Coverage, 0, 1)
	for _, row := range rows {
		if row.Tool == tool {
			out = append(out, row)
		}
	}
	return out
}

func (p primaryInventory) complete() bool {
	return p.known && p.applicable > 0 && p.completed == p.applicable
}

func primaryToolForLanguage(diag *result.Result, language string) (string, bool) {
	// PrimaryExtractorTools is injected in the registry's append-only language
	// order (Go, TypeScript, Python, Rust, .NET). Keep adapter tool names out of core;
	// an unknown language or shortened inventory abstains instead of guessing.
	index := -1
	switch language {
	case "go":
		index = 0
	case "typescript":
		index = 1
	case "python":
		index = 2
	case "rust":
		index = 3
	case "dotnet":
		index = 4
	}
	if index < 0 || index >= len(diag.PrimaryExtractorTools) {
		return "", false
	}
	return diag.PrimaryExtractorTools[index], true
}

func metricsProduced(computed []result.MetricResult, names ...string) bool {
	for _, name := range names {
		if _, ok := metricByName(computed, name); !ok {
			return false
		}
	}
	return true
}

func applyPromotion(dim *state.Dimension, observed, notApplicable []string, reasons map[string]string) {
	status, missing := state.Promote(dim.Name, observed, notApplicable)
	for i := range missing {
		if reason := reasons[missing[i].Fact]; reason != "" {
			missing[i].Reason = reason
		}
	}
	dim.Status = status
	dim.Confidence = state.ConfidenceFor(status)
	dim.Unknown = append(dim.Unknown, missing...)
}

// intentDimension inventories the declared modules and layers. Rule errors
// belong to the repository state, not to a second report evaluator.
func intentDimension(_ *result.Result, p policy.PolicySnapshot, _ Observations) state.Dimension {
	dim := state.NewDimension(state.DimensionIntent, state.OwnerIntent)
	if len(p.Topology.Modules) == 0 {
		applyPromotion(&dim, nil, nil, map[string]string{
			state.FactDeclaredIntentInventory: "the policy declares no modules",
		})
		return dim
	}
	applyPromotion(&dim, []string{state.FactDeclaredIntentInventory}, nil, nil)
	dim.Coverage = state.Coverage{Basis: "declared modules", Observed: len(p.Topology.Modules), Total: len(p.Topology.Modules)}
	dim.Metrics = []state.MetricValue{
		count("declared_modules", len(p.Topology.Modules), provPolicy),
		count("declared_layers", len(p.Topology.Layers), provPolicy),
	}
	return dim
}

// structureDimension reports dependency structure and direction. Its
// denominator is how much of the discovered FIRST-PARTY edge set resolved to a
// declared module: an edge to a library was discovered but can never resolve
// inside the module map, so counting it would cap the ratio by construction in
// any library-using repository. First-party edges no module owns stay in the
// denominator — those are the gap the module map itself leaves.
func structureDimension(diag *result.Result, f Observations) state.Dimension {
	dim := state.NewDimension(state.DimensionStructure, state.OwnerStructure)
	primaries := primaryDependencyInventory(diag, f)
	reasons := map[string]string{
		state.FactPrimaryDependencyInventory: "no applicable primary dependency inventory completed over this tree",
		state.FactInternalEdgeClassification: "module, direction, layer, or cycle classification is incomplete for the discovered internal dependencies",
	}
	if !primaries.known || primaries.applicable == 0 {
		if primaries.known {
			reasons[state.FactPrimaryDependencyInventory] = "every primary extractor proved its language absent, so dependency structure does not apply to this tree"
		}
		applyPromotion(&dim, nil, nil, reasons)
		return dim
	}

	ce := diag.ClassifiedEdges
	observed := make([]string, 0, 2)
	if primaries.complete() {
		observed = append(observed, state.FactPrimaryDependencyInventory)
	}
	if len(primaries.gaps) > 0 {
		reasons[state.FactPrimaryDependencyInventory] = "dependency evidence is incomplete for source files: " + strings.Join(primaries.gaps, "; ")
	}
	if primaries.completed > 0 && ce != nil &&
		ce.ClassifiedInternalDependencies == ce.InternalDependencies && metricsProduced(diag.Metrics, "cycle") {
		observed = append(observed, state.FactInternalEdgeClassification)
	}
	applyPromotion(&dim, observed, nil, reasons)
	if ce == nil {
		return dim
	}

	external := ce.DependencyEdges - ce.InternalDependencies
	firstPartyEdges := ce.InternalDependencies + ce.UnmappedFirstPartyDependencies
	dim.Coverage = state.Coverage{Basis: "discovered first-party dependencies resolved inside the declared module map", Observed: ce.InternalDependencies, Total: firstPartyEdges}
	dim.Metrics = []state.MetricValue{
		count("internal_edges", ce.InternalDependencies, provRelationship),
		count("library_edges", ce.LibraryDependencies, provRelationship),
		count("unmapped_first_party_edges", ce.UnmappedFirstPartyDependencies, provRelationship),
		count("same_module_edges", ce.SameModuleDependencies, provRelationship),
		count("connected_modules", ce.DependencyModules, provRelationship),
	}
	dim.Metrics = append(dim.Metrics, metricValues(diag.Metrics, "cycle")...)
	dim.Confidence = weakest(state.ConfidenceFor(dim.Status), metricConfidence(diag.Metrics, "cycle"))
	if external > 0 {
		dim.Unknown = append(dim.Unknown, state.UnknownFact{
			Fact: state.FactExternalDependencyStructure,
			Reason: strconv.Itoa(external) + " dependencies leave the declared module map (" +
				strconv.Itoa(ce.LibraryDependencies) + " on libraries, " +
				strconv.Itoa(ce.UnmappedFirstPartyDependencies) + " to or from first-party code no module owns), so their direction and layer are outside this claim",
			Owner: state.OwnerStructure,
		})
	}
	return dim
}

// modularityDimension reports cohesion, hubs, and public surface over the
// declared module set. Its denominator is the declared modules: modularity is a
// statement about boundaries somebody drew, so an undeclared tree has none.
func modularityDimension(diag *result.Result, p policy.PolicySnapshot, f Observations) state.Dimension {
	dim := state.NewDimension(state.DimensionModularity, state.OwnerModularity)
	modules := len(p.Topology.Modules)
	reasons := map[string]string{
		state.FactDeclaredModuleInventory:   "the configuration declares no module, so there is no boundary inventory",
		state.FactModuleBoundaryAttribution: "the complete first-party graph was not attributed to declared modules or explicit outside-map results",
		state.FactModuleGraphShape:          "an applicable cohesion, hub, or encapsulation graph fact was not produced",
	}
	if modules == 0 {
		applyPromotion(&dim, nil, nil, reasons)
		return dim
	}
	withPublic, publicEntries := 0, 0
	for _, def := range p.Topology.Modules {
		if len(def.Public) > 0 {
			withPublic++
			publicEntries += len(def.Public)
		}
	}

	observed := []string{state.FactDeclaredModuleInventory}
	var notApplicable []string
	primaries := primaryDependencyInventory(diag, f)
	ce := diag.ClassifiedEdges
	switch {
	case primaries.known && primaries.applicable == 0:
		// Every language producer proved absence. Boundary attribution and
		// graph-shape metrics have producer-proved empty denominators.
		notApplicable = append(notApplicable, state.FactModuleBoundaryAttribution, state.FactModuleGraphShape)
	case primaries.complete():
		if ce != nil && ce.AttributedFirstPartyNodes == ce.FirstPartyNodes {
			observed = append(observed, state.FactModuleBoundaryAttribution)
		}
		if metricsProduced(diag.Metrics, "blast_radius", "encapsulation") {
			observed = append(observed, state.FactModuleGraphShape)
		}
	}
	applyPromotion(&dim, observed, notApplicable, reasons)
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
			Fact:   state.FactInferredPublicSurface,
			Reason: "no declared module states a public surface, so inferring one is outside this claim",
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
// The edge split is the denominator; the seam ledger is the unit a reader acts
// on. Both travel: forty imports expressing one seam is one thing to redesign,
// and reporting only the edge count reads as forty.
func couplingDimension(diag *result.Result) state.Dimension {
	dim := state.NewDimension(state.DimensionCoupling, state.OwnerCoupling)
	ce := diag.ClassifiedEdges
	reasons := map[string]string{
		state.FactCouplingCandidateInventory:       "no cross-boundary candidate was identified: the module map matched nothing, or every dependency left the declared modules",
		state.FactCouplingStrength:                 "integration strength is unknown on one or more cross-boundary candidates, so no rung is invented",
		state.FactCouplingDistance:                 "architectural distance is unknown on one or more cross-boundary candidates, so no rung is invented",
		state.FactCouplingVolatility:               "volatility is undeclared on one or more scored cross-boundary edges, so no rung is invented",
		state.FactExtractorResolutionWithinCeiling: "TypeScript imports or resolution inputs are incomplete; omitted edges may understate the candidate denominator",
	}
	tsPartial, roslyn := score.TSUnresolvedPartial(*diag), score.RoslynIncomplete(*diag)
	if roslyn {
		// Both producers may block the fact in a mixed repository: name each.
		const roslynReason = "roslyn left symbols unresolved or projects unloaded; an unresolved reference may hide a cross-boundary edge"
		if tsPartial {
			reasons[state.FactExtractorResolutionWithinCeiling] += "; " + roslynReason
		} else {
			reasons[state.FactExtractorResolutionWithinCeiling] = roslynReason
		}
	}
	if ce == nil || ce.Scored+ce.Abstained == 0 {
		applyPromotion(&dim, nil, nil, reasons)
		return dim
	}
	denominator := ce.Scored + ce.Abstained
	observed := []string{state.FactCouplingCandidateInventory}
	if ce.Abstained == 0 {
		observed = append(observed, state.FactCouplingStrength, state.FactCouplingDistance)
	}
	if ce.UnratedVolatilityEdges == 0 {
		observed = append(observed, state.FactCouplingVolatility)
	} else {
		reasons[state.FactCouplingVolatility] = unratedVolatilityReason(ce.UnratedVolatilityEdges, ce.UnratedVolatilityModules)
	}
	if !tsPartial && !roslyn {
		observed = append(observed, state.FactExtractorResolutionWithinCeiling)
	}
	applyPromotion(&dim, observed, nil, reasons)
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
			// Edges that pass the bc_score.v7 qualification (strong strength,
			// declared high volatility, module boundary, no cohesive role). A
			// diagnostic fact about edges, separate from the logical module
			// pairs counted in the seam ledger.
			count("qualifying_edges", tail.DistributedMonolithEdges, provRelationship))
	}
	dim.Metrics = append(dim.Metrics, seamMetrics(diag.Seams)...)
	dim.Metrics = append(dim.Metrics, metricValues(diag.Metrics, "unbalanced_edge")...)
	dim.Confidence = weakest(state.ConfidenceFor(dim.Status), metricConfidence(diag.Metrics, "unbalanced_edge"))
	return dim
}

// maxUnratedModulesNamed bounds the module list in the unrated-volatility
// reason so the text never grows with the module map.
const maxUnratedModulesNamed = 5

// unratedVolatilityReason names the modules whose undeclared volatility leaves
// scored edges unrated. The list is the relationship stage's sorted order.
func unratedVolatilityReason(edges int, modules []string) string {
	reason := fmt.Sprintf("volatility is undeclared on %d scored cross-boundary edge(s)", edges)
	if len(modules) == 0 {
		return reason + "; declare `volatility:` or `subdomain:` on the modules involved"
	}
	named := modules[:min(len(modules), maxUnratedModulesNamed)]
	list := strings.Join(named, ", ")
	if extra := len(modules) - len(named); extra > 0 {
		list += fmt.Sprintf(" and %d more", extra)
	}
	return reason + "; declare `volatility:` or `subdomain:` on: " + list
}

// seamMetrics reports the ledger's shape: how many logical seams the edges
// express, and how many of them sit in each interesting quadrant.
//
// The counts are seams, never edges. The two differ by an order of magnitude on
// a real repository, and reporting the edge count as the seam count is the
// weighting defect the ledger exists to fix (plan R4).
func seamMetrics(seams []result.Seam) []state.MetricValue {
	if len(seams) == 0 {
		return nil
	}
	var distributed, tight, unrated int
	for _, s := range seams {
		if s.DistributedMonolith {
			distributed++
		}
		if s.Quadrant == string(relationship.SeamQuadrantTight) {
			tight++
		}
		if s.Confidence == string(relationship.SeamConfidenceUnrated) {
			unrated++
		}
	}
	return []state.MetricValue{
		count("seams", len(seams), provRelationship),
		{Name: "distributed_monolith_seams", Value: float64(distributed), Unit: unitCount,
			Denominator: &state.MetricDenominator{Observed: len(seams) - unrated, Total: len(seams)},
			Provenance:  []string{provRelationship}},
		count("tight_seams", tight, provRelationship),
		count("unrated_seams", unrated, provRelationship),
	}
}

// changeLocalityDimension leaves co-change unmeasured: module touch counts
// cannot establish whether modules changed together.
func changeLocalityDimension() state.Dimension {
	dim := state.NewDimension(state.DimensionChangeLocality, state.OwnerChangeLocality)
	applyPromotion(&dim, nil, nil, map[string]string{
		state.FactEligibleCommitSample:    "co-change history is not collected",
		state.FactCommitModuleAttribution: "co-change module attribution is not collected",
	})
	return dim
}

// complexityDimension promotes only on the complete declared module graph.
// File and function size tails remain out-of-claim diagnostics: they may be
// absent while module-graph complexity is measured, and they never move status.
func complexityDimension(diag *result.Result, p policy.PolicySnapshot, f Observations) state.Dimension {
	dim := state.NewDimension(state.DimensionComplexity, state.OwnerComplexity)
	reasons := map[string]string{
		state.FactDeclaredModuleGraph:      "no declared module graph was assembled over the production-source module set",
		state.FactDependencyChainDepth:     "dependency-chain depth is incomplete because the module dependency graph is incomplete",
		state.FactModuleFanInDistribution:  "the module fan-in distribution is incomplete because the module dependency graph is incomplete",
		state.FactModuleFanOutDistribution: "the module fan-out distribution is incomplete because the module dependency graph is incomplete",
	}

	productionModules := make(map[string]struct{})
	productionFiles, productionLOC, largest := 0, 0, 0
	for file, loc := range f.FileLOC {
		if !fileclass.IsProduction(classOf(f.FileClassIndex, file)) {
			continue
		}
		productionFiles++
		productionLOC += loc
		largest = max(largest, loc)
		if module, ok := p.Topology.ModuleMap.ModuleForFile(file); ok {
			productionModules[module] = struct{}{}
		}
	}
	if len(productionModules) == 0 {
		reasons[state.FactDeclaredModuleGraph] = "no declared module contains production source, so architecture-level complexity does not apply"
		applyPromotion(&dim, nil, nil, reasons)
		appendComplexityDiagnostics(&dim, f, p.Assessment.FunctionLOCThreshold, productionFiles, productionLOC, largest)
		return dim
	}

	observed := make([]string, 0, 4)
	graph := diag.ModuleGraphComplexity
	if graph != nil && graph.Modules == len(p.Topology.Modules) {
		// The declared module denominator is known even when one dependency
		// producer is incomplete. That makes the result partial rather than
		// letting out-of-claim syntax facts promote an absent graph.
		observed = append(observed, state.FactDeclaredModuleGraph)
	}
	primaries := primaryDependencyInventory(diag, f)
	edges := diag.ClassifiedEdges
	graphComplete := graph != nil && primaries.complete() && edges != nil &&
		edges.ClassifiedInternalDependencies == edges.InternalDependencies &&
		edges.AttributedFirstPartyNodes == edges.FirstPartyNodes
	if graphComplete {
		observed = append(observed,
			state.FactDependencyChainDepth,
			state.FactModuleFanInDistribution,
			state.FactModuleFanOutDistribution,
		)
	}
	applyPromotion(&dim, observed, nil, reasons)

	totalModules := len(p.Topology.Modules)
	observedModules := 0
	if graphComplete {
		observedModules = totalModules
	}
	dim.Coverage = state.Coverage{
		Basis:    "declared modules with complete dependency-chain and degree values",
		Observed: observedModules, Total: totalModules,
	}
	if graphComplete {
		denominator := &state.MetricDenominator{Observed: totalModules, Total: totalModules}
		dim.Metrics = append(dim.Metrics,
			state.MetricValue{Name: "max_dependency_chain", Value: float64(graph.MaxDependencyChain), Unit: unitCount, Denominator: denominator, Provenance: []string{provRelationship}},
			state.MetricValue{Name: "module_fan_in_p90", Value: float64(graph.FanInP90), Unit: unitCount, Denominator: denominator, Provenance: []string{provRelationship}},
			state.MetricValue{Name: "module_fan_out_p90", Value: float64(graph.FanOutP90), Unit: unitCount, Denominator: denominator, Provenance: []string{provRelationship}},
		)
	}
	appendComplexityDiagnostics(&dim, f, p.Assessment.FunctionLOCThreshold, productionFiles, productionLOC, largest)
	dim.Confidence = state.ConfidenceFor(dim.Status)
	return dim
}

func appendComplexityDiagnostics(dim *state.Dimension, f Observations, threshold, productionFiles, productionLOC, largest int) {
	if len(f.FileLOC) > 0 {
		dim.Metrics = append(dim.Metrics,
			count("production_files", productionFiles, provFileClass),
			count("production_loc", productionLOC, provFileClass),
			count("largest_production_file_loc", largest, provFileClass),
		)
	} else {
		dim.Unknown = append(dim.Unknown, state.UnknownFact{
			Fact: state.FactCodeSizeTail, Reason: "the source walk produced no file, so the out-of-claim file-size tail is unavailable", Owner: state.OwnerComplexitySize,
		})
	}

	functions := functionLengthDistribution(f.SyntaxFacts, threshold)
	if functions.Observed > 0 {
		denominator := &state.MetricDenominator{Observed: functions.Observed, Total: functions.Total}
		dim.Metrics = append(dim.Metrics,
			state.MetricValue{Name: "function_loc_p50", Value: float64(functions.P50), Unit: unitCount, Denominator: denominator, Provenance: []string{provAcquisition}},
			state.MetricValue{Name: "function_loc_p90", Value: float64(functions.P90), Unit: unitCount, Denominator: denominator, Provenance: []string{provAcquisition}},
			state.MetricValue{Name: "function_loc_max", Value: float64(functions.Max), Unit: unitCount, Denominator: denominator, Provenance: []string{provAcquisition}},
			state.MetricValue{Name: "functions_over_threshold", Value: float64(functions.OverThreshold), Unit: unitCount, Denominator: denominator, Provenance: []string{provAcquisition}},
		)
	}
	if functions.Observed < functions.Total || functions.Total == 0 {
		dim.Unknown = append(dim.Unknown, state.UnknownFact{
			Fact: state.FactFunctionLengthDistribution, Reason: "ast-grep supplied no complete function or method extent for part or all of the out-of-claim size distribution", Owner: state.OwnerComplexitySize,
		})
	}
	dim.Unknown = append(dim.Unknown, state.UnknownFact{
		Fact: state.FactCognitiveComplexity, Reason: "no cognitive-complexity analyzer is claimed; module-graph shape is the architecture-level measure", Owner: state.OwnerComplexitySize,
	})
}

// testabilityDimension reports exercised production-code coverage attributed to
// the declared module map. Supplied coverage is report input, never a reason to
// execute the target repository's tests. The static-only branch keeps coverage
// metrics absent when coverage is disabled while using the same required-fact
// promotion contract.
func testabilityDimension(p policy.PolicySnapshot, f Observations) state.Dimension {
	if f.SuppliedCoverage == nil {
		return legacyTestabilityDimension(f)
	}

	dim := state.NewDimension(state.DimensionTestability, state.OwnerTestability)
	if len(f.FileClassIndex) == 0 {
		applyPromotion(&dim, nil, nil, map[string]string{
			state.FactProductionSourceInventory: "no file was classified, so test and production files cannot be told apart",
			state.FactSuppliedCoverageUnits:     "without a classified source inventory, supplied coverage units cannot be interpreted",
			state.FactCoveragePathResolution:    "without a classified source inventory, coverage paths cannot be resolved against supported source",
			state.FactCoverageModuleAttribution: "without a classified source inventory, coverage cannot be attributed to declared modules",
			state.FactCoverageFreshness:         "without a classified source inventory, freshness cannot establish applicable source evidence",
		})
		return dim
	}
	tests, production := testAndProductionFileCounts(f.FileClassIndex)
	staticMetrics := staticTestabilityMetrics(tests, production)
	if production == 0 {
		dim.Metrics = staticMetrics
		return unmeasured(dim, state.UnknownFact{
			Fact:   state.FactProductionSourceInventory,
			Reason: "no supported production source was classified, so exercised production code is not applicable",
			Owner:  state.OwnerTestability,
		})
	}

	summary := summarizeSuppliedCoverage(p.Topology, f)
	reasons := map[string]string{
		state.FactProductionSourceInventory: "no supported production source was classified",
		state.FactSuppliedCoverageUnits:     summary.unitsReason,
		state.FactCoveragePathResolution:    summary.pathReason,
		state.FactCoverageModuleAttribution: summary.attributionReason,
		state.FactCoverageFreshness:         summary.freshnessReason,
	}
	observed := []string{state.FactProductionSourceInventory}
	if summary.unitsComplete {
		observed = append(observed, state.FactSuppliedCoverageUnits)
	}
	if summary.unresolvedPaths == 0 {
		observed = append(observed, state.FactCoveragePathResolution)
	}
	if summary.attributionComplete {
		observed = append(observed, state.FactCoverageModuleAttribution)
	}
	if summary.freshnessMatched {
		observed = append(observed, state.FactCoverageFreshness)
	}
	applyPromotion(&dim, observed, nil, reasons)
	dim.Coverage = state.Coverage{
		Basis:    "declared modules represented by attributed supplied coverage",
		Observed: len(summary.coveredModules), Total: len(p.Topology.Modules),
	}
	dim.Metrics = staticMetrics
	dim.Metrics = append(dim.Metrics, summary.metrics()...)
	dim.Unknown = append(dim.Unknown,
		state.UnknownFact{
			Fact: state.FactAssertionQuality, Reason: "exercised instrumentation points do not establish assertion strength or mutation resistance", Owner: state.OwnerTestability,
		},
		state.UnknownFact{
			Fact: state.FactBoundaryTestSemantics, Reason: "coverage does not identify which declared module boundaries a test meaningfully exercises", Owner: state.OwnerTestability,
		},
	)
	return dim
}

func legacyTestabilityDimension(f Observations) state.Dimension {
	dim := state.NewDimension(state.DimensionTestability, state.OwnerTestability)
	reasons := map[string]string{
		state.FactProductionSourceInventory: "no file was classified, so test and production files cannot be told apart",
		state.FactSuppliedCoverageUnits:     "coverage is disabled, so no supplied coverage units were observed",
		state.FactCoveragePathResolution:    "coverage is disabled, so no supplied coverage paths were available to resolve",
		state.FactCoverageModuleAttribution: "coverage is disabled, so no supplied coverage was available to attribute to declared modules",
		state.FactCoverageFreshness:         "coverage is disabled, so no supplied coverage freshness could be established",
	}
	if len(f.FileClassIndex) == 0 {
		applyPromotion(&dim, nil, nil, reasons)
		return dim
	}
	tests, production := testAndProductionFileCounts(f.FileClassIndex)
	observed := []string(nil)
	if production > 0 {
		observed = append(observed, state.FactProductionSourceInventory)
	}
	applyPromotion(&dim, observed, nil, reasons)
	dim.Coverage = state.Coverage{Basis: "classified source files", Observed: tests + production, Total: len(f.FileClassIndex)}
	dim.Metrics = staticTestabilityMetrics(tests, production)
	return dim
}

func testAndProductionFileCounts(index map[string]fileclass.FileClass) (tests, production int) {
	for _, class := range index {
		switch {
		case class == fileclass.Test:
			tests++
		case fileclass.IsProduction(class):
			production++
		}
	}
	return tests, production
}

func staticTestabilityMetrics(tests, production int) []state.MetricValue {
	return []state.MetricValue{
		count("test_files", tests, provFileClass),
		count("production_files", production, provFileClass),
		ratio("test_to_production_files", tests, production, provFileClass),
	}
}

type suppliedCoverageSummary struct {
	coveredUnits, totalUnits int
	unit                     string
	declaredModules          int
	unresolvedPaths          int
	mergedFacts              int
	coveredModules           map[string]struct{}
	provenance               []string
	unitsComplete            bool
	attributionComplete      bool
	freshnessMatched         bool
	unitsReason              string
	pathReason               string
	attributionReason        string
	freshnessReason          string
}

type suppliedCoverageAccumulator struct {
	summary          suppliedCoverageSummary
	factsByFile      map[string]modevidence.CoverageFact
	unattributed     map[string]struct{}
	provenance       map[string]struct{}
	freshnessReasons map[string]struct{}
	factsComplete    bool
	compatible       bool
	unit             string
}

func summarizeSuppliedCoverage(topology policy.TopologyView, f Observations) suppliedCoverageSummary {
	acc := suppliedCoverageAccumulator{
		summary: suppliedCoverageSummary{
			declaredModules: len(topology.Modules), coveredModules: map[string]struct{}{},
			freshnessMatched: len(f.SuppliedCoverage) > 0,
		},
		factsByFile:  make(map[string]modevidence.CoverageFact),
		unattributed: make(map[string]struct{}), provenance: make(map[string]struct{}),
		freshnessReasons: make(map[string]struct{}), factsComplete: len(f.SuppliedCoverage) > 0, compatible: true,
	}
	for _, ingest := range f.SuppliedCoverage {
		acc.add(ingest, topology, f.FileClassIndex)
	}
	return acc.finish(topology)
}

func (a *suppliedCoverageAccumulator) add(ingest modevidence.CoverageIngest, topology policy.TopologyView, classes map[string]fileclass.FileClass) {
	a.summary.unresolvedPaths += ingest.UnresolvedPaths
	addCoverageProvenance(a.provenance, ingest)
	if ingest.Freshness != modevidence.FreshnessMatched {
		a.summary.freshnessMatched = false
		reason := ingest.Reason
		if reason == "" {
			if ingest.Freshness == modevidence.FreshnessStale {
				reason = "worktree_differs_from_ref"
			} else {
				reason = "freshness_unverified"
			}
		}
		a.freshnessReasons[reason] = struct{}{}
	}
	if len(ingest.Facts) == 0 || coverageIngestHasRejectedFacts(ingest.Reason) {
		a.factsComplete = false
	}
	for _, fact := range ingest.Facts {
		a.addFact(fact, topology, classes)
	}
}

func (a *suppliedCoverageAccumulator) addFact(fact modevidence.CoverageFact, topology policy.TopologyView, classes map[string]fileclass.FileClass) {
	class, classified := classes[fact.File]
	if !classified {
		a.unattributed[fact.File] = struct{}{}
		return
	}
	if !fileclass.IsProduction(class) {
		return
	}
	module, attributed := topology.ModuleMap.ModuleForFile(fact.File)
	if !attributed {
		a.unattributed[fact.File] = struct{}{}
		return
	}
	a.summary.coveredModules[module] = struct{}{}
	if a.unit == "" {
		a.unit = fact.Unit
	} else if fact.Unit != a.unit {
		a.compatible = false
	}
	if previous, duplicate := a.factsByFile[fact.File]; duplicate {
		if previous.Unit != fact.Unit || previous.TotalUnits != fact.TotalUnits {
			a.compatible = false
			return
		}
		a.summary.mergedFacts++
		if fact.CoveredUnits > previous.CoveredUnits {
			a.factsByFile[fact.File] = fact
		}
		return
	}
	a.factsByFile[fact.File] = fact
}

func (a *suppliedCoverageAccumulator) finish(topology policy.TopologyView) suppliedCoverageSummary {
	a.summary.unit = a.unit
	if a.compatible {
		for _, fact := range a.factsByFile {
			a.summary.coveredUnits += fact.CoveredUnits
			a.summary.totalUnits += fact.TotalUnits
		}
	}
	a.summary.unitsComplete = a.factsComplete && a.compatible && len(a.factsByFile) > 0
	a.summary.unitsReason = "no complete, compatible supplied coverage-unit denominator was observed"
	if !a.compatible {
		a.summary.unitsReason = "supplied coverage facts use incompatible units or disagree about a file denominator"
	}
	a.summary.pathReason = "one or more supplied coverage paths could not be resolved inside the scan root"
	a.finishAttribution(topology)
	a.summary.freshnessReason = "coverage freshness was not matched to the scanned source bytes"
	if reasons := sortedStringSet(a.freshnessReasons); len(reasons) > 0 {
		a.summary.freshnessReason = strings.Join(reasons, "; ")
	}
	a.summary.provenance = sortedStringSet(a.provenance)
	return a.summary
}

func (a *suppliedCoverageAccumulator) finishAttribution(topology policy.TopologyView) {
	missingModules := make([]string, 0)
	for module := range topology.Modules {
		if _, covered := a.summary.coveredModules[module]; !covered {
			missingModules = append(missingModules, module)
		}
	}
	sort.Strings(missingModules)
	unattributedPaths := sortedStringSet(a.unattributed)
	a.summary.attributionComplete = len(topology.Modules) > 0 && len(missingModules) == 0 && len(unattributedPaths) == 0
	switch {
	case len(topology.Modules) == 0:
		a.summary.attributionReason = "the configuration declares no module, so coverage has no module denominator"
	case len(missingModules) > 0 && len(unattributedPaths) > 0:
		a.summary.attributionReason = "missing coverage for declared modules: " + strings.Join(missingModules, ", ") + "; unattributed coverage paths: " + strings.Join(unattributedPaths, ", ")
	case len(missingModules) > 0:
		a.summary.attributionReason = "missing coverage for declared modules: " + strings.Join(missingModules, ", ")
	case len(unattributedPaths) > 0:
		a.summary.attributionReason = "unattributed coverage paths: " + strings.Join(unattributedPaths, ", ")
	default:
		a.summary.attributionReason = "coverage module attribution is incomplete"
	}
}

func coverageIngestHasRejectedFacts(reason string) bool {
	for _, marker := range []string{
		"coverage_source_unavailable", "coverage_parser_unavailable", "coverage_parse_failed",
		"coverage_facts_empty", "invalid_coverage_fact",
	} {
		if strings.Contains(reason, marker) {
			return true
		}
	}
	return false
}

func addCoverageProvenance(provenance map[string]struct{}, ingest modevidence.CoverageIngest) {
	format, version := ingest.Format, ingest.ToolVersion
	if format == "" {
		format = "unknown"
	}
	if version == "" {
		version = "unknown"
	}
	provenance["extract/coverage/"+format+"@"+version] = struct{}{}
	freshness := ingest.Freshness
	if freshness == "" {
		freshness = modevidence.FreshnessUnverified
	}
	provenance["coverage/freshness/"+string(freshness)] = struct{}{}
}

func sortedStringSet(values map[string]struct{}) []string {
	out := make([]string, 0, len(values))
	for value := range values {
		out = append(out, value)
	}
	sort.Strings(out)
	return out
}

func (s suppliedCoverageSummary) metrics() []state.MetricValue {
	provenance := append([]string(nil), s.provenance...)
	metrics := make([]state.MetricValue, 0, 6)
	if s.unitsComplete {
		denominator := &state.MetricDenominator{Observed: s.coveredUnits, Total: s.totalUnits}
		metrics = append(metrics,
			state.MetricValue{Name: "covered_units", Value: float64(s.coveredUnits), Unit: s.unit, Denominator: denominator, Provenance: append([]string(nil), provenance...)},
			state.MetricValue{Name: "total_units", Value: float64(s.totalUnits), Unit: s.unit, Denominator: denominator, Provenance: append([]string(nil), provenance...)},
			state.MetricValue{Name: "coverage_ratio", Value: coverageRatio(s.coveredUnits, s.totalUnits), Unit: unitRatio, Denominator: denominator, Provenance: append([]string(nil), provenance...)},
		)
	}
	moduleProvenance := append([]string{provPolicy}, provenance...)
	metrics = append(metrics,
		state.MetricValue{Name: "modules_with_coverage", Value: float64(len(s.coveredModules)), Unit: unitCount,
			Denominator: &state.MetricDenominator{Observed: len(s.coveredModules), Total: s.declaredModules},
			Provenance:  moduleProvenance},
		state.MetricValue{Name: "unresolved_coverage_paths", Value: float64(s.unresolvedPaths), Unit: unitCount, Provenance: append([]string(nil), provenance...)},
	)
	if s.mergedFacts > 0 {
		metrics = append(metrics, state.MetricValue{Name: "merged_coverage_facts", Value: float64(s.mergedFacts), Unit: unitCount, Provenance: provenance})
	}
	return metrics
}

func coverageRatio(covered, total int) float64 {
	if total == 0 {
		return 0
	}
	return float64(covered) / float64(total)
}

// operationsDimension measures declared operational-topology completeness.
// Repository manifests corroborate declarations but never claim that a unit is
// running. Analyzer health remains an out-of-claim disclosure and cannot move
// this status or confidence.
func operationsDimension(diag *result.Result, p policy.PolicySnapshot, f Observations) state.Dimension {
	dim := state.NewDimension(state.DimensionOperations, state.OwnerOperations)
	modules := len(p.Topology.Modules)
	reasons := map[string]string{
		state.FactDeclaredOperationalTopology: "the configuration declares no module, so there is no operational-topology denominator",
		state.FactCorroboratedDeployUnit:      "one or more declared modules have no independently corroborating deploy manifest",
		state.FactOwnerProvenance:             "one or more declared modules lack a declared or CODEOWNERS-backed owner; git-author fallback is not an ownership statement",
		state.FactTopologyReconciliation:      "declared and corroborated deploy-unit facts did not both reach assessment for reconciliation",
	}
	if modules == 0 {
		applyPromotion(&dim, nil, nil, reasons)
		return dim
	}

	withOwner, distinctOwners := resolvedOwnerCounts(p.Topology.Modules)
	declaredDeployUnits := declaredDeployUnitCount(p.Topology.Modules, f.DeclaredDeployUnits)
	corroboratedModules, corroboratedUnits := corroboratedDeployUnitCounts(p.Topology.Modules, f.CorroboratedDeployUnits)
	qualifyingOwners, ownersFromDeclared, ownersFromCodeowners, ownersFromGitAuthor := ownerProvenanceCounts(p.Topology.Modules, f.OwnerProvenance)
	completeModules, matchingUnits, mismatchedUnits := reconcileOperationalTopology(
		p.Topology.Modules, f.DeclaredDeployUnits, corroboratedModules, qualifyingOwners,
	)
	discoveryComplete, discoveryReason := deployDiscoveryComplete(diag.ToolCoverage)
	if !discoveryComplete {
		reasons[state.FactCorroboratedDeployUnit] = discoveryReason
		reasons[state.FactTopologyReconciliation] = discoveryReason
	}

	observed := []string{state.FactDeclaredOperationalTopology}
	if discoveryComplete && len(corroboratedModules) == modules {
		observed = append(observed, state.FactCorroboratedDeployUnit)
	}
	if len(qualifyingOwners) == modules {
		observed = append(observed, state.FactOwnerProvenance)
	}
	if discoveryComplete && f.DeclaredDeployUnits != nil && f.CorroboratedDeployUnits != nil {
		observed = append(observed, state.FactTopologyReconciliation)
	}
	applyPromotion(&dim, observed, nil, reasons)
	dim.Coverage = state.Coverage{
		Basis:    "declared modules with a corroborated deploy unit and qualifying owner",
		Observed: completeModules, Total: modules,
	}

	// An analyzer only belongs in its own denominator when it was applicable.
	// These metrics disclose tool health but are outside the declared-topology
	// claim and therefore do not participate in promotion or confidence.
	reporting, applicable := analyzerCoverageCounts(diag.ToolCoverage)
	dim.Metrics = []state.MetricValue{
		{Name: "modules_with_owner", Value: float64(withOwner), Unit: unitCount,
			Denominator: &state.MetricDenominator{Observed: withOwner, Total: modules},
			Provenance:  []string{provPolicy, provAcquisition}},
		{Name: "distinct_owners", Value: float64(distinctOwners), Unit: unitCount,
			Provenance: []string{provPolicy, provAcquisition}},
		count("owners_from_declared", ownersFromDeclared, provPolicy),
		count("owners_from_codeowners", ownersFromCodeowners, provAcquisition),
		count("owners_from_git_author_fallback", ownersFromGitAuthor, provAcquisition),
		count("declared_deploy_units", declaredDeployUnits, provPolicy),
		count("corroborated_deploy_units", corroboratedUnits, provAcquisition),
		{Name: "modules_with_corroborated_deploy_unit", Value: float64(len(corroboratedModules)), Unit: unitCount,
			Denominator: &state.MetricDenominator{Observed: len(corroboratedModules), Total: modules},
			Provenance:  []string{provAcquisition}},
		{Name: "matching_declared_deploy_units", Value: float64(matchingUnits), Unit: unitCount,
			Provenance: []string{provPolicy, provAcquisition}},
		{Name: "mismatched_declared_deploy_units", Value: float64(mismatchedUnits), Unit: unitCount,
			Provenance: []string{provPolicy, provAcquisition}},
		count("declared_external_systems", len(p.Topology.ExternalSystems), provPolicy),
		{Name: "analyzers_reporting_coverage", Value: float64(reporting), Unit: unitCount,
			Denominator: &state.MetricDenominator{Observed: reporting, Total: applicable},
			Provenance:  []string{provAcquisition}},
		count("coverage_gaps", len(diag.CoverageGaps), provAcquisition),
		count("analyzers_not_applicable", len(diag.ToolCoverage)-applicable, provAcquisition),
	}
	dim.Metrics = append(dim.Metrics, metricValues(diag.Metrics, "coverage")...)
	dim.Confidence = state.ConfidenceFor(dim.Status)
	dim.Unknown = append(dim.Unknown, state.UnknownFact{
		Fact:   state.FactRuntimeTopology,
		Reason: "committed manifests corroborate declared deploy units; they do not observe what is actually running",
		Owner:  state.OwnerOperations,
	}, state.UnknownFact{
		Fact:   state.FactSupplyChainInventory,
		Reason: "SBOM and vulnerability facts are a separate report family and have no collector in v1",
		Owner:  state.OwnerOperations,
	})
	for _, gap := range diag.CoverageGaps {
		dim.Unknown = append(dim.Unknown, state.UnknownFact{
			Fact:   state.FactAnalyzerHealth,
			Reason: "analyzer " + gap.Tool + " did not run over this tree (gate: " + gap.Gate + "); install it with: " + gap.InstallCmd,
			Owner:  state.OwnerOperations,
		})
	}
	return dim
}

func deployDiscoveryComplete(rows []modevidence.Coverage) (bool, string) {
	discovery := coverageRows(rows, "deploy-unit")
	if len(discovery) == 1 && discovery[0].Status == modevidence.StatusOK {
		return true, ""
	}
	reasons := []string{"deploy-unit discovery did not complete, so operational topology cannot be fully reconciled"}
	for _, row := range discovery {
		if row.Reason != "" {
			reasons = append(reasons, row.Reason)
		}
	}
	return false, strings.Join(reasons, ": ")
}

func resolvedOwnerCounts(modules map[string]policy.ModuleDef) (withOwner, distinct int) {
	owners := make(map[string]struct{})
	for _, def := range modules {
		if def.Owner == "" {
			continue
		}
		owners[def.Owner] = struct{}{}
		withOwner++
	}
	return withOwner, len(owners)
}

func declaredDeployUnitCount(modules map[string]policy.ModuleDef, declaredUnits map[string]string) int {
	count := 0
	for module, unit := range declaredUnits {
		if _, declared := modules[module]; declared && unit != "" {
			count++
		}
	}
	return count
}

func corroboratedDeployUnitCounts(
	modules map[string]policy.ModuleDef,
	facts map[string]modevidence.CorroboratedDeployUnit,
) (map[string]modevidence.CorroboratedDeployUnit, int) {
	corroborated := make(map[string]modevidence.CorroboratedDeployUnit)
	units := make(map[string]struct{})
	for module, fact := range facts {
		if _, declared := modules[module]; !declared || fact.Unit == "" || !corroboratingDeploySource(fact.Source) {
			continue
		}
		corroborated[module] = fact
		units[fact.Unit] = struct{}{}
	}
	return corroborated, len(units)
}

func ownerProvenanceCounts(
	modules map[string]policy.ModuleDef,
	facts map[string]modevidence.OwnerProvenance,
) (map[string]struct{}, int, int, int) {
	qualifying := make(map[string]struct{})
	declaredCount, codeownersCount, gitAuthorCount := 0, 0, 0
	for module, fact := range facts {
		def, declared := modules[module]
		if !declared || fact.Owner == "" || fact.Owner != def.Owner {
			continue
		}
		switch fact.Source {
		case modevidence.TopologySourceDeclared:
			declaredCount++
			qualifying[module] = struct{}{}
		case modevidence.TopologySourceCodeowners:
			codeownersCount++
			qualifying[module] = struct{}{}
		case modevidence.TopologySourceGitAuthor:
			gitAuthorCount++
		}
	}
	return qualifying, declaredCount, codeownersCount, gitAuthorCount
}

func reconcileOperationalTopology(
	modules map[string]policy.ModuleDef,
	declaredUnits map[string]string,
	corroborated map[string]modevidence.CorroboratedDeployUnit,
	qualifyingOwners map[string]struct{},
) (complete, matching, mismatched int) {
	for module := range modules {
		_, hasCorroboration := corroborated[module]
		_, hasQualifyingOwner := qualifyingOwners[module]
		if hasCorroboration && hasQualifyingOwner {
			complete++
		}
		declaredUnit, hasDeclaration := declaredUnits[module]
		corroboratedFact, hasCorroboration := corroborated[module]
		if !hasDeclaration || !hasCorroboration {
			continue
		}
		if declaredUnit == corroboratedFact.Unit {
			matching++
		} else {
			mismatched++
		}
	}
	return complete, matching, mismatched
}

func analyzerCoverageCounts(rows []modevidence.Coverage) (reporting, applicable int) {
	for _, row := range rows {
		if row.Status == modevidence.StatusAbsent || row.Status == modevidence.StatusDisabled {
			continue
		}
		applicable++
		if row.Status == modevidence.StatusOK || row.Status == modevidence.StatusPartial {
			reporting++
		}
	}
	return reporting, applicable
}

func corroboratingDeploySource(source modevidence.TopologySource) bool {
	switch source {
	case modevidence.TopologySourceDockerfile, modevidence.TopologySourceK8s,
		modevidence.TopologySourceWorkspace, modevidence.TopologySourcePyproject,
		modevidence.TopologySourceGoMain:
		return true
	default:
		return false
	}
}

// driftDimension cannot measure historical seam drift without a stored input.
func driftDimension() state.Dimension {
	dim := state.NewDimension(state.DimensionDrift, state.OwnerDrift)
	applyPromotion(&dim, nil, nil, map[string]string{
		state.FactAdmissiblePersistedReference: "stored architecture-state references are no longer supported",
		state.FactCompleteTwoSidedSeamIdentity: "two-sided seam identity is not available in a report measurement",
	})
	return dim
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

// classOf resolves a file's class, defaulting to production when the walk did
// not classify it — the same fallback the metric filters use.
func classOf(index map[string]fileclass.FileClass, file string) fileclass.FileClass {
	if class, ok := index[file]; ok {
		return class
	}
	return fileclass.Production
}
