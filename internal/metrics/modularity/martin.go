package modularity

import (
	"fmt"
	"math"
	"sort"
	"strings"

	"github.com/alexei-led/archfit/internal/metrics/internal/modgraph"
	"github.com/alexei-led/archfit/internal/metrics/internal/result"
	"github.com/alexei-led/archfit/internal/model/coupling"
	"github.com/alexei-led/archfit/internal/model/diagnostic"
	"github.com/alexei-led/archfit/internal/model/graph"
	"github.com/alexei-led/archfit/internal/model/signal"
)

// computeAfferentEfferent computes per-module afferent (Ca) and efferent (Ce)
// coupling counts over first-party modules only. Self-loops are excluded.
// ca[m] = number of distinct first-party modules that import m.
// ce[m] = number of distinct first-party modules that m imports.
func computeAfferentEfferent(g *graph.Graph) (ca, ce map[string]int, firstParty map[string]struct{}) {
	firstParty = make(map[string]struct{})
	for _, n := range g.Nodes() {
		firstParty[modgraph.ModuleKey(n.ID())] = struct{}{}
	}
	ca = make(map[string]int, len(firstParty))
	ce = make(map[string]int, len(firstParty))
	for m := range firstParty {
		ca[m] = 0
		ce[m] = 0
	}

	// Track distinct pairs to avoid double-counting multi-edge graphs.
	ceEdges := make(map[[2]string]struct{})
	caEdges := make(map[[2]string]struct{})
	for _, e := range g.Edges() {
		from, to := modgraph.ModuleKey(e.From), modgraph.ModuleKey(e.To)
		if from == to {
			continue
		}
		_, fromFirst := firstParty[from]
		_, toFirst := firstParty[to]
		if fromFirst && toFirst {
			if _, seen := ceEdges[[2]string{from, to}]; !seen {
				ceEdges[[2]string{from, to}] = struct{}{}
				ce[from]++
			}
			if _, seen := caEdges[[2]string{to, from}]; !seen {
				caEdges[[2]string{to, from}] = struct{}{}
				ca[to]++
			}
		}
	}
	return ca, ce, firstParty
}

// ComputeInstability returns per-module instability I=Ce/(Ca+Ce) from the graph.
// Returns nil when graph is nil. First-party modules only (same as BlastRadius).
func ComputeInstability(g *graph.Graph) map[string]float64 {
	if g == nil {
		return nil
	}
	ca, ce, firstParty := computeAfferentEfferent(g)
	inst := make(map[string]float64, len(firstParty))
	for m := range firstParty {
		total := ca[m] + ce[m]
		if total == 0 {
			inst[m] = 0 // isolated module: stable by convention (no dependents, no dependencies)
		} else {
			inst[m] = float64(ce[m]) / float64(total)
		}
	}
	return inst
}

// UnstableDependency reports whether the edge from→to is an unstable dependency
// (Designite smell): I(to) > I(from), meaning the caller depends on a module that
// is itself more dependent on others than it is depended upon.
// instability must be precomputed by ComputeInstability.
// Returns false when either module is absent from the instability map (unknown → conservative).
func UnstableDependency(from, to string, instability map[string]float64) bool {
	iFrom, okFrom := instability[from]
	iTo, okTo := instability[to]
	if !okFrom || !okTo {
		return false
	}
	return iTo > iFrom
}

// ---------------------------------------------------------------------------
// InstabilityMetric
// ---------------------------------------------------------------------------

// InstabilityMetric reports per-module instability I=Ce/(Ca+Ce): the fraction
// of dependencies that are outgoing. High instability means the module depends
// on many others and is depended upon by few — common for leaf/glue modules.
// Report-only (band "info"); high I is not inherently bad, but pairing with
// volatile targets (see change_amplification) is a risk.
//
// Shared-DTO trap: high Ca + low I is NOT automatically good — a widely-imported
// shared-DTO module can become a change magnet if its API is volatile. Volatility
// and strength (see abstractness) must moderate the reading. These metrics are
// report-only for precisely this reason.
type InstabilityMetric struct{}

// Name returns "instability".
func (m InstabilityMetric) Name() string { return "instability" }

// Version returns "instability.v1".
func (m InstabilityMetric) Version() string { return "instability.v1" }

const instabilityDef = "per-module instability I=Ce/(Ca+Ce): fraction of dependencies that are outgoing; " +
	"high I = depends on many, depended on by few"

