package classify

import (
	"path"
	"slices"
	"sort"
	"strings"

	"github.com/alexei-led/archfit/internal/config"
	"github.com/alexei-led/archfit/internal/model/coupling"
	"github.com/alexei-led/archfit/internal/model/graph"
)

// CloneOnlyPair is a cross-module clone pair (analyzers.clones) between two
// modules that share NO import-graph edge in either direction — the book's
// "duplicated knowledge" (Ch7): functional coupling through shared logic that
// is invisible to the import graph. The symmetric-strength upgrade in classify
// only fires on an existing edge, so without this detection copy-paste drift
// between unconnected modules changes no metric, finding, or score.
//
// The Classification is scored through the standard book formula with
// StrengthSymmetric at the module-pair distance and the worst-of-pair
// volatility — never a hardcoded severity. Report-only: consumed by the
// bc/duplicated_knowledge advisory (engine), never by coupling_balance or the
// gate, and no graph edge is invented — the scorer's inputs stay tool-derived
// facts.
type CloneOnlyPair struct {
	FromModule string // canonical pair order: FromModule < ToModule
	ToModule   string
	FromPath   string // representative duplicated-code file in FromModule ("" when evidence resolves none)
	ToPath     string
	Locations  []graph.Location // both sides' duplicated-code locations (sorted, deduped upstream)

	Classification coupling.Classification
}

