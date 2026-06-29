package engine

import (
	"path/filepath"

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
// for CoA (connascence of algorithm) tagging in classify.
// Keys are "[a]\x00[b]" with a≤b (canonical sorted pair, from clone.ModulePairs).
//
// Clusters where any file is Test or Generated are excluded — test/mock duplication
// must not trigger a StrengthSymmetric upgrade on production coupling edges (C4).
// index is the FileClassIndex from the loc walk (nil is safe — falls back to
// built-in filename heuristics: mock_*.go, _test.go, *.pb.go, etc.).
func buildClonePairSet(clusters []clone.Cluster, mm config.ModuleMap, index map[string]fileclass.FileClass) map[string]struct{} {
	prodClusters := make([]clone.Cluster, 0, len(clusters))
	cfg := syntax.FileClassConfig{} // empty: index already encodes user config patterns; fallback uses built-ins
	for _, c := range clusters {
		if clusterHasTestOrGenerated(c.Files, index, cfg) {
			continue
		}
		prodClusters = append(prodClusters, c)
	}
	pairs := clone.ModulePairs(prodClusters, func(f string) string {
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