// Calculate computes per-module instability and reports modules with I>0.7.
func (m InstabilityMetric) Calculate(in signal.CommonInput) diagnostic.MetricResult {
	if in.Graph == nil {
		return m.naResult()
	}
	inst := ComputeInstability(in.Graph)
	n := len(inst)
	if n < 2 {
		return m.naResult()
	}

	type entry struct {
		module string
		i      float64
	}
	var unstable []entry
	for mod, i := range inst {
		if i > 0.7 {
			unstable = append(unstable, entry{mod, i})
		}
	}
	sort.Slice(unstable, func(a, b int) bool {
		if unstable[a].i != unstable[b].i {
			return unstable[a].i > unstable[b].i
		}
		return unstable[a].module < unstable[b].module
	})

	confidence := result.ConfidenceHigh
	if n < result.ModularitySmallN {
		confidence = result.ConfidenceLow
	}

	var display string
	if len(unstable) == 0 {
		display = "0 modules with I>0.7"
	} else {
		var sb strings.Builder
		fmt.Fprintf(&sb, "%d unstable modules (I>0.7): ", len(unstable))
		for i, e := range unstable {
			if i == 5 {
				fmt.Fprintf(&sb, "+%d more", len(unstable)-5)
				break
			}
			if i > 0 {
				sb.WriteString(", ")
			}
			fmt.Fprintf(&sb, "%s (I=%.2f)", result.ShortModule(e.module), e.i)
		}
		display = sb.String()
	}

	return diagnostic.MetricResult{
		Name:       m.Name(),
		Value:      float64(len(unstable)),
		Display:    display,
		Band:       result.BandInformational,
		Confidence: confidence,
		Version:    m.Version(),
		Mode:       result.ModeRatio,
		Definition: instabilityDef,
	}
}

func (m InstabilityMetric) naResult() diagnostic.MetricResult {
	return diagnostic.MetricResult{
		Name: m.Name(), Value: 0, Display: result.BandNA, Band: result.BandNA,
		Confidence: result.ConfidenceLow, Version: m.Version(), Mode: result.ModeRatio,
		Definition: instabilityDef,
	}
}

// ---------------------------------------------------------------------------
// AbstractnessMetric
// ---------------------------------------------------------------------------

// AbstractnessMetric reports per-module abstractness A = contract_inbound /
// (contract_inbound + concrete_inbound). This is a proxy derived from coupling
// classifications (strength labels assigned by the classify step) rather than
// SCIP type-kind data; it approximates Martin's A without requiring a full symbol
// index. A module with zero classified inbound edges gets A=0 (entirely concrete
// is the conservative default when evidence is absent).
//
// Beyond Balanced Coupling, report-only.
type AbstractnessMetric struct{}

// Name returns "abstractness".
func (m AbstractnessMetric) Name() string { return "abstractness" }

// Version returns "abstractness.v1".
func (m AbstractnessMetric) Version() string { return "abstractness.v1" }

const abstractnessDef = "per-module abstractness A=contract_inbound/(contract+concrete_inbound): " +
	"fraction of inbound edges that are contract-strength; beyond Balanced Coupling, report-only"

// Calculate computes per-module abstractness from the coupling classification index.
func (m AbstractnessMetric) Calculate(in signal.CommonInput) diagnostic.MetricResult {
	if in.Graph == nil {
		return m.naResult()
	}

	// Build the first-party set from the graph.
	firstParty := make(map[string]struct{})
	for _, n := range in.Graph.Nodes() {
		firstParty[modgraph.ModuleKey(n.ID())] = struct{}{}
	}
	n := len(firstParty)

	// Per-module inbound edge counts by strength class.
	contractIn := make(map[string]int, n)
	concreteIn := make(map[string]int, n)

	// The coupling.Index key is "from\x00to\x00kind" where from/to are raw node
	// IDs of the form "kind:path" (e.g. "module:pkg.b"). Apply ModuleKey to
	// collapse them to the same module unit used by firstParty.
	for key, c := range in.Classifications {
		parts := strings.SplitN(key, "\x00", 3)
		if len(parts) != 3 {
			continue
		}
		to := modgraph.ModuleKey(parts[1])
		if _, ok := firstParty[to]; !ok {
			continue
		}
		switch c.Strength {
		case coupling.StrengthContract:
			contractIn[to]++
		case coupling.StrengthModel, coupling.StrengthFunctional, coupling.StrengthIntrusive:
			concreteIn[to]++
		}
	}

	// Compute abstractness per module.
	type entry struct {
		module string
		a      float64
	}
	var abstract []entry
	for mod := range firstParty {
		ct := contractIn[mod]
		cc := concreteIn[mod]
		total := ct + cc
		if total == 0 {
			continue // A=0 (unknown → concrete default); skip display unless flagged
		}
		a := float64(ct) / float64(total)
		if a > 0.5 {
			abstract = append(abstract, entry{mod, a})
		}
	}
	sort.Slice(abstract, func(i, j int) bool {
		if abstract[i].a != abstract[j].a {
			return abstract[i].a > abstract[j].a
		}
		return abstract[i].module < abstract[j].module
	})

	var display string
	if len(abstract) == 0 {
		display = "0 modules with A>0.5"
	} else {
		var sb strings.Builder
		fmt.Fprintf(&sb, "%d abstract modules (A>0.5): ", len(abstract))
		for i, e := range abstract {
			if i == 5 {
				fmt.Fprintf(&sb, "+%d more", len(abstract)-5)
				break
			}
			if i > 0 {
				sb.WriteString(", ")
			}
			fmt.Fprintf(&sb, "%s (A=%.2f)", result.ShortModule(e.module), e.a)
		}
		display = sb.String()
	}

	return diagnostic.MetricResult{
		Name:       m.Name(),
		Value:      float64(len(abstract)),
		Display:    display,
		Band:       result.BandInformational,
		Confidence: result.ConfidenceLow, // proxy metric, not from SCIP type kinds
		Version:    m.Version(),
		Mode:       result.ModeRatio,
		Definition: abstractnessDef,
	}
}