// CloneOnlyPairs returns the scored duplicated-knowledge pairs for g: every
// cross-module clone pair in c.CrossModuleClonePairs whose modules have no
// import edge between them, no human-approved label accepting the pair, and a
// non-balanced book score (SeverityNone pairs — e.g. two frozen modules — are
// balanced by the formula and emit nothing, same as the per-edge advisory
// pipeline). Deterministic: pairs are processed in sorted key order.
func CloneOnlyPairs(g *graph.Graph, c config.ClassifyConfig) []CloneOnlyPair {
	if len(c.CrossModuleClonePairs) == 0 {
		return nil
	}
	mi := buildModuleIndex(c.Modules)
	scorer := c.Scorer
	if scorer == nil {
		scorer = coupling.DefaultScorer()
	}

	// Owner-degeneracy precomputes — same invariants as Run.
	degenerateExplicit, degenerateOwners := ownerDegeneracy(c)

	effectiveVol := computeEffectiveVolatility(g, mi, c.Modules, c.VolatilityCascadeEnabled, c.CrossModuleClonePairs)
	connected := connectedModulePairs(g, mi)

	keys := make([]string, 0, len(c.CrossModuleClonePairs))
	for k := range c.CrossModuleClonePairs {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	var out []CloneOnlyPair
	for _, key := range keys {
		if _, hasEdge := connected[key]; hasEdge {
			// An edge exists — the pair is owned by the per-edge path. Ceiling:
			// the symmetric upgrade only raises functional/unknown strengths, so
			// a pair whose edges are all contract/model/intrusive or pinned keeps
			// that strength and the clone evidence surfaces nowhere — deliberate,
			// config-authoritative and human-pinned labels are never overridden.
			continue
		}
		fromMod, toMod, ok := strings.Cut(key, "\x00")
		if !ok {
			continue
		}
		if pairLabelApproved(c.ApprovedLabels, fromMod, toMod) {
			continue // a reviewer accepted this seam — mirrors the fromPin guard
		}
		locs := c.CloneEvidence[key]
		fromPath, toPath := pairRepresentativePaths(locs, fromMod, toMod, mi)

		dist, basis := moduleDistance(fromMod, toMod, cloneLang(fromPath), c.Modules, c.ExplicitOwners, degenerateExplicit, degenerateOwners)
		cl := coupling.Classification{
			// Duplicated knowledge is bidirectional implementation-level
			// coupling with no visible connection: symmetric and implicit.
			Strength:       coupling.StrengthSymmetric,
			Distance:       dist,
			Volatility:     worseVolatility(pairVolatility(effectiveVol, fromMod), pairVolatility(effectiveVol, toMod)),
			Explicitness:   coupling.ExplicitnessImplicit,
			DistanceBasis:  basis,
			CloneLocations: locs,
		}
		cl.Score = scorer.Score(cl)
		cl.Severity = cl.Score.Band
		if cl.Severity == coupling.SeverityNone {
			continue // balanced by the book formula — no finding
		}
		out = append(out, CloneOnlyPair{
			FromModule:     fromMod,
			ToModule:       toMod,
			FromPath:       fromPath,
			ToPath:         toPath,
			Locations:      locs,
			Classification: cl,
		})
	}
	return out
}

// connectedModulePairs returns the canonical pair key of every module pair
// linked by at least one import edge in either direction.
func connectedModulePairs(g *graph.Graph, mi moduleIndex) map[string]struct{} {
	connected := make(map[string]struct{})
	if g == nil {
		return connected
	}
	for _, e := range g.Edges() {
		fromMod, okF := mi.moduleFor(pathFromID(e.From))
		toMod, okT := mi.moduleFor(pathFromID(e.To))
		if !okF || !okT || fromMod == toMod {
			continue
		}
		connected[modulePairKey(fromMod, toMod)] = struct{}{}
	}
	return connected
}

// pairLabelApproved reports whether a human-approved pinned label exists for
// the module pair in either direction. ApprovedLabels are keyed by the ordered
// (from, to) pair while clone pairs are undirected, so both orders count.
func pairLabelApproved(approved map[string]string, a, b string) bool {
	if len(approved) == 0 {
		return false
	}
	if _, ok := approved[a+"\x00"+b]; ok {
		return true
	}
	_, ok := approved[b+"\x00"+a]
	return ok
}

// pairRepresentativePaths returns the first clone location file resolving to
// each side of the pair. Locations arrive sorted (clonePairEvidence), so the
// result is deterministic. Resolution goes through ModuleForFile — the same
// file→module resolution buildClonePairSet used to form the pair key.
func pairRepresentativePaths(locs []graph.Location, fromMod, toMod string, mi moduleIndex) (fromPath, toPath string) {
	for _, l := range locs {
		mod, ok := mi.mm.ModuleForFile(l.File)
		if !ok {
			continue
		}
		switch {
		case mod == fromMod && fromPath == "":
			fromPath = l.File
		case mod == toMod && toPath == "":
			toPath = l.File
		}
	}
	return fromPath, toPath
}

// cloneLang derives the language tag for the structural-distance separator
// (moduleSegments) from a clone evidence file's extension, using the same
// convention registry ModuleForFile consults. Each extension is claimed by
// exactly one language, so map iteration order is immaterial. Empty when the
// extension is unclaimed — moduleSegments then falls back to the slash default.
func cloneLang(file string) string {
	ext := path.Ext(file)
	if ext == "" {
		return ""
	}
	for lang, conv := range graph.BuiltinConventions {
		if slices.Contains(conv.FileExtensions, ext) {
			return lang
		}
	}
	return ""
}

// volatilityRiskRank orders volatility levels by book Ch9 ordinal for
// worst-of-pair selection. High outranks undeclared/unknown (all three score
// V=10) so a declared high is reported over an artifact of missing config.
var volatilityRiskRank = map[coupling.Volatility]int{
	coupling.VolatilityFrozen:     1,
	coupling.VolatilityLow:        2,
	coupling.VolatilityMedium:     3,
	coupling.VolatilityUndeclared: 4,
	coupling.VolatilityUnknown:    4,
	coupling.VolatilityHigh:       5,
}

// worseVolatility returns the riskier of two volatility levels. A clone pair
// is symmetric — a change to the shared logic propagates in both directions —
// so the more volatile side drives the pair's risk.
func worseVolatility(a, b coupling.Volatility) coupling.Volatility {
	if volatilityRiskRank[b] > volatilityRiskRank[a] {
		return b
	}
	return a
}

// pairVolatility reads a module's effective (post-cascade) volatility,
// falling back to unknown for a module the effective map does not know.
func pairVolatility(effectiveVol map[string]coupling.Volatility, mod string) coupling.Volatility {
	if v, ok := effectiveVol[mod]; ok {
		return v
	}
	return coupling.VolatilityUnknown
}
