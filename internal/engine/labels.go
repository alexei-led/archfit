package engine

import (
	"path/filepath"
	"sort"

	"github.com/alexei-led/archfit/internal/config"
	"github.com/alexei-led/archfit/internal/labels"
	"github.com/alexei-led/archfit/internal/model/clone"
	"github.com/alexei-led/archfit/internal/model/fileclass"
	"github.com/alexei-led/archfit/internal/model/finding"
	"github.com/alexei-led/archfit/internal/model/graph"
	"github.com/alexei-led/archfit/internal/syntax"
)

// applyPinnedLabels validates pinned labels and injects the approved ones into
// the classify config (precedence: config globs > approved labels > extractor
// hint). Freshness is checked against the full import graph — on full runs
// only (a delta graph is partial and would false-stale every label). Returns
// one labels/stale advisory per ignored stale label plus the count of approved
// labels with llm provenance (used to lower coupling_balance confidence).
// Deterministic — the gate never calls an LLM; labels are reviewed YAML.
func applyPinnedLabels(g *graph.Graph, classifyCfg *config.ClassifyConfig, mode Mode, lbls []labels.Label) ([]finding.Finding, int) {
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

	llmCount := labels.LLMApprovedCount(lbls, evidence)

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
	return out, llmCount
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
// for the clone-driven classify paths (Symmetric-strength upgrade, cascade
// exclusion, duplicated-knowledge pairing), plus the real
// duplicated-code locations backing each pair (see clonePairEvidence).
// Keys are "[a]\x00[b]" with a≤b (canonical sorted pair, from clone.ModulePairs).
//
// Clusters where any file is Test or Generated are excluded — test/mock duplication
// must not trigger a StrengthSymmetric upgrade on production coupling edges (C4).
// index is the FileClassIndex from the loc walk (nil is safe — falls back to
// built-in filename heuristics: mock_*.go, _test.go, *.pb.go, etc.).
func buildClonePairSet(clusters []clone.Cluster, mm config.ModuleMap, index map[string]fileclass.FileClass) (map[string]struct{}, map[string][]graph.Location) {
	prodClusters := make([]clone.Cluster, 0, len(clusters))
	cfg := syntax.FileClassConfig{} // empty: index already encodes user config patterns; fallback uses built-ins
	for _, c := range clusters {
		if clusterHasTestOrGenerated(c.Files, index, cfg) {
			continue
		}
		prodClusters = append(prodClusters, c)
	}
	moduleFor := func(f string) string {
		mod, ok := mm.ModuleForFile(f)
		if !ok {
			return ""
		}
		return mod
	}
	pairs := clone.ModulePairs(prodClusters, moduleFor)
	set := make(map[string]struct{}, len(pairs))
	for _, p := range pairs {
		// clone.ModulePairs already returns sorted pairs [a,b] with a≤b.
		set[p[0]+"\x00"+p[1]] = struct{}{}
	}
	return set, clonePairEvidence(prodClusters, moduleFor)
}

// clonePairEvidence maps each canonical module-pair key to the real
// duplicated-code locations (both sides) that produced the pairing, so a
// Symmetric-strength finding (classify.go) can cite jscpd's actual file:line
// instead of only the edge's baseline provenance (e.g. a Rust crate's
// Cargo.toml:0). Built from the same production-only clusters as
// buildClonePairSet; deterministic (sorted, deduped) per key. A cluster whose
// report carried no start-line data still contributes a Location with Line: 0 —
// the file path alone is still more specific than the baseline provenance.
func clonePairEvidence(clusters []clone.Cluster, moduleFor func(string) string) map[string][]graph.Location {
	out := make(map[string][]graph.Location)
	for _, c := range clusters {
		for i := 0; i < len(c.Files); i++ {
			modI := moduleFor(c.Files[i])
			if modI == "" {
				continue
			}
			for j := i + 1; j < len(c.Files); j++ {
				modJ := moduleFor(c.Files[j])
				if modJ == "" || modJ == modI {
					continue
				}
				a, b := modI, modJ
				if a > b {
					a, b = b, a
				}
				key := a + "\x00" + b
				out[key] = appendUniqueLocation(out[key], c.Files[i], cloneStartLine(c, i))
				out[key] = appendUniqueLocation(out[key], c.Files[j], cloneStartLine(c, j))
			}
		}
	}
	for k, locs := range out {
		sort.Slice(locs, func(i, j int) bool {
			if locs[i].File != locs[j].File {
				return locs[i].File < locs[j].File
			}
			return locs[i].Line < locs[j].Line
		})
		out[k] = locs
	}
	return out
}

// cloneStartLine returns the start line jscpd reported for Files[i] in c, or 0
// when the cluster carries no per-file location data.
func cloneStartLine(c clone.Cluster, i int) int {
	if i < len(c.Locations) {
		return c.Locations[i].StartLine
	}
	return 0
}

// appendUniqueLocation appends loc to locs unless an identical entry is
// already present.
func appendUniqueLocation(locs []graph.Location, file string, line int) []graph.Location {
	loc := graph.Location{File: file, Line: line}
	for _, l := range locs {
		if l == loc {
			return locs
		}
	}
	return append(locs, loc)
}

// clusterHasTestOrGenerated reports whether any file in the cluster is not a
// production file (test, generated, or vendor). Kept in engine/ to avoid an
// engine → metrics/modularity import (wrong dependency direction).
func clusterHasTestOrGenerated(files []string, index map[string]fileclass.FileClass, cfg syntax.FileClassConfig) bool {
	for _, f := range files {
		lang := cloneLangFromExt(filepath.Ext(f))
		fc := syntax.LookupFileClass(filepath.ToSlash(f), index, lang, cfg)
		if !fileclass.IsProduction(fc) {
			return true
		}
	}
	return false
}

// cloneLangFromExt maps a file extension to a language tag for LookupFileClass.
// Only the languages supported by archfit's extractors need coverage here.
func cloneLangFromExt(ext string) string {
	switch ext {
	case ".go":
		return "go"
	case ".ts", ".tsx":
		return "typescript"
	case ".js", ".jsx", ".mjs", ".cjs":
		return "javascript"
	case ".py":
		return "python"
	case ".rs":
		return "rust"
	default:
		return ""
	}
}