func (m AbstractnessMetric) naResult() diagnostic.MetricResult {
	return diagnostic.MetricResult{
		Name: m.Name(), Value: 0, Display: result.BandNA, Band: result.BandNA,
		Confidence: result.ConfidenceLow, Version: m.Version(), Mode: result.ModeRatio,
		Definition: abstractnessDef,
	}
}

// ---------------------------------------------------------------------------
// MartinDistanceMetric
// ---------------------------------------------------------------------------

// MartinDistanceMetric reports per-module distance from the main sequence:
// Dms = |A+I-1|. Modules with Dms > 0.5 fall into the zone of pain (too
// concrete + stable: A≈0, I≈0) or the zone of uselessness (too abstract +
// unstable: A≈1, I≈1). Beyond Balanced Coupling, report-only.
type MartinDistanceMetric struct{}

// Name returns "martin_distance".
func (m MartinDistanceMetric) Name() string { return "martin_distance" }

// Version returns "martin_distance.v1".
func (m MartinDistanceMetric) Version() string { return "martin_distance.v1" }

const martinDistanceDef = "distance from main sequence Dms=|A+I-1|; >0.5 = zone of pain " +
	"(too concrete+stable) or uselessness (too abstract+unstable); beyond Balanced Coupling, report-only"

// Calculate computes Dms per module and reports those exceeding 0.5.
func (m MartinDistanceMetric) Calculate(in signal.CommonInput) diagnostic.MetricResult {
	if in.Graph == nil {
		return m.naResult()
	}

	// Reuse the two component metrics' computations.
	inst := ComputeInstability(in.Graph)
	if len(inst) < 2 {
		return m.naResult()
	}

	// Rebuild firstParty and abstractness inline (avoids coupling to the metric types).
	firstParty := make(map[string]struct{}, len(inst))
	for mod := range inst {
		firstParty[mod] = struct{}{}
	}

	contractIn := make(map[string]int, len(firstParty))
	concreteIn := make(map[string]int, len(firstParty))
	for key, c := range in.Classifications {
		parts := strings.SplitN(key, "\x00", 3)
		if len(parts) != 3 {
			continue
		}
		// Classification keys store raw node IDs ("kind:path"); apply ModuleKey
		// to match the firstParty set which is keyed by collapsed module paths.
		to := modgraph.ModuleKey(parts[1])
		if _, ok := firstParty[to]; !ok {
			continue
		}
		switch c.Strength {
		case coupling.StrengthContract:
			contractIn[to]++
		case coupling.StrengthModel, coupling.StrengthFunctional, coupling.StrengthIntrusive:
			concreteIn[to]++
		}
	}

	type entry struct {
		module string
		dms    float64
		a      float64
		i      float64
	}
	var flagged []entry
	for mod := range firstParty {
		ct := contractIn[mod]
		cc := concreteIn[mod]
		total := ct + cc
		var a float64
		if total > 0 {
			a = float64(ct) / float64(total)
		}
		i := inst[mod]
		dms := math.Abs(a + i - 1)
		if dms > 0.5 {
			flagged = append(flagged, entry{mod, dms, a, i})
		}
	}
	sort.Slice(flagged, func(x, y int) bool {
		if flagged[x].dms != flagged[y].dms {
			return flagged[x].dms > flagged[y].dms
		}
		return flagged[x].module < flagged[y].module
	})

	var display string
	if len(flagged) == 0 {
		display = "0 modules in zone of pain/uselessness (Dms>0.5)"
	} else {
		var sb strings.Builder
		fmt.Fprintf(&sb, "%d modules in zone of pain/uselessness (Dms>0.5): ", len(flagged))
		for idx, e := range flagged {
			if idx == 5 {
				fmt.Fprintf(&sb, "+%d more", len(flagged)-5)
				break
			}
			if idx > 0 {
				sb.WriteString(", ")
			}
			fmt.Fprintf(&sb, "%s (Dms=%.2f, A=%.2f, I=%.2f)",
				result.ShortModule(e.module), e.dms, e.a, e.i)
		}
		display = sb.String()
	}

	return diagnostic.MetricResult{
		Name:       m.Name(),
		Value:      float64(len(flagged)),
		Display:    display,
		Band:       result.BandInformational,
		Confidence: result.ConfidenceLow, // derived from proxy A
		Version:    m.Version(),
		Mode:       result.ModeRatio,
		Definition: martinDistanceDef,
	}
}

func (m MartinDistanceMetric) naResult() diagnostic.MetricResult {
	return diagnostic.MetricResult{
		Name: m.Name(), Value: 0, Display: result.BandNA, Band: result.BandNA,
		Confidence: result.ConfidenceLow, Version: m.Version(), Mode: result.ModeRatio,
		Definition: martinDistanceDef,
	}
}
